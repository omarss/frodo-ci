package security

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecFunc invokes a security tool in dir and returns its stdout and exit code.
// It is a Scanner field so tests can inject canned tool output without the tool
// installed. A non-zero exit is returned in exitCode (not as err) because most
// scanners exit non-zero precisely when they find something.
type ExecFunc func(ctx context.Context, dir, name string, args ...string) (stdout []byte, exitCode int, err error)

func execCommand(ctx context.Context, dir, name string, args ...string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		return out, ee.ExitCode(), nil
	}
	if err != nil {
		return nil, -1, err
	}
	return out, 0, nil
}

// scanModule runs a single scan and returns its findings. SARIF-capable tools
// are parsed uniformly; validators without SARIF gate on exit code.
func (s *Scanner) scanModule(ctx context.Context, sc Scan, tool, dir string) ([]Finding, error) {
	switch tool {
	case "semgrep":
		out, _, err := s.exec(ctx, dir, "semgrep", "scan", "--sarif", "--quiet", "--config", "auto", ".")
		if err != nil {
			return nil, err
		}
		return parseSARIF(out, sc.Type, tool)

	case "hadolint":
		var all []Finding
		for _, df := range findDockerfiles(dir) {
			out, _, err := s.exec(ctx, dir, "hadolint", "--format", "sarif", "--no-fail", df)
			if err != nil {
				return nil, err
			}
			fs, perr := parseSARIF(out, sc.Type, tool)
			if perr != nil {
				return nil, perr
			}
			all = append(all, fs...)
		}
		return all, nil

	case "trivy":
		return s.sarifToFile(ctx, sc.Type, tool, dir, func(out string) []string {
			return []string{"fs", "--quiet", "--format", "sarif", "--output", out, "."}
		})

	case "gitleaks":
		return s.sarifToFile(ctx, sc.Type, tool, dir, func(out string) []string {
			return []string{"detect", "--no-git", "--no-banner", "--exit-code", "0",
				"--report-format", "sarif", "--report-path", out, "--source", "."}
		})

	default:
		// Validators without SARIF (kubeconform, actionlint): a non-zero exit is a
		// single blocking finding carrying the tool's last line of output.
		out, code, err := s.exec(ctx, dir, tool, validatorArgs(tool)...)
		if err != nil {
			return nil, err
		}
		if code == 0 {
			return nil, nil
		}
		return []Finding{{RuleID: string(sc.Type), Kind: sc.Type, Tool: tool, Severity: "high",
			Message: tool + ": " + lastLine(out)}}, nil
	}
}

// sarifToFile runs a tool that writes its SARIF to a file, then parses it.
// A missing or empty report is treated as zero findings (e.g. gitleaks writes
// no report when it finds nothing).
func (s *Scanner) sarifToFile(ctx context.Context, kind ScanType, tool, dir string, args func(out string) []string) ([]Finding, error) {
	tmp, err := os.CreateTemp("", "frodo-sarif-*.json")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	if _, _, err := s.exec(ctx, dir, tool, args(path)...); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || len(data) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseSARIF(data, kind, tool)
}

func validatorArgs(tool string) []string {
	switch tool {
	case "kubeconform":
		return []string{"-strict", "-summary", "."}
	case "actionlint":
		return []string{"-no-color"}
	}
	return nil
}

// findDockerfiles collects Dockerfiles under dir, skipping heavy/irrelevant
// directories so a scan does not descend into dependencies.
func findDockerfiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "vendor", ".git", "dist", "target":
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name == "Dockerfile" || strings.HasSuffix(name, ".dockerfile") || strings.HasSuffix(name, ".Dockerfile") {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

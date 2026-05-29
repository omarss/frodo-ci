package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

// TestRootRegistersAllCommands guards the public CLI surface: every command in
// the requirements must be wired into the root command.
func TestRootRegistersAllCommands(t *testing.T) {
	root := NewRootCommand()
	want := []string{
		"init", "init-module", "validate-config", "lint-config", "plan",
		"run", "ci", "cd", "review", "explain", "doctor", "schemas",
		"templates", "fingerprint",
	}
	have := make(map[string]bool, len(root.Commands()))
	for _, c := range root.Commands() {
		have[c.Name()] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("root command is missing subcommand %q", w)
		}
	}
}

// TestSubcommandTrees guards nested subcommands.
func TestSubcommandTrees(t *testing.T) {
	root := NewRootCommand()
	cases := map[string][]string{
		"schemas":   {"export"},
		"templates": {"list", "explain"},
	}
	for parent, children := range cases {
		cmd := findCommand(root.Commands(), parent)
		if cmd == nil {
			t.Fatalf("missing parent command %q", parent)
		}
		for _, child := range children {
			if findCommand(cmd.Commands(), child) == nil {
				t.Errorf("command %q is missing subcommand %q", parent, child)
			}
		}
	}
}

// TestHelpRuns ensures the command tree renders help without error.
func TestHelpRuns(t *testing.T) {
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--help returned an error: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected --help to produce output")
	}
}

func findCommand(cmds []*cobra.Command, name string) *cobra.Command {
	for _, c := range cmds {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

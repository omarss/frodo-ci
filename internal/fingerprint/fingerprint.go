// Package fingerprint computes the deterministic hash that decides whether a
// stage's expensive work can be skipped. The same repository state, config,
// templates, toolchains, inputs, and dependency fingerprints always produce the
// same fingerprint; any change flips it, so the cache only ever skips work on an
// exact match.
package fingerprint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

const version = "frodo-fp-v1"

// Hasher accumulates length-prefixed, labeled fields into a digest. Length
// prefixing makes the encoding unambiguous so distinct inputs cannot collide by
// concatenation.
type Hasher struct{ h hash.Hash }

// New returns a Hasher seeded with the fingerprint format version.
func New() *Hasher {
	f := &Hasher{h: sha256.New()}
	return f.Text("version", version)
}

// Field mixes a labeled byte field into the digest.
func (f *Hasher) Field(label string, data []byte) *Hasher {
	writeLP(f.h, []byte(label))
	writeLP(f.h, data)
	return f
}

// Text mixes a labeled string field into the digest.
func (f *Hasher) Text(label, s string) *Hasher { return f.Field(label, []byte(s)) }

// Sum returns the hex-encoded digest.
func (f *Hasher) Sum() string { return hex.EncodeToString(f.h.Sum(nil)) }

func writeLP(h hash.Hash, b []byte) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], uint64(len(b)))
	_, _ = h.Write(buf[:n])
	_, _ = h.Write(b)
}

// FileDigest returns the hex SHA-256 of a file's contents, or the sentinel
// "absent" when the file does not exist (so presence/absence is captured).
func FileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "absent", nil
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// FilesDigest hashes a set of repo-relative files deterministically: the paths
// are sorted and de-duplicated, and each path is mixed in with its content
// digest so both the set and the contents matter.
func FilesDigest(root string, relPaths []string) (string, error) {
	sorted := dedupeSorted(relPaths)
	h := New()
	for _, rel := range sorted {
		d, err := FileDigest(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		h.Text("path", rel).Text("digest", d)
	}
	return h.Sum(), nil
}

// StageInputs collects everything that affects a single stage's fingerprint.
type StageInputs struct {
	RootConfig              []byte
	Toolchains              []byte
	SecurityBaselineVersion int
	LintRulesVersion        int
	ModuleConfig            []byte
	StageFile               []byte   // optional stage-override file
	Template                []byte   // optional resolved template
	Files                   []string // repo-relative matched files, raw inputs, and lockfiles
	DependencyFingerprints  []string // fingerprints of dependency modules
}

// Compute produces the deterministic fingerprint for a stage.
func Compute(root string, in StageInputs) (string, error) {
	filesDigest, err := FilesDigest(root, in.Files)
	if err != nil {
		return "", err
	}
	h := New().
		Field("root", in.RootConfig).
		Field("toolchains", in.Toolchains).
		Text("security-baseline-version", strconv.Itoa(in.SecurityBaselineVersion)).
		Text("lint-rules-version", strconv.Itoa(in.LintRulesVersion)).
		Field("module", in.ModuleConfig).
		Field("stage", in.StageFile).
		Field("template", in.Template).
		Text("files", filesDigest)
	for _, d := range dedupeSorted(in.DependencyFingerprints) {
		h.Text("dependency", d)
	}
	return h.Sum(), nil
}

// Short returns a display-friendly prefix of a fingerprint.
func Short(fp string) string {
	if len(fp) <= 12 {
		return fp
	}
	return fp[:12]
}

func dedupeSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

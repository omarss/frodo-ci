package cache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a file (and parents) with content under dir.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveRestoreOutputs(t *testing.T) {
	c, err := NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	writeFile(t, src, "dist/index.js", "console.log(1)")
	writeFile(t, src, "dist/sub/util.js", "module.exports={}")
	writeFile(t, src, "dist/types.d.ts", "export const x: number")
	writeFile(t, src, "src/index.ts", "should not be archived")

	fp := "abc123def456"
	if err := c.SaveOutputs(fp, src, []string{"dist", "missing"}); err != nil {
		t.Fatalf("SaveOutputs: %v", err)
	}

	// Restore into a fresh directory and assert the dist tree comes back exactly,
	// and that non-declared paths (src/) are not present.
	dst := t.TempDir()
	ok, err := c.RestoreOutputs(fp, dst)
	if err != nil || !ok {
		t.Fatalf("RestoreOutputs ok=%v err=%v", ok, err)
	}
	for rel, want := range map[string]string{
		"dist/index.js":    "console.log(1)",
		"dist/sub/util.js": "module.exports={}",
		"dist/types.d.ts":  "export const x: number",
	} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("restored %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("restored %s = %q, want %q", rel, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "src")); !os.IsNotExist(err) {
		t.Error("src/ should not have been archived/restored")
	}
}

func TestRestoreOutputsMiss(t *testing.T) {
	c, err := NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ok, err := c.RestoreOutputs("nosuchfingerprint", t.TempDir())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Error("restore of an unknown fingerprint should report a miss")
	}
}

func TestSaveOutputsNothingToArchive(t *testing.T) {
	c, err := NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// All declared outputs are absent -> no archive written -> later restore misses.
	if err := c.SaveOutputs("fp", t.TempDir(), []string{"dist", "target"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := c.RestoreOutputs("fp", t.TempDir()); ok {
		t.Error("no archive should have been written when nothing existed")
	}
}

// TestExtractTarRejectsTraversal proves the zip-slip guard: an archive whose
// entry escapes the base directory is refused rather than written outside it.
func TestExtractTarRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("pwned")
	_ = tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()

	gr, _ := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	base := t.TempDir()
	if err := extractTar(tar.NewReader(gr), base); err == nil {
		t.Fatal("expected traversal entry to be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(base), "escape.txt")); !os.IsNotExist(err) {
		t.Error("traversal entry must not be written outside the base dir")
	}
}

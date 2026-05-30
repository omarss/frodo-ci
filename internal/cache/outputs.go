package cache

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// outputsPath is where a fingerprint's archived build outputs live, sharded the
// same way as the JSON marker.
func (c *LocalCache) outputsPath(fingerprint string) string {
	shard := "00"
	if len(fingerprint) >= 2 {
		shard = fingerprint[:2]
	}
	return filepath.Join(c.dir, shard, fingerprint+".outputs.tgz")
}

// SaveOutputs archives paths (relative to baseDir) into a single gzip-compressed
// tar keyed by the fingerprint. Missing paths are skipped so a stage that did
// not produce every declared output still caches what it did. Writing is atomic
// (temp-then-rename) so a crash never leaves a half-written archive. If nothing
// exists to archive, it is a no-op.
func (c *LocalCache) SaveOutputs(fingerprint, baseDir string, paths []string) error {
	if fingerprint == "" {
		return errors.New("cache: SaveOutputs requires a fingerprint")
	}
	if len(paths) == 0 {
		return nil
	}
	dst := c.outputsPath(fingerprint)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-out-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)

	written, werr := writeTar(tw, baseDir, paths)
	// Close in order; capture the first error.
	cerr := tw.Close()
	if gzerr := gz.Close(); cerr == nil {
		cerr = gzerr
	}
	if clo := tmp.Close(); cerr == nil {
		cerr = clo
	}
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		if werr != nil {
			return werr
		}
		return cerr
	}
	if written == 0 { // nothing on disk matched the declared outputs
		_ = os.Remove(tmpName)
		return nil
	}
	return os.Rename(tmpName, dst)
}

// RestoreOutputs extracts a fingerprint's archived outputs into baseDir,
// reporting whether an archive was present. A missing archive is a clean miss.
func (c *LocalCache) RestoreOutputs(fingerprint, baseDir string) (bool, error) {
	f, err := os.Open(c.outputsPath(fingerprint))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return false, err
	}
	defer func() { _ = gz.Close() }()

	if err := extractTar(tar.NewReader(gz), baseDir); err != nil {
		return false, err
	}
	return true, nil
}

// writeTar walks each path (relative to baseDir) and writes its files into tw
// under baseDir-relative names. Returns the number of entries written.
func writeTar(tw *tar.Writer, baseDir string, paths []string) (int, error) {
	count := 0
	for _, p := range paths {
		rel := filepath.Clean(filepath.FromSlash(p))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue // never archive outside the module dir
		}
		root := filepath.Join(baseDir, rel)
		err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil // a declared output the stage didn't produce
				}
				return err
			}
			name, rerr := filepath.Rel(baseDir, path)
			if rerr != nil {
				return rerr
			}
			link := ""
			if info.Mode()&os.ModeSymlink != 0 {
				if link, err = os.Readlink(path); err != nil {
					return err
				}
			}
			hdr, herr := tar.FileInfoHeader(info, link)
			if herr != nil {
				return herr
			}
			hdr.Name = filepath.ToSlash(name)
			if info.IsDir() {
				hdr.Name += "/"
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			count++
			if !info.Mode().IsRegular() {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()
			_, err = io.Copy(tw, file)
			return err
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

// extractTar writes a tar stream into baseDir, guarding against path-traversal
// ("zip slip") so a malformed archive can never write outside baseDir.
func extractTar(tr *tar.Reader, baseDir string) error {
	cleanBase := filepath.Clean(baseDir)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(cleanBase, filepath.FromSlash(hdr.Name))
		if target != cleanBase && !strings.HasPrefix(target, cleanBase+string(os.PathSeparator)) {
			return fmt.Errorf("cache: archive entry %q escapes the module directory", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, fs.FileMode(hdr.Mode)|0o700); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil { //nolint:gosec // size bounded by our own archive
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

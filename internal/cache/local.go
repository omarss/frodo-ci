package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// LocalCache is a filesystem-backed cache. Entries are JSON files sharded by the
// first two hex characters of the fingerprint to avoid huge flat directories.
type LocalCache struct {
	dir string
}

// NewLocalCache creates (if needed) and returns a filesystem cache rooted at dir.
func NewLocalCache(dir string) (*LocalCache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir %s: %w", dir, err)
	}
	return &LocalCache{dir: dir}, nil
}

func (c *LocalCache) path(fingerprint string) string {
	shard := "00"
	if len(fingerprint) >= 2 {
		shard = fingerprint[:2]
	}
	return filepath.Join(c.dir, shard, fingerprint+".json")
}

// Has reports whether an exact fingerprint match exists.
func (c *LocalCache) Has(fingerprint string) (bool, error) {
	_, err := os.Stat(c.path(fingerprint))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Restore returns the cached entry for a fingerprint, if present.
func (c *LocalCache) Restore(fingerprint string) (Entry, bool, error) {
	data, err := os.ReadFile(c.path(fingerprint))
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		// A corrupt entry is treated as a miss so the stage simply re-runs.
		return Entry{}, false, nil
	}
	return e, true, nil
}

// Save writes an entry atomically (write-temp-then-rename) so a crash can never
// leave a half-written marker that would be mistaken for a valid cache hit.
func (c *LocalCache) Save(entry Entry) error {
	if entry.Fingerprint == "" {
		return errors.New("cache: entry has no fingerprint")
	}
	dst := c.path(entry.Fingerprint)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dst)
}

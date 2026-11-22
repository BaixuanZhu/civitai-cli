package hashcache

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"civitai-cli/internal/app"
)

// Entry is one cached file hash. A hit requires both Size and MTime to
// match the current file; either changing invalidates the entry so the
// file is rehashed (parent design D8). MTime round-trips through JSON as
// RFC3339Nano, preserving the instant for time.Time.Equal comparison.
type Entry struct {
	Size   int64     `json:"size"`
	MTime  time.Time `json:"mtime"`
	SHA256 string    `json:"sha256"`
}

// Cache maps absolute file paths to hash entries and persists them at
// os.UserCacheDir()/civ/cache/hashes.json. A Cache is safe for concurrent
// use (the sync worker pool calls Get/Put from several goroutines). The
// zero value is not usable; obtain one from Load. A nil *Cache is a valid
// disabled cache: Get misses, Put and Save are no-ops — the cmd layer
// passes nil when CIV_NO_CACHE is set, keeping this package env-agnostic.
type Cache struct {
	mu      sync.Mutex
	path    string
	entries map[string]Entry
	dirty   bool
}

// Load reads the hash cache from os.UserCacheDir()/civ/cache/hashes.json.
// A missing file yields an empty cache. A corrupt or unreadable file warns
// on stderr and rebuilds an empty cache (degradation is never fatal) and
// marks it dirty so the next Save heals the file. When the cache directory
// cannot be determined, Load returns an empty cache with no path (Save is
// then a no-op). Load never returns nil; the cmd layer decides disablement.
func Load() *Cache {
	c := &Cache{entries: make(map[string]Entry)}
	dir, err := os.UserCacheDir()
	if err != nil {
		app.Warnf("hash cache disabled: cannot determine cache directory: %v", err)
		return c
	}
	c.path = filepath.Join(dir, "civ", "cache", "hashes.json")
	b, err := os.ReadFile(c.path)
	if err != nil {
		if !os.IsNotExist(err) {
			app.Warnf("hash cache unreadable (%v); starting empty", err)
		}
		return c
	}
	if err := json.Unmarshal(b, &c.entries); err != nil {
		app.Warnf("hash cache corrupt (%v); rebuilding", err)
		c.entries = make(map[string]Entry)
		c.dirty = true
	}
	return c
}

// Get returns the cached SHA256 for path when the entry's size and mtime
// both match fi. A nil receiver, a missing entry, or any size/mtime drift
// yields ("", false). mtime uses fi.ModTime().Equal so JSON round-trip
// precision does not cause spurious misses.
func (c *Cache) Get(path string, fi fs.FileInfo) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[path]
	if !ok {
		return "", false
	}
	if e.Size != fi.Size() || !e.MTime.Equal(fi.ModTime()) {
		return "", false
	}
	return e.SHA256, true
}

// Put records the hash for path keyed by fi's size and mtime, marking the
// cache dirty. A nil receiver is a no-op (disabled cache).
func (c *Cache) Put(path string, fi fs.FileInfo, hash string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[path] = Entry{Size: fi.Size(), MTime: fi.ModTime(), SHA256: hash}
	c.dirty = true
}

// Save persists the cache atomically (temp file in the same directory +
// rename) only when dirty. A nil receiver, a clean cache, or an unknown
// path is a no-op returning nil. The parent directory is created 0700.
// On any write failure the temp file is removed and the error returned;
// the previous cache file is never left half-written.
func (c *Cache) Save() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty || c.path == "" {
		return nil
	}
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("hashcache: create %s: %w", dir, err)
	}
	b, err := json.Marshal(c.entries)
	if err != nil {
		return fmt.Errorf("hashcache: encode: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "hashes-*.json.tmp")
	if err != nil {
		return fmt.Errorf("hashcache: create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("hashcache: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("hashcache: close temp: %w", err)
	}
	if err := os.Rename(tmpName, c.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("hashcache: rename: %w", err)
	}
	c.dirty = false
	return nil
}

package hashcache

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// setCacheDir points os.UserCacheDir at dir for the test so Load/Save never
// touch a real user cache. Windows resolves it from %LocalAppData%; Unix
// from $XDG_CACHE_HOME.
func setCacheDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("LocalAppData", dir)
	} else {
		t.Setenv("XDG_CACHE_HOME", dir)
	}
}

// newTestCache builds an in-memory cache backed by a temp-dir path (no env
// juggling) for Get/Put/Save semantics.
func newTestCache(t *testing.T) *Cache {
	t.Helper()
	return &Cache{path: filepath.Join(t.TempDir(), "hashes.json"), entries: make(map[string]Entry)}
}

func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(old); log.SetFlags(oldFlags) })
	fn()
	return buf.String()
}

func TestCache_GetPutHit(t *testing.T) {
	path := writeTemp(t, "m.safetensors", []byte("content"))
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	c := newTestCache(t)
	if _, ok := c.Get(path, fi); ok {
		t.Fatalf("empty cache must miss")
	}
	c.Put(path, fi, "HASH")
	got, ok := c.Get(path, fi)
	if !ok || got != "HASH" {
		t.Errorf("Get after Put = (%q,%v), want (HASH,true)", got, ok)
	}
}

func TestCache_MTimeInvalidation(t *testing.T) {
	path := writeTemp(t, "m.safetensors", []byte("content"))
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	c := newTestCache(t)
	c.Put(path, fi, "HASH")

	// Bump mtime forward; size is unchanged, so a miss proves mtime gating.
	newTime := fi.ModTime().Add(3 * time.Second)
	if err := os.Chtimes(path, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat2: %v", err)
	}
	if fi2.Size() != fi.Size() {
		t.Fatalf("precondition: size must be unchanged")
	}
	if _, ok := c.Get(path, fi2); ok {
		t.Errorf("mtime change must invalidate the entry")
	}
	// The original fi still hits (Get compares against the passed fi).
	if _, ok := c.Get(path, fi); !ok {
		t.Errorf("original fi must still hit")
	}
}

func TestCache_SizeInvalidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "m.safetensors")
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	origMTime := fi.ModTime()
	c := newTestCache(t)
	c.Put(path, fi, "HASH")

	// Grow the file, then restore the original mtime so size is the only
	// difference (isolates size gating from mtime gating).
	if err := os.WriteFile(path, []byte("short-and-longer"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if err := os.Chtimes(path, origMTime, origMTime); err != nil {
		t.Fatalf("chtimes restore: %v", err)
	}
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat2: %v", err)
	}
	if !fi2.ModTime().Equal(origMTime) {
		t.Skipf("filesystem mtime precision prevents isolating size (got %v want %v)", fi2.ModTime(), origMTime)
	}
	if fi2.Size() == fi.Size() {
		t.Fatalf("precondition: size must differ")
	}
	if _, ok := c.Get(path, fi2); ok {
		t.Errorf("size change must invalidate the entry")
	}
}

func TestCache_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	path := writeTemp(t, "m.safetensors", []byte("round-trip-content"))
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	c := Load()
	c.Put(path, fi, "ROUNDTRIPHASH")
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The cache file must exist at the documented path; no temp leftovers.
	cacheFile := filepath.Join(dir, "civ", "cache", "hashes.json")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("cache file must exist after Save: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(cacheFile))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("atomic write left a temp file: %s", e.Name())
		}
	}
	// A fresh Load must hit with a re-stat fi (RFC3339Nano mtime fidelity).
	c2 := Load()
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat2: %v", err)
	}
	got, ok := c2.Get(path, fi2)
	if !ok || got != "ROUNDTRIPHASH" {
		t.Errorf("round-trip Get = (%q,%v), want (ROUNDTRIPHASH,true)", got, ok)
	}
}

func TestCache_SaveOnlyWhenDirty(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	cacheFile := filepath.Join(dir, "civ", "cache", "hashes.json")

	// A fresh Load of a missing file is clean: Save must not create it.
	c := Load()
	if err := c.Save(); err != nil {
		t.Fatalf("clean Save: %v", err)
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Errorf("clean cache must not write a file, stat err = %v", err)
	}
	// After a Put the cache is dirty and Save writes it.
	path := writeTemp(t, "m.safetensors", []byte("x"))
	fi, _ := os.Stat(path)
	c.Put(path, fi, "H")
	if err := c.Save(); err != nil {
		t.Fatalf("dirty Save: %v", err)
	}
	if _, err := os.Stat(cacheFile); err != nil {
		t.Errorf("dirty cache must write a file: %v", err)
	}
}

func TestCache_CorruptDegrades(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	cacheFile := filepath.Join(dir, "civ", "cache", "hashes.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cacheFile, []byte("{corrupt"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	var c *Cache
	logs := captureLog(t, func() { c = Load() })
	if c == nil {
		t.Fatalf("corrupt cache must rebuild, not return nil")
	}
	if !strings.Contains(logs, "corrupt") {
		t.Errorf("corrupt cache must warn on stderr, logs: %s", logs)
	}
	// The rebuilt cache is usable and dirty so Save heals the file.
	path := writeTemp(t, "m.safetensors", []byte("y"))
	fi, _ := os.Stat(path)
	c.Put(path, fi, "HEAL")
	if err := c.Save(); err != nil {
		t.Fatalf("heal Save: %v", err)
	}
	c2 := Load()
	if got, ok := c2.Get(path, fi); !ok || got != "HEAL" {
		t.Errorf("healed cache must hit, got (%q,%v)", got, ok)
	}
}

func TestCache_NilReceiverSafe(t *testing.T) {
	var c *Cache // disabled cache (CIV_NO_CACHE)
	path := writeTemp(t, "m.safetensors", []byte("z"))
	fi, _ := os.Stat(path)
	if got, ok := c.Get(path, fi); ok || got != "" {
		t.Errorf("nil Get must miss, got (%q,%v)", got, ok)
	}
	c.Put(path, fi, "H") // must not panic
	if err := c.Save(); err != nil {
		t.Errorf("nil Save must be a no-op returning nil, got %v", err)
	}
}

func TestCache_ConcurrentGetPut(t *testing.T) {
	// The sync worker pool calls Get/Put concurrently; the mutex must keep
	// the map race-free (validated under -race). Files are created up front
	// so worker goroutines never call testing.T helpers.
	c := newTestCache(t)
	dir := t.TempDir()
	const n = 32
	paths := make([]string, n)
	fis := make([]fs.FileInfo, n)
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, fmt.Sprintf("m%d.safetensors", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("content-%d", i)), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		paths[i], fis[i] = p, fi
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Put(paths[i], fis[i], "H")
			c.Get(paths[i], fis[i])
		}(i)
	}
	wg.Wait()
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for i := 0; i < n; i++ {
		if got, ok := c.Get(paths[i], fis[i]); !ok || got != "H" {
			t.Errorf("entry %d lost after concurrent Put/Save: (%q,%v)", i, got, ok)
		}
	}
}

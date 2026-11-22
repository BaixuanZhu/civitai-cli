package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"civitai-cli/internal/api"
)

// syncVersionBody is a synthetic by-hash hit: model summary, two trigger
// words, and a primary file with clean scan results.
const syncVersionBody = `{"id":555,"modelId":777,"name":"v1","baseModel":"SD 1.5",` +
	`"trainedWords":["a","b"],"model":{"name":"TestModel","type":"LORA"},` +
	`"files":[{"name":"m.safetensors","primary":true,"pickleScanResult":"Success","virusScanResult":"Success","hashes":{"SHA256":"X"}}]}`

// setCacheDir points os.UserCacheDir at dir so the hash cache is isolated
// and controllable across runs. Windows uses %LocalAppData%; Unix
// $XDG_CACHE_HOME.
func setCacheDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("LocalAppData", dir)
	} else {
		t.Setenv("XDG_CACHE_HOME", dir)
	}
}

func hashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func writeSyncFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// syncRouteHandler serves by-hash routes keyed on the queried hash; any
// unmapped hash gets the API's 404 shape (a not_found data row).
func syncRouteHandler(routes map[string]string) http.HandlerFunc {
	const prefix = "/model-versions/by-hash/"
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, prefix) {
			if body, ok := routes[strings.TrimPrefix(r.URL.Path, prefix)]; ok {
				w.Write([]byte(body))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Model not found"}`))
	}
}

func runSyncCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	var o syncOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runSync(context.Background(), &env.g, pos, &o)
}

// countHashCalls wraps computeHash with a call counter so tests can prove
// cache hits skip recomputation.
func countHashCalls(t *testing.T) *atomic.Int64 {
	t.Helper()
	var calls atomic.Int64
	old := computeHash
	computeHash = func(path string) (string, error) {
		calls.Add(1)
		return old(path)
	}
	t.Cleanup(func() { computeHash = old })
	return &calls
}

// syncRowsJSON runs sync with --json and decodes the row array.
func syncRowsJSON(t *testing.T, env *testEnv, args ...string) []syncRow {
	t.Helper()
	env.buf.Reset()
	if err := runSyncCmd(t, env, append(args, "--json")...); err != nil {
		t.Fatalf("sync --json: %v", err)
	}
	var rows []syncRow
	if err := json.Unmarshal(env.buf.Bytes(), &rows); err != nil {
		t.Fatalf("json rows must parse: %v\n%s", err, env.buf.String())
	}
	return rows
}

func TestSync_FoundNotFoundDupe_Table(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "")
	dataDir := t.TempDir()
	pa := writeSyncFile(t, dataDir, "a.safetensors", "registered")
	pb := writeSyncFile(t, dataDir, "b.safetensors", "registered") // same hash → dupe
	pc := writeSyncFile(t, dataDir, "c.safetensors", "unknown")    // not_found
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	if err := runSyncCmd(t, env, dataDir); err != nil {
		t.Fatalf("all-success sync must return nil (exit 0), got %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{"FILE", "STATUS", "MODEL_URL", "TestModel", "v1", "SD 1.5", "https://civitai.com/models/777", pa, pb, pc} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "found dupe"); n != 2 {
		t.Errorf("two files share a version → 2 'found dupe' rows, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "not_found") {
		t.Errorf("unknown hash must render a not_found row:\n%s", out)
	}
	// Progress goes to stderr, never stdout.
	if strings.Contains(out, "[INFO]") {
		t.Errorf("stdout must not carry progress logs:\n%s", out)
	}
	if !strings.Contains(env.logs.String(), "sync 3/3") {
		t.Errorf("progress must report n/total on stderr, logs: %s", env.logs.String())
	}
}

func TestSync_CSV(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "")
	dataDir := t.TempDir()
	pa := writeSyncFile(t, dataDir, "a.safetensors", "registered")
	writeSyncFile(t, dataDir, "b.safetensors", "registered")
	pc := writeSyncFile(t, dataDir, "c.safetensors", "unknown")
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))
	if err := runSyncCmd(t, env, dataDir, "--format", "csv"); err != nil {
		t.Fatalf("sync csv: %v", err)
	}
	recs, err := csv.NewReader(strings.NewReader(env.buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("csv must parse: %v\n%s", err, env.buf.String())
	}
	if len(recs) != 4 { // header + 3 rows
		t.Fatalf("csv record count = %d, want 4:\n%v", len(recs), recs)
	}
	if strings.Join(recs[0], ",") != strings.Join(syncHeader, ",") {
		t.Errorf("csv header = %v, want %v", recs[0], syncHeader)
	}
	byFile := map[string][]string{}
	for _, r := range recs[1:] {
		byFile[r[0]] = r
	}
	if byFile[pa][1] != "found dupe" || byFile[pa][2] != "TestModel" || byFile[pa][5] != "2" {
		t.Errorf("csv found-dupe row mismatch: %v", byFile[pa])
	}
	if byFile[pc][1] != "not_found" {
		t.Errorf("csv not_found row mismatch: %v", byFile[pc])
	}
}

func TestSync_JSONAndGlobalJSON(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "")
	dataDir := t.TempDir()
	pa := writeSyncFile(t, dataDir, "a.safetensors", "registered")
	writeSyncFile(t, dataDir, "b.safetensors", "registered")
	pc := writeSyncFile(t, dataDir, "c.safetensors", "unknown")
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	for _, args := range [][]string{{dataDir, "--format", "json"}, {dataDir, "--json"}} {
		rows := syncRowsJSON(t, env, args...)
		if len(rows) != 3 {
			t.Fatalf("json row count = %d, want 3", len(rows))
		}
		byFile := map[string]syncRow{}
		for _, r := range rows {
			byFile[r.File] = r
		}
		ra, rc := byFile[pa], byFile[pc]
		if ra.Status != "found" || !ra.Dupe || ra.Model != "TestModel" || ra.Version != "v1" || ra.Base != "SD 1.5" || ra.Triggers != 2 || ra.Pickle != "Success" || ra.Virus != "Success" || ra.ModelURL != "https://civitai.com/models/777" || ra.VersionID != 555 {
			t.Errorf("json found-dupe row mismatch: %+v", ra)
		}
		if rc.Status != "not_found" || rc.Dupe {
			t.Errorf("json not_found row mismatch: %+v", rc)
		}
	}
}

func TestSync_FiftyFilesThreeFormats(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "1") // isolate formats from cache reuse
	dataDir := t.TempDir()
	// f00..f02 share a registered version (dupe); the rest are unknown.
	for i := 0; i < 50; i++ {
		content := fmt.Sprintf("unknown-%d", i)
		if i < 3 {
			content = "registered"
		}
		writeSyncFile(t, dataDir, fmt.Sprintf("f%02d.safetensors", i), content)
	}
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	rows := syncRowsJSON(t, env, dataDir)
	if len(rows) != 50 {
		t.Fatalf("50 files must yield 50 rows, got %d", len(rows))
	}
	found, notFound, dupes := 0, 0, 0
	for _, r := range rows {
		switch r.Status {
		case "found":
			found++
			if r.Dupe {
				dupes++
			}
		case "not_found":
			notFound++
		default:
			t.Errorf("unexpected status %q", r.Status)
		}
	}
	if found != 3 || dupes != 3 || notFound != 47 {
		t.Errorf("tally = found %d dupe %d not_found %d, want 3/3/47", found, dupes, notFound)
	}
	// CSV and table must also render all 50 rows without error.
	env.buf.Reset()
	if err := runSyncCmd(t, env, dataDir, "--format", "csv"); err != nil {
		t.Fatalf("csv: %v", err)
	}
	recs, err := csv.NewReader(strings.NewReader(env.buf.String())).ReadAll()
	if err != nil || len(recs) != 51 {
		t.Fatalf("csv must have 51 records (header+50), got %d err %v", len(recs), err)
	}
	env.buf.Reset()
	if err := runSyncCmd(t, env, dataDir, "--format", "table"); err != nil {
		t.Fatalf("table: %v", err)
	}
	if n := strings.Count(env.buf.String(), "not_found"); n != 47 {
		t.Errorf("table not_found count = %d, want 47", n)
	}
}

func TestSync_CacheHitSecondRun(t *testing.T) {
	cacheDir := t.TempDir()
	setCacheDir(t, cacheDir)
	t.Setenv("CIV_NO_CACHE", "")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "a.safetensors", "registered")
	writeSyncFile(t, dataDir, "b.safetensors", "unknown-1")
	writeSyncFile(t, dataDir, "c.safetensors", "unknown-2")
	calls := countHashCalls(t)
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := env.buf.String()
	if calls.Load() != 3 {
		t.Fatalf("first run must hash all 3 files, got %d", calls.Load())
	}
	// The cache file must be persisted after phase 1.
	cacheFile := filepath.Join(cacheDir, "civ", "cache", "hashes.json")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("cache file must exist after first run: %v", err)
	}
	// Second run: every hash is a cache hit → zero recomputation.
	env.buf.Reset()
	if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("second run must recompute nothing (cache hits), total calls = %d", calls.Load())
	}
	if env.buf.String() != first {
		t.Errorf("second run output must match the first:\nfirst=%s\nsecond=%s", first, env.buf.String())
	}
}

func TestSync_CIV_NO_CACHE_Bypass(t *testing.T) {
	cacheDir := t.TempDir()
	setCacheDir(t, cacheDir)
	t.Setenv("CIV_NO_CACHE", "1")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "a.safetensors", "registered")
	writeSyncFile(t, dataDir, "b.safetensors", "unknown")
	calls := countHashCalls(t)
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	for i := 0; i < 2; i++ {
		env.buf.Reset()
		if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if calls.Load() != 4 {
		t.Errorf("CIV_NO_CACHE must recompute every run (2+2), got %d", calls.Load())
	}
	cacheFile := filepath.Join(cacheDir, "civ", "cache", "hashes.json")
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Errorf("CIV_NO_CACHE must not write a cache file, stat err = %v", err)
	}
}

func TestSync_Rehash(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "a.safetensors", "registered")
	writeSyncFile(t, dataDir, "b.safetensors", "unknown")
	calls := countHashCalls(t)
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("first run hashes = %d, want 2", calls.Load())
	}
	env.buf.Reset()
	if err := runSyncCmd(t, env, dataDir, "--json", "--rehash"); err != nil {
		t.Fatalf("rehash run: %v", err)
	}
	if calls.Load() != 4 {
		t.Errorf("--rehash must recompute all files (2+2), got %d", calls.Load())
	}
}

func TestSync_MTimeInvalidation(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "")
	dataDir := t.TempDir()
	pa := writeSyncFile(t, dataDir, "a.safetensors", "registered")
	writeSyncFile(t, dataDir, "b.safetensors", "unknown")
	calls := countHashCalls(t)
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Touch one file's mtime; only it must be rehashed on the next run.
	fi, err := os.Stat(pa)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	newTime := fi.ModTime().Add(5 * time.Second)
	if err := os.Chtimes(pa, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	env.buf.Reset()
	if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("mtime change must rehash exactly one file (2+1), got %d", calls.Load())
	}
}

func TestSync_ExtFilter(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "1")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "a.safetensors", "x")
	writeSyncFile(t, dataDir, "b.pt", "x")
	writeSyncFile(t, dataDir, "c.txt", "x")
	writeSyncFile(t, dataDir, "d.ckpt", "x")
	writeSyncFile(t, dataDir, "sub/e.bin", "x") // recursive
	env := newTestEnv(t, syncRouteHandler(nil)) // everything not_found

	rows := syncRowsJSON(t, env, dataDir)
	if len(rows) != 4 {
		t.Fatalf("default ext must scan 4 model files (skip .txt), got %d", len(rows))
	}
	for _, r := range rows {
		if strings.HasSuffix(r.File, ".txt") {
			t.Errorf(".txt must be excluded by default: %s", r.File)
		}
	}
	// --ext override (dot optional) selects only the named extension.
	rows = syncRowsJSON(t, env, dataDir, "--ext", "txt")
	if len(rows) != 1 || !strings.HasSuffix(rows[0].File, ".txt") {
		t.Errorf("--ext txt must select only the .txt file, got %+v", rows)
	}
	rows = syncRowsJSON(t, env, dataDir, "--ext", ".safetensors,.pt")
	if len(rows) != 2 {
		t.Errorf("--ext .safetensors,.pt must select 2 files, got %d", len(rows))
	}
}

func TestSync_HashErrorRowExit0(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "1")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "good.safetensors", "registered")
	bad := writeSyncFile(t, dataDir, "bad.safetensors", "whatever")
	old := computeHash
	computeHash = func(path string) (string, error) {
		if path == bad {
			return "", errors.New("simulated hash failure")
		}
		return old(path)
	}
	t.Cleanup(func() { computeHash = old })
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	// A hash failure is an error row, not a command failure (exit 0).
	if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
		t.Fatalf("hash failure must not fail sync: %v", err)
	}
	var rows []syncRow
	if err := json.Unmarshal(env.buf.Bytes(), &rows); err != nil {
		t.Fatalf("json: %v", err)
	}
	byFile := map[string]syncRow{}
	for _, r := range rows {
		byFile[r.File] = r
	}
	if byFile[bad].Status != "error" || !strings.Contains(byFile[bad].Detail, "simulated") {
		t.Errorf("hash failure must render an error row: %+v", byFile[bad])
	}
	if byFile[filepath.Join(dataDir, "good.safetensors")].Status != "found" {
		t.Errorf("the good file must still be looked up: %+v", rows)
	}
	if !strings.Contains(env.logs.String(), "hash failed") {
		t.Errorf("hash failure must warn on stderr, logs: %s", env.logs.String())
	}
}

func TestSync_NetworkExit2(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "1")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "a.safetensors", "x")
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	newSyncClient = func(g *globals, conc int) *api.Client {
		return api.NewClient(
			api.WithBaseURL(env.srv.URL),
			api.WithToken(g.resolveToken()),
			api.WithTimeout(g.Timeout),
			api.WithConcurrency(conc),
			api.WithBackoffs(time.Millisecond, time.Millisecond),
		)
	}
	err := runSyncCmd(t, env, dataDir, "--json")
	var ne *api.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("network failure must yield NetworkError, got %v", err)
	}
	if exitCodeFor(err, false) != 2 {
		t.Errorf("network failure must map to exit 2, got %d", exitCodeFor(err, false))
	}
	// Rows still render with STATUS=error.
	var rows []syncRow
	if jerr := json.Unmarshal(env.buf.Bytes(), &rows); jerr != nil {
		t.Fatalf("json: %v", jerr)
	}
	if len(rows) != 1 || rows[0].Status != "error" {
		t.Errorf("network failure must render an error row: %+v", rows)
	}
}

func TestSync_NotFoundExit0(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "1")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "a.safetensors", "unknown-a")
	writeSyncFile(t, dataDir, "b.safetensors", "unknown-b")
	env := newTestEnv(t, syncRouteHandler(nil)) // all 404
	if err := runSyncCmd(t, env, dataDir, "--json"); err != nil {
		t.Fatalf("not_found rows must not fail sync (exit 0), got %v", err)
	}
	var rows []syncRow
	json.Unmarshal(env.buf.Bytes(), &rows)
	for _, r := range rows {
		if r.Status != "not_found" {
			t.Errorf("unregistered hash must be not_found, got %+v", r)
		}
	}
}

func TestSync_UsageErrors(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("CIV_NO_CACHE", "1")
	env := newTestEnv(t, syncRouteHandler(nil))
	dataDir := t.TempDir()
	aFile := writeSyncFile(t, dataDir, "a.safetensors", "x")
	cases := []struct {
		name string
		args []string
	}{
		{"no argument", nil},
		{"two arguments", []string{dataDir, dataDir}},
		{"missing directory", []string{filepath.Join(dataDir, "nope")}},
		{"file not directory", []string{aFile}},
		{"bad format", []string{dataDir, "--format", "xml"}},
		{"concurrency zero", []string{dataDir, "--concurrency", "0"}},
		{"concurrency nine", []string{dataDir, "--concurrency", "9"}},
		{"empty ext", []string{dataDir, "--ext", ","}},
	}
	for _, tc := range cases {
		err := runSyncCmd(t, env, tc.args...)
		if err == nil {
			t.Errorf("%s: must be rejected", tc.name)
			continue
		}
		if exitCodeFor(err, false) != 1 {
			t.Errorf("%s: usage error must map to exit 1, got %d", tc.name, exitCodeFor(err, false))
		}
	}
}

func TestSync_CancelSavesHashes(t *testing.T) {
	cacheDir := t.TempDir()
	setCacheDir(t, cacheDir)
	t.Setenv("CIV_NO_CACHE", "")
	dataDir := t.TempDir()
	writeSyncFile(t, dataDir, "a.safetensors", "registered")
	writeSyncFile(t, dataDir, "b.safetensors", "unknown")
	env := newTestEnv(t, syncRouteHandler(map[string]string{hashOf("registered"): syncVersionBody}))

	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	old := computeHash
	computeHash = func(path string) (string, error) {
		h, err := old(path)
		once.Do(cancel) // cancel after the first hash is computed
		return h, err
	}
	t.Cleanup(func() { computeHash = old; cancel() })

	var o syncOptions
	pos := parseForTest(t, &env.g, o.register, []string{dataDir})
	err := runSync(ctx, &env.g, pos, &o)
	var ne *api.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("cancellation must yield NetworkError (exit 2), got %v", err)
	}
	// Already-computed hashes must still be persisted for the rerun.
	cacheFile := filepath.Join(cacheDir, "civ", "cache", "hashes.json")
	b, readErr := os.ReadFile(cacheFile)
	if readErr != nil {
		t.Fatalf("cache must be saved even on cancel: %v", readErr)
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatalf("cache json: %v", err)
	}
	if len(entries) < 1 {
		t.Errorf("at least one computed hash must be saved on cancel, got %d", len(entries))
	}
}

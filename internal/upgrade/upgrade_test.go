package upgrade

import (
	"archive/zip"
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"civitai-cli/internal/config"
)

func TestParseChecksum(t *testing.T) {
	const asset = "civitai-cli-windows-amd64.zip"
	tests := []struct {
		name string
		text string
		file string
		want string
		ok   bool
	}{
		{"text mode (two spaces)", "abc123  " + asset + "\n", asset, "abc123", true},
		{"binary mode (*prefix)", "abc123 *" + asset + "\n", asset, "abc123", true},
		{"CRLF line ending", "abc123  " + asset + "\r\n", asset, "abc123", true},
		{"multi-line hit", "dead00  other.zip\nabc123  " + asset + "\n", asset, "abc123", true},
		{"filename mismatch", "abc123  other.zip\n", asset, "", false},
		{"wrong field count", "abc123\n", asset, "", false},
		{"empty text", "", asset, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseChecksum(tt.text, tt.file)
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseChecksum = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.2.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"0.3.0", "0.2.9", 1},
		{"0.2", "0.2.0", 0}, // missing segments count as 0
		{"0.2.0", "0.2", 0},
		{"0.10.0", "0.9.0", 1}, // numeric segment compare, not string compare
		{"0.1.0-dev", "0.2.0", -1},
		{"0.2.0", "0.1.0-dev", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			if got := compareVersions(tt.a, tt.b); got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestExpectedAssetName(t *testing.T) {
	want := "civitai-cli-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	if got := expectedAssetName(); got != want {
		t.Errorf("expectedAssetName() = %q, want %q", got, want)
	}
}

func TestCleanupBakAt(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "civ.exe")

	if cleanupBakAt(exe) {
		t.Fatal("no .bak exists, should report false")
	}

	if err := os.WriteFile(exe+".bak", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !cleanupBakAt(exe) {
		t.Fatal("expected .bak to be removed")
	}
	if _, err := os.Stat(exe + ".bak"); !os.IsNotExist(err) {
		t.Fatal(".bak still exists after cleanup")
	}
}

func TestReplaceExeAt(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "civ.exe")
	newExe := filepath.Join(dir, "new.exe")
	if err := os.WriteFile(cur, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newExe, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExeAt(cur, newExe); err != nil {
		t.Fatalf("replaceExeAt failed: %v", err)
	}

	got, err := os.ReadFile(cur)
	if err != nil || string(got) != "new" {
		t.Errorf("current exe content = %q (err: %v), want \"new\"", got, err)
	}
	if _, err := os.Stat(newExe); !os.IsNotExist(err) {
		t.Error("new exe should have been moved away")
	}
	if _, err := os.Stat(cur + ".bak"); !os.IsNotExist(err) {
		t.Error(".bak should be cleaned up after successful replace")
	}
}

func TestReplaceExeAt_CurrentMissing(t *testing.T) {
	dir := t.TempDir()
	newExe := filepath.Join(dir, "new.exe")
	if err := os.WriteFile(newExe, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := replaceExeAt(filepath.Join(dir, "missing.exe"), newExe)
	if err == nil {
		t.Fatal("expected error when current exe does not exist")
	}
	// on failure the new exe must be untouched (rollback semantics)
	if _, statErr := os.Stat(newExe); statErr != nil {
		t.Error("new exe should remain untouched on failure")
	}
}

// TestReplaceExeAt_RollbackOnMoveFailure pins the rollback path: when the new
// exe cannot be moved into place (here the source is missing), the old exe must
// be restored from .bak so the install is never left without a binary.
func TestReplaceExeAt_RollbackOnMoveFailure(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "civ.exe")
	if err := os.WriteFile(cur, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	// newExe does not exist: step 1 (old -> .bak) succeeds, step 2 (new -> cur)
	// fails, so the rollback must restore cur from .bak.
	err := replaceExeAt(cur, filepath.Join(dir, "missing-new.exe"))
	if err == nil {
		t.Fatal("expected error when the new exe cannot be moved into place")
	}
	got, readErr := os.ReadFile(cur)
	if readErr != nil || string(got) != "old" {
		t.Errorf("old exe must be restored on rollback, got %q (err: %v)", got, readErr)
	}
	if _, statErr := os.Stat(cur + ".bak"); !os.IsNotExist(statErr) {
		t.Error(".bak should be gone after rollback restores the old exe")
	}
}

// writeTestZip builds a zip with the given entries in dir, returns the zip path.
func writeTestZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(dir, "test.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return zipPath
}

func TestExtractSingleExe(t *testing.T) {
	dir := t.TempDir()
	zipPath := writeTestZip(t, dir, map[string]string{
		"readme.txt":      "hi",
		"civitai-cli/bin": "skip", // directory prefix should still match by base name
		"civ.exe":         "PAYLOAD",
	})

	out, err := extractSingleExe(zipPath, dir)
	if err != nil {
		t.Fatalf("extractSingleExe failed: %v", err)
	}
	defer os.Remove(out)

	got, err := os.ReadFile(out)
	if err != nil || string(got) != "PAYLOAD" {
		t.Errorf("extracted content = %q (err: %v), want \"PAYLOAD\"", got, err)
	}
}

func TestExtractSingleExe_MissingExe(t *testing.T) {
	dir := t.TempDir()
	zipPath := writeTestZip(t, dir, map[string]string{
		"readme.txt": "hi",
	})

	_, err := extractSingleExe(zipPath, dir)
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("expected 'does not contain' error, got: %v", err)
	}
}

func TestDownloadOnce(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "asset.zip")
	if err := downloadOnce(srv.URL, dest); err != nil {
		t.Fatalf("downloadOnce failed: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "hello" {
		t.Errorf("downloaded content = %q, want \"hello\"", got)
	}
	if !strings.HasPrefix(gotUA, "civ/") {
		t.Errorf("User-Agent = %q, want civ/* prefix", gotUA)
	}
}

func TestDownloadOnce_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "asset.zip")
	err := downloadOnce(srv.URL, dest)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got: %v", err)
	}
}

// setConfigDir points os.UserConfigDir at dir for the duration of the test so
// applyPersistedProxy reads a config file the test controls, never a real
// user profile. Windows resolves the config dir from %AppData%; Unix from
// $XDG_CONFIG_HOME (mirrors internal/config's isolation helper).
func setConfigDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}

// isolateDownloadClient clears downloadClient's Transport before the test and
// restores the cleared state afterwards, so tests mutating the shared client
// never leak transport state into each other.
func isolateDownloadClient(t *testing.T) {
	t.Helper()
	downloadClient.Transport = nil
	t.Cleanup(func() { downloadClient.Transport = nil })
}

// proxyURLOf returns the URL tr's Proxy function resolves to ("" when nil).
func proxyURLOf(t *testing.T, tr *http.Transport) string {
	t.Helper()
	u, err := tr.Proxy(&http.Request{})
	if err != nil {
		t.Fatalf("Transport.Proxy() error: %v", err)
	}
	if u == nil {
		return ""
	}
	return u.String()
}

func TestApplyPersistedProxy_SetsTransport(t *testing.T) {
	setConfigDir(t, t.TempDir())
	isolateDownloadClient(t)

	const proxyURL = "http://127.0.0.1:7890"
	if err := config.Save(config.Config{Proxy: proxyURL}); err != nil {
		t.Fatal(err)
	}

	applyPersistedProxy()

	tr, ok := downloadClient.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("downloadClient.Transport = %T, want *http.Transport", downloadClient.Transport)
	}
	if got := proxyURLOf(t, tr); got != proxyURL {
		t.Errorf("Transport.Proxy = %q, want %q", got, proxyURL)
	}
}

func TestApplyPersistedProxy_NoProxyKeepsNilTransport(t *testing.T) {
	setConfigDir(t, t.TempDir()) // empty dir: no config file
	isolateDownloadClient(t)

	applyPersistedProxy()

	if downloadClient.Transport != nil {
		t.Errorf("downloadClient.Transport = %T, want nil (env proxies via DefaultTransport)", downloadClient.Transport)
	}
}

// TestApplyPersistedProxy_InvalidSavedValueDegrades pins the degrade path: a
// hand-edited config can hold an invalid proxy (config.Save does not
// validate; validation happens at `civ proxy set`), and the apply step must
// warn and keep the nil Transport instead of failing the upgrade.
func TestApplyPersistedProxy_InvalidSavedValueDegrades(t *testing.T) {
	setConfigDir(t, t.TempDir())
	isolateDownloadClient(t)

	if err := config.Save(config.Config{Proxy: "ftp://127.0.0.1:21"}); err != nil {
		t.Fatal(err)
	}

	// captureLog redirects the standard logger (app.Warnf writes to it).
	var buf bytes.Buffer
	old := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(old); log.SetFlags(oldFlags) })

	applyPersistedProxy() // must not panic

	if downloadClient.Transport != nil {
		t.Errorf("downloadClient.Transport = %T, want nil after invalid saved proxy", downloadClient.Transport)
	}
	if !strings.Contains(buf.String(), "[WARN]") {
		t.Errorf("expected a [WARN] degrade line on stderr, got log %q", buf.String())
	}
}

// TestHelp pins the curated `civ upgrade help` page contract: main prints
// the string verbatim (so it must carry its own "civ upgrade -" title,
// unlike the runner-routed command pages), the Usage and Examples sections
// of the shared layout, and the upgrade-specific notes (exit codes, the
// .bak swap, and that the --proxy flag does not apply here).
func TestHelp(t *testing.T) {
	page := Help()
	for _, want := range []string{
		"civ upgrade - ",
		"Usage:\n  civ upgrade",
		"What it does:",
		"Examples:\n  civ upgrade\n",
		".bak",
		"0 success",
		"the --proxy flag does not apply",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("upgrade help page missing %q", want)
		}
	}
	if !strings.HasSuffix(page, "\n") {
		t.Errorf("upgrade help page must end with a newline")
	}
}

// TestHelp_SectionOrder applies the cross-all-pages layout audit (task
// 11-22 prd AC8) to the tenth page: Help is printed verbatim by main — no
// runner, so no appended global block — and must keep the shared order on
// the sections it does have (Usage before Examples; no Options, upgrade is
// flagless), document json (un)availability itself (the Output section
// states --json does not apply), and embed no copy of any global flag row
// (the page explains --proxy does not apply instead).
func TestHelp_SectionOrder(t *testing.T) {
	page := Help()
	u := strings.Index(page, "Usage:")
	e := strings.Index(page, "Examples:")
	if u < 0 || e < 0 || u >= e {
		t.Errorf("upgrade help: Usage(%d) must precede Examples(%d)", u, e)
	}
	if strings.Index(page, "Usage:") > strings.Index(page, "Exit codes:") {
		t.Errorf("upgrade help: Usage must open the page before Exit codes")
	}
	if !strings.Contains(page, "--json") {
		t.Errorf("upgrade help: must document json (un)availability — the global --json row never reaches this page")
	}
	for _, row := range []string{"Global flags", "--token <key>", "--proxy <url>", "--timeout <dur>"} {
		if strings.Contains(page, row) {
			t.Errorf("upgrade help: must not embed the global flag row %q (main prints the page without the shared block)", row)
		}
	}
}

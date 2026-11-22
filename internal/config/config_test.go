package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setConfigDir points os.UserConfigDir at dir for the duration of the test
// so config reads/writes never touch a real user profile. Windows resolves
// the config dir from %AppData%; Unix from $XDG_CONFIG_HOME.
func setConfigDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}

// captureLog redirects the standard logger to a buffer for fn and returns
// what was written (app.Warnf logs through it).
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

func TestPath(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)
	want := filepath.Join(dir, "civ", "config.json")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoad_MissingIsZeroValue(t *testing.T) {
	setConfigDir(t, t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("missing config must not error: %v", err)
	}
	if c.Token != "" {
		t.Errorf("missing config must yield zero token, got %q", c.Token)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)
	if err := Save(Config{Token: "secret-key"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The file must exist at the documented path with the JSON shape.
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("config file must exist after Save: %v", err)
	}
	if !strings.Contains(string(b), `"token"`) || !strings.Contains(string(b), "secret-key") {
		t.Errorf("config file shape unexpected: %s", b)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Token != "secret-key" {
		t.Errorf("round-trip token = %q, want secret-key", c.Token)
	}
	// 0600 semantics are best-effort on Windows; only assert on Unix where
	// the mode bits are meaningful (parent design D8).
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(Path())
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("config perms = %o, want 600", fi.Mode().Perm())
		}
	}
}

func TestSave_OverwritesExisting(t *testing.T) {
	setConfigDir(t, t.TempDir())
	if err := Save(Config{Token: "first"}); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := Save(Config{Token: "second"}); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	c, _ := Load()
	if c.Token != "second" {
		t.Errorf("overwrite token = %q, want second", c.Token)
	}
	// No stray temp files left behind by the atomic write.
	entries, err := os.ReadDir(filepath.Dir(Path()))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("atomic write left a temp file: %s", e.Name())
		}
	}
}

func TestDelete_Idempotent(t *testing.T) {
	setConfigDir(t, t.TempDir())
	// Deleting a missing config is a no-op (logout friendliness).
	if err := Delete(); err != nil {
		t.Fatalf("Delete on missing config must be nil: %v", err)
	}
	if err := Save(Config{Token: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(Path()); !os.IsNotExist(err) {
		t.Errorf("config file must be gone after Delete, stat err = %v", err)
	}
	// Second delete is still nil.
	if err := Delete(); err != nil {
		t.Fatalf("second Delete must be nil: %v", err)
	}
	c, _ := Load()
	if c.Token != "" {
		t.Errorf("Load after Delete must be zero, got %q", c.Token)
	}
}

func TestLoad_CorruptDegrades(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	var c Config
	var err error
	logs := captureLog(t, func() { c, err = Load() })
	if err != nil {
		t.Fatalf("corrupt config must degrade, not error: %v", err)
	}
	if c.Token != "" {
		t.Errorf("corrupt config must yield zero token, got %q", c.Token)
	}
	if !strings.Contains(logs, "corrupt") {
		t.Errorf("corrupt config must warn on stderr, logs: %s", logs)
	}
}

func TestResolveToken_Priority(t *testing.T) {
	cases := []struct {
		name      string
		flag      string
		env       string
		configTok string
		want      string
	}{
		{"flag wins over env and config", "flag-key", "env-key", "config-key", "flag-key"},
		{"env wins over config", "", "env-key", "config-key", "env-key"},
		{"config when flag and env empty", "", "", "config-key", "config-key"},
		{"anonymous when all empty", "", "", "", ""},
		{"flag only", "flag-key", "", "", "flag-key"},
		{"env only", "", "env-key", "", "env-key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			setConfigDir(t, dir)
			t.Setenv("CIVITAI_API_KEY", tc.env)
			if tc.configTok != "" {
				if err := Save(Config{Token: tc.configTok}); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}
			if got := ResolveToken(tc.flag); got != tc.want {
				t.Errorf("ResolveToken(%q) = %q, want %q", tc.flag, got, tc.want)
			}
		})
	}
}

func TestResolveProxy_Priority(t *testing.T) {
	cases := []struct {
		name        string
		flag        string
		configProxy string
		corrupt     bool
		want        string
	}{
		{"flag wins over persisted", "http://127.0.0.1:7890", "http://10.0.0.1:8080", false, "http://127.0.0.1:7890"},
		{"empty flag falls back to persisted", "", "http://10.0.0.1:8080", false, "http://10.0.0.1:8080"},
		{"missing config file yields empty (env/direct)", "", "", false, ""},
		{"both empty yields empty", "", "", false, ""},
		{"whitespace flag falls through to persisted", "   ", "socks5://127.0.0.1:1080", false, "socks5://127.0.0.1:1080"},
		{"whitespace flag with no config yields empty", " \t ", "", false, ""},
		{"corrupt config file degrades to empty", "", "http://10.0.0.1:8080", true, ""},
		{"flag still wins over corrupt config", "http://127.0.0.1:7890", "http://10.0.0.1:8080", true, "http://127.0.0.1:7890"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setConfigDir(t, t.TempDir())
			if tc.corrupt {
				if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(Path(), []byte("{not json"), 0o600); err != nil {
					t.Fatalf("write corrupt: %v", err)
				}
			} else if tc.configProxy != "" {
				if err := Save(Config{Token: "tok", Proxy: tc.configProxy}); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}
			// captureLog keeps the corrupt-case app.Warnf out of test output
			// (ResolveProxy swallows Load errors by contract).
			var got string
			logs := captureLog(t, func() { got = ResolveProxy(tc.flag) })
			if got != tc.want {
				t.Errorf("ResolveProxy(%q) = %q, want %q", tc.flag, got, tc.want)
			}
			if tc.corrupt && !strings.Contains(logs, "corrupt") {
				t.Errorf("corrupt config must warn on stderr, logs: %s", logs)
			}
		})
	}
}

// TestLoad_TokenOnlyFileDecodesEmptyProxy pins the additive-schema contract:
// a config written before proxy support (token only) decodes with an empty
// Proxy — no migration needed (task 11-22 design §2).
func TestLoad_TokenOnlyFileDecodesEmptyProxy(t *testing.T) {
	setConfigDir(t, t.TempDir())
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(Path(), []byte(`{"token":"secret-key"}`), 0o600); err != nil {
		t.Fatalf("write token-only config: %v", err)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Token != "secret-key" {
		t.Errorf("token = %q, want secret-key", c.Token)
	}
	if c.Proxy != "" {
		t.Errorf("token-only file must decode Proxy as empty, got %q", c.Proxy)
	}
}

// TestSaveLoadRoundTripProxy asserts the proxy key round-trips alongside
// the token with the documented JSON shape.
func TestSaveLoadRoundTripProxy(t *testing.T) {
	setConfigDir(t, t.TempDir())
	if err := Save(Config{Token: "secret-key", Proxy: "http://127.0.0.1:7890"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("config file must exist after Save: %v", err)
	}
	if !strings.Contains(string(b), `"proxy"`) || !strings.Contains(string(b), "http://127.0.0.1:7890") {
		t.Errorf("config file shape unexpected: %s", b)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Token != "secret-key" || c.Proxy != "http://127.0.0.1:7890" {
		t.Errorf("round-trip = {Token: %q, Proxy: %q}, want both preserved", c.Token, c.Proxy)
	}
}

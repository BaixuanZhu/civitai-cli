package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"civitai-cli/internal/api"
	"civitai-cli/internal/config"
)

// runLoginCmd mirrors the Login command wiring — login flag registration
// plus the shared arg permutation — without os.Exit, so precedence tests
// exercise the same parse path as the real CLI.
func runLoginCmd(t *testing.T, env *testEnv, stdin io.Reader, args ...string) error {
	t.Helper()
	var o loginOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runLogin(context.Background(), &env.g, pos, &o, stdin)
}

// TestLogin_CandidatePrecedence drives runLogin through the real flag
// parse path for every candidate source (flag, positional, global --token,
// stdin) and every rejection shape (empty/whitespace, multi-positional),
// asserting the saved token, the probe's Authorization header, and the
// exit mapping.
func TestLogin_CandidatePrecedence(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		stdin   string
		wantKey string // saved token and probe auth ("Bearer <wantKey>")
		wantErr string // non-empty: expect exit 1 and this message fragment
	}{
		{name: "flag key", args: []string{"--key", "flag-key"}, wantKey: "flag-key"},
		{name: "flag key shorthand t", args: []string{"-t", "short-key"}, wantKey: "short-key"},
		{name: "flag key equals form", args: []string{"--key=eq-key"}, wantKey: "eq-key"},
		{name: "flag beats positional and global token", args: []string{"--key", "flag-key", "pos-key", "--token", "global-key"}, wantKey: "flag-key"},
		{name: "positional key", args: []string{"pos-key"}, wantKey: "pos-key"},
		{name: "positional beats global token", args: []string{"pos-key", "--token", "global-key"}, wantKey: "pos-key"},
		{name: "global token", args: []string{"--token", "global-key"}, wantKey: "global-key"},
		// An explicitly empty --token is indistinguishable from an absent one
		// (StringVar zero value), so it falls back to the stdin prompt.
		{name: "empty global token falls back to stdin", args: []string{"--token", ""}, stdin: "stdin-key\n", wantKey: "stdin-key"},
		{name: "stdin prompt fallback", stdin: "stdin-key\n", wantKey: "stdin-key"},
		{name: "stdin pipe trims whitespace", stdin: "  piped-key  \n", wantKey: "piped-key"},
		{name: "key dash reads stdin", args: []string{"--key", "-"}, stdin: "piped-key\n", wantKey: "piped-key"},
		{name: "t dash reads stdin", args: []string{"-t", "-"}, stdin: "piped-key\n", wantKey: "piped-key"},
		{name: "flag key trims whitespace", args: []string{"--key", "  spaced  "}, wantKey: "spaced"},
		{name: "positional trims whitespace", args: []string{"  spaced  "}, wantKey: "spaced"},
		{name: "empty flag key", args: []string{"--key", ""}, wantErr: "no API key"},
		// Piped stdin must NOT rescue an explicit empty flag: an empty
		// --key/-t is an empty candidate (exit 1), never a prompt fallthrough.
		{name: "empty flag key ignores piped stdin", args: []string{"--key", ""}, stdin: "stdin-key\n", wantErr: "no API key"},
		{name: "empty t shorthand ignores piped stdin", args: []string{"-t", ""}, stdin: "stdin-key\n", wantErr: "no API key"},
		{name: "whitespace flag key", args: []string{"--key", "   "}, wantErr: "no API key"},
		{name: "empty positional", args: []string{""}, wantErr: "no API key"},
		{name: "multiple positionals", args: []string{"a", "b"}, wantErr: "at most one"},
		{name: "empty stdin", stdin: "", wantErr: "no API key"},
		{name: "whitespace stdin", stdin: "   \n", wantErr: "no API key"},
		{name: "empty stdin with key dash", args: []string{"--key", "-"}, stdin: "\n", wantErr: "no API key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth string
			env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				w.Write([]byte(`{"items":[],"metadata":{}}`))
			})
			err := runLoginCmd(t, env, strings.NewReader(tc.stdin), tc.args...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				if got := loginExitCode(err, false); got != 1 {
					t.Errorf("empty/usage input must map to exit 1, got %d", got)
				}
				if env.lastPath != "" {
					t.Errorf("rejected input must not probe the API, got request %q", env.lastPath)
				}
				c, _ := config.Load()
				if c.Token != "" {
					t.Errorf("rejected input must not save a token, got %q", c.Token)
				}
				return
			}
			if err != nil {
				t.Fatalf("runLogin: %v", err)
			}
			if gotAuth != "Bearer "+tc.wantKey {
				t.Errorf("probe must carry the candidate key, Authorization = %q", gotAuth)
			}
			c, err := config.Load()
			if err != nil {
				t.Fatalf("config.Load: %v", err)
			}
			if c.Token != tc.wantKey {
				t.Errorf("saved token = %q, want %q", c.Token, tc.wantKey)
			}
			if !strings.Contains(env.buf.String(), "Logged in") {
				t.Errorf("success must confirm on stdout, got %q", env.buf.String())
			}
		})
	}
}

func TestLogin_SuccessSavesConfig(t *testing.T) {
	var gotAuth string
	var gotQuery url.Values
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	// Surrounding whitespace must be trimmed before probing/saving.
	if err := runLogin(context.Background(), &env.g, nil, &loginOptions{}, strings.NewReader("  candidate-key  \n")); err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	c, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if c.Token != "candidate-key" {
		t.Errorf("saved token = %q, want trimmed candidate-key", c.Token)
	}
	if gotAuth != "Bearer candidate-key" {
		t.Errorf("probe must carry the candidate key, Authorization = %q", gotAuth)
	}
	if gotQuery.Get("favorites") != "true" || gotQuery.Get("limit") != "1" {
		t.Errorf("probe must be GET /models?favorites=true&limit=1, got %v", gotQuery)
	}
	if !strings.Contains(env.buf.String(), "Logged in") {
		t.Errorf("success must confirm on stdout, got %q", env.buf.String())
	}
	// Acceptance: after login, subsequent commands auto-resolve the token.
	if got := env.g.resolveToken(); got != "candidate-key" {
		t.Errorf("resolveToken after login = %q, want candidate-key", got)
	}
}

// TestLogin_LoadModifySavePreservesConfig pins the load-modify-save flow:
// login rewrites only the token of the loaded config instead of writing a
// fresh Config. Config currently has just the token field, so the sibling
// preservation guarantee (e.g. the planned proxy setting surviving a
// login) becomes observable once config gains a second field; today the
// test asserts the modify-over-existing-file path.
func TestLogin_LoadModifySavePreservesConfig(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	if err := config.Save(config.Config{Token: "old-key"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := runLoginCmd(t, env, strings.NewReader(""), "--key", "new-key"); err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	c, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if c.Token != "new-key" {
		t.Errorf("login must replace the saved token via load-modify-save, got %q", c.Token)
	}
}

func TestLogin_Rejected401Exit4NoSave(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Unauthorized"}`))
	})
	err := runLogin(context.Background(), &env.g, nil, &loginOptions{}, strings.NewReader("bad-key\n"))
	var ue *api.UnauthorizedError
	if !errors.As(err, &ue) {
		t.Fatalf("401 must yield UnauthorizedError, got %v", err)
	}
	if got := loginExitCode(err, false); got != 4 {
		t.Errorf("rejected key must map to exit 4, got %d", got)
	}
	if _, statErr := os.Stat(config.Path()); !os.IsNotExist(statErr) {
		t.Errorf("401 must not write a config file, stat err = %v", statErr)
	}
	c, _ := config.Load()
	if c.Token != "" {
		t.Errorf("401 must not save a token, got %q", c.Token)
	}
}

func TestLogin_NetworkExit2NoSave(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	// Shorten retries so the exhausted-network path is fast and offline.
	newAPIClient = func(g *globals) *api.Client {
		return api.NewClient(
			api.WithBaseURL(env.srv.URL),
			api.WithToken(g.resolveToken()),
			api.WithTimeout(g.Timeout),
			api.WithBackoffs(time.Millisecond, time.Millisecond),
		)
	}
	err := runLogin(context.Background(), &env.g, nil, &loginOptions{}, strings.NewReader("key\n"))
	var ne *api.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("500 retries exhausted must yield NetworkError, got %v", err)
	}
	if got := loginExitCode(err, false); got != 2 {
		t.Errorf("network failure must map to exit 2, got %d", got)
	}
	c, _ := config.Load()
	if c.Token != "" {
		t.Errorf("network failure must not save a token, got %q", c.Token)
	}
}

func TestLogin_PromptOnStderrKeyNotEchoed(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[]}`))
	})
	const key = "supersecretkey"
	stderr := captureStderr(t, func() {
		if err := runLogin(context.Background(), &env.g, nil, &loginOptions{}, strings.NewReader(key+"\n")); err != nil {
			t.Errorf("runLogin: %v", err)
		}
	})
	if !strings.Contains(stderr, "Paste API key:") {
		t.Errorf("login must prompt on stderr, got %q", stderr)
	}
	if strings.Contains(stderr, key) || strings.Contains(env.buf.String(), key) {
		t.Errorf("the API key must never be echoed to stdout/stderr")
	}
}

func TestLogout_RemovesConfig(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	if err := config.Save(config.Config{Token: "k"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := runLogout(context.Background(), &env.g, nil); err != nil {
		t.Fatalf("runLogout: %v", err)
	}
	if !strings.Contains(env.buf.String(), "Logged out") {
		t.Errorf("logout must confirm removal, got %q", env.buf.String())
	}
	if _, err := os.Stat(config.Path()); !os.IsNotExist(err) {
		t.Errorf("config file must be gone after logout, stat err = %v", err)
	}
	if env.g.resolveToken() != "" {
		t.Errorf("resolveToken after logout must be anonymous")
	}
}

func TestLogout_IdempotentFriendly(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	// No config saved: logout is a friendly no-op, exit 0.
	if err := runLogout(context.Background(), &env.g, nil); err != nil {
		t.Fatalf("runLogout without a saved key must succeed: %v", err)
	}
	if !strings.Contains(env.buf.String(), "No saved API key") {
		t.Errorf("logout without a key must say so, got %q", env.buf.String())
	}
	// A second logout is still clean.
	if err := runLogout(context.Background(), &env.g, nil); err != nil {
		t.Fatalf("second logout must succeed: %v", err)
	}
}

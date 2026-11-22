package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"civitai-cli/internal/api"
	"civitai-cli/internal/config"
)

func TestLogin_SuccessSavesConfig(t *testing.T) {
	var gotAuth string
	var gotQuery url.Values
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	// Surrounding whitespace must be trimmed before probing/saving.
	if err := runLogin(context.Background(), &env.g, nil, strings.NewReader("  candidate-key  \n")); err != nil {
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

func TestLogin_Rejected401Exit4NoSave(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Unauthorized"}`))
	})
	err := runLogin(context.Background(), &env.g, nil, strings.NewReader("bad-key\n"))
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
	err := runLogin(context.Background(), &env.g, nil, strings.NewReader("key\n"))
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

func TestLogin_EmptyInputExit1(t *testing.T) {
	for _, input := range []string{"", "   ", "\n", "  \t  \n"} {
		env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("empty input must not probe the API")
			w.Write([]byte(`{"items":[]}`))
		})
		err := runLogin(context.Background(), &env.g, nil, strings.NewReader(input))
		if err == nil {
			t.Errorf("input %q must be rejected", input)
			continue
		}
		if got := loginExitCode(err, false); got != 1 {
			t.Errorf("input %q: empty input must map to exit 1, got %d", input, got)
		}
		if env.lastPath != "" {
			t.Errorf("input %q: no API request expected, got %q", input, env.lastPath)
		}
		c, _ := config.Load()
		if c.Token != "" {
			t.Errorf("input %q: must not save a token", input)
		}
	}
}

func TestLogin_UsageErrorWithArgs(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runLogin(context.Background(), &env.g, []string{"extra"}, strings.NewReader("k\n"))
	if err == nil || !strings.Contains(err.Error(), "no arguments") {
		t.Fatalf("login with positional args must be a usage error, got %v", err)
	}
	if loginExitCode(err, false) != 1 {
		t.Errorf("usage error must map to exit 1")
	}
}

func TestLogin_PromptOnStderrKeyNotEchoed(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[]}`))
	})
	const key = "supersecretkey"
	stderr := captureStderr(t, func() {
		if err := runLogin(context.Background(), &env.g, nil, strings.NewReader(key+"\n")); err != nil {
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
		t.Fatalf("logout without a saved key must succeed: %v", err)
	}
	if !strings.Contains(env.buf.String(), "No saved API key") {
		t.Errorf("logout without a key must say so, got %q", env.buf.String())
	}
	// A second logout is still clean.
	if err := runLogout(context.Background(), &env.g, nil); err != nil {
		t.Fatalf("second logout must succeed: %v", err)
	}
}

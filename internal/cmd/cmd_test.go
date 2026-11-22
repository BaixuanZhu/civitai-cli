package cmd

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/testutil"
)

// testEnv wires command runs to an httptest server (zero network) and
// captures stdout plus log warnings. It swaps the newAPIClient factory
// and restores everything on cleanup.
type testEnv struct {
	g    globals
	buf  bytes.Buffer
	logs bytes.Buffer
	srv  *httptest.Server

	mu        sync.Mutex
	lastPath  string
	lastQuery url.Values
}

func newTestEnv(t *testing.T, handler http.HandlerFunc) *testEnv {
	t.Helper()
	t.Setenv("CIVITAI_API_KEY", "") // deterministic anonymous default
	isolateConfigDir(t)             // never read a real user config token
	env := &testEnv{}
	env.g.out = &env.buf
	env.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env.mu.Lock()
		env.lastPath = r.URL.Path
		env.lastQuery = r.URL.Query()
		env.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(env.srv.Close)

	oldFactory := newAPIClient
	newAPIClient = func(g *globals) *api.Client {
		return api.NewClient(
			api.WithBaseURL(env.srv.URL),
			api.WithToken(g.resolveToken()),
			api.WithTimeout(g.Timeout),
		)
	}
	t.Cleanup(func() { newAPIClient = oldFactory })

	oldSyncFactory := newSyncClient
	newSyncClient = func(g *globals, concurrency int) *api.Client {
		return api.NewClient(
			api.WithBaseURL(env.srv.URL),
			api.WithToken(g.resolveToken()),
			api.WithTimeout(g.Timeout),
			api.WithConcurrency(concurrency),
		)
	}
	t.Cleanup(func() { newSyncClient = oldSyncFactory })

	oldLog := log.Writer()
	log.SetOutput(&env.logs)
	t.Cleanup(func() { log.SetOutput(oldLog) })
	return env
}

// isolateConfigDir points os.UserConfigDir at a fresh empty temp dir so
// token resolution and login/logout never touch a real user profile.
// Windows resolves the config dir from %AppData%; Unix from
// $XDG_CONFIG_HOME. It returns the dir for tests that inspect the file.
func isolateConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	return dir
}

func (e *testEnv) query() url.Values {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastQuery
}

// fixtureHandler serves pinned testdata fixtures by request path; any
// unmapped path gets the API's 404 shape.
func fixtureHandler(t *testing.T, routes map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Model not found"}`))
			return
		}
		w.Write(testutil.LoadFixture(t, name))
	}
}

// parseForTest mirrors execute's flag handling (global + command flags,
// permutation for flags-after-positional UX) without os.Exit.
func parseForTest(t *testing.T, g *globals, register func(*flag.FlagSet), args []string) []string {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerGlobalFlags(fs, g)
	if register != nil {
		register(fs)
	}
	if err := fs.Parse(permuteArgs(fs, args)); err != nil {
		t.Fatalf("flag parse: %v", err)
	}
	return fs.Args()
}

func runSearchCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	var o searchOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runSearch(context.Background(), &env.g, pos, &o)
}

func runShowCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	var o showOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runShow(context.Background(), &env.g, pos, &o)
}

func runTagsCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	var o tagsOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runTags(context.Background(), &env.g, pos, &o)
}

package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"civitai-cli/internal/api"
	"civitai-cli/internal/app"
	"civitai-cli/internal/config"
)

// newProxyTestGlobals returns globals with an isolated config dir, a
// captured stdout, and deterministic token/proxy env (no ambient
// CIVITAI_API_KEY, HTTPS_PROXY, or HTTP_PROXY leaks into resolution).
func newProxyTestGlobals(t *testing.T) (*globals, *strings.Builder) {
	t.Helper()
	isolateConfigDir(t)
	t.Setenv("CIVITAI_API_KEY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "")
	buf := &strings.Builder{}
	return &globals{out: buf}, buf
}

// runProxyCmd mirrors the Proxy command wiring — global flag registration
// plus the shared arg permutation — without os.Exit, so proxy tests
// exercise the same parse path as the real CLI (proxy has no
// command-specific flags, hence the nil register).
func runProxyCmd(t *testing.T, g *globals, args ...string) error {
	t.Helper()
	pos := parseForTest(t, g, nil, args)
	return runProxy(context.Background(), g, pos)
}

// captureLogs runs fn with the standard logger redirected to a buffer and
// returns what was written; the app leveled helpers (Debugf/Infof/Warnf)
// all log through log.Printf (mirrors captureStderr for the log path).
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf strings.Builder
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	fn()
	return buf.String()
}

// TestProxy_SetGetUnset walks the happy path end to end against a temp
// config: get with nothing set reports (none)/direct, set persists the URL
// and keeps the token, get reports it back with source config, unset
// clears it (token preserved, file rewritten not deleted) and is
// idempotent (prd AC4/AC5).
func TestProxy_SetGetUnset(t *testing.T) {
	g, buf := newProxyTestGlobals(t)
	if err := config.Save(config.Config{Token: "tok"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := runProxyCmd(t, g, "get"); err != nil {
		t.Fatalf("get with nothing set: %v", err)
	}
	for _, want := range []string{
		"Persisted proxy: (none)",
		"Effective proxy: (none)",
		"Source: direct",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("initial get output %q must contain %q", buf.String(), want)
		}
	}

	buf.Reset()
	if err := runProxyCmd(t, g, "set", "http://127.0.0.1:7890"); err != nil {
		t.Fatalf("set: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "http://127.0.0.1:7890") || !strings.Contains(out, config.Path()) {
		t.Errorf("set must report the value and the config path, got %q", out)
	}
	c, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load after set: %v", err)
	}
	if c.Proxy != "http://127.0.0.1:7890" || c.Token != "tok" {
		t.Errorf("set must rewrite only the proxy, got %+v", c)
	}

	buf.Reset()
	if err := runProxyCmd(t, g, "get"); err != nil {
		t.Fatalf("get after set: %v", err)
	}
	for _, want := range []string{
		"Persisted proxy: http://127.0.0.1:7890",
		"Effective proxy: http://127.0.0.1:7890",
		"Source: config",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("get after set output %q must contain %q", buf.String(), want)
		}
	}

	buf.Reset()
	if err := runProxyCmd(t, g, "unset"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	if !strings.Contains(buf.String(), "Proxy unset") || !strings.Contains(buf.String(), config.Path()) {
		t.Errorf("unset must report cleared + config path, got %q", buf.String())
	}
	c, err = config.Load()
	if err != nil {
		t.Fatalf("config.Load after unset: %v", err)
	}
	if c.Proxy != "" {
		t.Errorf("unset must clear the proxy, got %q", c.Proxy)
	}
	if c.Token != "tok" {
		t.Errorf("unset must preserve the token, got %q", c.Token)
	}
	if _, statErr := os.Stat(config.Path()); statErr != nil {
		t.Errorf("unset must rewrite the config file, never delete it: %v", statErr)
	}

	// Idempotent: a second unset on an already-empty proxy stays exit 0.
	buf.Reset()
	if err := runProxyCmd(t, g, "unset"); err != nil {
		t.Fatalf("second unset must succeed: %v", err)
	}
}

// TestProxy_SetBadURLIsUsageErrorNoSave proves the validation gate (prd
// R5/AC4): every malformed URL is rejected as a usage error (exit 1 via
// the global map) before any write, leaving a pre-existing config intact.
func TestProxy_SetBadURLIsUsageErrorNoSave(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"no scheme", "not-a-url", "missing a scheme"},
		{"missing host", "http://", "missing a host"},
		{"unsupported scheme", "ftp://127.0.0.1:21", "unsupported proxy scheme"},
		{"empty url", "", "proxy URL is empty"},
		{"whitespace only", "   ", "proxy URL is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := newProxyTestGlobals(t)
			if err := config.Save(config.Config{Token: "tok", Proxy: "http://prior:1"}); err != nil {
				t.Fatalf("seed config: %v", err)
			}
			err := runProxyCmd(t, g, "set", tc.url)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
			if got := exitCodeFor(err, false); got != 1 {
				t.Errorf("bad URL must map to exit 1, got %d", got)
			}
			c, loadErr := config.Load()
			if loadErr != nil {
				t.Fatalf("config.Load: %v", loadErr)
			}
			if c.Proxy != "http://prior:1" || c.Token != "tok" {
				t.Errorf("rejected set must leave the config unchanged, got %+v", c)
			}
		})
	}
}

// TestProxy_UsageErrors covers the argument-count and action dispatch
// errors: each maps to exit 1 and touches nothing.
func TestProxy_UsageErrors(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"set without url", []string{"set"}, "requires exactly one"},
		{"set with two urls", []string{"set", "http://a:1", "http://b:2"}, "requires exactly one"},
		{"get with argument", []string{"get", "extra"}, "takes no arguments"},
		{"unset with argument", []string{"unset", "extra"}, "takes no arguments"},
		{"unknown action", []string{"bogus"}, "unknown proxy action"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := newProxyTestGlobals(t)
			err := runProxyCmd(t, g, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
			if got := exitCodeFor(err, false); got != 1 {
				t.Errorf("usage error must map to exit 1, got %d", got)
			}
			c, _ := config.Load()
			if c != (config.Config{}) {
				t.Errorf("usage error must not write config, got %+v", c)
			}
		})
	}
}

// TestProxy_JsonStatusShape pins the --json contract of get / bare proxy
// ({"persisted","effective","source"} — prd R1) plus the env-source
// detection and the set/unset result payloads.
func TestProxy_JsonStatusShape(t *testing.T) {
	// Source config: a persisted proxy shows up in all three fields.
	g, buf := newProxyTestGlobals(t)
	if err := config.Save(config.Config{Token: "t", Proxy: "http://p:1"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := runProxyCmd(t, g, "--json", "get"); err != nil {
		t.Fatalf("json get: %v", err)
	}
	assertJSONFields(t, buf.String(), map[string]any{
		"persisted": "http://p:1",
		"effective": "http://p:1",
		"source":    "config",
	})

	// Source env: nothing persisted (fresh config dir), HTTPS_PROXY set.
	// The explicit effective value stays empty — env applies via
	// DefaultTransport, which only the source field makes observable.
	g3, buf3 := newProxyTestGlobals(t)
	t.Setenv("HTTPS_PROXY", "http://envp:1")
	if err := runProxyCmd(t, g3, "--json", "get"); err != nil {
		t.Fatalf("json get (env): %v", err)
	}
	assertJSONFields(t, buf3.String(), map[string]any{
		"persisted": "",
		"effective": "",
		"source":    "env",
	})

	// set / unset --json result payloads.
	g2, buf2 := newProxyTestGlobals(t)
	if err := runProxyCmd(t, g2, "--json", "set", "http://127.0.0.1:7890"); err != nil {
		t.Fatalf("json set: %v", err)
	}
	assertJSONFields(t, buf2.String(), map[string]any{
		"status": "set",
		"proxy":  "http://127.0.0.1:7890",
		"config": config.Path(),
	})
	buf2.Reset()
	if err := runProxyCmd(t, g2, "--json", "unset"); err != nil {
		t.Fatalf("json unset: %v", err)
	}
	assertJSONFields(t, buf2.String(), map[string]any{
		"status": "unset",
		"config": config.Path(),
	})
	var res map[string]any
	if err := json.Unmarshal([]byte(buf2.String()), &res); err != nil {
		t.Fatalf("json unset output %q: %v", buf2.String(), err)
	}
	if _, ok := res["proxy"]; ok {
		t.Errorf("json unset must omit the proxy field, got %v", res)
	}
}

// assertJSONFields unmarshals a single JSON object from out and checks it
// has exactly the want keys with the want values.
func assertJSONFields(t *testing.T, out string, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json output %q: %v", out, err)
	}
	if len(got) != len(want) {
		t.Errorf("json object has %d fields (%v), want exactly %d (%v)", len(got), got, len(want), want)
	}
	for k, v := range want {
		if gv, ok := got[k]; !ok || gv != v {
			t.Errorf("json field %q = %v, want %v", k, gv, v)
		}
	}
}

// TestProxy_BareFriendlyStatus proves bare `civ proxy` prints the same
// status report as get (design D5).
func TestProxy_BareFriendlyStatus(t *testing.T) {
	g, buf := newProxyTestGlobals(t)
	if err := runProxyCmd(t, g); err != nil {
		t.Fatalf("bare proxy: %v", err)
	}
	for _, want := range []string{"Persisted proxy: (none)", "Effective proxy: (none)", "Source: direct"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("bare proxy output %q must contain %q", buf.String(), want)
		}
	}
}

// TestProxy_VerboseDebugNamesProxy proves the --verbose contract (prd R6):
// with a proxy in effect a [DEBUG] line names it and its source on stderr;
// with none configured the line names the source instead.
func TestProxy_VerboseDebugNamesProxy(t *testing.T) {
	g, _ := newProxyTestGlobals(t)
	if err := config.Save(config.Config{Proxy: "http://127.0.0.1:7890"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	app.SetVerbose(true)
	t.Cleanup(func() { app.SetVerbose(false) })
	logs := captureLogs(t, func() {
		if err := runProxyCmd(t, g, "--verbose", "get"); err != nil {
			t.Fatalf("verbose get: %v", err)
		}
	})
	if !strings.Contains(logs, "[DEBUG] proxy: routing through http://127.0.0.1:7890 (source: config)") {
		t.Errorf("verbose get must debug-log the effective proxy + source, got %q", logs)
	}

	g2, _ := newProxyTestGlobals(t)
	logs = captureLogs(t, func() {
		if err := runProxyCmd(t, g2, "--verbose", "get"); err != nil {
			t.Fatalf("verbose get (none): %v", err)
		}
	})
	if !strings.Contains(logs, "[DEBUG] proxy: no explicit proxy configured (source: direct)") {
		t.Errorf("verbose get with nothing set must name the source, got %q", logs)
	}
}

// TestClientFactories_ApplyProxy proves the C3/C4 drift-prevention
// requirement offline: the shared clientOptions and BOTH production
// factory seams (newAPIClient, newSyncClient) route requests through the
// resolved proxy, with the --proxy flag beating a persisted-but-different
// config proxy (dialing the saved one would fail DNS and hit no local
// server). An http target shows the classic absolute-URI forward-proxy
// request end to end; the production factories target https://civitai.com,
// which a fixed http proxy reaches via a CONNECT tunnel — the mini proxy
// records the CONNECT and refuses to tunnel (no offline TLS to
// civitai.com), so Get fails fast under a short context after exactly one
// proxy hit.
func TestClientFactories_ApplyProxy(t *testing.T) {
	isolateConfigDir(t)
	t.Setenv("CIVITAI_API_KEY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "")
	if err := config.Save(config.Config{Token: "tok", Proxy: "http://configured-not-dialed:1"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	var mu sync.Mutex
	var seen []string
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		targetHits++
		mu.Unlock()
		w.Write([]byte(`{"items":[]}`))
	}))
	t.Cleanup(target.Close)

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodConnect: // https target through an http proxy
			mu.Lock()
			seen = append(seen, "CONNECT "+r.URL.Host)
			mu.Unlock()
			http.Error(w, "mini proxy does not tunnel offline", http.StatusBadGateway)
		default: // http target: absolute-URI forward request; relay it
			mu.Lock()
			seen = append(seen, r.URL.String())
			mu.Unlock()
			resp, err := http.Get(target.URL + r.URL.RequestURI())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		}
	}))
	t.Cleanup(proxySrv.Close)

	g := &globals{Proxy: proxySrv.URL, Timeout: 5 * time.Second, out: io.Discard}

	// Happy path through the shared options: the request lands on the
	// proxy as an absolute URI and is relayed to the target.
	t.Run("clientOptions round trip", func(t *testing.T) {
		c := api.NewClient(append([]api.Option{api.WithBaseURL(target.URL)}, clientOptions(g)...)...)
		if _, err := c.Get(context.Background(), "/models", nil); err != nil {
			t.Fatalf("Get through the local forward proxy must succeed offline: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(seen) != 1 || !strings.HasPrefix(seen[0], target.URL+"/models") {
			t.Errorf("forward proxy saw %v, want the absolute URI %s/models (flag proxy must beat the saved one)", seen, target.URL)
		}
		if targetHits != 1 {
			t.Errorf("target served %d requests, want 1 relayed hit", targetHits)
		}
	})

	// The production factory vars keep the production https base URL, so
	// their proxy wiring is observable as a CONNECT for civitai.com.
	factories := []struct {
		name  string
		build func() *api.Client
	}{
		{"newAPIClient", func() *api.Client { return newAPIClient(g) }},
		{"newSyncClient", func() *api.Client { return newSyncClient(g, 2) }},
	}
	for _, f := range factories {
		t.Run(f.name, func(t *testing.T) {
			mu.Lock()
			seen = nil
			mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			if _, err := f.build().Get(ctx, "/models", nil); err == nil {
				t.Fatal("Get must fail: the mini proxy refuses to tunnel to civitai.com offline")
			}
			mu.Lock()
			defer mu.Unlock()
			if len(seen) != 1 || seen[0] != "CONNECT civitai.com:443" {
				t.Errorf("forward proxy saw %v, want one CONNECT for civitai.com:443 (the flag proxy must beat the saved one)", seen)
			}
		})
	}
}

// TestPermuteArgs_ProxyFlagAfterQuery proves the C3 parse concern: the
// value-carrying --proxy flag is consumed correctly even after the
// positional query, because permuteArgs consults the registered FlagSet
// (fs.Lookup) rather than a hand-maintained value-flag list.
func TestPermuteArgs_ProxyFlagAfterQuery(t *testing.T) {
	var g globals
	var o searchOptions
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerGlobalFlags(fs, &g)
	o.register(fs)

	got := permuteArgs(fs, []string{"anima", "--proxy", "http://127.0.0.1:7890", "--limit", "1"})
	want := []string{"--proxy", "http://127.0.0.1:7890", "--limit", "1", "anima"}
	if len(got) != len(want) {
		t.Fatalf("permuted args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("permuted args = %v, want %v", got, want)
		}
	}
	if err := fs.Parse(got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if g.Proxy != "http://127.0.0.1:7890" {
		t.Errorf("--proxy after the query must bind, got %q", g.Proxy)
	}
	if o.limit != 1 || len(fs.Args()) != 1 || fs.Args()[0] != "anima" {
		t.Errorf("limit = %d, args = %v; the query must stay positional", o.limit, fs.Args())
	}
}

// TestClientOptions_VerboseNamesProxy proves the R6 contract for data
// commands: building a client with --verbose debug-logs the effective
// proxy and its source (flag beating the saved config), and stays silent
// when verbose is off.
func TestClientOptions_VerboseNamesProxy(t *testing.T) {
	isolateConfigDir(t)
	t.Setenv("CIVITAI_API_KEY", "")
	if err := config.Save(config.Config{Token: "tok", Proxy: "http://config:1"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	app.SetVerbose(true)
	t.Cleanup(func() { app.SetVerbose(false) })

	logs := captureLogs(t, func() { clientOptions(&globals{Proxy: "http://flag:1"}) })
	if !strings.Contains(logs, "proxy: routing through http://flag:1 (source: flag)") {
		t.Errorf("verbose client build must name the flag proxy beating config, got %q", logs)
	}

	logs = captureLogs(t, func() { clientOptions(&globals{}) })
	if !strings.Contains(logs, "proxy: routing through http://config:1 (source: config)") {
		t.Errorf("verbose client build must name the config proxy, got %q", logs)
	}

	app.SetVerbose(false)
	logs = captureLogs(t, func() { clientOptions(&globals{Proxy: "http://flag:1"}) })
	if strings.Contains(logs, "proxy:") {
		t.Errorf("verbose-off client build must not log the proxy, got %q", logs)
	}
}

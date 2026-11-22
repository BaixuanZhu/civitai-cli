# Design — civ proxy support

Technical design for PRD `11-22-proxy-support` (model A). Stdlib-only,
Windows-only, offline-testable. Mirrors the token precedence/persistence pattern.

## 1. Architecture & boundaries

New leaf package `internal/proxy` (imports only `net/http`, `net/url`, `fmt`,
`strings`). It is the single source of truth for proxy validation, precedence,
and transport construction — reused by `api`, `cmd`, `config`, `upgrade`.

```
internal/proxy/proxy.go        (NEW leaf, stdlib only)
  Validate(rawURL) (*url.URL, error)
  Resolve(flagProxy, configProxy string) string        // pure precedence
  NewTransport(rawURL) (*http.Transport, error)        // clone DefaultTransport + fixed Proxy

internal/config/config.go      (EDIT)
  Config += Proxy string `json:"proxy"`
  ResolveProxy(flagProxy string) string                // Load + proxy.Resolve  (mirrors ResolveToken)

internal/api/client.go         (EDIT)
  WithProxy(rawURL string) Option                      // proxy.NewTransport -> c.httpClient.Transport

internal/cmd/globals.go        (EDIT)
  globals += Proxy string
  registerGlobalFlags += --proxy
  newAPIClient += api.WithProxy(config.ResolveProxy(g.Proxy))
internal/cmd/sync.go           (EDIT)
  newSyncClient += same proxy option (shared options builder to avoid drift)
internal/cmd/proxy.go          (NEW)  civ proxy <set|get|unset>, pure runProxy + execute
main.go                        (EDIT)  dispatch case "proxy"; usage() += proxy + --proxy + env note

internal/upgrade/upgrade.go    (EDIT)
  fetchLatestRelease uses downloadClient (unify the two clients)
  Run() start: apply config.ResolveProxy("") -> downloadClient.Transport via proxy.NewTransport
```

Dependency direction stays one-way (spec): `proxy` is a leaf (no internal
imports). `config -> proxy`, `api -> proxy`, `cmd -> {api,config,proxy}`,
`upgrade -> {app,config,proxy}`. No cycles. `app` is untouched (resolution needs
config, which app cannot import).

## 2. Contracts & data flow

### proxy.Validate
- `url.Parse(rawURL)`; require `Scheme ∈ {http, https, socks5, socks5h}` and
  `Host != ""`. Reject empty/missing-scheme/missing-host/unsupported-scheme with
  a descriptive error. Returns the parsed `*url.URL` on success.
- SOCKS5 is supported by `net/http.Transport`'s dialer (Go ≥1.9); no extra dep.

### proxy.Resolve (pure)
```
if strings.TrimSpace(flagProxy) != "" { return flagProxy }
return configProxy   // may be "" -> caller leaves DefaultTransport (env)
```
Env is NOT read here: when the result is "", the client keeps nil Transport so
`http.DefaultTransport` applies `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` as today.

### proxy.NewTransport
```
u, err := Validate(rawURL); if err != nil { return nil, err }
tr := http.DefaultTransport.(*http.Transport).Clone()   // preserve dial/TLS/HTTP2/timeouts
tr.Proxy = func(*http.Request) (*url.URL, error) { return u, nil }
return tr, nil
```
Fixed proxy for all civ outbound; NO_PROXY intentionally not consulted here
(D8). Clone preserves ForceAttemptHTTP2, dial keepalive, TLS handshake timeout.

### config
- `Config.Proxy` added; JSON `{"token":"...","proxy":"..."}`. Old files without
  `proxy` decode to "" (backward compatible, no migration).
- `ResolveProxy(flag)`: `c,_ := Load(); return proxy.Resolve(flag, c.Proxy)`.
  Load degrades to zero on corrupt/missing (existing behavior) → anonymous/direct.
- `Save`/`Delete` unchanged (atomic 0600). `unset` loads, zeroes Proxy, Saves
  (preserves Token); it does NOT Delete the file (token must survive).

### api.WithProxy
```
func WithProxy(rawURL string) Option {
  return func(c *Client) {
    if strings.TrimSpace(rawURL) == "" { return }      // keep DefaultTransport (env)
    tr, err := proxy.NewTransport(rawURL)
    if err != nil { app.Warnf("invalid proxy %q (%v); falling back to env/direct", rawURL, err); return }
    c.httpClient.Transport = tr
  }
}
```
Degrade-on-invalid mirrors config's corrupt-file philosophy (D6). Order vs
WithHTTPClient is documented (WithProxy sets Transport on the current client;
tests using WithHTTPClient+WithProxy must order accordingly). WithTimeout still
sets Timeout independently.

### cmd wiring
- `globals.Proxy` + `--proxy` flag: `"proxy URL for this invocation; overrides the saved proxy and HTTP(S)_PROXY"`.
- Factor a shared `clientOptions(g) []api.Option` (token + timeout + proxy) used
  by BOTH `newAPIClient` and `newSyncClient` so the two seams cannot drift
  (code-reuse guide). Concurrency stays sync-specific.
- `--verbose`: `app.Infof`/`Debugf` already logs `GET <url>`; add a debug line
  naming the effective proxy + source when set.

### cmd/proxy.go (pure runProxy + execute)
- `set <url>`: `proxy.Validate` → on error return usage error (exit 1, NO save);
  `c,_ := config.Load(); c.Proxy = url; config.Save(c)`; report value + `config.Path()`.
- `get` / bare `civ proxy`: report persisted (`config.Load().Proxy`), effective
  (`config.ResolveProxy("")`), and source (config / env / direct). `--json` →
  `{"persisted":...,"effective":...,"source":...}`.
- `unset`: `c,_ := config.Load(); c.Proxy=""; config.Save(c)` (idempotent);
  report cleared. Token preserved.
- Uses `execute("proxy", ...)` (global exit map: usage→1). No network. Mirrors
  login/skill structure (pure run, injectable stdout via globals).

### upgrade
- Unify: `fetchLatestRelease` switches `http.DefaultClient.Do` → `downloadClient.Do`
  so all upgrade traffic uses one client (removes the global-DefaultClient mutation).
- `Run()` start:
  ```
  if p := config.ResolveProxy(""); p != "" {
    if tr, err := proxy.NewTransport(p); err == nil { downloadClient.Transport = tr } else { app.Warnf(...) }
  }
  ```
  Persisted proxy + env (nil Transport fallback) both honored. `--proxy` flag not
  parsed for upgrade (D7) — documented.

## 3. Compatibility & migration

- Config schema additive (`proxy` key); existing token-only files decode fine.
- No behavior change when no proxy configured: nil Transport → DefaultTransport
  → env exactly as today (R4/AC2). Existing env-proxy users unaffected.
- `--proxy` is a new global flag; `permuteArgs` already handles value flags after
  the command name (no parser change needed).
- install.ps1 / NSIS unchanged (env proxy already native).

## 4. Trade-offs

- Fixed proxy ignores NO_PROXY when explicitly set (D8): acceptable for two
  target hosts; avoids importing `golang.org/x/net/http/httpproxy` (zero-dep).
- New `internal/proxy` leaf vs stuffing helpers into `app`: chosen leaf keeps
  `app` a zero-internal-import infra package and gives proxy a clear home
  (matches env/report/hashcache/skill pattern). Cost: one spec update.
- upgrade unifies onto downloadClient (small refactor) rather than mutating
  global `http.DefaultClient.Transport`: cleaner, no process-wide side effect.
- `--proxy` not wired into upgrade's flag parsing (upgrade bypasses execute):
  persisted config covers the real need; avoids refactoring upgrade into the
  global-flag runner. Documented limitation.

## 5. Testing strategy (offline, httptest)

- `proxy`: table-driven Validate (valid http/https/socks5/socks5h; empty; no
  scheme; no host; ftp rejected), Resolve precedence, NewTransport (Proxy func
  returns the URL; clone keeps ForceAttemptHTTP2).
- `config.ResolveProxy`: temp config dir (set APPDATA/XDG_CONFIG_HOME to t.TempDir),
  write token+proxy, assert flag>config>"" precedence; corrupt file degrades.
- `api.WithProxy`: start an httptest server acting as a forward proxy (handler
  sees absolute-URI `r.URL`), build client `WithProxy(proxyServer.URL)` +
  `WithBaseURL(target)`, assert the proxy received the request. Invalid proxy →
  warn + nil Transport (no panic).
- `cmd/proxy`: runProxy set/get/unset against temp config; bad URL → exit 1 +
  config unchanged; `--json` shape; unset preserves token. Mirror login_test.
- `cmd` factories: assert clientOptions includes the resolved proxy (swap
  newAPIClient seam in tests as today).
- `upgrade`: unit-test the apply step sets downloadClient.Transport when config
  proxy present; no real network.
- Full gate: `go vet ./...`, `go test ./...`, `go test -race ./...`.
- Live integration smoke: proxy path is unit-covered (httptest); the anonymous
  online smoke need not add a proxy case (no registered local proxy on the
  runner). Optional manual live check with a real proxy before release.

## 6. Operational / rollback

- Pure additive feature; rollback = revert the commit(s). No data migration.
- A bad persisted proxy degrades to env/direct (warn), never hard-fails a read.
- Spec update (directory-structure.md) lands with the code (R8).
- Release: lands in the NEXT version (e.g. 0.2.0); CHANGELOG `## [x.y.z]`
  section added before tagging (awk contract). Not retroactive to 0.1.1.
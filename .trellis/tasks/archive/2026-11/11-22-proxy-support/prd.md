# civ proxy support

## Goal

Let users route civ's outbound traffic through an HTTP/HTTPS/SOCKS5 proxy
conveniently and persistently, so `civ login` / `search` / `show` / `vet` /
`sync` / `upgrade` work when a direct connection to civitai.com fails
(timeout / DNS pollution). User value: a one-time `civ proxy set` instead of
per-shell environment variables, plus a `--proxy` flag for ad-hoc overrides.

## Background / Confirmed Facts (repository evidence)

- **Env proxy already works today.** The API client builds
  `&http.Client{Timeout: defaultTimeout}` with a nil Transport
  ([internal/api/client.go:98](../../../internal/api/client.go)), so it uses
  `http.DefaultTransport` (`Proxy = http.ProxyFromEnvironment`). Upgrade uses
  `http.DefaultClient` for the GitHub API call
  ([internal/upgrade/upgrade.go:161](../../../internal/upgrade/upgrade.go)) and
  `downloadClient = &http.Client{Timeout: 0}` for assets
  ([internal/upgrade/upgrade.go:44](../../../internal/upgrade/upgrade.go)); both
  nil Transport. So `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` are already honored
  everywhere. The user's failing `civ login` was a direct dial to a civitai.com
  IPv6 address that timed out (no proxy env set). Immediate workaround: set
  `HTTPS_PROXY` and retry.
- **Go does NOT read the Windows system proxy.** `ProxyFromEnvironment` reads
  only env vars, not the WinINET/IE proxy in `HKCU\...\Internet Settings`. A
  system-proxy mode would need a registry read — explicitly OUT OF SCOPE (user
  chose model A, not C).
- **Config is extensible.** `config.Config` holds only `Token`; its doc comment
  anticipates future keys
  ([internal/config/config.go:21](../../../internal/config/config.go)).
  `Load`/`Save`/`Delete` are atomic, 0600, degrade-to-zero on corrupt/missing.
  `ResolveToken` is the precedence pattern to mirror
  ([internal/config/config.go:123](../../../internal/config/config.go)).
- **Two client factory seams.** `newAPIClient(g)`
  ([internal/cmd/globals.go:63](../../../internal/cmd/globals.go)) and
  `newSyncClient(g, concurrency)` (internal/cmd/sync.go) build every API client;
  both must apply the proxy. `globals` + `registerGlobalFlags`
  ([internal/cmd/globals.go:26](../../../internal/cmd/globals.go),
  [:37](../../../internal/cmd/globals.go)) carry/bind global flags.
- **Functional-options client.** `api.NewClient(opts...)`, `WithHTTPClient`,
  `WithTimeout` ([internal/api/client.go:43-109](../../../internal/api/client.go)).
  A new `WithProxy` option clones DefaultTransport and sets a fixed `Proxy`.
- **install.ps1 already honors proxy** natively via `Invoke-WebRequest`
  ([install.ps1:27](../../../install.ps1)); no change needed there.
- **Layering (spec).** `app` is a zero-internal-import leaf; `upgrade` depends on
  `app` only; new subcommands live in `cmd` (pure `run*` + `execute`); Civitai
  endpoints funnel through `api.Client.Get` (upgrade→GitHub is the accepted
  exception). See
  [.trellis/spec/backend/directory-structure.md](../../spec/backend/directory-structure.md).
- **Contracts.** Zero third-party deps (stdlib only); Windows-only build;
  all-English code/comments/CLI output; offline httptest tests; exit codes
  0/1/2/3/4. See
  [.trellis/spec/backend/quality-guidelines.md](../../spec/backend/quality-guidelines.md).

## Key Decisions

- D1 (user, this turn). Configuration model **A**: `civ proxy set/get/unset`
  subcommand (persisted to config) + `--proxy` global flag (ad-hoc) + preserved
  env support. NO Windows system-proxy auto-detect (model C rejected).
- D2. Precedence mirrors the token chain: `--proxy` flag > persisted
  `config.Proxy` > standard `HTTP_PROXY`/`HTTPS_PROXY` env (via DefaultTransport
  when no explicit proxy is set). An explicit proxy (flag/config) overrides env.
- D3. New leaf package `internal/proxy` (stdlib net/http + net/url only) owns
  `Validate`, `Resolve` (pure flag>config strings), `NewTransport` (clone
  DefaultTransport + fixed Proxy). Reused by api, cmd, config, upgrade — single
  source of truth, no duplication.
- D4. `config.Config` gains `Proxy string`; `config.ResolveProxy(flag)` mirrors
  `ResolveToken` (loads config, delegates to `proxy.Resolve`).
- D5. Subcommand shape `civ proxy <set|get|unset>` (mirrors skill/login UX):
  `set <url>` validates then persists; `get` shows persisted + effective proxy
  and source; `unset` clears (idempotent). `--json` supported. Bare `civ proxy`
  prints effective status (friendly default).
- D6. Validation accepts schemes http/https/socks5/socks5h with a non-empty
  host:port; malformed input at `set` is a usage error (exit 1, no save). An
  invalid persisted value at runtime degrades gracefully (warn + fall back to
  env/direct), mirroring config's corrupt-file philosophy.
- D7. Upgrade applies the persisted config proxy + env to BOTH its clients
  (unify the GitHub API call onto `downloadClient`, set its Transport once).
  The `--proxy` flag applies to API commands; upgrade does not parse global
  flags, so it honors persisted config + env (documented).
- D8. NO_PROXY is honored only in env-fallback mode (no explicit proxy). With an
  explicit flag/config proxy, all civ outbound (civitai.com + github.com) uses
  it. Documented tradeoff; civ targets two hosts, so this is acceptable.

## Requirements

- R1. `civ proxy set <url>` validates and persists the proxy to config.json;
  `civ proxy get` reports persisted + effective proxy and source; `civ proxy
  unset` clears it (idempotent). `--json` emits machine-readable results.
- R2. `--proxy <url>` global flag (after the command name) overrides the
  persisted proxy for that invocation, on every API command.
- R3. The resolved proxy applies to ALL civ outbound HTTP: both API client
  factories (`newAPIClient`, `newSyncClient`) and the upgrade downloader +
  GitHub API call. No command bypasses it.
- R4. Precedence is exactly D2; with nothing set, behavior equals today (env via
  DefaultTransport). Existing env-proxy users see no regression.
- R5. Proxy URL validation per D6 at the system boundary; malformed `set` input
  → exit 1, no config written; invalid persisted value → warn + degrade.
- R6. Output conventions preserved: results to stdout, logs/warnings to stderr;
  `--verbose` logs the proxy in use; proxy commands report what changed and the
  config path.
- R7. Stdlib-only; Windows-only build unchanged; all-English; offline httptest
  tests; `civ help` + README document the mechanism, precedence, and env
  workaround.
- R8. Spec update: add `internal/proxy` leaf + `config.Proxy`/`ResolveProxy` +
  upgrade→config/proxy dependency to directory-structure.md.

## Acceptance Criteria (observable)

- [ ] AC1. With a proxy configured via `civ proxy set` (or `--proxy`), a request
  routes through it — proven offline by an httptest forward-proxy receiving the
  absolute-URI request; `--verbose` shows the proxy.
- [ ] AC2. Precedence observable in tests: `--proxy` beats config; config beats
  env; none set → DefaultTransport (env) behavior unchanged.
- [ ] AC3. Upgrade applies the persisted proxy to both the GitHub API call and
  the asset download (unit test asserts the Transport is set).
- [ ] AC4. `civ proxy set not-a-url` → exit 1, clear message, config unchanged;
  `civ proxy set http://127.0.0.1:7890` → exit 0, value persisted, read back by
  `get` in a new process.
- [ ] AC5. `civ proxy unset` clears the persisted proxy (token preserved);
  subsequent resolution falls back to env/direct.
- [ ] AC6. `go vet ./...`, `go test ./...`, `go test -race ./...` green; existing
  tests unaffected; new table-driven tests cover Validate, Resolve, NewTransport,
  config.ResolveProxy, WithProxy wiring, and runProxy set/get/unset.
- [ ] AC7. `civ help` lists `proxy` + `--proxy`; README has a proxy section
  (mechanism, precedence, env workaround, NO_PROXY note); CHANGELOG entry added
  for the next version.

## Out of Scope

- Windows system-proxy (WinINET registry) auto-detection / "system" mode
  (model C, rejected by user).
- PAC / WPAD auto-discovery; DNS-over-HTTPS; a general retry/DNS overhaul.
- Changing install.ps1's existing native proxy handling.
- `--proxy` flag support for the `upgrade` command itself (upgrade honors
  persisted config + env; D7).
- An explicit "force direct" override that suppresses a set env proxy (users
  unset env to go direct; deferred).
- Interactive TUI beyond the established stdin pattern (proxy set is arg-driven).
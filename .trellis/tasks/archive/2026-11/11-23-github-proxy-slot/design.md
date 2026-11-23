# Design: github proxy slot (child 11-23-github-proxy-slot)

## Summary

Add a second, GitHub-scoped proxy slot to config and the `civ proxy` command. The civitai slot (`Proxy`) keeps every current behavior; the new `GithubProxy` slot is persisted, reported, and validated here, and only consumed by `civ upgrade` (child 11-23-upgrade-direct).

## Current state (verified in code)

- `internal/config/config.go`: `Config{Token, Proxy}` + `ResolveProxy(flagProxy)` (flag > saved); load is tolerant (missing file → zero value).
- `internal/cmd/proxy.go`: `runProxy` dispatches set/get/unset positionals; `runProxySet` validates then load-modify-saves `cfg.Proxy`; status prints three lines (`Persisted proxy:`/`Effective proxy:`/`Source:`) resolved via `config.ResolveProxy("")` + `proxySource`; JSON shapes `proxyStatusJSON`/`proxyResultJSON`. The command registers no flags of its own today (`execute("proxy", proxyHelp, args, nil, runProxy)`).
- `proxy help` Notes document the single-slot precedence; root usage and README describe one saved proxy.

## Design decisions

- **D1 — Config schema.** `Config` gains `GithubProxy string \`json:"githubProxy,omitempty"\``. `ResolveGithubProxy(flagProxy string) string` mirrors `ResolveProxy` exactly (trim flag > saved slot), giving upgrade the same seam shape as the data commands. Zero-value tolerance gives old files an empty slot for free; a round-trip test pins it.
- **D2 — Command surface.** proxy gains its first own flag: a `--github` bool on `set` and `unset` (options struct + `register` func — the standard recipe; the runner appends globals as usual). Positionals stay `set <url>` / `unset`; `--github` switches the slot. Rationale: keeps the existing positional grammar intact (no breaking `set <url>` muscle memory) and reads naturally: `civ proxy set --github socks5h://127.0.0.1:1080`.
- **D3 — unset semantics.** Bare `unset` clears both slots; `unset --github` clears only github. Rationale: "unset" most often means "stop routing everything"; the partial form covers the realistic case (drop github acceleration, keep civitai relay). Documented in help; idempotent both ways.
- **D4 — Status rendering, additive.** The civitai block keeps its three lines byte-identical (integration assertions + scripts depend on them). The github scope appends: `GitHub persisted: <v|none>`, `GitHub effective: <v|none>`, `GitHub source: <config|env|direct>`. Effective = the saved slot only (the per-invocation --proxy flag is invisible to this report, same rule as civitai today). Source detection reuses `proxySource` logic against the slot value.
- **D5 — JSON, additive.** `proxyStatusJSON` gains `Github proxyScopeStatus \`json:"github"\`` (nested {persisted, effective, source}); top-level keys keep civitai semantics (old consumers unaffected). `proxyResultJSON` gains `Scope string \`json:"scope,omitempty"\`` set to "github" when `--github` was used (absent for civitai = old shape byte-compatible).
- **D6 — Scope naming in docs/help.** The two scopes are named **civitai** (data commands) and **github** (upgrade). `proxy help` Notes get a precedence recap per scope. Root usage `Proxy:` paragraph and README gain one clause each about the github slot.
- **D7 — No upgrade wiring here.** `internal/upgrade` untouched; the slot is dead config until 11-23-upgrade-direct reads it (that child's precedence: flag > github slot > env). Keeping the wiring child separate leaves this child verifiable on its own.

## Contracts touched

- config.json schema: additive field; old files load fine.
- `proxy get` human output: three appended lines (additive); `--json`: additive nested object.
- set/unset JSON: additive `scope` key. Exit codes unchanged.

## Test plan

- config: round-trip with both slots; old-file tolerance; ResolveGithubProxy precedence (flag > slot).
- cmd/proxy: table tests for set/unset both slots + idempotency; status text (civitai lines unchanged + github block, per-scope sources with/without env); JSON shapes (additive keys, scope field); validation-before-write preserved for --github.
- integration: new set --github / unset --github runs + snapshot assertions; existing proxy assertions unchanged.

## Rollback

Revert commit; config files holding `githubProxy` are tolerated by older builds? No — older builds ignore unknown JSON keys (tolerant decode), so rollback is safe both ways.

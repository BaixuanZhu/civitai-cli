# Design: upgrade direct (child 11-23-upgrade-direct)

## Summary

Remove the persisted-proxy auto-application from `civ upgrade` and add a per-invocation `--proxy <url>` flag. Upgrade's proxy precedence becomes: `--proxy` flag > environment (`http.DefaultTransport`) > direct. The saved config proxy (`civ proxy set`) no longer participates — it exists for civitai.com traffic on restrictive networks, and routing GitHub release traffic through shared proxy IPs is what triggers the 403 rate-limit the user hit.

## Current state (verified in code)

- `internal/upgrade/upgrade.go`: package var `downloadClient = &http.Client{Timeout: 0}`; `applyPersistedProxy()` reads `config.ResolveProxy("")` and installs `proxy.NewTransport(p)` on it; `Run()` calls `applyPersistedProxy()` first thing; both the GitHub API check (`fetchLatestRelease`) and `downloadOnce` ride `downloadClient`.
- `main.go` `case "upgrade"`: help tokens (`help`/`-h`/`-help`/`--help`, exactly one arg) print `upgrade.Help()`; otherwise `upgrade.Run()`.
- Tests: three `TestApplyPersistedProxy_*` tests pin today's behavior; `TestHelp` asserts the page says "the --proxy flag does not apply"; `TestHelp_SectionOrder` pins "no Options section, no embedded global rows".
- `internal/cmd/proxy.go` help and root usage claim upgrade honors the saved proxy — cross-doc updates needed (parent AC3).

## Design decisions

- **D1 — Precedence.** `--proxy <url>` (validated) > saved github slot (`Config.GithubProxy`, via `config.ResolveGithubProxy`) > env via nil `Transport` > direct. The civitai slot (`Config.Proxy`) is never read by upgrade — that coupling is what produced the 403. Rationale: the 403 comes from relay IPs; the github slot exists so users who need GitHub acceleration (e.g. a local relay port) can persist it without touching civitai traffic; env stays because a system-level env proxy is the user's explicit machine config and `NO_PROXY` still governs it. Depends on child 11-23-github-proxy-slot for the slot itself.
- **D2 — Arg parsing lives in `internal/upgrade`.** New signature `Run(args []string)`. A `flag.FlagSet` with `fs.Usage = func(){}` (mirroring the cmd runner) parses only `--proxy`; `flag.ErrHelp` and bare `help` are still intercepted by `main` BEFORE `Run`, so `Run` sees only real args. Unknown flags/extra positionals → clear stderr line + exit 1 (usage error, before any network). This keeps `main` a pure router (design P1) and preserves design D4 (upgrade outside the cmd runner).
- **D3 — Per-run transport application.** `applyPersistedProxy` is deleted. In its place, `Run` resolves through `config.ResolveGithubProxy(flagValue)`: empty result → leave `downloadClient.Transport` nil (env/direct); non-empty → `proxy.Validate` (invalid → `[WARN]` degrade line + keep nil transport, mirroring the old degrade philosophy — never fail an upgrade over a hand-edited config) and install the transport. `isolateDownloadClient` test seam is kept; a package-level note documents that `downloadClient` must start each run with a nil or flag/slot-derived transport so env fallback semantics hold.
- **D4 — Help page.** `upgrade.Help()` gains an `Options:` section documenting `--proxy <url>` and a rewritten `Proxy:` paragraph (direct default; `--proxy` wins; the github slot honored; env honored; the civitai slot intentionally not consulted). The page is still printed verbatim by `main` with no global-flag block. `TestHelp` and `TestHelp_SectionOrder` are updated: the "--proxy flag does not apply" assertion flips to asserting the new Options row; the "no Options section" expectation flips to "has Options before Examples" (still Usage < Options < Examples < Exit codes order, matching the shared layout audit's spirit for a flagless-until-now page).
- **D5 — Nothing else reads upgrade's proxy path.** `internal/cmd` commands keep `config.ResolveProxy(g.Proxy)` semantics untouched (parent constraint; `civ proxy` behavior unchanged for them).

## Contracts touched

- `upgrade.Run()` → `upgrade.Run(args []string)` — internal package, single caller in `main`; no external consumer.
- Help-page wording contract (stdout, exit 0) — user-visible, CHANGELOG + README updated in same change.

## Test plan

- Replace the three `TestApplyPersistedProxy_*` with flag-driven equivalents (config saved + no flag → transport nil; valid flag → transport Proxy URL matches; invalid flag → exit-1-before-network via a `Run` seam that fails on any dial; missing flag value / extra args → usage error).
- Help tests updated per D4.
- No new fixtures; existing `setConfigDir` helper reused for the "saved proxy ignored" case.

## Rollback

Single-commit child; revert restores `applyPersistedProxy` and `Run()` signature. No data migration (config file untouched).

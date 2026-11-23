# upgrade: default direct to GitHub, stop auto-applying persisted proxy

## Goal

civ upgrade no longer routes GitHub API/asset traffic through the persisted proxy by default (shared proxy IPs trigger GitHub 403); env proxies stay honored via the default transport and an explicit per-invocation --proxy flag is added for users who need a relay.

## Requirements

- **R1** — `civ upgrade` must NOT consult the civitai proxy slot (`Config.Proxy`, set via plain `civ proxy set`) when building its HTTP transport. The default is a direct connection (subject only to R2/R3).
- **R2** — Environment proxies (`HTTP(S)_PROXY`/`NO_PROXY` via `http.DefaultTransport`) keep applying when no flag and no github slot are set — unchanged stdlib semantics, so users behind a system-level proxy are not broken.
- **R3** — New `--proxy <url>` flag on `civ upgrade` sets an explicit relay for that invocation only, winning over the github slot. The value is validated with `proxy.Validate`; an invalid value is a usage error: a clear stderr message and exit 1 BEFORE any network call.
- **R4** — The saved github slot (`Config.GithubProxy`, introduced by child 11-23-github-proxy-slot) IS consulted by upgrade: precedence `--proxy` flag > github slot > env > direct. An invalid hand-edited slot value degrades: warn on stderr, fall back to env (mirroring the old degrade philosophy), never a failed upgrade.
- **R5** — `internal/upgrade` keeps owning its argument handling: `main` still routes bare help tokens (`civ upgrade help` / `-h` / `--help` → page, stdout, exit 0) and otherwise hands `os.Args[2:]` to the package. Upgrade stays outside the `cmd` runner (design D4 of task 11-22-command-help).
- **R6** — The `civ upgrade help` page documents the new behavior honestly: default direct, env honored, github slot honored, `--proxy` flag documented in an Options section, and no stale "the --proxy flag does not apply" wording.
- **R7** — Civitai-slot semantics for all data commands are unchanged (the `civ proxy` command surface itself is extended by child 11-23-github-proxy-slot, which this child depends on).

## Acceptance Criteria

- [ ] **AC1** Unit test: with a valid proxy in the civitai slot and no flag, `downloadClient.Transport` stays nil after run setup (civitai slot provably ignored); the GitHub check would go direct.
- [ ] **AC1b** Unit test: with a valid `githubProxy` saved and no flag, the transport's Proxy resolves to the slot URL (slot consumed); an invalid hand-edited slot warns on stderr and degrades to nil transport (env fallback), never failing the run.
- [ ] **AC2** Unit test: `--proxy http://127.0.0.1:7890` (with a saved github slot present) installs a transport whose Proxy resolves to the FLAG URL — flag wins over the slot — for both the API check and the asset download (one shared client).
- [ ] **AC3** Unit test: `--proxy not-a-url` fails before any network dial, exit 1, stderr names the validation problem.
- [ ] **AC4** Unit test: `--proxy` with a missing value and unknown extra args are usage errors (exit 1, no network).
- [ ] **AC5** Help page contains the new proxy wording and an Options section with `--proxy <url>`; it still carries no copy of the shared global-flag block (main prints it verbatim).
- [ ] **AC6** Old `applyPersistedProxy` tests are replaced (function removed); `isolateDownloadClient` seam semantics preserved for the new per-run application.
- [ ] **AC7** Integration: a new `upgrade_proxy_dead` case runs `civ upgrade --proxy http://127.0.0.1:1` against the live script and asserts the check fails (exit 1), proving the flag routes traffic; the real self-update case stays last and unchanged.
- [ ] **AC8** `go build ./...`, `go vet ./...`, `go test ./...` green.

## Constraints

- Zero new dependencies; stdlib + existing `internal/proxy`, `internal/config`, `internal/app` only.
- Exit-code contract: usage errors exit 1 (same bucket as other upgrade failures); network failures keep their existing mapping.
- The change is user-facing behavior: CHANGELOG `Unreleased` entry and README proxy paragraph update land with this child.

## Notes

- Motivation: GitHub rate-limits shared/datacenter proxy IPs with 403; the civitai proxy exists for civitai.com traffic on restrictive networks, not for GitHub release checks.
- Depends on: 11-23-github-proxy-slot (consumes `Config.GithubProxy` / `config.ResolveGithubProxy`).
- Migration for users who relied on the civitai slot for upgrade: `civ proxy set --github <url>` for a persistent GitHub relay, `--proxy <url>` per invocation, or `HTTPS_PROXY` for one shot.

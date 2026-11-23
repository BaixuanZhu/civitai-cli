# proxy: dedicated GitHub proxy slot (set --github) for upgrade

## Goal

Config gains a githubProxy slot (`civ proxy set --github <url>`) that applies only to `civ upgrade` traffic, so GitHub can be relayed through a different (e.g. acceleration) proxy than civitai; civitai slot semantics unchanged; status/get report both scopes with additive JSON.

## Requirements

- **R1** — `internal/config` gains a `GithubProxy` field (JSON key `githubProxy`, omitempty), tolerant-decoded so existing config files load unchanged. `ResolveGithubProxy(flagProxy string) string` mirrors `ResolveProxy`: non-empty flag wins, else the saved slot. The sibling `Token`/`Proxy` fields survive load-modify-save round trips.
- **R2** — `civ proxy set --github <url>` validates with `proxy.Validate` and saves ONLY the github slot; plain `civ proxy set <url>` saves ONLY the civitai slot (back-compat). Malformed URL exits 1 before any write (existing contract).
- **R3** — `civ proxy unset` clears BOTH slots (file rewritten, token preserved); `civ proxy unset --github` clears only the github slot. Both idempotent, exit 0.
- **R4** — Status report (`civ proxy`, `civ proxy get`) keeps today's first three lines byte-identical for the civitai scope (`Persisted proxy:` / `Effective proxy:` / `Source:`) and appends the github scope as three lines (`GitHub persisted:` / `GitHub effective:` / `GitHub source:`). Github effective = saved slot (empty when none); github source = "config" when the slot is set, else the same env/direct detection as civitai.
- **R5** — JSON shape is additive: `proxy get --json` keeps the existing top-level `{persisted, effective, source}` (civitai semantics) and adds a `"github": {persisted, effective, source}` object. set/unset result JSON gains an optional `"scope": "civitai"|"github"` field.
- **R6** — The github slot is consumed ONLY by `civ upgrade` (wired in child 11-23-upgrade-direct, which lands after this one). Data commands (search/show/vet/sync/tags) never read it. No command reads the civitai slot for GitHub traffic.
- **R7** — `proxy help` page documents the two scopes, `--github` on set/unset, and the per-scope precedence chains.
- **R8** — URL-rewriting mirror sites (ghproxy-style URL-prefix acceleration) are OUT of scope: the slots hold http/https/socks5/socks5h proxy URLs only.

## Acceptance Criteria

- [ ] **AC1** Unit tests: set --github persists only `githubProxy` (civitai slot and token untouched); set without the flag persists only `proxy`; unset clears both; unset --github clears only github; all idempotent.
- [ ] **AC2** Unit test: status human output keeps the three civitai lines verbatim and appends the three github lines; sources computed per scope (slot set → config; else env → env; else direct).
- [ ] **AC3** Unit test: `proxy get --json` old keys unchanged + `github` object present with the persisted slot; set/unset JSON carries `scope` when --github is used.
- [ ] **AC4** Old config files without `githubProxy` load with an empty slot (tolerant decode); a config with both fields round-trips.
- [ ] **AC5** integration-test.sh proxy section extended: set --github case + snapshot asserting `githubProxy` in config.json; unset --github case; status/get text assertions updated for the appended github lines while the existing civitai assertions keep passing unchanged.
- [ ] **AC6** `go build ./...`, `go vet ./...`, `go test ./...` green.
- [ ] **AC7** CHANGELOG Unreleased entry (Added: GitHub proxy slot) and README proxy section updated.

## Constraints

- Zero new dependencies; changes confined to `internal/config`, `internal/cmd/proxy.go`, docs, and tests. `internal/upgrade` is NOT touched by this child (contract owned by 11-23-upgrade-direct).
- Exit-code contract unchanged (usage errors exit 1).
- Human output for the civitai scope must not drift (integration assertions depend on it).

## Notes

- Motivation (user feedback): "github 有一些免费的加速站，如果完全不给配又有点一棍子打死" — a persistent github-only relay keeps upgrade usable for users who need acceleration without coupling it to the civitai proxy or retyping a flag.
- Precedence recap after both this child and 11-23-upgrade-direct land: upgrade = `--proxy` flag > github slot > env > direct; data commands = `--proxy` flag > civitai slot > env > direct. The two slots never cross.

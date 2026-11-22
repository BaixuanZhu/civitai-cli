# Implement — civ proxy support

Execution plan for PRD `11-22-proxy-support` + `design.md`. Model A.
Run only AFTER the final planning summary is approved and `task.py start` sets
status in_progress. Stdlib-only; offline tests; all-English.

## Ordered checklist

### Phase A — foundation (leaf, no internal deps)
- [ ] A1. Create `internal/proxy/proxy.go`: `Validate`, `Resolve`, `NewTransport`
  (design §2). GoDoc on every exported symbol. Clone DefaultTransport; fixed
  Proxy func; schemes http/https/socks5/socks5h.
- [ ] A2. `internal/proxy/proxy_test.go`: table-driven Validate / Resolve /
  NewTransport (design §5).
- [ ] A3. Gate: `go test ./internal/proxy/...` green.

### Phase B — persistence + client option (parallelizable, disjoint files)
- [ ] B1. `internal/config/config.go`: add `Proxy string` field + `ResolveProxy`
  (Load + proxy.Resolve). Update package doc comment (token → token+proxy).
- [ ] B2. `internal/config/config_test.go`: ResolveProxy precedence + corrupt
  degrade (temp config dir).
- [ ] B3. `internal/api/client.go`: add `WithProxy` Option (degrade-on-invalid).
- [ ] B4. `internal/api/client_test.go`: WithProxy forward-proxy httptest wiring +
  invalid-proxy degrade.
- [ ] B5. Gate: `go test ./internal/config/... ./internal/api/...` green.

### Phase C — command surface + wiring (parallelizable, disjoint files)
- [ ] C1. `internal/cmd/proxy.go`: pure `runProxy` (set/get/unset) + `Proxy(args)`
  entry via `execute`; `--json` shape; mirrors login/skill.
- [ ] C2. `internal/cmd/proxy_test.go`: set/get/unset, bad URL exit 1 no save,
  unset preserves token, --json (temp config; swap stdout).
- [ ] C3. `internal/cmd/globals.go`: `globals.Proxy` + `--proxy` flag; factor
  shared `clientOptions(g)`; `newAPIClient` uses it.
- [ ] C4. `internal/cmd/sync.go`: `newSyncClient` uses the same `clientOptions`.
- [ ] C5. `internal/upgrade/upgrade.go`: unify fetchLatestRelease onto
  downloadClient; apply config.ResolveProxy("") at Run() start.
- [ ] C6. `internal/upgrade/upgrade_test.go`: apply-step sets Transport when
  config proxy present.
- [ ] C7. Gate: `go test ./internal/cmd/... ./internal/upgrade/...` green.

### Phase D — integration + docs (main agent wires shared files)
- [ ] D1. `main.go`: dispatch `case "proxy": cmd.Proxy(os.Args[2:])`; usage()
  adds proxy command + `--proxy` global flag + env/NO_PROXY note.
- [ ] D2. `README.md`: proxy section (set/get/unset, --proxy, precedence, env
  workaround, NO_PROXY + upgrade-flag notes).
- [ ] D3. `.trellis/spec/backend/directory-structure.md`: add `internal/proxy`
  leaf, `config.Proxy`/`ResolveProxy`, upgrade→config/proxy dependency, "Where
  things go" row for proxy.
- [ ] D4. `CHANGELOG.md`: add `## [Unreleased]` (or next version) `### Added`
  proxy entry. (Date set only when tagging.)

### Phase E — full quality gate (trellis-check)
- [ ] E1. `go vet ./...`
- [ ] E2. `go test ./...`
- [ ] E3. `go test -race ./...`
- [ ] E4. `go build ./...`
- [ ] E5. `make build` + manual smoke: `civ proxy set http://127.0.0.1:PORT`,
  `civ proxy get`, `civ search anima --limit 1 --verbose` through a real local
  proxy (CIV_NO_BOOTSTRAP=1; do NOT touch real PATH/registry). `civ proxy unset`.
- [ ] E6. trellis-check cross-layer review (config field added across layers;
  two client seams; precedence; degrade paths).

## Parallelization (user preference: 能并行就并行)

- Phase A first (foundation). Then B1/B2 (config) ∥ B3/B4 (api) ∥ C5/C6 (upgrade)
  have DISJOINT file ownership — dispatch in parallel.
- C1/C2 (cmd/proxy) ∥ C3/C4 (globals/sync) disjoint files; both depend on A+B.
- Main agent wires shared files (main.go, README, CHANGELOG, spec) in Phase D
  after parallel tracks converge, then runs the full gate (E).
- If done inline instead of dispatched, follow A→B→C→D→E sequentially.

## Validation commands

```
go test ./internal/proxy/...                 # A
go test ./internal/config/... ./internal/api/...   # B
go test ./internal/cmd/... ./internal/upgrade/...  # C
go vet ./... && go test ./... && go test -race ./... && go build ./...   # E
```
Local smoke MUST set CIV_NO_BOOTSTRAP=1 (never write real PATH). Registry tests
stay CI-only (CIV_TEST_REGISTRY unset locally).

## Risky files / rollback points

- `internal/cmd/globals.go` + `sync.go`: two factory seams — MUST both apply the
  proxy via the shared `clientOptions` (drift = sync bypasses proxy). Review gate.
- `internal/upgrade/upgrade.go`: unifying fetchLatestRelease onto downloadClient
  changes the GitHub API client; verify the Accept/User-Agent headers preserved.
- `internal/api/client.go`: WithProxy vs WithHTTPClient/WithTimeout order —
  document; tests assert production order (NewClient default client + WithProxy).
- `config.Save` on unset must preserve Token (load-modify-save, not Delete).
- Rollback: feature is additive; revert the commit(s). No migration. A bad
  persisted proxy degrades to env/direct (never hard-fails).

## Follow-up checks before task.py start

- [ ] prd.md converged (no blocking open questions; D1–D8 resolved).
- [ ] design.md + implement.md present (complex task).
- [ ] implement.jsonl + check.jsonl curated with real spec/research entries
  (Qoder is a sub-agent-dispatch platform; empty/seed-only manifests not ready).
- [ ] Final planning summary presented AND explicitly approved by the user in a
  subsequent message (task-creation consent ≠ implementation approval).
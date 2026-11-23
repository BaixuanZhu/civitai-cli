# Implement: upgrade direct (child 11-23-upgrade-direct)

> **Wave scope override (parent prd constraint):** touch ONLY `internal/upgrade/*.go` and its tests. Do NOT touch `main.go`, `scripts/`, `CHANGELOG.md`, `README.md`, `internal/cmd/*`, or `internal/config/*` — the parent integration pass owns shared files; config's `GithubProxy`/`ResolveGithubProxy` is delivered by the sibling 11-23-github-proxy-slot (landed before this child starts). Do NOT git commit. Other children's changes may be dirty in the tree — ignore errors in files you don't own.

Execution order and validation for this child. Validation commands assume repo root, Go toolchain per go.mod.

## Checklist

1. **internal/upgrade/upgrade.go**
   - Delete `applyPersistedProxy` and its `Run`-time call; remove the now-stale doc paragraph on `downloadClient` that mentions persisted-proxy installation (replace with flag/env semantics per design D3).
   - Change `Run()` → `Run(args []string)`: build a local `flag.FlagSet` (name "upgrade", `flag.ContinueOnError`, `fs.Usage = func() {}`, output io.Discard), register `--proxy`; on parse error print a one-line usage hint to stderr and exit 1 (before any network).
   - Apply the proxy per design D3: `resolved := config.ResolveGithubProxy(flagValue)`; empty → ensure `downloadClient.Transport == nil`; non-empty → `proxy.Validate`: invalid value degrades with a `[WARN]` on stderr and keeps the nil transport (never fails the upgrade), valid → install `proxy.NewTransport` result. NOTE: an invalid explicit `--proxy` FLAG is still a usage error (exit 1 before network) — only the saved slot degrades.
2. **upgrade.Help()**: add `Options:` section with `--proxy <url>`; rewrite the `Proxy:` paragraph (direct default; env honored; saved proxy not consulted). Keep title/Usage/What-it-does/Exit codes/Output sections; page still ends with newline.
3. **main.go**: `upgrade.Run()` → `upgrade.Run(os.Args[2:])`; keep `isUpgradeHelpArg` interception before it. Update root usage: the `Proxy:` paragraph and `upgrade` line no longer claim the saved proxy applies to upgrade.
4. **Tests (internal/upgrade/upgrade_test.go)**
   - Remove the three `TestApplyPersistedProxy_*`; add: `TestRun_CivitaiSlotIgnored` (civitai slot saved, no flag → transport nil — reuse `setConfigDir`, `isolateDownloadClient`, `proxyURLOf`), `TestRun_GithubSlotConsumed` (slot saved, no flag → transport Proxy == slot URL), `TestRun_GithubSlotInvalidDegrades` (hand-edited bad slot → `[WARN]` + nil transport, no failure), `TestRun_ProxyFlagWinsOverSlot`, `TestRun_ProxyFlagInstallsTransport`, `TestRun_InvalidProxyFlagExitsBeforeNetwork`, `TestRun_BadArgsUsageError`. Use a Run seam (extract the pre-network portion into an unexported helper if calling Run directly would dial) so no test touches the network.
   - Update `TestHelp` ("the --proxy flag does not apply" → assert the new Options row and new Proxy paragraph wording), `TestHelp_SectionOrder` (Options now expected between Usage and Examples; still no embedded global-flag rows).
5. **Docs**: CHANGELOG `Unreleased` → Changed entry (upgrade ignores the saved proxy; `--proxy` added). README proxy section: same correction.
6. **scripts/integration-test.sh**: add `run upgrade_proxy_dead upgrade --proxy http://127.0.0.1:1` (exit 1 — connection refused proves the flag routes) placed BEFORE the real upgrade run; keep the real run last.

## Validation

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...` (all packages)
- [ ] Manual smoke: `go run . upgrade --proxy not-a-url` → exit 1, clear stderr, no network wait; `go run . upgrade help` → new page.

## Review gates

- Gate A (after step 2): help wording diff reviewed against prd R5 — no stale proxy claims anywhere (grep: `grep -rn "saved proxy" internal/upgrade README.md main.go`).
- Gate B (before commit): parent constraint check — zero new imports beyond stdlib + existing internal leaves; `git diff --stat` shows only expected files.

## Rollback point

Single commit on main; `git revert <sha>` restores old behavior. No config/data migration.

## Notes for the implementer

- Do NOT change civitai-slot semantics for data commands — but note `internal/cmd/proxy.go` itself is legitimately touched by the sibling 11-23-github-proxy-slot FIRST; this child only reads `Config.GithubProxy`. Verify the sibling has landed before starting here.
- This child lands BEFORE download-progress; expect its commit to move the download lines you also touch there — rebase order matters.

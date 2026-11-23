# Implement: github proxy slot (child 11-23-github-proxy-slot)

> **Wave scope override (parent prd constraint):** touch ONLY `internal/config/config.go`, `internal/config/config_test.go`, `internal/cmd/proxy.go`, `internal/cmd/proxy_test.go`. Do NOT touch `main.go`, `scripts/`, `CHANGELOG.md`, `README.md`, or `internal/upgrade/*` — the parent integration pass owns shared files; upgrade wiring is 11-23-upgrade-direct's contract. Do NOT git commit.

Execution order and validation. Land BEFORE 11-23-upgrade-direct (which consumes the slot).

## Checklist

1. **internal/config/config.go**
   - Add `GithubProxy string \`json:"githubProxy,omitempty"\`` to `Config` with GoDoc mirroring the `Proxy` field comment.
   - Add `ResolveGithubProxy(flagProxy string) string` mirroring `ResolveProxy` (flag wins after trim, else saved slot).
2. **internal/cmd/proxy.go**
   - Add a proxyOptions struct + register func binding `--github` (bool); thread it through `runProxy` (signature gains the options, or reads from a context — follow the existing options-struct pattern used by other commands).
   - `runProxySet`: on `--github`, validate then set `cfg.GithubProxy` (leave `cfg.Proxy` untouched); else current path. Human line: `GitHub proxy set to <url>.` vs `Proxy set to <url>.` (distinguishes scope); JSON result gains `"scope":"github"` when set.
   - `runProxyUnset`: bare → clear both slots (message says both cleared); `--github` → clear only github. Idempotent both ways; JSON scope field as above.
   - `reportProxyStatus`: keep the three civitai lines byte-identical; append github lines per design D4 with per-scope source detection (reuse `proxySource` against the slot value). JSON: nested `github` object per D5.
   - `proxyHelp`: Usage/Notes updated for `--github`, two scopes, per-scope precedence (D6).
3. **main.go usage()** — `Proxy:` paragraph: mention the github slot in one clause.
4. **Tests**
   - `internal/config/config_test.go`: both-slots round-trip; old-file (no githubProxy) tolerance; `ResolveGithubProxy` precedence.
   - `internal/cmd/proxy_test.go`: set/unset slot isolation + idempotency (table); status text incl. github block with slot set/unset and env present/absent; JSON additive shapes; `set --github not-a-url` exit 1 with config untouched.
5. **scripts/integration-test.sh** — after the existing `proxy_set` snapshot: `run proxy_set_gh proxy set --github http://127.0.0.1:7891` + `snap_cfg proxy_after_setgh`; assert config has `proxy` AND `githubProxy`; add `run proxy_unset_gh proxy unset --github` + snapshot asserting `githubProxy` cleared and `proxy` kept; extend the `proxy get` text check with the github line when the slot is set (keep civitai assertions verbatim). Restore end state: the existing `rm -f "$CFG"` cleanup already resets; ensure the gh cases sit BEFORE that cleanup and leave no residue for login checks.
6. **Docs** — CHANGELOG Unreleased → Added; README proxy section.

## Validation

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] Manual smoke: `civ proxy set --github socks5h://127.0.0.1:1080` → status shows both scopes; `civ proxy unset --github` → only github cleared; `civ proxy unset` → both cleared, token intact.

## Review gates

- Gate A (after step 2): human status diff — civitai three lines byte-identical (diff old vs new output with an empty github slot: only appended lines allowed).
- Gate B (before commit): JSON snapshots old-vs-new — only additive keys; `grep -rn "githubProxy" internal/ scripts/` shows the full footprint; no `internal/upgrade` changes in this commit.

## Rollback point

Single commit; older builds tolerate the unknown config key, so revert is safe both ways.

## Notes for the implementer

- Do not touch `internal/upgrade` — wiring the slot into upgrade is 11-23-upgrade-direct's contract.
- Keep `--github` off `get`/`status` (they always show both scopes).

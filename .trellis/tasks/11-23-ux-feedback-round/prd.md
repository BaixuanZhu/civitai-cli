# UX feedback round: upgrade direct, download progress, help routing, shell completion, skill help trim

## Source

User feedback on v0.3.0 (2026-11-23), five items:

1. `civ upgrade` routes through the persisted proxy and easily triggers GitHub 403 — upgrade should not use that proxy.
2. Downloads show no speed or progress — poor perception.
3. Many commands need help but seemingly lack it.
4. No shell auto-completion.
5. `civ skill help` prints a `Notes:` section that is redundant and should be deleted.

## Requirement set (owned here, decomposed to children)

- **R1** — `civ upgrade` must not apply the persisted civitai proxy to its GitHub traffic by default; it connects directly unless the user opts into a relay. The opt-in has two forms: a per-invocation `--proxy` flag and a persisted GitHub-only slot (`civ proxy set --github <url>`), so GitHub acceleration (e.g. a local relay port) can be configured independently of civitai. → children `11-23-github-proxy-slot`, `11-23-upgrade-direct`
- **R2** — The upgrade asset download (the only network download in the tool) must show live progress (percent, sizes, speed) on an interactive terminal, and stay quiet in scripts. → child `11-23-download-progress`
- **R3** — `civ help <command>` must reach every command's curated help page. (Item 3 is largely already delivered by v0.3.0's per-command pages; the residual gap is the `civ help <cmd>` entry point plus making the overview point at it.) → child `11-23-help-routing`
- **R4** — The tool must ship static shell completion for bash, zsh, fish, and PowerShell via a new `civ completion <shell>` subcommand. → child `11-23-shell-completion`
- **R5** — The `civ skill help` page must drop its `Notes:` section. → child `11-23-skill-help-trim`

## Task map

| Child | Deliverable | Verifiable by |
|---|---|---|
| 11-23-skill-help-trim | skill help page without Notes | existing page-structure tests + integration help case |
| 11-23-help-routing | `civ help <cmd>` routes to curated pages | unit test + integration cases |
| 11-23-github-proxy-slot | config + `civ proxy` gain a GitHub-only proxy slot (`set --github`, additive status/JSON) | unit tests + integration snapshot cases |
| 11-23-upgrade-direct | upgrade ignores civitai slot; `--proxy` flag; consumes the github slot (flag > github slot > env > direct) | unit tests + integration dead-proxy case |
| 11-23-download-progress | TTY progress bar for the upgrade zip fetch | pure formatter table tests + non-TTY silence test |
| 11-23-shell-completion | new `civ completion` subcommand, 4 shells | unit tests (script presence, drift guard) + integration cases |

Ordering: `github-proxy-slot` lands before `upgrade-direct` (upgrade consumes the slot); `download-progress` lands after `upgrade-direct` (both touch `internal/upgrade/Run`); the rest are independent. Note: URL-rewriting mirror sites (ghproxy-style) are explicitly out of scope for R1 — the slots hold http/https/socks5/socks5h proxy URLs only.

## Cross-child acceptance criteria (parent integration review)

- [ ] **AC1** Build and unit tests green on the integration branch (`go build ./...`, `go test ./...`, `go vet ./...`).
- [ ] **AC2** `scripts/integration-test.sh` gains cases for every new/changed surface in the same change (repo spec: quality-guidelines "Integration"): `help_<cmd>_named` routing cases, `completion_*` cases, the `upgrade --proxy` dead-port case; existing cases keep passing.
- [ ] **AC3** User-facing docs consistent: `README.md` documents `civ completion` install one-liners, the new upgrade proxy behavior, and `civ help <command>`; root usage text lists the `completion` command.
- [ ] **AC4** `CHANGELOG.md` gains an `Unreleased` section (Keep-a-Changelog; the `## [x.y.z]` heading contract untouched) covering: Added (GitHub proxy slot, download progress, `civ help <cmd>` routing, `civ completion`) and Changed (upgrade no longer honors the civitai proxy, skill help trimmed).
- [ ] **AC5** All five user-feedback items verified end-to-end against the built binary (manual smoke list in the final review).
- [ ] **AC6** No new third-party dependency; layering rules (`main -> cmd -> leaves`, one-way) preserved; each child archived with `--skip-branch-validation` per the no-PR direct-to-main flow.

## Constraints

- Zero third-party dependencies (project contract); stdlib + existing internal leaves only.
- All-English output and docs; GoDoc on new exported symbols.
- stdout = results/progress, stderr = logs; exit-code contract (0/1/2/3/4) unchanged except where a child PRD explicitly maps a new usage error.
- `upgrade` stays outside the `cmd` runner (design D4 of task 11-22-command-help): `main` routes its help tokens; its flags are parsed inside `internal/upgrade`.
- **Shared-file ownership (wave execution):** children implement against DISJOINT core file sets in parallel and must NOT touch `main.go`, `scripts/`, `CHANGELOG.md`, `README.md`, or `docs/` — the parent integration pass owns all shared-file updates (dispatch/usage entries, integration-test.sh cases, CHANGELOG, README) in one final commit. Child PRD ACs that reference those files are verified at parent integration (AC2/AC3/AC4), not at child check time.

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Each child carries its own prd/design/implement; this parent owns the requirement set, task map, cross-child acceptance criteria, and the final integration review.

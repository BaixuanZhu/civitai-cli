# help: route civ help <command> to per-command pages

## Goal

civ help <command> (e.g. civ help search) prints that command's curated help page instead of the generic overview; bare civ help keeps the overview; unknown names keep the current unknown-command error path.

## Requirements

- **R1** — `civ help <name>` prints the named command's curated help page to stdout with exit 0, for all ten user-facing commands: search, show, vet, sync, tags, login, logout, proxy, skill (pages owned by `internal/cmd`) and upgrade (page owned by `internal/upgrade`, printed verbatim by main today).
- **R2** — Bare `civ help` (no further args) prints the generic overview, unchanged, stdout exit 0.
- **R3** — `civ help <unknown>` behaves like an unknown command: `unknown command: <x>` on stderr, the overview on stderr, exit 1 — exactly what bare `civ <unknown>` does today (consistency beats a bespoke error).
- **R4** — The nine `cmd` pages keep their single-source property: a new exported accessor in `internal/cmd` renders a page by name from one map that lives next to the page consts; no page text moves or duplicates.
- **R5** — Root usage text updated: the "Run ... for flags and examples" hint mentions both `civ <command> help` and `civ help <command>`; a one-line note may point readers at the new form.

## Acceptance Criteria

- [ ] **AC1** Unit test: the exported accessor returns rendered pages (with the appended global block) for all nine cmd commands and not-found for unknown names.
- [ ] **AC2** Unit/integration: `civ help search` output is byte-identical to `civ search help` output.
- [ ] **AC3** Integration: `run help_search_named help search`, `run help_upgrade_named help upgrade` (exit 0); `run help_unknown help nope` asserts exit 1 with the unknown-command line.
- [ ] **AC4** Bare `civ help` output unchanged (existing `help_cmd` integration case keeps passing).
- [ ] **AC5** `go build ./...`, `go vet ./...`, `go test ./...` green.

## Constraints

- `main` keeps routing; `cmd` exposes one small exported helper (GoDoc required); `upgrade.Help()` is reused as-is — no page text duplication.
- No change to the per-page contract from task 11-22-command-help (stdout, exit 0, curated layout).

## Notes

- Addresses feedback item 3 together with v0.3.0's existing per-command pages: users typing `civ help <cmd>` (a very common CLI habit) currently get only the overview.

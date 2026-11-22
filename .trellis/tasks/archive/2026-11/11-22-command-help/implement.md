# Implementation Plan: Per-command help pages

Preconditions: task status `in_progress`; branch `feat/command-help` created
and set via `task.py set-branch`. Design contracts D1–D8 are frozen; file
ownership per D6.

## Wave A0 — runner mechanism (serial, one agent or inline)

1. `internal/cmd/help.go`: `globalHelpBlock` const, `writeCommandHelp`,
   `isBareHelp` (D2, D3). GoDoc on all exported symbols per 注释规则.
2. `internal/cmd/globals.go`: add `help string` param to `execute` and
   `executeWithExitCode`; `fs.Usage = func() {}`; ErrHelp branch and
   bare-help branch print to stdout + exit 0 (D1). Update both doc comments.
3. `internal/cmd/search.go`: convert to `execute("search", searchHelp, ...)`
   and author `searchHelp` (reference page for the layout contract).
4. Gate: `go build ./... && go vet ./... && go test ./internal/cmd/` — the
   package must compile with only search converted; the other 8 call sites
   are updated in the SAME wave by passing `""` temporarily? **No** — to keep
   the tree always-green, A0 updates ALL 9 call sites with placeholder-free
   minimal pages? No: A0 changes the signature and mechanically passes each
   command's existing one-line purpose as the body (from main.go usage text);
   A1/B then replace those with curated pages. This keeps every commit green.

## Wave A1 ∥ B — curated pages (parallel, disjoint files)

- **Agent A1**: `search.go` (full page), `show.go`, `vet.go` (include exit-3
  note), `sync.go`, `tags.go`; `main.go` (overview pointer line D5, upgrade
  routing D4); `internal/upgrade/upgrade.go` (`Help()` + GoDoc).
- **Agent B**: `login.go` (page covers key precedence + history warning),
  `logout.go`, `proxy.go` (set/get/unset usage), `skill.go`
  (platform/scope/force + interactive note).
- Gate per agent: `go build ./... && go vet ./... && go test ./...` (offline).
- Unit tests for D7 land with each agent: A1 adds
  `TestHelpPages_SectionOrder` + `writeCommandHelp` rendering test + upgrade
  `Help()` test; B adds per-command constant content tests
  (`TestLoginHelp_ContainsKeyPrecedence` etc.).

## Wave C — check (serial, trellis-check agent)

1. `scripts/integration-test.sh`: new `help_pages` section per D7 (10
   commands × keyword+flag, stdout-only assertion, login anti-hang,
   `search help me` live query, `--nope` exit 1 without `Usage of search`,
   `civ help` pointer).
2. Full gate: `go vet ./...`, `go test ./...`, `bash scripts/integration-test.sh`.
3. Cross-cutting: layout uniformity across the 10 pages, GoDoc on new
   exported symbols, all-English, no stray stderr on help paths.

## Wave D — finish (inline)

1. README: replace/extend the help section with per-command help examples.
2. CHANGELOG: new `## [0.3.0] - Unreleased` → Added bullet.
3. Spec update (trellis-update-spec): design-principles "new subcommand"
   recipe gains the mandatory help page step; quality-guidelines integration
   coverage already covers it — cross-reference.
4. Commit, `task.py finish`, archive, merge to main.

## Rollback points

- After each wave: `git checkout -- .` (single-feature branch, no shared
  files with other work).
- The mechanism (A0) is independently revertable; curated pages are pure data.

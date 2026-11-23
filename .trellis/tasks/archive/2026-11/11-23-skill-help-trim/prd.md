# skill help: drop the redundant Notes section

> **Wave scope override (parent prd constraint):** this child touches ONLY `internal/cmd/skill.go`. Do NOT touch `main.go`, `scripts/`, `CHANGELOG.md`, `README.md`, or test files — no test asserts on the Notes content (verified), the parent integration pass owns shared files. Do NOT git commit.

## Goal

Remove the Notes block from the civ skill help page; keep Usage/Options/Examples and the shared global-flags section. The interactive-menu details that lived in Notes are already discoverable in the menus themselves and the CHANGELOG.

## Requirements

- **R1** — The `skillHelp` body in `internal/cmd/skill.go` no longer contains the `Notes:` section; the page keeps `Usage:`, `Options:`, `Examples:` (with a `--json` example), and the runner-appended shared global-flags block.
- **R2** — No other help page changes; the cross-page layout audit from task 11-22-command-help keeps passing for all ten pages.
- **R3** — CHANGELOG `Unreleased` entry notes the trim under Changed.

## Acceptance Criteria

- [ ] **AC1** `civ skill help` output contains no `Notes:` line; Usage/Options/Examples/global-block order preserved (`TestAllHelpPages_SectionOrder` green).
- [ ] **AC2** Existing help-related tests updated where they referenced the Notes content; no test asserts on Notes anymore.
- [ ] **AC3** Integration `help_skill_kw` / `help_skill_flag` cases keep passing.
- [ ] **AC4** `go build ./...`, `go vet ./...`, `go test ./...` green.

## Constraints

- Lightweight task: PRD-only; no design/implement artifacts required.
- User-facing text change only — no flag, exit-code, or behavior changes.

## Notes

- Directly implements feedback item 5 ("notes 是有些多余的，可以直接删掉").

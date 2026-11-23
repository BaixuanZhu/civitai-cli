# PRD: Interactive skill selection (arrow-key menus, drop Proceed confirm)

## Problem

`civ skill install` without `--platform` drives a plain-text prompt flow:
numbered platform multi-select, typed scope answer, then a final
`Proceed? [y/N]:` confirmation. Users expect an arrow-key menu (↑↓ move,
space toggle, Enter confirm); the extra confirmation after the choices have
just been made adds a step with no value.

## Goal

Replace the three interactive prompts of `civ skill` with arrow-key menus,
and remove the final `Proceed` confirmation, while keeping a text fallback
for non-TTY stdin and the project's stdlib-only / offline-testable contracts.

## Requirements

- **R1 — menus**: three interactions become arrow-key selectors:
  1. platform selection — **multi-select**: ↑/↓ move, Space toggles, `a`
    toggles all, Enter accepts; detected platforms pre-toggled (same
    preselect semantics as today);
  2. scope — **single-select** (user/project, default highlighted);
  3. install conflict decision — **single-select**
    (overwrite/skip/backup, default skip; matches today's conflictFunc).
- **R2 — no final confirm**: after selections, print the resolved target
  paths once (as a record, not a question) and execute immediately. The
  `Proceed? [y/N]:` step is removed in BOTH the menu UI and the text
  fallback.
- **R3 — cancel semantics**: Esc or `q` (or Ctrl+C as today) cancels with
  the existing `skill: canceled by user` error → exit 1; in the conflict
  prompt, cancel maps to the safe `skip` (unchanged).
- **R4 — TTY fallback**: when stdin is not a console (pipe, CI, injected
  reader in tests) or console mode setup fails, the existing text prompt
  flow runs unchanged (minus R2's confirmation). Non-interactive
  `--platform ... --scope ...` paths are untouched.
- **R5 — stdlib-only**: raw console access via hand-written `syscall`
  (kernel32 GetConsoleMode/SetConsoleMode), mirroring `internal/env`'s
  precedent — no `golang.org/x/term`, no third-party TUI library. New
  `internal/console` leaf package (zero internal imports).
- **R6 — offline tests**: key-sequence decoding and menu frame rendering
  are pure and table-driven tested offline; the syscall shell stays thin.
  Existing `strings.Reader`-driven skill tests keep passing via the
  fallback path.
- **R7 — integration coverage** (project spec mandate): the text fallback
  (piped stdin) and the no-Confirm behavior are asserted in
  `scripts/integration-test.sh` in the same change; menu UI itself is
  manual/CI-ungoverned (no TTY in CI).
- **R8 — docs**: README skill section + `civ skill help` page describe the
  menu UI and the fallback; CHANGELOG entry added.

## Acceptance criteria

- **AC1**: interactive `civ skill install` on a real terminal (manual
  verification, recorded in the task journal): platform menu navigates with
  ↑/↓, Space toggles, detected platforms pre-toggled, Enter accepts; scope
  and conflict menus are single-select; no `Proceed?` prompt appears.
- **AC2**: piped-stdin install (`printf '...\n' | civ skill install`)
  completes via the text fallback with one fewer prompt than before (no
  confirmation line), exit 0.
- **AC3**: Esc/`q` in a menu exits 1 with `skill: canceled by user`;
  conflict-prompt cancel still resolves to `skip`.
- **AC4**: `go vet ./...`, `go test ./...` offline green; console decode and
  render tests cover ↑/↓/Space/Enter/Esc/a and multi-frame redraw.
- **AC5**: integration script green including new fallback assertions.
- **AC6**: no third-party dependency added (`go.mod` still zero `require`);
  `internal/console` imports no `internal/` package.
- **AC7**: non-interactive `--platform`/`--scope`/`--force` behavior
  byte-identical to before (existing tests unchanged and green).

## Out of scope

- Mouse support, theming/colors, search-as-you-type filtering; applying the
  menu UI to commands other than `skill` (login's prompt is a secret input,
  not a list).

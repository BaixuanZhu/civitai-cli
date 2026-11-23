# Implement: Interactive skill selection (arrow-key menus)

Execution checklist for design D1–D7. Order matters: pure cores first (they
gate the API), then the syscall shell, then the consumer, docs, integration.

## Step 1 — internal/console pure cores (D1, D4)

- [x] `internal/console/keys.go`: `key` type (up/down/space/enter/esc/rune/
      pending/none) + `decodeKey(buf []byte) (key, int)`. Table: `\x1b[A/B/C/D`,
      `" "`, `"\r"`, `"\n"`, `"a"`, `"q"`, bare `"\x1b"`, `\x03` (Ctrl+C →
      cancel), split `\x1b[` → pending, unknown rune → none-consumed skip.
- [x] `internal/console/render.go`: `menuState{cursor, toggled[], done, canceled}`
      + `step(state, k, multi) state` (clamp cursor, toggle, toggle-all via
      `a`, accept, cancel; single-select ignores space; empty-enter in
      multi-select is a no-op — menu stays open, deviation from D5's
      "re-prompt once then error", noted below) + `renderFrame(state, title,
      rows, multi) string` (title, `❯`/space gutter, `[x]`/`[ ]` toggles for
      multi, footer hint, constant line count for in-place repaint).
- [x] `internal/console/keys_test.go`, `render_test.go`: offline table-driven
      tests (golden strings for frames; reducer matrix up/down/space/a/enter/
      esc at boundary positions).

## Step 2 — syscall shell + public API (D1, D3, D5)

- [x] `internal/console/mode_windows.go` (`//go:build windows`): kernel32 via
      `syscall.NewLazyDLL` (GetStdHandle/GetConsoleMode/SetConsoleMode),
      hand-declared mode constants, save/restore with defer, VT input on
      stdin + VT processing on stderr, `IsTerminal(fd)`. Mirrors
      `internal/env/path_windows.go` style (zero-dep contract, package doc).
- [x] `internal/console/mode_other.go` (`//go:build !windows`): `IsTerminal`
      via `os.Stat` ModeCharDevice heuristic; raw-mode setup returns
      `ErrNotSupported` → caller falls back (report/width_* precedent).
- [x] `internal/console/console.go`: `Selection{Indexes []int; Canceled bool}`,
      `SelectMulti(title, rows, preselect) (Selection, error)`,
      `SelectOne(title, rows, defaultIdx) (Selection, error)`. Thin loops:
      setup raw mode → render → read → decode → step → repaint → restore.
      Mode/setup failure returns a sentinel error, never panics (R4).
- [x] Gate: package imports no `internal/` package; `go vet ./internal/console`
      green on windows (and CI-side check for other GOOS via _other file).

## Step 3 — skill.go integration (D5, D2)

- [x] `runSkill` interactive branch: if `console.IsTerminal(os.Stdin.Fd())`,
      drive menus (wording/rows built in cmd: `platform  root  [detected]`
      labels, scope rows user/project, conflict rows overwrite/skip/backup
      with skip pre-cursored); any console error → fall back to the text
      prompter on the same stdin (R4).
- [x] Cancel (Esc/q/Ctrl+C) at platform/scope menu → existing
      `skill: canceled by user` → exit 1 (R3). Conflict-menu cancel → `skip`.
- [x] Print resolved target paths once after selection (record, not a
      question); execute immediately (R2).
- [x] Text fallback: identical flow minus the `Proceed? [y/N]` block; delete
      `skillIsYes`; conflict text prompt unchanged.
- [x] Empty menu selection accepted by console is impossible (reducer
      no-ops empty-enter), so cmd-side empty check stays only in the text
      path (`parseInteractivePlatforms`).
- [x] Update `internal/cmd/skill_test.go`: drop the `y\n` confirm lines from
      interactive inputs; replace `TestSkill_InteractiveCancel` with a
      menu-seam cancel test (inject the menu selection seam — fake driver
      returning Canceled → exit-1 error); keep EOF test.

## Step 4 — docs (R8)

- [x] `internal/cmd/skill.go` `skillHelp`: interactive-mode note now describes
      arrow-key menus on a terminal, text fallback otherwise, no confirmation
      step.
- [x] README skill section: same wording sync.
- [x] CHANGELOG unreleased entry.

## Step 5 — integration coverage (R7, spec mandate)

- [x] `scripts/integration-test.sh`: plant a detection marker
      (`$ISO_HOME/.claude`), then `printf '\nuser\n' | civ skill install`
      (piped stdin → text fallback): assert exit 0, claude installed, and
      stderr does NOT contain `Proceed?`; existing `--platform` cases
      unchanged.

## Step 6 — quality gate (AC4–AC7)

- [x] `go vet ./...` + `go test ./...` offline green.
- [x] `bash scripts/integration-test.sh` green.
- [x] `go.mod` still zero `require`; `internal/console` has no `internal/`
      imports.
- [ ] Manual menu smoke on a real terminal, recorded in the task journal
      (AC1): ↑/↓ move, space toggles, `a` toggles all, Enter accepts, no
      `Proceed?`, Esc exits 1.

## Validation commands

```bash
go vet ./...
go test ./...
bash scripts/integration-test.sh
go list -deps ./internal/console | grep civitai-cli/internal
```

## Review gates / rollback

- Gate after Step 2: console API review before wiring skill.go (seam check).
- Gate after Step 3: full test suite before docs/integration polish.
- Rollback: single `git revert` (D7); fallback path is the safety net.

## Deviations from design (decided at activation)

- D5 "empty platform Enter re-prompts once then errors": implemented as
  menu-stays-open no-op instead — closer to menu idioms, cancel path
  preserved, and the text-path empty rule is untouched.
- Design's `cmd/skill.go` path is `internal/cmd/skill.go` in the actual tree.

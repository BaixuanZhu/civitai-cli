# Design: Interactive skill selection (arrow-key menus)

Status: PLANNED (queued behind 11-22-command-help). Contracts below are the
current best understanding; re-verify against the code at activation time —
in particular whether 11-22-command-help changed `skillHelp` wording.

## D1 — internal/console leaf package

```
internal/console/
├── console.go      # public API: SelectMulti / SelectOne / IsTerminal
├── keys.go         # pure: decodeKey(buf) (key, n) from VT byte sequences
├── render.go       # pure: frame rendering (option rows, cursor, toggles)
├── mode_windows.go # thin syscall shell: raw mode save/set/restore (kernel32)
└── *_test.go       # offline table-driven tests for keys.go / render.go
```

Zero `internal/` imports (leaf, like `env`). `syscall` + `golang.org/x/sys`
rule: **hand-written syscall only** (GetStdHandle, GetConsoleMode,
SetConsoleMode), matching `internal/env/path_windows.go`.

## D2 — Public API (consumer-side stays in cmd)

```go
// Selection is the outcome of a menu: the accepted choice indexes and
// whether the user canceled.
type Selection struct{ Indexes []int; Canceled bool }

// SelectMulti runs an arrow-key multi-select menu on the terminal.
// title/rows are static labels; preselect lists initially toggled indexes;
// the footer documents the key bindings. Requires a console stdin+stderr;
// callers fall back to text prompts when IsTerminal is false.
func SelectMulti(title string, rows []string, preselect []int) (Selection, error)

// SelectOne runs an arrow-key single-select menu (Enter accepts, no toggle).
func SelectOne(title string, rows []string, defaultIdx int) (Selection, error)

// IsTerminal reports whether fd is an interactive console handle.
func IsTerminal(fd uintptr) bool
```

`cmd/skill.go` owns the wording and the fallback decision (seam at the
consumer, consistent with the project's factory-seam pattern); `console`
knows nothing about skills.

## D3 — Raw console mechanics (Windows, VT)

- stdin: `SetConsoleMode(h, old &^ (ENABLE_LINE_INPUT|ENABLE_ECHO_INPUT|ENABLE_PROCESSED_INPUT) | ENABLE_VIRTUAL_TERMINAL_INPUT)`
  → arrow keys arrive as `ESC[A` / `ESC[B`, Space as `0x20`, Enter as
  `0x0D`, Esc as `0x1B`, letters as plain bytes. Restore the saved mode on
  every return (defer).
- stderr: OR in `ENABLE_VIRTUAL_TERMINAL_PROCESSING` so ANSI redraw works on
  classic conhost (Windows Terminal has it by default); restore after.
- Redraw strategy: print the frame, then on each key event `ESC[<n>A` +
  `ESC[J`-style repaint of the rows in place (render.go computes the frame
  diff — keep it simple: full row-block repaint, options are ≤ 4 rows).
- If either SetConsoleMode fails → return a sentinel error; the caller uses
  the text fallback (R4). Never crash on weird terminals.

## D4 — Pure cores (testability)

- `decodeKey`: input-side state machine over a byte buffer returning
  (keyUp/keyDown/keySpace/keyEnter/keyEsc/keyRune('a'|'q'), consumed). Table
  tests feed `\x1b[A`, `\x1b[B`, `\x1b[C`, `\x1b[D`, `" "`, `"\r"`, `"\n"`,
  `"a"`, `"q"`, `"\x1b"`, split-buffer partial sequences.
- `renderFrame`: renders `title`, `❯ row` cursor marker, `[x]`/`[ ]`
  toggles, footer hint; pure string output, table-tested (golden strings).
- `step(state, key) state`: the reducer (cursor clamp, toggle set, all-toggle,
  accept, cancel) — table-driven; SelectMulti/SelectOne are thin loops over
  step + render + read.

## D5 — skill.go integration

- `skillPrompter` gains a `tty bool` (or a `menu` interface) so tests keep
  injecting `strings.Reader` (fallback). Flow when tty:
  1. `SelectMulti("Select platforms for civ skill install:", rows, detectedIdxs)`
     — rows carry `platform  root  [detected]` labels as today.
  2. `SelectOne("Scope:", [user project], defaultIdx)`.
  3. Print resolved target paths once (`fmt.Fprintln` block, no question).
  4. Conflict: `SelectOne("civ-prompt already exists at ... (version v):", [overwrite skip backup], skipIdx)`; cancel → `"skip"`.
- Esc/`q`/Ctrl+C at 1–2 → `skill: canceled by user` (exit 1, unchanged text).
- Empty platform selection on Enter (nothing toggled): re-prompt once, then
  error (same rule as an empty text answer today).
- Text fallback: identical to today minus the `Proceed?` block (R2).

## D6 — Tests

- Unit: console package (D4) + skill fallback flow via `strings.Reader`
  (existing pattern; now one prompt shorter).
- Integration (`scripts/integration-test.sh`): piped-stdin install exercises
  the fallback and asserts the confirmation prompt text is GONE from stderr
  and install still succeeds (exit 0); `--platform` non-interactive path
  unchanged. Menu UI itself is manual (AC1, journal note).

## D7 — Rollback

Pure UX change, no persisted state; revert the commit. Fallback path is the
safety net — worst case the feature degrades to today's text prompts.

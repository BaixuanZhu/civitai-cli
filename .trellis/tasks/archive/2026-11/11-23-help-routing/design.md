# Design: help routing (child 11-23-help-routing)

## Summary

Make `civ help <name>` a first-class entry point to the ten curated help pages. `internal/cmd` gains a tiny exported lookup over its nine page consts; `main` routes `help <name>` to it (or to `upgrade.Help()`), keeping bare `help` and unknown-name behavior exactly as today.

## Current state (verified in code)

- `main.go` `case "help", "-h", "--help": usage(os.Stdout)` — extra args are silently ignored (`civ help search` prints the overview).
- Nine pages are unexported consts in `internal/cmd` (`searchHelp`, ..., `skillHelp`), rendered via `writeCommandHelp(w, name, body)` which appends `globalHelpBlock`. `upgrade.Help()` is produced and printed verbatim by `main`.
- `TestAllHelpPages_SectionOrder` pins the layout of the nine runner pages; upgrade's page is pinned in `internal/upgrade/upgrade_test.go`. cmd cannot import upgrade (design D4), so the lookup covers nine names and main special-cases `upgrade`.

## Design decisions

- **D1 — Accessor shape.** `cmd.WriteHelpPage(w io.Writer, name string) bool` in `help.go`: looks up `name` in an unexported `map[string]string` (`helpPages`) populated once from the nine consts; on hit renders via `writeCommandHelp` and returns true; on miss returns false. One map beside the consts keeps single-source (R4); a test pins map keys to exactly the nine expected names so a future page can't be added without joining the map.
- **D2 — main routing.** `case "help", "-h", "--help":` gains a branch: `len(os.Args) >= 3` → `name := os.Args[2]`; `cmd.WriteHelpPage(os.Stdout, name)` hit → return; `name == "upgrade"` → `fmt.Print(upgrade.Help())`, return; else fall through to the existing unknown-command path (`unknown command: <x>` to stderr, `usage(os.Stderr)`, exit 1). More than one trailing arg (`civ help search extra`) → same unknown path using the second word? Decision: treat `civ help a b` as unknown `a b` is over-thinking; route on `os.Args[2]` only and ignore further args — no, silent ignoring caused this gap. Final: require exactly one arg after `help`; extra args → unknown-command error naming the full remainder. This matches how strict the tool is elsewhere (e.g. `login a b` is a usage error).
- **D3 — Flag forms.** `-h`/`--help` with a trailing word (`civ -h search`) keeps today's behavior (overview) — only the literal `help` keyword takes a target. Documented in usage text to avoid surprise.
- **D4 — Usage text.** Root overview: change `Run "civ <command> help" (or civ <command> -h) for flags and examples.` to also mention `civ help <command>`. One line; no new section.
- **D5 — No page text moves.** `upgrade.Help()` stays the verbatim contract; the nine bodies stay where they are. The only new rendering path is the map lookup.

## Contracts touched

- New exported symbol `cmd.WriteHelpPage` (GoDoc required). main's `help` case branch. Root usage text (user-visible). No exit-code changes: hit → 0, bare → 0, unknown → 1 (same as today's unknown command).

## Test plan

- Unit: `WriteHelpPage` hits all nine names with correct title line and appended global block; misses return false and write nothing.
- Unit/integration: byte-equality of `help search` vs `search help` (integration `help_search_named`).
- Existing tests keep passing untouched (per-page layout audit, `help_cmd`).

## Rollback

Revert one commit in `main.go` + `cmd/help.go`; zero data impact.

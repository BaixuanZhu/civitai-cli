# Design: Per-command help pages

## D1 — Mechanism lives in the shared runner (internal/cmd)

`execute` / `executeWithExitCode` gain one `help string` parameter (the full
command page *without* the global block). Three changes inside
`executeWithExitCode`:

```go
fs := flag.NewFlagSet(name, flag.ContinueOnError)
fs.SetOutput(os.Stderr)
fs.Usage = func() {}   // suppress the Go default dump (also on unknown flags)
...
if err := fs.Parse(permuteArgs(fs, args)); err != nil {
    if errors.Is(err, flag.ErrHelp) {          // -h / -help / --help
        writeCommandHelp(os.Stdout, name, help)
        os.Exit(0)
    }
    app.Fail(err)
}
if isBareHelp(fs.Args()) {                     // single positional "help"
    writeCommandHelp(os.Stdout, name, help)
    os.Exit(0)
}
```

Why this shape:

- `fs.Usage = func() {}` kills the default dump in *all* paths (help AND
  unknown-flag errors). Parse error messages themselves are untouched — the
  one-line `flag provided but not defined: -nope` still reaches stderr via the
  flag package, then `app.Fail` prints its `civ:` line; net effect: unknown
  flag output gets *cleaner*, exit 1 preserved (prd AC5).
- The flag package returns `flag.ErrHelp` only for `-h`/`-help`/`--help`
  before any other parse error, so `civ search --limit x -h` fails on
  `--limit` first (standard CLI behavior, matches kubectl/docker).
- Bare-keyword check runs AFTER a successful parse and BEFORE `run`, so no
  prompt, network call, or config access can happen (prd R7).

## D2 — Bare-keyword predicate

```go
// isBareHelp reports whether pos is exactly one "help" positional.
func isBareHelp(pos []string) bool { return len(pos) == 1 && pos[0] == "help" }
```

`civ search help me` → `pos = ["help","me"]` → not help → real query (prd R6).
`civ login help` → intercepted before the stdin prompt. `civ proxy help` /
`civ skill help` → intercepted before action dispatch. Flag values are never
positional, so `civ login --key help` is unaffected.

## D3 — Page layout and the shared global block

`internal/cmd/help.go` (new):

```go
// globalHelpBlock is the single source for the global flags section.
const globalHelpBlock = `Global flags (accepted after the command name):
  --json           Machine-readable output (human decoration off)
  --token <key>    API token; overrides CIVITAI_API_KEY and the saved config
  --proxy <url>    Outbound proxy for this invocation; overrides saved + env
  --timeout <dur>  Per-request HTTP timeout (default 30s)
  --verbose        Verbose [INFO]/[DEBUG] logging to stderr
`

// writeCommandHelp prints the page: title/body from the command constant,
// then the shared global block.
func writeCommandHelp(w io.Writer, name, body string) { ... }
```

Per-command constant, e.g. in `search.go`:

```go
// searchHelp is the curated `civ search help` page (without the global block).
const searchHelp = `search Civitai models

Usage:
  civ search <query> [flags]

Options:
  --type <list>    Model type filter; repeatable or comma-separated
                   (lora, checkpoint, ti, locon, ...)
  --base <list>    BaseModel filter, comma-separated; unregistered values warn
  --sort <order>   hot, highest-rated, most-downloaded, most-liked,
                   most-discussed, most-collected, most-images, newest, oldest
  --period <p>     Ranking period: day, week, month, year, all
  --tag <name>     Exact tag filter
  --user <name>    Creator username filter
  --limit <n>      Max results, 1-100 (default 20)

Examples:
  civ search stocking --type lora --limit 5
  civ search "masterpiece" --sort newest --base sdxl --json
`
```

Section contract (prd R4/AC8): `Usage:` → `Options:` (omit when the command
has no own flags, e.g. `logout`) → `Examples:` → appended `Global flags`.
Title line is `<one-line purpose>`; `writeCommandHelp` prints
`civ <name> - <title>` — i.e. constants start with the bare purpose text.
Options rows align descriptions at a fixed column; legal values live in the
description, defaults in `(default …)`.

## D4 — upgrade (flagless, outside internal/cmd)

`internal/upgrade` exposes:

```go
// Help returns the curated `civ upgrade help` page (no flags of its own).
func Help() string
```

`main.go` routes: `case "upgrade":` → if `os.Args[2:]` is a bare `help`
positional or an `-h`/`-help`/`--help` flag token, `fmt.Print(upgrade.Help())`
and return (no exit — falls off main normally). The routing predicate is a
tiny helper in `main.go` (`isUpgradeHelpArg`), NOT exported from `cmd`, to
keep `cmd` unaware of `upgrade`.

## D5 — main.go overview

`usage()` gains one line after the Commands block:

```
Run "civ <command> help" (or civ <command> -h) for flags and examples.
```

## D6 — Frozen file ownership for parallel implementation

| Owner | Files |
|---|---|
| Agent A | `internal/cmd/help.go` (new), `internal/cmd/globals.go`, `internal/cmd/{search,show,vet,sync,tags}.go`, `main.go`, `internal/upgrade/upgrade.go` |
| Agent B | `internal/cmd/{login,logout,proxy,skill}.go` |

Agent B consumes the frozen contract: `execute(name, help, args, register, run)`
(A lands the signature change first — Wave A0 serial, then A1/B parallel).

## D7 — Tests

- Unit (offline, table-driven, in `internal/cmd`):
  `TestIsBareHelp`, `TestWriteCommandHelp_ContainsSections` (per command
  constant: `Usage:`, a signature flag row, `Examples:`, and that the rendered
  page ends with the shared `--proxy` row — proving the global block append),
  `TestHelpPages_SectionOrder` (index(Usage) < index(Options) < index(Examples)
  < index(Global flags) across all constants). `upgrade.Help()` content test
  in `internal/upgrade`.
- Integration (`scripts/integration-test.sh`, new `help_pages` section): for
  each of the 10 commands assert `help` keyword and `-h` flag → exit 0, output
  on **stdout** (stderr empty), section markers present; `civ login help`
  with stdin from `/dev/null` (anti-hang, exits immediately); `civ search
  help me` still returns results (live query); `civ search --nope` exit 1 and
  stderr does NOT contain `Usage of search` (default dump gone — AC5);
  `civ help` contains the pointer line (AC6).

## D8 — Compatibility / rollback

No persisted state, no network protocol, no flag semantics change; the only
observable behavior change is the help surface (stdout/exit 0) and the removal
of the stderr default dump on `-h` and unknown flags. Rollback = revert the
commit. `--json` + `-h` together: help wins, plain text (help is not data).

# Per-command help pages (`civ <cmd> help` and `-h`/`--help`)

## Problem

`civ` has no usable per-command help. `civ search -h` prints the Go FlagSet
default dump: it goes to **stderr**, lists flags alphabetically with no usage
synopsis, no examples, and no separation of command flags from global flags.
`civ search help` does not exist — the word `help` is parsed as a search query
and hits the network. A user (or agent) holding the binary cannot discover what
options a complex command like `search` offers without reading the README.

## Goal

Every subcommand ships a curated, built-in help page with two equivalent entry
points, printed to stdout with exit 0.

## Requirements

- **R1 — coverage**: all subcommands get a help page: `search`, `show`, `vet`,
  `sync`, `tags`, `login`, `logout`, `proxy`, `skill`, `upgrade`.
- **R2 — dual entry**: `civ <cmd> help` (bare positional keyword) and
  `civ <cmd> -h` / `--help` / `-help` (flag form) print the same page.
- **R3 — stream/exit contract**: help goes to **stdout** with exit **0**
  (today: stderr dump). Errors keep their existing streams/codes.
- **R4 — page content**: one-line purpose, `Usage:` with positional arguments,
  command-specific flags with legal values and defaults, at least two
  `Examples:`, then a shared `Global flags` section. The global block is
  rendered from ONE shared constant (never copy-pasted per command, so adding
  a global flag later cannot drift the pages).
- **R5 — discoverability**: the `civ help` overview gains a line pointing to
  per-command help.
- **R6 — literal-word escape**: only a *bare single* `help` positional triggers
  the page. `civ search help me` still searches the query "help me"; flag
  values (`civ login --key help`) are unaffected.
- **R7 — no hangs**: `civ login help` with empty stdin exits immediately
  (interception happens before any prompt or network call).
- **R8 — integration coverage** (project spec mandate): the per-command help
  surface is asserted in `scripts/integration-test.sh` in this same change.
- **R9 — docs**: README and CHANGELOG updated.

## Acceptance criteria

- **AC1**: `civ search help`, `civ search -h`, and `civ search --help` each
  exit 0, print to stdout, and contain `Usage:`, the `--sort` row with its
  legal values, `Examples:`, and the global `--proxy` row.
- **AC2**: every command listed in R1 passes the same exit-0/stdout/section
  assertions (signature flag per command asserted in the integration script).
- **AC3**: `civ login help < /dev/null` returns exit 0 without hanging.
- **AC4**: `civ search help me` performs a real search (query "help me"),
  same output shape as any search.
- **AC5**: an unknown flag (`civ search --nope`) still exits 1 with a one-line
  error; the Go default flag dump no longer appears.
- **AC6**: `civ help` output contains a "Run `civ <command> help`" pointer.
- **AC7**: `go vet ./...` and `go test ./...` pass offline; the integration
  script passes including the new help assertions.
- **AC8**: section order is uniform across pages: Usage → Options → Examples →
  Global flags (spot-asserted programmatically).

## Out of scope

- `man` pages, `--help` for the internal `unbootstrap` command, shell
  completion, and any behavioral change to flag parsing itself.

# Design Principles

> The design contract behind the layout: high cohesion, low coupling,
> extensibility, maintainability — expressed the Go way (composition and
> function values, no class hierarchies). File placement rules live in
> [Directory Structure](./directory-structure.md); this file explains the
> *principles* every change must preserve.

---

## The Four Principles

| # | Principle | In this codebase |
|---|---|---|
| P1 | **High cohesion** — one package, one job | `api` owns *all* outbound HTTP; `report` owns *all* rendering; `cmd` owns *all* flag/exit-code plumbing |
| P2 | **Low coupling** — one-way dependencies, replaceable seams | `cmd -> api/resolver/compare/report/hashcache/config`; leaves (`app`, `env`, `skill`) import nothing sideways; tests swap seam vars |
| P3 | **Extensible by recipe** — growth follows a fixed checklist | New subcommand / endpoint / option / format / machine schema each have a step list (below), so features never renegotiate the architecture |
| P4 | **Maintainable by construction** — pure cores, stable contracts | `run*` functions return errors instead of exiting; exit codes 0–5 (5 is resolve's completed-not-ready verdict) and stdout-only-results are public contracts |

Parent design contract: `.trellis/tasks/archive/2026-11/11-22-civitai-cli-go/design.md` (§1, D1–D9).

---

## P1 — High Cohesion

Every package has a single sentence of responsibility, and everything that
sentence covers lives inside it:

| Package | Owns | Must NOT contain |
|---|---|---|
| `internal/api` | outbound HTTP, retry, auth, error classification, response decode | any printing, flag parsing, exit codes |
| `internal/report` | tables, JSON rendering, formatting vocabulary (k/M, dates, widths) | API knowledge, logging, HTTP |
| `internal/cmd` | flag sets, orchestration, exit-code mapping, UX | direct `net/http`, formatting arithmetic |
| `internal/resolver` | pure readiness policy, reference validation, evidence correlation, deterministic report | HTTP, flags, printing, filesystem/env, clock, exit |
| `internal/compare` | pure comparison registries, tri-state equality, derived differences | HTTP, flags, printing, filesystem, clock, randomness, exit |
| `internal/hashcache` | streaming SHA256 + size/mtime cache persistence | API calls, CLI flags |
| `internal/config` | token persistence + priority resolution | anything token-unrelated |
| `internal/app` | Version, Fail, UserAgent, leveled logging, Bootstrap gate | any `internal/` import (it is the leaf everyone may depend on) |

The sharpest cohesion tool is the **choke point**: `Client.Get`
(`internal/api/client.go`) is the only place that builds requests, applies the
User-Agent/token, retries, gates concurrency, and enforces the body limit.
Every endpoint method (`models.go`, `versions.go`, `tags.go`) funnels through
it, so cross-cutting behavior changes in exactly one file.

Command files mirror this: `internal/cmd/vet.go` is flag parsing + a pure
`runVet` + the vet-specific exit override — nothing else.

---

## P2 — Low Coupling

### One-way dependency rule

```
main.go -> cmd -> {api, report, hashcache, config, skill} -> app (leaf)
```

An import that points *upward* or *sideways* (e.g. `api` importing `report`)
is wrong regardless of how convenient it looks. `internal/app` imports nothing
from `internal/`, which is what makes it dependency-free for everyone.

### Object orientation, Go style

Go has no inheritance; this project gets the same benefits from four idioms,
and new code should prefer them in this order:

1. **Encapsulated struct + methods** — behavior lives on the type that owns
   the data: `Client.Get`, `Cache.Get/Put/Save`. Fields stay unexported;
   callers configure through the constructor.
2. **Constructor + functional options** — `NewClient(opts ...Option)` with
   `WithBaseURL/WithToken/WithTimeout/WithConcurrency/WithBackoffs`
   (`api/client.go`). Options extend construction without breaking callers or
   growing parameter lists; see the recipe below.
3. **Function values as the polymorphism seam** — `Option`, `exitMap`,
   `hint`, `ConflictFunc` (skill install), and the `cmd` factory vars. This
   is how tests substitute behavior without interfaces.
4. **Small stdlib interfaces at the consumer** — `io.Writer` for output
   (`globals.out`). Declare a new interface only when a *real* second
   implementation exists; this codebase deliberately uses a replaceable
   concrete factory (`var newAPIClient = func(...)`) instead of a
   `ClientInterface`, because the only variation is construction, not behavior.

### The sanctioned seams (complete list)

| Seam | File | Purpose |
|---|---|---|
| `var newAPIClient` | `cmd/globals.go` | tests point commands at `httptest` |
| `var newSyncClient`, `var computeHash` | `cmd/sync.go` | same, plus hash-call counting |
| `globals.out io.Writer` | `cmd/globals.go` | capture stdout in tests |
| `executeWithExitCode(exitMap, hint)` | `cmd/globals.go` | per-command exit semantics (vet's 3) |
| `WithHTTPClient/WithBaseURL` | `api/client.go` | transport/base injection for tests |
| nil `*hashcache.Cache` | `hashcache/store.go` | disabled cache: Get misses, Put/Save no-ops — a nil object pattern; the cmd layer passes nil for `CIV_NO_CACHE` |
| `ConflictFunc` | `skill/install.go` | caller-supplied conflict resolution callback |
| `var skillMenuSelectMulti` / `skillMenuSelectOne` / `skillStdinIsConsole` | `cmd/skill.go` | tests fake arrow-key menus or force the text-fallback path |

These package-level vars are the *only* sanctioned mutable package state.
Do not add another global; add (or extend) a seam, injection point, or option
instead.

### Interactive terminal UI (menus)

**What**: TTY-only prompts (arrow-key menus) live in `internal/console` — a
leaf package with zero `internal/` imports and hand-written kernel32
syscalls, mirroring `internal/env`. Wording, option rows, and the
menu-vs-fallback decision stay in the consuming command (consumer-side
seam); `console` knows nothing about the domain. `civ skill` (2026-11-23)
is the template.

**Contract** (`internal/console/console.go`):

```go
type Selection struct{ Indexes []int; Canceled bool }
func SelectMulti(title string, rows []string, preselect []int) (Selection, error)
func SelectOne(title string, rows []string, defaultIdx int) (Selection, error)
func IsTerminal(fd uintptr) bool
```

- Raw modes: stdin minus line/echo/processed input plus VT input, stderr
  plus VT processing; saved modes restored on every return path (`defer`).
  A mode/setup failure returns an error (never panics) and the caller falls
  back to text prompts on the same stdin.
- Key decoding and frame rendering are pure (`keys.go`, `render.go`) and
  table-tested offline; the syscall shell and read loop stay thin. Menu UI
  itself is manual-smoke territory (no TTY in CI).
- Cancel/error matrix: Esc/q/Ctrl+C at a choice menu → the command's cancel
  error (`skill: canceled by user`, exit 1); at the install-conflict menu →
  the safe default (`skip`); a read error mid-menu clears the drawn frame
  first so fallback prompts never print below a stale menu skeleton.

**Wrong**:
```go
if console.IsTerminal(os.Stdin.Fd()) { // injected *strings.Reader tests
    runMenus()                          // hang on raw reads when go test
}                                       // runs inside a real console
```

**Correct**:
```go
// Type-assert *os.File first: an injected reader is never a console, even
// when the test process itself has one (skillStdinIsConsole in skill.go).
if f, ok := stdin.(*os.File); ok && console.IsTerminal(f.Fd()) {
    runMenus()
}
```

---

## P3 — Extensibility Recipes

Growth never renegotiates the architecture; it follows these checklists.

### Add a subcommand

1. `main.go`: one `case` in the dispatch switch + one line in `usage()`.
   Nothing else — main stays logic-free.
2. `internal/cmd/<name>.go`: a curated `<name>Help` const (see
   [Command help pages](#command-help-pages) below); a `register` function
   binding the command's flags; a pure
   `run<Name>(ctx context.Context, g *globals, pos []string) error`
   that does the work; wire via `execute` (default exit codes) or
   `executeWithExitCode` with an `exitMap`/`hint` if semantics differ
   (pattern: `vetExitCode`/`vetErrorHint` in `vet.go`).
3. Fetch through `newAPIClient(g)`; render through `internal/report`; write
   results to `g.stdout()`.
4. Tests: replace the seam vars, serve `httptest` fixtures, assert output
   *and* exit codes (`cmd/vet_test.go`, `cmd/login_test.go` are templates);
   add the help-page assertions to `scripts/integration-test.sh`.

### Add or change a machine-output schema

1. New machine output that normalizes upstream data carries a versioned
   envelope: `schemaVersion` + `kind` (`civ.search/v1`, `civ.show/v1`,
   `civ.recipe/v1`, compare `schemaVersion: 1`) so a later field change is a
   deliberate schema event, not an accident.
2. When changing an existing default `--json` shape, keep the legacy shape
   reachable through an explicit `--view full` (search/show pattern) and pin
   both shapes in tests before wiring the default. An explicit view flag that
   does nothing, or a "full" path that is compact-plus-extra-fields, is a
   contract bug.
3. Respect the list/detail invariant: list and summary views never carry large
   free-form fields (prompt/negativePrompt, description HTML, raw metadata,
   ComfyUI/workflow payloads); only an explicitly selected detail result or a
   documented legacy detail view may. Enforce at the DTO level, not in the
   renderer.
4. Put policy/evaluation in a pure package (`internal/resolver`,
   `internal/compare`) with function-value seams; `cmd` owns flags, HTTP
   adapters, rendering, and exit mapping. Do not re-decode or reinterpret the
   same field independently in a renderer.

### Command help pages

**What**: every subcommand (including the flagless `upgrade`, via
`upgrade.Help()` routed by `main.go`) ships a curated help page reached two
equivalent ways: `civ <cmd> help` (single bare positional) and
`civ <cmd> -h/-help/--help` (flag package returns `flag.ErrHelp`). Both
print to **stdout** with **exit 0** — never stderr — before any run work
(no prompt, network, or config access).

**Contract** (`cmd/help.go`):

- `execute`/`executeWithExitCode` take the page as their `help string`
  parameter; the runner owns interception (`fs.Usage = func(){}` suppresses
  the Go default dump everywhere, including unknown-flag errors, which stay
  exit 1 with a one-line message).
- Page constant layout, in order: one-line purpose (first line),
  `Usage:` (positional args shown), `Options:` (own flags only, legal
  values + defaults in the description; omit when the command has none),
  `Examples:` (2+, one showing `--json`), optional `Notes:`/`Exit codes:`.
  `writeCommandHelp` prepends `civ <name> - ` and appends the shared
  `globalHelpBlock`.
- **Never embed global-flag rows in a page constant** — `globalHelpBlock`
  is the single source; `TestAllHelpPages_SectionOrder` fails on duplicates.

**Wrong**:
```go
const myHelp = `do X

Options:
  --json  ...   // global flag copied into the page — drifts on the next global change
`
```

**Correct**:
```go
const myHelp = `do X

Usage:
  civ mycmd <arg> [flags]

Options:
  --flag <v>  what it does (values: a, b, c; default a)

Examples:
  civ mycmd foo --flag b --json
`
```

**Tests**: `TestAllHelpPages_SectionOrder` (rendered section order + no
duplicated global rows) per package; integration `help_pages` section
asserts dual entry, stdout-only, signature flags, and the `civ login help`
anti-hang (empty stdin).

### Add an API endpoint

1. Response struct in `api/types.go` — tolerant decode: unknown fields
   ignored, `json.Number` for drift-prone numerics, value structs for stats.
2. Method on `*Client` in `api/<thing>.go`, funneled through `Client.Get`;
   return the raw body plus a `Parse*` helper so `--json` can pass through
   untouched (pattern: `ModelsRawItems`).
3. CLI-facing enum mapping goes in `api/enums.go` as a pinned table with a
   "verified live" comment.
4. `testdata/*.json` verbatim fixture + an entry in `testdata/FIXTURES.md`;
   `httptest` test asserting URL shape, headers, and each error class.

### Add a client option

One `With<X>(...) Option` function in `api/client.go` with GoDoc stating the
default and interaction order (see `WithTimeout`'s note vs `WithHTTPClient`),
applied by the existing `NewClient` loop. No new constructor overloads.

### Add a rendering format or primitive

`report/<name>.go` pure functions; no `api` imports, no logging, output goes
to the writer the caller passes. Keep stdout = results only (see
[Logging](./logging-guidelines.md)).

---

## P4 — Maintainability

- **Pure command cores.** `run*` functions return `error`; only
  `execute*`/`app.Fail` call `os.Exit`. This single rule is what makes exit
  codes and rendering offline-testable.
- **Errors classify, wrap, degrade.** `%w` wrapping with a package prefix;
  `errors.As` for class; degradation warns and continues — never fatal (full
  tables in [Error Handling](./error-handling.md)).
- **Stable public contracts.** Exit codes 0/1/2/3/4, `--json` shapes, and
  stdout/stderr separation are asserted by `scripts/integration-test.sh`;
  treat changing them as a breaking release, not a refactor.
- **Concurrency documented on the type.** `Client` (semaphore) and `Cache`
  (mutex) state their concurrency contract in GoDoc; keep it true.
- **Constants centralized with their justification** (`api/client.go` const
  block); no magic numbers inline.
- **Zero third-party deps** keeps upgrades and supply-chain review trivial —
  `go.mod` gaining a `require` is a design-level decision, not a convenience.

---

## Go Conventions Quick Reference

- `context.Context` is the first parameter; every wait selects on
  `ctx.Done()`; requests use `http.NewRequestWithContext`.
- Package comments (`// Package xxx ...`) and `// Symbol ...` GoDoc on every
  exported symbol, explaining *why* (see [Quality](./quality-guidelines.md)).
- Files: lowercase, one concern; platform variants via suffix
  (`path_windows.go`, `width_other.go`); tests beside code as
  `TestXxx_Behavior`.
- All text English — code, comments, CLI output, logs, tests.
- No `init()`, no ORM-style reflection magic, no CLI framework: stdlib
  `flag` + a switch is the deliberate choice for a tool this size.

---

## Anti-Patterns

| Anti-pattern | Do instead |
|---|---|
| Declaring an interface "for flexibility" before a second implementation exists | Replaceable concrete factory var (`newAPIClient`) or function value; introduce an interface only at a real second consumer |
| `api` importing `report`/`cmd` (or any upward/sideways import) | Return data; let `cmd` render via `report` |
| HTTP calls outside `Client.Get` | Add/extend an endpoint method in `internal/api` |
| `os.Exit` inside `run*` or library code | Return the error; `execute*` maps and exits |
| A new package-level mutable global | Extend the sanctioned seams list above — or inject |
| Making `config.Load`/`hashcache.Load` fatal on corruption | Warn + degrade to zero value (existing contract) |
| Growing `NewClient` parameter lists | Add a `With<X>` option |
| Printing from `api`/`report`/`hashcache` | Return errors/data; presentation belongs to `cmd` + `report` |

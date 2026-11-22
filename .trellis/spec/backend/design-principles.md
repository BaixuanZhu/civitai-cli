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
| P2 | **Low coupling** — one-way dependencies, replaceable seams | `cmd -> api/report/hashcache/config`; leaves (`app`, `env`, `skill`) import nothing sideways; tests swap seam vars |
| P3 | **Extensible by recipe** — growth follows a fixed checklist | New subcommand / endpoint / option / format each have a step list (below), so features never renegotiate the architecture |
| P4 | **Maintainable by construction** — pure cores, stable contracts | `run*` functions return errors instead of exiting; exit codes 0–4 and stdout-only-results are public contracts |

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

These package-level vars are the *only* sanctioned mutable package state.
Do not add another global; add (or extend) a seam, injection point, or option
instead.

---

## P3 — Extensibility Recipes

Growth never renegotiates the architecture; it follows these checklists.

### Add a subcommand

1. `main.go`: one `case` in the dispatch switch + one line in `usage()`.
   Nothing else — main stays logic-free.
2. `internal/cmd/<name>.go`: a `register` function binding the command's
   flags; a pure `run<Name>(ctx context.Context, g *globals, pos []string) error`
   that does the work; wire via `execute` (default exit codes) or
   `executeWithExitCode` with an `exitMap`/`hint` if semantics differ
   (pattern: `vetExitCode`/`vetErrorHint` in `vet.go`).
3. Fetch through `newAPIClient(g)`; render through `internal/report`; write
   results to `g.stdout()`.
4. Tests: replace the seam vars, serve `httptest` fixtures, assert output
   *and* exit codes (`cmd/vet_test.go`, `cmd/login_test.go` are templates).

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

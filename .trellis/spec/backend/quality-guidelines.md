# Quality Guidelines

> Code quality standards for `civitai-cli` (the `civ` CLI).

---

## Overview

The project is **standard-library-only**, **all-English**, **offline-testable**,
and **reproducibly buildable**. These are hard contracts inherited from the
reference project (`danbooru-tag-mcp`) and pinned in design.md D1/D7/D9. CI
(`.github/workflows/ci.yml`) runs `go vet`, `go test`, `go test -race`, and
`go build` on every push/PR; the release workflow gates on a green suite before
publishing.

---

## Forbidden Patterns

| Forbidden | Why |
|---|---|
| Any third-party module (`go.mod` `require`) | Zero-dependency contract; `go list -m all` must print only `civitai-cli`. The `vendor/golang.org/x/...` entries in `go list -deps` are the toolchain's own stdlib-internal vendoring, NOT external modules. |
| `net/http` calls outside `internal/api` `Client.Get` | `Get` is the single outbound choke point (UA, auth, retry, concurrency gate, body limit, error shrink apply uniformly). |
| cobra / other CLI frameworks | Hand-written `switch os.Args[1]` dispatch + per-command `flag.NewFlagSet`; six commands do not justify a framework. |
| Chinese (or any non-English) in comments, GoDoc, CLI output, logs, test names, script echo | zh-CN Windows console mojibake broke `make` before; everything is English. |
| Non-ASCII UI glyphs in console output, except the `[✓]` toggle marker | Classic conhost under zh-CN code pages renders non-ASCII as mojibake; structural/alignment-critical glyphs stay ASCII (`>` cursor — `internal/console/render.go`). `[✓]` (U+2713) is the sole user-approved exception (2026-11-23): neutral width, so it cannot break column alignment the way ambiguous-width glyphs (`●`, `❯`) do on CJK terminals. |
| Wrapping a caller's fd in `os.NewFile` to stat/poll it | The GC finalizer closes the descriptor on collection, silently killing the caller's stdin; probe with `syscall.Fstat` / `GetConsoleMode` instead (`internal/console/mode_other.go`). |
| External network in unit tests | Unit tests use `httptest` + `testdata/` fixtures only; CI's unit-test path never touches the network. |
| Test-only exported API | Tests shorten retry backoffs by editing private fields in-package (`cli.backoffs`) or replace `cmd` factory seams; no production export exists just for tests. |
| `Compress-Archive` for release zips | Local-time timestamps break reproducibility; use `scripts/zipfile` (pins entry headers to `SOURCE_DATE_EPOCH`). |
| `setx` for PATH writes | Truncates long PATH values; use `env.Persist` (registry `HKCU\Environment` + `WM_SETTINGCHANGE` broadcast). |
| Registry-writing tests on a developer machine | Gated by `CIV_TEST_REGISTRY=1`, run ONLY on the isolated CI windows VM. |
| Hand-concatenated query URLs | Enum values contain spaces -> silent empty responses; always `url.Values.Encode()`. |
| `os.Exit` inside command logic | Pure `run*` functions return `error`; only the `execute*` runner / `app.Fail` exit. |

---

## Required Patterns

- **GoDoc on every exported symbol**: `// Symbol ...` opening with the symbol
  name; package comments `// Package xxx ...`. Comments explain *why* (design
  rationale, concurrency contract) and cite evidence (`verified live
  2026-11-22`), not restating the code. Private functions are commonly commented
  too; constants are centralized with their justification.
- **Functional options** for the API client: `NewClient(WithBaseURL,
  WithHTTPClient, WithToken, WithTimeout, WithConcurrency, WithBackoffs...)`.
- **Tolerant decoding** (design D4): unknown fields ignored; unknown enums kept
  verbatim (string receive + render mapping, fall back to raw on miss);
  `json.Number` for drift-prone numerics (`nsfwLevel` models-series bitmask,
  `steps`/`cfgScale`); value structs for stats (zero-safe); `*ImageMeta` for
  nullable meta (null skipped and counted). The two `nsfwLevel` forms (models
  numeric vs images string) are decoded independently, never cross-inferred.
- **Pinned enum tables** (`api/enums.go`) with a "verified live" comment;
  unknown sort/type/period are CLI usage errors, unknown baseModel only warns
  (the API silently ignores it).
- **Explicit `context.Context` first param** through handler -> api ->
  `http.NewRequestWithContext`; every wait `select`s on `ctx.Done()`.
- **Reproducible-build trio**: pinned toolchain (`go.mod` `go 1.27` /
  `toolchain go1.27.3`, CI `go-version-file`), `CGO_ENABLED=0 -trimpath
  -buildvcs=false`, deterministic zip (`SOURCE_DATE_EPOCH`). Version injected via
  `-ldflags -X civitai-cli/internal/app.Version=<git describe>`; no version
  literal hardcoded in source.
- **LF line endings** (`.gitattributes` `* text=auto eol=lf`); exe/zip `binary`.
- **CHANGELOG `## [x.y.z]` heading** is the release awk machine contract — do not
  change the format casually.

---

## Testing Requirements

- **Unit tests** (`go test ./...`, zero network, no build tags):
  - `api`: `httptest` fake servers assert UA, retry counts, backoff, error
    shapes, `Retry-After`, concurrency gate. A `countingServer`
    (`atomic.AddInt32`) verifies attempt counts; backoffs shortened in-package
    so retry tests finish in milliseconds.
  - `types`: table-driven decode (nsfwLevel dual forms, null meta, empty stats,
    unknown enums).
  - `cmd`: drive pure `run*` functions against `httptest` via the
    `newAPIClient`/`newSyncClient`/`computeHash` seams; assert rendering and exit
    codes (`TestExitCodeFor`, `TestVetExitCode`, `TestLogin_*`). Interactive
    flows: the text fallback is driven by an injected `strings.Reader`; menu
    mode is covered through the `skillMenu*` seam fakes (no TTY in tests), and
    the `*os.File` type-assert gate keeps injected readers off the real-console
    path even when `go test` runs in a terminal.
  - `hashcache`: temp-dir invalidation (size/mtime drift), atomic save, nil
    disabled cache.
  - `upgrade`: pure functions (`compareVersions`, `parseChecksum`,
    `replaceExeAt`, `cleanupBakAt`) + fake exes in a temp dir.
- **Expected-failure coverage is mandatory** (convention, 2026-11-24): every
  new command, flag, parser, or decoder ships table cases whose *expected
  outcome is failure* — usage errors, zero/negative/overflow numbers,
  wrong-case enums, empty/whitespace/unicode input, malformed JSON/root
  shapes, and boundary values (0, max, max+1). Assert the exit class, empty
  stdout on pre-render failures, and zero requests when validation precedes
  network. A green suite means every failure path was exercised and
  asserted, not that failures were omitted. Flag-parse-level failures
  (non-integer/overflow flag values) belong in a direct `FlagSet`
  parse test; run-level validation belongs in the `run*` table test.
- **Fixtures**: `testdata/*.json` are verbatim live bodies; `testdata/FIXTURES.md`
  records fetch dates + source URLs (regenerate via plan appendix D). Re-fetch
  deliberately on drift and review field changes before editing `types.go`.
- **Registry tests**: `CIV_TEST_REGISTRY=1` gate; `t.Skip` when unset. Runs only
  on the CI isolated VM; writes a throwaway named value and deletes it, never
  the user PATH.
- **Integration** (`scripts/integration-test.sh`): real exe + real network,
  asserts per-subcommand exit codes and `--json` shapes via python. Only CI
  weekly schedule / manual dispatch + local `make integration`. Exports
  `CIV_NO_BOOTSTRAP=1` / `CIV_NO_CACHE=1` itself (cache replay would mask upstream
  drift — this is the drift canary). Never in the release-blocking path.
- **Integration coverage is part of every CLI-surface change** (convention,
  2026-11-22): any task that adds, modifies, or removes CLI surface (command,
  subcommand, flag, input form, output shape, exit code) must update
  `scripts/integration-test.sh` in the same change — new surface gets cases,
  changed behavior gets updated assertions, removed features get their cases
  deleted (dead cases mask regressions and lie about the surface). The task's
  PRD lists the intended integration cases in its acceptance criteria; the
  check phase verifies script-to-surface parity. Offline-assertable cases
  (usage errors, local file effects) always run; live-API cases share the
  script's existing network preconditions.
- **Gate**: `go vet ./... && go test ./... && go test -race ./...` must be green
  before commit/release.

---

## Code Review Checklist

- [ ] No new `go.mod` require; `go list -m all` still only `civitai-cli`.
- [ ] New endpoint goes through `Client.Get`; no direct `net/http` elsewhere.
- [ ] All text English; exported symbols have `// Symbol ...` GoDoc.
- [ ] New behavior has a table-driven / `httptest` test covering success **and
      expected failures** (usage/range/case/malformed/boundary); unit tests
      stay offline.
- [ ] CLI surface changed (add/modify/remove command, flag, input form, output,
      exit code) → `scripts/integration-test.sh` updated in the same change.
- [ ] Errors wrapped with `%w` + package prefix; classified with `errors.As`;
      degradation warns instead of failing.
- [ ] Exit-code semantics preserved (0/1/2/3/4, resolve 5; `sync` not_found = data row).
- [ ] stdout carries only results; logs/progress/warnings on stderr.
- [ ] Reproducible-build flags / deterministic zip untouched; toolchain upgrade
      synced between `go.mod` and CI.
- [ ] Registry/PATH side effects gated (`CIV_TEST_REGISTRY`, `Bootstrap`,
      `CIV_NO_BOOTSTRAP`); never run registry tests locally.
- [ ] `go vet` / `go test` / `go test -race` green.
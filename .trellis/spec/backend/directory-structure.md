# Directory Structure

> How backend code is organized in `civitai-cli` (the `civ` CLI).

---

## Overview

`civitai-cli` is a zero-third-party-dependency Go CLI (module name `civitai-cli`,
bare, no domain prefix). All Go code lives under `internal/`; `main.go` only
routes. The layering is one-directional and interfaces/seams are defined at the
consumer, so each layer is testable in isolation against `httptest` servers and
fixtures (no external network in unit tests).

Design contract: `.trellis/tasks/archive/2026-11/11-22-civitai-cli-go/design.md` (§1, D1–D9).

---

## Directory Layout

```
civitai-cli/
├── go.mod                  # module civitai-cli; go 1.27; toolchain go1.27.3; ZERO require
├── main.go                 # dispatch (switch os.Args[1]) + startup maintenance only
├── Makefile                # build/test/release targets (see README "Building from source")
├── install.ps1             # one-line PowerShell installer (ASCII, no BOM)
├── installer/civitai-cli.nsi   # per-user NSIS setup wizard (no UAC)
├── scripts/zipfile/        # deterministic zip packager (main.go + main_test.go)
├── scripts/integration-test.sh # live-API smoke (real exe; CI weekly/manual + make integration)
├── .github/workflows/{ci,release}.yml
├── .gitattributes          # * text=auto eol=lf; exe/zip binary; journal merge=union
├── CHANGELOG.md            # Keep-a-Changelog; "## [x.y.z]" heading is the release awk contract
├── README.md               # English user documentation
├── docs/plan.md            # migrated Chinese planning doc + implementation-status header
├── LICENSE                 # MIT, Copyright (c) 2026 BaixuanZhu
├── internal/
│   ├── app/                # leaf infra: Version, Fail, UserAgent, leveled logger, Bootstrap gate
│   │   ├── app.go          #   Version var, Fail(err), UserAgent()
│   │   ├── log.go          #   SetVerbose/Verbose, Infof/Debugf/Warnf (atomic gate)
│   │   └── bootstrap.go    #   Bootstrap var, BootstrapEnabled() (env/ldflags/Temp decision)
│   ├── api/                # the ONLY outbound choke point + tolerant types + error classes
│   │   ├── client.go       #   Client.Get, retry loop, concurrency gate, functional options
│   │   ├── errors.go       #   ApiError/UnauthorizedError/NotFoundError/NetworkError + classify
│   │   ├── types.go        #   response structs + ParseModels/ParseModel/ParseVersion/ParseTags
│   │   ├── enums.go        #   CLI->API mapping tables (sort/period/type/baseModel whitelist)
│   │   ├── models.go       #   SearchModels, GetModel
│   │   ├── versions.go     #   GetVersion, GetVersionByHash
│   │   ├── tags.go         #   ListTags
│   │   ├── compact*.go     #   compact search/show envelopes (civ.search/v1, civ.show/v1)
│   │   ├── recipe.go       #   official recipe summary/detail + shared scalar normalizer
│   │   ├── images.go       #   bounded GET /images list + exact-image detail
│   │   ├── community.go    #   community recipe sample/detail projection
│   │   └── compare.go      #   presence-aware model/version/file comparison snapshots
│   ├── resolver/           # pure readiness policy: ref grammar, evidence correlation, report
│   │   ├── types.go policy.go resolve.go
│   ├── compare/            # pure comparison semantics: registries, tri-state equality, diffs
│   │   ├── compare.go diff.go
│   ├── cmd/                # command handlers: flag parse -> api -> report -> exit code
│   │   ├── globals.go      #   globals, registerGlobalFlags, execute/executeWithExitCode,
│   │   │                   #   exitCodeFor, printErrorHint, permuteArgs, newAPIClient seam
│   │   ├── help.go         #   per-command help pages: globalHelpBlock, writeCommandHelp,
│   │   │                   #   isBareHelp (stdout + exit 0 for `civ <cmd> help` / -h)
│   │   ├── search.go show.go recipe.go resolve.go compare.go vet.go sync.go tags.go login.go prompts.go skill.go proxy.go
│   ├── config/             # leaf: token + scoped proxy persistence, priority resolution
│   │   └── config.go       #   token + scoped proxy slots (proxy, githubProxy); Path/Load/Save/Delete, ResolveToken/ResolveProxy(ResolveGithubProxy)
│   ├── proxy/              # leaf: outbound proxy validate/resolve/transport
│   │   └── proxy.go        #   Validate/Resolve/NewTransport (stdlib net/http + net/url only)
│   ├── hashcache/          # leaf: streaming SHA256 + size/mtime cache
│   │   ├── cache.go        #   ComputeSHA256 (M2)
│   │   └── store.go        #   Cache Load/Get/Put/Save (M3)
│   ├── report/             # leaf: rendering primitives
│   │   ├── table.go json.go format.go
│   │   └── width_windows.go width_other.go  # console width (syscall / fallback 80)
│   ├── env/                # Windows-only leaf: user PATH via registry syscall
│   │   └── path_windows.go #   EnsureUserPath/Persist (HKCU\Environment + WM_SETTINGCHANGE)
│   ├── upgrade/            # self-update from GitHub Releases
│   │   └── upgrade.go      #   Run, compareVersions, parseChecksum, replaceExeAt, CleanupStaleBak
│   ├── skill/              # leaf: bundled civ-prompt Skill (go:embed) + multi-platform install
│   │   ├── embed.go        #   //go:embed assets/civ-prompt -> FS; SkillName const
│   │   ├── skill.go        #   Platform/Scope, RootFor, DetectPlatforms, version render/extract
│   │   ├── install.go      #   Install/Remove/Update, ConflictFunc, Result (pure file ops)
│   │   └── assets/civ-prompt/  # SKILL.md (standard six fields) + references/ (embedded source)
│   └── testutil/           # shared test helpers (no production import)
└── testdata/               # verbatim live fixtures + probes/ + FIXTURES.md manifest
```

---

## Module Organization

### Dependency direction (one-way)

```
main.go (dispatch + global-flag assembly + startup maintenance)
  └─ internal/cmd/*  (handler: flags -> api -> report -> exit code)
       ├─ internal/api       (Client.Get choke point + endpoint methods + types + errors)
       ├─ internal/resolver  (pure readiness policy; environment supplied as function values)
       ├─ internal/compare   (pure comparison semantics; snapshots arrive from api)
       ├─ internal/report    (tabwriter / JSON / k-M / date / truncate / console width)
       ├─ internal/hashcache (ComputeSHA256 + Cache)
       ├─ internal/config    (token/proxy read/write + ResolveToken/ResolveProxy)
       ├─ internal/proxy     (outbound proxy: Validate/Resolve/NewTransport)
       └─ internal/skill     (bundled civ-prompt Skill: embed + install/remove/update)
  infrastructure: internal/app (leaf), internal/proxy (leaf), internal/env (leaf),
                  internal/upgrade (deps: app + config + proxy)
```

- `internal/app` is a **leaf**: it imports nothing from `internal/`, so `api`,
  `cmd`, `config`, `hashcache`, and `upgrade` can all depend on it without an
  import cycle. It holds `Version`, `Fail`, `UserAgent`, the leveled logger, and
  the `Bootstrap` gate.
- `internal/resolver` and `internal/compare` are **pure policy packages**: no
  HTTP, filesystem, clock, printing, or process exit. `cmd` supplies resolver
  environment seams (lookups, stat, hash) and feeds compare snapshots decoded
  by `api`; neither package imports `cmd`, and `api` never imports them.
- `internal/env`, `internal/report`, `internal/hashcache`, `internal/config`,
  `internal/skill`, and `internal/proxy` are leaves (or depend only on other
  leaves: `config` imports `app` + `proxy`). `proxy` owns outbound proxy
  validation (`Validate`), precedence (`Resolve`), and transport construction
  (`NewTransport`) using only stdlib `net/http` + `net/url` — zero internal
  imports — so `api`, `cmd`, `config`, and `upgrade` reuse it without cycles;
  `config.Proxy` / `config.ResolveProxy` mirror the token field/resolver.
  `env` is Windows-only
  (`path_windows.go` suffix gates compilation) and uses hand-written `syscall`
  (no `golang.org/x/sys`) to keep the zero-dependency contract. `skill` embeds
  its `assets/civ-prompt` tree via `go:embed` and does pure file operations
  (hand-written frontmatter version extraction, no YAML library).
- `internal/upgrade` depends on `app`, `config`, and `proxy` — `Run` parses
  its own `--proxy` flag and resolves `--proxy` flag > saved github slot >
  env for its one shared download client via `proxy.NewTransport` (the
  upgrade→GitHub call is the accepted exception to the `Client.Get` choke
  point); `main` calls `upgrade.Run(os.Args[2:])` for the `upgrade` command
  and `upgrade.CleanupStaleBak()` at startup.

### Interfaces / seams at the consumer

The `cmd` layer never constructs an API client inline. It depends on replaceable
factory seams so tests redirect the same command code at an `httptest` server:

```go
// internal/cmd/globals.go
var newAPIClient = func(g *globals) *api.Client { ... }   // tests replace this
// internal/cmd/sync.go
var newSyncClient = func(g *globals, concurrency int) *api.Client { ... }
var computeHash = hashcache.ComputeSHA256                  // tests wrap to count calls
```

Both client factories build their shared options (token + timeout + proxy)
through one `clientOptions(g)` helper, so the proxy resolved by
`config.ResolveProxy(g.Proxy)` cannot drift between the single-shot and sync
paths.

Command logic lives in **pure `run*` functions** that return `error` and never
call `os.Exit`; the shared `execute`/`executeWithExitCode` runner owns flag
parsing, verbose mode, the error line + hint, help-page interception
(`civ <cmd> help` / `-h` → stdout, exit 0, before any run work), and
`os.Exit`. This is what makes exit-code and rendering behavior table-testable
offline.

> Note vs the reference project: `danbooru-tag-mcp` keeps response structs in the
> consumer (`service`). Here the response structs and `Parse*` decoders live in
> `internal/api/types.go` (design D4); endpoint methods return **raw bodies** so
> `--json` can pass through untouched (`ModelsRawItems`) while human output uses
> the parsed structs. The consumer-side seam is the client factory, not a
> separately declared interface.

### Where things go

| Concern | Package | Rule |
|---|---|---|
| New outbound endpoint | `internal/api` | MUST funnel through `Client.Get`; never use `net/http` directly elsewhere |
| New subcommand | `internal/cmd` | own `FlagSet` via `register`, pure `run*`, render via `report`, map exit via `execute*` |
| Response struct / decode | `internal/api/types.go` | tolerant decode; `json.Number` for drift-prone numerics |
| CLI->API enum mapping | `internal/api/enums.go` | pinned tables + "verified live" comment |
| Rendering primitive | `internal/report` | no API knowledge |
| Cross-cutting infra | `internal/app` | leaf, no `internal/` imports |
| Token / proxy persistence | `internal/config` | leaf (imports `app` + `proxy`); `Config.Proxy` + `ResolveProxy` mirror the token pattern; `cmd` calls `ResolveToken`/`ResolveProxy` at the seams |
| Outbound proxy URL | `internal/proxy` | leaf, stdlib `net/http`+`net/url` only; owns `Validate`/`Resolve`/`NewTransport`; `api`/`upgrade` install transports via `NewTransport`, `cmd`/`config` resolve precedence via `Resolve` |
| Bundled Skill management | `internal/skill` | leaf; `go:embed` source in `assets/`; pure file ops + `ConflictFunc` callback; `cmd/skill` owns interactive/flag UX |
| Command help page | `internal/cmd` | `<name>Help` const beside the command + `help` param on `execute*`; shared `globalHelpBlock` in `cmd/help.go` is the ONLY copy of global-flag rows; `upgrade` (outside `cmd`) exposes `upgrade.Help()` routed by `main.go` |

---

## Naming Conventions

- Files: lowercase, one concern per file (`client.go`, `errors.go`, `types.go`,
  `enums.go`); command files named after the command (`search.go`, `vet.go`).
- Platform-specific files use the Go suffix convention: `path_windows.go`,
  `width_windows.go` / `width_other.go`.
- Tests: `<file>_test.go` beside the code; test names `TestXxx_Behavior`
  (e.g. `TestVet_NotFoundMapsToExit3`, `TestLogin_Rejected401Exit4NoSave`).
- Exported symbols: `// Symbol ...` GoDoc required (see quality-guidelines).
- Fixtures: `testdata/*.json` verbatim API bodies; fetch dates/URLs recorded in
  `testdata/FIXTURES.md` (JSON has no comment syntax). Probe evidence under
  `testdata/probes/`.

---

## Examples

- Well-organized command: `internal/cmd/vet.go` — pure `runVet`, command-level
  exit override (`vetExitCode`/`vetErrorHint`), best-effort degraded model fetch
  (`fetchVetModel` warns, never turns a hit into exit 3).
- Choke point: `internal/api/client.go` — `Get` -> `doWithRetry` -> `doOnce`,
  functional options, `fetchOutcome` intermediate struct, `transientError`
  marker.
- Leaf infra: `internal/app/log.go` — atomic verbose gate, stderr-only leveled
  logging.
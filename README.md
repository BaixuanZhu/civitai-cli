# civitai-cli (`civ`)

A zero-dependency, single-file command-line tool for **finding** and **vetting**
[Civitai](https://civitai.com) models from the terminal.

- **Find a model** — search and filter the registry (`civ search`), inspect a
  model and its versions (`civ show`), complete tag names (`civ tags`).
- **Vet a model** — reverse-look-up a local file (or a SHA256 / AutoV2 hash)
  against the registry (`civ vet`) to answer "who made this, what base model,
  what trigger words, is it safe, what license, how do others prompt it", and
  batch-audit a whole directory (`civ sync`).

It is **anonymous by default** (the entire read surface needs no token), written
in pure Go standard library (no third-party modules), and ships as one static
`civ.exe` for Windows x64 and ARM64. An optional API token unlocks personal
queries. It does **not** download models or images, upload, or bypass NSFW
gating.

> Platform: v1 is Windows-only (amd64 + arm64). The binary name is `civ`; the
> release artifacts are named `civitai-cli-windows-<arch>...`.

---

## Quick start

```bash
# Find models: search the registry (anonymous, no token needed)
civ search anima --limit 5

# Vet a model: reverse-look-up a local file against the Civitai registry
civ vet ./models/loras/your-model.safetensors
```

`civ vet` streams the file's SHA256, queries the registry by hash, and prints a
review report (model / version / creator, trigger words, files with full hashes
and pickle/virus scan results, license grants, example-image counts).

No local file handy? Try a known hash directly (this one resolves to a real
model):

```bash
civ vet FBA1195327        # AutoV2 (first 10 hex chars of the SHA256)
```

Add `--json` to any command for machine-readable output:

```bash
civ search anima --limit 5 --json | python -c "import json,sys; print(len(json.load(sys.stdin)))"
```

---

## Features

- **Core commands** — `search`, `show`, `vet`, `sync`, `tags`, `login`/`logout`,
  and `skill` — plus `upgrade`, `version`, and `help`.
- **Vetting flagship** — `vet` accepts a file path, a 64-char SHA256, or a
  10-char AutoV2; `sync` batch-audits a directory with a size/mtime hash cache,
  duplicate detection, and CSV/JSON/table output.
- **Example-image recipes** — `--prompts` on `show -v` and `vet` prints
  copyable prompt / negativePrompt / steps / cfgScale / sampler / scheduler
  blocks (and counts images whose meta is null).
- **Bundled Agent Skill** — `civ skill install` ships the embedded `civ-prompt`
  skill to claude / zcode / codex, teaching coding agents to build text-to-image
  prompts from real Civitai trigger words and example-image recipes.
- **Self-update** — `civ upgrade` pulls the latest GitHub Release, verifies
  SHA256 against `checksums.txt`, and swaps the binary via a same-volume `.bak`
  rename with rollback.
- **Zero-dependency single file** — pure Go standard library, `CGO_ENABLED=0`,
  one static `civ.exe`; no runtime, no installer required for the portable zip.
- **Anonymous by default** — optional token via `--token`, `CIVITAI_API_KEY`,
  or `civ login`; stable exit codes and `--json` contracts for scripting.
- **Reproducible builds** — pinned Go toolchain, trimmed paths, deterministic
  zip, SHA-256 checksums, keyless build-provenance attestation, and a CI
  cross-rebuild verification job.

---

## Install

Pick one of three options. All install per-user (no admin / UAC) and verify
SHA-256 where a checksum manifest is available.

### Option 1 — Setup wizard (NSIS, recommended for new users)

1. Download `civitai-cli-windows-amd64-setup.exe` (or
   `civitai-cli-windows-arm64-setup.exe` on ARM64) from the
   [latest Release](https://github.com/BaixuanZhu/civitai-cli/releases/latest).
2. Double-click it. The wizard installs **per-user, no UAC prompt**, to
   `%LOCALAPPDATA%\Programs\civitai-cli`, and registers that directory into your
   user `PATH` (delegated to `civ.exe`'s built-in bootstrap).
3. Open a **new** terminal and run `civ version`.

Uninstall via Windows *Add/Remove Programs* or the Start Menu shortcut
(*civitai-cli > Uninstall civitai-cli*). The uninstaller first removes the install
directory from your user `PATH` (via `civ.exe unbootstrap`), then deletes the exe,
any leftover `.bak`, the registry entries, the cache (`%LOCALAPPDATA%\civ`), and
the config/token (`%APPDATA%\civ`).

> The installer stub is x86 and runs under emulation on ARM64 Windows, while
> dropping the native ARM64 `civ.exe`.

### Option 2 — One-line PowerShell installer

```powershell
iwr -useb https://raw.githubusercontent.com/BaixuanZhu/civitai-cli/main/install.ps1 | iex
```

This downloads the latest Release's portable zip for your OS arch, verifies
SHA-256 against `checksums.txt`, extracts `civ.exe` to
`%LOCALAPPDATA%\Programs\civitai-cli`, and runs it once to trigger PATH
self-bootstrap. Open a new terminal afterward.

**Mirror / self-hosted source:** set `CIV_INSTALLER_MIRROR` to a download URL
prefix (trailing slash optional) before running the one-liner:

```powershell
$env:CIV_INSTALLER_MIRROR = "https://mirror.example.com/civitai-cli/"
iwr -useb https://raw.githubusercontent.com/BaixuanZhu/civitai-cli/main/install.ps1 | iex
```

The script defaults to the GitHub Release feed
(`https://github.com/BaixuanZhu/civitai-cli/releases/latest/download/`). It also
honors `HTTPS_PROXY` / `HTTP_PROXY` natively. To install to a custom directory,
run the script locally with a parameter:

```powershell
.\install.ps1 -InstallDir "D:\tools\civitai-cli"
```

### Option 3 — Portable zip

1. Download `civitai-cli-windows-amd64.zip` (or
   `civitai-cli-windows-arm64.zip`) from the
   [latest Release](https://github.com/BaixuanZhu/civitai-cli/releases/latest).
   The zip contains a single `civ.exe`.
2. Extract `civ.exe` to any directory.
3. Run it once (`civ.exe version`) — release builds silently register their own
   directory into the user `PATH` — or add the directory to `PATH` manually.

Verify the download against the release `checksums.txt` (GNU `sha256sum`
format):

```bash
sha256sum -c checksums.txt --ignore-missing
```

---

## Commands

Global flags are accepted **after the command name** only:
`civ search anima --json` works; `civ --json search` is a usage error.

### Global flags

| Flag | Default | Description |
|---|---|---|
| `--json` | off | Machine-readable output; all human decoration is turned off. |
| `--token <key>` | empty | API token; overrides `CIVITAI_API_KEY` and the saved config. Anonymous by default. |
| `--timeout <dur>` | `30s` | Per-request HTTP timeout (Go duration, e.g. `10s`, `1m`). |
| `--verbose` | off | Verbose `[INFO]`/`[DEBUG]` logging to **stderr**. |

> **Design note D6 (`-v` conflict):** the global debug flag is `--verbose` with
> **no short alias**. `show`'s `-v` / `--ver` carries *version* semantics
> (`-v <versionId>`), so the two never collide. The bare `-v` / `--version`
> aliases only exist for `civ version` (no command context), and `-h` / `--help`
> for `civ help`.

### `civ search <query>` — find models

```bash
civ search stocking --type lora --limit 10
civ search stocking --type lora --sort most-downloaded --limit 10
civ search anime --base Illustrious --type lora --limit 5
civ search "flux.1 d" --base "Flux.1 D" --period month --limit 20
civ search anima --json                 # raw items array passthrough
```

| Flag | Default | Description |
|---|---|---|
| `--type` | none | Model type filter; repeatable or comma-separated (`--type lora,ti`). Case-insensitive aliases: `lora`, `checkpoint`, `ti`/`textualinversion`, `locon`, `dora`, `hypernet`/`hypernetwork`, `controlnet`, `vae`, `upscaler`, `poses`, `wildcards`, `other`, and more. |
| `--base` | none | baseModel filter, comma-separated. **Only the first value is applied** (see API notes); unregistered values warn but pass through. Quote values containing spaces (`--base "Flux.1 D"`). |
| `--sort` | `hot` | `hot`, `highest-rated`, `most-downloaded`, `most-liked`, `most-discussed`, `most-collected`, `most-images`, `newest`, `oldest`, `recently-added`.* |
| `--period` | `all` | Ranking period: `day`, `week`, `month`, `year`, `all`. |
| `--tag` | none | Exact tag filter. |
| `--user` | none | Creator username filter. |
| `--limit` | `20` | Max results, **1–100** (the API hard-caps at 100). |

\* `recently-added` is a registered API enum but requires a `collectionId` on
`/models`; plain model search returns HTTP 400 for it. The mapping is kept so
the server's explanatory error reaches you.

Human output is an aligned table (`ID NAME TYPE BASE DL RATIO UPDATED NSFW
CREATOR`); `--json` passes through the raw API `items` array unchanged.

### `civ show <modelId>` — inspect a model

```bash
civ show 2458426                      # model header + version table
civ show 2458426 -v 3263843           # drill into a version: files, hashes, trigger words
civ show 2458426 -v 3263843 --prompts # append example-image generation recipes
civ show 2458426 --json               # full model object
```

| Flag | Default | Description |
|---|---|---|
| `-v`, `--ver <versionId>` | none | Drill into this version (files, full hashes, scan results, trigger words). |
| `--prompts` | off | With `-v`, append example-image recipes (prompt / negativePrompt / steps / cfgScale / sampler / scheduler). Requires `-v`. |

The model header shows name, creator, type, tags, the four license grants
(`allowNoCredit` / `allowCommercialUse` / `allowDerivatives` /
`allowDifferentLicense`), stats, updated date, and the Civitai URL. Trigger
words and prompt recipes are plain-text blocks (one per line, no quotes or
bullets) so you can copy them straight into a registry.

### `civ vet <path|sha256|autov2>` — vet a model (flagship)

```bash
civ vet ./models/loras/foo.safetensors   # local file: stream SHA256, then look up
civ vet FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB  # full SHA256
civ vet FBA1195327                       # AutoV2 (first 10 hex chars)
civ vet FBA1195327 --prompts             # append example-image recipes
civ vet FBA1195327 --json                # structured review object
```

| Flag | Default | Description |
|---|---|---|
| `--prompts` | off | Append example-image generation recipes. |

Input classification: 64 hex chars → SHA256, 10 hex chars → AutoV2 (both
case-insensitive, normalized to uppercase), anything else → a local file path
(hashed streaming with a 32 KB buffer, constant memory). The report includes
model/version/creator, trigger words, files with full SHA256/AutoV2 and
pickle/virus scan results, license grants, and example-image counts.

Exit codes: `0` hit, `3` hash not registered (404), `1` usage error, `2` network
failure, `4` rejected token. `--json` emits `{hash, version, model, examples}`
(`model` is null if the owning-model fetch degraded).

### `civ sync <dir>` — batch-audit a directory

```bash
civ sync ./models/loras                          # aligned table
civ sync ./models/loras --format csv > audit.csv # CSV for spreadsheets
civ sync ./models/loras --json                   # row array (same as --format json)
civ sync ./models/loras --ext .safetensors,.ckpt --concurrency 6
civ sync ./models/loras --rehash                 # ignore cache hits, recompute every hash
```

| Flag | Default | Description |
|---|---|---|
| `--ext` | `.safetensors,.pt,.pth,.ckpt,.bin` | Comma-separated extensions to scan (leading dot optional). |
| `--format` | `table` | `table`, `csv`, or `json`. The global `--json` forces `json`. |
| `--concurrency` | `3` | API concurrency, **1–8**. Local hashing uses a fixed pool of 4 workers. |
| `--rehash` | off | Recompute every hash, ignoring cache hits. |

Recursively scans the directory, hashes files (with the size/mtime cache),
looks each hash up via by-hash, and prints one audit row per file:
`FILE STATUS MODEL VERSION BASE TRIGGERS PICKLE VIRUS MODEL_URL`. Files sharing
a registry version are marked `dupe`. A `not_found` file is a **data row, not an
error** — `sync` exits `0` as long as every API call succeeded (see Exit codes).

### `civ tags <query>` — tag-name completion

```bash
civ tags anima --limit 8
civ tags anima --json     # array of tag names
```

| Flag | Default | Description |
|---|---|---|
| `--limit` | `20` | Max tag names, **1–100**. |

### `civ login` / `civ logout` — token management

```bash
civ login     # paste your API key at the prompt (read from stdin; never echoed)
civ logout    # remove the saved key (idempotent)
```

`login` validates the pasted key with a favorites probe
(`GET /models?favorites=true&limit=1`: 200 = valid, 401 = rejected) and, on
success, saves it to `%APPDATA%\civ\config.json`. A rejected key exits `4` and
saves nothing; a network failure exits `2`. You can also pipe a key:
`echo "$KEY" | civ login`. `logout` deletes the config file and exits `0` even
when no key was saved.

### `civ upgrade` — self-update

```bash
civ upgrade
```

Checks the latest GitHub Release of `BaixuanZhu/civitai-cli`, compares versions
(never downgrades a newer local build), downloads the matching
`civitai-cli-windows-<arch>.zip`, verifies SHA256 against `checksums.txt`, and
replaces the running exe via a `.bak` rename (with rollback on failure).
Leftover `.bak` files are cleaned on every startup.

### `civ skill <action>` — install the civ-prompt Agent Skill

```bash
civ skill install                          # interactive: pick platforms + scope, confirm
civ skill install --platform claude,zcode,codex --scope user
civ skill install --platform all --force   # non-interactive, overwrite an existing copy
civ skill update --platform claude         # rewrite only when the version differs
civ skill remove --platform zcode          # delete the skill directory
civ skill install --platform codex --json  # machine-readable [{platform,path,action}]
```

`civ skill` installs, removes, and updates **civ-prompt**, a bundled
[Agent Skills](https://agentskills.io) skill that teaches a coding agent to craft
text-to-image prompts from real Civitai data: the trigger words (`trainedWords`)
that activate a LoRA, plus example-image recipes (prompt / negativePrompt / steps /
cfgScale / sampler / scheduler) gathered via `civ vet/show/search --prompts --json`.
The skill tree is embedded in the binary (`go:embed`), so any installed `civ` can
install it without the repository source on disk. This is a purely local file
operation — no network.

| Flag | Default | Description |
|---|---|---|
| `--platform` | none | `claude`, `zcode`, `codex` (comma-separated) or `all`. **Omit for interactive selection.** |
| `--scope` | `user` | `user` (global home) or `project` (current directory). |
| `--force` | off | Overwrite an existing skill without prompting. |
| `--json` | off | Emit a `[{platform,path,action}]` array instead of the human summary. |

**Platform → skill root.** Each platform gets a real copy in its native root (no
symlinks, so Windows needs no elevated privileges):

| Platform | user scope | project scope |
|---|---|---|
| `claude` | `~/.claude/skills/civ-prompt/` | `<cwd>/.claude/skills/civ-prompt/` |
| `zcode` | `~/.zcode/skills/civ-prompt/` | `<cwd>/.zcode/skills/civ-prompt/` |
| `codex` | `~/.agents/skills/civ-prompt/` | `<cwd>/.agents/skills/civ-prompt/` |

> codex reads the shared `.agents/skills/` root, **not** `.codex/skills/`. When both
> zcode and codex are selected, each gets its own native copy (zcode then sees two;
> its `.zcode/skills` copy takes priority). If codex is not detected, the skill is
> still written to `.agents/skills/` and takes effect once codex is installed — a
> stderr warning notes this.

**Interactive mode** (no `--platform`): civ detects installed platforms
(`~/.claude`, `~/.zcode`, `~/.codex`/`~/.agents`, or binaries on PATH), preselects
them, and prompts on stderr for a numbered multi-select, a scope (default `user`),
and a confirmation showing the target paths. Canceling exits `1`. When the target
already exists, install prompts `overwrite` / `skip` / `backup` (default `skip`;
`backup` renames the old directory to `civ-prompt.bak`). `--force` overwrites
without prompting. The `civ-prompt.bak` backup is kept for safety; its `name` no
longer matches the directory name, so platform skill scanners ignore it (it is not
loaded as a duplicate). `remove` deletes only the active `civ-prompt/` — delete any
leftover `.bak` manually if you want it gone.

**Actions reported:** `installed`, `updated`, `up-to-date`, `skipped`, `removed`,
`backed-up`, `not-found`. `update` compares the installed `metadata.version`
against the running civ version and rewrites only when they differ (or `--force`),
preserving the skill name. `remove` deletes only `civ-prompt/` and never touches
zcode's `.skills_store_lock.json`. Exit codes: `0` success, `1` usage error
(unknown action, invalid platform/scope, or interactive cancel).

### `civ version` / `civ help`

```bash
civ version    # aliases: civ -v, civ --version
civ help       # aliases: civ -h, civ --help
```

---

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success / hit. For `sync`, every API call succeeded — `not_found` files are data rows, not failures. |
| `1` | Usage error: bad/unknown flag, unknown command, missing argument, invalid id/path. Also an anonymous `401` on personal-state parameters (needs a token), and a `404` on `search`/`show`/`tags`. |
| `2` | Network failure: transport error, empty body, `429`, or `5xx` after retries were exhausted (or the context was canceled). |
| `3` | Hash not registered — **`vet` only**. The by-hash lookup returned `404`; the file may not originate from Civitai or may have been modified. |
| `4` | Token rejected (`401` with a token present). Re-check `--token` / `CIVITAI_API_KEY`, or run `civ login`. |

Notes:

- `sync` treats a per-file `404` as a `not_found` **data row** and keeps exit
  `0`; only a non-404 API failure (network/auth) changes the exit code.
- `vet` maps `404` to `3`; every other command maps `404` to the usage class
  (`1`).
- An anonymous `401` (e.g. a personal-state filter without a token) is a usage
  error (`1`) with a hint to supply a token, not `4`.

---

## Token & config

The read surface is fully anonymous. A token is an identity, not a VIP pass: it
adds personal-state filters (`favorites` / `hidden`) and access to gated
resources; it does not increase read capability.

**Token priority (highest first):**

1. `--token <key>` flag
2. `CIVITAI_API_KEY` environment variable
3. Saved config file (`civ login`)

**Config file:** `os.UserConfigDir()/civ/config.json` — on Windows
`%APPDATA%\civ\config.json` — holding `{"token":"..."}`. Writes use `0600`
(owner-only) semantics, best-effort ACL on Windows, and are atomic
(temp file + rename). A missing or corrupt config degrades to anonymous with a
stderr warning; it never blocks a read command.

**Login probe:** `civ login` validates the key with
`GET /models?favorites=true&limit=1` (200 = valid, 401 = invalid) before saving.

**Anonymous NSFW boundary:** `vet` and `show --prompts` read example images from
the *version detail* (`images[]`), which the API returns in full even
anonymously (including Mature examples and their prompt meta). The separate
`/images` *browse* endpoint clamps anonymous requests to Soft — v1 does not use
that endpoint, so vetting is unaffected. civ never bypasses NSFW gating: with a
token you only see what your account settings allow.

---

## Environment variables

| Variable | Effect |
|---|---|
| `CIVITAI_API_KEY` | API token; below `--token`, above the saved config. |
| `CIV_NO_BOOTSTRAP` | Non-empty disables the startup user-`PATH` registration (any build). |
| `CIV_NO_CACHE` | Non-empty bypasses the `sync` hash cache entirely (read **and** write). Used by integration tests so cached responses cannot mask upstream API drift. |
| `CIV_TEST_REGISTRY` | `=1` enables the gated registry round-trip test (`internal/env`). **CI isolated VM only — never set on a developer machine.** Daily `go test` skips it when unset. |
| `CIV_INSTALLER_MIRROR` | `install.ps1` download URL prefix override (trailing slash optional); defaults to the GitHub Release feed. |

---

## Hash cache

`sync` caches local file hashes to avoid recomputing SHA256 on every run.

- **Path:** `os.UserCacheDir()/civ/cache/hashes.json` — on Windows
  `%LOCALAPPDATA%\civ\cache\hashes.json`.
- **Shape:** a map of absolute file path → `{size, mtime, sha256}`.
- **Invalidation:** an entry hits only when **both** the file's size and mtime
  match; either changing forces a rehash. mtime is compared with `time.Equal`
  so JSON round-trip precision never causes spurious misses.
- **Atomic writes:** temp file in the same directory + rename; the cache is
  saved right after the hashing phase, so a later network failure (or Ctrl+C)
  never discards hashing labor — the rerun hits the cache.
- **`--rehash`:** recompute every hash, ignoring cache hits.
- **`CIV_NO_CACHE`:** non-empty disables the cache (a nil cache: Get misses,
  Put/Save are no-ops).

`vet` always computes the hash fresh (it does not use this cache); the cache is
a `sync` optimization. A corrupt or unreadable cache warns on stderr and
rebuilds empty — degradation is never fatal.

---

## Building from source

Requires GNU Make (bundled with Git Bash / MinGW / WSL) and the **pinned Go
toolchain**. `go.mod` declares `go 1.27` with `toolchain go1.27.3`; CI reads it
via `go-version-file: go.mod`. Reproducible builds require the same compiler
patch release — upgrade the toolchain in `go.mod` and CI together.

```bash
make build           # dev build (Bootstrap=off, no PATH writes) -> dist/<arch>/civ.exe
make test            # unit tests (zero network: httptest + fixtures)
make vet             # go vet ./...
```

| Target | Description |
|---|---|
| `build` | Dev build (`Bootstrap=off`, no user PATH writes) → `dist/<arch>/civ.exe`. |
| `build-dist` | Release-flavor build (`Bootstrap` on) → `dist/<arch>/civ.exe`. |
| `test` | `go test ./...` (zero network). |
| `test-race` | `go test -race ./...`. |
| `vet` | `go vet ./...`. |
| `fmt` | `gofmt -w .`. |
| `tidy` | `go mod tidy`. |
| `run` | Build then run: `make run ARGS='search anima'`. |
| `zip` | Deterministic zip of the already-built exe (no rebuild). |
| `installer` | Build NSIS setup exe → `dist/civitai-cli-windows-<arch>-setup.exe`. |
| `installer-only` | Package the setup exe from the already-built exe (no rebuild). |
| `release` | Build portable zip + `checksums.txt` → `dist/`. |
| `dist-all` | Build both release assets for the current `GOARCH`. |
| `integration` | Online smoke vs the live Civitai API (needs `scripts/integration-test.sh`). |
| `clean` | Remove `dist/`. |

Build flags: `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, and
`-ldflags "-s -w -X civitai-cli/internal/app.Version=<git describe>"`. Dev
builds additionally inject `Bootstrap=off`. Cross-compile ARM64 with
`make build GOARCH=arm64`; artifacts are stored per-arch (`dist/<arch>/civ.exe`)
so dual-arch builds never clobber each other. The NSIS targets require
`makensis` on `PATH` (`scoop install nsis` / `choco install nsis`).

---

## Reproducible builds & verifying downloads

Release integrity rests on **reproducible builds + SHA-256 + keyless
attestation** (no Authenticode signing).

- **Reproducible exe & zip:** pinned toolchain (`go.mod`), `CGO_ENABLED=0`,
  `-trimpath`, `-buildvcs=false`, and a deterministic zip helper
  (`scripts/zipfile`) that pins every entry header to `SOURCE_DATE_EPOCH` (the
  commit time). The same source + toolchain builds byte-identical artifacts on
  any machine. `Compress-Archive` is deliberately avoided (local-time
  timestamps break reproducibility).
- **`checksums.txt`:** GNU coreutils `sha256sum` manifest covering both zips and
  both setup exes. Shared by `civ upgrade` and `install.ps1` (both match by bare
  file name and tolerate the binary-mode `*` prefix).
- **Attestation:** each release asset carries a keyless GitHub build-provenance
  attestation (sigstore, signed via the workflow's OIDC identity) proving which
  commit/workflow produced it. Verify locally:

  ```bash
  gh attestation verify --repo BaixuanZhu/civitai-cli civitai-cli-windows-amd64.zip
  ```

- **`verify-repro` CI job:** after publishing, a Linux runner cross-rebuilds the
  exe from the tagged source and compares its SHA-256 byte-for-byte against the
  published Windows-built asset (extracted from the zip). A mismatch fails the
  job.

> The NSIS setup exe is **not** byte-reproducible (`makensis` embeds build
> metadata); it is covered by `checksums.txt` + attestation instead. The
> portable zip's `civ.exe` is the reproducible artifact.

Verify a portable download manually:

```bash
sha256sum -c checksums.txt --ignore-missing
```

---

## API notes

Civitai has no official OpenAPI spec; the client is built against live-probed
behavior (verified 2026-11-22). A few facts affect what you see:

- **CDN 5-minute cache.** Responses carry `s-maxage=300`: the same URL is a free
  CDN HIT for 5 minutes. A just-published model or version may not appear
  immediately; retry after a few minutes. This is also why a second `sync`/`vet`
  run within 5 minutes is faster.
- **baseModel whitelist.** The singular `baseModel` query param is silently
  ignored for *all* values; the working filter is the plural `baseModels`, and
  the API applies **only the first** value. `civ search --base` therefore sends
  the first value and warns when you pass more than one. Unregistered base
  values are silently ignored by the API (empty results, no error), so the CLI
  warns against a verified platform list (63 values) instead of failing. Quote
  values containing spaces (`--base "Flux.1 D"`).
- **Anonymous NSFW clamp.** The `/images` browse endpoint clamps anonymous
  requests to Soft without erroring. v1 reads example images from version
  detail (`images[]`), which is returned in full anonymously, so `vet` /
  `show --prompts` are unaffected. civ never bypasses NSFW gating.
- **Enum values contain spaces** (`Most Downloaded`, `SDXL 1.0`). URLs are
  always built via `url.Values.Encode()`; a bare space causes a silent empty
  response, which the client treats as retryable.
- **`limit` hard cap is 100.** `--limit` is validated CLI-side (1–100); the API
  returns a Zod `too_big` error above 100.
- **Tolerant decoding.** Unknown fields are ignored, unknown enum values are
  preserved verbatim, empty stats are zero-safe, and `images[].meta` may be null
  even when `hasMeta` is true (such images are skipped and counted). Upstream
  schema drift degrades output rather than crashing.

---

## License

[MIT](LICENSE) — Copyright (c) 2026 BaixuanZhu.
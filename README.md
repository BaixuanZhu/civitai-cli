# civitai-cli (`civ`)

[![Release](https://img.shields.io/github/v/release/BaixuanZhu/civitai-cli)](https://github.com/BaixuanZhu/civitai-cli/releases/latest)
[![CI](https://github.com/BaixuanZhu/civitai-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/BaixuanZhu/civitai-cli/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/BaixuanZhu/civitai-cli)](https://github.com/BaixuanZhu/civitai-cli/blob/main/go.mod)
[![License: MIT](https://img.shields.io/github/license/BaixuanZhu/civitai-cli)](LICENSE)
[![Platform: Windows amd64/arm64](https://img.shields.io/badge/platform-Windows%20amd64%20%7C%20arm64-blue)](#install)

A zero-dependency, single-file command-line tool for **finding**, **understanding**,
and **vetting** [Civitai](https://civitai.com) models from the terminal.

- **Find a model** — search and filter the registry (`civ search`), inspect a
  model and its versions (`civ show`), complete tag names (`civ tags`), and
  diff two models or versions field by field (`civ compare`).
- **Understand how it is used** — `civ recipe` returns trigger words,
  provenance, and prompt-free example summaries in one bounded call; select
  one example to get its exact prompt / negativePrompt / sampler settings.
- **Vet a model** — reverse-look-up a local file (or a SHA256 / AutoV2 hash)
  against the registry (`civ vet`) to answer "who made this, what base model,
  what trigger words, is it safe, what license, how do others prompt it", and
  batch-audit a whole directory (`civ sync`).
- **Check a local set** — `civ resolve` reports whether an explicitly declared
  set of local checkpoint/LoRA files is ready to run (exact hashes, one
  checkpoint anchor, compatible dependents): exit `0` ready, `5` completed
  not ready.

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

# Understand a version: trigger words + prompt-free example summaries
civ recipe 3263843 --json

# One example's full prompt text (1-based sourceIndex from the summary)
civ recipe 3263843 --example 1 --json

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

Add `--json` to any command for machine-readable output. `search`, `show`, and
`recipe` default to compact, versioned envelopes; `search`/`show` take
`--view full` to restore the pre-0.5 legacy shape:

```bash
civ search anima --limit 5 --json | python -c "import json,sys; print(len(json.load(sys.stdin)['items']))"
```

---

## Features

- **Grouped commands** — discover (`search`, `show`, `compare`, `tags`),
  recipes (`recipe`), local models (`vet`, `sync`, `resolve`), and utilities
  (`login`/`logout`, `proxy`, `skill`, `upgrade`, `completion`, `version`,
  `help`); `civ help` groups them the same way.
- **Vetting flagship** — `vet` accepts a file path, a 64-char SHA256, or a
  10-char AutoV2; `sync` batch-audits a directory with a size/mtime hash cache,
  duplicate detection, and CSV/JSON/table output.
- **Recipe context** — `civ recipe` returns a prompt-free summary (trigger
  words, provenance, selection counts, bounded example summaries) and one
  example's exact prompt text on demand (`--example <sourceIndex>`); the
  community source takes one bounded `/images` sample, and
  `recipe image <imageId>` returns one exact image detail.
- **Compact versioned JSON** — `--json` emits small envelopes
  (`civ.search/v1`, `civ.show/v1`, `civ.recipe/v1`; `schemaVersion: 1` for
  resolve and compare), while `--view full` on `search`/`show` reproduces the
  pre-0.5 output byte-for-byte.
- **Local readiness** — `civ resolve` evaluates only the refs you pass (local
  path, full SHA256, versioned AIR, `version:<id>`) with exact-hash matching
  and no downloads.
- **Comparison** — `civ compare models|versions` diffs 2–5 exact IDs with
  all-or-nothing fetches; `--diff` hides fields confirmed equal.
- **Bundled Agent Skill** — `civ skill install` ships the embedded `civ-prompt`
  skill to claude / zcode / codex, teaching coding agents to build text-to-image
  prompts from real Civitai trigger words and example-image recipes via
  `civ recipe`.
- **Self-update** — `civ upgrade` pulls the latest GitHub Release, verifies
  SHA256 against `checksums.txt`, and swaps the binary via a same-volume `.bak`
  rename with rollback.
- **Zero-dependency single file** — pure Go standard library, `CGO_ENABLED=0`,
  one static `civ.exe`; no runtime, no installer required for the portable zip.
- **Anonymous by default** — optional token via `--token`, `CIVITAI_API_KEY`,
  or `civ login`; stable exit codes and `--json` contracts for scripting.
- **Blocked-network friendly** — route every outbound request through an
  HTTP/HTTPS/SOCKS5 proxy: persisted with `civ proxy set`, per-invocation with
  `--proxy`, or the standard `HTTP(S)_PROXY` environment variables.
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
| `--proxy <url>` | empty | Outbound proxy URL (`http://`, `https://`, `socks5://`, or `socks5h://`); overrides the saved proxy and `HTTP(S)_PROXY` for this invocation. |
| `--timeout <dur>` | `30s` | Per-request HTTP timeout (Go duration, e.g. `10s`, `1m`). |
| `--verbose` | off | Verbose `[INFO]`/`[DEBUG]` logging to **stderr**. |

> **Design note D6 (`-v` conflict):** the global debug flag is `--verbose` with
> **no short alias**. `show`'s `-v` / `--ver` carries *version* semantics
> (`-v <versionId>`), so the two never collide. The bare `-v` / `--version`
> aliases only exist for `civ version` (no command context); the bare `-h` /
> `--help` prints the top-level `civ help`.

### Getting help

Every command ships a built-in help page with the full flag list, legal
values, and examples — on **stdout**, exit code `0`:

```bash
civ search help       # same page as: civ search -h / civ search --help
civ vet help
civ sync help
civ help search       # the same pages, named the other way around
```

The keyword is only intercepted when it is the *single* argument:
`civ search help me` still searches the query "help me". `civ help <command>`
reaches the same curated pages (`civ help upgrade` routes to upgrade's page),
and an unknown name exits `1`. `civ help` prints the command overview with a
pointer to these pages.

### Shell completion

Static completion scripts for bash, zsh, fish, and PowerShell ship inside
the binary — no `civ` subprocess runs at completion time:

```bash
source <(civ completion bash)                              # bash, this session
civ completion zsh > "${fpath[1]}/_civ"                    # zsh: any directory in $fpath
civ completion fish | source                               # fish, this session
civ completion powershell | Out-String | Invoke-Expression # PowerShell, this session
```

For a persistent install, drop the script in your shell's completion
directory (the script headers point at it) — for PowerShell, add the
`Invoke-Expression` line to `$PROFILE` instead.

### `civ search <query>` — find models

```bash
civ search stocking --type lora --limit 10
civ search stocking --type lora --sort most-downloaded --limit 10
civ search anime --base Illustrious --type lora --limit 5
civ search "flux.1 d" --base "Flux.1 D" --period month --limit 20
civ search anima --json                 # compact versioned envelope (default)
civ search anima --json --view full     # legacy raw items array passthrough
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
| `--view <v>` | `compact` | `--json` detail level: `compact` (versioned `civ.search/v1` envelope of selection fields) or `full` (the pre-0.5 raw `items` array). Requires `--json`. |

\* `recently-added` is a registered API enum but requires a `collectionId` on
`/models`; plain model search returns HTTP 400 for it. The mapping is kept so
the server's explanatory error reaches you.

Human output is an aligned table (`ID NAME TYPE BASE DL RATIO UPDATED NSFW
CREATOR`). `--json` defaults to the compact `civ.search/v1` envelope
(`schemaVersion`, `kind: "modelSearch"`, `items` with the selection fields and
every version reference). `--view full` restores the legacy raw API `items`
array byte-for-byte; see [API notes](#api-notes).

When human search stdout is piped or redirected, model NAME values are
written in full; interactive consoles retain width-based truncation, and the
other columns keep their existing limits. A failed console-width query uses
the same full-NAME fallback. JSON output is unchanged.

Both human and compact output end with a local summary of the returned page:
`returned` (this response's item count — never a match total, cursor, or
next-page estimate), non-exclusive `baseCoverage` (each distinct observed base
counts at most once per model, so coverage counts need not sum to `returned`),
and explicit `modelsWithMultipleBases` / `modelsWithoutBase` counters.
Model-level `baseModels` is the evidence source; only when it has no usable
value does the summary fall back to distinct bases across all embedded
versions. No primary base, percentage, or family inference is added. The
human footer shows at most eight base categories (`+K more (use --json for
all)` when more exist); compact JSON always carries the full sorted list under
`summary`.

### `civ show <modelId>` — inspect a model

```bash
civ show 2458426                      # model header + version table
civ show 2458426 -v 3263843           # drill into a version: files, hashes, trigger words
civ show 2458426 -v 3263843 --prompts # append example-image generation recipes
civ show 2458426 --json                # compact modelSummary envelope (default)
civ show 2458426 --json --view full    # legacy decoded model object
```

| Flag | Default | Description |
|---|---|---|
| `-v`, `--ver <versionId>` | none | Drill into this version (files, full hashes, scan results, trigger words). |
| `--prompts` | off | With `-v`, append example-image recipes (prompt / negativePrompt / steps / cfgScale / sampler / scheduler). Requires `-v`. |
| `--view <v>` | `compact` | `--json` detail level: `compact` (`civ.show/v1` `modelSummary`/`versionSummary`, all versions/files/hashes/scans retained, prompts reduced to counts) or `full` (the legacy decoded payload). Requires `--json`; conflicts with `--prompts`. |

The model header shows name, creator, type, tags, the four license grants
(`allowNoCredit` / `allowCommercialUse` / `allowDerivatives` /
`allowDifferentLicense`), stats, updated date, and the Civitai URL. Trigger
words and prompt recipes are plain-text blocks (one per line, no quotes or
bullets) so you can copy them straight into a registry.

### `civ recipe <versionId>` — compact recipe context

`recipe` answers "how is this version actually used?" in one read-only call.
The summary is deliberately prompt-free: it carries model/version identity,
trigger words, provenance, selection counts, and a bounded list of example
**summaries** (source index, text availability, small parameters). Full prompt
text appears only when you explicitly select one example.

```bash
civ recipe 3263843                    # official summary, 3 example summaries
civ recipe 3263843 --examples 0       # selection counts only, examples: []
civ recipe 3263843 --example 1        # singular detail: that example's prompt text
civ recipe 3263843 --source community --limit 10
civ recipe 3263843 --source community --sort newest --json
civ recipe 3263843 --source community --stack-summary --json  # aggregate stack view
civ recipe image 123456 --json        # one exact community image detail
```

| Flag | Default | Description |
|---|---|---|
| `--examples <n>` | `3` | Number of official example **summaries**, **0–10**. `0` returns version context and counts with `examples: []`; `1` is still a prompt-free summary. |
| `--example <index>` | none | Singular detail for one official example by its `sourceIndex` (1-based, as shown in the summary). Returns only that example's prompt/negativePrompt. Mutually exclusive with an explicitly supplied `--examples`. |
| `--source <s>` | `official` | `official` (one `GET /model-versions/{id}` request, never `/images`) or `community` (one bounded `GET /images` sample). Combining both is not supported. |
| `--limit <n>` | `20` | Community sample size, **1–100**. Ignored by the official source. |
| `--sort <s>` | `most-reactions` | Community order: `most-reactions` (fixed `period=AllTime`) or `newest` (case-insensitive; the API spelling `Most Reactions` is accepted). Rejected with the official source. |
| `--stack-summary` | off | Community source only: replace per-image observations with descriptive aggregates (resource stacks, parameter distributions, text availability) computed from the same single bounded response. Rejected with the official source and with `recipe image`. |

The community source performs exactly one bounded `GET /images` request
(`modelVersionId`, `withMeta=true`, `type=image`, `limit`, `sort`,
`period=AllTime`; no page/cursor/NSFW overrides). The default view returns
per-image observations only (image/post/user provenance, original order,
reactions, normalized parameters, resource stacks, and coverage counts) — no
aggregation, frequency table, ranking, quality score, or recommendation.
Coverage counts `returned`/`withMeta`/`usable` describe this one response; a
continuation is reported but never followed, and an empty sample is a
visibility outcome, not proof the version has no images.

`--source community --stack-summary` keeps that same single request and
instead emits descriptive aggregates (`kind: "communityStackSummary"`, fixed
top 5 per group, remaining identities/values folded into `other`): resource
usage counted by distinct images (a repeated entry on one image counts once;
identity is modelVersionId, else hash, else name, with no cross-basis merging;
unidentified entries are counted without inventing a row, and unknown types
stay verbatim), per-LoRA valid-weight medians with sample/image coverage
(duplicate weights on one image count once; invalid weights are excluded, and
nothing is defaulted or clamped), four parameter distributions
(steps/cfgScale/sampler/scheduler) with an explicit `observedImages`
denominator per field, and prompt/negative-prompt availability as status
counts with exact rates (null when the sample is empty). The aggregate never
emits prompt text, raw metadata, workflows, media URLs, per-image
observations, or cursor/continuation values. The sample is bounded,
non-random, and visibility-biased: aggregates are evidence to inspect, not
recommended, stable, best, or reproducible settings. `civ recipe image
<imageId>` performs exactly one `imageId`+`withMeta=true`+`type=image`+`limit=1`
request and returns only that image's selected detail; it never reuses list
state or fetches other images.

JSON schemas are versioned as `civ.recipe/v1` (`kind: "recipeSummary"` or
`"recipeExampleDetail"` for the official source; `"communityRecipeSample"`,
`"communityStackSummary"`, or `"communityImageDetail"` for the community
views). Known numeric fields share
one normalizer: numeric
`20` and string `"20"` become the same JSON number, LoRA weights normalize the
same way, seeds stay exact decimal strings, and missing/null stays null while
invalid/non-scalar shapes become null plus a structured issue. Empty or
missing metadata is never replaced with invented trigger words, sampler
settings, or compatibility claims. Raw `meta`, ComfyUI/workflow payloads, and
description HTML are not returned by recipe views.

### `civ resolve <ref>...` — resource-set readiness

`resolve` evaluates an explicitly declared set of local resources and reports
whether **this invocation's set** is ready. It is not a scanner, downloader,
or library: only the refs you pass are examined.

```bash
civ resolve ./models/checkpoint.safetensors ./models/lora.safetensors
civ resolve version:3263843 --json
civ resolve urn:air:anima:checkpoint:civitai:2458426@3263843
civ resolve FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB --json
```

Accepted refs (no bare IDs, names, AutoV2/short hashes, or AIR without a
version):

| Form | Meaning |
|---|---|
| `<path>` | Explicit local regular file (path syntax required: separator, leading `.`/`~`, drive prefix, or a filename extension). |
| `<sha256>` | Full 64-character SHA256 content reference. |
| `<air>` | AIR URN with an exact `@<versionId>`. |
| `version:<id>` | Exact positive model-version ID. |

`READY` requires every ref to be an explicit, present local file whose SHA256
matches exactly one visible registry file, exactly one Checkpoint anchor, and
a reviewed compatible outcome for every dependent resource (LORA,
TextualInversion, Controlnet, VAE). Missing base metadata and uncovered
model-type tuples are **unknown** and block readiness; there are no aliases or
name-only matches. Remote-only refs (hash, AIR, `version:<id>`) are diagnostic
and can never make a set ready. No model bytes are downloaded.

Exit codes are part of the contract: `0` ready, `5` completed **not ready**
(the report still prints on stdout), `1` usage/local I/O/decode, `2` network,
`4` rejected token. `--json` emits the versioned report (`schemaVersion: 1`,
`policyVersion: "resolver-v1"`) with `ready`, `summary`, per-resource results,
evidence, and findings.

### `civ compare models|versions <id>...` — field-by-field diff

```bash
civ compare models 2458426 260267 --json
civ compare versions 3108589 3263843 --json
civ compare versions 3108589 3263843 --diff   # hide confirmed-equal fields
```

| Flag | Default | Description |
|---|---|---|
| `--diff` | off | Hide fields confirmed equal across all subjects; keep changes and indeterminate states. Rejected together with `--json` (the JSON shape always carries every field). |

Comparison takes **2–5 unique exact IDs** of one kind. The first subject is
the baseline: derived trigger-word/resource/file changes are reported as
baseline-to-each-later-subject, while field equality considers every subject.
Fetches are strict and all-or-nothing (any selected 404/decode failure aborts
before stdout); version comparison deduplicates parent-model fetches. Missing,
null, and empty values stay distinct. Output never includes prompts,
description HTML, image/download payloads, scores, ranks, or a "winner"; JSON
is schema version `1` with `kind: "model"` or `"version"`.

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
failure, `4` rejected token. `--json` emits `{hash, version, model,
latestPublished, examples}` (`model` is null if the owning-model fetch
degraded); `vet --prompts --json` stays the focused `{prompts, skipped}`
recipe view.

Normal reports add one informational `LATEST` line (and the `latestPublished`
JSON block) describing the latest **published visible** version in the owning
model's response: the unique maximum valid RFC3339 `publishedAt` instant,
plus the vetted version's relationship (`currentLatest` / `newerPublished` /
`tiedLatest` / `unknown`) and an exact, case-sensitive base-model comparison
(`changed` / `unchanged` / `unknown`). Ties report sorted tied IDs without
picking a winner; a missing or malformed timestamp on any visible version
makes the latest claim explicitly unknown instead of guessing; if the vetted
version is absent from the visible list, the relationship is unknown while
the latest identity may still be reported. The context reuses the
owning-model response vet already fetches — no extra request — and it is
descriptive only: never an upgrade recommendation, outdated verdict, or trend
claim.

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
civ login --key "$KEY"   # flag form (short: -t)
civ login "$KEY"         # positional form
civ login --token "$KEY" # reuses the global token flag
civ login                # interactive prompt (read from stdin; never echoed)
civ login --key -        # read the key from stdin (history-safe one-liner)
civ logout               # remove the saved key (idempotent)
```

`login` accepts the key from, highest precedence first: the `--key` / `-t`
flag, exactly one positional argument, the global `--token` flag, or the
stdin prompt (`echo "$KEY" | civ login` also works, and `--key -` / `-t -`
reads stdin without carrying the key in the command line). The candidate is
validated with a favorites probe (`GET /models?favorites=true&limit=1`:
200 = valid, 401 = rejected) and, on success, saved to
`%APPDATA%\civ\config.json` — login rewrites only the token of the loaded
config, so other settings in the file survive. An empty or whitespace key,
or more than one positional argument, exits `1` without saving; a rejected
key exits `4` and saves nothing; a network failure exits `2`.

A key passed on the command line (`--key`, `-t`, positional) is recorded in
your shell history and visible in the process list; prefer
`echo "$KEY" | civ login` or `civ login --key -` when that matters.
`logout` deletes the config file and exits `0` even when no key was saved.

### `civ proxy <set|get|unset>` — outbound proxy

```bash
civ proxy set http://127.0.0.1:7890    # validate + persist next to the token
civ proxy set --github http://127.0.0.1:7891  # persist the GitHub-only slot (civ upgrade)
civ proxy get                          # persisted + effective proxy and its source
civ proxy                              # bare form: same status view as get
civ proxy get --json                   # both scopes, {"persisted","effective","source"} each
civ proxy unset                        # clear the saved proxy (token preserved)
civ proxy unset --github               # clear only the GitHub slot
civ search anima --proxy socks5://127.0.0.1:1080   # ad-hoc, this invocation only
```

Route civ's outbound HTTP through an HTTP, HTTPS, SOCKS5, or SOCKS5h proxy —
for networks where a direct connection to civitai.com times out or fails DNS.
There are two persisted slots that never cross: the **civitai slot**
(`civ proxy set`), routing the data commands (`search`, `show`, `vet`,
`sync`, `tags`), and the **GitHub slot** (`civ proxy set --github`), routing
only `civ upgrade`. `civ upgrade` connects directly by default and resolves
`--proxy` flag > GitHub slot > `HTTP(S)_PROXY` env, so a shared-proxy 403 on
GitHub no longer blocks self-updates.
`set` validates before anything is saved — the scheme must be `http`,
`https`, `socks5`, or `socks5h` with a non-empty `host:port`; malformed input
exits `1` and saves nothing. `unset` rewrites only the targeted slot(s) of
the loaded config (idempotent), so the saved token survives. The slots live
in the same `%APPDATA%\civ\config.json` as the token.

**Proxy priority (highest first):**

1. `--proxy <url>` flag
2. Saved config file (`civ proxy set`)
3. `HTTP_PROXY` / `HTTPS_PROXY` environment variables

`--proxy` is a global flag and follows the standard placement rule — after
the command name (`civ --proxy ... search` is a usage error). When neither
the flag nor a saved proxy is set, behavior is exactly as before proxy
support: the standard environment variables apply through Go's built-in
transport, so existing `HTTP(S)_PROXY` users see no change.

**Behind a blocked network with no saved proxy?** The environment-only
workaround still works:

```bash
export HTTPS_PROXY=http://127.0.0.1:7890   # PowerShell: $env:HTTPS_PROXY = "http://127.0.0.1:7890"
civ search anima --limit 5
```

`--json` on `get` emits `{"persisted":...,"effective":...,"source":...}`,
where `source` names where the effective proxy comes from: `config`, `env`,
or `direct`. A saved proxy that fails validation at runtime (for example, a
hand-edited config) warns on stderr and traffic falls back to env/direct —
it never fails a command.

> **NO_PROXY note:** with an explicit `--proxy` flag or saved proxy,
> `NO_PROXY` is **not** consulted — all civ outbound traffic (civitai.com,
> plus github.com for `civ upgrade`) goes through that proxy. civ talks to
> exactly these two hosts, so this is a deliberate tradeoff; `NO_PROXY` still
> applies in the environment-only fallback.

> **Upgrade note:** `civ upgrade` ignores the civitai slot (GitHub
> rate-limits shared proxy IPs with 403) and connects directly by default.
> To relay the upgrade through a proxy, pass `civ upgrade --proxy <url>` or
> persist the GitHub slot once with `civ proxy set --github <url>`.

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
civ skill install                          # interactive: pick platforms + scope (menus on a terminal)
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
cfgScale / sampler / scheduler) read through `civ recipe` (a prompt-free summary,
then one explicitly selected example's detail; `civ vet/show --prompts --json`
remains the legacy path). The skill works from the compact `--json` envelopes by
default. The skill tree is embedded in the binary (`go:embed`), so any installed
`civ` can install it without the repository source on disk. This is a purely
local file operation — no network.

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
(`~/.claude`, `~/.zcode`, `~/.codex`/`~/.agents`, or binaries on PATH) and
preselects them. On a terminal the choices are arrow-key menus: a platform
multi-select (up/down move, Space toggles, `a` toggles all, Enter accepts),
a scope single-select (default `user`), and — when the target already
exists — an `overwrite` / `skip` / `backup` single-select (default `skip`;
`backup` renames the old directory to `civ-prompt.bak`). `q`, `Esc`, or
`Ctrl+C` cancels with exit `1`; canceling the conflict menu picks the safe
`skip`. After the choices, the resolved target paths print once and the
action runs immediately — there is no confirmation step. When stdin is not a
terminal (pipes, CI), civ falls back to plain text prompts on stderr with
the same defaults, minus the confirmation. `--force` overwrites without
prompting. The `civ-prompt.bak` backup is kept for safety; its `name` no
longer matches the directory name, so platform skill scanners ignore it (it
is not loaded as a duplicate). `remove` deletes only the active
`civ-prompt/` — delete any leftover `.bak` manually if you want it gone.

**Actions reported:** `installed`, `updated`, `up-to-date`, `skipped`, `removed`,
`backed-up`, `not-found`. `update` compares the installed `metadata.version`
against the running civ version and rewrites only when they differ (or `--force`),
preserving the skill name. `remove` deletes only `civ-prompt/` and never touches
zcode's `.skills_store_lock.json`. Exit codes: `0` success, `1` usage error
(unknown action, invalid platform/scope, or interactive cancel).

### `civ version` / `civ help`

```bash
civ version    # aliases: civ -v, civ --version
civ help       # aliases: civ -h, civ --help; lists commands and global flags
```

Per-command help: `civ <command> help` or `civ <command> -h` (see
[Getting help](#getting-help)).

---

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success / hit. For `sync`, every API call succeeded — `not_found` files are data rows, not failures. |
| `1` | Usage error: bad/unknown flag, unknown command, missing argument, invalid id/path. Also an anonymous `401` on personal-state parameters (needs a token), and a `404` on `search`/`show`/`tags`. |
| `2` | Network failure: transport error, empty body, `429`, or `5xx` after retries were exhausted (or the context was canceled). |
| `3` | Hash not registered — **`vet` only**. The by-hash lookup returned `404`; the file may not originate from Civitai or may have been modified. |
| `4` | Token rejected (`401` with a token present). Re-check `--token` / `CIVITAI_API_KEY`, or run `civ login`. |
| `5` | Completed but **not ready** — `resolve` only. The evaluation finished and the report is on stdout; this is a verdict, not an operational error. |

Notes:

- `sync` treats a per-file `404` as a `not_found` **data row** and keeps exit
  `0`; only a non-404 API failure (network/auth) changes the exit code.
- `vet` maps `404` to `3`; every other command maps `404` to the usage class
  (`1`). `resolve` reports an unregistered hash as an unresolved resource
  inside a completed not-ready result (`5`).
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
`%APPDATA%\civ\config.json` — holding `{"token":"...","proxy":"..."}` (the
optional `proxy` key is written by `civ proxy set`; token-only files from
earlier versions work unchanged). Writes use `0600`
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
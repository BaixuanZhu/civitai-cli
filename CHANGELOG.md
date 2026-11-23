# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release notes are extracted from the `## [x.y.z]` section matching the tag (see
`.github/workflows/release.yml`); replace `- Unreleased` with the release date
when tagging. The `## [x.y.z]` heading format is a machine contract for that
awk extraction — do not change it casually.

## [0.3.0] - Unreleased

### Added

- Built-in per-command help: `civ <command> help` and `civ <command>
  -h/--help` print a curated page (usage with positional arguments, the
  command's flags with legal values and defaults, examples, and the shared
  global-flags section) to stdout with exit 0 — for every command including
  `upgrade`. This replaces the Go default flag dump that previously went to
  stderr with no synopsis or examples. The `help` keyword is only
  intercepted as a single argument, so `civ search help me` still searches
  the query "help me", and `civ login help` returns immediately without
  touching stdin. `civ help` now points at the per-command pages, and
  unknown-flag errors no longer print the noisy default usage dump.

### Changed

- `civ skill install` without `--platform` now asks through arrow-key menus
  on a terminal: a platform multi-select (up/down move, Space toggles, `a`
  toggles all, Enter accepts; detected platforms pre-toggled), a scope
  single-select, and an overwrite/skip/backup single-select for conflicts
  (default `skip`). `q`, `Esc`, or `Ctrl+C` cancels with the existing
  `skill: canceled by user` error (exit 1); canceling the conflict menu
  resolves to the safe `skip`. The final `Proceed? [y/N]` confirmation is
  removed everywhere: the resolved target paths print once as a record and
  the action runs immediately. Non-terminal stdin (pipes, CI) keeps the
  text prompt flow, minus the confirmation. The menus live in a new
  `internal/console` leaf package built on hand-written kernel32 syscalls
  (GetStdHandle/GetConsoleMode/SetConsoleMode), preserving the
  stdlib-only dependency contract; non-Windows and VT-less terminals fall
  back to the text prompts automatically.

## [0.2.0] - 2026-11-22

### Added

- `civ login` accepts the API key non-interactively: `civ login --key <token>`
  (short `-t`), a single positional `civ login <token>`, or the global
  `--token` flag — in that precedence, with the interactive prompt kept as the
  fallback. `--key -` / `-t -` reads the key from stdin (gh/curl convention)
  for a history-safe one-liner. Saving is load-modify-save, so other config
  fields survive a login; empty or whitespace candidates exit 1 before any
  probe or save. Help and README document the history-exposure tradeoff.
- Proxy support: `civ proxy set <url>` validates and persists an outbound
  proxy (`http` / `https` / `socks5` / `socks5h`, non-empty `host:port`) to
  the config file next to the token; `civ proxy get` (or bare `civ proxy`)
  reports the persisted and effective proxy plus its source, with `--json`;
  `civ proxy unset` clears it idempotently while preserving the token.
  Malformed `set` input exits 1 without saving.
- `--proxy <url>` global flag overrides the saved proxy for a single
  invocation on every API command. Precedence: `--proxy` flag > saved config >
  `HTTP_PROXY`/`HTTPS_PROXY` (env keeps applying unchanged when nothing
  explicit is set). The resolved proxy covers all civ outbound traffic,
  including `civ upgrade` (GitHub release check and asset download — upgrade
  honors the saved config proxy and env but does not parse the `--proxy`
  flag). With an explicit proxy, `NO_PROXY` is not consulted (civ targets
  exactly civitai.com and github.com). A persisted proxy that fails
  validation at runtime warns and degrades to env/direct instead of failing
  the command.

## [0.1.1] - 2026-11-22

### Fixed

- Uninstall now removes the install directory from the user `PATH`. The NSIS
  uninstaller previously left a dead `PATH` entry behind; it now runs the
  internal `civ unbootstrap` command (`env.RemoveUserPath`) before deleting the
  exe, reusing the same safe registry write path as the install bootstrap
  (no `setx`/NSIS string truncation of long `PATH` values).

## [0.1.0] - 2026-11-22

### Added

- `civ search <query>`: search Civitai models with `--type`, `--base`, `--sort`,
  `--period`, `--tag`, `--user`, and `--limit` filters; human-readable table or
  `--json` (raw items array). Anonymous by default; an optional API token
  (`--token` / `CIVITAI_API_KEY` / config) unlocks personal queries.
- `civ show <modelId>`: model header plus a version table; `-v <versionId>`
  drills into a single version (files, stats, and trigger words / prompts).
- `civ tags <query>`: tag-name completion with `--limit`.
- `civ vet <hash|file>`: vet a local file or a known hash against the Civitai
  registry. Streams SHA256 for file inputs (with an on-disk hash cache),
  resolves auto-v2 hashes, and maps results to exit codes (0 hit, 3 not found,
  2 network failure, 4 token rejected).
- `civ sync <dir>`: walk a directory for model files, hash them (cached,
  concurrent), batch-look-up each against the registry, and report matches /
  duplicates / `not_found` rows as CSV, JSON, or a table.
- `civ login` / `civ logout`: store and clear the API token under
  `%APPDATA%\civ\config.json`; token precedence is `--token` >
  `CIVITAI_API_KEY` > config.
- `civ skill install|remove|update`: install, remove, and update the bundled
  **civ-prompt** Agent Skill across claude (`~/.claude/skills`), zcode
  (`~/.zcode/skills`), and codex (`~/.agents/skills`, never `.codex/skills`).
  Interactive platform/scope selection with detected-platform preselect, or
  non-interactive `--platform claude,zcode,codex|all --scope user|project --force`;
  conflict overwrite/skip/backup, version-aware update, `--json` results, and real
  per-platform copies (no symlinks). The skill tree is embedded via `go:embed`, so
  every installed civ carries it. civ-prompt grounds text-to-image prompts in real
  Civitai trigger words and example-image recipes (`civ vet/show/search --prompts`).
- Resilient API client: a single outbound chokepoint with bounded retry
  (5xx / network / 429, `Retry-After` honored), a concurrency gate, shrunken
  error bodies, and tolerant JSON decoding for upstream schema drift.
- `civ upgrade`: self-update from GitHub Releases — version comparison
  (never downgrades a newer local build), streamed download with linear
  backoff, SHA256 verification against `checksums.txt`, and a same-volume
  `.bak` rename swap with rollback. Leftover `.bak` files are cleaned on
  every startup.
- PATH self-bootstrap: release builds silently register their own directory
  into the user PATH (`HKCU\Environment` + `WM_SETTINGCHANGE` broadcast, no
  `setx`). Disabled for dev builds (`Bootstrap=off`), when `CIV_NO_BOOTSTRAP`
  is set, or when the exe runs from the system Temp dir.
- Windows distribution: a per-user NSIS setup wizard (no UAC) and an ASCII
  one-line PowerShell installer (`install.ps1`), both verifying SHA256 and
  delegating PATH registration to the exe. The uninstaller removes the binary,
  registry entries, the cache (`%LOCALAPPDATA%\civ`), and the config/token
  (`%APPDATA%\civ`).
- Reproducible builds: pinned Go toolchain, `CGO_ENABLED=0`, `-trimpath`,
  `-buildvcs=false`, and a deterministic zip helper (`scripts/zipfile`) keyed
  to `SOURCE_DATE_EPOCH`.
- GitHub Actions CI (unit tests on every push/PR; a weekly + manual online
  smoke against the live API) and tag-triggered release (dual-arch setup exes
  and portable zips, `checksums.txt`, keyless build-provenance attestation, and
  a Linux cross-rebuild `verify-repro` job that compares published zip hashes).
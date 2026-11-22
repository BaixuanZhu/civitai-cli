# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release notes are extracted from the `## [x.y.z]` section matching the tag (see
`.github/workflows/release.yml`); replace `- Unreleased` with the release date
when tagging. The `## [x.y.z]` heading format is a machine contract for that
awk extraction — do not change it casually.

## [0.1.0] - Unreleased

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
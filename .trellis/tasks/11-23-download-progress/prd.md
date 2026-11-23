# upgrade: download progress bar with percent and speed

## Goal

The upgrade zip download (the only network download in the tool) shows live progress on a TTY: percentage, downloaded/total size, and transfer speed, refreshed in place; non-TTY output stays a single quiet line.

## Requirements

- **R1** — While the upgrade asset downloads, an interactive stdout (TTY) shows in-place progress: percent when the total is known, downloaded/total size, and current transfer speed. The line ends with a newline on completion.
- **R2** — Non-TTY stdout keeps today's script-friendly shape: the existing `Downloading <asset> ...` line, no cursor movement, no ANSI sequences on stdout.
- **R3** — Total size comes from the HTTP `Content-Length` of the response; when the header is absent the GitHub asset `Size` (already present in the release JSON) is used; when both are absent, progress shows downloaded bytes and speed without a percent.
- **R4** — Speed is the rate over the most recent refresh interval (bytes written between two repaints divided by the elapsed time), so a stalled connection visibly drops to 0.
- **R5** — The retry loop (`downloadFile`, up to 3 retries) resets progress state per attempt: no bleeding of counts or rates across attempts.
- **R6** — Progress renders on stdout next to the existing upgrade status lines; stderr logging discipline unchanged; no `--json` impact (upgrade has none).
- **R7** — Size and rate formatting reuses `internal/report.FormatSizeKB` (the shared formatting vocabulary); no new formatter is invented (code-reuse guide).

## Acceptance Criteria

- [ ] **AC1** Pure formatter function (counts/total/elapsed → display string) is table-tested: known total renders `NN% (x/y, z/s)` shape; unknown total renders `(x, z/s)` without percent; KB/MB boundaries match `report.FormatSizeKB` output.
- [ ] **AC2** Unit test: with a non-TTY stdout (pipe), the download path emits no `\r`, no ANSI escape, and exactly the same lines as today.
- [ ] **AC3** Unit test: the progress writer repaints at most at the throttle interval and always once on completion; final state ends with `\n`.
- [ ] **AC4** Unit test: a retry after a mid-stream failure starts counting from zero (per-attempt state).
- [ ] **AC5** Unit test: missing `Content-Length` falls back to the asset size; missing both yields byte/speed-only output.
- [ ] **AC6** TTY detection uses `internal/console.IsTerminal` (existing leaf), never a new syscall site.
- [ ] **AC7** `go build ./...`, `go vet ./...`, `go test ./...` green.

## Constraints

- Zero new dependencies; stdlib + `internal/console` (TTY detection) + `internal/report` (size formatting) only — both are leaves, no import cycle with `internal/upgrade`.
- Progress must not corrupt the exit-code contract or the `--json`-free upgrade output.
- This child is sequenced AFTER `11-23-upgrade-direct` (both touch the download path in `internal/upgrade/Run`).

## Notes

- Verification of the live TTY rendering is manual (parent AC5 smoke list); everything render-related below the TTY gate is unit-tested.
- The asset is a few MB; the win is perceptual feedback on slow networks, not throughput.

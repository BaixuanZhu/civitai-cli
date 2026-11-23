# Implement: download progress (child 11-23-download-progress)

> **Wave scope override (parent prd constraint):** touch ONLY `internal/upgrade/*.go` and its tests. Do NOT touch `main.go`, `scripts/`, `CHANGELOG.md`, `README.md`, or any non-upgrade package. Do NOT git commit. This child lands after 11-23-upgrade-direct.

Execution order and validation. Land AFTER 11-23-upgrade-direct (same `Run` region). Validation commands assume repo root.

## Checklist

1. **internal/upgrade/progress.go** (new)
   - `formatProgress(n, total int64, rateBytesPerSec float64) string` — pure renderer: `NN% (x/y, z/s)` when total > 0, `(x, z/s)` when total == 0; sizes via `report.FormatSizeKB(float64(bytes)/1024)`; rate likewise; percent integer-rounded. GoDoc each exported-nothing file section per convention (package is internal; unexported symbols with doc comments where non-obvious).
   - `type progressWriter struct { dst io.Writer; out io.Writer; total int64; ... now func() time.Time; lastRepaint time.Time; lastCount int64; interval time.Duration; started time.Time }` implementing `io.Writer` + `Finish()`: counts bytes, repaints `\r%-*s` (pad to clear leftovers) at most every `interval` (const 120ms) and once on `Finish` (which also writes `\n`).
   - `newProgressWriter(dst io.Writer, total int64, out io.Writer, isTTY bool) io.Writer`: non-TTY → return `dst` unchanged (zero-cost passthrough, D6).
2. **internal/upgrade/upgrade.go**
   - `downloadOnce(url, dest)` → `downloadOnce(url, dest string, expectedSize int64)`: build the meter when `console.IsTerminal(os.Stdout.Fd())`, `io.Copy(meter, resp.Body)`; total = Content-Length if > 0 else expectedSize.
   - `downloadFile` passes `expectedSize` through (retry loop unchanged — each attempt gets a fresh meter naturally since the meter is created inside `downloadOnce`).
   - `Run`: pass `asset.Size` down; on TTY print `Downloading <asset> ...` WITHOUT `\n` and let `Finish` terminate the line; on non-TTY keep today's `...\n` exactly.
3. **Tests (internal/upgrade/progress_test.go + upgrade_test.go additions)**
   - Table tests for `formatProgress` (0%, 42%, 100%; unknown total; KB and MB magnitudes vs `FormatSizeKB`; rate 0 → `0 B/s` rendering per existing vocabulary).
   - `TestProgressWriter_ThrottlesAndFinishes`: fake clock seam, feed chunks, count repaints ≤ expected, assert final `\n`.
   - `TestProgressWriter_NonTTYIsPassthrough`: newProgressWriter returns dst unchanged.
   - `TestDownloadOnce_NonTTYEmitsNoControlChars`: httptest server streaming N bytes; run copy with piped (non-TTY) stdout — assert dest bytes correct and captured output has no `\r`/ESC. (Use the same pipe trick existing tests use for output capture.)
   - `TestDownloadOnce_ProgressCountsFromZeroOnRetry`: first attempt fails mid-stream (server hijack/early close), second succeeds; assert via meter seam or log capture that the second attempt's counts start at 0 (simplest: expose nothing — assert total bytes written to dest across attempts == content length and no error; meter reset is structural — verify by reading code path or a small injectable hook).
4. **Docs**: CHANGELOG Unreleased → Added (upgrade download progress). README upgrade paragraph: one clause.

## Validation

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] Manual smoke (parent AC5 list): `go run . upgrade` on a TTY (Windows Terminal / Git Bash tty) shows the live line; piping to a file shows the old quiet output.

## Review gates

- Gate A (after step 1): pure renderer output shapes match prd AC1 exactly — review the table test cases as the contract.
- Gate B (before commit): grep the non-TTY path for escape bytes (`\r`, `\x1b`) — must appear ONLY behind the TTY gate; `git diff` import list = stdlib + console + report only.

## Rollback point

Single commit; revert. Non-TTY behavior unchanged by construction, so rollback risk is low.

## Notes for the implementer

- Do not add a cumulative-average readout; one speed number per prd R4.
- `report.FormatSizeKB` takes KB as float64 — pass `float64(bytes)/1024`; do not wrap a second formatter around it.
- Keep the 120ms interval as an unexported const; do not make it a flag.

# Design: download progress (child 11-23-download-progress)

## Summary

Wrap the upgrade asset download with a TTY-gated, throttled, in-place progress line: percent (when total known), downloaded/total size, current speed. Scripts (non-TTY) see byte-identical output to today. Sequenced after `11-23-upgrade-direct`, which reshapes the same `Run` function.

## Current state (verified in code)

- `Run` prints `Downloading <asset> ...\n`, then `downloadFile(url, tmp, 3)` → up to 4 attempts via `downloadOnce`: streamed `io.Copy(f, resp.Body)` with no metering; then `Verifying SHA256... OK` etc.
- `internal/console.IsTerminal(fd uintptr) bool` exists (leaf, no internal imports) — the established TTY probe.
- `report.FormatSizeKB(kb float64)` renders the shared human size vocabulary (e.g. KB→MB) — reuse per code-reuse guide; upgrade currently does not import `report` or `console` (both leaves; adding these imports is cycle-safe).

## Design decisions

- **D1 — Layout of the change.** New file `internal/upgrade/progress.go` holds: the `progressWriter` (an `io.Writer` metering wrapper), the pure `formatProgress` renderer, and the TTY gate helper. `upgrade.go` changes are minimal: print the label without a trailing newline on TTY, wrap `resp.Body` (or the file copy) with the meter, and close the line after `downloadFile` returns. Keeping the new machinery in its own file keeps `upgrade.go` cohesive (mechanism vs. policy) and mirrors how `console` owns menu rendering.
- **D2 — Output shape.** On TTY: `Downloading civitai-cli-windows-amd64.zip ...  42% (2.1/5.0 MB, 640 KB/s)` refreshed in place with `\r` + space padding; on completion the final render is overwritten by `done`-style completion — actually simpler and honest: leave the last progress render, move to the next status line with `\n` (the next step prints `Verifying SHA256... `). No separate "done" word for the download step; the flowing line is the done signal. On non-TTY: exactly today's bytes — label with `\n`, silent copy, no ANSI.
- **D3 — Total size resolution.** `downloadOnce` gains an `expectedSize int64` parameter. Order: `Content-Length` when > 0; else the release asset `Size` passed down from `Run`; else 0 (unknown → byte/speed-only render, no percent). The asset `Size` field is already parsed into `githubRelease`.
- **D4 — Speed.** Rate over the last repaint interval: on each `Write`, accumulate bytes; a repaint due every ~120 ms computes `(bytesNow - bytesAtLastRepaint) / dt`. A stalled connection therefore decays to 0 B/s within one interval. The first interval's rate may spike on fast networks — acceptable; cumulative average is NOT shown (one number, current speed, matches the feedback).
- **D5 — Throttle & finish.** Repaint at most every 120 ms (constant, unexported) plus a forced final repaint on `Close`/`Finish` so the 100% state is always seen; each `downloadOnce` attempt constructs a fresh meter, giving per-attempt reset for free (R5) — `downloadFile` retry state stays as-is.
- **D6 — Where the meter sits.** Meter wraps the destination-file write side (`io.MultiWriter`-free): implement `io.Writer` on the meter and `io.Copy(dstFile, io.TeeReader(resp.Body, meter))`? Cleaner: `meter := newProgressWriter(file, total, tty)` and `io.Copy(meter, resp.Body)` — the meter forwards to the file after counting. On non-TTY, `newProgressWriter` returns the file itself (nil-cost passthrough, zero branching in the copy loop).
- **D7 — Testability.** `formatProgress(n, total, rate float64)` is pure → table tests (percent rounding, unknown total, KB/MB boundaries vs `FormatSizeKB`). The meter takes an `io.Writer` + a `now func() time.Time` seam (defaults to `time.Now`) so throttle behavior is deterministic. TTY gate takes a bool/`uintptr` fd — tests exercise the non-TTY path through the real copy functions (no escape bytes asserted) and the TTY render path purely.

## Contracts touched

- Internal-only: `downloadOnce(url, dest)` → `downloadOnce(url, dest, expectedSize)`. User-visible: stdout progress rendering (TTY only). Exit codes unchanged.

## Rollback

Revert the child commit; non-TTY behavior is unchanged so rollback risk is minimal.

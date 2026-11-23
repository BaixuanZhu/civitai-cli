package upgrade

// This file holds the TTY-gated, in-place download progress machinery
// (design D1): the pure formatProgress renderer, the progressWriter metering
// io.Writer, and the TTY gate seam. Keeping it out of upgrade.go leaves the
// download policy cohesive while the console/report imports (both leaves)
// stay cycle-safe in one place.

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"civitai-cli/internal/console"
	"civitai-cli/internal/report"
)

// progressRepaintInterval caps how often the live download line repaints
// (design D5): frequent enough to feel live, sparse enough not to flood the
// terminal with escape traffic on fast networks. Not a flag — one throttle
// for every run.
const progressRepaintInterval = 120 * time.Millisecond

// progressPadFloor is the minimum repaint width. Run prints the label
// "Downloading <asset> ... " (45 columns for the agreed asset name) before
// the meter takes the line over, so every repaint pads to at least 64
// columns to fully overwrite the label tail — a shorter render would leave
// stale characters behind the fresh progress text.
const progressPadFloor = 64

// progressGate reports whether stdout can host the in-place progress line.
// It is a package seam (like downloadClient) so tests can force the TTY or
// non-TTY path deterministically even when go test runs attached to a real
// console — the same trick as skillStdinIsConsole in cmd. The TTY probe
// itself is internal/console.IsTerminal, never a new syscall site (prd AC6).
var progressGate = func() bool { return console.IsTerminal(os.Stdout.Fd()) }

// formatProgress renders the live download state as one display string:
// "NN% (x/y, z/s)" when the total size is known, "(x, z/s)" when it is not.
// Sizes and the rate reuse report.FormatSizeKB so the progress vocabulary
// can never drift from the rest of the tool (prd R7); rateBytesPerSec is
// given in bytes per second and converted here. Percent is integer-rounded.
// Pure function, table-tested.
func formatProgress(n, total int64, rateBytesPerSec float64) string {
	rate := report.FormatSizeKB(rateBytesPerSec/1024) + "/s"
	if total <= 0 {
		return fmt.Sprintf("(%s, %s)", report.FormatSizeKB(float64(n)/1024), rate)
	}
	pct := int(math.Round(float64(n) * 100 / float64(total)))
	return fmt.Sprintf("%d%% (%s/%s, %s)",
		pct,
		report.FormatSizeKB(float64(n)/1024),
		report.FormatSizeKB(float64(total)/1024),
		rate,
	)
}

// progressWriter is an io.Writer that counts bytes on their way to dst and
// repaints a throttled, in-place progress line on out. It is not safe for
// concurrent use: a single io.Copy drives it, and each download attempt
// constructs a fresh one so counts and rates never bleed across retries
// (prd R5).
type progressWriter struct {
	dst      io.Writer // final destination (the dest file)
	out      io.Writer // progress display (stdout on a TTY)
	total    int64     // expected total; 0 = unknown (no percent)
	now      func() time.Time
	interval time.Duration // throttle between repaints

	count       int64     // bytes forwarded to dst so far
	lastCount   int64     // bytes counted at the last repaint (rate window)
	lastRepaint time.Time // clock time of the last repaint
	padWidth    int       // widest line written so far, to clear leftovers
	finished    bool
}

// Write forwards p to dst, then repaints the progress line when the throttle
// interval has elapsed since the last repaint.
func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	w.count += int64(n)
	if w.now().Sub(w.lastRepaint) >= w.interval {
		w.repaint()
	}
	return n, err
}

// Finish forces a final repaint — so the completed state is always seen,
// even when the copy finished inside one throttle window — and terminates
// the line with a newline. Idempotent.
func (w *progressWriter) Finish() {
	if w.finished {
		return
	}
	w.finished = true
	w.repaint()
	io.WriteString(w.out, "\n")
}

// repaint redraws the line in place: a carriage return, the rendered state,
// then padding so a shorter render never leaves tail characters of the
// previous line behind. The rate is the bytes written since the previous
// repaint over the elapsed wall time, so a stalled transfer visibly decays
// to 0 within one interval (prd R4); a zero elapsed time yields rate 0
// instead of dividing by zero.
func (w *progressWriter) repaint() {
	now := w.now()
	dt := now.Sub(w.lastRepaint).Seconds()
	var rate float64
	if dt > 0 {
		rate = float64(w.count-w.lastCount) / dt
	}
	line := formatProgress(w.count, w.total, rate)
	if len(line) > w.padWidth {
		w.padWidth = len(line)
	}
	if w.padWidth < progressPadFloor {
		w.padWidth = progressPadFloor
	}
	io.WriteString(w.out, "\r"+line+strings.Repeat(" ", w.padWidth-len(line)))
	w.lastCount = w.count
	w.lastRepaint = now
}

// newProgressWriter wraps dst with live progress rendering when isTTY is
// true; when false it returns dst unchanged, so the non-TTY copy loop pays
// nothing for the feature and scripts keep byte-identical silence
// (prd R2). out is the writer the repaints target (os.Stdout in production).
func newProgressWriter(dst io.Writer, total int64, out io.Writer, isTTY bool) io.Writer {
	if !isTTY {
		return dst
	}
	t := time.Now()
	return &progressWriter{
		dst:         dst,
		out:         out,
		total:       total,
		now:         time.Now,
		interval:    progressRepaintInterval,
		lastRepaint: t,
	}
}

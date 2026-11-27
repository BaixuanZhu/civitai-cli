package report

import (
	"io"
	"os"
	"text/tabwriter"
	"unicode/utf8"
)

// fallbackConsoleWidth is the layout width used when no real console
// width is available: non-file writers, pipes, redirects, and failed
// platform queries all share this 80-column budget so layout arithmetic
// stays identical to the pre-probe behavior.
const fallbackConsoleWidth = 80

// NewTableWriter returns the project-standard tabwriter: elastic columns
// (minwidth 0, tabwidth 4, padding 2, space padchar). Callers write
// tab-separated rows and must Flush before returning.
func NewTableWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
}

// Truncate shortens s to at most max runes, marking the cut with a
// single-character ellipsis. max <= 0 yields "". Rune-based counting is
// an approximation of display cells (CJK double-width is not modeled);
// it keeps truncation deterministic and never splits a rune.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// ConsoleWidthFor reports the output width available to w and whether w
// is an interactive console. A non-*os.File writer (including bytes.Buffer)
// and a nil file are never a console, so an injected test writer cannot
// accidentally inherit the process's real terminal. A positive platform
// width yields (width, true); a zero/negative result — pipes, redirects,
// missing procedures, failed queries — yields (fallbackConsoleWidth, false)
// silently, because a rendering fallback is not a degradation worth a
// warning. On non-Windows an interactive character device reports the
// existing 80-column fallback with console=true while a pipe reports
// (fallbackConsoleWidth, false). Callers use the mode bit to relax
// layout guarantees on redirected output (search prints full model
// names there) while the width keeps driving the column budgets.
func ConsoleWidthFor(w io.Writer) (width int, console bool) {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return fallbackConsoleWidth, false
	}
	return consoleWidthOrFallback(consoleWidth(f.Fd()))
}

// consoleWidthOrFallback normalizes a platform width query: a positive
// width proves an interactive console; zero or negative means the query
// failed or the target is not a console, so the fallback width applies
// with console=false.
func consoleWidthOrFallback(width int) (int, bool) {
	if width > 0 {
		return width, true
	}
	return fallbackConsoleWidth, false
}

// ConsoleWidth returns the stdout console width in columns, falling back
// to 80 when stdout is not a console (pipes, redirects, CI) or the query
// fails. Layout budgets use this to keep tables from wrapping wildly. It
// is the compatibility wrapper over ConsoleWidthFor for callers that do
// not need the console/non-console distinction.
func ConsoleWidth() int {
	width, _ := ConsoleWidthFor(os.Stdout)
	return width
}

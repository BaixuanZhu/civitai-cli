package report

import (
	"io"
	"text/tabwriter"
	"unicode/utf8"
)

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

// ConsoleWidth returns the stdout console width in columns, falling back
// to 80 when stdout is not a console (pipes, redirects, CI) or the query
// fails. Layout budgets use this to keep tables from wrapping wildly.
func ConsoleWidth() int {
	if w := consoleWidth(); w > 0 {
		return w
	}
	return 80
}

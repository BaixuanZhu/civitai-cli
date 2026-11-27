package report

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"Pantyhose & Stockings NSFW", 12, "Pantyhose &…"},
		{"abcdef", 1, "…"},
		{"abc", 0, ""},
		{"abc", -3, ""},
		{"日本語の名前", 4, "日本語…"},
	}
	for _, tc := range cases {
		if got := Truncate(tc.in, tc.max); got != tc.want {
			t.Errorf("Truncate(%q,%d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
}

func TestConsoleWidth_FallbackUnderTests(t *testing.T) {
	// go test pipes stdout, so the console query fails and the documented
	// 80-column fallback must apply.
	if got := ConsoleWidth(); got != 80 {
		t.Errorf("ConsoleWidth under piped stdout = %d, want 80", got)
	}
}

func TestConsoleWidthOrFallback(t *testing.T) {
	cases := []struct {
		in          int
		wantWidth   int
		wantConsole bool
	}{
		{120, 120, true}, // successful query keeps the real width
		{80, 80, true},   // a console of exactly the fallback width is still a console
		{0, 80, false},   // failed query / non-console falls back without the mode bit
		{-5, 80, false},  // negative is as invalid as zero
	}
	for _, tc := range cases {
		width, console := consoleWidthOrFallback(tc.in)
		if width != tc.wantWidth || console != tc.wantConsole {
			t.Errorf("consoleWidthOrFallback(%d) = (%d, %v), want (%d, %v)",
				tc.in, width, console, tc.wantWidth, tc.wantConsole)
		}
	}
}

func TestConsoleWidthFor_BufferIsNotConsole(t *testing.T) {
	// An injected test writer must never inherit the process's real
	// terminal, even when go test itself runs inside a console.
	width, console := ConsoleWidthFor(&bytes.Buffer{})
	if width != 80 || console {
		t.Errorf("ConsoleWidthFor(&bytes.Buffer{}) = (%d, %v), want (80, false)", width, console)
	}
}

func TestConsoleWidthFor_NilFileIsNotConsole(t *testing.T) {
	var f *os.File
	width, console := ConsoleWidthFor(f)
	if width != 80 || console {
		t.Errorf("ConsoleWidthFor(nil *os.File) = (%d, %v), want (80, false)", width, console)
	}
}

func TestConsoleWidthFor_PipeIsNotConsole(t *testing.T) {
	// A real *os.File that is a pipe exercises the platform query path
	// (not the type-assertion shortcut) without swapping os.Stdout.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	width, console := ConsoleWidthFor(w)
	if width != 80 || console {
		t.Errorf("ConsoleWidthFor(pipe write end) = (%d, %v), want (80, false)", width, console)
	}
}

func TestNewTableWriter_AlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	tw := NewTableWriter(&buf)
	tw.Write([]byte("ID\tNAME\tDL\n"))
	tw.Write([]byte("1\talpha\t10\n"))
	tw.Write([]byte("22\tbeta\t200\n"))
	if err := tw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// ID column = max cell width 2 + padding 2 = 4: the NAME cell must
	// start at index 4 in every row.
	if lines[0][4:8] != "NAME" || lines[1][4:9] != "alpha" || lines[2][4:8] != "beta" {
		t.Errorf("columns misaligned:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "\t") {
		t.Errorf("tabwriter must expand tabs")
	}
}

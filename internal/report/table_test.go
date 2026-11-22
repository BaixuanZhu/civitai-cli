package report

import (
	"bytes"
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

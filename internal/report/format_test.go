package report

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCompactNumber(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{45200, "45.2k"},
		{309445, "309.4k"},
		{1000000, "1.0M"},
		{1234567, "1.2M"},
	}
	for _, tc := range cases {
		if got := CompactNumber(tc.in); got != tc.want {
			t.Errorf("CompactNumber(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDateText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2026-08-24T23:42:31.929Z", "2026-08-24"},
		{"2026-03-12", "2026-03-12"},
		{"", ""},
		{"not-a-date", "not-a-date"},
		{"2026/03/12", "2026/03/12"},
	}
	for _, tc := range cases {
		if got := DateText(tc.in); got != tc.want {
			t.Errorf("DateText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRatioText(t *testing.T) {
	cases := []struct {
		up, down int64
		want     string
	}{
		{356, 2, "178:1"},
		{11548, 38, "304:1"},
		{100, 0, "100:1"},
		{0, 0, "0:1"},
		{1, 3, "0:1"},
	}
	for _, tc := range cases {
		if got := RatioText(tc.up, tc.down); got != tc.want {
			t.Errorf("RatioText(%d,%d) = %q, want %q", tc.up, tc.down, got, tc.want)
		}
	}
}

func TestNSFWLevelText(t *testing.T) {
	cases := []struct {
		in   json.Number
		want string
	}{
		{"", ""},
		{"0", "None"},
		{"1", "None"},
		{"2", "Soft"},
		{"3", "None+Soft"},
		{"4", "Mature"},
		{"7", "None+Soft+Mature"},
		{"8", "X"},
		{"31", "None+Soft+Mature+X+L16"},
		{"16", "L16"},
		{"48", "L16+L32"},
		{"Soft", "Soft"}, // string-form drift preserved as-is
	}
	for _, tc := range cases {
		if got := NSFWLevelText(tc.in); got != tc.want {
			t.Errorf("NSFWLevelText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatSizeKB(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{512, "512.0 KB"},
		{231400, "226.0 MB"},
		{4084209.625, "3.9 GB"},
	}
	for _, tc := range cases {
		if got := FormatSizeKB(tc.in); got != tc.want {
			t.Errorf("FormatSizeKB(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDisplayType(t *testing.T) {
	if got := DisplayType("Checkpoint"); got != "CKPT" {
		t.Errorf("DisplayType(Checkpoint) = %q", got)
	}
	if got := DisplayType("LORA"); got != "LORA" {
		t.Errorf("DisplayType(LORA) = %q", got)
	}
	if got := DisplayType("BrandNewType"); got != "BrandNewType" {
		t.Errorf("unknown type must pass through, got %q", got)
	}
}

func TestWriteRaw_Verbatim(t *testing.T) {
	var buf bytes.Buffer
	raw := []byte(`[{"id":1,"weird":  "spacing"}]`)
	if err := WriteRaw(&buf, raw); err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	want := string(raw) + "\n"
	if buf.String() != want {
		t.Errorf("WriteRaw altered the payload:\n got %q\nwant %q", buf.String(), want)
	}
}

func TestWriteIndent(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteIndent(&buf, map[string]int{"id": 7}); err != nil {
		t.Fatalf("WriteIndent: %v", err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output not valid JSON: %s", buf.String())
	}
	if buf.String() != "{\n  \"id\": 7\n}\n" {
		t.Errorf("unexpected indentation: %q", buf.String())
	}
}

package upgrade

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"civitai-cli/internal/report"
)

// progressFakeClock is a deterministic clock seam for progressWriter tests:
// repaint eligibility and rate math advance only when the test says so.
type progressFakeClock struct{ t time.Time }

func (c *progressFakeClock) now() time.Time          { return c.t }
func (c *progressFakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// progressForceGate pins the TTY gate so tests exercise the rendering or the
// passthrough path deterministically, regardless of whether go test runs
// attached to a real console.
func progressForceGate(t *testing.T, isTTY bool) {
	t.Helper()
	old := progressGate
	progressGate = func() bool { return isTTY }
	t.Cleanup(func() { progressGate = old })
}

// progressCaptureStdout swaps os.Stdout for a pipe and returns a collect
// function yielding everything written while swapped. The gate-forced tests
// decide the rendering path themselves; the capture proves nothing leaks
// onto the real stdout (and would show it if it did).
func progressCaptureStdout(t *testing.T) (collect func() string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })
	return func() string {
		w.Close()
		b, _ := io.ReadAll(r)
		r.Close()
		return string(b)
	}
}

// TestFormatProgress (prd AC1) pins the renderer contract: percent shape
// with a known total, byte/speed-only shape without, integer percent
// rounding, and KB/MB magnitudes matching report.FormatSizeKB exactly.
func TestFormatProgress(t *testing.T) {
	// kb routes through the shared formatter so parity cases derive their
	// size text from the source of truth instead of a hand-copied literal.
	kb := func(b float64) string { return report.FormatSizeKB(b / 1024) }
	tests := []struct {
		name            string
		n, total        int64
		rateBytesPerSec float64
		want            string
	}{
		{"zero percent", 0, 100 * 1024, 0, "0% (0.0 KB/100.0 KB, 0.0 KB/s)"},
		{"mid percent", 2202009, 5242880, 640 * 1024, "42% (2.1 MB/5.0 MB, 640.0 KB/s)"},
		{"complete", 5242880, 5242880, 1024, "100% (5.0 MB/5.0 MB, 1.0 KB/s)"},
		{"unknown total", 1536, 0, 2048, "(1.5 KB, 2.0 KB/s)"},
		{"rounds percent up", 2, 3, 0, "67% (" + kb(2) + "/" + kb(3) + ", 0.0 KB/s)"},
		{"kb magnitude matches report", 400, 4096, 512, "10% (" + kb(400) + "/" + kb(4096) + ", 0.5 KB/s)"},
		{"mb magnitude matches report", 236953600, 473907200, 1024 * 1024,
			"50% (" + kb(236953600) + "/" + kb(473907200) + ", 1.0 MB/s)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatProgress(tt.n, tt.total, tt.rateBytesPerSec); got != tt.want {
				t.Errorf("formatProgress(%d, %d, %v) = %q, want %q",
					tt.n, tt.total, tt.rateBytesPerSec, got, tt.want)
			}
		})
	}
}

// TestProgressWriter_ThrottlesAndFinishes (prd AC3): repaints happen at most
// once per interval (counted via the leading \r) and Finish always forces a
// final repaint plus the terminating newline.
func TestProgressWriter_ThrottlesAndFinishes(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	clock := &progressFakeClock{t: start}
	var dst, out bytes.Buffer
	w := &progressWriter{
		dst:         &dst,
		out:         &out,
		total:       4096,
		now:         clock.now,
		interval:    progressRepaintInterval,
		lastRepaint: start,
	}
	payload := bytes.Repeat([]byte("a"), 4096)

	write := func(p []byte) {
		t.Helper()
		if _, err := w.Write(p); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	repaints := func() int { return strings.Count(out.String(), "\r") }

	write(payload[:2048]) // inside the first window: no repaint yet
	if got := repaints(); got != 0 {
		t.Fatalf("before the interval elapsed: repaints = %d, want 0 (%q)", got, out.String())
	}

	clock.advance(progressRepaintInterval + 10*time.Millisecond)
	write(payload[2048:3072]) // 3072 bytes = 75%
	if got := repaints(); got != 1 {
		t.Fatalf("after one interval: repaints = %d, want 1 (%q)", got, out.String())
	}

	clock.advance(50 * time.Millisecond)
	write(payload[3072:3584]) // still inside the window: throttled
	if got := repaints(); got != 1 {
		t.Fatalf("throttled write: repaints = %d, want 1 (%q)", got, out.String())
	}

	clock.advance(80 * time.Millisecond) // 130 ms since the last repaint
	write(payload[3584:])                // 4096 bytes = 100%
	if got := repaints(); got != 2 {
		t.Fatalf("after the second interval: repaints = %d, want 2 (%q)", got, out.String())
	}

	w.Finish()
	s := out.String()
	if got := repaints(); got != 3 {
		t.Errorf("Finish must force a final repaint: repaints = %d, want 3", got)
	}
	if !strings.HasSuffix(s, "\n") {
		t.Errorf("Finish must terminate the line with \\n, output ends with %q", s[len(s)-1:])
	}
	if i, j := strings.Index(s, "75%"), strings.Index(s, "100%"); !(i >= 0 && j > i) {
		t.Errorf("renders must progress from 75%% to 100%%, output = %q", s)
	}
	if dst.Len() != 4096 {
		t.Errorf("destination received %d bytes, want 4096", dst.Len())
	}
}

// TestProgressWriter_FinishWithoutWrites covers the degenerate attempt: no
// bytes ever arrived, and Finish must still render a zero state and close
// the line (the rate guard must not divide by zero on the un-advanced clock).
func TestProgressWriter_FinishWithoutWrites(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	clock := &progressFakeClock{t: start}
	var out bytes.Buffer
	w := &progressWriter{
		dst:         io.Discard,
		out:         &out,
		total:       8192,
		now:         clock.now,
		interval:    progressRepaintInterval,
		lastRepaint: start,
	}

	w.Finish()
	s := out.String()
	if strings.Count(s, "\r") != 1 {
		t.Errorf("zero-byte Finish: repaints = %d, want 1 (%q)", strings.Count(s, "\r"), s)
	}
	if !strings.HasSuffix(s, "\n") || !strings.Contains(s, "0%") {
		t.Errorf("zero-byte Finish must render 0%% and end with \\n, got %q", s)
	}
}

// TestProgressWriter_NonTTYIsPassthrough: with a false gate the constructor
// returns dst unchanged (identity, not a wrapper), so the copy loop keeps
// zero overhead and cannot emit anything.
func TestProgressWriter_NonTTYIsPassthrough(t *testing.T) {
	dst := &bytes.Buffer{}
	var out bytes.Buffer
	got := newProgressWriter(dst, 100, &out, false)
	if got != io.Writer(dst) {
		t.Fatalf("non-TTY newProgressWriter must return dst unchanged, got %T", got)
	}
	if _, err := got.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if dst.String() != "hello" {
		t.Errorf("destination = %q, want \"hello\"", dst.String())
	}
	if out.Len() != 0 {
		t.Errorf("non-TTY passthrough wrote %d progress bytes, want 0", out.Len())
	}
}

// TestDownloadOnce_NonTTYEmitsNoControlChars (prd AC2): piped stdout keeps
// today's silent shape — the file bytes are exact and nothing, not even a
// carriage return or an escape, reaches stdout.
func TestDownloadOnce_NonTTYEmitsNoControlChars(t *testing.T) {
	progressForceGate(t, false)

	payload := bytes.Repeat([]byte("z"), 64*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Write(payload[:32*1024])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.Write(payload[32*1024:])
	}))
	defer srv.Close()

	collect := progressCaptureStdout(t)
	dest := filepath.Join(t.TempDir(), "asset.zip")
	if err := downloadOnce(srv.URL, dest, int64(len(payload))); err != nil {
		t.Fatalf("downloadOnce failed: %v", err)
	}
	out := collect()

	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, payload) {
		t.Errorf("downloaded content mismatch (err: %v), want the streamed %d bytes", err, len(payload))
	}
	if out != "" {
		t.Errorf("non-TTY download must stay silent, stdout = %q", out)
	}
	if strings.ContainsAny(out, "\r\x1b") {
		t.Errorf("non-TTY stdout must contain no \\r or ESC, got %q", out)
	}
}

// TestDownloadOnce_ProgressCountsFromZeroOnRetry (prd AC4): the first attempt
// fails mid-stream (server declares 8192 bytes but closes after 4096, which
// the client sees as an unexpected EOF), the retry succeeds, and the second
// attempt's meter starts counting from zero — its final render is exactly
// 100% of 8.0 KB, never a carried-over count.
func TestDownloadOnce_ProgressCountsFromZeroOnRetry(t *testing.T) {
	progressForceGate(t, true)

	full := bytes.Repeat([]byte("y"), 8192)
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Length", "8192")
		if n == 1 {
			w.Write(full[:4096]) // half, then the handler returns with the length unfulfilled
			return
		}
		w.Write(full)
	}))
	defer srv.Close()

	collect := progressCaptureStdout(t)
	dest := filepath.Join(t.TempDir(), "asset.zip")
	// retries=1: two attempts, matching the production call's shape.
	if err := downloadFile(srv.URL, dest, 1, int64(len(full))); err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}
	out := collect()

	if n := atomic.LoadInt32(&requests); n != 2 {
		t.Errorf("server saw %d requests, want 2 (one failed attempt plus the retry)", n)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, full) {
		t.Errorf("downloaded content mismatch (err: %v), want the full %d bytes", err, len(full))
	}
	if !strings.Contains(out, "50% (4.0 KB/8.0 KB") {
		t.Errorf("first attempt must render its half-download state, output = %q", out)
	}
	if !strings.Contains(out, "100% (8.0 KB/8.0 KB") {
		t.Errorf("retry must complete at exactly 100%%, output = %q", out)
	}
	// Bleed canary: a meter carrying counts across attempts would render
	// 12288 bytes on the second Finish (150% of 8.0 KB).
	if strings.Contains(out, "150%") || strings.Contains(out, "12.0 KB") {
		t.Errorf("progress state bled across attempts, output = %q", out)
	}
}

// TestDownloadOnce_ProgressTotalFallback (prd AC5): the render's total comes
// from Content-Length when present, falls back to the release asset size
// when the header is absent, and renders byte/speed only when both are
// missing.
func TestDownloadOnce_ProgressTotalFallback(t *testing.T) {
	progressForceGate(t, true)

	payload := bytes.Repeat([]byte("q"), 2048)
	// chunked serves the body without a Content-Length header.
	chunked := func(w http.ResponseWriter) {
		w.Write(payload[:1024])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.Write(payload[1024:])
	}

	t.Run("content length wins", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "2048")
			w.Write(payload)
		}))
		defer srv.Close()

		collect := progressCaptureStdout(t)
		dest := filepath.Join(t.TempDir(), "asset.zip")
		if err := downloadOnce(srv.URL, dest, 4096); err != nil { // 4096 must be ignored
			t.Fatalf("downloadOnce failed: %v", err)
		}
		out := collect()
		if !strings.Contains(out, "100% (2.0 KB/2.0 KB") {
			t.Errorf("Content-Length must drive the total, output = %q", out)
		}
		if strings.Contains(out, "4.0 KB") {
			t.Errorf("expectedSize leaked into a Content-Length download, output = %q", out)
		}
	})

	t.Run("expected size fallback", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			chunked(w)
		}))
		defer srv.Close()

		collect := progressCaptureStdout(t)
		dest := filepath.Join(t.TempDir(), "asset.zip")
		if err := downloadOnce(srv.URL, dest, 2048); err != nil {
			t.Fatalf("downloadOnce failed: %v", err)
		}
		out := collect()
		if !strings.Contains(out, "100% (2.0 KB/2.0 KB") {
			t.Errorf("expectedSize must back the total without Content-Length, output = %q", out)
		}
	})

	t.Run("unknown total", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			chunked(w)
		}))
		defer srv.Close()

		collect := progressCaptureStdout(t)
		dest := filepath.Join(t.TempDir(), "asset.zip")
		if err := downloadOnce(srv.URL, dest, 0); err != nil {
			t.Fatalf("downloadOnce failed: %v", err)
		}
		out := collect()
		if strings.Contains(out, "%") {
			t.Errorf("unknown total must render no percent, output = %q", out)
		}
		if !strings.Contains(out, "\r") || !strings.HasSuffix(out, "\n") || !strings.Contains(out, "(2.0 KB, ") {
			t.Errorf("unknown total must still repaint in place and close the line, output = %q", out)
		}
	})
}

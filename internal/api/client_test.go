package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newFastTestClient builds a client pointed at srv with zero backoffs and
// no jitter: retry tests finish instantly. Same-package tests mutate the
// private fields directly (no test-only exported API, per design D2).
func newFastTestClient(srv *httptest.Server, opts ...Option) *Client {
	c := NewClient(append([]Option{WithBaseURL(srv.URL)}, opts...)...)
	c.backoffs = []time.Duration{0, 0, 0}
	c.jitter = 0
	return c
}

func TestClient_UserAgentAndBearer(t *testing.T) {
	var gotUA, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv, WithToken("secret-key"))
	if _, err := c.Get(context.Background(), "/models", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.HasPrefix(gotUA, "civ/") || !strings.Contains(gotUA, "civitai-cli") {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestClient_AnonymousSendsNoAuthHeader(t *testing.T) {
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasAuth = r.Header["Authorization"]
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	if _, err := c.Get(context.Background(), "/models", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hasAuth {
		t.Errorf("anonymous client must not send Authorization")
	}
}

func TestClient_RetriesTransportError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			// Hijack and close without a response: a transport error.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("server does not support hijacking")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	body, err := c.Get(context.Background(), "/models", nil)
	if err != nil {
		t.Fatalf("Get should succeed after retries: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %s", body)
	}
	if calls.Load() != 3 {
		t.Errorf("attempts = %d, want 3 (2 failures + success)", calls.Load())
	}
}

func TestClient_RetriesEmptyBody(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusOK) // 200 with empty body: retryable
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	if _, err := c.Get(context.Background(), "/models", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("attempts = %d, want 2", calls.Load())
	}
}

func TestClient_Retries429WithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	if _, err := c.Get(context.Background(), "/models", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("attempts = %d, want 2", calls.Load())
	}
}

func TestClient_Retries5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	if _, err := c.Get(context.Background(), "/models", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("attempts = %d, want 3", calls.Load())
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{400, `{"error":{"name":"ZodError","message":"[{\"code\":\"too_big\",\"message\":\"Too big\",\"path\":[\"limit\"]}]"}}`, func(err error) bool { var e *ApiError; return errors.As(err, &e) }},
		{401, `{"error":"Unauthorized"}`, func(err error) bool { var e *UnauthorizedError; return errors.As(err, &e) }},
		{403, `{"error":"Forbidden"}`, func(err error) bool { var e *ApiError; return errors.As(err, &e) }},
		{404, `{"error":"Model not found"}`, func(err error) bool { var e *NotFoundError; return errors.As(err, &e) }},
	}
	for _, tc := range cases {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		c := newFastTestClient(srv)
		_, err := c.Get(context.Background(), "/models", nil)
		srv.Close()
		if err == nil || !tc.check(err) {
			t.Errorf("status %d: wrong error type: %v", tc.status, err)
		}
		if calls.Load() != 1 {
			t.Errorf("status %d: attempts = %d, want 1 (4xx is deterministic)", tc.status, calls.Load())
		}
	}
}

func TestClient_RetryExhaustion(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	_, err := c.Get(context.Background(), "/models", nil)
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("expected NetworkError, got %T: %v", err, err)
	}
	if ne.Attempts != 4 {
		t.Errorf("attempts = %d, want 4 (initial + 3 retries)", ne.Attempts)
	}
	if calls.Load() != 4 {
		t.Errorf("server calls = %d, want 4", calls.Load())
	}
}

func TestClient_ConcurrencyGate(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inFlight.Add(1)
		for {
			old := maxInFlight.Load()
			if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
				break
			}
		}
		<-release // hold the request until the test releases it
		inFlight.Add(-1)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv, WithConcurrency(2))
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), "/models", nil); err != nil {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	// Wait until the gate is saturated, then verify the cap held.
	deadline := time.Now().Add(5 * time.Second)
	for inFlight.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // give extra goroutines a chance to leak past the gate
	if got := maxInFlight.Load(); got > 2 {
		t.Errorf("max in-flight = %d, gate cap 2 violated", got)
	}
	close(release)
	wg.Wait()
}

func TestClient_ConcurrencyClamped(t *testing.T) {
	c := NewClient(WithConcurrency(99))
	if cap(c.sem) != maxConcurrency {
		t.Errorf("concurrency = %d, want clamp to %d", cap(c.sem), maxConcurrency)
	}
	c = NewClient(WithConcurrency(0))
	if cap(c.sem) != 1 {
		t.Errorf("concurrency = %d, want clamp to 1", cap(c.sem))
	}
}

func TestClient_BodyLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write(make([]byte, maxBodyBytes+2))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	_, err := c.Get(context.Background(), "/models", nil)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected body-limit error, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("overflow must be fatal (no retry), attempts = %d", calls.Load())
	}
}

func TestClient_URLEncoding(t *testing.T) {
	var gotQuery url.Values
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	q := url.Values{}
	q.Set("sort", "Most Downloaded")
	q.Set("query", "stocking & pantyhose")
	q.Add("types", "LORA")
	q.Add("types", "LoCon")
	if _, err := c.Get(context.Background(), "/models", q); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotPath != "/models" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery.Get("sort") != "Most Downloaded" {
		t.Errorf("sort not encoded/decoded correctly: %q", gotQuery.Get("sort"))
	}
	if gotQuery.Get("query") != "stocking & pantyhose" {
		t.Errorf("query = %q", gotQuery.Get("query"))
	}
	if got := gotQuery["types"]; len(got) != 2 || got[0] != "LORA" || got[1] != "LoCon" {
		t.Errorf("types repeated params = %v", got)
	}
}

func TestClient_NilQueryOmitsQuestionMark(t *testing.T) {
	var rawURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawURI = r.URL.RequestURI()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newFastTestClient(srv)
	if _, err := c.Get(context.Background(), "/models/123", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rawURI != "/models/123" {
		t.Errorf("request URI = %q, want no query separator", rawURI)
	}
}

func TestClient_ContextCanceled(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newFastTestClient(srv)
	_, err := c.Get(ctx, "/models", nil)
	if err == nil {
		t.Fatalf("expected error from canceled context")
	}
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("expected NetworkError wrapping ctx cause, got %T: %v", err, err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("2"); d != 2*time.Second {
		t.Errorf("seconds form = %v", d)
	}
	if d := parseRetryAfter("0"); d != 0 {
		t.Errorf("zero = %v", d)
	}
	if d := parseRetryAfter("-5"); d != 0 {
		t.Errorf("negative = %v", d)
	}
	future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d < 8*time.Second || d > 11*time.Second {
		t.Errorf("http-date form = %v", d)
	}
	past := time.Now().Add(-10 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(past); d != 0 {
		t.Errorf("past date = %v", d)
	}
	if d := parseRetryAfter("garbage"); d != 0 {
		t.Errorf("garbage = %v", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Errorf("empty = %v", d)
	}
}

func TestBackoffFor_JitterBoundsAndCap(t *testing.T) {
	c := NewClient()
	for i := 0; i < 200; i++ {
		for idx, base := range c.backoffs {
			w := c.backoffFor(idx, "")
			lo := time.Duration(float64(base) * 0.8)
			hi := time.Duration(float64(base) * 1.2)
			if w < lo || w > hi {
				t.Fatalf("jitter out of ±20%%: %v not in [%v,%v]", w, lo, hi)
			}
		}
	}
	// Retry-After overrides the backoff, capped at 30s.
	if w := c.backoffFor(0, "3"); w != 3*time.Second {
		t.Errorf("Retry-After override = %v", w)
	}
	if w := c.backoffFor(0, "9999"); w != maxRetryAfterWait {
		t.Errorf("Retry-After cap = %v", w)
	}
	// Backoff cap at 8s even with absurd configured values.
	c.backoffs = []time.Duration{time.Minute}
	if w := c.backoffFor(0, ""); w > maxRetryWait {
		t.Errorf("backoff cap violated: %v", w)
	}
}

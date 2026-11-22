package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"civitai-cli/internal/app"
)

// Client behavior constants (parent design D2).
const (
	defaultBaseURL     = "https://civitai.com/api/v1"
	defaultTimeout     = 30 * time.Second
	defaultConcurrency = 3
	maxConcurrency     = 8
	maxBodyBytes       = 5 << 20 // 5 MB; read limit is +1 to detect overflow
	maxRetryWait       = 8 * time.Second
	maxRetryAfterWait  = 30 * time.Second // sanity cap on server-provided waits
	defaultJitter      = 0.2              // ±20% on each backoff
)

// Client is the Civitai API client. Get is the ONLY outbound choke point:
// every endpoint method funnels through it, so retry, concurrency gating,
// auth, User-Agent, and the body limit apply uniformly. A Client is safe
// for concurrent use; the semaphore bounds in-flight requests.
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
	userAgent  string
	sem        chan struct{}   // concurrency gate, capacity = concurrency
	backoffs   []time.Duration // wait before retry N; tests shorten directly
	jitter     float64         // fractional jitter applied to backoffs
}

// Option configures a Client at construction (functional options pattern).
type Option func(*Client)

// WithBaseURL overrides the API base URL (tests point it at httptest).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// WithHTTPClient replaces the underlying http.Client. The caller keeps
// ownership; its Timeout is used as-is.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithToken sets the Bearer token. Empty means anonymous (the default;
// the read surface needs no token).
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithTimeout sets the per-request HTTP timeout on the default client.
// It has no effect when combined with WithHTTPClient (order-dependent).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.httpClient.Timeout = d
		}
	}
}

// WithConcurrency sets the in-flight request gate (default 3, clamped
// to 1..8 — well under the 20-concurrent level verified safe live).
func WithConcurrency(n int) Option {
	return func(c *Client) {
		if n < 1 {
			n = 1
		}
		if n > maxConcurrency {
			n = maxConcurrency
		}
		c.sem = make(chan struct{}, n)
	}
}

// WithBackoffs replaces the retry wait sequence. The number of retries
// equals len(backoffs); the default {500ms, 1s, 2s} means one initial
// attempt plus three retries.
func WithBackoffs(ds ...time.Duration) Option {
	return func(c *Client) { c.backoffs = append([]time.Duration(nil), ds...) }
}

// NewClient builds a Client with production defaults: 30s timeout,
// anonymous, concurrency 3, backoffs {500ms, 1s, 2s} with ±20% jitter.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: defaultTimeout},
		baseURL:    defaultBaseURL,
		userAgent:  app.UserAgent(),
		sem:        make(chan struct{}, defaultConcurrency),
		backoffs:   []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second},
		jitter:     defaultJitter,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// fetchOutcome is the raw result of one attempt, before retry
// classification. fatal marks failures retrying cannot fix (request
// construction, body overflow, deterministic 4xx answers).
type fetchOutcome struct {
	body       []byte
	status     int
	retryAfter string
	err        error
	fatal      bool
}

// Get fetches path (e.g. "/models" or "/models/2458426") with optional
// query values and returns the raw response body. Query strings are
// always built via url.Values.Encode — hand-concatenated URLs caused
// silent empty responses for enum values containing spaces (verified
// live). Non-2xx answers become typed errors (see errors.go); transient
// failures are retried per the backoff sequence.
func (c *Client) Get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	// Cancellation is a network-class failure (exit 2) on every path,
	// including before any attempt was made.
	if err := ctx.Err(); err != nil {
		return nil, &NetworkError{Err: err}
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, &NetworkError{Err: fmt.Errorf("api: waiting for concurrency slot: %w", ctx.Err())}
	}

	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	app.Debugf("GET %s", u)
	return c.doWithRetry(ctx, u)
}

// doWithRetry runs the attempt loop: initial attempt plus len(backoffs)
// retries. Only transientError causes are retried; fatal outcomes return
// immediately. Every wait honors ctx cancellation.
func (c *Client) doWithRetry(ctx context.Context, u string) ([]byte, error) {
	var lastErr error
	attempts := 0
	for {
		attempts++
		out := c.doOnce(ctx, u)
		if out.err == nil {
			return out.body, nil
		}
		if out.fatal {
			return nil, out.err
		}
		var te *transientError
		if !errors.As(out.err, &te) {
			return nil, out.err
		}
		lastErr = out.err
		if attempts > len(c.backoffs) {
			break
		}
		wait := c.backoffFor(attempts-1, out.retryAfter)
		app.Infof("retry %d/%d in %s: %v", attempts, len(c.backoffs), wait.Round(time.Millisecond), lastErr)
		select {
		case <-ctx.Done():
			return nil, &NetworkError{Attempts: attempts, Err: ctx.Err()}
		case <-time.After(wait):
		}
	}
	return nil, &NetworkError{Attempts: attempts, Err: lastErr}
}

// backoffFor computes the wait before retry index i: the configured
// backoff with ±jitter, capped at 8s; a valid Retry-After header (seconds
// or HTTP-date) overrides the wait, capped at 30s for sanity.
func (c *Client) backoffFor(i int, retryAfter string) time.Duration {
	wait := c.backoffs[i]
	if c.jitter > 0 {
		delta := (rand.Float64()*2 - 1) * c.jitter
		wait = time.Duration(float64(wait) * (1 + delta))
	}
	if wait > maxRetryWait {
		wait = maxRetryWait
	}
	if wait < 0 {
		wait = 0
	}
	if ra := parseRetryAfter(retryAfter); ra > 0 {
		if ra > maxRetryAfterWait {
			ra = maxRetryAfterWait
		}
		wait = ra
	}
	return wait
}

// parseRetryAfter interprets a Retry-After header value (delta-seconds
// or HTTP-date). Invalid or past values yield 0 (use the normal backoff).
func parseRetryAfter(s string) time.Duration {
	if s == "" {
		return 0
	}
	if secs, err := strconv.Atoi(s); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(s); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// doOnce performs a single HTTP attempt and classifies the outcome.
// Retryable: transport errors, empty 200 bodies (the silent-empty-response
// pitfall), 429, 5xx. Deterministic: 400/401/403/404 and body overflow.
func (c *Client) doOnce(ctx context.Context, u string) fetchOutcome {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fetchOutcome{err: fmt.Errorf("api: build request: %w", err), fatal: true}
	}
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fetchOutcome{err: &transientError{err: fmt.Errorf("api: request failed: %w", err)}}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return fetchOutcome{err: &transientError{err: fmt.Errorf("api: read body: %w", err)}}
	}
	if len(body) > maxBodyBytes {
		return fetchOutcome{err: fmt.Errorf("api: response body exceeds %d-byte limit", maxBodyBytes), fatal: true}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if len(body) == 0 {
			return fetchOutcome{err: &transientError{err: errors.New("api: empty response body")}}
		}
		return fetchOutcome{body: body, status: resp.StatusCode}
	}

	apiErr := classifyHTTPError(resp.StatusCode, body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fetchOutcome{
			status:     resp.StatusCode,
			retryAfter: resp.Header.Get("Retry-After"),
			err:        &transientError{err: apiErr},
		}
	}
	return fetchOutcome{status: resp.StatusCode, err: apiErr, fatal: true}
}

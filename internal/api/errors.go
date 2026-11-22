package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// maxDetailBytes caps raw error-body snippets kept in error messages so a
// CDN HTML error page cannot flood the terminal (design D3).
const maxDetailBytes = 300

// ApiError is a deterministic request rejection: HTTP 400 parameter
// validation (ZodError) and other non-2xx statuses except 401/404.
// Detail carries the Zod issue summary when available, otherwise a raw
// body snippet. A 400 usually means a CLI-side bug or upstream API drift.
type ApiError struct {
	Status int    // HTTP status code
	Detail string // Zod summary or raw body snippet
}

// Error renders the rejection with its HTTP status and detail summary.
func (e *ApiError) Error() string {
	return fmt.Sprintf("api: request rejected (HTTP %d): %s", e.Status, e.Detail)
}

// UnauthorizedError is HTTP 401. With a token it means the token is
// invalid/expired (exit 4); anonymous 401 only happens for personal-state
// parameters (favorites/hidden) and is a usage-class error needing a token.
type UnauthorizedError struct {
	Detail string // server error message, e.g. "Unauthorized"
}

// Error renders the 401 failure.
func (e *UnauthorizedError) Error() string {
	return "api: unauthorized (HTTP 401): " + e.Detail
}

// NotFoundError is HTTP 404 (unknown model/version/hash). Commands map it
// by their own semantics: vet (M2) exits 3; M1 commands treat it as a
// usage-class error (exit 1).
type NotFoundError struct {
	Detail string // server error message, e.g. "Model not found"
}

// Error renders the 404 failure.
func (e *NotFoundError) Error() string {
	return "api: not found (HTTP 404): " + e.Detail
}

// NetworkError means retries were exhausted on transient failures
// (transport error, empty 200 body, 429, 5xx) or the context was canceled
// while waiting. Exit code 2.
type NetworkError struct {
	Attempts int   // total attempts made (initial + retries)
	Err      error // last transient cause
}

// Error renders the exhausted-retry failure with its attempt count.
func (e *NetworkError) Error() string {
	return fmt.Sprintf("api: network failure after %d attempt(s): %v", e.Attempts, e.Err)
}

// Unwrap exposes the last transient cause for errors.Is/As inspection.
func (e *NetworkError) Unwrap() error { return e.Err }

// transientError marks a retryable failure (transport error, empty body,
// 429, 5xx). It is internal to the retry loop; callers see NetworkError
// once retries are exhausted.
type transientError struct{ err error }

// Error renders the underlying transient cause.
func (e *transientError) Error() string { return e.err.Error() }

// Unwrap exposes the underlying cause.
func (e *transientError) Unwrap() error { return e.err }

// classifyHTTPError maps a non-2xx response to its typed error, extracting
// a human-readable detail from the body (both API error shapes plus the
// plain {"message": "..."} shape used by some 400s).
func classifyHTTPError(status int, body []byte) error {
	detail := errorDetail(body)
	switch status {
	case 401:
		return &UnauthorizedError{Detail: detail}
	case 404:
		return &NotFoundError{Detail: detail}
	default:
		return &ApiError{Status: status, Detail: detail}
	}
}

// errorDetail extracts a display summary from an error body. Shapes
// (verified live 2026-11-22, see testdata/err_*.json and probes/):
//
//	{"error":"Model not found"}                          → string form
//	{"error":{"name":"ZodError","message":"[{...}]"}}    → Zod summary
//	{"message":"Recently Added sort requires ..."}       → plain message
//	anything else (HTML, empty)                          → 300-byte snippet
func errorDetail(body []byte) string {
	var wrap struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &wrap); err == nil {
		if len(wrap.Error) > 0 {
			var s string
			if json.Unmarshal(wrap.Error, &s) == nil {
				return s
			}
			var z struct {
				Name    string `json:"name"`
				Message string `json:"message"`
			}
			if json.Unmarshal(wrap.Error, &z) == nil && z.Name == "ZodError" {
				if sum := zodSummary(z.Message); sum != "" {
					return sum
				}
				return truncateDetail(z.Message)
			}
			return truncateDetail(string(wrap.Error))
		}
		if wrap.Message != "" {
			return wrap.Message
		}
	}
	return truncateDetail(string(body))
}

// zodIssue is one entry of the JSON array embedded in ZodError.message.
// Values is []any because enum options may be strings or numbers; Errors
// carries nested issues for invalid_union (e.g. the types param union of
// single enum value and array).
type zodIssue struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Values  []any        `json:"values"`
	Path    []any        `json:"path"`
	Errors  [][]zodIssue `json:"errors"`
}

// zodSummary condenses the first Zod issue into a single line: the issue
// message (or the leaked valid values when no message is present), plus
// the parameter path. For invalid_union it descends into the first nested
// issue, which carries the concrete enum leak.
func zodSummary(message string) string {
	var issues []zodIssue
	if err := json.Unmarshal([]byte(message), &issues); err != nil || len(issues) == 0 {
		return ""
	}
	iss := issues[0]
	path := iss.Path
	if len(iss.Errors) > 0 {
		for _, group := range iss.Errors {
			if len(group) > 0 {
				iss = group[0]
				break
			}
		}
		if len(iss.Path) == 0 {
			iss.Path = path
		}
	}
	var parts []string
	if iss.Message != "" {
		parts = append(parts, iss.Message)
	} else if len(iss.Values) > 0 {
		vals := make([]string, len(iss.Values))
		for i, v := range iss.Values {
			vals[i] = fmt.Sprint(v)
		}
		parts = append(parts, "expected one of: "+strings.Join(vals, ", "))
	}
	if len(iss.Path) > 0 {
		segs := make([]string, len(iss.Path))
		for i, p := range iss.Path {
			segs[i] = fmt.Sprint(p)
		}
		parts = append(parts, "at "+strings.Join(segs, "."))
	}
	return strings.Join(parts, "; ")
}

// truncateDetail caps s at maxDetailBytes on a rune boundary and marks
// the cut with "...".
func truncateDetail(s string) string {
	if len(s) <= maxDetailBytes {
		return s
	}
	s = s[:maxDetailBytes]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "..."
}

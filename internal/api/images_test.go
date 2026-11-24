package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// imagesRecorder is an httptest server that records every request path and
// decoded query so list/detail tests can pin exact request shapes and
// prove boundedness (exactly one logical request, no follow-up).
type imagesRecorder struct {
	mu      sync.Mutex
	srv     *httptest.Server
	paths   []string
	queries []url.Values
	raws    []string
}

// newImagesRecorder starts a recorder backed by handler. The server and
// its client are cleaned up with the test.
func newImagesRecorder(t *testing.T, handler http.HandlerFunc) *imagesRecorder {
	t.Helper()
	rec := &imagesRecorder{}
	rec.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.paths = append(rec.paths, r.URL.Path)
		rec.queries = append(rec.queries, r.URL.Query())
		rec.raws = append(rec.raws, r.URL.RawQuery)
		rec.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(rec.srv.Close)
	return rec
}

// newImagesBodyRecorder serves one fixed body for every request.
func newImagesBodyRecorder(t *testing.T, body string) *imagesRecorder {
	t.Helper()
	return newImagesRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
}

func (r *imagesRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.paths)
}

func (r *imagesRecorder) path(i int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paths[i]
}

func (r *imagesRecorder) query(i int) url.Values {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.queries[i]
}

func (r *imagesRecorder) raw(i int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.raws[i]
}

// assertExactQuery fails unless the query carries exactly the wanted keys
// and values. Exactness is the point: a stray nsfw, page, cursor, sort, or
// filter parameter changes the sample.
func assertExactQuery(t *testing.T, got url.Values, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("query has %d keys, want exactly %d: got %v", len(got), len(want), got)
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Errorf("param %s = %q, want %q", key, got.Get(key), value)
		}
	}
}

// assertNoQueryParams fails when any forbidden parameter is present.
func assertNoQueryParams(t *testing.T, got url.Values, forbidden ...string) {
	t.Helper()
	for _, key := range forbidden {
		if got.Has(key) {
			t.Errorf("forbidden query param %s present: %v", key, got)
		}
	}
}

// TestListImages_ExactQuery pins the one bounded list request: exact path,
// approved params only, and percent-encoded spaces for "Most Reactions".
func TestListImages_ExactQuery(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		sort  string
	}{
		{"defaults", CommunityDefaultLimit, CommunitySortMostReactions},
		{"newest explicit limit", 5, CommunitySortNewest},
		{"minimum limit", 1, CommunitySortMostReactions},
		{"maximum limit", 100, CommunitySortNewest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newImagesBodyRecorder(t, `{"items":[]}`)
			c := newFastTestClient(rec.srv)
			if _, err := c.ListImages(context.Background(), 3263843, tc.limit, tc.sort); err != nil {
				t.Fatalf("ListImages: %v", err)
			}
			if rec.count() != 1 {
				t.Fatalf("requests = %d, want exactly 1", rec.count())
			}
			if rec.path(0) != "/images" {
				t.Errorf("path = %q, want /images", rec.path(0))
			}
			assertExactQuery(t, rec.query(0), map[string]string{
				"modelVersionId": "3263843",
				"withMeta":       "true",
				"type":           "image",
				"limit":          strconv.Itoa(tc.limit),
				"sort":           tc.sort,
				"period":         "AllTime",
			})
			assertNoQueryParams(t, rec.query(0), "modelId", "page", "cursor", "nsfw", "browsingLevel")
			if strings.Contains(rec.raw(0), " ") {
				t.Errorf("raw query contains an unencoded space: %q", rec.raw(0))
			}
		})
	}
}

// itoaForTest renders a small positive int without importing strconv in
// every table case.
func itoaForTest(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 3)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestListImages_ValidationBeforeNetwork pins local-only validation: no
// request is made for an invalid version id, limit, or sort.
func TestListImages_ValidationBeforeNetwork(t *testing.T) {
	cases := []struct {
		name    string
		version int
		limit   int
		sort    string
	}{
		{"zero version", 0, 20, CommunitySortMostReactions},
		{"negative version", -1, 20, CommunitySortMostReactions},
		{"zero limit", 3263843, 0, CommunitySortMostReactions},
		{"negative limit", 3263843, -1, CommunitySortMostReactions},
		{"limit above max", 3263843, 101, CommunitySortMostReactions},
		{"unsupported sort", 3263843, 20, "Random"},
		{"empty sort", 3263843, 20, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newImagesBodyRecorder(t, `{"items":[]}`)
			c := newFastTestClient(rec.srv)
			if _, err := c.ListImages(context.Background(), tc.version, tc.limit, tc.sort); err == nil {
				t.Fatalf("%s: expected an error", tc.name)
			}
			if rec.count() != 0 {
				t.Errorf("%s: must fail before network, got %d requests", tc.name, rec.count())
			}
		})
	}
}

// TestGetImage_ExactQuery pins the independent exact-image request: only
// imageId, withMeta, type, and limit=1.
func TestGetImage_ExactQuery(t *testing.T) {
	rec := newImagesBodyRecorder(t, `{"items":[]}`)
	c := newFastTestClient(rec.srv)
	if _, err := c.GetImage(context.Background(), 1001); err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	if rec.count() != 1 {
		t.Fatalf("requests = %d, want exactly 1", rec.count())
	}
	if rec.path(0) != "/images" {
		t.Errorf("path = %q, want /images", rec.path(0))
	}
	assertExactQuery(t, rec.query(0), map[string]string{
		"imageId":  "1001",
		"withMeta": "true",
		"type":     "image",
		"limit":    "1",
	})
	assertNoQueryParams(t, rec.query(0),
		"sort", "period", "modelVersionId", "modelId", "page", "cursor", "nsfw", "browsingLevel")
}

// TestGetImage_ValidationBeforeNetwork rejects non-positive image ids
// before any request.
func TestGetImage_ValidationBeforeNetwork(t *testing.T) {
	for _, id := range []int{0, -1} {
		rec := newImagesBodyRecorder(t, `{"items":[]}`)
		c := newFastTestClient(rec.srv)
		if _, err := c.GetImage(context.Background(), id); err == nil {
			t.Fatalf("image id %d must fail", id)
		}
		if rec.count() != 0 {
			t.Errorf("image id %d: must fail before network", id)
		}
	}
}

// TestImages_HTTPErrorClassification keeps the shared error classes for
// the browse endpoint: 404 is a deterministic NotFoundError and 401 an
// UnauthorizedError, each observed with one request.
func TestImages_HTTPErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"not found", http.StatusNotFound, `{"error":"not found"}`, func(err error) bool {
			var nfe *NotFoundError
			return errors.As(err, &nfe)
		}},
		{"unauthorized", http.StatusUnauthorized, `{"error":"unauthorized"}`, func(err error) bool {
			var ue *UnauthorizedError
			return errors.As(err, &ue)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newImagesRecorder(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			})
			c := newFastTestClient(rec.srv)
			_, err := c.GetImage(context.Background(), 1001)
			if err == nil || !tc.check(err) {
				t.Fatalf("error = %v, want the shared %s class", err, tc.name)
			}
			if rec.count() != 1 {
				t.Errorf("requests = %d, want one non-retried request", rec.count())
			}
		})
	}
}

// TestDecodeImagesEnvelope pins tolerant envelope handling: unknown
// members are ignored, absent/null items are a successful empty sample,
// and non-array items or non-object bodies are errors.
func TestDecodeImagesEnvelope(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantLen int
		wantErr bool
	}{
		{"items array", `{"items":[{"id":1},{"id":2}],"extra":true}`, 2, false},
		{"empty array", `{"items":[]}`, 0, false},
		{"absent items", `{}`, 0, false},
		{"null items", `{"items":null}`, 0, false},
		{"items object", `{"items":{}}`, 0, true},
		{"items string", `{"items":"nope"}`, 0, true},
		{"array body", `[]`, 0, true},
		{"null body", `null`, 0, true},
		{"number body", `5`, 0, true},
		{"malformed body", `{"items":`, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := decodeImagesEnvelope([]byte(tc.body))
			if err == nil {
				items, itemsErr := decodeImagesItems(env)
				if itemsErr != nil {
					err = itemsErr
				} else {
					if items == nil {
						t.Errorf("%s: items must be non-nil", tc.name)
					}
					if len(items) != tc.wantLen {
						t.Errorf("%s: items = %d, want %d", tc.name, len(items), tc.wantLen)
					}
				}
			}
			if tc.wantErr && err == nil {
				t.Fatalf("%s: expected an envelope error", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
		})
	}
}

// TestContinuationOffered pins presence-only continuation detection. The
// opaque token is never exposed and an empty string does not claim an
// offered page.
func TestContinuationOffered(t *testing.T) {
	cases := []struct {
		name     string
		metadata string
		want     bool
	}{
		{"next cursor", `{"nextCursor":"abc"}`, true},
		{"next page", `{"nextPage":"https://example.test/next"}`, true},
		{"both", `{"nextCursor":"abc","nextPage":"next"}`, true},
		{"object cursor", `{"nextCursor":{"opaque":true}}`, true},
		{"empty metadata", `{}`, false},
		{"null metadata", `null`, false},
		{"absent metadata", ``, false},
		{"string metadata", `"nope"`, false},
		{"empty cursor", `{"nextCursor":""}`, false},
		{"blank cursor", `{"nextCursor":"   "}`, false},
		{"null cursor", `{"nextCursor":null,"nextPage":null}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := continuationOffered(jsonRawOrNil(tc.metadata))
			if got != tc.want {
				t.Errorf("continuationOffered(%s) = %v, want %v", tc.metadata, got, tc.want)
			}
		})
	}
}

// jsonRawOrNil converts an empty test literal to a nil RawMessage so the
// absent member case is exercised rather than an empty JSON value.
func jsonRawOrNil(s string) []byte {
	if s == "" {
		return nil
	}
	return []byte(s)
}

// TestListImages_ContinuationDoesNotFanOut proves a response that offers
// both continuation forms still causes exactly one request: nothing
// follows the opaque token.
func TestListImages_ContinuationDoesNotFanOut(t *testing.T) {
	rec := newImagesBodyRecorder(t, `{"items":[{"id":1}],"metadata":{"nextCursor":"secret-cursor","nextPage":"https://example.test/page2"}}`)
	c := newFastTestClient(rec.srv)
	if _, err := c.ListImages(context.Background(), 3263843, 20, CommunitySortMostReactions); err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if rec.count() != 1 {
		t.Fatalf("requests = %d, want exactly 1 (never follow continuation)", rec.count())
	}
}

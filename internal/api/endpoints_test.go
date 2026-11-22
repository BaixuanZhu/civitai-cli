package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// recordingServer captures the last request's path and decoded query.
type recordingServer struct {
	srv  *httptest.Server
	path string
	q    url.Values
	raw  string
}

func newRecordingServer(body string) *recordingServer {
	rs := &recordingServer{}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.path = r.URL.Path
		rs.q = r.URL.Query()
		rs.raw = r.URL.RawQuery
		w.Write([]byte(body))
	}))
	return rs
}

func TestSearchModels_QueryConstruction(t *testing.T) {
	rs := newRecordingServer(`{"items":[]}`)
	defer rs.srv.Close()
	c := newFastTestClient(rs.srv)

	p := SearchParams{
		Query:      "stocking",
		Types:      []string{"LORA", "LoCon"},
		BaseModels: []string{"Illustrious"},
		Tag:        "anime",
		Username:   "someone",
		Sort:       "Most Downloaded",
		Period:     "Month",
		Limit:      20,
		Cursor:     "2",
	}
	if _, err := c.SearchModels(context.Background(), p); err != nil {
		t.Fatalf("SearchModels: %v", err)
	}
	if rs.path != "/models" {
		t.Errorf("path = %q", rs.path)
	}
	checks := map[string]string{
		"query":    "stocking",
		"tag":      "anime",
		"username": "someone",
		"sort":     "Most Downloaded",
		"period":   "Month",
		"limit":    "20",
		"cursor":   "2",
	}
	for k, want := range checks {
		if got := rs.q.Get(k); got != want {
			t.Errorf("param %s = %q, want %q", k, got, want)
		}
	}
	if got := rs.q["types"]; len(got) != 2 || got[0] != "LORA" || got[1] != "LoCon" {
		t.Errorf("types must be repeated params, got %v", got)
	}
	if got := rs.q["baseModels"]; len(got) != 1 || got[0] != "Illustrious" {
		t.Errorf("baseModels (plural) = %v", got)
	}
	if rs.q.Has("baseModel") {
		t.Errorf("singular baseModel param is dead upstream and must not be sent")
	}
	// Spaces must be percent/plus-encoded, never raw (silent-empty pitfall).
	if containsRawSpace(rs.raw) {
		t.Errorf("raw query contains unencoded space: %q", rs.raw)
	}
}

func containsRawSpace(raw string) bool {
	for _, c := range raw {
		if c == ' ' {
			return true
		}
	}
	return false
}

func TestSearchModels_OmitsEmptyParams(t *testing.T) {
	rs := newRecordingServer(`{"items":[]}`)
	defer rs.srv.Close()
	c := newFastTestClient(rs.srv)

	if _, err := c.SearchModels(context.Background(), SearchParams{Query: "anima"}); err != nil {
		t.Fatalf("SearchModels: %v", err)
	}
	if len(rs.q) != 1 || rs.q.Get("query") != "anima" {
		t.Errorf("empty params must be omitted, got %v", rs.q)
	}
}

func TestGetModel_Path(t *testing.T) {
	rs := newRecordingServer(`{"id":2458426}`)
	defer rs.srv.Close()
	c := newFastTestClient(rs.srv)

	if _, err := c.GetModel(context.Background(), 2458426); err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if rs.path != "/models/2458426" {
		t.Errorf("path = %q", rs.path)
	}
}

func TestGetVersion_Path(t *testing.T) {
	rs := newRecordingServer(`{"id":3263843}`)
	defer rs.srv.Close()
	c := newFastTestClient(rs.srv)

	if _, err := c.GetVersion(context.Background(), 3263843); err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if rs.path != "/model-versions/3263843" {
		t.Errorf("path = %q", rs.path)
	}
}

func TestGetVersionByHash_PathEscaped(t *testing.T) {
	rs := newRecordingServer(`{"id":3263843}`)
	defer rs.srv.Close()
	c := newFastTestClient(rs.srv)

	hash := "FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB"
	if _, err := c.GetVersionByHash(context.Background(), hash); err != nil {
		t.Fatalf("GetVersionByHash: %v", err)
	}
	if rs.path != "/model-versions/by-hash/"+hash {
		t.Errorf("path = %q", rs.path)
	}
}

func TestListTags_QueryConstruction(t *testing.T) {
	rs := newRecordingServer(`{"items":[{"name":"anime"}]}`)
	defer rs.srv.Close()
	c := newFastTestClient(rs.srv)

	if _, err := c.ListTags(context.Background(), "ani", 20); err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if rs.path != "/tags" {
		t.Errorf("path = %q", rs.path)
	}
	if rs.q.Get("query") != "ani" || rs.q.Get("limit") != "20" {
		t.Errorf("query = %v", rs.q)
	}
}

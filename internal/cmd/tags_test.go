package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const tagsBody = `{"items":[{"name":"anime","link":"/tags/anime"},{"name":"anima","link":"/tags/anima"}],"metadata":{}}`

func TestTags_NameList(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(tagsBody))
	})
	if err := runTagsCmd(t, env, "ani"); err != nil {
		t.Fatalf("runTags: %v", err)
	}
	if env.buf.String() != "anime\nanima\n" {
		t.Errorf("names must print one per line, got %q", env.buf.String())
	}
	q := env.query()
	if q.Get("query") != "ani" || q.Get("limit") != "20" {
		t.Errorf("query params = %v", q)
	}
	if env.lastPath != "/tags" {
		t.Errorf("path = %q", env.lastPath)
	}
}

func TestTags_JSONNameArray(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(tagsBody))
	})
	if err := runTagsCmd(t, env, "ani", "--json"); err != nil {
		t.Fatalf("runTags --json: %v", err)
	}
	var names []string
	if err := json.Unmarshal(env.buf.Bytes(), &names); err != nil {
		t.Fatalf("--json must parse as an array: %v", err)
	}
	if len(names) != 2 || names[0] != "anime" || names[1] != "anima" {
		t.Errorf("names = %v", names)
	}
}

func TestTags_LimitValidation(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(tagsBody))
	})
	for _, limit := range []string{"0", "101"} {
		if err := runTagsCmd(t, env, "ani", "--limit", limit); err == nil {
			t.Errorf("limit %s must be rejected", limit)
		}
	}
	if err := runTagsCmd(t, env, "ani", "--limit", "5"); err != nil {
		t.Fatalf("valid limit: %v", err)
	}
	if env.query().Get("limit") != "5" {
		t.Errorf("limit param = %q", env.query().Get("limit"))
	}
}

func TestTags_MissingQuery(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {})
	if err := runTagsCmd(t, env); err == nil {
		t.Errorf("missing query must be rejected")
	}
}

func TestTags_EmptyResults(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[]}`))
	})
	if err := runTagsCmd(t, env, "zzz"); err != nil {
		t.Fatalf("runTags: %v", err)
	}
	if !strings.Contains(env.buf.String(), "No tags found") {
		t.Errorf("empty message missing: %q", env.buf.String())
	}
}

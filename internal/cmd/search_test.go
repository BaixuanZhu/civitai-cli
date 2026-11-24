package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/testutil"
)

func TestSearch_TableFixture(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	if err := runSearchCmd(t, env, "anima"); err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{"ID", "NAME", "CREATOR", "2458426", "Anima", "309.4k", "circle"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	if env.query().Get("query") != "anima" {
		t.Errorf("query param = %q", env.query().Get("query"))
	}
	// Every rendered line must stay within the 80-column fallback budget.
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len([]rune(line)) > 80 {
			t.Errorf("line exceeds 80 columns (%d): %q", len([]rune(line)), line)
		}
	}
}

func TestSearch_JSONViewFullVerbatim(t *testing.T) {
	fixture := testutil.LoadFixture(t, "models_search_anima.json")
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	if err := runSearchCmd(t, env, "anima", "--json", "--view", "full"); err != nil {
		t.Fatalf("runSearch --json --view full: %v", err)
	}
	wantItems, err := api.ModelsRawItems(fixture)
	if err != nil {
		t.Fatalf("extract expected items: %v", err)
	}
	if env.buf.String() != string(wantItems)+"\n" {
		t.Errorf("--view full output is not the verbatim items array")
	}
	var arr []map[string]any
	if err := json.Unmarshal(env.buf.Bytes(), &arr); err != nil || len(arr) != 2 {
		t.Errorf("full output must parse as a 2-element items array: %v", err)
	}
}

func TestSearch_JSONDefaultCompact(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	if err := runSearchCmd(t, env, "anima", "--json"); err != nil {
		t.Fatalf("runSearch --json: %v", err)
	}
	out := env.buf.String()
	if strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("default --json must be the versioned envelope, not the legacy array:\n%s", out)
	}
	var envl struct {
		SchemaVersion string           `json:"schemaVersion"`
		Kind          string           `json:"kind"`
		Items         []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &envl); err != nil {
		t.Fatalf("compact output must parse: %v\n%s", err, out)
	}
	if envl.SchemaVersion != "civ.search/v1" || envl.Kind != "modelSearch" {
		t.Errorf("envelope = %q/%q", envl.SchemaVersion, envl.Kind)
	}
	if len(envl.Items) != 2 {
		t.Fatalf("items = %d, want the same 2 upstream models", len(envl.Items))
	}
	first := envl.Items[0]
	if first["id"] != float64(2458426) || first["name"] != "Anima" {
		t.Errorf("first item identity = %v/%v", first["id"], first["name"])
	}
	versions, ok := first["versions"].([]any)
	if !ok || len(versions) != 8 {
		t.Errorf("first item versions = %v, want all 8 refs", first["versions"])
	}
	for _, excluded := range []string{`"description"`, `"images"`, `"files"`, `"downloadUrl"`} {
		if strings.Contains(out, excluded) {
			t.Errorf("compact search leaks %s", excluded)
		}
	}
}

func TestSearch_JSONExplicitCompactMatchesDefault(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	if err := runSearchCmd(t, env, "anima", "--json"); err != nil {
		t.Fatalf("default compact: %v", err)
	}
	defaultOut := env.buf.String()
	env.buf.Reset()
	if err := runSearchCmd(t, env, "anima", "--json", "--view", "compact"); err != nil {
		t.Fatalf("explicit compact: %v", err)
	}
	if env.buf.String() != defaultOut {
		t.Errorf("explicit --view compact must equal the default output")
	}
}

func TestSearch_JSONCompactEmptyItems(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	if err := runSearchCmd(t, env, "nothingmatches", "--json"); err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	var envl struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &envl); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if envl.Items == nil || len(envl.Items) != 0 {
		t.Errorf("empty upstream items must produce [], got %v", envl.Items)
	}
	if !strings.Contains(env.buf.String(), `"items": []`) {
		t.Errorf("empty items must serialize as []:\n%s", env.buf.String())
	}
}

func TestSearch_ViewValidation(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"view requires json", []string{"anima", "--view", "full"}, "--view requires --json"},
		{"unknown view", []string{"anima", "--json", "--view", "huge"}, "valid values: compact, full"},
		{"empty view", []string{"anima", "--json", "--view="}, `unknown --view ""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, fixtureHandler(t, map[string]string{
				"/models": "models_search_anima.json",
			}))
			err := runSearchCmd(t, env, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
			if exitCodeFor(err, false) != 1 {
				t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
			}
			if env.lastPath != "" {
				t.Errorf("usage error must not send a request, hit %q", env.lastPath)
			}
			if env.buf.Len() != 0 {
				t.Errorf("usage error must keep stdout empty, got %q", env.buf.String())
			}
		})
	}
}

func TestSearch_JSONCompactDecodeFailureNoFallback(t *testing.T) {
	// The legacy full path never decodes items, so it succeeds on this body;
	// the compact contract rejects the invalid numeric text. A decode error
	// must stay an error instead of silently falling back to full output.
	body := []byte(`{"items":[{"id":"oops"}],"metadata":{}}`)
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	})
	if err := runSearchCmd(t, env, "anima", "--json", "--view", "full"); err != nil {
		t.Fatalf("the legacy full path must tolerate the body: %v", err)
	}
	if env.buf.Len() == 0 {
		t.Fatalf("legacy full path produced no output")
	}
	env.buf.Reset()
	if err := runSearchCmd(t, env, "anima", "--json"); err == nil {
		t.Fatalf("invalid numeric text must be a compact decode error")
	}
	if env.buf.Len() != 0 {
		t.Errorf("decode failure must not fall back to full output, got %q", env.buf.String())
	}
}

func TestSearch_ViewRequestParity(t *testing.T) {
	fixture := testutil.LoadFixture(t, "models_search_anima.json")
	for _, view := range []string{"compact", "full"} {
		t.Run(view, func(t *testing.T) {
			hits := 0
			env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.Write(fixture)
			})
			err := runSearchCmd(t, env, "anima", "--json", "--view", view,
				"--type", "lora", "--limit", "5")
			if err != nil {
				t.Fatalf("runSearch --view %s: %v", view, err)
			}
			if hits != 1 {
				t.Errorf("requests = %d, want exactly 1", hits)
			}
			if env.lastPath != "/models" {
				t.Errorf("path = %q, want /models", env.lastPath)
			}
			q := env.query()
			if q.Get("query") != "anima" || q.Get("limit") != "5" || q.Get("types") != "LORA" {
				t.Errorf("query = %v", q)
			}
		})
	}
}

func TestSearch_FlagToQueryMapping(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	err := runSearchCmd(t, env, "stocking",
		"--type", "lora,ti", "--type", "locon",
		"--sort", "most-downloaded", "--period", "month",
		"--base", "illustrious", "--tag", "anime", "--user", "bob", "--limit", "5")
	if err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	q := env.query()
	checks := map[string]string{
		"query":    "stocking",
		"sort":     "Most Downloaded",
		"period":   "Month",
		"tag":      "anime",
		"username": "bob",
		"limit":    "5",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("param %s = %q, want %q", k, got, want)
		}
	}
	if got := q["types"]; len(got) != 3 || got[0] != "LORA" || got[1] != "TextualInversion" || got[2] != "LoCon" {
		t.Errorf("types = %v, want [LORA TextualInversion LoCon]", got)
	}
	if got := q["baseModels"]; len(got) != 1 || got[0] != "Illustrious" {
		t.Errorf("baseModels = %v, want canonical [Illustrious]", got)
	}
}

func TestSearch_UnknownBaseWarnsButProceeds(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	if err := runSearchCmd(t, env, "anima", "--base", "Anima 2"); err != nil {
		t.Fatalf("unknown base must warn, not fail: %v", err)
	}
	if !strings.Contains(env.logs.String(), "[WARN]") || !strings.Contains(env.logs.String(), "Anima 2") {
		t.Errorf("expected [WARN] naming the unknown base, got: %s", env.logs.String())
	}
	if got := env.query()["baseModels"]; len(got) != 1 || got[0] != "Anima 2" {
		t.Errorf("unknown base must pass through unchanged, got %v", got)
	}
}

func TestSearch_MultiBaseKeepsFirstWithWarning(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	if err := runSearchCmd(t, env, "anima", "--base", "Illustrious,Pony"); err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	if !strings.Contains(env.logs.String(), "first baseModel filter") {
		t.Errorf("expected first-value-wins warning, got: %s", env.logs.String())
	}
	if got := env.query()["baseModels"]; len(got) != 1 || got[0] != "Illustrious" {
		t.Errorf("baseModels = %v, want only [Illustrious]", got)
	}
}

func TestSearch_LimitOver100Rejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runSearchCmd(t, env, "anima", "--limit", "101")
	if err == nil {
		t.Fatalf("limit 101 must be rejected CLI-side")
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
	}
	if env.lastPath != "" {
		t.Errorf("no request may be sent for invalid limit")
	}
}

func TestSearch_UnknownSortRejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runSearchCmd(t, env, "anima", "--sort", "bogus")
	if err == nil || !strings.Contains(err.Error(), "valid values") {
		t.Fatalf("unknown sort must list valid values, got: %v", err)
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
	}
}

func TestSearch_UnknownTypeRejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	if err := runSearchCmd(t, env, "anima", "--type", "gan"); err == nil {
		t.Fatalf("unknown type must be rejected")
	}
}

func TestSearch_MissingQueryRejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	if err := runSearchCmd(t, env); err == nil {
		t.Fatalf("missing query must be rejected")
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	if err := runSearchCmd(t, env, "nothingmatches"); err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	if !strings.Contains(env.buf.String(), "No models found") {
		t.Errorf("empty result message missing: %q", env.buf.String())
	}
}

func TestSearchLayout_WidthBudget(t *testing.T) {
	for _, width := range []int{80, 100, 120, 200} {
		c := searchLayout(width)
		total := 7 + c.name + c.typ + c.base + 6 + 6 + 10 + 4 + c.creator + 16
		if width >= 80 && total > width {
			t.Errorf("width %d: layout total %d exceeds budget", width, total)
		}
		if c.name < 12 {
			t.Errorf("width %d: NAME cap %d below floor 12", width, c.name)
		}
	}
	if searchLayout(120).name <= searchLayout(80).name {
		t.Errorf("wider terminals must give NAME more room")
	}
}

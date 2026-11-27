package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/report"
	"civitai-cli/internal/testutil"
)

// TestSearch_TableFixture drives the human command through the injected
// bytes.Buffer writer, which report.ConsoleWidthFor classifies as
// non-TTY: model names must therefore be emitted verbatim. The former
// all-lines-fit-80 assertion is intentionally gone from this case —
// non-TTY rows may exceed 80 columns now that NAME is uncapped.
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
	// The 17-rune fixture name exceeds the old 12-rune NAME cap: non-TTY
	// output must carry it verbatim and never emit the ellipsized form.
	if !strings.Contains(out, "Animagine XL V3.1") {
		t.Errorf("non-TTY output must contain the full model name:\n%s", out)
	}
	if strings.Contains(out, "Animagine X…") {
		t.Errorf("non-TTY output must not ellipsize the model name:\n%s", out)
	}
	// The summary footer follows the table exactly once and reports the
	// same facts the compact JSON summary carries for this fixture.
	for _, want := range []string{
		"Returned: 2; multi-base models: 0; models without base: 0",
		"Base coverage: Anima=1; SDXL 1.0=1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "Returned:"); n != 1 {
		t.Errorf("summary footer must appear exactly once, found %d:\n%s", n, out)
	}
	if strings.Index(out, "Returned:") < strings.Index(out, "2458426") {
		t.Errorf("summary footer must be appended after the table:\n%s", out)
	}
	if env.query().Get("query") != "anima" {
		t.Errorf("query param = %q", env.query().Get("query"))
	}
}

// TestRenderSearchTable_ConsoleModes pins the mode bit on the renderer
// seam: a successful console query (console=true) keeps the existing
// width-based NAME cap — including the 12-rune floor shared by widths 80
// and the narrow width 60 — while non-TTY output (console=false) writes
// NAME verbatim. Every other column keeps its width-derived cap in both
// modes, and the summary footer stays exactly once after the table.
func TestRenderSearchTable_ConsoleModes(t *testing.T) {
	model := api.Model{
		ID:         260267,
		Name:       "Animagine XL V3.1",
		Type:       "Checkpoint",
		BaseModels: []string{"SDXL 1.0"},
		Stats:      api.Stats{DownloadCount: 309400, ThumbsUpCount: 100, ThumbsDownCount: 2},
		Creator:    api.Creator{Username: "CagliostroLab"},
		ModelVersions: []api.ModelVersion{
			{PublishedAt: "2024-05-01T00:00:00Z"},
		},
	}
	cases := []struct {
		name        string
		width       int
		console     bool
		wantName    string
		forbidName  string
		wantCreator string
	}{
		{"console 80", 80, true, "Animagine X…", "Animagine XL V3.1", "Caglio…"},
		{"console 60 narrow", 60, true, "Animagine X…", "Animagine XL V3.1", "Caglio…"},
		{"console 120", 120, true, "Animagine XL V3.1", "Animagine X…", "CagliostroL…"},
		{"non-TTY 80", 80, false, "Animagine XL V3.1", "Animagine X…", "Caglio…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.console {
				// Cross-check the hard-coded expectation against the
				// production layout/truncation oracle so this table cannot
				// silently drift from searchLayout arithmetic.
				if want := report.Truncate(model.Name, searchLayout(tc.width).name); want != tc.wantName {
					t.Fatalf("wantName %q does not match the layout oracle %q", tc.wantName, want)
				}
			}
			var buf bytes.Buffer
			if err := renderSearchTable(&buf, []api.Model{model}, tc.width, tc.console); err != nil {
				t.Fatalf("renderSearchTable: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, tc.wantName) {
				t.Errorf("output must contain NAME %q:\n%s", tc.wantName, out)
			}
			if strings.Contains(out, tc.forbidName) {
				t.Errorf("output must not contain NAME %q:\n%s", tc.forbidName, out)
			}
			// Non-NAME cells keep their existing width-derived values in
			// every mode; the narrow widths share the 80-column caps.
			for _, want := range []string{tc.wantCreator, "CKPT", "SDXL 1.0", "309.4k", "50:1", "2024-05-01"} {
				if !strings.Contains(out, want) {
					t.Errorf("output missing cell %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "\t") {
				t.Errorf("tabwriter must expand tabs:\n%s", out)
			}
			if n := strings.Count(out, "Returned:"); n != 1 {
				t.Errorf("summary footer must appear exactly once, found %d:\n%s", n, out)
			}
			if strings.Index(out, "Returned:") < strings.Index(out, "260267") {
				t.Errorf("summary footer must be appended after the table:\n%s", out)
			}
		})
	}
}

// TestRenderSearchTable_NonTTYFullNames pins the exact-string contract of
// the non-TTY path: a name beyond the 48-rune maximum and a multibyte
// Unicode name are both emitted verbatim, with no ellipsized variant.
func TestRenderSearchTable_NonTTYFullNames(t *testing.T) {
	const longASCII = "An Extremely Long Model Name That Goes Well Beyond Forty Eight Runes"
	const unicodeName = "星空の美少女メイドモデル超絶可愛い版"
	models := []api.Model{
		{ID: 910001, Name: longASCII, Type: "LORA", BaseModels: []string{"Pony"}, Creator: api.Creator{Username: "alice"}},
		{ID: 910002, Name: unicodeName, Type: "LORA", BaseModels: []string{"Pony"}, Creator: api.Creator{Username: "bob"}},
	}
	var buf bytes.Buffer
	if err := renderSearchTable(&buf, models, 80, false); err != nil {
		t.Fatalf("renderSearchTable: %v", err)
	}
	out := buf.String()
	for _, want := range []string{longASCII, unicodeName} {
		if !strings.Contains(out, want) {
			t.Errorf("non-TTY output must contain the verbatim name %q:\n%s", want, out)
		}
	}
	// Every other cell is short enough to stay uncapped, so any ellipsis
	// at all would prove NAME truncation leaked into the non-TTY path.
	if strings.Contains(out, "…") {
		t.Errorf("non-TTY output must not ellipsize any cell:\n%s", out)
	}
	if strings.Contains(out, "\t") {
		t.Errorf("tabwriter must expand tabs:\n%s", out)
	}
	if strings.Index(out, longASCII) > strings.Index(out, unicodeName) {
		t.Errorf("rows must keep the input order:\n%s", out)
	}
}

// TestRenderSearchTable_OtherColumnsKeepCaps proves only NAME reacts to
// the console bit: overlong TYPE/BASE/CREATOR values keep their current
// capped forms in both modes at the width-80 fallback, and row order is
// the input order.
func TestRenderSearchTable_OtherColumnsKeepCaps(t *testing.T) {
	models := []api.Model{
		{ID: 920001, Name: "Animagine XL V3.1", Type: "SuperLongTypeName",
			BaseModels: []string{"Some Extremely Long Base Model Name"},
			Creator:    api.Creator{Username: "a-very-long-creator-name"}},
		{ID: 920002, Name: "Short", Type: "LORA", BaseModels: []string{"Pony"},
			Creator: api.Creator{Username: "bob"}},
	}
	for _, console := range []bool{true, false} {
		var buf bytes.Buffer
		if err := renderSearchTable(&buf, models, 80, console); err != nil {
			t.Fatalf("renderSearchTable(console=%v): %v", console, err)
		}
		out := buf.String()
		for _, want := range []string{"Sup…", "Some Ex…", "a-very…"} {
			if !strings.Contains(out, want) {
				t.Errorf("console=%v: capped column %q missing:\n%s", console, want, out)
			}
		}
		if strings.Index(out, "920001") > strings.Index(out, "920002") {
			t.Errorf("console=%v: rows must keep the input order:\n%s", console, out)
		}
		if console {
			if !strings.Contains(out, "Animagine X…") || strings.Contains(out, "Animagine XL V3.1") {
				t.Errorf("console output must keep the capped NAME:\n%s", out)
			}
		} else {
			if !strings.Contains(out, "Animagine XL V3.1") || strings.Contains(out, "Animagine X…") {
				t.Errorf("non-TTY output must keep the full NAME:\n%s", out)
			}
		}
	}
}

// TestSearch_NonTTYFullNamesThroughPipe routes the command's stdout
// through a real os.Pipe — an actual non-console *os.File — so the
// platform query path (not just the writer type assertion) is covered:
// the full NAME must survive and the footer must still follow the table.
func TestSearch_NonTTYFullNamesThroughPipe(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	env.g.out = w
	runErr := runSearchCmd(t, env, "anima")
	if cerr := w.Close(); cerr != nil {
		t.Fatalf("close pipe writer: %v", cerr)
	}
	outBytes, readErr := io.ReadAll(r)
	if cerr := r.Close(); cerr != nil {
		t.Fatalf("close pipe reader: %v", cerr)
	}
	if readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	if runErr != nil {
		t.Fatalf("runSearch through pipe: %v", runErr)
	}
	out := string(outBytes)
	if !strings.Contains(out, "Animagine XL V3.1") {
		t.Errorf("piped output must contain the full model name:\n%s", out)
	}
	if strings.Contains(out, "Animagine X…") {
		t.Errorf("piped output must not ellipsize the model name:\n%s", out)
	}
	if !strings.Contains(out, "Returned: 2; multi-base models: 0; models without base: 0") {
		t.Errorf("piped output must keep the summary footer:\n%s", out)
	}
	if env.query().Get("query") != "anima" {
		t.Errorf("query param = %q", env.query().Get("query"))
	}
}

// TestSearch_TableFooterMatchesCompactSummary proves the human footer and
// the compact JSON summary report identical facts for the same response:
// the expected footer lines are rebuilt from the decoded compact summary
// rather than hard-coded, so any semantic drift between the two output
// paths fails here.
func TestSearch_TableFooterMatchesCompactSummary(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	if err := runSearchCmd(t, env, "anima", "--json"); err != nil {
		t.Fatalf("runSearch --json: %v", err)
	}
	var compactEnv struct {
		Summary api.SearchResultSummary `json:"summary"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &compactEnv); err != nil {
		t.Fatalf("decode compact summary: %v", err)
	}
	s := compactEnv.Summary
	if s.Returned != 2 || s.ModelsWithMultipleBases != 0 || s.ModelsWithoutBase != 0 {
		t.Errorf("compact summary counters = %+v", s)
	}
	parts := make([]string, len(s.BaseCoverage))
	for i, c := range s.BaseCoverage {
		parts[i] = fmt.Sprintf("%s=%d", c.BaseModel, c.Models)
	}
	wantReturned := fmt.Sprintf("Returned: %d; multi-base models: %d; models without base: %d",
		s.Returned, s.ModelsWithMultipleBases, s.ModelsWithoutBase)
	wantCoverage := "Base coverage: " + strings.Join(parts, "; ")

	env.buf.Reset()
	if err := runSearchCmd(t, env, "anima"); err != nil {
		t.Fatalf("runSearch human: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{wantReturned, wantCoverage} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing compact-derived line %q:\n%s", want, out)
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
	// The legacy full view stays the raw upstream array: no envelope and
	// no injected summary object.
	if strings.Contains(env.buf.String(), `"summary"`) {
		t.Errorf("--view full must not gain a summary:\n%s", env.buf.String())
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
		SchemaVersion string                  `json:"schemaVersion"`
		Kind          string                  `json:"kind"`
		Items         []map[string]any        `json:"items"`
		Summary       api.SearchResultSummary `json:"summary"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &envl); err != nil {
		t.Fatalf("compact output must parse: %v\n%s", err, out)
	}
	if envl.SchemaVersion != "civ.search/v1" || envl.Kind != "modelSearch" {
		t.Errorf("envelope = %q/%q", envl.SchemaVersion, envl.Kind)
	}
	wantSummary := api.SearchResultSummary{
		Returned: 2,
		BaseCoverage: []api.SearchBaseCoverage{
			{BaseModel: "Anima", Models: 1},
			{BaseModel: "SDXL 1.0", Models: 1},
		},
	}
	if !reflect.DeepEqual(envl.Summary, wantSummary) {
		t.Errorf("summary = %+v, want %+v", envl.Summary, wantSummary)
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
		Items   []json.RawMessage       `json:"items"`
		Summary api.SearchResultSummary `json:"summary"`
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
	wantSummary := api.SearchResultSummary{Returned: 0, BaseCoverage: []api.SearchBaseCoverage{}}
	if !reflect.DeepEqual(envl.Summary, wantSummary) {
		t.Errorf("empty summary = %+v, want %+v", envl.Summary, wantSummary)
	}
	if !strings.Contains(env.buf.String(), `"baseCoverage": []`) {
		t.Errorf("empty coverage must serialize as []:\n%s", env.buf.String())
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
		{"uppercase view", []string{"anima", "--json", "--view", "COMPACT"}, `unknown --view "COMPACT"`},
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
	// The human path (with the new summary footer) must make the same
	// single request: summarization never adds a call or follows a cursor.
	t.Run("human", func(t *testing.T) {
		hits := 0
		env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.Write(fixture)
		})
		if err := runSearchCmd(t, env, "anima", "--type", "lora", "--limit", "5"); err != nil {
			t.Fatalf("runSearch human: %v", err)
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
	if env.buf.Len() != 0 {
		t.Errorf("invalid limit must keep stdout empty, got %q", env.buf.String())
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
	if env.lastPath != "" {
		t.Errorf("no request may be sent for an unknown sort, hit %q", env.lastPath)
	}
	if env.buf.Len() != 0 {
		t.Errorf("unknown sort must keep stdout empty, got %q", env.buf.String())
	}
}

func TestSearch_UnknownTypeRejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runSearchCmd(t, env, "anima", "--type", "gan")
	if err == nil {
		t.Fatalf("unknown type must be rejected")
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
	}
	if env.lastPath != "" {
		t.Errorf("no request may be sent for an unknown type, hit %q", env.lastPath)
	}
	if env.buf.Len() != 0 {
		t.Errorf("unknown type must keep stdout empty, got %q", env.buf.String())
	}
}

func TestSearch_UnknownPeriodRejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runSearchCmd(t, env, "anima", "--period", "decade")
	if err == nil || !strings.Contains(err.Error(), "valid values") {
		t.Fatalf("unknown period must list valid values, got: %v", err)
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
	}
	if env.lastPath != "" {
		t.Errorf("no request may be sent for an unknown period, hit %q", env.lastPath)
	}
	if env.buf.Len() != 0 {
		t.Errorf("unknown period must keep stdout empty, got %q", env.buf.String())
	}
}

func TestSearch_LimitZeroRejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runSearchCmd(t, env, "anima", "--limit", "0")
	if err == nil || !strings.Contains(err.Error(), "--limit must be 1-100") {
		t.Fatalf("limit 0 must be rejected CLI-side, got: %v", err)
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
	}
	if env.lastPath != "" {
		t.Errorf("no request may be sent for an invalid limit, hit %q", env.lastPath)
	}
	if env.buf.Len() != 0 {
		t.Errorf("invalid limit must keep stdout empty, got %q", env.buf.String())
	}
}

func TestSearch_MissingQueryRejected(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runSearchCmd(t, env)
	if err == nil {
		t.Fatalf("missing query must be rejected")
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
	}
	if env.lastPath != "" {
		t.Errorf("no request may be sent without a query, hit %q", env.lastPath)
	}
	if env.buf.Len() != 0 {
		t.Errorf("missing query must keep stdout empty, got %q", env.buf.String())
	}
}

// TestSearch_CaseInsensitiveAliasesAccepted keeps the success-side
// coverage for the case-insensitive sort/period/type mapping next to the
// rejection matrix above.
func TestSearch_CaseInsensitiveAliasesAccepted(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	err := runSearchCmd(t, env, "anima", "--sort", "HOT", "--period", "All", "--type", "LORA")
	if err != nil {
		t.Fatalf("case-insensitive sort/period/type aliases must be accepted: %v", err)
	}
	q := env.query()
	if got := q.Get("sort"); got != "Hot" {
		t.Errorf("sort = %q, want %q", got, "Hot")
	}
	if got := q.Get("period"); got != "AllTime" {
		t.Errorf("period = %q, want %q", got, "AllTime")
	}
	if got := q["types"]; len(got) != 1 || got[0] != "LORA" {
		t.Errorf("types = %v, want [LORA]", got)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	if err := runSearchCmd(t, env, "nothingmatches"); err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	// The empty human form keeps "No models found." and appends the footer
	// with returned 0 and no fabricated named categories.
	want := "No models found.\n" +
		"Returned: 0; multi-base models: 0; models without base: 0\n" +
		"Base coverage: none\n"
	if env.buf.String() != want {
		t.Errorf("empty human output = %q, want %q", env.buf.String(), want)
	}
}

// TestSearch_TableSummaryOverflow pins the human category window: with
// more than searchSummaryHumanBaseLimit observed bases the footer shows
// exactly the first eight sorted entries plus the overflow marker, while
// the compact JSON summary keeps every category.
func TestSearch_TableSummaryOverflow(t *testing.T) {
	const total = 10 // two categories beyond the human window
	var body strings.Builder
	body.WriteString(`{"items":[`)
	for i := 1; i <= total; i++ {
		if i > 1 {
			body.WriteString(",")
		}
		fmt.Fprintf(&body, `{"id":%d,"baseModels":["base%02d"]}`, i, i)
	}
	body.WriteString(`],"metadata":{}}`)
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body.String()))
	})
	if err := runSearchCmd(t, env, "overflow"); err != nil {
		t.Fatalf("runSearch human: %v", err)
	}
	var coverageLine string
	for _, line := range strings.Split(env.buf.String(), "\n") {
		if strings.HasPrefix(line, "Base coverage:") {
			coverageLine = line
		}
	}
	// Equal counts sort by exact name ascending, so the window is base01..base08.
	wantLine := "Base coverage: base01=1; base02=1; base03=1; base04=1; base05=1; base06=1; base07=1; base08=1; +2 more (use --json for all)"
	if coverageLine != wantLine {
		t.Errorf("coverage line = %q, want %q", coverageLine, wantLine)
	}
	if strings.Contains(coverageLine, "=0") {
		t.Errorf("no zero-count named category may appear: %q", coverageLine)
	}
	if !strings.Contains(env.buf.String(), "Returned: 10; multi-base models: 0; models without base: 0") {
		t.Errorf("overflow must not change the returned count:\n%s", env.buf.String())
	}

	env.buf.Reset()
	if err := runSearchCmd(t, env, "overflow", "--json"); err != nil {
		t.Fatalf("runSearch --json: %v", err)
	}
	var envl struct {
		Summary api.SearchResultSummary `json:"summary"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &envl); err != nil {
		t.Fatalf("decode compact summary: %v", err)
	}
	if envl.Summary.Returned != total {
		t.Errorf("compact returned = %d, want %d", envl.Summary.Returned, total)
	}
	if len(envl.Summary.BaseCoverage) != total {
		t.Fatalf("JSON keeps every category: coverage = %d, want %d", len(envl.Summary.BaseCoverage), total)
	}
	for i, c := range envl.Summary.BaseCoverage {
		wantName := fmt.Sprintf("base%02d", i+1)
		if c.BaseModel != wantName || c.Models != 1 {
			t.Errorf("coverage[%d] = %+v, want %s=1", i, c, wantName)
		}
	}
}

// errOnContainsWriter fails any write whose payload contains substr, so a
// test can fail the summary footer independently of the table flush that
// precedes it.
type errOnContainsWriter struct{ substr string }

// Write implements io.Writer, failing on the first payload containing substr.
func (w errOnContainsWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), w.substr) {
		return 0, errors.New("footer boom")
	}
	return len(p), nil
}

func TestSearch_SummaryWriteErrorPropagates(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models": "models_search_anima.json",
	}))
	env.g.out = errOnContainsWriter{substr: "Returned:"}
	err := runSearchCmd(t, env, "anima")
	if err == nil || !strings.Contains(err.Error(), "footer boom") {
		t.Fatalf("footer writer error must propagate through runSearch, got %v", err)
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
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

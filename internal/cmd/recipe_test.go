package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/testutil"
)

// recipeVersionPath is the single endpoint every official recipe invocation
// may request.
const recipeVersionPath = "/model-versions/3263843"

// recipeRequests records every request path a recipe invocation made, so
// tests can prove exactly-one-request behavior and the absence of model,
// image, or community fallback calls.
type recipeRequests struct {
	mu    sync.Mutex
	paths []string
}

func (r *recipeRequests) record(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

func (r *recipeRequests) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.paths)
}

func (r *recipeRequests) first() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.paths) == 0 {
		return ""
	}
	return r.paths[0]
}

func (r *recipeRequests) pathList() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

// newRecipeTestEnv serves inline bodies first, then pinned fixtures by
// path, and 404s everything else with the API's error shape.
func newRecipeTestEnv(t *testing.T, routes, bodies map[string]string) (*testEnv, *recipeRequests) {
	t.Helper()
	reqs := &recipeRequests{}
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		reqs.record(r.URL.Path)
		if body, ok := bodies[r.URL.Path]; ok {
			w.Write([]byte(body))
			return
		}
		if name, ok := routes[r.URL.Path]; ok {
			w.Write(testutil.LoadFixture(t, name))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Model not found"}`))
	})
	return env, reqs
}

func newRecipeFixtureEnv(t *testing.T) (*testEnv, *recipeRequests) {
	t.Helper()
	return newRecipeTestEnv(t, map[string]string{recipeVersionPath: "version_3263843.json"}, nil)
}

func runRecipeCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	var o recipeOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runRecipe(context.Background(), &env.g, pos, &o)
}

// testRecipeVersionBody builds an inline version response around an images
// array literal.
func testRecipeVersionBody(images string) string {
	return `{"id":3263843,"modelId":2458426,"name":"v1","baseModel":"SD 1.5",` +
		`"baseModelType":"Standard","publishedAt":"2026-01-02T03:04:05Z","trainedWords":[],` +
		`"model":{"name":"Mini","type":"LORA"},"images":` + images + `}`
}

// assertRecipeBytesNoKeys recursively walks a JSON document and fails on
// any forbidden key. Exact key equality keeps promptStatus and
// negativePromptStatus distinct from prompt and negativePrompt.
func assertRecipeBytesNoKeys(t *testing.T, out []byte, forbidden ...string) {
	t.Helper()
	blocked := make(map[string]bool, len(forbidden))
	for _, key := range forbidden {
		blocked[key] = true
	}
	var node any
	if err := json.Unmarshal(out, &node); err != nil {
		t.Fatalf("output must be valid JSON: %v\n%s", err, out)
	}
	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch n := node.(type) {
		case map[string]any:
			for key, child := range n {
				if blocked[key] {
					t.Errorf("%s.%s: forbidden key present in recipe output", path, key)
				}
				walk(child, path+"."+key)
			}
		case []any:
			for i, child := range n {
				walk(child, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(node, "$")
}

// assertRecipeBytesNoText fails when any source-text sentinel appears in
// the raw output.
func assertRecipeBytesNoText(t *testing.T, out []byte, sentinels ...string) {
	t.Helper()
	text := string(out)
	for _, sentinel := range sentinels {
		if strings.Contains(text, sentinel) {
			t.Errorf("recipe output leaks source text %q:\n%s", sentinel, text)
		}
	}
}

func decodeRecipeSummaryOutput(t *testing.T, out []byte) *api.RecipeSummary {
	t.Helper()
	var s api.RecipeSummary
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("summary JSON must decode: %v\n%s", err, out)
	}
	return &s
}

func decodeRecipeDetailOutput(t *testing.T, out []byte) *api.RecipeDetail {
	t.Helper()
	var d api.RecipeDetail
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("detail JSON must decode: %v\n%s", err, out)
	}
	return &d
}

// TestRecipe_SummaryJSON pins the one-request prompt-free summary shape
// against the version fixture.
func TestRecipe_SummaryJSON(t *testing.T) {
	env, reqs := newRecipeFixtureEnv(t)
	if err := runRecipeCmd(t, env, "3263843", "--json"); err != nil {
		t.Fatalf("recipe --json: %v", err)
	}
	if reqs.count() != 1 || reqs.first() != recipeVersionPath {
		t.Fatalf("recipe must make exactly one version request, got paths %v", reqs.pathList())
	}

	out := env.buf.Bytes()
	s := decodeRecipeSummaryOutput(t, out)
	if s.SchemaVersion != "civ.recipe/v1" || s.Kind != "recipeSummary" {
		t.Errorf("schema/kind = %q/%q", s.SchemaVersion, s.Kind)
	}
	if s.Provenance.Source != "civitai.officialApi" ||
		s.Provenance.APIPath != "/api/v1/model-versions/3263843" ||
		s.Provenance.VersionID != 3263843 {
		t.Errorf("provenance = %+v", s.Provenance)
	}
	if s.Subject.Model.ID == nil || *s.Subject.Model.ID != 2458426 ||
		s.Subject.Model.Name == nil || *s.Subject.Model.Name != "Anima" ||
		s.Subject.Model.Type == nil || *s.Subject.Model.Type != "Checkpoint" {
		t.Errorf("model identity = %+v", s.Subject.Model)
	}
	if s.Subject.Version.ID == nil || *s.Subject.Version.ID != 3263843 ||
		s.Subject.Version.Name == nil || *s.Subject.Version.Name != "turbo-v1.1" ||
		s.Subject.Version.BaseModel == nil || *s.Subject.Version.BaseModel != "Anima" ||
		s.Subject.Version.BaseModelType == nil || *s.Subject.Version.BaseModelType != "Standard" {
		t.Errorf("version identity = %+v", s.Subject.Version)
	}
	if s.Subject.TrainedWords == nil || len(s.Subject.TrainedWords) != 0 {
		t.Errorf("fixture trainedWords = %#v, want empty array", s.Subject.TrainedWords)
	}
	if s.Selection.Mode != "first" || s.Selection.Requested != 3 || s.Selection.Returned != 3 {
		t.Errorf("selection = %+v", s.Selection)
	}
	if s.Selection.Total == nil || *s.Selection.Total != 9 ||
		s.Selection.WithMetadata == nil || *s.Selection.WithMetadata != 9 ||
		s.Selection.WithoutMetadata == nil || *s.Selection.WithoutMetadata != 0 ||
		s.Selection.Truncated == nil || !*s.Selection.Truncated {
		t.Errorf("selection counts = %+v", s.Selection)
	}
	if len(s.Examples) != 3 {
		t.Fatalf("examples = %d, want 3", len(s.Examples))
	}
	for i, ex := range s.Examples {
		if ex.SourceIndex != i+1 {
			t.Errorf("example %d sourceIndex = %d", i, ex.SourceIndex)
		}
	}
	first := s.Examples[0]
	if first.Parameters == nil || first.Parameters.Steps == nil || *first.Parameters.Steps != 8 ||
		first.Parameters.CFGScale == nil || *first.Parameters.CFGScale != 1 ||
		first.Parameters.Sampler == nil || *first.Parameters.Sampler != "Euler" ||
		first.Parameters.Scheduler == nil || *first.Parameters.Scheduler != "simple" ||
		first.Parameters.Seed == nil || *first.Parameters.Seed != "866354325141838" ||
		first.Parameters.Denoise == nil || *first.Parameters.Denoise != 1 {
		t.Errorf("small parameters = %+v", first.Parameters)
	}
	if first.Parameters.Width != nil || first.Parameters.Height != nil {
		t.Errorf("object-shaped dimensions must be null, got %v/%v", first.Parameters.Width, first.Parameters.Height)
	}
	assertRecipeBytesNoKeys(t, out, "prompt", "negativePrompt", "description", "meta", "comfy", "workflow", "example")
	assertRecipeBytesNoText(t, out, "rx-78-2 gundam", "chromatic aberration")
}

// TestRecipe_SummaryHumanPromptFree pins the human summary: context is
// present, prompt values are not, and missing dimensions stay visibly
// missing instead of being guessed.
func TestRecipe_SummaryHumanPromptFree(t *testing.T) {
	env, reqs := newRecipeFixtureEnv(t)
	if err := runRecipeCmd(t, env, "3263843"); err != nil {
		t.Fatalf("recipe: %v", err)
	}
	if reqs.count() != 1 {
		t.Fatalf("requests = %d, want 1", reqs.count())
	}
	out := env.buf.String()
	for _, want := range []string{
		"MODEL      Anima (2458426)",
		"VERSION    turbo-v1.1 (3263843)   base: Anima / Standard   published: 2026-08-24",
		"PROVENANCE civitai.officialApi   /api/v1/model-versions/3263843",
		"TRIGGER WORDS (0)",
		"EXAMPLES  returned 3 of 9 (9 with meta, 0 without); truncated",
		"  [1]  metadata available   prompt provided   negativePrompt provided",
		"       steps 8   cfgScale 1   seed 866354325141838   denoise 1",
		"       sampler Euler   scheduler simple   width -   height -",
		"       issues: parameters.width unsupportedJsonShape, parameters.height unsupportedJsonShape",
		"Prompt text is omitted from summaries; use --example <sourceIndex> for one example's detail.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human summary missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"rx-78-2 gundam", "chromatic aberration", "masterpiece", "negativePrompt: worst"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("human summary leaks prompt text %q:\n%s", forbidden, out)
		}
	}
}

// TestRecipe_ExamplesSelectionMatrix pins every approved count, including
// count 1 staying prompt-free and count 0 returning context-only counts.
func TestRecipe_ExamplesSelectionMatrix(t *testing.T) {
	cases := []struct {
		count     string
		wantLen   int
		wantTrunc bool
	}{
		{"0", 0, true},
		{"1", 1, true},
		{"5", 5, true},
		{"10", 9, false},
	}
	for _, tc := range cases {
		t.Run("count "+tc.count, func(t *testing.T) {
			env, reqs := newRecipeFixtureEnv(t)
			if err := runRecipeCmd(t, env, "3263843", "--examples", tc.count, "--json"); err != nil {
				t.Fatalf("--examples %s: %v", tc.count, err)
			}
			if reqs.count() != 1 {
				t.Fatalf("requests = %d, want 1", reqs.count())
			}
			out := env.buf.Bytes()
			s := decodeRecipeSummaryOutput(t, out)
			if len(s.Examples) != tc.wantLen || s.Selection.Returned != tc.wantLen {
				t.Errorf("examples/returned = %d/%d, want %d", len(s.Examples), s.Selection.Returned, tc.wantLen)
			}
			if s.Selection.Truncated == nil || *s.Selection.Truncated != tc.wantTrunc {
				t.Errorf("truncated = %v, want %v", s.Selection.Truncated, tc.wantTrunc)
			}
			if s.Selection.Total == nil || *s.Selection.Total != 9 {
				t.Errorf("total = %v, want 9 (full source scope)", s.Selection.Total)
			}
			assertRecipeBytesNoKeys(t, out, "prompt", "negativePrompt", "description", "meta", "comfy", "workflow")
			assertRecipeBytesNoText(t, out, "rx-78-2 gundam", "chromatic aberration")
			if tc.wantLen == 0 && !strings.Contains(string(out), `"examples": []`) {
				t.Errorf("zero count must serialize an empty examples array:\n%s", out)
			}
		})
	}
}

// TestRecipe_ExamplesCountOneHumanPromptFree proves the human one-item
// summary is still a summary, not implicit detail.
func TestRecipe_ExamplesCountOneHumanPromptFree(t *testing.T) {
	env, _ := newRecipeFixtureEnv(t)
	if err := runRecipeCmd(t, env, "3263843", "--examples", "1"); err != nil {
		t.Fatalf("--examples 1: %v", err)
	}
	out := env.buf.String()
	if !strings.Contains(out, "EXAMPLES  returned 1 of 9") {
		t.Errorf("count-1 summary counts missing:\n%s", out)
	}
	if strings.Contains(out, "prompt:") || strings.Contains(out, "rx-78-2 gundam") {
		t.Errorf("count-1 human summary must stay prompt-free:\n%s", out)
	}
}

// TestRecipe_ExampleDetailJSON pins the only prompt-bearing view: one
// request, one selected source example, no examples array, no raw source
// blobs, and no nonselected text.
func TestRecipe_ExampleDetailJSON(t *testing.T) {
	env, reqs := newRecipeFixtureEnv(t)
	if err := runRecipeCmd(t, env, "3263843", "--example", "1", "--json"); err != nil {
		t.Fatalf("--example 1 --json: %v", err)
	}
	if reqs.count() != 1 || reqs.first() != recipeVersionPath {
		t.Fatalf("detail must make exactly one version request, got %v", reqs.pathList())
	}
	out := env.buf.Bytes()
	d := decodeRecipeDetailOutput(t, out)
	if d.SchemaVersion != "civ.recipe/v1" || d.Kind != "recipeExampleDetail" {
		t.Errorf("schema/kind = %q/%q", d.SchemaVersion, d.Kind)
	}
	if d.Selection.Mode != "sourceIndex" || d.Selection.Requested != 1 || d.Selection.Returned != 1 {
		t.Errorf("detail selection = %+v", d.Selection)
	}
	if d.Selection.Total == nil || *d.Selection.Total != 9 ||
		d.Selection.WithMetadata == nil || *d.Selection.WithMetadata != 9 ||
		d.Selection.WithoutMetadata == nil || *d.Selection.WithoutMetadata != 0 {
		t.Errorf("detail counts = %+v", d.Selection)
	}
	if d.Selection.Truncated != nil {
		t.Errorf("detail truncated must be null")
	}
	if d.Example == nil || d.Example.SourceIndex != 1 {
		t.Fatalf("detail example = %+v", d.Example)
	}
	if d.Example.TextParameters == nil ||
		d.Example.TextParameters.Prompt == nil ||
		!strings.Contains(*d.Example.TextParameters.Prompt, "rx-78-2 gundam") {
		t.Fatalf("selected prompt missing: %+v", d.Example.TextParameters)
	}
	if d.Example.TextParameters.NegativePrompt == nil ||
		!strings.Contains(*d.Example.TextParameters.NegativePrompt, "chromatic aberration") {
		t.Errorf("selected negative prompt missing")
	}
	if d.Example.Parameters == nil || d.Example.Parameters.Seed == nil ||
		*d.Example.Parameters.Seed != "866354325141838" {
		t.Errorf("detail seed precision drifted: %+v", d.Example.Parameters)
	}
	if strings.Contains(string(out), `"examples"`) {
		t.Errorf("detail must not contain an examples array:\n%s", out)
	}
	assertRecipeBytesNoKeys(t, out, "examples", "meta", "comfy", "workflow", "description", "files", "stats", "downloadUrl")
	assertRecipeBytesNoText(t, out, "hina", "han juri", "wriggle nightbug", "megumin", "chaika trabant")
}

// TestRecipe_ExampleDetailOtherIndex proves indirect selection returns only
// the requested example's text.
func TestRecipe_ExampleDetailOtherIndex(t *testing.T) {
	env, _ := newRecipeFixtureEnv(t)
	if err := runRecipeCmd(t, env, "3263843", "--example", "7", "--json"); err != nil {
		t.Fatalf("--example 7 --json: %v", err)
	}
	out := env.buf.Bytes()
	d := decodeRecipeDetailOutput(t, out)
	if d.Example == nil || d.Example.TextParameters == nil || d.Example.TextParameters.Prompt == nil ||
		!strings.Contains(*d.Example.TextParameters.Prompt, "wriggle nightbug") {
		t.Fatalf("example 7 prompt missing: %+v", d.Example)
	}
	assertRecipeBytesNoText(t, out, "rx-78-2 gundam", "hina", "megumin")
}

// TestRecipe_ExampleDetailHuman proves the human detail prints the selected
// verbatim text and nothing from nonselected examples.
func TestRecipe_ExampleDetailHuman(t *testing.T) {
	env, reqs := newRecipeFixtureEnv(t)
	if err := runRecipeCmd(t, env, "3263843", "--example", "1"); err != nil {
		t.Fatalf("--example 1: %v", err)
	}
	if reqs.count() != 1 {
		t.Fatalf("requests = %d, want 1", reqs.count())
	}
	out := env.buf.String()
	for _, want := range []string{
		"EXAMPLE  source index 1 of 9 (9 with meta, 0 without)",
		"  prompt:\n",
		"rx-78-2 gundam",
		"  negativePrompt:\n",
		"chromatic aberration",
		"steps 8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human detail missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"hina", "megumin", "chaika trabant"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("human detail leaks nonselected text %q:\n%s", forbidden, out)
		}
	}
}

// TestRecipe_OutOfRangeDetail proves an out-of-range index fails after the
// one fetch with empty stdout and no summary fallback.
func TestRecipe_OutOfRangeDetail(t *testing.T) {
	env, reqs := newRecipeFixtureEnv(t)
	err := runRecipeCmd(t, env, "3263843", "--example", "10")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("out-of-range index must be a selection error, got %v", err)
	}
	if reqs.count() != 1 {
		t.Errorf("requests = %d, want exactly the one version fetch", reqs.count())
	}
	if env.buf.Len() != 0 {
		t.Errorf("out-of-range detail must write no partial stdout:\n%s", env.buf.String())
	}
	if got := exitCodeFor(err, false); got != 1 {
		t.Errorf("out-of-range detail must map to exit 1, got %d", got)
	}
}

// TestRecipe_UsageErrorsBeforeNetwork pins local-only validation: every
// rejected selector or id fails with zero requests and zero stdout.
func TestRecipe_UsageErrorsBeforeNetwork(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no version id", nil},
		{"two positionals", []string{"3263843", "3263844"}},
		{"non-numeric id", []string{"abc"}},
		{"zero id", []string{"0"}},
		{"both selectors", []string{"3263843", "--examples", "3", "--example", "1"}},
		{"both selectors same target", []string{"3263843", "--examples", "1", "--example", "1"}},
		{"count above range", []string{"3263843", "--examples", "11"}},
		{"count below range", []string{"3263843", "--examples", "-1"}},
		{"zero example index", []string{"3263843", "--example", "0"}},
		{"negative example index", []string{"3263843", "--example", "-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, reqs := newRecipeFixtureEnv(t)
			err := runRecipeCmd(t, env, tc.args...)
			if err == nil {
				t.Fatalf("%s: expected a usage error", tc.name)
			}
			if reqs.count() != 0 {
				t.Errorf("%s: must fail before network, got requests %v", tc.name, reqs.pathList())
			}
			if env.buf.Len() != 0 {
				t.Errorf("%s: usage error must not write stdout", tc.name)
			}
			if got := exitCodeFor(err, false); got != 1 {
				t.Errorf("%s: usage-class error must map to exit 1, got %d", tc.name, got)
			}
		})
	}
}

// TestRecipe_MalformedSelectorValues pins flag-parse rejection for
// non-integer selector values (usage class, exit 1, no network).
func TestRecipe_MalformedSelectorValues(t *testing.T) {
	for _, args := range [][]string{
		{"3263843", "--examples", "abc"},
		{"3263843", "--example", "abc"},
		{"-5"}, // a leading-dash positional is flag syntax, rejected before network
	} {
		fs := flag.NewFlagSet("recipe", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var o recipeOptions
		o.register(fs)
		if err := fs.Parse(permuteArgs(fs, args)); err == nil {
			t.Errorf("%v: malformed selector must fail flag parsing", args)
		}
	}
}

// TestRecipe_OptionsPresenceTracking pins the explicit-presence semantics
// that let an explicit --examples 3 coexist with the default and make
// --example 0 an invalid index rather than "absent".
func TestRecipe_OptionsPresenceTracking(t *testing.T) {
	fs := flag.NewFlagSet("recipe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o recipeOptions
	o.register(fs)
	if fs.Lookup("examples") == nil || fs.Lookup("example") == nil {
		t.Fatalf("recipe must register --examples and --example")
	}
	if o.examples.set || o.example.set {
		t.Fatalf("defaults must not look explicitly supplied")
	}
	if err := fs.Parse(permuteArgs(fs, []string{"3263843", "--examples", "0"})); err != nil {
		t.Fatalf("parse --examples 0: %v", err)
	}
	if !o.examples.set || o.examples.value != 0 {
		t.Errorf("explicit --examples 0 not tracked: %+v", o.examples)
	}
	if o.example.set {
		t.Errorf("--example must stay absent")
	}
}

// TestRecipe_NotFoundAndUnauthorizedMapping keeps the shared exit classes:
// 404 maps to 1 (exit 3 stays vet-only) and 401 maps to 4 only with a
// token. Both are single-request failures.
func TestRecipe_NotFoundAndUnauthorizedMapping(t *testing.T) {
	env, reqs := newRecipeTestEnv(t, nil, nil) // every path 404s
	err := runRecipeCmd(t, env, "3263843", "--json")
	var nfe *api.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
	if reqs.count() != 1 {
		t.Errorf("404 must be one non-retried request, got %d", reqs.count())
	}
	if got := exitCodeFor(err, false); got != 1 {
		t.Errorf("recipe 404 must map to exit 1 (3 is vet-only), got %d", got)
	}
	if env.buf.Len() != 0 {
		t.Errorf("failed recipe must keep stdout clean")
	}
}

// TestRecipe_UnauthorizedMapping exercises the shared 401 exit contract
// against a handler that returns HTTP 401.
func TestRecipe_UnauthorizedMapping(t *testing.T) {
	reqs := &recipeRequests{}
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		reqs.record(r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})
	err := runRecipeCmd(t, env, "3263843", "--json")
	var ue *api.UnauthorizedError
	if !errors.As(err, &ue) {
		t.Fatalf("expected UnauthorizedError, got %v", err)
	}
	if reqs.count() != 1 {
		t.Errorf("401 must be one non-retried request, got %d", reqs.count())
	}
	if got := exitCodeFor(err, false); got != 1 {
		t.Errorf("anonymous 401 must map to exit 1, got %d", got)
	}
	if got := exitCodeFor(err, true); got != 4 {
		t.Errorf("rejected token must map to exit 4, got %d", got)
	}
}

// TestRecipe_IDMismatchAndMalformedResponses pins decode-level command
// errors: a conflicting returned id and malformed bodies are errors, not
// rewritten output.
func TestRecipe_IDMismatchAndMalformedResponses(t *testing.T) {
	cases := map[string]string{
		"version id mismatch": `{"id":999,"modelId":2458426,"model":{"name":"Mini","type":"LORA"},"trainedWords":[],"images":[]}`,
		"malformed body":      `not json`,
		"images not array":    `{"id":3263843,"modelId":2458426,"model":{"name":"Mini","type":"LORA"},"trainedWords":[],"images":{}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeVersionPath: body})
			err := runRecipeCmd(t, env, "3263843", "--json")
			if err == nil {
				t.Fatalf("%s: expected an error", name)
			}
			if reqs.count() != 1 {
				t.Errorf("%s: requests = %d, want 1", name, reqs.count())
			}
			if env.buf.Len() != 0 {
				t.Errorf("%s: stdout must stay empty on decode errors", name)
			}
		})
	}
}

// TestRecipe_EmptySourceAndMissingImages pins AC7: explicit [] is a
// successful zero-example summary, while absent images is unknown (not
// proof of emptiness) and detail selection fails without fabrication.
func TestRecipe_EmptySourceAndMissingImages(t *testing.T) {
	t.Run("explicit empty images", func(t *testing.T) {
		env, _ := newRecipeTestEnv(t, nil, map[string]string{recipeVersionPath: testRecipeVersionBody(`[]`)})
		if err := runRecipeCmd(t, env, "3263843", "--json"); err != nil {
			t.Fatalf("empty source: %v", err)
		}
		out := env.buf.Bytes()
		s := decodeRecipeSummaryOutput(t, out)
		if len(s.Examples) != 0 || s.Selection.Total == nil || *s.Selection.Total != 0 ||
			s.Selection.Truncated == nil || *s.Selection.Truncated {
			t.Errorf("explicit [] summary = %+v", s.Selection)
		}
		if !strings.Contains(string(out), `"examples": []`) {
			t.Errorf("empty source must serialize examples as []:\n%s", out)
		}
		if strings.Contains(string(out), `"images"`) {
			t.Errorf("explicit [] must not report an images issue:\n%s", out)
		}
		assertRecipeBytesNoKeys(t, out, "prompt", "negativePrompt", "description", "meta", "comfy", "workflow")
		assertRecipeBytesNoText(t, out, "rx-78-2 gundam", "chromatic aberration")

		env2, reqs2 := newRecipeTestEnv(t, nil, map[string]string{recipeVersionPath: testRecipeVersionBody(`[]`)})
		if err := runRecipeCmd(t, env2, "3263843", "--example", "1", "--json"); err == nil {
			t.Fatalf("detail on an empty source must fail")
		}
		if reqs2.count() != 1 || env2.buf.Len() != 0 {
			t.Errorf("detail on empty source: requests=%d stdout=%d bytes", reqs2.count(), env2.buf.Len())
		}
	})

	t.Run("absent images", func(t *testing.T) {
		body := `{"id":3263843,"modelId":2458426,"model":{"name":"Mini","type":"LORA"},"trainedWords":[]}`
		env, _ := newRecipeTestEnv(t, nil, map[string]string{recipeVersionPath: body})
		if err := runRecipeCmd(t, env, "3263843", "--json"); err != nil {
			t.Fatalf("absent images: %v", err)
		}
		s := decodeRecipeSummaryOutput(t, env.buf.Bytes())
		if len(s.Examples) != 0 || s.Selection.Total != nil || s.Selection.Truncated != nil {
			t.Errorf("absent images must report unknown counts: %+v", s.Selection)
		}
		found := false
		for _, issue := range s.Issues {
			if issue.Field == "images" && issue.Reason == "notProvided" {
				found = true
			}
		}
		if !found {
			t.Errorf("absent images must carry a notProvided issue: %+v", s.Issues)
		}
		assertRecipeBytesNoKeys(t, env.buf.Bytes(), "prompt", "negativePrompt", "description", "meta", "comfy", "workflow")
		assertRecipeBytesNoText(t, env.buf.Bytes(), "rx-78-2 gundam", "chromatic aberration")
	})
}

// TestRecipe_MissingMetadataKeepsSourceIndex proves a null-meta entry is
// preserved in its source position with explicit missing semantics.
func TestRecipe_MissingMetadataKeepsSourceIndex(t *testing.T) {
	body := testRecipeVersionBody(`[{"url":"u1","hasMeta":true,"meta":null},{"url":"u2","meta":{"steps":20}}]`)
	env, _ := newRecipeTestEnv(t, nil, map[string]string{recipeVersionPath: body})
	if err := runRecipeCmd(t, env, "3263843", "--examples", "2", "--json"); err != nil {
		t.Fatalf("recipe: %v", err)
	}
	s := decodeRecipeSummaryOutput(t, env.buf.Bytes())
	if len(s.Examples) != 2 {
		t.Fatalf("examples = %d, want 2", len(s.Examples))
	}
	if s.Examples[0].SourceIndex != 1 || s.Examples[0].Parameters != nil ||
		s.Examples[0].MetadataStatus != "notProvided" {
		t.Errorf("null-meta example = %+v", s.Examples[0])
	}
	if s.Examples[1].SourceIndex != 2 || s.Examples[1].Parameters == nil ||
		s.Examples[1].Parameters.Steps == nil || *s.Examples[1].Parameters.Steps != 20 {
		t.Errorf("second example = %+v", s.Examples[1])
	}
	if s.Selection.WithMetadata == nil || *s.Selection.WithMetadata != 1 ||
		s.Selection.WithoutMetadata == nil || *s.Selection.WithoutMetadata != 1 {
		t.Errorf("metadata counts = %+v", s.Selection)
	}
}

// TestRecipe_HumanPromptFreeMatrix extends the prompt-free guarantee to the
// human renderer across default, zero, middle, maximum, empty-source, and
// missing-metadata inputs.
func TestRecipe_HumanPromptFreeMatrix(t *testing.T) {
	fixtureRoutes := map[string]string{recipeVersionPath: "version_3263843.json"}
	missingImages := `{"id":3263843,"modelId":2458426,"model":{"name":"Mini","type":"LORA"},"trainedWords":[]}`
	cases := []struct {
		name   string
		routes map[string]string
		bodies map[string]string
		args   []string
	}{
		{"fixture default", fixtureRoutes, nil, []string{"3263843"}},
		{"fixture count 0", fixtureRoutes, nil, []string{"3263843", "--examples", "0"}},
		{"fixture count 1", fixtureRoutes, nil, []string{"3263843", "--examples", "1"}},
		{"fixture count 5", fixtureRoutes, nil, []string{"3263843", "--examples", "5"}},
		{"fixture count 10", fixtureRoutes, nil, []string{"3263843", "--examples", "10"}},
		{"empty source", nil, map[string]string{recipeVersionPath: testRecipeVersionBody(`[]`)}, []string{"3263843"}},
		{"missing metadata", nil, map[string]string{recipeVersionPath: testRecipeVersionBody(`[{"url":"u1","meta":null},{"url":"u2","meta":{"prompt":"secret","steps":20}}]`)}, []string{"3263843"}},
		{"missing images", nil, map[string]string{recipeVersionPath: missingImages}, []string{"3263843"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := newRecipeTestEnv(t, tc.routes, tc.bodies)
			if err := runRecipeCmd(t, env, tc.args...); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			out := env.buf.String()
			for _, forbidden := range []string{"rx-78-2 gundam", "chromatic aberration", "masterpiece", "secret"} {
				if strings.Contains(out, forbidden) {
					t.Errorf("%s: human summary leaks prompt text %q:\n%s", tc.name, forbidden, out)
				}
			}
		})
	}
}

// TestRecipe_HelpPage pins the curated page contract: section order, both
// selectors documented with their semantics, no duplicated global block,
// and a --json example. Registration itself belongs to the parent
// integration pass.
func TestRecipe_HelpPage(t *testing.T) {
	for _, section := range []string{"Usage:", "Options:", "Examples:"} {
		if !strings.Contains(recipeHelp, section) {
			t.Errorf("recipe help missing %q section", section)
		}
	}
	for _, sig := range []string{
		"--examples <count>", "(0-10;", "default 3",
		"--example <index>", "1-based source index",
	} {
		if !strings.Contains(recipeHelp, sig) {
			t.Errorf("recipe help missing signature %q", sig)
		}
	}
	u := strings.Index(recipeHelp, "Usage:")
	o := strings.Index(recipeHelp, "Options:")
	e := strings.Index(recipeHelp, "Examples:")
	if !(u < o && o < e) {
		t.Errorf("recipe help section order violated: Usage(%d) < Options(%d) < Examples(%d)", u, o, e)
	}
	if strings.Contains(recipeHelp, "Global flags") {
		t.Errorf("page body must not embed the global block")
	}
	hasJSON := false
	for _, line := range strings.Split(recipeHelp, "\n") {
		if strings.HasPrefix(line, "  civ ") && strings.Contains(line, "--json") {
			hasJSON = true
		}
	}
	if !hasJSON {
		t.Errorf("recipe help must show a --json example")
	}
	for _, forbidden := range []string{"download this model", "generate images", "export the workflow", "ready to run"} {
		if strings.Contains(strings.ToLower(recipeHelp), forbidden) {
			t.Errorf("recipe help must not claim %q", forbidden)
		}
	}
}

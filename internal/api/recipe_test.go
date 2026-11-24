package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"civitai-cli/internal/testutil"
)

// fixtureRecipeRaw loads the verbatim version fixture every official-version
// recipe test projects.
func fixtureRecipeRaw(t *testing.T) []byte {
	t.Helper()
	return testutil.LoadFixture(t, "version_3263843.json")
}

func mustRecipeSummary(t *testing.T, raw []byte, requestedID, count int) *RecipeSummary {
	t.Helper()
	s, err := ParseRecipeSummary(raw, requestedID, count)
	if err != nil {
		t.Fatalf("ParseRecipeSummary: %v", err)
	}
	return s
}

func mustRecipeDetail(t *testing.T, raw []byte, requestedID, sourceIndex int) *RecipeDetail {
	t.Helper()
	d, err := ParseRecipeDetail(raw, requestedID, sourceIndex)
	if err != nil {
		t.Fatalf("ParseRecipeDetail: %v", err)
	}
	return d
}

func marshalRecipe(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal recipe output: %v", err)
	}
	return string(b)
}

// recipeBody builds a synthetic version body around an images array
// literal. extra appends additional top-level members (for drift cases).
func recipeBody(images string, extra string) []byte {
	body := `{"id":3263843,"modelId":2458426,"name":"v1","baseModel":"SD 1.5","baseModelType":"Standard",` +
		`"publishedAt":"2026-01-02T03:04:05Z","trainedWords":[],"model":{"name":"Mini","type":"LORA"},` +
		`"images":` + images
	if extra != "" {
		body += "," + extra
	}
	return []byte(body + "}")
}

// recipeMetaSummary projects one synthetic image carrying the given meta
// literal.
func recipeMetaSummary(t *testing.T, meta string) *RecipeSummary {
	t.Helper()
	return mustRecipeSummary(t, recipeBody(`[{"url":"https://example.test/1.png","meta":`+meta+`}]`, ""), 3263843, 1)
}

func onlyExample(t *testing.T, s *RecipeSummary) RecipeSummaryExample {
	t.Helper()
	if len(s.Examples) != 1 {
		t.Fatalf("examples = %d, want exactly 1", len(s.Examples))
	}
	return s.Examples[0]
}

func hasIssue(issues []RecipeIssue, field, reason string) bool {
	for _, issue := range issues {
		if issue.Field == field && issue.Reason == reason {
			return true
		}
	}
	return false
}

// assertNoJSONKeys recursively fails when a marshaled recipe output
// contains any forbidden key. Exact key equality keeps promptStatus and
// negativePromptStatus distinct from prompt and negativePrompt.
func assertNoJSONKeys(t *testing.T, v any, forbidden ...string) {
	t.Helper()
	blocked := make(map[string]bool, len(forbidden))
	for _, key := range forbidden {
		blocked[key] = true
	}
	var node any
	if err := json.Unmarshal([]byte(marshalRecipe(t, v)), &node); err != nil {
		t.Fatalf("decode marshaled recipe output: %v", err)
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

// assertNoSourceText fails when any literal source-text sentinel appears in
// the marshaled output.
func assertNoSourceText(t *testing.T, v any, sentinels ...string) {
	t.Helper()
	out := marshalRecipe(t, v)
	for _, sentinel := range sentinels {
		if strings.Contains(out, sentinel) {
			t.Errorf("recipe output leaks source text %q: %s", sentinel, out)
		}
	}
}

// TestParseRecipeSummary_Fixture pins the approved summary contract against
// the verbatim version fixture: identity, provenance, first-N order, counts,
// text statuses, small parameters, seed precision, and object-shaped
// dimensions reported as unsupported instead of guessed.
func TestParseRecipeSummary_Fixture(t *testing.T) {
	raw := fixtureRecipeRaw(t)
	s := mustRecipeSummary(t, raw, 3263843, 3)

	if s.SchemaVersion != recipeSchemaVersion || s.Kind != recipeSummaryKind {
		t.Errorf("schema/kind = %q/%q", s.SchemaVersion, s.Kind)
	}
	if s.Provenance.Source != recipeSourceOfficialAPI ||
		s.Provenance.APIPath != "/api/v1/model-versions/3263843" ||
		s.Provenance.VersionID != 3263843 {
		t.Errorf("provenance = %+v", s.Provenance)
	}

	if s.Subject.Model.ID == nil || *s.Subject.Model.ID != 2458426 {
		t.Errorf("model id = %v, want 2458426", s.Subject.Model.ID)
	}
	if s.Subject.Model.Name == nil || *s.Subject.Model.Name != "Anima" {
		t.Errorf("model name = %v, want Anima", s.Subject.Model.Name)
	}
	if s.Subject.Model.Type == nil || *s.Subject.Model.Type != "Checkpoint" {
		t.Errorf("model type = %v, want Checkpoint", s.Subject.Model.Type)
	}
	if s.Subject.Version.ID == nil || *s.Subject.Version.ID != 3263843 {
		t.Errorf("version id = %v", s.Subject.Version.ID)
	}
	if s.Subject.Version.Name == nil || *s.Subject.Version.Name != "turbo-v1.1" {
		t.Errorf("version name = %v", s.Subject.Version.Name)
	}
	if s.Subject.Version.BaseModel == nil || *s.Subject.Version.BaseModel != "Anima" {
		t.Errorf("baseModel = %v", s.Subject.Version.BaseModel)
	}
	if s.Subject.Version.BaseModelType == nil || *s.Subject.Version.BaseModelType != "Standard" {
		t.Errorf("baseModelType = %v", s.Subject.Version.BaseModelType)
	}
	if s.Subject.Version.PublishedAt == nil || *s.Subject.Version.PublishedAt != "2026-08-24T23:42:31.929Z" {
		t.Errorf("publishedAt = %v", s.Subject.Version.PublishedAt)
	}
	// The fixture's trainedWords is an explicit []; it must stay an empty
	// non-nil array without a notProvided issue.
	if s.Subject.TrainedWords == nil || len(s.Subject.TrainedWords) != 0 {
		t.Errorf("trainedWords = %#v, want empty non-nil", s.Subject.TrainedWords)
	}
	if hasIssue(s.Issues, "trainedWords", recipeReasonNotProvided) {
		t.Errorf("explicit [] must not produce a trainedWords issue: %+v", s.Issues)
	}

	wantSel := RecipeSelection{Mode: recipeModeFirst, Requested: 3, Returned: 3}
	if s.Selection.Mode != wantSel.Mode || s.Selection.Requested != 3 || s.Selection.Returned != 3 {
		t.Errorf("selection = %+v", s.Selection)
	}
	if s.Selection.Total == nil || *s.Selection.Total != 9 ||
		s.Selection.WithMetadata == nil || *s.Selection.WithMetadata != 9 ||
		s.Selection.WithoutMetadata == nil || *s.Selection.WithoutMetadata != 0 {
		t.Errorf("selection counts = %+v", s.Selection)
	}
	if s.Selection.Truncated == nil || !*s.Selection.Truncated {
		t.Errorf("3 of 9 examples must report truncated=true: %+v", s.Selection.Truncated)
	}

	if len(s.Examples) != 3 {
		t.Fatalf("examples = %d, want 3", len(s.Examples))
	}
	for i, ex := range s.Examples {
		if ex.SourceIndex != i+1 {
			t.Errorf("example %d sourceIndex = %d, want %d (source order)", i, ex.SourceIndex, i+1)
		}
		if ex.Provenance.Source != recipeSourceExample || ex.Provenance.VersionID != 3263843 {
			t.Errorf("example %d provenance = %+v", i, ex.Provenance)
		}
		if ex.Provenance.ImageURL == nil || !strings.HasPrefix(*ex.Provenance.ImageURL, "https://image.civitai.com/") {
			t.Errorf("example %d imageUrl = %v", i, ex.Provenance.ImageURL)
		}
		if ex.MetadataStatus != recipeStatusAvailable || ex.PromptStatus != recipeStatusProvided || ex.NegativePromptStatus != recipeStatusProvided {
			t.Errorf("example %d statuses = %s/%s/%s", i, ex.MetadataStatus, ex.PromptStatus, ex.NegativePromptStatus)
		}
	}

	first := s.Examples[0]
	p := first.Parameters
	if p == nil {
		t.Fatalf("first example parameters are nil")
	}
	if p.Steps == nil || *p.Steps != 8 {
		t.Errorf("steps = %v, want 8", p.Steps)
	}
	if p.CFGScale == nil || *p.CFGScale != 1 {
		t.Errorf("cfgScale = %v, want 1", p.CFGScale)
	}
	if p.Sampler == nil || *p.Sampler != "Euler" {
		t.Errorf("sampler = %v", p.Sampler)
	}
	if p.Scheduler == nil || *p.Scheduler != "simple" {
		t.Errorf("scheduler = %v", p.Scheduler)
	}
	if p.Seed == nil || *p.Seed != "866354325141838" {
		t.Errorf("seed = %v, want exact decimal string", p.Seed)
	}
	if p.Denoise == nil || *p.Denoise != 1 {
		t.Errorf("denoise = %v, want 1", p.Denoise)
	}
	// meta.width/height are ComfyUI node objects; they must stay null with
	// an unsupported-shape issue, never guessed from the outer image.
	if p.Width != nil || p.Height != nil {
		t.Errorf("object-shaped dimensions must be null, got %v/%v", p.Width, p.Height)
	}
	if !hasIssue(first.Issues, "parameters.width", recipeReasonUnsupportedShape) ||
		!hasIssue(first.Issues, "parameters.height", recipeReasonUnsupportedShape) {
		t.Errorf("expected width/height unsupported issues, got %+v", first.Issues)
	}

	// A prompt-free summary is materially smaller than the verbatim body.
	if got, limit := len(marshalRecipe(t, s)), len(raw); got >= limit {
		t.Errorf("summary output %d bytes is not smaller than the source body %d bytes", got, limit)
	}
}

// TestParseRecipeSummary_RedactionAtEveryCount enforces the global
// invariant: summaries are prompt-free at every count, including 1 and 0.
func TestParseRecipeSummary_RedactionAtEveryCount(t *testing.T) {
	raw := fixtureRecipeRaw(t)
	forbidden := []string{"prompt", "negativePrompt", "description", "meta", "comfy", "workflow"}
	sentinels := []string{"rx-78-2 gundam", "chromatic aberration", "masterpiece, best quality", "worst quality"}
	for count := 0; count <= 10; count++ {
		s := mustRecipeSummary(t, raw, 3263843, count)
		assertNoJSONKeys(t, s, forbidden...)
		assertNoSourceText(t, s, sentinels...)
		if want := min(count, 9); len(s.Examples) != want {
			t.Errorf("count %d: examples = %d, want %d", count, len(s.Examples), want)
		}
	}
	// Count 1 is still a summary: no text status carries a value, and the
	// singular example member does not exist at all.
	one := mustRecipeSummary(t, raw, 3263843, 1)
	if len(one.Examples) != 1 {
		t.Fatalf("count 1 returned %d examples", len(one.Examples))
	}
	var node map[string]any
	if err := json.Unmarshal([]byte(marshalRecipe(t, one)), &node); err != nil {
		t.Fatalf("decode count-1 summary: %v", err)
	}
	if _, ok := node["example"]; ok {
		t.Errorf("summary must not contain a singular example member: %v", node)
	}
}

// TestRecipeSummaryDTO_HasNoDetailTextMembers makes prompt leakage
// unrepresentable rather than a renderer discipline: the summary DTO
// family must have no detail text member and no raw-source member.
func TestRecipeSummaryDTO_HasNoDetailTextMembers(t *testing.T) {
	textType := reflect.TypeOf(RecipeTextParameters{})
	check := func(name string, typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type == textType || f.Type == reflect.PointerTo(textType) {
				t.Errorf("%s.%s carries detail text parameters", name, f.Name)
			}
			switch f.Name {
			case "Prompt", "NegativePrompt", "Meta", "Comfy", "Workflow", "Description", "TextParameters":
				t.Errorf("%s.%s is a detail-only member", name, f.Name)
			}
		}
	}
	check("RecipeSummary", reflect.TypeOf(RecipeSummary{}))
	check("RecipeSummaryExample", reflect.TypeOf(RecipeSummaryExample{}))
}

// TestParseRecipeDetail_Fixture pins singular detail: only source index 1's
// text is returned, verbatim, alongside the same small parameters and seed
// precision as the summary.
func TestParseRecipeDetail_Fixture(t *testing.T) {
	d := mustRecipeDetail(t, fixtureRecipeRaw(t), 3263843, 1)

	if d.SchemaVersion != recipeSchemaVersion || d.Kind != recipeExampleDetailKind {
		t.Errorf("schema/kind = %q/%q", d.SchemaVersion, d.Kind)
	}
	if d.Selection.Mode != recipeModeSourceIndex || d.Selection.Requested != 1 || d.Selection.Returned != 1 {
		t.Errorf("detail selection = %+v", d.Selection)
	}
	if d.Selection.Total == nil || *d.Selection.Total != 9 ||
		d.Selection.WithMetadata == nil || *d.Selection.WithMetadata != 9 ||
		d.Selection.WithoutMetadata == nil || *d.Selection.WithoutMetadata != 0 {
		t.Errorf("detail counts = %+v", d.Selection)
	}
	if d.Selection.Truncated != nil {
		t.Errorf("detail truncation is not applicable and must be null, got %v", *d.Selection.Truncated)
	}
	if d.Example == nil {
		t.Fatalf("detail example is nil")
	}
	ex := d.Example
	if ex.SourceIndex != 1 || ex.MetadataStatus != recipeStatusAvailable ||
		ex.PromptStatus != recipeStatusProvided || ex.NegativePromptStatus != recipeStatusProvided {
		t.Errorf("detail example identity/statuses = %+v", ex.RecipeSummaryExample)
	}
	if ex.TextParameters == nil {
		t.Fatalf("detail textParameters are nil for a meta-bearing example")
	}
	if ex.TextParameters.Prompt == nil || !strings.Contains(*ex.TextParameters.Prompt, "rx-78-2 gundam") {
		t.Errorf("selected positive prompt missing its sentinel: %v", ex.TextParameters.Prompt)
	}
	if ex.TextParameters.NegativePrompt == nil || !strings.Contains(*ex.TextParameters.NegativePrompt, "chromatic aberration") {
		t.Errorf("selected negative prompt missing its sentinel: %v", ex.TextParameters.NegativePrompt)
	}
	p := ex.Parameters
	if p == nil || p.Steps == nil || *p.Steps != 8 || p.CFGScale == nil || *p.CFGScale != 1 ||
		p.Seed == nil || *p.Seed != "866354325141838" || p.Sampler == nil || *p.Sampler != "Euler" ||
		p.Scheduler == nil || *p.Scheduler != "simple" || p.Denoise == nil || *p.Denoise != 1 {
		t.Errorf("detail small parameters drifted: %+v", p)
	}

	// Detail carries no examples array and no raw large source members.
	var node map[string]any
	if err := json.Unmarshal([]byte(marshalRecipe(t, d)), &node); err != nil {
		t.Fatalf("decode detail output: %v", err)
	}
	if _, ok := node["examples"]; ok {
		t.Errorf("detail must not contain an examples array")
	}
	assertNoJSONKeys(t, d, "examples", "meta", "comfy", "workflow", "description", "files", "stats", "downloadUrl")

	// Only the selected example's text is present.
	assertNoSourceText(t, d, "hina", "han juri", "wriggle nightbug", "megumin", "chaika trabant")
}

// TestParseRecipeDetail_SelectsExactlyOneSourceIndex proves another index
// returns only that example: index 7 is outside the default summary window
// and its unique prompt appears while example 1's unique sentinel does not.
func TestParseRecipeDetail_SelectsExactlyOneSourceIndex(t *testing.T) {
	d := mustRecipeDetail(t, fixtureRecipeRaw(t), 3263843, 7)
	if d.Example == nil || d.Example.SourceIndex != 7 || d.Selection.Requested != 7 {
		t.Fatalf("detail example = %+v", d.Example)
	}
	if d.Example.TextParameters == nil || d.Example.TextParameters.Prompt == nil ||
		!strings.Contains(*d.Example.TextParameters.Prompt, "wriggle nightbug") {
		t.Errorf("selected example 7 prompt missing: %v", d.Example.TextParameters)
	}
	assertNoSourceText(t, d, "rx-78-2 gundam")
}

// TestParseRecipeDetail_IndependentOfNonSelectedSource proves detail does
// not grow or change when nonselected prompts, unknown metadata, or comfy
// payloads change while full-source metadata counts stay the same.
func TestParseRecipeDetail_IndependentOfNonSelectedSource(t *testing.T) {
	base := recipeBody(`[
		{"url":"u1","meta":{"prompt":"selected prompt","negativePrompt":"neg","steps":20,"seed":7}},
		{"url":"u2","meta":{"prompt":"other prompt","negativePrompt":"other neg","steps":30,"seed":8}}
	]`, `"description":"<p>long description</p>"`)
	changed := recipeBody(`[
		{"url":"u1","meta":{"prompt":"selected prompt","negativePrompt":"neg","steps":20,"seed":7}},
		{"url":"u2","meta":{"prompt":"completely different text","negativePrompt":"changed","steps":99,"seed":9,"comfy":{"prompt":{"1":{"class_type":"KSampler"}}}}}
	]`, `"description":"<p>different description</p>","comfy":{"workflow":{"nodes":[]}}`)

	first := mustRecipeDetail(t, base, 3263843, 1)
	second := mustRecipeDetail(t, changed, 3263843, 1)
	if got, want := marshalRecipe(t, second), marshalRecipe(t, first); got != want {
		t.Errorf("detail changed with nonselected source content:\n got: %s\nwant: %s", got, want)
	}
}

// TestParseRecipeDetail_OutOfRangeAndUnavailable covers the post-fetch
// selection failures: no partial detail, no summary fallback.
func TestParseRecipeDetail_OutOfRangeAndUnavailable(t *testing.T) {
	raw := fixtureRecipeRaw(t)
	for _, index := range []int{0, -1, 10, 99} {
		if _, err := ParseRecipeDetail(raw, 3263843, index); err == nil {
			t.Errorf("index %d must be out of range", index)
		}
	}
	noImages := recipeBody(`[]`, "")
	if _, err := ParseRecipeDetail(noImages, 3263843, 1); err == nil {
		t.Errorf("empty images must not yield a fabricated detail")
	}
	missingImages := []byte(`{"id":3263843,"modelId":2458426,"model":{"name":"Mini","type":"LORA"},"trainedWords":[]}`)
	if _, err := ParseRecipeDetail(missingImages, 3263843, 1); err == nil {
		t.Errorf("absent images must not yield a fabricated detail")
	}
}

// TestParseRecipeSummary_CountMatrix pins the bounded first-N behavior,
// including zero and the 0-10 boundary values.
func TestParseRecipeSummary_CountMatrix(t *testing.T) {
	raw := fixtureRecipeRaw(t)
	for count := 0; count <= 10; count++ {
		s := mustRecipeSummary(t, raw, 3263843, count)
		if s.Selection.Requested != count {
			t.Errorf("count %d: requested = %d", count, s.Selection.Requested)
		}
		if want := min(count, 9); s.Selection.Returned != want || len(s.Examples) != want {
			t.Errorf("count %d: returned/examples = %d/%d, want %d", count, s.Selection.Returned, len(s.Examples), want)
		}
		if s.Selection.Total == nil || *s.Selection.Total != 9 {
			t.Errorf("count %d: total = %v", count, s.Selection.Total)
		}
		if s.Selection.Truncated == nil {
			t.Errorf("count %d: truncated must be known", count)
		} else if want := count < 9; *s.Selection.Truncated != want {
			t.Errorf("count %d: truncated = %v, want %v", count, *s.Selection.Truncated, want)
		}
	}
	zero := mustRecipeSummary(t, raw, 3263843, 0)
	if zero.Examples == nil || len(zero.Examples) != 0 {
		t.Errorf("zero count examples = %#v, want empty non-nil array", zero.Examples)
	}
	if !strings.Contains(marshalRecipe(t, zero), `"examples":[]`) {
		t.Errorf("zero count must serialize examples as []: %s", marshalRecipe(t, zero))
	}
}

// TestParseRecipeSummary_ImagesPresenceMatrix distinguishes absent/null
// (unknown source) from explicit [] (known empty) and rejects non-array
// containers as command errors.
func TestParseRecipeSummary_ImagesPresenceMatrix(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"id":3263843,"modelId":2458426,"model":{"name":"Mini","type":"LORA"},"trainedWords":[]}`),
		[]byte(`{"id":3263843,"modelId":2458426,"model":{"name":"Mini","type":"LORA"},"trainedWords":[],"images":null}`),
	} {
		s := mustRecipeSummary(t, body, 3263843, 3)
		if s.Examples == nil || len(s.Examples) != 0 {
			t.Errorf("absent/null images examples = %#v", s.Examples)
		}
		if s.Selection.Total != nil || s.Selection.WithMetadata != nil || s.Selection.WithoutMetadata != nil || s.Selection.Truncated != nil {
			t.Errorf("absent/null images must report unknown counts: %+v", s.Selection)
		}
		if !hasIssue(s.Issues, "images", recipeReasonNotProvided) {
			t.Errorf("absent/null images must carry a notProvided issue: %+v", s.Issues)
		}
	}

	empty := mustRecipeSummary(t, recipeBody(`[]`, ""), 3263843, 3)
	if empty.Examples == nil || len(empty.Examples) != 0 {
		t.Errorf("explicit [] examples = %#v", empty.Examples)
	}
	if empty.Selection.Total == nil || *empty.Selection.Total != 0 ||
		empty.Selection.WithMetadata == nil || *empty.Selection.WithMetadata != 0 ||
		empty.Selection.WithoutMetadata == nil || *empty.Selection.WithoutMetadata != 0 {
		t.Errorf("explicit [] counts = %+v", empty.Selection)
	}
	if empty.Selection.Truncated == nil || *empty.Selection.Truncated {
		t.Errorf("explicit [] truncated = %v, want false", empty.Selection.Truncated)
	}
	if hasIssue(empty.Issues, "images", recipeReasonNotProvided) {
		t.Errorf("explicit [] must not carry an images issue: %+v", empty.Issues)
	}

	for _, images := range []string{`{}`, `"nope"`, `5`, `true`} {
		if _, err := ParseRecipeSummary(recipeBody(images, ""), 3263843, 3); err == nil {
			t.Errorf("images = %s must be a decode error", images)
		}
	}
}

// TestParseRecipeSummary_NullMetaKeepsSourceIndex proves a null-meta entry
// stays in the selected window with its original source index, is counted
// as withoutMetadata, and never reindexes later entries.
func TestParseRecipeSummary_NullMetaKeepsSourceIndex(t *testing.T) {
	body := recipeBody(`[
		{"url":"u1","hasMeta":true,"meta":null},
		{"url":"u2","meta":{"prompt":"p2","steps":20}}
	]`, "")
	s := mustRecipeSummary(t, body, 3263843, 2)
	if len(s.Examples) != 2 {
		t.Fatalf("examples = %d, want 2 (null meta must not be dropped)", len(s.Examples))
	}
	if s.Examples[0].SourceIndex != 1 || s.Examples[1].SourceIndex != 2 {
		t.Errorf("source indexes = %d/%d, want 1/2", s.Examples[0].SourceIndex, s.Examples[1].SourceIndex)
	}
	if s.Examples[0].MetadataStatus != recipeStatusNotProvided || s.Examples[0].Parameters != nil {
		t.Errorf("null meta example = %+v", s.Examples[0])
	}
	if s.Examples[0].PromptStatus != recipeStatusNotProvided || s.Examples[0].NegativePromptStatus != recipeStatusNotProvided {
		t.Errorf("null meta text statuses = %s/%s", s.Examples[0].PromptStatus, s.Examples[0].NegativePromptStatus)
	}
	if !hasIssue(s.Examples[0].Issues, "parameters", recipeReasonNotProvided) {
		t.Errorf("null meta must carry a parameters notProvided issue: %+v", s.Examples[0].Issues)
	}
	if s.Examples[1].MetadataStatus != recipeStatusAvailable || s.Examples[1].Parameters == nil {
		t.Errorf("second example must keep its own metadata: %+v", s.Examples[1])
	}
	if s.Selection.WithMetadata == nil || *s.Selection.WithMetadata != 1 ||
		s.Selection.WithoutMetadata == nil || *s.Selection.WithoutMetadata != 1 {
		t.Errorf("metadata counts = %+v", s.Selection)
	}
}

// TestParseRecipeSummary_TextStatuses pins provided/empty/notProvided/
// unsupportedJsonShape without exposing text in the summary.
func TestParseRecipeSummary_TextStatuses(t *testing.T) {
	cases := []struct {
		name       string
		meta       string
		wantPrompt string
		wantNeg    string
		wantIssue  string // empty means no text issue
		promptText string
	}{
		{"provided", `{"prompt":"hello world","negativePrompt":"bad"}`, recipeStatusProvided, recipeStatusProvided, "", ""},
		{"empty is distinct", `{"prompt":"","negativePrompt":""}`, recipeStatusEmpty, recipeStatusEmpty, "", ""},
		{"null", `{"prompt":null,"negativePrompt":null}`, recipeStatusNotProvided, recipeStatusNotProvided, "", ""},
		{"absent", `{}`, recipeStatusNotProvided, recipeStatusNotProvided, "", ""},
		{"number prompt", `{"prompt":12345,"negativePrompt":false}`, recipeStatusUnsupported, recipeStatusUnsupported, "prompt", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := onlyExample(t, recipeMetaSummary(t, tc.meta))
			if ex.PromptStatus != tc.wantPrompt || ex.NegativePromptStatus != tc.wantNeg {
				t.Errorf("statuses = %s/%s, want %s/%s", ex.PromptStatus, ex.NegativePromptStatus, tc.wantPrompt, tc.wantNeg)
			}
			if tc.wantIssue != "" && !hasIssue(ex.Issues, tc.wantIssue, recipeReasonUnsupportedShape) {
				t.Errorf("missing %s unsupported issue: %+v", tc.wantIssue, ex.Issues)
			}
			if tc.wantIssue == "" && hasIssue(ex.Issues, "prompt", recipeReasonUnsupportedShape) {
				t.Errorf("unexpected prompt issue: %+v", ex.Issues)
			}
			// Summary must never carry text values, not even in issues.
			assertNoSourceText(t, ex, "hello world", "12345")
		})
	}
}

// TestParseRecipeDetail_TextMatrix pins null plus issue for absent text,
// explicit "" for empty text, and null for unsupported shapes.
func TestParseRecipeDetail_TextMatrix(t *testing.T) {
	cases := []struct {
		name       string
		meta       string
		wantPrompt string
		wantText   *string
		wantIssue  string
	}{
		{"provided", `{"prompt":"hello world","negativePrompt":"bad"}`, recipeStatusProvided, strPtr("hello world"), ""},
		{"empty", `{"prompt":"","negativePrompt":""}`, recipeStatusEmpty, strPtr(""), ""},
		{"null", `{"prompt":null,"negativePrompt":null}`, recipeStatusNotProvided, nil, recipeReasonNotProvided},
		{"absent", `{}`, recipeStatusNotProvided, nil, recipeReasonNotProvided},
		{"unsupported", `{"prompt":["x"],"negativePrompt":{"v":1}}`, recipeStatusUnsupported, nil, recipeReasonUnsupportedShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := recipeBody(`[{"url":"u","meta":`+tc.meta+`}]`, "")
			d := mustRecipeDetail(t, body, 3263843, 1)
			if d.Example == nil || d.Example.TextParameters == nil {
				t.Fatalf("textParameters must be a non-null object when meta is present")
			}
			tp := d.Example.TextParameters
			if d.Example.PromptStatus != tc.wantPrompt {
				t.Errorf("promptStatus = %s, want %s", d.Example.PromptStatus, tc.wantPrompt)
			}
			switch {
			case tc.wantText == nil && tp.Prompt != nil:
				t.Errorf("prompt must be null, got %q", *tp.Prompt)
			case tc.wantText != nil && tp.Prompt == nil:
				t.Errorf("prompt = null, want %q", *tc.wantText)
			case tc.wantText != nil && *tp.Prompt != *tc.wantText:
				t.Errorf("prompt = %q, want %q", *tp.Prompt, *tc.wantText)
			}
			if tc.wantIssue == "" {
				if hasIssue(d.Example.Issues, "textParameters.prompt", recipeReasonNotProvided) ||
					hasIssue(d.Example.Issues, "textParameters.prompt", recipeReasonUnsupportedShape) {
					t.Errorf("unexpected prompt issue: %+v", d.Example.Issues)
				}
				return
			}
			if !hasIssue(d.Example.Issues, "textParameters.prompt", tc.wantIssue) ||
				!hasIssue(d.Example.Issues, "textParameters.negativePrompt", tc.wantIssue) {
				t.Errorf("missing text issues %s: %+v", tc.wantIssue, d.Example.Issues)
			}
		})
	}

	// Meta absent: parameters and textParameters are null, statuses not
	// provided, and the metadata state is explicit.
	d := mustRecipeDetail(t, recipeBody(`[{"url":"u","meta":null}]`, ""), 3263843, 1)
	if d.Example == nil || d.Example.Parameters != nil || d.Example.TextParameters != nil {
		t.Errorf("null meta must yield null parameters and textParameters: %+v", d.Example)
	}
	if !hasIssue(d.Example.Issues, "parameters", recipeReasonNotProvided) {
		t.Errorf("null meta detail issue missing: %+v", d.Example.Issues)
	}
	if d.Example.PromptStatus != recipeStatusNotProvided {
		t.Errorf("null meta prompt status = %s", d.Example.PromptStatus)
	}
}

func strPtr(s string) *string { return &s }

// TestParseRecipeSummary_ScalarNormalization pins the approved shared
// normalizer: numeric strings equal numbers, fractional integers are
// rejected, invalid/non-scalar shapes become null plus issue, and unknown
// fields or units are never coerced.
func TestParseRecipeSummary_ScalarNormalization(t *testing.T) {
	numeric := onlyExample(t, recipeMetaSummary(t, `{"steps":20,"cfgScale":7.5,"sampler":"Euler",`+
		`"scheduler":"karras","seed":866354325141838,"width":512,"height":768,"denoise":0.5}`))
	stringsMeta := onlyExample(t, recipeMetaSummary(t, `{"steps":"20","cfgScale":"7.5","sampler":"Euler",`+
		`"scheduler":"karras","seed":"866354325141838","width":"512","height":"768","denoise":"0.5"}`))
	if got, want := marshalRecipe(t, stringsMeta.Parameters), marshalRecipe(t, numeric.Parameters); got != want {
		t.Errorf("numeric strings must normalize like numbers:\n got: %s\nwant: %s", got, want)
	}
	p := numeric.Parameters
	if p.Steps == nil || *p.Steps != 20 || p.CFGScale == nil || *p.CFGScale != 7.5 ||
		p.Seed == nil || *p.Seed != "866354325141838" || p.Width == nil || *p.Width != 512 ||
		p.Height == nil || *p.Height != 768 || p.Denoise == nil || *p.Denoise != 0.5 {
		t.Errorf("normalized parameters = %+v", p)
	}
	if len(numeric.Issues) != 0 {
		t.Errorf("fully valid meta must not emit issues: %+v", numeric.Issues)
	}

	cases := []struct {
		name   string
		meta   string
		field  string
		reason string
		nilOf  func(*RecipeParameters) bool
	}{
		{"fractional steps", `{"steps":8.5}`, "parameters.steps", recipeReasonInvalidNumber,
			func(p *RecipeParameters) bool { return p.Steps == nil }},
		{"fractional width", `{"width":512.5}`, "parameters.width", recipeReasonInvalidNumber,
			func(p *RecipeParameters) bool { return p.Width == nil }},
		{"object width", `{"width":{"class_type":"ResolutionSelector"}}`, "parameters.width", recipeReasonUnsupportedShape,
			func(p *RecipeParameters) bool { return p.Width == nil }},
		{"array height", `{"height":[768]}`, "parameters.height", recipeReasonUnsupportedShape,
			func(p *RecipeParameters) bool { return p.Height == nil }},
		{"bool sampler", `{"sampler":true}`, "parameters.sampler", recipeReasonUnsupportedShape,
			func(p *RecipeParameters) bool { return p.Sampler == nil }},
		{"invalid steps text", `{"steps":"abc"}`, "parameters.steps", recipeReasonInvalidNumber,
			func(p *RecipeParameters) bool { return p.Steps == nil }},
		{"empty steps string", `{"steps":""}`, "parameters.steps", recipeReasonInvalidNumber,
			func(p *RecipeParameters) bool { return p.Steps == nil }},
		{"null cfgScale", `{"cfgScale":null}`, "parameters.cfgScale", recipeReasonNotProvided,
			func(p *RecipeParameters) bool { return p.CFGScale == nil }},
		{"absent scheduler", `{}`, "parameters.scheduler", recipeReasonNotProvided,
			func(p *RecipeParameters) bool { return p.Scheduler == nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := onlyExample(t, recipeMetaSummary(t, tc.meta))
			if ex.Parameters == nil {
				t.Fatalf("parameters must be an object when meta is present")
			}
			if !tc.nilOf(ex.Parameters) {
				t.Errorf("field must normalize to null: %+v", ex.Parameters)
			}
			if !hasIssue(ex.Issues, tc.field, tc.reason) {
				t.Errorf("missing issue %s/%s: %+v", tc.field, tc.reason, ex.Issues)
			}
		})
	}

	// Explicit empty string stays "" for small string fields.
	emptyStr := onlyExample(t, recipeMetaSummary(t, `{"sampler":"","scheduler":""}`))
	if emptyStr.Parameters == nil || emptyStr.Parameters.Sampler == nil || *emptyStr.Parameters.Sampler != "" {
		t.Errorf("explicit empty sampler must stay \"\": %+v", emptyStr.Parameters)
	}
	if hasIssue(emptyStr.Issues, "parameters.sampler", recipeReasonNotProvided) {
		t.Errorf("explicit empty string must not be reported missing: %+v", emptyStr.Issues)
	}

	// Object-shaped dimensions are never replaced by outer image fields.
	withOuter := `[{"url":"u","width":1024,"height":1024,"meta":{"steps":8,"width":{"inputs":{"megapixels":1}},"height":{"inputs":{"megapixels":1}}}}]`
	ex := onlyExample(t, mustRecipeSummary(t, recipeBody(withOuter, ""), 3263843, 1))
	if ex.Parameters == nil || ex.Parameters.Width != nil || ex.Parameters.Height != nil {
		t.Errorf("outer image dimensions must not replace object-shaped meta dimensions: %+v", ex.Parameters)
	}
}

// TestParseRecipeSummary_SeedPrecision pins the decimal-string seed rule
// against a seed above 2^53, where a float64 round trip would corrupt it.
func TestParseRecipeSummary_SeedPrecision(t *testing.T) {
	const precise = "8663543251418380123"
	for _, meta := range []string{
		`{"seed":8663543251418380123}`,
		`{"seed":"8663543251418380123"}`,
		`{"seed":8.663543251418380123e18}`,
	} {
		ex := onlyExample(t, recipeMetaSummary(t, meta))
		if ex.Parameters == nil || ex.Parameters.Seed == nil {
			t.Fatalf("seed %s did not normalize", meta)
		}
		if *ex.Parameters.Seed != precise {
			t.Errorf("seed %s = %q, want %q", meta, *ex.Parameters.Seed, precise)
		}
		out := marshalRecipe(t, ex.Parameters)
		if !strings.Contains(out, `"seed":"`+precise+`"`) {
			t.Errorf("seed must marshal as an exact decimal string: %s", out)
		}
	}
	// Non-integer seed text and fractions are invalid, never rounded.
	invalidSeeds := []struct {
		meta   string
		reason string
	}{
		{`{"seed":"oops"}`, recipeReasonInvalidNumber},
		{`{"seed":1.5}`, recipeReasonInvalidNumber},
		{`{"seed":{"v":1}}`, recipeReasonUnsupportedShape},
	}
	for _, tc := range invalidSeeds {
		ex := onlyExample(t, recipeMetaSummary(t, tc.meta))
		if ex.Parameters == nil || ex.Parameters.Seed != nil {
			t.Errorf("seed %s must be null: %+v", tc.meta, ex.Parameters)
		}
		if !hasIssue(ex.Issues, "parameters.seed", tc.reason) {
			t.Errorf("seed %s missing %s issue: %+v", tc.meta, tc.reason, ex.Issues)
		}
	}
}

// TestParseRecipeSummary_UnknownFieldsIgnored pins the whitelist: raw
// metadata bags, ComfyUI payloads, and unknown scalars are never promoted
// into recipe keys.
func TestParseRecipeSummary_UnknownFieldsIgnored(t *testing.T) {
	meta := `{"prompt":"p","steps":8,"comfy":{"prompt":{"1":{"class_type":"KSampler"}}},` +
		`"engine":"comfy","models":[{"name":"x"}],"resources":[{"type":"lora"}],` +
		`"Model":{"name":"m"},"vaes":[],"scaleFactor":2,"workflow":{"nodes":[]}}`
	s := recipeMetaSummary(t, meta)
	assertNoJSONKeys(t, s, "comfy", "engine", "models", "resources", "Model", "vaes", "scaleFactor", "workflow", "class_type")
	ex := onlyExample(t, s)
	if ex.Parameters == nil || ex.Parameters.Steps == nil || *ex.Parameters.Steps != 8 {
		t.Errorf("known fields must still normalize: %+v", ex.Parameters)
	}
}

// TestParseRecipeSummary_TrainedWordsMatrix pins the trainedWords contract:
// absent/null -> [] plus notProvided, explicit [] stays empty, unknown
// shapes are reported, and string values are preserved verbatim.
func TestParseRecipeSummary_TrainedWordsMatrix(t *testing.T) {
	base := `"model":{"name":"Mini","type":"LORA"},"images":[]`
	cases := []struct {
		name      string
		body      string
		want      []string
		wantIssue string
	}{
		{"absent", `{"id":3263843,"modelId":2458426,` + base + `}`, []string{}, recipeReasonNotProvided},
		{"null", `{"id":3263843,"modelId":2458426,"trainedWords":null,` + base + `}`, []string{}, recipeReasonNotProvided},
		{"empty", `{"id":3263843,"modelId":2458426,"trainedWords":[],` + base + `}`, []string{}, ""},
		{"values preserved", `{"id":3263843,"modelId":2458426,"trainedWords":[" foo ", "bar"],` + base + `}`, []string{" foo ", "bar"}, ""},
		{"non-array", `{"id":3263843,"modelId":2458426,"trainedWords":"foo",` + base + `}`, []string{}, recipeReasonUnsupportedShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustRecipeSummary(t, []byte(tc.body), 3263843, 3)
			if s.Subject.TrainedWords == nil {
				t.Fatalf("trainedWords must always be an array")
			}
			if !reflect.DeepEqual(s.Subject.TrainedWords, tc.want) {
				t.Errorf("trainedWords = %#v, want %#v", s.Subject.TrainedWords, tc.want)
			}
			if tc.wantIssue != "" && !hasIssue(s.Issues, "trainedWords", tc.wantIssue) {
				t.Errorf("missing trainedWords issue %s: %+v", tc.wantIssue, s.Issues)
			}
			if tc.wantIssue == "" && hasIssue(s.Issues, "trainedWords", recipeReasonNotProvided) {
				t.Errorf("unexpected trainedWords issue: %+v", s.Issues)
			}
		})
	}

	// Mixed array elements: strings survive in order, non-strings are
	// dropped with a per-index issue.
	mixed := mustRecipeSummary(t, []byte(`{"id":3263843,"modelId":2458426,"trainedWords":["a",7,"b"],`+base+`}`), 3263843, 3)
	if !reflect.DeepEqual(mixed.Subject.TrainedWords, []string{"a", "b"}) {
		t.Errorf("trainedWords = %#v", mixed.Subject.TrainedWords)
	}
	if !hasIssue(mixed.Issues, "trainedWords[1]", recipeReasonUnsupportedShape) {
		t.Errorf("mixed array must flag index 1: %+v", mixed.Issues)
	}
}

// TestParseRecipeSummary_IdentityMatrix pins nullable identity semantics:
// absence is explicit, empty strings are preserved, and unknown enum
// values stay verbatim.
func TestParseRecipeSummary_IdentityMatrix(t *testing.T) {
	minimal := mustRecipeSummary(t, []byte(`{}`), 1, 3)
	if minimal.Subject.Model.ID != nil || minimal.Subject.Version.ID != nil {
		t.Errorf("missing ids must be null: %+v", minimal.Subject)
	}
	for _, want := range []struct{ field, reason string }{
		{"model.id", recipeReasonNotProvided},
		{"model.name", recipeReasonNotProvided},
		{"model.type", recipeReasonNotProvided},
		{"version.id", recipeReasonNotProvided},
		{"version.name", recipeReasonNotProvided},
		{"version.baseModel", recipeReasonNotProvided},
		{"version.baseModelType", recipeReasonNotProvided},
		{"version.publishedAt", recipeReasonNotProvided},
		{"images", recipeReasonNotProvided},
	} {
		if !hasIssue(minimal.Issues, want.field, want.reason) {
			t.Errorf("missing issue %s/%s: %+v", want.field, want.reason, minimal.Issues)
		}
	}

	unknown := mustRecipeSummary(t, []byte(`{"id":1,"modelId":2,"idType":"x","baseModel":"Future Base",`+
		`"baseModelType":"Anime","model":{"name":"","type":"BrandNewType"},"trainedWords":[],"images":[]}`), 1, 3)
	if unknown.Subject.Model.Type == nil || *unknown.Subject.Model.Type != "BrandNewType" {
		t.Errorf("unknown model type must be preserved: %v", unknown.Subject.Model.Type)
	}
	if unknown.Subject.Model.Name == nil || *unknown.Subject.Model.Name != "" {
		t.Errorf("explicit empty model name must stay \"\": %v", unknown.Subject.Model.Name)
	}
	if hasIssue(unknown.Issues, "model.name", recipeReasonNotProvided) {
		t.Errorf("explicit empty string must not be reported missing: %+v", unknown.Issues)
	}
	if unknown.Subject.Version.BaseModel == nil || *unknown.Subject.Version.BaseModel != "Future Base" {
		t.Errorf("unknown base model must be preserved: %v", unknown.Subject.Version.BaseModel)
	}
	if unknown.Subject.Version.BaseModelType == nil || *unknown.Subject.Version.BaseModelType != "Anime" {
		t.Errorf("baseModelType = %v", unknown.Subject.Version.BaseModelType)
	}
}

// TestParseRecipeSummary_VersionIDMismatch proves a conflicting nonzero
// returned id is a command error, while a missing id degrades to null plus
// an issue.
func TestParseRecipeSummary_VersionIDMismatch(t *testing.T) {
	if _, err := ParseRecipeSummary(fixtureRecipeRaw(t), 999999, 3); err == nil {
		t.Errorf("conflicting returned version id must error")
	}
	noID := mustRecipeSummary(t, []byte(`{"modelId":2,"model":{"name":"M","type":"LORA"},"trainedWords":[],"images":[]}`), 1, 3)
	if noID.Subject.Version.ID != nil {
		t.Errorf("missing returned id must be null, got %v", *noID.Subject.Version.ID)
	}
	if !hasIssue(noID.Issues, "version.id", recipeReasonNotProvided) {
		t.Errorf("missing returned id must carry an issue: %+v", noID.Issues)
	}
	zero := mustRecipeSummary(t, []byte(`{"id":0,"modelId":2,"model":{"name":"M","type":"LORA"},"trainedWords":[],"images":[]}`), 1, 3)
	if zero.Subject.Version.ID == nil || *zero.Subject.Version.ID != 0 {
		t.Errorf("returned zero id must be preserved as 0")
	}
}

// TestParseRecipeSummary_MalformedBodies keeps fundamental decode failures
// as command errors.
func TestParseRecipeSummary_MalformedBodies(t *testing.T) {
	cases := map[string]string{
		"not json":     `not json`,
		"array body":   `[]`,
		"string body":  `"x"`,
		"images shape": `{"id":1,"images":{"a":1}}`,
	}
	for name, body := range cases {
		if _, err := ParseRecipeSummary([]byte(body), 1, 3); err == nil {
			t.Errorf("%s: expected decode error", name)
		}
	}
	if _, err := ParseRecipeSummary([]byte(`{}`), 0, 3); err == nil {
		t.Errorf("non-positive requested id must be rejected")
	}
}

// TestParseRecipeSummary_Deterministic proves a fixed decoded response and
// selection produce byte-identical JSON with no timestamp-only difference.
func TestParseRecipeSummary_Deterministic(t *testing.T) {
	raw := fixtureRecipeRaw(t)
	first := marshalRecipe(t, mustRecipeSummary(t, raw, 3263843, 3))
	second := marshalRecipe(t, mustRecipeSummary(t, raw, 3263843, 3))
	if first != second {
		t.Errorf("summary output is not deterministic")
	}
}

// TestParseRecipeSummary_IndependentOfOmittedExamples proves the summary
// window is bounded: changing an omitted example's prompt or Comfy payload
// cannot grow or alter the returned summary while counts stay the same.
func TestParseRecipeSummary_IndependentOfOmittedExamples(t *testing.T) {
	base := recipeBody(`[
		{"url":"u1","meta":{"prompt":"p1","steps":8}},
		{"url":"u2","meta":{"prompt":"p2","steps":8}},
		{"url":"u3","meta":{"prompt":"p3","steps":8}},
		{"url":"u4","meta":{"prompt":"original fourth","steps":8}}
	]`, "")
	changed := recipeBody(`[
		{"url":"u1","meta":{"prompt":"p1","steps":8}},
		{"url":"u2","meta":{"prompt":"p2","steps":8}},
		{"url":"u3","meta":{"prompt":"p3","steps":8}},
		{"url":"u4","meta":{"prompt":"a much longer replacement prompt with lots of extra text","steps":8,"comfy":{"prompt":{"1":{"class_type":"KSampler"}}}}}
	]`, `"description":"<p>changed</p>"`)
	first := mustRecipeSummary(t, base, 3263843, 3)
	second := mustRecipeSummary(t, changed, 3263843, 3)
	if got, want := marshalRecipe(t, second), marshalRecipe(t, first); got != want {
		t.Errorf("summary changed with omitted example content:\n got: %s\nwant: %s", got, want)
	}
	if second.Selection.WithMetadata == nil || *second.Selection.WithMetadata != 4 {
		t.Errorf("counts must still cover the full source: %+v", second.Selection)
	}
	assertNoSourceText(t, second, "original fourth", "a much longer replacement")
}

// TestParseRecipeSummary_EmptyURLAndIssues pins image URL presence
// semantics shared by both views.
func TestParseRecipeExampleSummary_ImageURLPresence(t *testing.T) {
	cases := []struct {
		image     string
		wantURL   *string
		wantIssue string
	}{
		{`{"url":"https://example.test/a.png","meta":{"steps":8}}`, strPtr("https://example.test/a.png"), ""},
		{`{"url":"","meta":{"steps":8}}`, strPtr(""), ""},
		{`{"url":null,"meta":{"steps":8}}`, nil, recipeReasonNotProvided},
		{`{"meta":{"steps":8}}`, nil, recipeReasonNotProvided},
		{`{"url":42,"meta":{"steps":8}}`, nil, recipeReasonUnsupportedShape},
	}
	for _, tc := range cases {
		s := mustRecipeSummary(t, recipeBody(`[`+tc.image+`]`, ""), 3263843, 1)
		ex := onlyExample(t, s)
		switch {
		case tc.wantURL == nil && ex.Provenance.ImageURL != nil:
			t.Errorf("%s: imageUrl must be null, got %q", tc.image, *ex.Provenance.ImageURL)
		case tc.wantURL != nil && ex.Provenance.ImageURL == nil:
			t.Errorf("%s: imageUrl = null, want %q", tc.image, *tc.wantURL)
		case tc.wantURL != nil && *ex.Provenance.ImageURL != *tc.wantURL:
			t.Errorf("%s: imageUrl = %q, want %q", tc.image, *ex.Provenance.ImageURL, *tc.wantURL)
		}
		if tc.wantIssue != "" && !hasIssue(ex.Issues, "imageUrl", tc.wantIssue) {
			t.Errorf("%s: missing imageUrl issue %s: %+v", tc.image, tc.wantIssue, ex.Issues)
		}
		for _, issue := range ex.Issues {
			if issue.Field == "imageUrl" && tc.wantIssue == "" {
				t.Errorf("%s: unexpected imageUrl issue: %+v", tc.image, issue)
			}
		}
	}
}

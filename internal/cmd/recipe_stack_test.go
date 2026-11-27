package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"

	"civitai-cli/internal/api"
)

// testCommunityStackBody is the sentinel mixed body for stack-summary
// command tests: one fully described image (prompt text, raw metadata,
// workflow/ComfyUI, media locations, a duplicated LoRA weight), one
// sparse-metadata image repeating the checkpoint, one null-meta image, and
// continuation metadata that must never be followed or emitted.
func testCommunityStackBody() string {
	return `{"items":[
		{"id":1001,"postId":10010,"username":"stackuser1","createdAt":"2026-01-02T03:04:05Z",
			"type":"image","baseModel":"SD 1.5","nsfwLevel":"Soft","browsingLevel":1,
			"url":"https://image.civitai.com/sentinel-stack-media","hash":"sentinel-stack-blurhash",
			"thumbnailUrl":"https://thumb.example.test/sentinel-stack-thumb",
			"stats":{"likeCount":3,"dislikeCount":0,"heartCount":1,"laughCount":0,"cryCount":0,"commentCount":2},
			"meta":{"prompt":"sentinel-stack-prompt","negativePrompt":"sentinel-stack-negative",
				"steps":"20","cfgScale":7,"sampler":"Euler","scheduler":"simple",
				"unknownField":"sentinel-stack-unknown","workflow":"sentinel-stack-workflow","comfy":"sentinel-stack-comfy",
				"civitaiResources":[{"type":"checkpoint","modelVersionId":777,"modelName":"sentinel-stack-modelname"}],
				"resources":[{"type":"lora","name":"Stack LoRA","weight":"0.5"},{"type":"lora","name":"Stack LoRA","weight":0.5}]}},
		{"id":1002,"meta":{"steps":30,"civitaiResources":[{"type":"checkpoint","modelVersionId":777}]}},
		{"id":1003,"meta":null}
	],"metadata":{"nextCursor":"sentinel-stack-cursor","nextPage":"https://example.test/sentinel-stack-next"}}`
}

// decodeCommunityStackOutput unmarshals the aggregate JSON view.
func decodeCommunityStackOutput(t *testing.T, out []byte) *api.CommunityStackSummary {
	t.Helper()
	var s api.CommunityStackSummary
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("stack summary JSON must decode: %v\n%s", err, out)
	}
	return &s
}

// TestRecipe_CommunityStackSummaryRequestJSON pins AC-1: exactly one
// bounded /images request with the existing six query keys, the versioned
// aggregate-only JSON contract, coverage copied from the list projection,
// and no observation, large-field, media, or continuation leakage.
func TestRecipe_CommunityStackSummaryRequestJSON(t *testing.T) {
	env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: testCommunityStackBody()})
	if err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary", "--json"); err != nil {
		t.Fatalf("community --stack-summary --json: %v", err)
	}
	if reqs.count() != 1 || reqs.first() != recipeImagesPath {
		t.Fatalf("stack summary must make exactly one /images request, got %v", reqs.pathList())
	}
	assertRecipeExactQuery(t, reqs.queryAt(0), map[string]string{
		"modelVersionId": "3263843",
		"withMeta":       "true",
		"type":           "image",
		"limit":          "20",
		"sort":           "Most Reactions",
		"period":         "AllTime",
	}, "modelId", "page", "cursor", "nsfw", "browsingLevel")

	out := env.buf.Bytes()
	s := decodeCommunityStackOutput(t, out)
	if s.SchemaVersion != "civ.recipe/v1" || s.Kind != "communityStackSummary" {
		t.Errorf("schema/kind = %q/%q", s.SchemaVersion, s.Kind)
	}
	if s.TopLimit != 5 {
		t.Errorf("topLimit = %d, want 5", s.TopLimit)
	}
	wantCaveats := []string{"boundedNonRandomSample", "continuationNotFollowed", "descriptiveEvidenceOnly"}
	if strings.Join(s.Caveats, ",") != strings.Join(wantCaveats, ",") {
		t.Errorf("caveats = %v", s.Caveats)
	}
	if s.Provenance.Source != "community-images" || s.Provenance.APIPath != "/api/v1/images" ||
		s.Provenance.VersionID != 3263843 {
		t.Errorf("provenance = %+v", s.Provenance)
	}
	if s.Query.VersionID != 3263843 || s.Query.Type != "image" ||
		s.Query.Sort != "Most Reactions" || s.Query.Period != "AllTime" || s.Query.Limit != 20 {
		t.Errorf("query = %+v", s.Query)
	}
	if s.Coverage.RequestedLimit != 20 || s.Coverage.Returned != 3 ||
		s.Coverage.WithMeta != 2 || s.Coverage.Usable != 2 || !s.Coverage.ContinuationOffered {
		t.Errorf("coverage = %+v", s.Coverage)
	}

	if len(s.Resources.Checkpoints.Top) != 1 ||
		s.Resources.Checkpoints.Top[0].Identity.ModelVersionID == nil ||
		*s.Resources.Checkpoints.Top[0].Identity.ModelVersionID != 777 ||
		s.Resources.Checkpoints.Top[0].ImageCount != 2 {
		t.Errorf("checkpoints = %+v, want id 777 over 2 distinct images", s.Resources.Checkpoints.Top)
	}
	if len(s.Resources.LoRAs.Top) != 1 {
		t.Fatalf("loras = %+v, want one identity", s.Resources.LoRAs.Top)
	}
	lora := s.Resources.LoRAs.Top[0]
	if lora.Identity.Name == nil || *lora.Identity.Name != "Stack LoRA" || lora.ImageCount != 1 ||
		lora.Weight.SampleCount != 1 || lora.Weight.ImagesWithWeight != 1 ||
		lora.Weight.Median == nil || *lora.Weight.Median != 0.5 {
		t.Errorf("lora row = %+v, want collapsed duplicate weight samples", lora)
	}
	if s.Parameters.Steps.ObservedImages != 2 || s.Parameters.Steps.SampledImages != 3 ||
		len(s.Parameters.Steps.Top) != 2 {
		t.Errorf("steps distribution = %+v", s.Parameters.Steps)
	}
	prompt := s.TextAvailability.PromptStatus
	if prompt.Provided != 1 || prompt.NotProvided != 2 ||
		prompt.ProvidedRate == nil || *prompt.ProvidedRate != 1.0/3.0 {
		t.Errorf("prompt status = %+v", prompt)
	}

	assertRecipeBytesNoKeys(t, out,
		"observations", "examples", "image", "textParameters",
		"prompt", "negativePrompt", "meta", "comfy", "workflow", "description",
		"url", "pageUrl", "thumbnailUrl", "blurHash", "nextCursor", "nextPage", "cursor",
		"username", "postId", "createdAt", "imageId", "upstreamPosition",
		"issues", "modelVersionIds", "stats")
	assertRecipeBytesNoText(t, out,
		"sentinel-stack-prompt", "sentinel-stack-negative", "sentinel-stack-unknown",
		"sentinel-stack-workflow", "sentinel-stack-comfy", "sentinel-stack-media",
		"sentinel-stack-blurhash", "sentinel-stack-thumb", "sentinel-stack-cursor",
		"sentinel-stack-next", "sentinel-stack-modelname", "stackuser1")
}

// TestRecipe_CommunityStackSummaryOptionMatrix pins the exact outgoing
// sort/limit for every approved option combination, one request per case.
func TestRecipe_CommunityStackSummaryOptionMatrix(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantSort  string
		wantLimit string
	}{
		{"defaults", []string{"3263843", "--source", "community", "--stack-summary"}, "Most Reactions", "20"},
		{"newest", []string{"3263843", "--source", "community", "--stack-summary", "--sort", "newest"}, "Newest", "20"},
		{"api spelling", []string{"3263843", "--source", "community", "--stack-summary", "--sort", "Most Reactions"}, "Most Reactions", "20"},
		{"minimum limit", []string{"3263843", "--source", "community", "--stack-summary", "--limit", "1"}, "Most Reactions", "1"},
		{"maximum limit", []string{"3263843", "--source", "community", "--stack-summary", "--limit", "100", "--sort", "newest"}, "Newest", "100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: `{"items":[]}`})
			if err := runRecipeCmd(t, env, append(tc.args, "--json")...); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if reqs.count() != 1 {
				t.Fatalf("%s: requests = %d, want 1", tc.name, reqs.count())
			}
			assertRecipeExactQuery(t, reqs.queryAt(0), map[string]string{
				"modelVersionId": "3263843",
				"withMeta":       "true",
				"type":           "image",
				"limit":          tc.wantLimit,
				"sort":           tc.wantSort,
				"period":         "AllTime",
			}, "modelId", "page", "cursor", "nsfw", "browsingLevel")
		})
	}
}

// TestRecipe_CommunityStackSummaryHuman pins the human aggregate view:
// deterministic section order, coverage, caveats, text rates, identity
// rows with weight coverage and median, parameter denominators with
// top/other, empty-group markers, and no per-image or large-field text.
func TestRecipe_CommunityStackSummaryHuman(t *testing.T) {
	env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: testCommunityStackBody()})
	if err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary"); err != nil {
		t.Fatalf("community --stack-summary: %v", err)
	}
	if reqs.count() != 1 {
		t.Fatalf("requests = %d, want 1", reqs.count())
	}
	out := env.buf.String()

	sections := []string{
		"COMMUNITY STACK  version 3263843   sort Most Reactions   period AllTime   limit 20",
		"SOURCE     community-images   /api/v1/images",
		"COVERAGE   returned 3 of 20 requested   with meta 2/3 (66%)   usable 2/3 (66%)   continuation yes (not followed)",
		"CAVEATS",
		"PROMPT AVAILABILITY",
		"NEGATIVE PROMPT AVAILABILITY",
		"RESOURCES   entries 4   identified 4   unidentified 0   images with entries 2",
		"CHECKPOINTS",
		"LORAS",
		"OTHER RESOURCES",
		"PARAMETERS",
	}
	previous := -1
	for _, section := range sections {
		at := strings.Index(out, section)
		if at < 0 {
			t.Fatalf("human stack summary missing %q:\n%s", section, out)
		}
		if at <= previous {
			t.Fatalf("section %q out of order:\n%s", section, out)
		}
		previous = at
	}

	for _, want := range []string{
		"descriptive evidence only",
		"never recommendations",
		"PROMPT AVAILABILITY   provided 1/3 (33%)   empty 0   notProvided 2   unsupportedJsonShape 0",
		"NEGATIVE PROMPT AVAILABILITY   provided 1/3 (33%)   empty 0   notProvided 2   unsupportedJsonShape 0",
		"CHECKPOINTS   entries 2   identified images 2",
		"  modelVersionId 777   images 2",
		"LORAS   entries 2   identified images 1",
		"  name Stack LoRA   images 1   median 0.5   weights 1/1 images   samples 1",
		"OTHER RESOURCES   entries 0   identified images 0",
		"  (none observed)",
		"  steps     observed 2/3 (66%)",
		"    20   1 images",
		"    30   1 images",
		"  cfgScale  observed 1/3 (33%)",
		"    7   1 images",
		"  sampler   observed 1/3 (33%)",
		"    Euler   1 images",
		"  scheduler observed 1/3 (33%)",
		"    simple   1 images",
		"    other 0 images   0 distinct values",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human stack summary missing %q:\n%s", want, out)
		}
	}

	for _, forbidden := range []string{
		"1001", "1002", "1003", "stackuser1", "sentinel-stack",
		"  prompt:", "negativePrompt:", "image ", "post ",
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("human stack summary leaks %q:\n%s", forbidden, out)
		}
	}
}

// TestRecipe_CommunityStackSummaryUsageErrorsBeforeNetwork pins AC-2:
// every invalid stack-summary combination fails before any request with a
// usage-class error and empty stdout.
func TestRecipe_CommunityStackSummaryUsageErrorsBeforeNetwork(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"official default with flag", []string{"3263843", "--stack-summary"}},
		{"explicit official with flag", []string{"3263843", "--source", "official", "--stack-summary"}},
		{"community with examples", []string{"3263843", "--source", "community", "--stack-summary", "--examples", "3"}},
		{"community with example", []string{"3263843", "--source", "community", "--stack-summary", "--example", "1"}},
		{"image with flag", []string{"image", "1001", "--stack-summary"}},
		{"invalid version", []string{"abc", "--source", "community", "--stack-summary"}},
		{"zero version", []string{"0", "--source", "community", "--stack-summary"}},
		{"version overflow", []string{"999999999999999999999999", "--source", "community", "--stack-summary"}},
		{"unsupported sort", []string{"3263843", "--source", "community", "--stack-summary", "--sort", "random"}},
		{"empty sort", []string{"3263843", "--source", "community", "--stack-summary", "--sort="}},
		{"zero limit", []string{"3263843", "--source", "community", "--stack-summary", "--limit", "0"}},
		{"negative limit", []string{"3263843", "--source", "community", "--stack-summary", "--limit=-1"}},
		{"limit above max", []string{"3263843", "--source", "community", "--stack-summary", "--limit", "101"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: `{"items":[]}`})
			err := runRecipeCmd(t, env, tc.args...)
			if err == nil {
				t.Fatalf("%s: expected a usage error", tc.name)
			}
			if reqs.count() != 0 {
				t.Errorf("%s: must fail before network, got %v", tc.name, reqs.pathList())
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

// TestRecipe_CommunityStackSummaryMalformedFlagValues pins flag-parse
// rejection for non-boolean stack-summary values, and that an explicit
// false selects the untouched list path.
func TestRecipe_CommunityStackSummaryMalformedFlagValues(t *testing.T) {
	for _, args := range [][]string{
		{"3263843", "--source", "community", "--stack-summary=not-bool"},
		{"3263843", "--source", "community", "--stack-summary", "--limit", "abc"},
		{"3263843", "--source", "community", "--stack-summary", "--limit", "999999999999999999999"},
		{"-5"},
	} {
		fs := flag.NewFlagSet("recipe", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var o recipeOptions
		o.register(fs)
		if err := fs.Parse(permuteArgs(fs, args)); err == nil {
			t.Errorf("%v: malformed flag value must fail parsing", args)
		}
	}

	t.Run("explicit false selects the list view", func(t *testing.T) {
		env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: testCommunityStackBody()})
		if err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary=false", "--json"); err != nil {
			t.Fatalf("--stack-summary=false: %v", err)
		}
		if reqs.count() != 1 {
			t.Fatalf("requests = %d, want 1", reqs.count())
		}
		var sample api.CommunitySample
		if err := json.Unmarshal(env.buf.Bytes(), &sample); err != nil {
			t.Fatalf("list JSON must decode: %v", err)
		}
		if sample.Kind != "communityRecipeSample" || len(sample.Observations) != 3 {
			t.Errorf("false flag must keep the observations view: kind %q observations %d",
				sample.Kind, len(sample.Observations))
		}
	})

	t.Run("bare flag selects the stack view", func(t *testing.T) {
		env, _ := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: testCommunityStackBody()})
		if err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary", "--json"); err != nil {
			t.Fatalf("--stack-summary: %v", err)
		}
		s := decodeCommunityStackOutput(t, env.buf.Bytes())
		if s.Kind != "communityStackSummary" {
			t.Errorf("kind = %q, want communityStackSummary", s.Kind)
		}
	})
}

// TestRecipe_CommunityStackSummaryEmptyContinuation pins the empty sample:
// zero counts, empty tops, null rates, n/a human rates, and an offered but
// never followed continuation with no token or URL anywhere.
func TestRecipe_CommunityStackSummaryEmptyContinuation(t *testing.T) {
	body := `{"items":[],"metadata":{"nextCursor":"sentinel-stack-cursor","nextPage":"https://example.test/sentinel-stack-next"}}`

	env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: body})
	if err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary", "--json"); err != nil {
		t.Fatalf("empty stack summary: %v", err)
	}
	if reqs.count() != 1 {
		t.Fatalf("requests = %d, want exactly 1 (no follow-up)", reqs.count())
	}
	out := env.buf.Bytes()
	s := decodeCommunityStackOutput(t, out)
	if s.Coverage.Returned != 0 || !s.Coverage.ContinuationOffered {
		t.Errorf("coverage = %+v", s.Coverage)
	}
	if s.Resources.EntryCount != 0 || len(s.Resources.Checkpoints.Top) != 0 ||
		len(s.Resources.LoRAs.Top) != 0 || len(s.Resources.OtherResources.Top) != 0 {
		t.Errorf("empty resources = %+v", s.Resources)
	}
	if s.Parameters.Steps.SampledImages != 0 || len(s.Parameters.Steps.Top) != 0 {
		t.Errorf("empty steps = %+v", s.Parameters.Steps)
	}
	if s.TextAvailability.PromptStatus.ProvidedRate != nil ||
		s.TextAvailability.NegativePromptStatus.ProvidedRate != nil {
		t.Errorf("zero denominator must keep null rates: %+v", s.TextAvailability)
	}
	assertRecipeBytesNoKeys(t, out, "nextCursor", "nextPage", "cursor", "observations")
	assertRecipeBytesNoText(t, out, "sentinel-stack-cursor", "example.test/next")

	human, _ := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: body})
	if err := runRecipeCmd(t, human, "3263843", "--source", "community", "--stack-summary"); err != nil {
		t.Fatalf("empty human stack summary: %v", err)
	}
	hout := human.buf.String()
	for _, want := range []string{
		"returned 0 of 20 requested",
		"continuation yes (not followed)",
		"provided 0/0 (n/a)",
		"  (none observed)",
		"observed 0/0 (n/a)",
	} {
		if !strings.Contains(hout, want) {
			t.Errorf("human empty stack summary missing %q:\n%s", want, hout)
		}
	}
	if strings.Contains(hout, "sentinel-stack-cursor") || strings.Contains(hout, "example.test/next") {
		t.Errorf("human empty stack summary leaks continuation metadata:\n%s", hout)
	}
}

// TestRecipe_CommunityStackSummaryErrorMapping keeps the shared exit
// classes for the stack view: one non-retried request, existing error
// classification, and empty stdout on every failure.
func TestRecipe_CommunityStackSummaryErrorMapping(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		env, reqs := newRecipeTestEnv(t, nil, nil) // every path 404s
		err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary", "--json")
		var nfe *api.NotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("expected NotFoundError, got %v", err)
		}
		if reqs.count() != 1 || reqs.first() != recipeImagesPath {
			t.Errorf("404 must be one /images request, got %v", reqs.pathList())
		}
		if got := exitCodeFor(err, false); got != 1 {
			t.Errorf("stack 404 must map to exit 1, got %d", got)
		}
		if env.buf.Len() != 0 {
			t.Errorf("failed stack summary must keep stdout clean")
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: `not json`})
		err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary", "--json")
		if err == nil {
			t.Fatal("malformed body must be an error, not empty success")
		}
		if reqs.count() != 1 {
			t.Errorf("malformed body must stay one request, got %d", reqs.count())
		}
		if got := exitCodeFor(err, false); got != 1 {
			t.Errorf("decode failure must map to exit 1, got %d", got)
		}
		if env.buf.Len() != 0 {
			t.Errorf("decode failure must keep stdout clean")
		}
	})

	t.Run("non-array items", func(t *testing.T) {
		env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: `{"items":{}}`})
		err := runRecipeCmd(t, env, "3263843", "--source", "community", "--stack-summary", "--json")
		if err == nil {
			t.Fatal("non-array items must be an envelope error")
		}
		if reqs.count() != 1 {
			t.Errorf("envelope failure must stay one request, got %d", reqs.count())
		}
		if env.buf.Len() != 0 {
			t.Errorf("envelope failure must keep stdout clean")
		}
	})
}

// TestRecipe_CommunityStackSummaryHelpAndCompatibility pins the curated
// help additions: the new usage line, option semantics, a JSON example,
// and the aggregate-only/prompt-free/non-random caveats, with no
// duplicated global-flag block.
func TestRecipe_CommunityStackSummaryHelpAndCompatibility(t *testing.T) {
	for _, sig := range []string{
		"civ recipe <versionId> --source community --stack-summary [--sort <order>] [--limit <count>] [--json]",
		"--stack-summary",
		"aggregate-only stack summary",
		"at most five resource identities",
		"civ recipe 3263843 --source community --stack-summary --json",
		"bounded non-random sample",
		"descriptive evidence",
		"never recommendations",
		"Valid only with --source community.",
	} {
		if !strings.Contains(recipeHelp, sig) {
			t.Errorf("recipe help missing stack-summary signature %q", sig)
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

	// Absent-flag compatibility is proven by the unchanged official,
	// community-list, and image-detail tests in this package; here the
	// no-flag invocation is spot-checked to keep the observations view.
	env, reqs := newRecipeTestEnv(t, nil, map[string]string{recipeImagesPath: testCommunityStackBody()})
	if err := runRecipeCmd(t, env, "3263843", "--source", "community", "--json"); err != nil {
		t.Fatalf("community list without flag: %v", err)
	}
	if reqs.count() != 1 {
		t.Fatalf("requests = %d, want 1", reqs.count())
	}
	var sample api.CommunitySample
	if err := json.Unmarshal(env.buf.Bytes(), &sample); err != nil {
		t.Fatalf("list JSON must decode: %v", err)
	}
	if sample.Kind != "communityRecipeSample" || len(sample.Observations) != 3 {
		t.Errorf("no-flag invocation must keep the observations view: kind %q observations %d",
			sample.Kind, len(sample.Observations))
	}
}

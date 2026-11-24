package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// communitySixItemBody is the AC-3 coverage fixture: null meta, {}, a
// prompt-only meta, scalar meta plus identified civitaiResources, an
// identified legacy resource with weight, and unknown-field-only meta. It
// also carries continuation metadata and item-level media/BlurHash values
// that no list output may expose.
const communitySixItemBody = `{
  "items": [
    {"id": 1, "postId": 11, "username": "alice", "createdAt": "2026-01-02T03:04:05Z",
     "type": "image", "baseModel": "SD 1.5", "nsfwLevel": "Soft", "browsingLevel": 1,
     "url": "https://image.civitai.com/sentinel-media",
     "hash": "sentinel-blurhash",
     "thumbnailUrl": "https://thumb.example.test/sentinel-thumb",
     "stats": {"likeCount": 3, "dislikeCount": 0, "heartCount": 1, "laughCount": 0, "cryCount": 0, "commentCount": 2},
     "meta": null},
    {"id": 2, "meta": {}},
    {"id": 3, "meta": {"prompt": "sentinel-prompt-C", "negativePrompt": "sentinel-negative-C"}},
    {"id": 4, "meta": {"steps": 20, "civitaiResources": [{"type": "checkpoint", "modelVersionId": 249861}]}},
    {"id": 5, "meta": {"resources": [{"type": "lora", "name": "Detail LoRA", "weight": "0.65"}]}},
    {"id": 6, "meta": {"unknownField": "sentinel-unknown", "anotherUnknown": 7}}
  ],
  "metadata": {"nextCursor": "sentinel-cursor", "nextPage": "https://example.test/next"}
}`

// communityOneItem wraps one item literal in a list envelope.
func communityOneItem(item string) string {
	return `{"items":[` + item + `]}`
}

// mustCommunitySample projects a list body with the approved defaults.
func mustCommunitySample(t *testing.T, body string) *CommunitySample {
	t.Helper()
	s, err := ParseCommunitySample([]byte(body), 3263843, CommunityDefaultLimit, CommunitySortMostReactions)
	if err != nil {
		t.Fatalf("ParseCommunitySample: %v", err)
	}
	return s
}

// communitySingleObservation projects exactly one item and returns it.
func communitySingleObservation(t *testing.T, item string) CommunityObservation {
	t.Helper()
	s := mustCommunitySample(t, communityOneItem(item))
	if len(s.Observations) != 1 {
		t.Fatalf("observations = %d, want exactly 1", len(s.Observations))
	}
	return s.Observations[0]
}

// findCommunityParam locates one normalized parameter by name and origin.
func findCommunityParam(obs CommunityObservation, name, origin string) (ObservedParameter, bool) {
	for _, param := range obs.Parameters {
		if param.Name == name && param.Origin == origin {
			return param, true
		}
	}
	return ObservedParameter{}, false
}

// communityParamNames renders the parameter sequence as origin.name so
// order tests read as one comparable list.
func communityParamNames(obs CommunityObservation) []string {
	out := make([]string, 0, len(obs.Parameters))
	for _, param := range obs.Parameters {
		out = append(out, param.Origin+"."+param.Name)
	}
	return out
}

// TestMapCommunitySort pins the reviewed CLI vocabulary and the exact API
// enums; nothing else is registered.
func TestMapCommunitySort(t *testing.T) {
	cases := map[string]string{
		"most-reactions":     CommunitySortMostReactions,
		"Most Reactions":     CommunitySortMostReactions,
		"  MOST-REACTIONS  ": CommunitySortMostReactions,
		"newest":             CommunitySortNewest,
		"Newest":             CommunitySortNewest,
	}
	for input, want := range cases {
		got, ok := MapCommunitySort(input)
		if !ok || got != want {
			t.Errorf("MapCommunitySort(%q) = %q/%v, want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"", "random", "oldest", "most-comments", "most-collected"} {
		if got, ok := MapCommunitySort(input); ok {
			t.Errorf("MapCommunitySort(%q) = %q, want rejection", input, got)
		}
	}
	if choices := CommunitySortChoices(); len(choices) != 2 || choices[0] != "most-reactions" || choices[1] != "newest" {
		t.Errorf("CommunitySortChoices() = %v", choices)
	}
}

// TestParseCommunitySample_SixItemCoverage pins AC-3: returned/withMeta/
// usable = 6/5/2, upstream order, per-item evidence, and presence-only
// continuation.
func TestParseCommunitySample_SixItemCoverage(t *testing.T) {
	s := mustCommunitySample(t, communitySixItemBody)

	if s.SchemaVersion != "civ.recipe/v1" || s.Kind != "communityRecipeSample" {
		t.Errorf("schema/kind = %q/%q", s.SchemaVersion, s.Kind)
	}
	if s.Provenance.Source != "community-images" ||
		s.Provenance.APIPath != "/api/v1/images" ||
		s.Provenance.VersionID != 3263843 {
		t.Errorf("provenance = %+v", s.Provenance)
	}
	if s.Query.VersionID != 3263843 || s.Query.Type != "image" ||
		s.Query.Sort != CommunitySortMostReactions || s.Query.Period != "AllTime" ||
		s.Query.Limit != CommunityDefaultLimit {
		t.Errorf("query = %+v", s.Query)
	}
	if s.Coverage.RequestedLimit != 20 || s.Coverage.Returned != 6 ||
		s.Coverage.WithMeta != 5 || s.Coverage.Usable != 2 ||
		!s.Coverage.ContinuationOffered {
		t.Errorf("coverage = %+v", s.Coverage)
	}
	if len(s.Observations) != 6 {
		t.Fatalf("observations = %d, want 6", len(s.Observations))
	}
	for i, obs := range s.Observations {
		if obs.UpstreamPosition != i+1 {
			t.Errorf("observation %d upstreamPosition = %d", i, obs.UpstreamPosition)
		}
		if obs.Source != "community-images" {
			t.Errorf("observation %d source = %q", i, obs.Source)
		}
	}

	first := s.Observations[0]
	if first.ImageID == nil || *first.ImageID != 1 || first.MetadataStatus != recipeStatusNotProvided || first.Usable {
		t.Errorf("null-meta observation = %+v", first)
	}
	if first.Reactions == nil || first.Reactions.Like == nil || *first.Reactions.Like != 3 ||
		first.Reactions.Dislike == nil || *first.Reactions.Dislike != 0 ||
		first.Reactions.Comment == nil || *first.Reactions.Comment != 2 {
		t.Errorf("reactions = %+v", first.Reactions)
	}
	if first.PageURL == nil || *first.PageURL != "https://civitai.com/images/1" {
		t.Errorf("page url = %v", first.PageURL)
	}
	if _, ok := findCommunityParam(first, "seed", communityOriginMeta); ok {
		t.Errorf("null meta must not produce parameters: %+v", first.Parameters)
	}
	if len(first.ModelVersionIDs) != 0 || len(first.Resources) != 0 {
		t.Errorf("null meta must not fabricate model versions or resources: %+v", first)
	}

	branch := s.Observations[1]
	if branch.MetadataStatus != recipeStatusAvailable || branch.Usable || len(branch.Parameters) != 0 {
		t.Errorf("empty-object meta observation = %+v", branch)
	}

	promptOnly := s.Observations[2]
	if promptOnly.MetadataStatus != recipeStatusAvailable || promptOnly.Usable {
		t.Errorf("prompt-only observation must not be usable: %+v", promptOnly)
	}
	if len(promptOnly.Parameters) != 0 {
		t.Errorf("prompt-only meta must not enter parameters: %+v", promptOnly.Parameters)
	}

	scalar := s.Observations[3]
	if !scalar.Usable {
		t.Errorf("steps plus identified resource must be usable: %+v", scalar)
	}
	steps, ok := findCommunityParam(scalar, "steps", communityOriginMeta)
	if !ok || !reflect.DeepEqual(steps.Value, int64(20)) {
		t.Errorf("steps parameter = %+v (present %v)", steps, ok)
	}
	if len(scalar.Resources) != 1 || scalar.Resources[0].Origin != "civitaiResources" ||
		scalar.Resources[0].Position != 0 || scalar.Resources[0].ModelVersionID == nil ||
		*scalar.Resources[0].ModelVersionID != 249861 {
		t.Errorf("civitaiResources = %+v", scalar.Resources)
	}
	if scalar.Resources[0].Weight != nil {
		t.Errorf("absent weight must stay null, got %v", scalar.Resources[0].Weight)
	}

	legacy := s.Observations[4]
	if !legacy.Usable || len(legacy.Resources) != 1 {
		t.Fatalf("legacy resource observation = %+v", legacy)
	}
	resource := legacy.Resources[0]
	if resource.Origin != "resources" || resource.Position != 0 ||
		resource.Name == nil || *resource.Name != "Detail LoRA" ||
		resource.Weight == nil || *resource.Weight != 0.65 {
		t.Errorf("legacy resource = %+v", resource)
	}

	unknownOnly := s.Observations[5]
	if unknownOnly.Usable || len(unknownOnly.Parameters) != 0 || unknownOnly.MetadataStatus != recipeStatusAvailable {
		t.Errorf("unknown-only observation = %+v", unknownOnly)
	}
	if len(unknownOnly.Issues) != 0 {
		t.Errorf("unknown keys must be ignored, got issues %+v", unknownOnly.Issues)
	}
}

// TestParseCommunitySample_EmptySample pins the successful zero-coverage
// sample: empty items, non-nil arrays, and no continuation claim.
func TestParseCommunitySample_EmptySample(t *testing.T) {
	for name, body := range map[string]string{
		"empty array":  `{"items":[],"metadata":{}}`,
		"absent items": `{"metadata":{}}`,
		"null items":   `{"items":null}`,
		"no metadata":  `{"items":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := mustCommunitySample(t, body)
			if s.Coverage.RequestedLimit != 20 || s.Coverage.Returned != 0 ||
				s.Coverage.WithMeta != 0 || s.Coverage.Usable != 0 ||
				s.Coverage.ContinuationOffered {
				t.Errorf("empty coverage = %+v", s.Coverage)
			}
			if s.Observations == nil || len(s.Observations) != 0 {
				t.Errorf("observations must be non-nil empty, got %#v", s.Observations)
			}
			out := marshalRecipe(t, s)
			if !strings.Contains(out, `"observations":[]`) {
				t.Errorf("empty sample must serialize observations as []: %s", out)
			}
			if strings.Contains(out, "nextCursor") || strings.Contains(out, "nextPage") {
				t.Errorf("continuation token must never be emitted: %s", out)
			}
		})
	}
}

// TestParseCommunitySample_Validation rejects contract violations instead
// of clamping them into an unrequested sample shape.
func TestParseCommunitySample_Validation(t *testing.T) {
	body := []byte(`{"items":[]}`)
	cases := []struct {
		name    string
		version int
		limit   int
		sort    string
	}{
		{"zero version", 0, 20, CommunitySortMostReactions},
		{"negative version", -1, 20, CommunitySortMostReactions},
		{"zero limit", 3263843, 0, CommunitySortMostReactions},
		{"limit above max", 3263843, 101, CommunitySortMostReactions},
		{"unsupported sort", 3263843, 20, "Random"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseCommunitySample(body, tc.version, tc.limit, tc.sort); err == nil {
				t.Fatalf("%s: expected an error", tc.name)
			}
		})
	}
}

// TestParseCommunitySample_MalformedEnvelope keeps malformed envelopes as
// errors while valid sparse payloads stay successful.
func TestParseCommunitySample_MalformedEnvelope(t *testing.T) {
	for name, body := range map[string]string{
		"not json":      `nope`,
		"array body":    `[]`,
		"items object":  `{"items":{}}`,
		"items string":  `{"items":"x"}`,
		"null body":     `null`,
		"truncated doc": `{"items":[`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCommunitySample([]byte(body), 3263843, 20, CommunitySortMostReactions); err == nil {
				t.Fatalf("%s: expected an envelope error", name)
			}
		})
	}
}

// TestCommunityScalarNormalization pins the shared-normalizer behavior on
// the community allowlist: numeric strings and numbers agree, seed stays
// an exact decimal string, null/empty/invalid shapes stay distinct, and
// sibling fields survive one bad field.
func TestCommunityScalarNormalization(t *testing.T) {
	cases := []struct {
		name       string
		meta       string
		want       map[string]any
		wantIssues []RecipeIssue
	}{
		{"steps numeric string", `{"steps":"20"}`, map[string]any{"steps": int64(20)}, nil},
		{"steps number", `{"steps":20}`, map[string]any{"steps": int64(20)}, nil},
		{"steps integral float", `{"steps":20.0}`, map[string]any{"steps": int64(20)}, nil},
		{"steps fraction invalid", `{"steps":20.5}`, map[string]any{"steps": nil},
			[]RecipeIssue{{Field: "parameters.steps", Reason: recipeReasonInvalidNumber}}},
		{"steps null stays null", `{"steps":null}`, map[string]any{"steps": nil}, nil},
		{"steps empty string invalid", `{"steps":""}`, map[string]any{"steps": nil},
			[]RecipeIssue{{Field: "parameters.steps", Reason: recipeReasonInvalidNumber}}},
		{"steps object unsupported", `{"steps":{"n":20}}`, map[string]any{"steps": nil},
			[]RecipeIssue{{Field: "parameters.steps", Reason: recipeReasonUnsupportedShape}}},
		{"steps boolean unsupported", `{"steps":true}`, map[string]any{"steps": nil},
			[]RecipeIssue{{Field: "parameters.steps", Reason: recipeReasonUnsupportedShape}}},
		{"cfgScale zero stays zero", `{"cfgScale":0}`, map[string]any{"cfgScale": float64(0)}, nil},
		{"cfgScale numeric string", `{"cfgScale":"0"}`, map[string]any{"cfgScale": float64(0)}, nil},
		{"seed number exact", `{"seed":866354325141838}`, map[string]any{"seed": "866354325141838"}, nil},
		{"seed big number exact", `{"seed":9007199254740993}`, map[string]any{"seed": "9007199254740993"}, nil},
		{"seed string exact", `{"seed":"866354325141838"}`, map[string]any{"seed": "866354325141838"}, nil},
		{"seed fraction invalid", `{"seed":1.5}`, map[string]any{"seed": nil},
			[]RecipeIssue{{Field: "parameters.seed", Reason: recipeReasonInvalidNumber}}},
		{"sampler empty stays empty", `{"sampler":""}`, map[string]any{"sampler": ""}, nil},
		{"sampler null stays null", `{"sampler":null}`, map[string]any{"sampler": nil}, nil},
		{"sampler number unsupported", `{"sampler":5}`, map[string]any{"sampler": nil},
			[]RecipeIssue{{Field: "parameters.sampler", Reason: recipeReasonUnsupportedShape}}},
		{"clipSkip numeric string", `{"clipSkip":"2"}`, map[string]any{"clipSkip": int64(2)}, nil},
		{"denoisingStrength numeric string", `{"denoisingStrength":"0.35"}`, map[string]any{"denoisingStrength": float64(0.35)}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := communitySingleObservation(t, `{"id":1001,"meta":`+tc.meta+`}`)
			for name, want := range tc.want {
				param, ok := findCommunityParam(obs, name, communityOriginMeta)
				if !ok {
					t.Fatalf("parameter %s missing; parameters = %+v", name, obs.Parameters)
				}
				if !reflect.DeepEqual(param.Value, want) {
					t.Errorf("parameter %s = %#v (%T), want %#v", name, param.Value, param.Value, want)
				}
			}
			for _, want := range tc.wantIssues {
				if !hasIssue(obs.Issues, want.Field, want.Reason) {
					t.Errorf("missing issue %s %s; issues = %+v", want.Field, want.Reason, obs.Issues)
				}
			}
		})
	}

	t.Run("absent keys are omitted", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1001,"meta":{"steps":1}}`)
		if _, ok := findCommunityParam(obs, "sampler", communityOriginMeta); ok {
			t.Errorf("absent sampler must not appear: %+v", obs.Parameters)
		}
		if names := communityParamNames(obs); len(names) != 1 || names[0] != "meta.steps" {
			t.Errorf("parameters = %v, want only meta.steps", names)
		}
	})

	t.Run("bad field does not abort sibling fields", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1001,"meta":{"steps":{"bad":1},"sampler":"Euler"}}`)
		sampler, ok := findCommunityParam(obs, "sampler", communityOriginMeta)
		if !ok || sampler.Value != "Euler" {
			t.Errorf("sibling sampler lost: %+v", obs.Parameters)
		}
		if !hasIssue(obs.Issues, "parameters.steps", recipeReasonUnsupportedShape) {
			t.Errorf("steps issue missing: %+v", obs.Issues)
		}
	})
}

// TestCommunityParameterDimensionsAndOrigins proves top-level dimensions
// supplement rather than overwrite metadata dimensions, with explicit
// origins in a fixed order.
func TestCommunityParameterDimensionsAndOrigins(t *testing.T) {
	obs := communitySingleObservation(t,
		`{"id":1001,"width":640,"height":480,"meta":{"seed":42,"width":512,"height":768}}`)
	names := communityParamNames(obs)
	want := []string{
		"meta.seed", "meta.width", "meta.height", "topLevel.width", "topLevel.height",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("parameter order = %v, want %v", names, want)
	}
	metaWidth, _ := findCommunityParam(obs, "width", communityOriginMeta)
	topWidth, _ := findCommunityParam(obs, "width", communityOriginTopLevel)
	if !reflect.DeepEqual(metaWidth.Value, int64(512)) || !reflect.DeepEqual(topWidth.Value, int64(640)) {
		t.Errorf("dimensions overwrote each other: meta %v top %v", metaWidth.Value, topWidth.Value)
	}

	topOnly := communitySingleObservation(t, `{"id":1002,"width":640,"height":480,"meta":null}`)
	if !topOnly.Usable {
		t.Errorf("top-level dimensions are supported scalar observations: %+v", topOnly)
	}
	if topOnly.MetadataStatus != recipeStatusNotProvided {
		t.Errorf("top-level dimensions must not be described as metadata: %+v", topOnly)
	}
}

// TestCommunityResourceStack pins AC-6: both origins, stack order,
// duplicates, unknown types, identity fields, and normalized weights with
// no inferred value.
func TestCommunityResourceStack(t *testing.T) {
	obs := communitySingleObservation(t, `{"id":1001,"meta":{
		"civitaiResources":[
			{"type":"checkpoint","modelVersionId":"249861","modelName":"ignored"},
			{"type":"lora","modelVersionId":249862,"weight":"0.65"},
			{"type":"customThing","name":"Custom"}
		],
		"resources":[
			{"type":"lora","name":"Legacy LoRA","hash":"ABC123","weight":0.65},
			{"name":"Duplicate","hash":"ABC123"}
		]}}`)
	if !obs.Usable {
		t.Fatalf("identified resources must be usable: %+v", obs)
	}
	if len(obs.Resources) != 5 {
		t.Fatalf("resources = %d, want 5", len(obs.Resources))
	}
	origins := []string{"civitaiResources", "civitaiResources", "civitaiResources", "resources", "resources"}
	positions := []int{0, 1, 2, 0, 1}
	for i, resource := range obs.Resources {
		if resource.Origin != origins[i] || resource.Position != positions[i] {
			t.Errorf("resource %d origin/position = %s/%d, want %s/%d",
				i, resource.Origin, resource.Position, origins[i], positions[i])
		}
	}

	checkpoint := obs.Resources[0]
	if checkpoint.Type == nil || *checkpoint.Type != "checkpoint" ||
		checkpoint.ModelVersionID == nil || *checkpoint.ModelVersionID != 249861 ||
		checkpoint.Name != nil || checkpoint.Hash != nil || checkpoint.Weight != nil {
		t.Errorf("checkpoint entry = %+v", checkpoint)
	}
	lora := obs.Resources[1]
	if lora.Type == nil || *lora.Type != "lora" ||
		lora.ModelVersionID == nil || *lora.ModelVersionID != 249862 ||
		lora.Weight == nil || *lora.Weight != 0.65 {
		t.Errorf("numeric-string weight entry = %+v", lora)
	}
	unknown := obs.Resources[2]
	if unknown.Type == nil || *unknown.Type != "customThing" ||
		unknown.Name == nil || *unknown.Name != "Custom" || unknown.Weight != nil {
		t.Errorf("unknown-type entry = %+v", unknown)
	}
	legacy := obs.Resources[3]
	if legacy.Type == nil || *legacy.Type != "lora" ||
		legacy.Name == nil || *legacy.Name != "Legacy LoRA" ||
		legacy.Hash == nil || *legacy.Hash != "ABC123" ||
		legacy.Weight == nil || *legacy.Weight != 0.65 {
		t.Errorf("legacy weighted entry = %+v", legacy)
	}
	if obs.Resources[3].Weight == nil || obs.Resources[1].Weight == nil ||
		*obs.Resources[3].Weight != *obs.Resources[1].Weight {
		t.Errorf("string and numeric weights must normalize equally: %v vs %v",
			obs.Resources[1].Weight, obs.Resources[3].Weight)
	}
	duplicate := obs.Resources[4]
	if duplicate.ModelVersionID != nil || duplicate.Name == nil || *duplicate.Name != "Duplicate" ||
		duplicate.Hash == nil || *duplicate.Hash != "ABC123" || duplicate.Weight != nil {
		t.Errorf("duplicate-name entry = %+v", duplicate)
	}
}

// TestCommunityMalformedResourceEntries pins per-entry degradation: bad
// members never abort the sample or invent identity/weight.
func TestCommunityMalformedResourceEntries(t *testing.T) {
	t.Run("non-array resources", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1,"meta":{"resources":{}}}`)
		if len(obs.Resources) != 0 || !hasIssue(obs.Issues, "resources", recipeReasonUnsupportedShape) {
			t.Errorf("non-array resources = %+v issues %+v", obs.Resources, obs.Issues)
		}
		if obs.Usable {
			t.Errorf("malformed resources must not be usable")
		}
	})

	t.Run("non-object entries keep original positions", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1,"meta":{"resources":[42,null,{"name":"Kept"}]}}`)
		if len(obs.Resources) != 1 {
			t.Fatalf("resources = %+v, want one surviving entry", obs.Resources)
		}
		kept := obs.Resources[0]
		if kept.Position != 2 || kept.Name == nil || *kept.Name != "Kept" {
			t.Errorf("surviving entry = %+v", kept)
		}
		if !hasIssue(obs.Issues, "resources[0]", recipeReasonUnsupportedShape) {
			t.Errorf("missing issue for malformed entry: %+v", obs.Issues)
		}
	})

	t.Run("invalid weight and type", func(t *testing.T) {
		obs := communitySingleObservation(t,
			`{"id":1,"meta":{"resources":[{"name":"X","weight":{"bad":1},"type":5}]}}`)
		if len(obs.Resources) != 1 {
			t.Fatalf("resources = %+v", obs.Resources)
		}
		resource := obs.Resources[0]
		if resource.Weight != nil {
			t.Errorf("invalid weight must stay null, got %v", resource.Weight)
		}
		if resource.Type != nil {
			t.Errorf("non-string type must stay null, got %v", resource.Type)
		}
		if !hasIssue(obs.Issues, "resources[0].weight", recipeReasonUnsupportedShape) ||
			!hasIssue(obs.Issues, "resources[0].type", recipeReasonUnsupportedShape) {
			t.Errorf("issues = %+v", obs.Issues)
		}
		if !obs.Usable {
			t.Errorf("the surviving name is identity evidence")
		}
	})

	t.Run("type and weight alone are not identity", func(t *testing.T) {
		obs := communitySingleObservation(t,
			`{"id":1,"meta":{"resources":[{"type":"lora","weight":0.5}]}}`)
		if obs.Usable {
			t.Errorf("type/weight-only entry must not be usable: %+v", obs)
		}
	})
}

// TestCommunityModelVersionIDsProvenance pins that top-level ids stay
// provenance and never become a resource stack or usability evidence.
func TestCommunityModelVersionIDsProvenance(t *testing.T) {
	obs := communitySingleObservation(t,
		`{"id":7,"modelVersionIds":[3263843,"3263844",null,{"bad":1}],"meta":null}`)
	if len(obs.Resources) != 0 {
		t.Errorf("top-level ids must not fabricate resources: %+v", obs.Resources)
	}
	if obs.Usable {
		t.Errorf("top-level ids alone must not be usable")
	}
	if !reflect.DeepEqual(obs.ModelVersionIDs, []int64{3263843, 3263844}) {
		t.Errorf("modelVersionIds = %v", obs.ModelVersionIDs)
	}
	if !hasIssue(obs.Issues, "modelVersionIds[3]", recipeReasonUnsupportedShape) {
		t.Errorf("malformed id element issue missing: %+v", obs.Issues)
	}
	out := marshalRecipe(t, obs)
	if !strings.Contains(out, `"resources":[]`) {
		t.Errorf("empty resources must serialize as []: %s", out)
	}
}

// TestCommunityMetadataStatus pins withMeta and the per-item degradation
// of non-object metadata.
func TestCommunityMetadataStatus(t *testing.T) {
	cases := []struct {
		name       string
		item       string
		wantStatus string
		wantMeta   int
		wantIssue  bool
	}{
		{"null meta", `{"id":1,"meta":null}`, recipeStatusNotProvided, 0, false},
		{"absent meta", `{"id":1}`, recipeStatusNotProvided, 0, false},
		{"empty object", `{"id":1,"meta":{}}`, recipeStatusAvailable, 1, false},
		{"object", `{"id":1,"meta":{"steps":20}}`, recipeStatusAvailable, 1, false},
		{"string meta", `{"id":1,"meta":"sentinel-meta-text"}`, recipeStatusUnsupported, 0, true},
		{"array meta", `{"id":1,"meta":[1,2]}`, recipeStatusUnsupported, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustCommunitySample(t, communityOneItem(tc.item))
			obs := s.Observations[0]
			if obs.MetadataStatus != tc.wantStatus {
				t.Errorf("metadataStatus = %q, want %q", obs.MetadataStatus, tc.wantStatus)
			}
			if s.Coverage.WithMeta != tc.wantMeta {
				t.Errorf("withMeta = %d, want %d", s.Coverage.WithMeta, tc.wantMeta)
			}
			if got := hasIssue(obs.Issues, "meta", recipeReasonUnsupportedShape); got != tc.wantIssue {
				t.Errorf("meta issue present = %v, want %v (issues %+v)", got, tc.wantIssue, obs.Issues)
			}
		})
	}
}

// TestCommunityReactions pins separate reaction/comment counters and the
// absent-versus-zero distinction; no aggregate score exists.
func TestCommunityReactions(t *testing.T) {
	t.Run("all counters", func(t *testing.T) {
		obs := communitySingleObservation(t,
			`{"id":1,"stats":{"likeCount":3,"dislikeCount":0,"heartCount":1,"laughCount":2,"cryCount":4,"commentCount":5}}`)
		r := obs.Reactions
		if r == nil {
			t.Fatal("reactions must be present")
		}
		checks := map[string]struct {
			got  *int64
			want int64
		}{
			"like":    {r.Like, 3},
			"dislike": {r.Dislike, 0},
			"heart":   {r.Heart, 1},
			"laugh":   {r.Laugh, 2},
			"cry":     {r.Cry, 4},
			"comment": {r.Comment, 5},
		}
		for name, check := range checks {
			if check.got == nil || *check.got != check.want {
				t.Errorf("%s = %v, want %d", name, check.got, check.want)
			}
		}
	})

	t.Run("absent stats", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1}`)
		if obs.Reactions != nil {
			t.Errorf("absent stats must stay null, got %+v", obs.Reactions)
		}
	})

	t.Run("empty stats object", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1,"stats":{}}`)
		if obs.Reactions == nil {
			t.Fatal("present stats object must yield a block")
		}
		if obs.Reactions.Like != nil || obs.Reactions.Comment != nil {
			t.Errorf("absent counters stay null: %+v", obs.Reactions)
		}
	})

	t.Run("invalid counter keeps siblings", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1,"stats":{"likeCount":"many","heartCount":0}}`)
		if obs.Reactions == nil || obs.Reactions.Like != nil {
			t.Fatalf("invalid likeCount = %+v", obs.Reactions)
		}
		if obs.Reactions.Heart == nil || *obs.Reactions.Heart != 0 {
			t.Errorf("heartCount = %v, want observed zero", obs.Reactions.Heart)
		}
		if !hasIssue(obs.Issues, "stats.likeCount", recipeReasonInvalidNumber) {
			t.Errorf("issues = %+v", obs.Issues)
		}
	})

	t.Run("non-object stats", func(t *testing.T) {
		obs := communitySingleObservation(t, `{"id":1,"stats":[]}`)
		if obs.Reactions != nil || !hasIssue(obs.Issues, "stats", recipeReasonUnsupportedShape) {
			t.Errorf("non-object stats = %+v issues %+v", obs.Reactions, obs.Issues)
		}
	})
}

// TestCommunityListRedactionAndContract proves every list output is
// large-field-free, keeps media locations out, carries the mandatory
// coverage/query keys, and has no aggregation/quality fields.
func TestCommunityListRedactionAndContract(t *testing.T) {
	s := mustCommunitySample(t, communitySixItemBody)
	out := marshalRecipe(t, s)

	assertNoJSONKeys(t, s,
		"prompt", "negativePrompt", "meta", "comfy", "workflow", "description",
		"url", "thumbnailUrl", "blurHash", "nextCursor", "nextPage",
		"average", "median", "frequency", "rank", "ranking", "score", "quality",
		"recommendation", "recommended", "best", "topPick", "histogram")
	assertNoSourceText(t, s,
		"sentinel-prompt-C", "sentinel-negative-C", "sentinel-unknown",
		"sentinel-media", "sentinel-blurhash", "sentinel-thumb",
		"sentinel-cursor", "example.test/next")

	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("sample JSON must decode: %v", err)
	}
	coverage, ok := doc["coverage"].(map[string]any)
	if !ok {
		t.Fatalf("coverage block missing: %s", out)
	}
	for _, key := range []string{"requestedLimit", "returned", "withMeta", "usable", "continuationOffered"} {
		if _, ok := coverage[key]; !ok {
			t.Errorf("coverage missing mandatory key %q", key)
		}
	}
	query, ok := doc["query"].(map[string]any)
	if !ok {
		t.Fatalf("query block missing: %s", out)
	}
	for _, key := range []string{"versionId", "type", "sort", "period", "limit"} {
		if _, ok := query[key]; !ok {
			t.Errorf("query missing mandatory key %q", key)
		}
	}
	if strings.Contains(strings.ToLower(out), "recommended") || strings.Contains(strings.ToLower(out), "quality score") {
		t.Errorf("list output contains recommendation language: %s", out)
	}
}

// TestCommunityObservationIssuesNeverContainSourceText guards the issue
// vocabulary: structured issues carry paths and reasons only, never the
// offending free-form value.
func TestCommunityObservationIssuesNeverContainSourceText(t *testing.T) {
	obs := communitySingleObservation(t,
		`{"id":1,"meta":{"steps":{"sentinel-object":"sentinel-inside"},"prompt":"sentinel-prompt"}}`)
	assertNoSourceText(t, obs, "sentinel-object", "sentinel-inside", "sentinel-prompt")
}

// TestParseCommunityImageDetail_Match pins the successful exact-image
// detail: one matching item, selected prompts only, normalized parameters
// and resources, and no list-query provenance.
func TestParseCommunityImageDetail_Match(t *testing.T) {
	body := `{"items":[{
		"id": 1001, "postId": 55, "username": "alice", "createdAt": "2026-01-02T03:04:05Z",
		"type": "image", "baseModel": "SD 1.5", "nsfwLevel": "Soft", "browsingLevel": 1,
		"modelVersionIds": [3263843],
		"stats": {"likeCount": 3, "dislikeCount": 0, "heartCount": 1, "laughCount": 0, "cryCount": 0, "commentCount": 2},
		"meta": {
			"prompt": "detail-prompt", "negativePrompt": "detail-negative",
			"seed": 866354325141838, "steps": "20", "cfgScale": 0, "clipSkip": 2,
			"denoisingStrength": "0.35",
			"resources": [{"type": "lora", "name": "Detail LoRA", "weight": "0.65"}],
			"comfy": "sentinel-comfy", "workflow": "sentinel-workflow"
		}
	}]}`
	d, err := ParseCommunityImageDetail([]byte(body), 1001)
	if err != nil {
		t.Fatalf("ParseCommunityImageDetail: %v", err)
	}
	if d.SchemaVersion != "civ.recipe/v1" || d.Kind != "communityImageDetail" {
		t.Errorf("schema/kind = %q/%q", d.SchemaVersion, d.Kind)
	}
	if d.Provenance.Source != "community-images" || d.Provenance.APIPath != "/api/v1/images" ||
		d.Provenance.ImageID != 1001 {
		t.Errorf("provenance = %+v", d.Provenance)
	}
	image := d.Image
	if image == nil {
		t.Fatal("detail image is missing")
	}
	if image.ImageID == nil || *image.ImageID != 1001 ||
		image.PageURL == nil || *image.PageURL != "https://civitai.com/images/1001" {
		t.Errorf("identity = %+v", image.CommunityImageIdentity)
	}
	if image.MetadataStatus != recipeStatusAvailable ||
		image.PromptStatus != recipeStatusProvided ||
		image.NegativePromptStatus != recipeStatusProvided {
		t.Errorf("statuses = %q/%q/%q", image.MetadataStatus, image.PromptStatus, image.NegativePromptStatus)
	}
	if image.TextParameters == nil || image.TextParameters.Prompt == nil ||
		*image.TextParameters.Prompt != "detail-prompt" ||
		image.TextParameters.NegativePrompt == nil ||
		*image.TextParameters.NegativePrompt != "detail-negative" {
		t.Errorf("text parameters = %+v", image.TextParameters)
	}
	checks := map[string]any{
		"seed":              "866354325141838",
		"steps":             int64(20),
		"cfgScale":          float64(0),
		"clipSkip":          int64(2),
		"denoisingStrength": float64(0.35),
	}
	for name, want := range checks {
		param, ok := findCommunityParam(CommunityObservation{Parameters: image.Parameters}, name, communityOriginMeta)
		if !ok || !reflect.DeepEqual(param.Value, want) {
			t.Errorf("detail parameter %s = %+v (present %v), want %#v", name, param.Value, ok, want)
		}
	}
	if len(image.Resources) != 1 || image.Resources[0].Weight == nil || *image.Resources[0].Weight != 0.65 {
		t.Errorf("detail resources = %+v", image.Resources)
	}

	out := marshalRecipe(t, d)
	assertNoJSONKeys(t, d, "examples", "observations", "meta", "comfy", "workflow",
		"description", "url", "thumbnailUrl", "blurHash", "versionId", "sort", "period", "modelId")
	assertNoSourceText(t, d, "sentinel-comfy", "sentinel-workflow")
	if !strings.Contains(out, "detail-prompt") {
		t.Errorf("selected prompt must be present in the detail view: %s", out)
	}
}

// TestParseCommunityImageDetail_Unavailable pins detail-unavailable
// outcomes: zero, multiple, mismatched, or non-object items never broaden
// the request and never return partial detail.
func TestParseCommunityImageDetail_Unavailable(t *testing.T) {
	cases := map[string]string{
		"empty items":       `{"items":[]}`,
		"null items":        `{"items":null}`,
		"absent items":      `{}`,
		"mismatched id":     `{"items":[{"id":999}]}`,
		"multiple matches":  `{"items":[{"id":1001},{"id":1001}]}`,
		"multiple mismatch": `{"items":[{"id":999},{"id":998}]}`,
		"non-object item":   `{"items":[42]}`,
		"null item":         `{"items":[null]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseCommunityImageDetail([]byte(body), 1001)
			if err == nil {
				t.Fatalf("%s: expected detail-unavailable", name)
			}
			if !strings.Contains(err.Error(), "unavailable") {
				t.Errorf("%s: error = %v, want a detail-unavailable message", name, err)
			}
		})
	}

	t.Run("invalid image id", func(t *testing.T) {
		if _, err := ParseCommunityImageDetail([]byte(`{"items":[]}`), 0); err == nil {
			t.Fatal("non-positive image id must fail")
		}
	})
}

// TestParseCommunityImageDetail_NullMeta keeps a visible-but-metadata-less
// image as a successful detail with explicit missing text semantics.
func TestParseCommunityImageDetail_NullMeta(t *testing.T) {
	d, err := ParseCommunityImageDetail([]byte(`{"items":[{"id":1001,"meta":null}]}`), 1001)
	if err != nil {
		t.Fatalf("ParseCommunityImageDetail: %v", err)
	}
	image := d.Image
	if image.MetadataStatus != recipeStatusNotProvided ||
		image.PromptStatus != recipeStatusNotProvided ||
		image.NegativePromptStatus != recipeStatusNotProvided {
		t.Errorf("statuses = %q/%q/%q", image.MetadataStatus, image.PromptStatus, image.NegativePromptStatus)
	}
	if image.TextParameters != nil {
		t.Errorf("no metadata must not yield text parameters: %+v", image.TextParameters)
	}
	if len(image.Issues) != 0 {
		t.Errorf("null meta is absence, not an issue: %+v", image.Issues)
	}
}

// TestParseCommunityImageDetail_MalformedEnvelope keeps malformed bodies
// as errors, distinct from a valid but unavailable selection.
func TestParseCommunityImageDetail_MalformedEnvelope(t *testing.T) {
	for name, body := range map[string]string{
		"not json":     `nope`,
		"array body":   `[]`,
		"items object": `{"items":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCommunityImageDetail([]byte(body), 1001); err == nil {
				t.Fatalf("%s: expected an envelope error", name)
			}
		})
	}
}

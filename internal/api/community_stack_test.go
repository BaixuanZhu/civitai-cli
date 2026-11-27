package api

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// mustCommunityStackSummary projects a list body into the aggregate view
// with the approved defaults.
func mustCommunityStackSummary(t *testing.T, body string) *CommunityStackSummary {
	t.Helper()
	s, err := ParseCommunityStackSummary([]byte(body), 3263843, CommunityDefaultLimit, CommunitySortMostReactions)
	if err != nil {
		t.Fatalf("ParseCommunityStackSummary: %v", err)
	}
	return s
}

// stackIdentityText renders one identity as type|basis|value so order and
// merge assertions read as one comparable string.
func stackIdentityText(id CommunityResourceIdentity) string {
	typ := "<null>"
	if id.Type != nil {
		typ = *id.Type
	}
	switch id.Basis {
	case communityStackBasisModelVersionID:
		return fmt.Sprintf("%s|modelVersionId|%d", typ, *id.ModelVersionID)
	case communityStackBasisHash:
		return fmt.Sprintf("%s|hash|%s", typ, *id.Hash)
	default:
		return fmt.Sprintf("%s|name|%s", typ, *id.Name)
	}
}

// stackUsageTexts renders usage rows as "identity=count" in emitted order.
func stackUsageTexts(top []CommunityResourceUsage) []string {
	out := make([]string, 0, len(top))
	for _, usage := range top {
		out = append(out, fmt.Sprintf("%s=%d", stackIdentityText(usage.Identity), usage.ImageCount))
	}
	return out
}

// stackLoRAUsageTexts renders LoRA rows as "identity=count" in emitted order.
func stackLoRAUsageTexts(top []CommunityLoRAUsage) []string {
	out := make([]string, 0, len(top))
	for _, usage := range top {
		out = append(out, fmt.Sprintf("%s=%d", stackIdentityText(usage.Identity), usage.ImageCount))
	}
	return out
}

// stackValueTexts renders distribution rows as "value=count" in emitted order.
func stackValueTexts(top []CommunityValueCount) []string {
	out := make([]string, 0, len(top))
	for _, row := range top {
		out = append(out, fmt.Sprintf("%v=%d", row.Value, row.ImageCount))
	}
	return out
}

// findLoRAUsage locates one LoRA row by its exact identity basis value.
func findLoRAUsage(top []CommunityLoRAUsage, basis, value string) (CommunityLoRAUsage, bool) {
	for _, usage := range top {
		if usage.Identity.Basis != basis {
			continue
		}
		switch basis {
		case communityStackBasisName:
			if usage.Identity.Name != nil && *usage.Identity.Name == value {
				return usage, true
			}
		case communityStackBasisHash:
			if usage.Identity.Hash != nil && *usage.Identity.Hash == value {
				return usage, true
			}
		}
	}
	return CommunityLoRAUsage{}, false
}

// assertStackMedian checks a nullable median against an exact binary-safe
// expectation with a tiny epsilon for decimal inputs.
func assertStackMedian(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("median = null, want %v", want)
	}
	if math.Abs(*got-want) > 1e-12 {
		t.Errorf("median = %v, want %v", *got, want)
	}
}

// TestParseCommunityStackSummary_ContractAndCoverage pins the envelope: the
// stack summary shares the exact provenance/query/coverage of the list
// projection, carries the fixed caveats and top limit, and has no
// observations member.
func TestParseCommunityStackSummary_ContractAndCoverage(t *testing.T) {
	sample := mustCommunitySample(t, communitySixItemBody)
	s := mustCommunityStackSummary(t, communitySixItemBody)

	if s.SchemaVersion != "civ.recipe/v1" || s.Kind != "communityStackSummary" {
		t.Errorf("schema/kind = %q/%q", s.SchemaVersion, s.Kind)
	}
	if !reflect.DeepEqual(s.Provenance, sample.Provenance) {
		t.Errorf("provenance = %+v, want list %+v", s.Provenance, sample.Provenance)
	}
	if !reflect.DeepEqual(s.Query, sample.Query) {
		t.Errorf("query = %+v, want list %+v", s.Query, sample.Query)
	}
	if !reflect.DeepEqual(s.Coverage, sample.Coverage) {
		t.Errorf("coverage = %+v, want list %+v", s.Coverage, sample.Coverage)
	}
	if !s.Coverage.ContinuationOffered {
		t.Errorf("continuation offered must be preserved: %+v", s.Coverage)
	}
	if !reflect.DeepEqual(s.Caveats, []string{"boundedNonRandomSample", "continuationNotFollowed", "descriptiveEvidenceOnly"}) {
		t.Errorf("caveats = %v", s.Caveats)
	}
	if s.TopLimit != 5 {
		t.Errorf("topLimit = %d, want 5", s.TopLimit)
	}

	if s.Resources.Checkpoints.Top == nil || s.Resources.LoRAs.Top == nil || s.Resources.OtherResources.Top == nil {
		t.Errorf("resource top arrays must be non-nil: %+v", s.Resources)
	}
	if s.Parameters.Steps.Top == nil || s.Parameters.CFGScale.Top == nil ||
		s.Parameters.Sampler.Top == nil || s.Parameters.Scheduler.Top == nil {
		t.Errorf("parameter top arrays must be non-nil: %+v", s.Parameters)
	}

	out := marshalRecipe(t, s)
	if strings.Contains(out, `"observations"`) {
		t.Errorf("stack summary must not contain observations: %s", out)
	}
	assertNoJSONKeys(t, s, "observations", "examples", "image", "textParameters",
		"prompt", "negativePrompt", "nextCursor", "nextPage")
	assertNoSourceText(t, s, "sentinel-prompt-C", "sentinel-negative-C", "sentinel-cursor", "example.test/next")

	// The six-item fixture has exactly one identified checkpoint, one
	// identified LoRA with one weight sample, and one steps observation.
	if s.Resources.EntryCount != 2 || s.Resources.IdentifiedEntryCount != 2 ||
		s.Resources.UnidentifiedEntryCount != 0 || s.Resources.ImagesWithEntries != 2 {
		t.Errorf("resource coverage = %+v", s.Resources)
	}
	if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, []string{"checkpoint|modelVersionId|249861=1"}) {
		t.Errorf("checkpoint top = %v", got)
	}
	if got := stackLoRAUsageTexts(s.Resources.LoRAs.Top); !reflect.DeepEqual(got, []string{"lora|name|Detail LoRA=1"}) {
		t.Errorf("lora top = %v", got)
	}
	lora := s.Resources.LoRAs.Top[0]
	assertStackMedian(t, lora.Weight.Median, 0.65)
	if lora.Weight.ImagesWithWeight != 1 || lora.Weight.SampleCount != 1 {
		t.Errorf("lora weight = %+v", lora.Weight)
	}
	if got := stackValueTexts(s.Parameters.Steps.Top); !reflect.DeepEqual(got, []string{"20=1"}) {
		t.Errorf("steps top = %v", got)
	}
	if s.Parameters.Steps.ObservedImages != 1 || s.Parameters.Steps.SampledImages != 6 {
		t.Errorf("steps distribution = %+v", s.Parameters.Steps)
	}
	if s.TextAvailability.PromptStatus.Provided != 1 || s.TextAvailability.PromptStatus.NotProvided != 5 {
		t.Errorf("prompt status = %+v", s.TextAvailability.PromptStatus)
	}
}

// TestCommunityStackResources_DuplicatesUseDistinctImages pins AC-3:
// resource usage counts distinct response positions, never raw entries,
// and neither repeated upstream IDs nor missing IDs change that unit.
func TestCommunityStackResources_DuplicatesUseDistinctImages(t *testing.T) {
	t.Run("duplicates within and across arrays", func(t *testing.T) {
		body := `{"items":[
			{"id":100,"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":777}],
				"resources":[{"type":"checkpoint","modelVersionId":777}]}},
			{"id":101,"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":777}]}}
		]}`
		s := mustCommunityStackSummary(t, body)
		if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, []string{"checkpoint|modelVersionId|777=2"}) {
			t.Fatalf("checkpoint top = %v, want one identity used by 2 images", got)
		}
		group := s.Resources.Checkpoints
		if group.EntryCount != 3 || group.IdentifiedImageCount != 2 {
			t.Errorf("group = %+v, want 3 entries over 2 identified images", group)
		}
		if s.Resources.EntryCount != 3 || s.Resources.IdentifiedEntryCount != 3 || s.Resources.ImagesWithEntries != 2 {
			t.Errorf("resource coverage = %+v", s.Resources)
		}
	})

	t.Run("repeated upstream id stays two positions", func(t *testing.T) {
		body := `{"items":[
			{"id":100,"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":777}]}},
			{"id":100,"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":777}]}}
		]}`
		s := mustCommunityStackSummary(t, body)
		if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, []string{"checkpoint|modelVersionId|777=2"}) {
			t.Fatalf("repeated image id must still count 2 positions, got %v", got)
		}
	})

	t.Run("missing image ids still count", func(t *testing.T) {
		body := `{"items":[
			{"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":777}]}},
			{"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":777}]}}
		]}`
		s := mustCommunityStackSummary(t, body)
		if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, []string{"checkpoint|modelVersionId|777=2"}) {
			t.Fatalf("missing image ids must still count 2 positions, got %v", got)
		}
	})
}

// TestCommunityStackResources_IdentityPrecedenceAndNoCrossBasisMerge pins
// the conservative identity contract: ID > hash > name, fallback on
// non-positive IDs, merge only on the exact same basis, and never across
// bases even when secondary labels match.
func TestCommunityStackResources_IdentityPrecedenceAndNoCrossBasisMerge(t *testing.T) {
	loraTop := func(t *testing.T, items ...string) []CommunityLoRAUsage {
		t.Helper()
		s := mustCommunityStackSummary(t, `{"items":[`+strings.Join(items, ",")+`]}`)
		return s.Resources.LoRAs.Top
	}
	loraItem := func(meta string) string {
		return `{"id":1,"meta":{"civitaiResources":[` + meta + `]}}`
	}

	t.Run("positive id wins over hash and name", func(t *testing.T) {
		top := loraTop(t, loraItem(`{"type":"lora","modelVersionId":5,"hash":"H1","name":"N1"}`))
		if len(top) != 1 {
			t.Fatalf("top = %+v, want one row", top)
		}
		id := top[0].Identity
		if id.Basis != communityStackBasisModelVersionID || id.ModelVersionID == nil || *id.ModelVersionID != 5 ||
			id.Hash != nil || id.Name != nil {
			t.Errorf("identity = %+v, want only the id basis populated", id)
		}
	})

	t.Run("zero id falls back to hash", func(t *testing.T) {
		top := loraTop(t, loraItem(`{"type":"lora","modelVersionId":0,"hash":"H1","name":"N1"}`))
		if len(top) != 1 || top[0].Identity.Basis != communityStackBasisHash ||
			top[0].Identity.Hash == nil || *top[0].Identity.Hash != "H1" ||
			top[0].Identity.ModelVersionID != nil || top[0].Identity.Name != nil {
			t.Fatalf("identity = %+v, want hash fallback", top)
		}
	})

	t.Run("negative id falls back to name", func(t *testing.T) {
		top := loraTop(t, loraItem(`{"type":"lora","modelVersionId":-3,"name":"N1"}`))
		if len(top) != 1 || top[0].Identity.Basis != communityStackBasisName ||
			top[0].Identity.Name == nil || *top[0].Identity.Name != "N1" ||
			top[0].Identity.ModelVersionID != nil || top[0].Identity.Hash != nil {
			t.Fatalf("identity = %+v, want name fallback", top)
		}
	})

	t.Run("same id with conflicting labels merges", func(t *testing.T) {
		top := loraTop(t,
			`{"id":1,"meta":{"civitaiResources":[{"type":"lora","modelVersionId":5,"name":"A"}]}}`,
			`{"id":2,"meta":{"civitaiResources":[{"type":"lora","modelVersionId":5,"name":"B"}]}}`)
		if len(top) != 1 || top[0].ImageCount != 2 {
			t.Fatalf("top = %v, want one merged id row used by 2 images", stackLoRAUsageTexts(top))
		}
		if top[0].Identity.Name != nil || top[0].Identity.Hash != nil {
			t.Errorf("secondary labels must not be emitted for an id identity: %+v", top[0].Identity)
		}
	})

	t.Run("same hash with different names merges", func(t *testing.T) {
		top := loraTop(t,
			`{"id":1,"meta":{"civitaiResources":[{"type":"lora","hash":"H","name":"A"}]}}`,
			`{"id":2,"meta":{"civitaiResources":[{"type":"lora","hash":"H","name":"B"}]}}`)
		if len(top) != 1 || top[0].ImageCount != 2 || top[0].Identity.Basis != communityStackBasisHash {
			t.Fatalf("top = %v, want one merged hash row", stackLoRAUsageTexts(top))
		}
		if top[0].Identity.Name != nil {
			t.Errorf("conflicting display names must not be emitted: %+v", top[0].Identity)
		}
	})

	t.Run("same name with different hashes stays split", func(t *testing.T) {
		top := loraTop(t,
			`{"id":1,"meta":{"civitaiResources":[{"type":"lora","name":"N","hash":"H1"}]}}`,
			`{"id":2,"meta":{"civitaiResources":[{"type":"lora","name":"N","hash":"H2"}]}}`)
		if got := stackLoRAUsageTexts(top); !reflect.DeepEqual(got, []string{"lora|hash|H1=1", "lora|hash|H2=1"}) {
			t.Fatalf("top = %v, want two hash identities", got)
		}
	})

	t.Run("id and name-only with matching label stay separate", func(t *testing.T) {
		top := loraTop(t,
			`{"id":1,"meta":{"civitaiResources":[{"type":"lora","modelVersionId":5,"name":"N"}]}}`,
			`{"id":2,"meta":{"civitaiResources":[{"type":"lora","name":"N"}]}}`)
		if got := stackLoRAUsageTexts(top); !reflect.DeepEqual(got, []string{"lora|modelVersionId|5=1", "lora|name|N=1"}) {
			t.Fatalf("top = %v, want no cross-basis merge", got)
		}
	})

	t.Run("names are case sensitive", func(t *testing.T) {
		top := loraTop(t,
			`{"id":1,"meta":{"civitaiResources":[{"type":"lora","name":"Detail"}]}}`,
			`{"id":2,"meta":{"civitaiResources":[{"type":"lora","name":"detail"}]}}`)
		if got := stackLoRAUsageTexts(top); !reflect.DeepEqual(got, []string{"lora|name|Detail=1", "lora|name|detail=1"}) {
			t.Fatalf("top = %v, want case-sensitive identities", got)
		}
	})
}

// TestCommunityStackResources_NoInferenceAndUnidentifiedCoverage pins that
// no identity is ever invented from top-level ids, the requested version,
// origin, or weight, and that identity-free entries are counted honestly.
func TestCommunityStackResources_NoInferenceAndUnidentifiedCoverage(t *testing.T) {
	body := `{"items":[
		{"id":1,"modelVersionIds":[3263843,777],"meta":{"resources":[
			{"type":"lora","weight":0.5},
			{"type":"checkpoint"},
			{"weight":0.7},
			{"name":"","hash":null,"modelVersionId":null}
		]}},
		{"id":2,"meta":{"resources":[
			{"name":"Kept"},
			{"type":"lora","modelVersionId":null,"name":null,"hash":""}
		]}},
		{"id":3,"meta":{"resources":[42,null,{"name":"Survivor"}]}}
	]}`
	s := mustCommunityStackSummary(t, body)

	if s.Resources.EntryCount != 7 || s.Resources.IdentifiedEntryCount != 2 ||
		s.Resources.UnidentifiedEntryCount != 5 || s.Resources.ImagesWithEntries != 3 {
		t.Errorf("resource coverage = %+v, want 7/2/5 over 3 images", s.Resources)
	}
	if len(s.Resources.Checkpoints.Top) != 0 || s.Resources.Checkpoints.IdentifiedImageCount != 0 {
		t.Errorf("checkpoints must stay empty: %+v", s.Resources.Checkpoints)
	}
	if len(s.Resources.LoRAs.Top) != 0 || s.Resources.LoRAs.EntryCount != 2 {
		t.Errorf("weight-only lora entries must not produce identities or medians: %+v", s.Resources.LoRAs)
	}
	if got := stackUsageTexts(s.Resources.OtherResources.Top); !reflect.DeepEqual(got, []string{"<null>|name|Kept=1", "<null>|name|Survivor=1"}) {
		t.Errorf("other resources top = %v", got)
	}

	// No identity may be inferred from the requested version, top-level
	// modelVersionIds, the origin array, or a weight.
	out := marshalRecipe(t, s)
	if strings.Contains(out, "3263843|") || strings.Contains(out, `"modelVersionId":777`) ||
		strings.Contains(out, `"modelVersionId":3263843`) {
		t.Errorf("identity inferred from version or top-level ids: %s", out)
	}
	if strings.Contains(out, `"median"`) {
		t.Errorf("no lora identity means no median rows: %s", out)
	}
}

// TestCommunityStackResources_TypeGroups pins the reviewed type vocabulary:
// only checkpoint and lora are canonicalized, everything else keeps its
// exact type in the other group, and untyped entries report type null.
func TestCommunityStackResources_TypeGroups(t *testing.T) {
	body := `{"items":[{"id":1,"meta":{"civitaiResources":[
		{"type":"checkpoint","modelVersionId":10},
		{"type":"Checkpoint","modelVersionId":10},
		{"type":" LORA ","modelVersionId":20},
		{"type":"loCon","modelVersionId":30},
		{"type":"lycoris","name":"Ly"},
		{"type":"customThing","name":"Custom"},
		{"modelVersionId":40},
		{"type":"   ","name":"Blank"},
		{"type":"lora","weight":0.9},
		{"type":"customThing","weight":0.8}
	]}}]}`
	s := mustCommunityStackSummary(t, body)

	if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, []string{"checkpoint|modelVersionId|10=1"}) {
		t.Errorf("checkpoints = %v, want case/trim-canonicalized single identity", got)
	}
	if s.Resources.Checkpoints.EntryCount != 2 {
		t.Errorf("checkpoint entries = %d, want 2", s.Resources.Checkpoints.EntryCount)
	}

	if got := stackLoRAUsageTexts(s.Resources.LoRAs.Top); !reflect.DeepEqual(got, []string{"lora|modelVersionId|20=1"}) {
		t.Errorf("loras = %v, want only the canonical lora identity", got)
	}
	lora := s.Resources.LoRAs.Top[0]
	if lora.Weight.Median != nil || lora.Weight.ImagesWithWeight != 0 || lora.Weight.SampleCount != 0 {
		t.Errorf("lora without valid weights must have null median and zero coverage: %+v", lora.Weight)
	}
	if s.Resources.LoRAs.EntryCount != 2 {
		t.Errorf("lora entries = %d, want 2 (identity + weight-only)", s.Resources.LoRAs.EntryCount)
	}

	wantOther := []string{
		"<null>|modelVersionId|40=1",
		"<null>|name|Blank=1",
		"customThing|name|Custom=1",
		"loCon|modelVersionId|30=1",
		"lycoris|name|Ly=1",
	}
	if got := stackUsageTexts(s.Resources.OtherResources.Top); !reflect.DeepEqual(got, wantOther) {
		t.Errorf("other resources = %v, want %v", got, wantOther)
	}
	if s.Resources.OtherResources.EntryCount != 6 || s.Resources.OtherResources.IdentifiedImageCount != 1 {
		t.Errorf("other group = %+v", s.Resources.OtherResources)
	}

	// Unknown and untyped weights are never assumed to be LoRAs, so no
	// median exists anywhere outside the lora group by construction.
	out := marshalRecipe(t, s.Resources.OtherResources)
	if strings.Contains(out, "median") || strings.Contains(out, "weight") {
		t.Errorf("other resources must not carry weight evidence: %s", out)
	}
}

// TestCommunityStackResources_DeterministicTopAndOther pins the total
// order: count descending, then basis precedence, then value, with exactly
// five rows, an omitted-identity count, and byte-identical repeated output.
func TestCommunityStackResources_DeterministicTopAndOther(t *testing.T) {
	body := `{"items":[
		{"id":1,"meta":{"civitaiResources":[{"type":"checkpoint","name":"Zed"}]}},
		{"id":2,"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":900}]}},
		{"id":3,"meta":{"civitaiResources":[{"type":"checkpoint","hash":"H2"}]}},
		{"id":4,"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":100}]}},
		{"id":5,"meta":{"civitaiResources":[{"type":"checkpoint","name":"Alpha"}]}},
		{"id":6,"meta":{"civitaiResources":[{"type":"checkpoint","hash":"H1"}]}},
		{"id":7,"meta":{"civitaiResources":[{"type":"checkpoint","modelVersionId":500}]}}
	]}`
	s := mustCommunityStackSummary(t, body)

	want := []string{
		"checkpoint|modelVersionId|100=1",
		"checkpoint|modelVersionId|500=1",
		"checkpoint|modelVersionId|900=1",
		"checkpoint|hash|H1=1",
		"checkpoint|hash|H2=1",
	}
	if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, want) {
		t.Fatalf("top = %v, want %v", got, want)
	}
	if s.Resources.Checkpoints.OtherIdentityCount != 2 {
		t.Errorf("otherIdentityCount = %d, want 2", s.Resources.Checkpoints.OtherIdentityCount)
	}

	out := marshalRecipe(t, s)
	if strings.Contains(out, "otherImageCount") {
		t.Errorf("omitted identity image counts must never be summed: %s", out)
	}
	again := marshalRecipe(t, mustCommunityStackSummary(t, body))
	if out != again {
		t.Errorf("repeated parse must marshal byte-identically:\n%s\n%s", out, again)
	}
}

// TestCommunityStackLoRAWeights_ValidSamplesCoverageAndMedian pins AC-5:
// only finite normalized weights become samples, equal per-image duplicates
// collapse, different per-image weights stay separate, and the median uses
// the arithmetic mean of the two middle values for even counts.
func TestCommunityStackLoRAWeights_ValidSamplesCoverageAndMedian(t *testing.T) {
	loraRow := func(t *testing.T, name, body string) CommunityLoRAUsage {
		t.Helper()
		s := mustCommunityStackSummary(t, body)
		row, ok := findLoRAUsage(s.Resources.LoRAs.Top, communityStackBasisName, name)
		if !ok {
			t.Fatalf("lora %q missing from top %v", name, stackLoRAUsageTexts(s.Resources.LoRAs.Top))
		}
		return row
	}

	t.Run("AC-5 mixed samples", func(t *testing.T) {
		body := `{"items":[
			{"id":1,"meta":{"resources":[{"type":"lora","name":"W","weight":0.5},{"type":"lora","name":"W","weight":"0.5"}]}},
			{"id":2,"meta":{"resources":[{"type":"lora","name":"W","weight":1.0}]}},
			{"id":3,"meta":{"resources":[{"type":"lora","name":"W"},{"type":"lora","name":"W","weight":"bad"}]}}
		]}`
		row := loraRow(t, "W", body)
		if row.ImageCount != 3 {
			t.Errorf("imageCount = %d, want 3", row.ImageCount)
		}
		if row.Weight.ImagesWithWeight != 2 || row.Weight.SampleCount != 2 {
			t.Errorf("weight coverage = %+v, want 2 images and 2 samples", row.Weight)
		}
		assertStackMedian(t, row.Weight.Median, 0.75)

		s := mustCommunityStackSummary(t, body)
		group := s.Resources.LoRAs
		if group.EntryCount != 5 || group.IdentifiedImageCount != 3 {
			t.Errorf("group = %+v, want 5 entries over 3 identified images", group)
		}
	})

	t.Run("no valid weights", func(t *testing.T) {
		row := loraRow(t, "Noweight",
			`{"items":[{"id":1,"meta":{"resources":[{"type":"lora","name":"Noweight","weight":null}]}}]}`)
		if row.ImageCount != 1 || row.Weight.Median != nil ||
			row.Weight.ImagesWithWeight != 0 || row.Weight.SampleCount != 0 {
			t.Errorf("row = %+v, want usage without weight evidence", row)
		}
	})

	t.Run("different weights in one image stay separate", func(t *testing.T) {
		row := loraRow(t, "Multi",
			`{"items":[{"id":1,"meta":{"resources":[{"type":"lora","name":"Multi","weight":0.4},{"type":"lora","name":"Multi","weight":0.8}]}}]}`)
		if row.ImageCount != 1 || row.Weight.ImagesWithWeight != 1 || row.Weight.SampleCount != 2 {
			t.Errorf("row = %+v, want 1 image with 2 retained samples", row)
		}
		assertStackMedian(t, row.Weight.Median, 0.6)
	})

	t.Run("odd count median", func(t *testing.T) {
		body := `{"items":[
			{"id":1,"meta":{"resources":[{"type":"lora","name":"Odd","weight":0.1}]}},
			{"id":2,"meta":{"resources":[{"type":"lora","name":"Odd","weight":0.3}]}},
			{"id":3,"meta":{"resources":[{"type":"lora","name":"Odd","weight":0.2}]}}
		]}`
		row := loraRow(t, "Odd", body)
		if row.Weight.SampleCount != 3 || row.Weight.ImagesWithWeight != 3 {
			t.Errorf("weight = %+v", row.Weight)
		}
		assertStackMedian(t, row.Weight.Median, 0.2)
	})

	t.Run("observed zero weight is a sample", func(t *testing.T) {
		row := loraRow(t, "Zero",
			`{"items":[{"id":1,"meta":{"resources":[{"type":"lora","name":"Zero","weight":0}]}}]}`)
		assertStackMedian(t, row.Weight.Median, 0)
		if row.Weight.SampleCount != 1 {
			t.Errorf("zero weight must be one sample: %+v", row.Weight)
		}
	})

	t.Run("negative and above-one weights retained", func(t *testing.T) {
		body := `{"items":[
			{"id":1,"meta":{"resources":[{"type":"lora","name":"Range","weight":-0.5}]}},
			{"id":2,"meta":{"resources":[{"type":"lora","name":"Range","weight":2.0}]}}
		]}`
		row := loraRow(t, "Range", body)
		if row.Weight.SampleCount != 2 {
			t.Errorf("finite negative/>1 weights must not be rejected: %+v", row.Weight)
		}
		assertStackMedian(t, row.Weight.Median, 0.75)
	})

	t.Run("non-finite weights rejected", func(t *testing.T) {
		body := `{"items":[{"id":1,"meta":{"resources":[
			{"type":"lora","name":"Bad","weight":"NaN"},
			{"type":"lora","name":"Bad","weight":"Inf"},
			{"type":"lora","name":"Bad","weight":{"bad":1}}
		]}}]}`
		row := loraRow(t, "Bad", body)
		if row.ImageCount != 1 || row.Weight.Median != nil || row.Weight.SampleCount != 0 {
			t.Errorf("non-finite/malformed weights must reduce coverage, not become samples: %+v", row)
		}
	})
}

// TestCommunityStackParameters_ObservedDenominatorsAndNormalization pins
// AC-6: numeric coalescing, zero and negative values observed, invalid
// values unobserved, exact case/whitespace-sensitive strings, and valid
// siblings surviving one bad field.
func TestCommunityStackParameters_ObservedDenominatorsAndNormalization(t *testing.T) {
	body := `{"items":[
		{"id":1,"meta":{"steps":20,"cfgScale":7.5,"sampler":"Euler","scheduler":"simple"}},
		{"id":2,"meta":{"steps":"20","cfgScale":"7.5","sampler":"euler","scheduler":"Simple"}},
		{"id":3,"meta":{"steps":20.0,"cfgScale":1,"sampler":"Euler ","scheduler":"simple"}},
		{"id":4,"meta":{"steps":0,"cfgScale":"1","sampler":"","scheduler":"  "}},
		{"id":5,"meta":{"steps":20.5,"cfgScale":"NaN","sampler":"ValidSibling","scheduler":null}},
		{"id":6,"meta":{"steps":"abc","cfgScale":-0.0,"sampler":5}},
		{"id":7,"meta":{"steps":-4}},
		{"id":8,"meta":null}
	]}`
	s := mustCommunityStackSummary(t, body)

	steps := s.Parameters.Steps
	if steps.ObservedImages != 5 || steps.SampledImages != 8 {
		t.Errorf("steps distribution = %+v, want 5 observed of 8 sampled", steps)
	}
	if got := stackValueTexts(steps.Top); !reflect.DeepEqual(got, []string{"20=3", "-4=1", "0=1"}) {
		t.Errorf("steps top = %v", got)
	}

	cfg := s.Parameters.CFGScale
	if cfg.ObservedImages != 5 || cfg.SampledImages != 8 {
		t.Errorf("cfgScale distribution = %+v, want 5 observed of 8 sampled", cfg)
	}
	if got := stackValueTexts(cfg.Top); !reflect.DeepEqual(got, []string{"1=2", "7.5=2", "0=1"}) {
		t.Errorf("cfgScale top = %v (numeric strings, 1.0, and signed zero must coalesce)", got)
	}

	sampler := s.Parameters.Sampler
	if sampler.ObservedImages != 4 {
		t.Errorf("sampler distribution = %+v, want 4 observed", sampler)
	}
	if got := stackValueTexts(sampler.Top); !reflect.DeepEqual(got, []string{"Euler=1", "Euler =1", "ValidSibling=1", "euler=1"}) {
		t.Errorf("sampler top = %v, want exact case/whitespace-sensitive values in bytewise order", got)
	}

	scheduler := s.Parameters.Scheduler
	if scheduler.ObservedImages != 3 {
		t.Errorf("scheduler distribution = %+v, want 3 observed", scheduler)
	}
	if got := stackValueTexts(scheduler.Top); !reflect.DeepEqual(got, []string{"simple=2", "Simple=1"}) {
		t.Errorf("scheduler top = %v", got)
	}
}

// TestCommunityStackParameters_DeterministicTopAndOther pins the numeric
// and lexical tie order, the fixed top five, and exact top-plus-other
// reconciliation for every parameter, including the zero-observed state.
func TestCommunityStackParameters_DeterministicTopAndOther(t *testing.T) {
	body := `{"items":[
		{"id":1,"meta":{"steps":30,"cfgScale":3.5,"sampler":"g","scheduler":"g"}},
		{"id":2,"meta":{"steps":10,"cfgScale":1.5,"sampler":"e","scheduler":"e"}},
		{"id":3,"meta":{"steps":70,"cfgScale":7.5,"sampler":"c","scheduler":"c"}},
		{"id":4,"meta":{"steps":20,"cfgScale":2.5,"sampler":"a","scheduler":"a"}},
		{"id":5,"meta":{"steps":60,"cfgScale":6.5,"sampler":"f","scheduler":"f"}},
		{"id":6,"meta":{"steps":40,"cfgScale":4.5,"sampler":"b","scheduler":"b"}},
		{"id":7,"meta":{"steps":50,"cfgScale":5.5,"sampler":"d","scheduler":"d"}}
	]}`
	s := mustCommunityStackSummary(t, body)

	cases := []struct {
		name string
		dist CommunityValueDistribution
		want []string
	}{
		{"steps", s.Parameters.Steps, []string{"10=1", "20=1", "30=1", "40=1", "50=1"}},
		{"cfgScale", s.Parameters.CFGScale, []string{"1.5=1", "2.5=1", "3.5=1", "4.5=1", "5.5=1"}},
		{"sampler", s.Parameters.Sampler, []string{"a=1", "b=1", "c=1", "d=1", "e=1"}},
		{"scheduler", s.Parameters.Scheduler, []string{"a=1", "b=1", "c=1", "d=1", "e=1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stackValueTexts(tc.dist.Top); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("top = %v, want %v", got, tc.want)
			}
			if tc.dist.Other.ImageCount != 2 || tc.dist.Other.DistinctValueCount != 2 {
				t.Errorf("other = %+v, want 2 images over 2 values", tc.dist.Other)
			}
			sum := tc.dist.Other.ImageCount
			for _, row := range tc.dist.Top {
				sum += row.ImageCount
			}
			if sum != tc.dist.ObservedImages || tc.dist.ObservedImages != 7 || tc.dist.SampledImages != 7 {
				t.Errorf("top plus other must reconcile to observed: %+v", tc.dist)
			}
		})
	}

	t.Run("zero observed", func(t *testing.T) {
		empty := mustCommunityStackSummary(t, `{"items":[{"id":1,"meta":null},{"id":2}]}`)
		for name, dist := range map[string]CommunityValueDistribution{
			"steps": empty.Parameters.Steps, "cfgScale": empty.Parameters.CFGScale,
			"sampler": empty.Parameters.Sampler, "scheduler": empty.Parameters.Scheduler,
		} {
			if dist.ObservedImages != 0 || dist.SampledImages != 2 ||
				dist.Top == nil || len(dist.Top) != 0 ||
				dist.Other.ImageCount != 0 || dist.Other.DistinctValueCount != 0 {
				t.Errorf("%s zero-observed distribution = %+v", name, dist)
			}
		}
	})
}

// TestCommunityStackTextAvailability_StatusCountsAndRates pins AC-7: the
// four statuses per field, whitespace as provided, unsupported metadata
// degrading both fields, non-object items as notProvided, and exact rates.
func TestCommunityStackTextAvailability_StatusCountsAndRates(t *testing.T) {
	body := `{"items":[
		{"id":1,"meta":{"prompt":"hello","negativePrompt":"world"}},
		{"id":2,"meta":{"prompt":"   ","negativePrompt":""}},
		{"id":3,"meta":{"prompt":"","negativePrompt":null}},
		{"id":4,"meta":{"prompt":123,"negativePrompt":{"x":1}}},
		{"id":5,"meta":{"prompt":true}},
		{"id":6,"meta":null},
		{"id":7},
		{"id":8,"meta":"sentinel-meta-shape"},
		42,
		null
	]}`
	s := mustCommunityStackSummary(t, body)

	prompt := s.TextAvailability.PromptStatus
	if prompt.Provided != 2 || prompt.Empty != 1 || prompt.NotProvided != 4 || prompt.UnsupportedJSONShape != 3 {
		t.Errorf("prompt status = %+v, want 2/1/4/3", prompt)
	}
	if prompt.Provided+prompt.Empty+prompt.NotProvided+prompt.UnsupportedJSONShape != s.Coverage.Returned {
		t.Errorf("prompt statuses must sum to returned: %+v", prompt)
	}
	if prompt.ProvidedRate == nil || *prompt.ProvidedRate != 0.2 {
		t.Errorf("prompt rate = %v, want 0.2", prompt.ProvidedRate)
	}

	negative := s.TextAvailability.NegativePromptStatus
	if negative.Provided != 1 || negative.Empty != 1 || negative.NotProvided != 6 || negative.UnsupportedJSONShape != 2 {
		t.Errorf("negative status = %+v, want 1/1/6/2", negative)
	}
	if negative.Provided+negative.Empty+negative.NotProvided+negative.UnsupportedJSONShape != s.Coverage.Returned {
		t.Errorf("negative statuses must sum to returned: %+v", negative)
	}
	if negative.ProvidedRate == nil || *negative.ProvidedRate != 0.1 {
		t.Errorf("negative rate = %v, want 0.1", negative.ProvidedRate)
	}

	out := marshalRecipe(t, s)
	assertNoJSONKeys(t, s, "prompt", "negativePrompt")
	assertNoSourceText(t, s, "hello", "world", "sentinel-meta-shape")
	_ = out
}

// TestCommunityStackTextAvailability_ZeroDenominator pins the null-rate
// contract for empty, absent, and null items.
func TestCommunityStackTextAvailability_ZeroDenominator(t *testing.T) {
	for name, body := range map[string]string{
		"empty array":  `{"items":[],"metadata":{}}`,
		"absent items": `{"metadata":{}}`,
		"null items":   `{"items":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := mustCommunityStackSummary(t, body)
			for label, status := range map[string]CommunityTextStatusCoverage{
				"prompt":   s.TextAvailability.PromptStatus,
				"negative": s.TextAvailability.NegativePromptStatus,
			} {
				if status.Provided != 0 || status.Empty != 0 || status.NotProvided != 0 ||
					status.UnsupportedJSONShape != 0 || status.ProvidedRate != nil {
					t.Errorf("%s status at zero denominator = %+v", label, status)
				}
			}
			out := marshalRecipe(t, s)
			if !strings.Contains(out, `"providedRate":null`) {
				t.Errorf("zero denominator must serialize a null rate: %s", out)
			}
		})
	}
}

// TestCommunityStackSummary_EmptyPartialAndContinuation pins honest empty
// and sparse states: zero counts, non-nil empty tops, no artificial median
// rows, sampled denominators, and presence-only continuation.
func TestCommunityStackSummary_EmptyPartialAndContinuation(t *testing.T) {
	t.Run("empty with continuation", func(t *testing.T) {
		body := `{"items":[],"metadata":{"nextCursor":"sentinel-cursor","nextPage":"https://example.test/next"}}`
		s := mustCommunityStackSummary(t, body)
		if !s.Coverage.ContinuationOffered || s.Coverage.Returned != 0 {
			t.Errorf("coverage = %+v", s.Coverage)
		}
		if s.Resources.EntryCount != 0 || s.Resources.ImagesWithEntries != 0 ||
			len(s.Resources.Checkpoints.Top) != 0 || len(s.Resources.LoRAs.Top) != 0 ||
			len(s.Resources.OtherResources.Top) != 0 {
			t.Errorf("empty resources = %+v", s.Resources)
		}
		for name, dist := range map[string]CommunityValueDistribution{
			"steps": s.Parameters.Steps, "cfgScale": s.Parameters.CFGScale,
			"sampler": s.Parameters.Sampler, "scheduler": s.Parameters.Scheduler,
		} {
			if dist.ObservedImages != 0 || dist.SampledImages != 0 || len(dist.Top) != 0 {
				t.Errorf("%s empty distribution = %+v", name, dist)
			}
		}
		out := marshalRecipe(t, s)
		assertNoJSONKeys(t, s, "nextCursor", "nextPage", "cursor")
		assertNoSourceText(t, s, "sentinel-cursor", "example.test/next")
		_ = out
	})

	t.Run("sparse items with one valid sibling", func(t *testing.T) {
		body := `{"items":[
			null,
			42,
			{"id":1,"meta":null},
			{"id":2,"meta":"sentinel-bad-shape"},
			{"id":3,"meta":{"steps":10,"civitaiResources":[{"type":"checkpoint","modelVersionId":5}]}}
		]}`
		s := mustCommunityStackSummary(t, body)
		if s.Coverage.Returned != 5 {
			t.Fatalf("coverage = %+v", s.Coverage)
		}
		if s.Parameters.Steps.ObservedImages != 1 || s.Parameters.Steps.SampledImages != 5 {
			t.Errorf("steps = %+v, want 1 observed of 5 sampled", s.Parameters.Steps)
		}
		if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, []string{"checkpoint|modelVersionId|5=1"}) {
			t.Errorf("checkpoints = %v", got)
		}
		prompt := s.TextAvailability.PromptStatus
		if prompt.NotProvided != 4 || prompt.UnsupportedJSONShape != 1 {
			t.Errorf("prompt status = %+v, want 4 notProvided and 1 unsupported", prompt)
		}
		assertNoSourceText(t, s, "sentinel-bad-shape")
	})
}

// TestCommunityStackSummary_ValidationAndMalformedEnvelope keeps contract
// violations and malformed envelopes as errors, never clamped success.
func TestCommunityStackSummary_ValidationAndMalformedEnvelope(t *testing.T) {
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
		{"negative limit", 3263843, -1, CommunitySortMostReactions},
		{"limit above max", 3263843, 101, CommunitySortMostReactions},
		{"unsupported sort", 3263843, 20, "Random"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseCommunityStackSummary(body, tc.version, tc.limit, tc.sort); err == nil {
				t.Fatalf("%s: expected an error", tc.name)
			}
		})
	}

	for name, malformed := range map[string]string{
		"not json":     `nope`,
		"array root":   `[]`,
		"null root":    `null`,
		"truncated":    `{"items":[`,
		"items object": `{"items":{}}`,
		"items string": `{"items":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCommunityStackSummary([]byte(malformed), 3263843, 20, CommunitySortMostReactions); err == nil {
				t.Fatalf("%s: expected an envelope error", name)
			}
		})
	}
}

// TestCommunityStackSummary_RedactionAndDeterminism pins AC-9: no
// prompt/negative text, raw metadata, workflow, media location, per-image
// identity, or continuation value leaks, no recommendation language
// appears, and repeated parses are byte-identical.
func TestCommunityStackSummary_RedactionAndDeterminism(t *testing.T) {
	body := `{"items":[
		{"id":12345,"postId":678,"username":"sentinel-user","createdAt":"2026-01-02T03:04:05Z",
			"url":"https://media.example.test/sentinel-media","hash":"sentinel-blurhash",
			"thumbnailUrl":"https://thumb.example.test/sentinel-thumb",
			"meta":{"prompt":"sentinel-prompt-text","negativePrompt":"sentinel-negative-text",
				"steps":20,"cfgScale":7,"sampler":"Euler","scheduler":"simple",
				"unknownField":"sentinel-unknown-meta","workflow":"sentinel-workflow","comfy":"sentinel-comfy",
				"civitaiResources":[{"type":"checkpoint","modelVersionId":777,"modelName":"sentinel-model-name"}],
				"resources":[{"type":"lora","name":"Sentinel LoRA","weight":0.5}]}},
		42,
		{"id":999,"meta":null}
	],"metadata":{"nextCursor":"sentinel-cursor","nextPage":"https://example.test/sentinel-next"}}`
	s := mustCommunityStackSummary(t, body)

	assertNoJSONKeys(t, s,
		"observations", "examples", "image", "textParameters",
		"prompt", "negativePrompt", "meta", "comfy", "workflow", "description",
		"url", "pageUrl", "thumbnailUrl", "blurHash",
		"nextCursor", "nextPage", "cursor",
		"username", "postId", "createdAt", "imageId", "upstreamPosition",
		"issues", "modelVersionIds", "stats", "likeCount", "supportingImages")
	assertNoSourceText(t, s,
		"sentinel-prompt-text", "sentinel-negative-text", "sentinel-unknown-meta",
		"sentinel-workflow", "sentinel-comfy", "sentinel-media", "sentinel-blurhash",
		"sentinel-thumb", "sentinel-cursor", "sentinel-next", "sentinel-user",
		"sentinel-model-name")

	out := marshalRecipe(t, s)
	lower := strings.ToLower(out)
	for _, phrase := range []string{"recommend", "quality score", "stability", "reproducib", "best", "top pick"} {
		if strings.Contains(lower, phrase) {
			t.Errorf("stack summary must not carry %q language: %s", phrase, out)
		}
	}

	// The selected identity labels are emitted; the sample context proves
	// aggregation actually happened over the sentinel body.
	if got := stackUsageTexts(s.Resources.Checkpoints.Top); !reflect.DeepEqual(got, []string{"checkpoint|modelVersionId|777=1"}) {
		t.Errorf("checkpoints = %v", got)
	}
	if got := stackLoRAUsageTexts(s.Resources.LoRAs.Top); !reflect.DeepEqual(got, []string{"lora|name|Sentinel LoRA=1"}) {
		t.Errorf("loras = %v", got)
	}

	again := marshalRecipe(t, mustCommunityStackSummary(t, body))
	if out != again {
		t.Errorf("repeated parse must marshal byte-identically:\n%s\n%s", out, again)
	}
}

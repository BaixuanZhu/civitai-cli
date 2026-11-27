package api

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"civitai-cli/internal/testutil"
)

// compactNumberText renders a nullable json.Number for assertions.
func compactNumberText(p *json.Number) string {
	if p == nil {
		return "<nil>"
	}
	return p.String()
}

// compactStringText renders a nullable string for assertions.
func compactStringText(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// compactBoolValue unwraps a nullable bool and reports presence.
func compactBoolValue(p *bool) (value, present bool) {
	if p == nil {
		return false, false
	}
	return *p, true
}

// compactMarshal marshals v and fails the test on error.
func compactMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal compact projection: %v", err)
	}
	return b
}

// compactMarshalIndent marshals v with two-space indentation for semantic
// size comparisons against the legacy full view.
func compactMarshalIndent(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal indented: %v", err)
	}
	return b
}

func TestCompactSearch_Fixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "models_search_anima.json")
	env, err := ParseCompactModels(raw)
	if err != nil {
		t.Fatalf("ParseCompactModels: %v", err)
	}
	if env.SchemaVersion != CompactSearchSchemaVersion || env.Kind != compactKindModelSearch {
		t.Fatalf("envelope = %q/%q", env.SchemaVersion, env.Kind)
	}
	if len(env.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(env.Items))
	}

	first := env.Items[0]
	if got := compactNumberText(first.ID); got != "2458426" {
		t.Errorf("id = %q", got)
	}
	if got := compactStringText(first.Name); got != "Anima" {
		t.Errorf("name = %q", got)
	}
	if got := compactStringText(first.URL); got != "https://civitai.com/models/2458426" {
		t.Errorf("url = %q", got)
	}
	if got := compactStringText(first.Type); got != "Checkpoint" {
		t.Errorf("type = %q", got)
	}
	if got := compactStringText(first.Creator); got != "circlestone_labs" {
		t.Errorf("creator = %q", got)
	}
	if !reflect.DeepEqual(first.Tags, []string{"anime", "art", "base model", "illustration"}) {
		t.Errorf("tags = %v", first.Tags)
	}
	if !reflect.DeepEqual(first.BaseModels, []string{"Anima"}) {
		t.Errorf("baseModels = %v", first.BaseModels)
	}
	if v, ok := compactBoolValue(first.NSFW); !ok || v {
		t.Errorf("nsfw = %v (present %v), want present false", v, ok)
	}
	if got := compactStringText(first.Availability); got != "Public" {
		t.Errorf("availability = %q", got)
	}
	if first.License == nil {
		t.Fatalf("license must be present when grants are supplied")
	}
	if v, ok := compactBoolValue(first.License.AllowNoCredit); !ok || !v {
		t.Errorf("allowNoCredit = %v (present %v)", v, ok)
	}
	if !reflect.DeepEqual(first.License.AllowCommercialUse, []string{"Image", "RentCivit"}) {
		t.Errorf("allowCommercialUse = %v", first.License.AllowCommercialUse)
	}
	if v, ok := compactBoolValue(first.License.AllowDerivatives); !ok || !v {
		t.Errorf("allowDerivatives = %v (present %v)", v, ok)
	}
	if v, ok := compactBoolValue(first.License.AllowDifferentLicense); !ok || v {
		t.Errorf("allowDifferentLicense = %v (present %v)", v, ok)
	}
	if first.Stats == nil {
		t.Fatalf("stats must be present for the fixture")
	}
	if got := compactNumberText(first.Stats.DownloadCount); got != "309445" {
		t.Errorf("downloadCount = %q", got)
	}
	if got := compactNumberText(first.Stats.ThumbsUpCount); got != "11548" {
		t.Errorf("thumbsUpCount = %q", got)
	}
	if got := compactNumberText(first.Stats.ThumbsDownCount); got != "38" {
		t.Errorf("thumbsDownCount = %q", got)
	}
	if got := compactNumberText(first.Stats.CommentCount); got != "852" {
		t.Errorf("commentCount = %q", got)
	}

	if len(first.Versions) != 8 {
		t.Fatalf("versions = %d, want all 8 embedded versions", len(first.Versions))
	}
	wantIDs := []string{"3263843", "3126581", "3108589", "3111778", "3108569", "2945208", "2836417", "2764263"}
	for i, want := range wantIDs {
		if got := compactNumberText(first.Versions[i].ID); got != want {
			t.Errorf("version[%d] id = %q, want %q (source order)", i, got, want)
		}
	}
	v := first.Versions[0]
	if compactStringText(v.Name) != "turbo-v1.1" ||
		compactStringText(v.BaseModel) != "Anima" ||
		compactStringText(v.BaseModelType) != "Standard" ||
		compactStringText(v.PublishedAt) != "2026-08-24T23:42:31.929Z" ||
		compactStringText(v.Availability) != "Public" {
		t.Errorf("first version ref = %+v", v)
	}
	if v.Stats == nil ||
		compactNumberText(v.Stats.DownloadCount) != "66832" ||
		compactNumberText(v.Stats.ThumbsUpCount) != "1934" ||
		compactNumberText(v.Stats.ThumbsDownCount) != "4" {
		t.Errorf("first version stats = %+v", v.Stats)
	}

	if got := compactNumberText(env.Items[1].ID); got != "260267" {
		t.Errorf("second item id = %q", got)
	}
	if len(env.Items[1].Versions) != 3 {
		t.Errorf("second item versions = %d, want 3", len(env.Items[1].Versions))
	}

	// The summary aggregates only this returned page: exact per-model
	// dedup, model-level precedence, coverage counted in models. Counts
	// tie at 1, so categories sort by exact name ascending.
	wantSummary := SearchResultSummary{
		Returned: 2,
		BaseCoverage: []SearchBaseCoverage{
			{BaseModel: "Anima", Models: 1},
			{BaseModel: "SDXL 1.0", Models: 1},
		},
		ModelsWithMultipleBases: 0,
		ModelsWithoutBase:       0,
	}
	if !reflect.DeepEqual(env.Summary, wantSummary) {
		t.Errorf("summary = %+v, want %+v", env.Summary, wantSummary)
	}

	// Excluded heavy fields must never leak into a list/summary projection.
	out := string(compactMarshal(t, env))
	for _, sentinel := range []string{
		`"description"`, `"modelVersions"`, `"files"`, `"images"`, `"downloadUrl"`,
		`"trainedWords"`, `"comfy"`, `"prompt"`, "<p>", `"model"`,
	} {
		if strings.Contains(out, sentinel) {
			t.Errorf("compact search output leaks %s", sentinel)
		}
	}
}

func TestCompactSearch_SummaryEvidencePolicy(t *testing.T) {
	cases := []struct {
		name string
		body string
		want SearchResultSummary
	}{
		{
			name: "usable model-level bases suppress version-only bases",
			body: `{"items":[{"baseModels":["SDXL 1.0"],"modelVersions":[{"baseModel":"Pony"},{"baseModel":"SDXL 1.0"}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:     1,
				BaseCoverage: []SearchBaseCoverage{{BaseModel: "SDXL 1.0", Models: 1}},
			},
		},
		{
			name: "absent model-level bases fall back to all embedded versions",
			body: `{"items":[{"modelVersions":[{"baseModel":"Pony"},{"baseModel":"SDXL 1.0"},{"baseModel":"Pony"}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned: 1,
				BaseCoverage: []SearchBaseCoverage{
					{BaseModel: "Pony", Models: 1},
					{BaseModel: "SDXL 1.0", Models: 1},
				},
				ModelsWithMultipleBases: 1,
			},
		},
		{
			name: "null model-level bases fall back to versions",
			body: `{"items":[{"baseModels":null,"modelVersions":[{"baseModel":"Pony"}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:     1,
				BaseCoverage: []SearchBaseCoverage{{BaseModel: "Pony", Models: 1}},
			},
		},
		{
			name: "empty model-level bases fall back to versions",
			body: `{"items":[{"baseModels":[],"modelVersions":[{"baseModel":"Pony"}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:     1,
				BaseCoverage: []SearchBaseCoverage{{BaseModel: "Pony", Models: 1}},
			},
		},
		{
			name: "all-blank model-level bases fall back to versions",
			body: `{"items":[{"baseModels":["","   "],"modelVersions":[{"baseModel":"Pony"}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:     1,
				BaseCoverage: []SearchBaseCoverage{{BaseModel: "Pony", Models: 1}},
			},
		},
		{
			name: "duplicates within model-level data count once per model",
			body: `{"items":[{"baseModels":["SDXL 1.0","Pony","Pony"]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned: 1,
				BaseCoverage: []SearchBaseCoverage{
					{BaseModel: "Pony", Models: 1},
					{BaseModel: "SDXL 1.0", Models: 1},
				},
				ModelsWithMultipleBases: 1,
			},
		},
		{
			name: "duplicates across fallback versions count once per model",
			body: `{"items":[{"modelVersions":[{"baseModel":"SDXL 1.0"},{"baseModel":"SDXL 1.0"}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:     1,
				BaseCoverage: []SearchBaseCoverage{{BaseModel: "SDXL 1.0", Models: 1}},
			},
		},
		{
			name: "absent null empty and blank version bases contribute nothing",
			body: `{"items":[{"baseModels":[],"modelVersions":[{"baseModel":""},{"baseModel":"  "},{"baseModel":null},{}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:          1,
				BaseCoverage:      []SearchBaseCoverage{},
				ModelsWithoutBase: 1,
			},
		},
		{
			name: "no model-level and no versions counts the model without base",
			body: `{"items":[{}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:          1,
				BaseCoverage:      []SearchBaseCoverage{},
				ModelsWithoutBase: 1,
			},
		},
		{
			name: "unknown mixed-case and whitespace-padded names survive verbatim",
			body: `{"items":[{"baseModels":["  Pony ","pony","PONY"]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned: 1,
				BaseCoverage: []SearchBaseCoverage{
					{BaseModel: "  Pony ", Models: 1},
					{BaseModel: "PONY", Models: 1},
					{BaseModel: "pony", Models: 1},
				},
				ModelsWithMultipleBases: 1,
			},
		},
		{
			name: "equal counts sort by exact name ascending",
			body: `{"items":[{"baseModels":["Zeta"]},{"baseModels":["Alpha"]},{"baseModels":["Mid"]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned: 3,
				BaseCoverage: []SearchBaseCoverage{
					{BaseModel: "Alpha", Models: 1},
					{BaseModel: "Mid", Models: 1},
					{BaseModel: "Zeta", Models: 1},
				},
			},
		},
		{
			name: "higher counts sort before name order",
			body: `{"items":[{"baseModels":["Zeta"]},{"baseModels":["Alpha"]},{"baseModels":["Zeta"]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned: 3,
				BaseCoverage: []SearchBaseCoverage{
					{BaseModel: "Zeta", Models: 2},
					{BaseModel: "Alpha", Models: 1},
				},
			},
		},
		{
			name: "multi-base model increments every distinct base once",
			body: `{"items":[{"baseModels":["SDXL 1.0","Pony"]},{"baseModels":["Pony"]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned: 2,
				BaseCoverage: []SearchBaseCoverage{
					{BaseModel: "Pony", Models: 2},
					{BaseModel: "SDXL 1.0", Models: 1},
				},
				ModelsWithMultipleBases: 1,
			},
		},
		{
			name: "worked reconciliation from the design",
			body: `{"items":[{"baseModels":["SDXL 1.0","Pony","Pony"]},{"modelVersions":[{"baseModel":"SDXL 1.0"},{"baseModel":"SDXL 1.0"}]},{"baseModels":[],"modelVersions":[{}]}],"metadata":{}}`,
			want: SearchResultSummary{
				Returned: 3,
				BaseCoverage: []SearchBaseCoverage{
					{BaseModel: "SDXL 1.0", Models: 2},
					{BaseModel: "Pony", Models: 1},
				},
				ModelsWithMultipleBases: 1,
				ModelsWithoutBase:       1,
			},
		},
		{
			name: "empty items produce zero counters and empty coverage",
			body: `{"items":[],"metadata":{}}`,
			want: SearchResultSummary{
				Returned:     0,
				BaseCoverage: []SearchBaseCoverage{},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compactEnv, err := ParseCompactModels([]byte(tc.body))
			if err != nil {
				t.Fatalf("ParseCompactModels: %v", err)
			}
			if !reflect.DeepEqual(compactEnv.Summary, tc.want) {
				t.Errorf("compact summary = %+v, want %+v", compactEnv.Summary, tc.want)
			}
			models, err := ParseModels([]byte(tc.body))
			if err != nil {
				t.Fatalf("ParseModels: %v", err)
			}
			legacy := SummarizeSearchModels(models)
			if !reflect.DeepEqual(legacy, tc.want) {
				t.Errorf("legacy summary = %+v, want %+v", legacy, tc.want)
			}
			// Human and compact consumers share one policy core, so the two
			// projections of the same response must agree exactly.
			if !reflect.DeepEqual(compactEnv.Summary, legacy) {
				t.Errorf("compact and legacy summaries disagree:\ncompact: %+v\nlegacy:  %+v", compactEnv.Summary, legacy)
			}
		})
	}

	t.Run("item order does not affect the summary", func(t *testing.T) {
		forward, err := ParseCompactModels([]byte(`{"items":[{"baseModels":["Zeta","Alpha"]},{"baseModels":["Alpha"]},{"baseModels":["Mid"]}],"metadata":{}}`))
		if err != nil {
			t.Fatalf("forward parse: %v", err)
		}
		permuted, err := ParseCompactModels([]byte(`{"items":[{"baseModels":["Mid"]},{"baseModels":["Alpha"]},{"baseModels":["Alpha","Zeta"]}],"metadata":{}}`))
		if err != nil {
			t.Fatalf("permuted parse: %v", err)
		}
		if !reflect.DeepEqual(forward.Summary, permuted.Summary) {
			t.Errorf("summary depends on input order:\nforward:  %+v\npermuted: %+v", forward.Summary, permuted.Summary)
		}
	})
}

func TestCompactSearch_RequiredKeys(t *testing.T) {
	env, err := ParseCompactModels([]byte(`{"items":[{"id":7}],"metadata":{}}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(compactMarshal(t, env), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"schemaVersion", "kind", "items", "summary"} {
		if _, ok := envelope[key]; !ok {
			t.Errorf("required key %q missing from search envelope", key)
		}
	}
	var summary map[string]json.RawMessage
	if err := json.Unmarshal(envelope["summary"], &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	for _, key := range []string{"returned", "baseCoverage", "modelsWithMultipleBases", "modelsWithoutBase"} {
		if _, ok := summary[key]; !ok {
			t.Errorf("required key %q missing from search summary", key)
		}
	}
	if string(summary["returned"]) != "1" {
		t.Errorf("returned = %s, want 1", summary["returned"])
	}
	// No observed base evidence: coverage must serialize as [], never null,
	// and no zero-count named category may be fabricated.
	if string(summary["baseCoverage"]) != "[]" {
		t.Errorf("baseCoverage = %s, want []", summary["baseCoverage"])
	}
	if string(summary["modelsWithMultipleBases"]) != "0" || string(summary["modelsWithoutBase"]) != "1" {
		t.Errorf("counters = %s/%s, want 0/1", summary["modelsWithMultipleBases"], summary["modelsWithoutBase"])
	}
}

func TestCompactModel_FixtureAndSearchParity(t *testing.T) {
	searchEnv, err := ParseCompactModels(testutil.LoadFixture(t, "models_search_anima.json"))
	if err != nil {
		t.Fatalf("ParseCompactModels: %v", err)
	}
	modelEnv, err := ParseCompactModel(testutil.LoadFixture(t, "models_2458426.json"))
	if err != nil {
		t.Fatalf("ParseCompactModel: %v", err)
	}
	if modelEnv.SchemaVersion != CompactShowSchemaVersion || modelEnv.Kind != compactKindModelSummary {
		t.Fatalf("envelope = %q/%q", modelEnv.SchemaVersion, modelEnv.Kind)
	}
	// The model-show summary is the identical projection used by search
	// items: one shared schema, no per-command drift.
	if !reflect.DeepEqual(searchEnv.Items[0], modelEnv.Model) {
		t.Errorf("model show summary differs from search item 0:\nshow: %+v\nsearch: %+v", modelEnv.Model, searchEnv.Items[0])
	}
	if len(modelEnv.Model.Versions) != 8 {
		t.Errorf("versions = %d, want all 8 (no latest-only selection)", len(modelEnv.Model.Versions))
	}

	out := string(compactMarshal(t, modelEnv))
	for _, sentinel := range []string{`"description"`, `"images"`, `"files"`, `"downloadUrl"`, `"comfy"`, "<p>"} {
		if strings.Contains(out, sentinel) {
			t.Errorf("compact model output leaks %s", sentinel)
		}
	}
}

func TestCompactVersion_Fixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "version_3263843.json")
	env, err := ParseCompactVersion(raw)
	if err != nil {
		t.Fatalf("ParseCompactVersion: %v", err)
	}
	if env.SchemaVersion != CompactShowSchemaVersion || env.Kind != compactKindVersionSummary {
		t.Fatalf("envelope = %q/%q", env.SchemaVersion, env.Kind)
	}
	v := env.Version
	if compactNumberText(v.ID) != "3263843" || compactNumberText(v.ModelID) != "2458426" {
		t.Errorf("ids = %s/%s", compactNumberText(v.ID), compactNumberText(v.ModelID))
	}
	if compactStringText(v.Name) != "turbo-v1.1" ||
		compactStringText(v.BaseModel) != "Anima" ||
		compactStringText(v.BaseModelType) != "Standard" ||
		compactStringText(v.PublishedAt) != "2026-08-24T23:42:31.929Z" {
		t.Errorf("identity = %+v", v)
	}
	if v.Availability != nil {
		t.Errorf("availability must be null when the source omits it, got %q", *v.Availability)
	}
	if compactNumberText(v.Model.ID) != "2458426" ||
		compactStringText(v.Model.Name) != "Anima" ||
		compactStringText(v.Model.Type) != "Checkpoint" {
		t.Errorf("owning model = %+v", v.Model)
	}
	if v.Stats == nil ||
		compactNumberText(v.Stats.DownloadCount) != "66832" ||
		compactNumberText(v.Stats.ThumbsUpCount) != "1934" ||
		v.Stats.ThumbsDownCount != nil {
		t.Errorf("stats = %+v, want 66832/1934/null", v.Stats)
	}
	if v.TrainedWords == nil || len(v.TrainedWords) != 0 {
		t.Errorf("trainedWords = %v, want supplied empty []", v.TrainedWords)
	}
	if len(v.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(v.Files))
	}
	f := v.Files[0]
	if compactNumberText(f.ID) != "3147349" ||
		compactStringText(f.Name) != "anima-turbo-v1.1.safetensors" ||
		compactNumberText(f.SizeKB) != "4084209.625" ||
		compactStringText(f.Type) != "Model" {
		t.Errorf("file identity = %+v", f)
	}
	if p, ok := compactBoolValue(f.Primary); !ok || !p {
		t.Errorf("primary = %v (present %v)", p, ok)
	}
	if compactStringText(f.Metadata.Format) != "SafeTensor" ||
		compactStringText(f.Metadata.FP) != "bf16" ||
		f.Metadata.Size != nil {
		t.Errorf("metadata = %+v", f.Metadata)
	}
	if compactStringText(f.PickleScanResult) != "Success" ||
		compactStringText(f.VirusScanResult) != "Success" ||
		compactStringText(f.ScannedAt) != "2026-08-24T23:40:47.547" {
		t.Errorf("scan facts = %+v", f)
	}
	wantHashes := map[string]string{
		"SHA256":    "FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB",
		"AutoV1":    "A231A5D0",
		"AutoV2":    "FBA1195327",
		"AutoV3":    "2377CE19704F",
		"BLAKE3":    "6CAAE380057827BF247C3B0C4F5B65A29783D021CE9B60ECE455E6E2197E4533",
		"CRC32":     "9BBADBC9",
		"SHA256_12": "FBA11953276B",
	}
	if !reflect.DeepEqual(f.Hashes, wantHashes) {
		t.Errorf("hashes = %v, want every supplied algorithm", f.Hashes)
	}
	if v.Examples == nil || *v.Examples != (CompactExamples{Total: 9, WithMetadata: 9, WithoutMetadata: 0}) {
		t.Errorf("examples = %+v, want 9/9/0", v.Examples)
	}

	out := string(compactMarshal(t, env))
	for _, sentinel := range []string{
		`"description"`, `"images"`, `"downloadUrl"`, `"comfy"`, `"workflow"`,
		`"prompt"`, `"negativePrompt"`, `"seed"`, `"hasMeta"`, `"meta"`,
		`"trainingDetails"`, `"trainingStatus"`, `"paidAccess"`, `"usageControl"`,
		`"licensingFee"`, "<p>",
	} {
		if strings.Contains(out, sentinel) {
			t.Errorf("compact version output leaks %s", sentinel)
		}
	}
}

func TestCompactModel_PresenceSemantics(t *testing.T) {
	t.Run("absent optional members are null", func(t *testing.T) {
		env, err := ParseCompactModel([]byte(`{"id":7}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		m := env.Model
		if m.Tags != nil || m.BaseModels != nil || m.Versions != nil || m.Stats != nil || m.License != nil {
			t.Errorf("absent arrays/objects must stay nil: %+v", m)
		}
		if m.NSFW != nil || m.Creator != nil || m.Availability != nil || m.Name != nil || m.Type != nil {
			t.Errorf("absent scalars must stay nil: %+v", m)
		}
		if compactStringText(m.URL) != "https://civitai.com/models/7" {
			t.Errorf("url = %q", compactStringText(m.URL))
		}
	})

	t.Run("explicit null matches absent", func(t *testing.T) {
		env, err := ParseCompactModel([]byte(`{"id":null,"tags":null,"baseModels":null,"modelVersions":null,"stats":null,"creator":null,"license":null}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		m := env.Model
		if m.ID != nil || m.URL != nil || m.Tags != nil || m.BaseModels != nil || m.Versions != nil || m.Stats != nil || m.Creator != nil {
			t.Errorf("explicit null must project to null: %+v", m)
		}
	})

	t.Run("explicit empty arrays and objects are preserved", func(t *testing.T) {
		env, err := ParseCompactModel([]byte(`{"id":7,"tags":[],"baseModels":[],"modelVersions":[],"stats":{},"creator":{}}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		m := env.Model
		if m.Tags == nil || len(m.Tags) != 0 {
			t.Errorf("tags = %v, want supplied []", m.Tags)
		}
		if m.BaseModels == nil || len(m.BaseModels) != 0 {
			t.Errorf("baseModels = %v, want supplied []", m.BaseModels)
		}
		if m.Versions == nil || len(m.Versions) != 0 {
			t.Errorf("versions = %v, want supplied []", m.Versions)
		}
		if m.Stats == nil || m.Stats.DownloadCount != nil || m.Stats.ThumbsUpCount != nil ||
			m.Stats.ThumbsDownCount != nil || m.Stats.CommentCount != nil {
			t.Errorf("empty stats must be an object with null members: %+v", m.Stats)
		}
		if m.Creator != nil {
			t.Errorf("creator {} has no username, so creator must be null, got %q", *m.Creator)
		}
	})

	t.Run("explicit zero false and empty string are values", func(t *testing.T) {
		env, err := ParseCompactModel([]byte(`{"id":0,"nsfw":false,"availability":"","stats":{"downloadCount":0},"allowNoCredit":false}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		m := env.Model
		if got := compactNumberText(m.ID); got != "0" {
			t.Errorf("explicit id 0 = %q, want \"0\"", got)
		}
		if got := compactStringText(m.URL); got != "https://civitai.com/models/0" {
			t.Errorf("url for explicit id 0 = %q", got)
		}
		if v, ok := compactBoolValue(m.NSFW); !ok || v {
			t.Errorf("nsfw = %v (present %v), want present false", v, ok)
		}
		if got := compactStringText(m.Availability); got != "" {
			t.Errorf("availability = %q, want explicit empty string", got)
		}
		if m.Stats == nil || compactNumberText(m.Stats.DownloadCount) != "0" || m.Stats.ThumbsUpCount != nil {
			t.Errorf("stats = %+v, want downloadCount 0 and the rest null", m.Stats)
		}
		if m.License == nil {
			t.Fatalf("license must be an object when one grant is supplied")
		}
		if v, ok := compactBoolValue(m.License.AllowNoCredit); !ok || v {
			t.Errorf("allowNoCredit = %v (present %v), want present false", v, ok)
		}
		if m.License.AllowCommercialUse != nil || m.License.AllowDerivatives != nil || m.License.AllowDifferentLicense != nil {
			t.Errorf("absent grants must stay null: %+v", m.License)
		}
	})

	t.Run("unknown enums and fields are tolerated", func(t *testing.T) {
		env, err := ParseCompactModel([]byte(`{"id":1,"type":"BrandNewType","availability":"Future","mystery":{"deep":true},"modelVersions":[{"id":2,"baseModel":"FutureBase","extra":"x"}]}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if compactStringText(env.Model.Type) != "BrandNewType" || compactStringText(env.Model.Availability) != "Future" {
			t.Errorf("unknown enums must be preserved: %+v", env.Model)
		}
		if len(env.Model.Versions) != 1 || compactStringText(env.Model.Versions[0].BaseModel) != "FutureBase" {
			t.Errorf("version ref = %+v", env.Model.Versions)
		}
		if strings.Contains(string(compactMarshal(t, env)), "mystery") {
			t.Errorf("unknown fields must not be emitted")
		}
	})

	t.Run("numeric string drift becomes a number", func(t *testing.T) {
		env, err := ParseCompactModel([]byte(`{"id":"123","stats":{"downloadCount":"456"}}`))
		if err != nil {
			t.Fatalf("numeric strings must be tolerated: %v", err)
		}
		if got := compactNumberText(env.Model.ID); got != "123" {
			t.Errorf("id = %q", got)
		}
		if got := compactStringText(env.Model.URL); got != "https://civitai.com/models/123" {
			t.Errorf("url = %q", got)
		}
		out := string(compactMarshal(t, env))
		if !strings.Contains(out, `"id":123`) || !strings.Contains(out, `"downloadCount":456`) {
			t.Errorf("numeric strings must marshal as numbers: %s", out)
		}
	})

	t.Run("invalid numeric text is a decode error", func(t *testing.T) {
		for _, body := range []string{
			`{"id":"abc"}`,
			`{"id":1,"stats":{"downloadCount":"many"}}`,
			`{"id":1,"modelVersions":[{"id":"v2"}]}`,
		} {
			if _, err := ParseCompactModel([]byte(body)); err == nil {
				t.Errorf("expected decode error for %s", body)
			}
		}
	})

	t.Run("malformed root containers are rejected", func(t *testing.T) {
		for _, body := range []string{``, `null`, `[]`, `"model"`, `123`, `{`} {
			if _, err := ParseCompactModel([]byte(body)); err == nil {
				t.Errorf("expected root error for %q", body)
			}
		}
	})
}

func TestCompactModel_RequiredKeys(t *testing.T) {
	env, err := ParseCompactModel([]byte(`{"id":7}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(compactMarshal(t, env), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var model map[string]json.RawMessage
	if err := json.Unmarshal(envelope["model"], &model); err != nil {
		t.Fatalf("unmarshal model: %v", err)
	}
	for _, key := range []string{
		"id", "name", "url", "type", "creator", "tags", "baseModels",
		"nsfw", "availability", "license", "stats", "versions",
	} {
		if _, ok := model[key]; !ok {
			t.Errorf("required key %q missing from model summary", key)
		}
	}
	for _, key := range []string{"name", "type", "creator", "tags", "baseModels", "nsfw", "availability", "license", "stats", "versions"} {
		if string(model[key]) != "null" {
			t.Errorf("key %q = %s, want null for a not-provided source", key, model[key])
		}
	}
}

func TestCompactVersion_PresenceSemantics(t *testing.T) {
	t.Run("absent optional members are null", func(t *testing.T) {
		env, err := ParseCompactVersion([]byte(`{"id":1}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		v := env.Version
		if v.ModelID != nil || v.Availability != nil || v.Stats != nil || v.TrainedWords != nil || v.Files != nil || v.Examples != nil {
			t.Errorf("absent members must stay nil: %+v", v)
		}
		if v.Model.ID != nil || v.Model.Name != nil || v.Model.Type != nil {
			t.Errorf("absent owning model members must stay nil: %+v", v.Model)
		}
	})

	t.Run("explicit empty arrays yield [] and zero examples", func(t *testing.T) {
		env, err := ParseCompactVersion([]byte(`{"id":1,"trainedWords":[],"files":[],"images":[],"stats":{}}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		v := env.Version
		if v.TrainedWords == nil || len(v.TrainedWords) != 0 {
			t.Errorf("trainedWords = %v, want supplied []", v.TrainedWords)
		}
		if v.Files == nil || len(v.Files) != 0 {
			t.Errorf("files = %v, want supplied []", v.Files)
		}
		if v.Examples == nil || *v.Examples != (CompactExamples{}) {
			t.Errorf("examples = %+v, want zero counts", v.Examples)
		}
		if v.Stats == nil || v.Stats.DownloadCount != nil || v.Stats.ThumbsUpCount != nil || v.Stats.ThumbsDownCount != nil {
			t.Errorf("empty stats must be an object with null members: %+v", v.Stats)
		}
	})

	t.Run("null images mean examples are not provided", func(t *testing.T) {
		env, err := ParseCompactVersion([]byte(`{"id":1,"images":null}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if env.Version.Examples != nil {
			t.Errorf("examples = %+v, want null", env.Version.Examples)
		}
	})

	t.Run("examples count non-null meta objects only", func(t *testing.T) {
		body := `{"id":1,"images":[{"meta":{"prompt":"secret text"}},{"meta":null},{"hasMeta":true},{"meta":{}},{"meta":[1]}]}`
		env, err := ParseCompactVersion([]byte(body))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		want := CompactExamples{Total: 5, WithMetadata: 2, WithoutMetadata: 3}
		if env.Version.Examples == nil || *env.Version.Examples != want {
			t.Errorf("examples = %+v, want %+v", env.Version.Examples, want)
		}
		out := string(compactMarshal(t, env))
		if strings.Contains(out, "secret text") || strings.Contains(out, `"prompt"`) {
			t.Errorf("example metadata must never leak into compact output: %s", out)
		}
	})

	t.Run("explicit zero empty and false file values are preserved", func(t *testing.T) {
		body := `{"id":1,"files":[{"id":0,"name":"","sizeKB":0,"primary":false,"metadata":{},"hashes":{}}]}`
		env, err := ParseCompactVersion([]byte(body))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(env.Version.Files) != 1 {
			t.Fatalf("files = %d", len(env.Version.Files))
		}
		f := env.Version.Files[0]
		if compactNumberText(f.ID) != "0" || compactStringText(f.Name) != "" || compactNumberText(f.SizeKB) != "0" {
			t.Errorf("explicit values lost: %+v", f)
		}
		if p, ok := compactBoolValue(f.Primary); !ok || p {
			t.Errorf("primary = %v (present %v), want present false", p, ok)
		}
		if f.Metadata.Format != nil || f.Metadata.FP != nil || f.Metadata.Size != nil {
			t.Errorf("empty metadata must be an object with null members: %+v", f.Metadata)
		}
		if f.Hashes == nil || len(f.Hashes) != 0 {
			t.Errorf("hashes = %v, want supplied {}", f.Hashes)
		}
	})

	t.Run("hashes keep unknown algorithms and absence", func(t *testing.T) {
		env, err := ParseCompactVersion([]byte(`{"id":1,"files":[{"hashes":{"SHA256":"abc","FutureHash":"zzz"}},{"id":2}]}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !reflect.DeepEqual(env.Version.Files[0].Hashes, map[string]string{"SHA256": "abc", "FutureHash": "zzz"}) {
			t.Errorf("hashes = %v", env.Version.Files[0].Hashes)
		}
		if env.Version.Files[1].Hashes != nil {
			t.Errorf("absent hashes must stay null, got %v", env.Version.Files[1].Hashes)
		}
	})

	t.Run("unknown enums and fields are tolerated", func(t *testing.T) {
		env, err := ParseCompactVersion([]byte(`{"id":1,"baseModel":"FutureBase","baseModelType":"Drift","mystery":true,"files":[{"id":2,"unknown":"x"}]}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if compactStringText(env.Version.BaseModel) != "FutureBase" || compactStringText(env.Version.BaseModelType) != "Drift" {
			t.Errorf("unknown enums lost: %+v", env.Version)
		}
		if strings.Contains(string(compactMarshal(t, env)), "mystery") {
			t.Errorf("unknown fields must not be emitted")
		}
	})

	t.Run("invalid numeric text is a decode error", func(t *testing.T) {
		for _, body := range []string{
			`{"id":"x"}`,
			`{"id":1,"files":[{"sizeKB":"big"}]}`,
			`{"id":1,"modelId":"model"}`,
		} {
			if _, err := ParseCompactVersion([]byte(body)); err == nil {
				t.Errorf("expected decode error for %s", body)
			}
		}
	})

	t.Run("malformed root containers are rejected", func(t *testing.T) {
		for _, body := range []string{``, `null`, `[]`, `"version"`, `{`} {
			if _, err := ParseCompactVersion([]byte(body)); err == nil {
				t.Errorf("expected root error for %q", body)
			}
		}
	})
}

func TestCompactVersion_RequiredKeys(t *testing.T) {
	env, err := ParseCompactVersion([]byte(`{"id":1,"files":[{"id":2}]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(compactMarshal(t, env), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var version map[string]json.RawMessage
	if err := json.Unmarshal(envelope["version"], &version); err != nil {
		t.Fatalf("unmarshal version: %v", err)
	}
	for _, key := range []string{
		"id", "modelId", "name", "baseModel", "baseModelType", "publishedAt",
		"availability", "model", "stats", "trainedWords", "files", "examples",
	} {
		if _, ok := version[key]; !ok {
			t.Errorf("required key %q missing from version summary", key)
		}
	}
	var owning map[string]json.RawMessage
	if err := json.Unmarshal(version["model"], &owning); err != nil {
		t.Fatalf("unmarshal owning model: %v", err)
	}
	for _, key := range []string{"id", "name", "type"} {
		if _, ok := owning[key]; !ok {
			t.Errorf("required key %q missing from owning model", key)
		}
	}
	var files []map[string]json.RawMessage
	if err := json.Unmarshal(version["files"], &files); err != nil || len(files) != 1 {
		t.Fatalf("files = %s (%v)", version["files"], err)
	}
	for _, key := range []string{
		"id", "name", "sizeKB", "type", "primary", "metadata",
		"pickleScanResult", "virusScanResult", "scannedAt", "hashes",
	} {
		if _, ok := files[0][key]; !ok {
			t.Errorf("required key %q missing from compact file", key)
		}
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(files[0]["metadata"], &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	for _, key := range []string{"format", "fp", "size"} {
		if _, ok := metadata[key]; !ok {
			t.Errorf("required key %q missing from file metadata", key)
		}
	}
}

func TestCompactModels_EnvelopeErrors(t *testing.T) {
	for _, body := range []string{
		``,
		`null`,
		`[]`,
		`{"metadata":{}}`,
		`{"items":null}`,
		`{"items":{}}`,
		`{"items":123}`,
		`{"items":[{"id":"bad"}]}`,
		`{`,
	} {
		if _, err := ParseCompactModels([]byte(body)); err == nil {
			t.Errorf("expected envelope error for %q", body)
		}
	}

	env, err := ParseCompactModels([]byte(`{"items":[],"metadata":{"nextCursor":"ignored"}}`))
	if err != nil {
		t.Fatalf("empty items must be valid: %v", err)
	}
	if env.Items == nil || len(env.Items) != 0 {
		t.Errorf("items = %v, want supplied []", env.Items)
	}
	// An empty page still carries the full summary contract: returned 0,
	// coverage [], and explicit zero counters — never a cursor-derived total.
	wantEmpty := SearchResultSummary{Returned: 0, BaseCoverage: []SearchBaseCoverage{}}
	if !reflect.DeepEqual(env.Summary, wantEmpty) {
		t.Errorf("empty summary = %+v, want %+v", env.Summary, wantEmpty)
	}
	out := string(compactMarshal(t, env))
	if strings.Contains(out, "nextCursor") || !strings.Contains(out, `"items":[]`) {
		t.Errorf("output must be %s with empty items and no metadata echo, got %s", CompactSearchSchemaVersion, out)
	}
	if !strings.Contains(out, `"baseCoverage":[]`) {
		t.Errorf("empty coverage must serialize as [], got %s", out)
	}
}

func TestCompact_DeterministicRepeat(t *testing.T) {
	fixtures := []struct {
		name  string
		parse func([]byte) (any, error)
	}{
		{"search", func(b []byte) (any, error) { return ParseCompactModels(b) }},
		{"model", func(b []byte) (any, error) { return ParseCompactModel(b) }},
		{"version", func(b []byte) (any, error) { return ParseCompactVersion(b) }},
	}
	files := map[string]string{
		"search":  "models_search_anima.json",
		"model":   "models_2458426.json",
		"version": "version_3263843.json",
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			raw := testutil.LoadFixture(t, files[f.name])
			first, err := f.parse(raw)
			if err != nil {
				t.Fatalf("first parse: %v", err)
			}
			second, err := f.parse(raw)
			if err != nil {
				t.Fatalf("second parse: %v", err)
			}
			if a, b := string(compactMarshal(t, first)), string(compactMarshal(t, second)); a != b {
				t.Errorf("repeated projection differs:\n%s\n%s", a, b)
			}
			// The summary is part of the repeated projection and must be
			// just as deterministic (map iteration must not leak).
			if s1, ok := first.(*CompactSearchEnvelope); ok {
				s2 := second.(*CompactSearchEnvelope)
				if !reflect.DeepEqual(s1.Summary, s2.Summary) {
					t.Errorf("repeated summary differs:\n%+v\n%+v", s1.Summary, s2.Summary)
				}
			}
		})
	}
}

// TestCompact_SizeAndRecordPreservation proves the reduction comes from
// field removal rather than dropped records or whitespace-only minification:
// both sides are rendered semantically (indented), record counts and orders
// are compared against the legacy decoders, and compact must stay under 50%
// of the full indented payload for every representative fixture.
func TestCompact_SizeAndRecordPreservation(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		raw := testutil.LoadFixture(t, "models_search_anima.json")
		env, err := ParseCompactModels(raw)
		if err != nil {
			t.Fatalf("compact: %v", err)
		}
		var source struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(raw, &source); err != nil {
			t.Fatalf("unmarshal source: %v", err)
		}
		full := compactMarshalIndent(t, source.Items)
		compact := compactMarshalIndent(t, env)
		if len(compact)*2 >= len(full) {
			t.Errorf("compact %d bytes is not under half of full %d bytes", len(compact), len(full))
		}
		if len(env.Items) != len(source.Items) {
			t.Fatalf("items = %d, want %d", len(env.Items), len(source.Items))
		}
		var wantVersions int
		for _, item := range env.Items {
			wantVersions += len(item.Versions)
		}
		if wantVersions != 11 {
			t.Errorf("version refs = %d, want all 11 embedded versions (8+3)", wantVersions)
		}
		// The summary describes exactly the records above: returned equals
		// the item count and every observed fixture base is covered once.
		if env.Summary.Returned != len(source.Items) {
			t.Errorf("summary returned = %d, want %d", env.Summary.Returned, len(source.Items))
		}
		wantCoverage := []SearchBaseCoverage{
			{BaseModel: "Anima", Models: 1},
			{BaseModel: "SDXL 1.0", Models: 1},
		}
		if !reflect.DeepEqual(env.Summary.BaseCoverage, wantCoverage) {
			t.Errorf("summary coverage = %+v, want %+v", env.Summary.BaseCoverage, wantCoverage)
		}
	})

	t.Run("model", func(t *testing.T) {
		raw := testutil.LoadFixture(t, "models_2458426.json")
		source, err := ParseModel(raw)
		if err != nil {
			t.Fatalf("legacy parse: %v", err)
		}
		env, err := ParseCompactModel(raw)
		if err != nil {
			t.Fatalf("compact: %v", err)
		}
		full := compactMarshalIndent(t, source)
		compact := compactMarshalIndent(t, env)
		if len(compact)*2 >= len(full) {
			t.Errorf("compact %d bytes is not under half of full %d bytes", len(compact), len(full))
		}
		if len(env.Model.Versions) != len(source.ModelVersions) {
			t.Fatalf("versions = %d, want %d", len(env.Model.Versions), len(source.ModelVersions))
		}
		for i := range source.ModelVersions {
			if got, want := compactNumberText(env.Model.Versions[i].ID), strconv.Itoa(source.ModelVersions[i].ID); got != want {
				t.Errorf("version[%d] id = %q, want %q", i, got, want)
			}
		}
	})

	t.Run("version", func(t *testing.T) {
		raw := testutil.LoadFixture(t, "version_3263843.json")
		source, err := ParseVersion(raw)
		if err != nil {
			t.Fatalf("legacy parse: %v", err)
		}
		env, err := ParseCompactVersion(raw)
		if err != nil {
			t.Fatalf("compact: %v", err)
		}
		full := compactMarshalIndent(t, source)
		compact := compactMarshalIndent(t, env)
		if len(compact)*2 >= len(full) {
			t.Errorf("compact %d bytes is not under half of full %d bytes", len(compact), len(full))
		}
		if len(env.Version.Files) != len(source.Files) {
			t.Fatalf("files = %d, want %d", len(env.Version.Files), len(source.Files))
		}
		for i := range source.Files {
			if !reflect.DeepEqual(env.Version.Files[i].Hashes, source.Files[i].Hashes) {
				t.Errorf("file[%d] hashes were not fully preserved", i)
			}
			if compactNumberText(env.Version.Files[i].ID) != strconv.Itoa(source.Files[i].ID) {
				t.Errorf("file[%d] id mismatch", i)
			}
		}
	})
}

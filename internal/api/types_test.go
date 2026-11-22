package api

import (
	"encoding/json"
	"strings"
	"testing"

	"civitai-cli/internal/testutil"
)

func TestParseModels_SearchFixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "models_search_anima.json")
	items, err := ParseModels(raw)
	if err != nil {
		t.Fatalf("ParseModels: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	m := items[0]
	if m.ID != 2458426 || m.Name != "Anima" || m.Type != "Checkpoint" {
		t.Errorf("unexpected first item: %+v", m)
	}
	if m.NSFWLevel != "7" {
		t.Errorf("nsfwLevel = %q, want numeric 7", m.NSFWLevel)
	}
	if len(m.BaseModels) == 0 || m.BaseModels[0] != "Anima" {
		t.Errorf("baseModels = %v, want [Anima]", m.BaseModels)
	}
	if m.Creator.Username != "circlestone_labs" {
		t.Errorf("creator = %q", m.Creator.Username)
	}
	if m.Stats.DownloadCount == 0 || m.Stats.ThumbsUpCount == 0 {
		t.Errorf("stats not decoded: %+v", m.Stats)
	}
	if len(m.ModelVersions) == 0 {
		t.Fatalf("modelVersions empty")
	}
	v := m.ModelVersions[0]
	if v.BaseModel != "Anima" || v.ID == 0 {
		t.Errorf("version not decoded: %+v", v)
	}
	if len(v.Files) == 0 || v.Files[0].Hashes["SHA256"] == "" {
		t.Errorf("file hashes not decoded")
	}
}

func TestParseModel_DetailFixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "models_2458426.json")
	m, err := ParseModel(raw)
	if err != nil {
		t.Fatalf("ParseModel: %v", err)
	}
	if m.ID != 2458426 {
		t.Errorf("id = %d", m.ID)
	}
	if !m.AllowNoCredit || !m.AllowDerivatives || m.AllowDifferentLicense {
		t.Errorf("license flags decoded wrong: %+v", m)
	}
	if len(m.AllowCommercialUse) == 0 {
		t.Errorf("allowCommercialUse empty")
	}
}

func TestParseVersion_DetailFixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "version_3263843.json")
	v, err := ParseVersion(raw)
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}
	if v.ID != 3263843 || v.ModelID != 2458426 {
		t.Errorf("ids wrong: %d / %d", v.ID, v.ModelID)
	}
	if v.Model == nil || v.Model.Name != "Anima" || v.Model.Type != "Checkpoint" {
		t.Errorf("model summary wrong: %+v", v.Model)
	}
	if v.BaseModel != "Anima" {
		t.Errorf("baseModel = %q", v.BaseModel)
	}
	if len(v.Files) == 0 {
		t.Fatalf("files empty")
	}
	f := v.Files[0]
	if f.Metadata.Format != "SafeTensor" || f.Metadata.FP == "" {
		t.Errorf("file metadata wrong: %+v", f.Metadata)
	}
	if f.PickleScanResult != "Success" || f.VirusScanResult != "Success" {
		t.Errorf("scan results wrong: %q / %q", f.PickleScanResult, f.VirusScanResult)
	}
	for _, algo := range []string{"SHA256", "AutoV1", "AutoV2", "AutoV3", "BLAKE3", "CRC32"} {
		if f.Hashes[algo] == "" {
			t.Errorf("hash %s missing", algo)
		}
	}
	if len(v.Images) == 0 {
		t.Fatalf("images empty")
	}
	withMeta := 0
	for _, im := range v.Images {
		if im.Meta != nil {
			withMeta++
			if im.Meta.Prompt == "" || im.Meta.Sampler == "" {
				t.Errorf("meta fields missing: %+v", im.Meta)
			}
		}
	}
	if withMeta == 0 {
		t.Errorf("expected at least one image with meta")
	}
}

func TestParseVersion_ByHashFixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "byhash_anima_turbo.json")
	v, err := ParseVersion(raw)
	if err != nil {
		t.Fatalf("ParseVersion(by-hash): %v", err)
	}
	if v.ID != 3263843 {
		t.Errorf("by-hash resolved to version %d, want 3263843", v.ID)
	}
	// The by-hash fixture is the canonical SHA256 lookup target.
	found := false
	for _, f := range v.Files {
		if strings.EqualFold(f.Hashes["SHA256"], "FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected file with the probed SHA256")
	}
}

func TestModel_NSFWLevelDrift(t *testing.T) {
	cases := []struct {
		name string
		json string
		want json.Number
	}{
		{"numeric", `{"id":1,"nsfwLevel":7}`, "7"},
		{"numeric string", `{"id":1,"nsfwLevel":"7"}`, "7"},
		{"enum string drift", `{"id":1,"nsfwLevel":"Soft"}`, "Soft"},
		{"null", `{"id":1,"nsfwLevel":null}`, ""},
		{"absent", `{"id":1}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m Model
			if err := json.Unmarshal([]byte(tc.json), &m); err != nil {
				t.Fatalf("decode must tolerate drift: %v", err)
			}
			if m.NSFWLevel != tc.want {
				t.Errorf("nsfwLevel = %q, want %q", m.NSFWLevel, tc.want)
			}
		})
	}
}

func TestVersionAndImage_NSFWLevelDrift(t *testing.T) {
	var v ModelVersion
	if err := json.Unmarshal([]byte(`{"id":1,"nsfwLevel":"3"}`), &v); err != nil {
		t.Fatalf("version decode: %v", err)
	}
	if v.NSFWLevel != "3" {
		t.Errorf("version nsfwLevel = %q", v.NSFWLevel)
	}
	var im Image
	if err := json.Unmarshal([]byte(`{"url":"u","nsfwLevel":"None","hasMeta":true,"meta":null}`), &im); err != nil {
		t.Fatalf("image decode: %v", err)
	}
	if im.NSFWLevel != "None" {
		t.Errorf("image nsfwLevel = %q, want the string form preserved", im.NSFWLevel)
	}
	if im.Meta != nil {
		t.Errorf("null meta must decode to nil")
	}
}

func TestImage_MetaNullSafety(t *testing.T) {
	// hasMeta=true does not guarantee meta (verified live): both null
	// and absent meta must decode without error.
	var v ModelVersion
	body := `{"id":1,"images":[{"url":"a","hasMeta":true,"meta":null},{"url":"b","hasMeta":true}]}`
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i, im := range v.Images {
		if im.Meta != nil {
			t.Errorf("image %d: meta should be nil", i)
		}
	}
}

func TestImage_MetaComfyWidthHeight(t *testing.T) {
	// ComfyUI uploads embed node objects in meta.width/height; decoding
	// must not fail (raw message preserves them for later milestones).
	var im Image
	body := `{"url":"u","meta":{"prompt":"p","steps":"8","cfgScale":1.5,"width":{"class_type":"ResolutionSelector"},"height":1024}}`
	if err := json.Unmarshal([]byte(body), &im); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if im.Meta == nil || im.Meta.Steps != "8" || im.Meta.CFGScale != "1.5" {
		t.Errorf("meta numeric drift not tolerated: %+v", im.Meta)
	}
	if !strings.Contains(string(im.Meta.Width), "ResolutionSelector") {
		t.Errorf("width object not preserved: %s", im.Meta.Width)
	}
}

func TestStats_ZeroValueSafe(t *testing.T) {
	var m Model
	if err := json.Unmarshal([]byte(`{"id":1,"stats":{}}`), &m); err != nil {
		t.Fatalf("empty stats: %v", err)
	}
	if m.Stats != (Stats{}) {
		t.Errorf("empty stats should be zero: %+v", m.Stats)
	}
	var m2 Model
	if err := json.Unmarshal([]byte(`{"id":1}`), &m2); err != nil {
		t.Fatalf("absent stats: %v", err)
	}
	if m2.Stats != (Stats{}) {
		t.Errorf("absent stats should be zero: %+v", m2.Stats)
	}
}

func TestModel_UnknownEnumPreserved(t *testing.T) {
	var m Model
	if err := json.Unmarshal([]byte(`{"id":1,"type":"BrandNewType","availability":"Future"}`), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Type != "BrandNewType" || m.Availability != "Future" {
		t.Errorf("unknown enums must be preserved as-is: %q / %q", m.Type, m.Availability)
	}
}

func TestModelsRawItems_VerbatimPassthrough(t *testing.T) {
	raw := testutil.LoadFixture(t, "models_search_anima.json")
	items, err := ModelsRawItems(raw)
	if err != nil {
		t.Fatalf("ModelsRawItems: %v", err)
	}
	if !json.Valid(items) {
		t.Fatalf("extracted items not valid JSON")
	}
	// Verbatim: the extracted bytes must appear unchanged in the source
	// body (no re-serialization), and must be an array.
	if !strings.Contains(string(raw), string(items)) {
		t.Errorf("items were re-serialized instead of passed through")
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(items, &arr); err != nil || len(arr) != 2 {
		t.Errorf("items must be the 2-element array: %v", err)
	}
}

func TestModelsRawItems_MissingItems(t *testing.T) {
	if _, err := ModelsRawItems([]byte(`{"metadata":{}}`)); err == nil {
		t.Errorf("expected error for envelope without items")
	}
}

func TestParseTags(t *testing.T) {
	raw := []byte(`{"items":[{"name":"anime","link":"/tags/anime"},{"name":"anima","link":"/tags/anima"}],"metadata":{}}`)
	tags, err := ParseTags(raw)
	if err != nil {
		t.Fatalf("ParseTags: %v", err)
	}
	if len(tags) != 2 || tags[0].Name != "anime" {
		t.Errorf("tags decoded wrong: %+v", tags)
	}
}

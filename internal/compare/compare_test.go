package compare

import (
	"encoding/json"
	"strings"
	"testing"

	"civitai-cli/internal/api"
)

// compareModelSnapshot parses one inline model body for a test.
func compareModelSnapshot(t *testing.T, body string) *api.CompareModel {
	t.Helper()
	m, err := api.ParseCompareModel([]byte(body))
	if err != nil {
		t.Fatalf("parse model: %v", err)
	}
	return m
}

// compareVersionSnapshot parses one inline version body for a test.
func compareVersionSnapshot(t *testing.T, body string) *api.CompareVersion {
	t.Helper()
	v, err := api.ParseCompareVersion([]byte(body))
	if err != nil {
		t.Fatalf("parse version: %v", err)
	}
	return v
}

// compareField finds a field by key or fails the test.
func compareField(t *testing.T, res *Result, key string) Field {
	t.Helper()
	for _, f := range res.Fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("field %q not found", key)
	return Field{}
}

// compareMarshal renders a value as compact JSON for assertions.
func compareMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestCompareStatesTable(t *testing.T) {
	cases := []struct {
		name       string
		values     []Value
		complete   bool
		comparison Comparison
	}{
		{"false/false", []Value{{State: StateValue, Value: false}, {State: StateValue, Value: false}}, true, ComparisonEqual},
		{"missing/missing", []Value{{State: StateMissing}, {State: StateMissing}}, false, ComparisonIndeterminate},
		{"emptySet/missing", []Value{{State: StateValue, Value: []string{}}, {State: StateMissing}}, false, ComparisonIndeterminate},
		{"zero/missing", []Value{{State: StateValue, Value: int64(0)}, {State: StateMissing}}, false, ComparisonIndeterminate},
		{"emptyString/missing", []Value{{State: StateValue, Value: ""}, {State: StateMissing}}, false, ComparisonIndeterminate},
		{"known/missing/known", []Value{{State: StateValue, Value: int64(1)}, {State: StateMissing}, {State: StateValue, Value: int64(2)}}, false, ComparisonDifferent},
		{"null/null", []Value{{State: StateNull}, {State: StateNull}}, true, ComparisonEqual},
		{"null/emptySet", []Value{{State: StateNull}, {State: StateValue, Value: []string{}}}, true, ComparisonDifferent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			complete, comparison := compareStates(tc.values)
			if complete != tc.complete || comparison != tc.comparison {
				t.Fatalf("got complete=%v comparison=%s, want %v/%s",
					complete, comparison, tc.complete, tc.comparison)
			}
		})
	}
}

func TestCompareSetsAreCaseSensitive(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"trainedWords":["Alpha"]}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"trainedWords":["alpha"]}`), Parent: parent},
	})
	trig := compareField(t, res, "trainedWords")
	if trig.Comparison != ComparisonDifferent {
		t.Fatalf("trigger words compare case-sensitively: %+v", trig)
	}
	d := trig.Derived.(*SetDiff)
	if len(d.Changes) != 1 || strings.Join(d.Changes[0].Added, ",") != "alpha" ||
		strings.Join(d.Changes[0].Removed, ",") != "Alpha" {
		t.Fatalf("case-distinct set delta = %+v", d.Changes)
	}
}

func TestCompareBuildVersions_IdenticalInputs(t *testing.T) {
	body := `{
		"id":1,"modelId":7,"name":"v1","baseModel":"Anima","baseModelType":"Standard",
		"publishedAt":"2026-01-01T00:00:00Z","status":"Published","availability":"Public",
		"nsfwLevel":3,"trainedWords":["b","a"],"stats":{"downloadCount":10,"thumbsUpCount":2,"thumbsDownCount":1},
		"files":[{"id":9,"name":"f.safetensors","type":"Model","primary":true,"sizeKB":10,
			"pickleScanResult":"Success","virusScanResult":"Success","scannedAt":"2026-01-01T00:00:00Z",
			"metadata":{"format":"SafeTensor","fp":"bf16"},"hashes":{"SHA256":"aa","AutoV2":"bb"}}],
		"images":[{"hasMeta":true,"meta":{"prompt":"p"}}]
	}`
	parent := compareModelSnapshot(t, `{"id":7,"name":"Anima","type":"Checkpoint","creator":{"username":"u"},
		"allowNoCredit":true,"allowCommercialUse":["Image"],"allowDerivatives":true,"allowDifferentLicense":false,
		"modelVersions":[{"id":1,"availability":"Public","status":"Published"}]}`)
	subs := []VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, body), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, body), Parent: parent},
	}
	res := BuildVersions(subs)
	if res.SchemaVersion != SchemaVersion || res.Kind != KindVersion || res.Baseline != 0 {
		t.Fatalf("header wrong: %+v", res)
	}
	if len(res.Subjects) != 2 || res.Subjects[0].VersionID != 1 || res.Subjects[1].VersionID != 2 {
		t.Fatalf("subject order/identity wrong: %+v", res.Subjects)
	}
	if vid := compareField(t, res, "versionId"); vid.Comparison != ComparisonDifferent || vid.Derived != nil {
		t.Fatalf("version identity must differ without derivation: %+v", vid)
	}
	for _, f := range res.Fields {
		if IsIdentityField(res.Kind, f.Key) {
			if f.Derived != nil {
				t.Fatalf("identity field %s must not derive changes: %+v", f.Key, f)
			}
			continue
		}
		if len(f.Values) != 2 {
			t.Fatalf("%s values=%d, want 2", f.Key, len(f.Values))
		}
		if f.Comparison != ComparisonEqual || !f.Complete {
			t.Fatalf("%s comparison=%s complete=%v; identical facts must be equal", f.Key, f.Comparison, f.Complete)
		}
		if f.Derived == nil {
			t.Fatalf("%s must carry a derived operation", f.Key)
		}
		switch d := f.Derived.(type) {
		case *ScalarDiff:
			if d.State != DerivedApplied || len(d.Changes) != 0 {
				t.Fatalf("%s scalar derived = %+v", f.Key, d)
			}
		case *SetDiff:
			if d.State != DerivedApplied || d.Changes == nil || len(d.Changes) != 1 {
				t.Fatalf("%s set derived = %+v", f.Key, d)
			}
			if d.Changes[0].Added == nil || d.Changes[0].Removed == nil {
				t.Fatalf("%s applied set diff must use non-nil arrays: %+v", f.Key, d.Changes[0])
			}
		case *NumberDiff:
			if d.State != DerivedApplied || len(d.Changes) != 0 {
				t.Fatalf("%s number derived = %+v", f.Key, d)
			}
		case *FilesDiff:
			if d.State != DerivedApplied || len(d.Changes) != 1 || d.Changes[0].State != DerivedApplied {
				t.Fatalf("%s files derived = %+v", f.Key, d)
			}
			if len(d.Changes[0].Pairs) != 1 || d.Changes[0].Pairs[0].MatchBasis != "fileId" {
				t.Fatalf("%s files pairs = %+v", f.Key, d.Changes[0].Pairs)
			}
		case *RecipeDiff:
			if d.State != DerivedApplied || len(d.Changes) != 0 {
				t.Fatalf("%s recipe derived = %+v", f.Key, d)
			}
		default:
			t.Fatalf("%s unexpected derived type %T", f.Key, f.Derived)
		}
	}
	// Trigger input order and duplicates must not create a difference.
	tw := compareField(t, res, "trainedWords")
	if compareMarshal(t, tw.Values[0].Value) != `["a","b"]` {
		t.Fatalf("trigger set must be sorted: %s", compareMarshal(t, tw.Values[0].Value))
	}
}

func TestCompareMissingNullEmptyRules(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"availability":"A","trainedWords":null}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7}`), Parent: parent},
		{ID: 3, Snapshot: compareVersionSnapshot(t, `{"id":3,"modelId":7,"availability":"B","trainedWords":[]}`), Parent: parent},
	})

	availability := compareField(t, res, "availability")
	if availability.Comparison != ComparisonDifferent || availability.Complete {
		t.Fatalf("two differing values plus a missing one must be different/incomplete: %+v", availability)
	}
	if availability.Values[1].State != StateMissing {
		t.Fatalf("second availability must be missing: %+v", availability.Values[1])
	}
	if d, ok := availability.Derived.(*ScalarDiff); !ok || d.State != DerivedIndeterminate || d.Changes != nil {
		t.Fatalf("scalar derived must be indeterminate without fabricating changes: %#v", availability.Derived)
	}

	triggers := compareField(t, res, "trainedWords")
	if triggers.Comparison != ComparisonDifferent {
		t.Fatalf("null and [] are different supplied states: %+v", triggers)
	}
	if triggers.Values[0].State != StateNull || triggers.Values[2].State != StateValue {
		t.Fatalf("trainedWords states wrong: %+v", triggers.Values)
	}
	if d, ok := triggers.Derived.(*SetDiff); !ok || d.State != DerivedIndeterminate {
		t.Fatalf("set derived must be indeterminate: %#v", triggers.Derived)
	}
}

func TestCompareMissingMissingAndKnownMissing(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7}`), Parent: parent},
	})
	base := compareField(t, res, "baseModel")
	if base.Comparison != ComparisonIndeterminate || base.Complete {
		t.Fatalf("missing/missing must be indeterminate: %+v", base)
	}

	res = BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"baseModel":"A"}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7}`), Parent: parent},
	})
	base = compareField(t, res, "baseModel")
	if base.Comparison != ComparisonIndeterminate || base.Complete {
		t.Fatalf("equal-known plus missing must be indeterminate, not equal: %+v", base)
	}
}

func TestCompareNullAndEmptyAreDifferent(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"availability":null}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"availability":""}`), Parent: parent},
	})
	f := compareField(t, res, "availability")
	if f.Comparison != ComparisonDifferent || !f.Complete {
		t.Fatalf("null and empty are supplied but different: %+v", f)
	}
	if d, ok := f.Derived.(*ScalarDiff); !ok || d.State != DerivedIndeterminate {
		t.Fatalf("null operand cannot produce a scalar transition: %#v", f.Derived)
	}
}

func TestCompareBoolFalseAndMissing(t *testing.T) {
	p1 := compareModelSnapshot(t, `{"id":7,"allowNoCredit":false}`)
	p2 := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7}`), Parent: p1},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7}`), Parent: p2},
	})
	f := compareField(t, res, "allowNoCredit")
	if f.Comparison != ComparisonIndeterminate || f.Complete {
		t.Fatalf("false plus missing must be indeterminate: %+v", f)
	}
	if f.Values[0].State != StateValue || f.Values[0].Value != false {
		t.Fatalf("supplied false must stay a value: %+v", f.Values[0])
	}
	if f.Values[0].Source != SourceModel {
		t.Fatalf("license must be labeled model-level, source=%q", f.Values[0].Source)
	}
}

func TestCompareModelSummaryAndLatestPublication(t *testing.T) {
	res := BuildModels([]ModelSubject{
		{ID: 1, Snapshot: compareModelSnapshot(t, `{
			"id":1,"name":"A","type":"Checkpoint","creator":{"username":"u"},
			"availability":"Public","allowNoCredit":false,
			"stats":{"downloadCount":"5","thumbsUpCount":1,"thumbsDownCount":0,"commentCount":2,"tippedAmountCount":3},
			"modelVersions":[
				{"id":10,"publishedAt":"2026-01-01T10:00:00+08:00"},
				{"id":11,"publishedAt":"2026-01-01T09:00:00Z"}
			]}`)},
		{ID: 2, Snapshot: compareModelSnapshot(t, `{"id":2,"name":"B","modelVersions":[]}`)},
	})
	count := compareField(t, res, "versionCount")
	if count.Comparison != ComparisonDifferent || count.Values[0].Value != int64(2) || count.Values[1].Value != int64(0) {
		t.Fatalf("version counts wrong: %+v", count.Values)
	}
	latest := compareField(t, res, "latestPublishedAt")
	if latest.Values[0].State != StateValue || latest.Values[0].Value != "2026-01-01T09:00:00Z" {
		t.Fatalf("latest publication must normalize the RFC3339 instant: %+v", latest.Values[0])
	}
	if latest.Values[0].Source != SourceDerived {
		t.Fatalf("latest publication source=%q", latest.Values[0].Source)
	}
	if latest.Values[1].State != StateMissing {
		t.Fatalf("empty version list cannot produce a latest timestamp: %+v", latest.Values[1])
	}
	stats := compareField(t, res, "downloadCount")
	if d, ok := stats.Derived.(*NumberDiff); !ok || d.State != DerivedIndeterminate {
		t.Fatalf("deltas require both operands: %#v", stats.Derived)
	}
	url := compareField(t, res, "url")
	if url.Values[0].Source != SourceDerived || url.Values[0].Value != "https://civitai.com/models/1" {
		t.Fatalf("model url projection wrong: %+v", url.Values[0])
	}
}

func TestCompareInvalidTimestampMakesSummaryMissing(t *testing.T) {
	res := BuildModels([]ModelSubject{
		{ID: 1, Snapshot: compareModelSnapshot(t, `{"id":1,"modelVersions":[{"id":10,"publishedAt":"not-a-date"}]}`)},
		{ID: 2, Snapshot: compareModelSnapshot(t, `{"id":2,"modelVersions":[{"id":20}]}`)},
	})
	latest := compareField(t, res, "latestPublishedAt")
	for i, v := range latest.Values {
		if v.State != StateMissing {
			t.Fatalf("subject %d invalid timestamp must be missing, got %+v", i, v)
		}
	}
	if latest.Comparison != ComparisonIndeterminate {
		t.Fatalf("invalid timestamps must stay indeterminate: %+v", latest)
	}
}

func TestCompareVersionSourcesAndAvailabilityFallback(t *testing.T) {
	parent := compareModelSnapshot(t, `{
		"id":7,"name":"Anima","type":"Checkpoint","creator":{"username":"u"},
		"availability":"Private","allowNoCredit":true,
		"modelVersions":[{"id":1,"availability":"Public","status":"Published"}]
	}`)
	res := BuildVersions([]VersionSubject{
		// Standalone detail omits availability/status: the matching embedded
		// entry supplies them, and the model's own availability is forbidden.
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"baseModel":"Anima","baseModelType":"Standard","stats":{"downloadCount":4}}`), Parent: parent},
		// Supplied empty availability stays authoritative; no fallback.
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"availability":"","status":"Archived"}`), Parent: parent},
	})
	av := compareField(t, res, "availability")
	if av.Values[0].Source != SourceEmbeddedVersion || av.Values[0].Value != "Public" {
		t.Fatalf("missing availability must fall back to the matching embedded version: %+v", av.Values[0])
	}
	if av.Values[1].Source != SourceVersion || av.Values[1].Value != "" {
		t.Fatalf("supplied empty availability must stay authoritative: %+v", av.Values[1])
	}
	if av.Comparison != ComparisonDifferent {
		t.Fatalf("Public vs empty must differ: %+v", av)
	}
	status := compareField(t, res, "status")
	if status.Values[0].Source != SourceEmbeddedVersion || status.Values[1].Source != SourceVersion {
		t.Fatalf("status sources wrong: %+v", status.Values)
	}
	if got := compareField(t, res, "baseModel"); got.Values[0].Source != SourceVersion {
		t.Fatalf("baseModel source=%q", got.Values[0].Source)
	}
	if got := compareField(t, res, "creator"); got.Values[0].Source != SourceModel {
		t.Fatalf("creator source=%q", got.Values[0].Source)
	}
	if got := compareField(t, res, "downloadCount"); got.Values[0].Source != SourceVersion {
		t.Fatalf("version stats source=%q", got.Values[0].Source)
	}
}

func TestCompareVersionMissingOwnerAndNoModelAvailabilitySubstitution(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7,"availability":"Public"}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7}`), Parent: parent},
	})
	av := compareField(t, res, "availability")
	if av.Values[0].State != StateMissing {
		t.Fatalf("model availability must never be substituted for version availability: %+v", av.Values[0])
	}
}

func TestCompareDerivedScalarAndSetChanges(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"baseModel":"SD 1.5","trainedWords":["alpha","beta"]}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"baseModel":"SDXL","trainedWords":["beta","gamma","gamma"]}`), Parent: parent},
		{ID: 3, Snapshot: compareVersionSnapshot(t, `{"id":3,"modelId":7,"baseModel":"SD 1.5","trainedWords":["alpha","beta"]}`), Parent: parent},
	})
	base := compareField(t, res, "baseModel")
	d := base.Derived.(*ScalarDiff)
	if d.State != DerivedApplied || len(d.Changes) != 1 {
		t.Fatalf("scalar changes = %+v", d)
	}
	if d.Changes[0].Subject != 1 || d.Changes[0].From != "SD 1.5" || d.Changes[0].To != "SDXL" {
		t.Fatalf("scalar change = %+v", d.Changes[0])
	}
	trig := compareField(t, res, "trainedWords")
	sd := trig.Derived.(*SetDiff)
	if sd.State != DerivedApplied || len(sd.Changes) != 2 {
		t.Fatalf("set changes = %+v", sd)
	}
	if strings.Join(sd.Changes[0].Added, ",") != "gamma" || strings.Join(sd.Changes[0].Removed, ",") != "alpha" {
		t.Fatalf("set delta subject1 = %+v", sd.Changes[0])
	}
	if len(sd.Changes[1].Added) != 0 || len(sd.Changes[1].Removed) != 0 {
		t.Fatalf("subject2 is equal to baseline: %+v", sd.Changes[1])
	}
}

func TestCompareFilePairingCascade(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	build := func(baseBody, candBody string) (*Result, FileSubjectDiff) {
		res := BuildVersions([]VersionSubject{
			{ID: 1, Snapshot: compareVersionSnapshot(t, baseBody), Parent: parent},
			{ID: 2, Snapshot: compareVersionSnapshot(t, candBody), Parent: parent},
		})
		files := compareField(t, res, "files")
		d := files.Derived.(*FilesDiff)
		if d.State != DerivedApplied || len(d.Changes) != 1 {
			t.Fatalf("files diff = %+v", d)
		}
		return res, d.Changes[0]
	}

	// Same registry ID with a changed hash.
	_, diff := build(
		`{"id":1,"modelId":7,"files":[{"id":9,"name":"a","type":"Model","hashes":{"SHA256":"AA"}}]}`,
		`{"id":2,"modelId":7,"files":[{"id":9,"name":"a","type":"Model","hashes":{"SHA256":"BB"}}]}`,
	)
	if len(diff.Pairs) != 1 || diff.Pairs[0].MatchBasis != "fileId" {
		t.Fatalf("same file id must pair by fileId: %+v", diff)
	}
	if len(diff.Pairs[0].Hashes.Changed) != 1 || diff.Pairs[0].Hashes.Changed[0].Algorithm != "SHA256" {
		t.Fatalf("hash change missing: %+v", diff.Pairs[0].Hashes)
	}

	// Same unique SHA256 with a new registry ID and name.
	_, diff = build(
		`{"id":1,"modelId":7,"files":[{"id":9,"name":"old","type":"Model","hashes":{"SHA256":"AA"}}]}`,
		`{"id":2,"modelId":7,"files":[{"id":10,"name":"new","type":"Model","hashes":{"SHA256":"AA"}}]}`,
	)
	if len(diff.Pairs) != 1 || diff.Pairs[0].MatchBasis != "sha256" {
		t.Fatalf("carried binary must pair by sha256: %+v", diff)
	}
	if len(diff.Pairs[0].Changes) == 0 {
		t.Fatalf("renamed carried binary must report metadata changes: %+v", diff.Pairs[0])
	}
	if len(diff.Added) != 0 || len(diff.Removed) != 0 {
		t.Fatalf("sha256 pair must consume both registry entries: %+v", diff)
	}

	// Same unique name/type with different content.
	_, diff = build(
		`{"id":1,"modelId":7,"files":[{"id":9,"name":"x","type":"Model","hashes":{"SHA256":"AA"}}]}`,
		`{"id":2,"modelId":7,"files":[{"id":10,"name":"x","type":"Model","hashes":{"SHA256":"BB"}}]}`,
	)
	if len(diff.Pairs) != 1 || diff.Pairs[0].MatchBasis != "nameType" {
		t.Fatalf("content replacement must pair by nameType: %+v", diff)
	}

	// Ambiguous SHA256 on the baseline never guesses a pair.
	_, diff = build(
		`{"id":1,"modelId":7,"files":[
			{"id":9,"name":"a","type":"Model","hashes":{"SHA256":"AA"}},
			{"id":10,"name":"b","type":"Model","hashes":{"SHA256":"AA"}}]}`,
		`{"id":2,"modelId":7,"files":[{"id":11,"name":"c","type":"Model","hashes":{"SHA256":"AA"}}]}`,
	)
	if len(diff.Pairs) != 0 {
		t.Fatalf("ambiguous sha256 must not pair: %+v", diff.Pairs)
	}
	if len(diff.Added) != 1 || diff.Added[0].Reason != "ambiguous sha256 match" {
		t.Fatalf("unmatched candidate must stay visible with a reason: %+v", diff.Added)
	}
	if len(diff.Removed) != 2 {
		t.Fatalf("unmatched baseline files must stay visible: %+v", diff.Removed)
	}

	// Duplicate name/type is also ambiguous.
	_, diff = build(
		`{"id":1,"modelId":7,"files":[
			{"id":9,"name":"x","type":"Model"},
			{"id":10,"name":"x","type":"Model"}]}`,
		`{"id":2,"modelId":7,"files":[{"id":11,"name":"x","type":"Model"}]}`,
	)
	if len(diff.Pairs) != 0 || len(diff.Added) != 1 || diff.Added[0].Reason != "ambiguous nameType match" {
		t.Fatalf("duplicate name/type must stay unmatched with a reason: %+v", diff)
	}
}

func TestCompareFileAndHashChanges(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	_, diff := func() (*Result, FileSubjectDiff) {
		res := BuildVersions([]VersionSubject{
			{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"files":[{
				"id":9,"name":"f","type":"Model","primary":true,"sizeKB":10,"scannedAt":"2026-01-01T00:00:00Z",
				"pickleScanResult":"Success","virusScanResult":"Success",
				"metadata":{"format":"SafeTensor","fp":"bf16"},
				"hashes":{"AutoV2":"bb","CRC32":"cc","SHA256":"aa"}}]}`), Parent: parent},
			{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"files":[{
				"id":9,"name":"f","type":"Model","primary":false,"sizeKB":11,"scannedAt":"2026-02-01T00:00:00Z",
				"pickleScanResult":"Success","virusScanResult":"Error",
				"metadata":{"format":"SafeTensor","fp":"fp16"},
				"hashes":{"AutoV2":"cc","BLAKE3":"dd","SHA256":"AA"}}]}`), Parent: parent},
		})
		return res, compareField(t, res, "files").Derived.(*FilesDiff).Changes[0]
	}()
	if len(diff.Pairs) != 1 {
		t.Fatalf("pair missing: %+v", diff)
	}
	pair := diff.Pairs[0]
	wantChanges := map[string]bool{"primary": false, "sizeKB": false, "scannedAt": false, "fp": false, "virusScanResult": false}
	for _, c := range pair.Changes {
		if _, ok := wantChanges[c.Field]; ok {
			wantChanges[c.Field] = true
		}
		if c.Field == "pickleScanResult" {
			t.Fatalf("unchanged scan must not appear: %+v", pair.Changes)
		}
	}
	for field, seen := range wantChanges {
		if !seen {
			t.Fatalf("matched-file change %q missing: %+v", field, pair.Changes)
		}
	}
	if pair.Hashes.State != DerivedApplied {
		t.Fatalf("hash state = %q", pair.Hashes.State)
	}
	if len(pair.Hashes.Changed) != 1 || pair.Hashes.Changed[0].Algorithm != "AutoV2" {
		t.Fatalf("hash change = %+v", pair.Hashes.Changed)
	}
	if len(pair.Hashes.Added) != 1 || pair.Hashes.Added[0].Algorithm != "BLAKE3" {
		t.Fatalf("hash addition = %+v", pair.Hashes.Added)
	}
	if len(pair.Hashes.Removed) != 1 || pair.Hashes.Removed[0].Algorithm != "CRC32" {
		t.Fatalf("hash removal = %+v", pair.Hashes.Removed)
	}
}

func TestCompareMissingScansStayMissing(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"files":[{"id":9,"name":"f","type":"Model","hashes":{"SHA256":"AA"}}]}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"files":[{"id":9,"name":"f","type":"Model","pickleScanResult":"Success","hashes":{"SHA256":"AA"}}]}`), Parent: parent},
	})
	pair := compareField(t, res, "files").Derived.(*FilesDiff).Changes[0].Pairs[0]
	for _, c := range pair.Changes {
		if c.Field == "pickleScanResult" {
			t.Fatalf("missing scan must not be compared as an empty value: %+v", c)
		}
	}
}

func TestCompareFileOrderAndHashOrderDoNotCreateDifferences(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"files":[
			{"id":10,"name":"b","type":"Model","hashes":{"SHA256":"BB","AutoV2":"b2"}},
			{"id":9,"name":"a","type":"Model","hashes":{"SHA256":"AA","AutoV2":"a2"}}]}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"files":[
			{"id":9,"name":"a","type":"Model","hashes":{"AutoV2":"A2","SHA256":"aa"}},
			{"id":10,"name":"b","type":"Model","hashes":{"SHA256":"BB","AutoV2":"b2"}}]}`), Parent: parent},
	})
	files := compareField(t, res, "files")
	if files.Comparison != ComparisonEqual {
		t.Fatalf("file/hash reordering and case-only hex must not differ: %+v", files.Values)
	}
	if d := files.Derived.(*FilesDiff); len(d.Changes[0].Pairs) != 2 || len(d.Changes[0].Pairs[0].Changes) != 0 {
		t.Fatalf("reordered files must pair without changes: %+v", d.Changes[0])
	}
	if d := files.Derived.(*FilesDiff); len(d.Changes[0].Pairs[0].Hashes.Changed) != 0 ||
		len(d.Changes[0].Pairs[0].Hashes.Added) != 0 || len(d.Changes[0].Pairs[0].Hashes.Removed) != 0 {
		t.Fatalf("case-only hash hex must be equal: %+v", d.Changes[0].Pairs[0].Hashes)
	}
}

func TestCompareRecipeCoverage(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"images":[
			{"hasMeta":true,"meta":null},
			{"hasMeta":true,"meta":{}},
			{"hasMeta":false},
			{"hasMeta":null,"meta":{"prompt":"p","steps":8,"width":{"nodes":{}}}},
			{}]}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"images":[]}`), Parent: parent},
	})
	cov := compareField(t, res, "recipeCoverage")
	base := cov.Values[0].Value.(RecipeCoverage)
	if base.ImagesTotal != 5 || base.HasMetaTrue != 2 || base.HasMetaFalse != 1 || base.HasMetaNull != 1 || base.HasMetaMissing != 1 {
		t.Fatalf("hasMeta coverage = %+v", base)
	}
	if base.MetaPresent != 2 || base.DeclaredMetaMissing != 1 {
		t.Fatalf("metadata coverage = %+v", base)
	}
	if base.NonEmpty.Prompt != 1 || base.NonEmpty.Steps != 1 || base.NonEmpty.Width != 1 {
		t.Fatalf("non-empty coverage = %+v", base.NonEmpty)
	}
	d := cov.Derived.(*RecipeDiff)
	if d.State != DerivedApplied {
		t.Fatalf("recipe diff state = %q", d.State)
	}
	changes := map[string]RecipeCountChange{}
	for _, c := range d.Changes {
		changes[c.Field] = c
	}
	if c, ok := changes["imagesTotal"]; !ok || c.From != 5 || c.To != 0 || c.Delta != -5 {
		t.Fatalf("imagesTotal delta = %+v", changes["imagesTotal"])
	}
	if _, ok := changes["hasMetaTrue"]; !ok {
		t.Fatalf("hasMetaTrue delta missing: %+v", d.Changes)
	}
	if _, ok := changes["prompt"]; !ok {
		t.Fatalf("prompt delta missing: %+v", d.Changes)
	}

	// Missing images is indeterminate, not zero coverage.
	res = BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"images":[]}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7}`), Parent: parent},
	})
	cov = compareField(t, res, "recipeCoverage")
	if cov.Comparison != ComparisonIndeterminate {
		t.Fatalf("missing images must be indeterminate: %+v", cov)
	}
	if d := cov.Derived.(*RecipeDiff); d.State != DerivedIndeterminate || d.Changes != nil {
		t.Fatalf("missing images must not fabricate zero-count deltas: %+v", d)
	}
}

func TestCompareThreeSubjectsBaselineOnly(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7}`)
	res := BuildVersions([]VersionSubject{
		{ID: 1, Snapshot: compareVersionSnapshot(t, `{"id":1,"modelId":7,"baseModel":"A","availability":"Public"}`), Parent: parent},
		{ID: 2, Snapshot: compareVersionSnapshot(t, `{"id":2,"modelId":7,"baseModel":"B","availability":"Public"}`), Parent: parent},
		{ID: 3, Snapshot: compareVersionSnapshot(t, `{"id":3,"modelId":7,"baseModel":"C","availability":"Public"}`), Parent: parent},
	})
	base := compareField(t, res, "baseModel")
	if base.Comparison != ComparisonDifferent {
		t.Fatalf("N-way difference must consider all subjects: %+v", base)
	}
	d := base.Derived.(*ScalarDiff)
	if len(d.Changes) != 2 || d.Changes[0].Subject != 1 || d.Changes[1].Subject != 2 {
		t.Fatalf("derived changes must be baseline-to-each-later only: %+v", d.Changes)
	}
	av := compareField(t, res, "availability")
	if av.Comparison != ComparisonEqual {
		t.Fatalf("three equal values must be equal: %+v", av)
	}
}

func TestCompareJSONDeterministicAndCurated(t *testing.T) {
	parent := compareModelSnapshot(t, `{"id":7,"name":"Anima","description":"SENTINEL-DESC"}`)
	baseBody := `{
		"id":1,"modelId":7,"name":"v1","baseModel":"Anima","trainedWords":["b","a"],
		"description":"SENTINEL-DESC",
		"downloadUrl":"SENTINEL-DL",
		"files":[{"id":9,"name":"f","type":"Model","downloadUrl":"SENTINEL-DL",
			"hashes":{"BLAKE3":"bb","SHA256":"aa","AutoV2":"cc"}}],
		"images":[{"url":"SENTINEL-IMG","hasMeta":true,"meta":{"prompt":"SENTINEL-PROMPT"}}]
	}`
	candBody := `{
		"id":2,"modelId":7,"name":"v2","baseModel":"Anima","trainedWords":["a","b"],
		"files":[{"id":9,"name":"f","type":"Model",
			"hashes":{"AutoV2":"cc","SHA256":"aa","BLAKE3":"bb"}}],
		"images":[{"url":"SENTINEL-IMG","hasMeta":true,"meta":{"prompt":"SENTINEL-PROMPT"}}]
	}`
	build := func() *Result {
		return BuildVersions([]VersionSubject{
			{ID: 1, Snapshot: compareVersionSnapshot(t, baseBody), Parent: parent},
			{ID: 2, Snapshot: compareVersionSnapshot(t, candBody), Parent: parent},
		})
	}
	first := compareMarshal(t, build())
	second := compareMarshal(t, build())
	if first != second {
		t.Fatalf("identical decoded input must produce byte-identical JSON\nfirst:  %s\nsecond: %s", first, second)
	}
	for _, banned := range []string{"SENTINEL-DESC", "SENTINEL-DL", "SENTINEL-IMG", "SENTINEL-PROMPT"} {
		if strings.Contains(first, banned) {
			t.Fatalf("JSON must not leak %q: %s", banned, first)
		}
	}
	for _, banned := range []string{`"rank"`, `"score"`, `"winner"`, `"recommendation"`, `"generatedAt"`} {
		if strings.Contains(first, banned) {
			t.Fatalf("JSON must not contain %s", banned)
		}
	}
	var doc struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		Baseline      int    `json:"baseline"`
		Subjects      []struct {
			Index     int   `json:"index"`
			ModelID   int64 `json:"modelId"`
			VersionID int64 `json:"versionId"`
		} `json:"subjects"`
		Fields []struct {
			Key        string          `json:"key"`
			ValueType  string          `json:"valueType"`
			Values     json.RawMessage `json:"values"`
			Complete   bool            `json:"complete"`
			Comparison string          `json:"comparison"`
			Derived    json.RawMessage `json:"derived"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(first), &doc); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if doc.SchemaVersion != 1 || doc.Kind != "version" || doc.Baseline != 0 {
		t.Fatalf("top-level contract wrong: %+v", doc)
	}
	if len(doc.Subjects) != 2 || doc.Subjects[0].Index != 0 || doc.Subjects[1].VersionID != 2 {
		t.Fatalf("subject contract wrong: %+v", doc.Subjects)
	}
	// Registry order is fixed; values[i] aligns with subjects[i].
	wantFirst := []string{"versionId", "versionName", "modelId", "modelName", "modelURL", "modelType", "creator"}
	for i, key := range wantFirst {
		if doc.Fields[i].Key != key {
			t.Fatalf("field %d = %q, want %q", i, doc.Fields[i].Key, key)
		}
	}
	for _, f := range doc.Fields {
		if len(f.Values) == 0 {
			t.Fatalf("field %s has no values", f.Key)
		}
	}
}

func TestCompareIdentityFields(t *testing.T) {
	if !IsIdentityField(KindModel, "modelId") || !IsIdentityField(KindModel, "url") {
		t.Fatalf("model identity fields wrong")
	}
	if IsIdentityField(KindModel, "name") {
		t.Fatalf("name is not an identity field")
	}
	if !IsIdentityField(KindVersion, "versionId") || !IsIdentityField(KindVersion, "modelId") ||
		!IsIdentityField(KindVersion, "modelURL") {
		t.Fatalf("version identity fields wrong")
	}
	if IsIdentityField(KindVersion, "versionName") {
		t.Fatalf("versionName is not an identity field")
	}
}

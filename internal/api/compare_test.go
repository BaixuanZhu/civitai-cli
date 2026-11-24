package api

import (
	"strings"
	"testing"
)

func TestCompareParseVersion_TrainedWordsPresence(t *testing.T) {
	cases := []struct {
		name string
		body string
		want Presence
	}{
		{"absent", `{"id":2}`, PresenceMissing},
		{"empty", `{"id":2,"trainedWords":[]}`, PresenceValue},
		{"null", `{"id":2,"trainedWords":null}`, PresenceNull},
		{"values", `{"id":2,"trainedWords":["a","b"]}`, PresenceValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := ParseCompareVersion([]byte(tc.body))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if v.TrainedWords.Presence != tc.want {
				t.Fatalf("presence = %q, want %q", v.TrainedWords.Presence, tc.want)
			}
			if tc.want == PresenceValue && v.TrainedWords.Value == nil {
				t.Fatalf("supplied list must not decode as a nil slice")
			}
		})
	}
}

func TestCompareParseModel_LicensePresence(t *testing.T) {
	absent, err := ParseCompareModel([]byte(`{"id":1}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if absent.AllowNoCredit.Presence != PresenceMissing {
		t.Fatalf("absent allowNoCredit presence = %q", absent.AllowNoCredit.Presence)
	}
	falseVal, err := ParseCompareModel([]byte(`{"id":1,"allowNoCredit":false,"allowDifferentLicense":false,"allowDerivatives":false}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for name, o := range map[string]Opt[bool]{
		"allowNoCredit":         falseVal.AllowNoCredit,
		"allowDifferentLicense": falseVal.AllowDifferentLicense,
		"allowDerivatives":      falseVal.AllowDerivatives,
	} {
		if o.Presence != PresenceValue || o.Value {
			t.Fatalf("%s must decode as supplied false, got %+v", name, o)
		}
	}
	nullVal, err := ParseCompareModel([]byte(`{"id":1,"allowNoCredit":null}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if nullVal.AllowNoCredit.Presence != PresenceNull {
		t.Fatalf("null allowNoCredit presence = %q", nullVal.AllowNoCredit.Presence)
	}
}

func TestCompareParseVersion_AvailabilityAndStatus(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantAv  Presence
		wantSta Presence
		av      string
	}{
		{"absent", `{"id":2,"modelId":7}`, PresenceMissing, PresenceMissing, ""},
		{"empty", `{"id":2,"modelId":7,"availability":"","status":""}`, PresenceValue, PresenceValue, ""},
		{"null", `{"id":2,"modelId":7,"availability":null,"status":null}`, PresenceNull, PresenceNull, ""},
		{"value", `{"id":2,"modelId":7,"availability":"Public","status":"Published"}`, PresenceValue, PresenceValue, "Public"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := ParseCompareVersion([]byte(tc.body))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if v.Availability.Presence != tc.wantAv || v.Status.Presence != tc.wantSta {
				t.Fatalf("availability=%q status=%q, want %q/%q",
					v.Availability.Presence, v.Status.Presence, tc.wantAv, tc.wantSta)
			}
			if v.Availability.Presence == PresenceValue && v.Availability.Value != tc.av {
				t.Fatalf("availability value = %q, want %q", v.Availability.Value, tc.av)
			}
			if !v.ModelID.HasValue() || v.ModelID.Value != 7 {
				t.Fatalf("owning model id must decode, got %+v", v.ModelID)
			}
		})
	}
	noOwner, err := ParseCompareVersion([]byte(`{"id":2}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if noOwner.ModelID.Presence != PresenceMissing {
		t.Fatalf("absent modelId presence = %q", noOwner.ModelID.Presence)
	}
}

func TestCompareParseModel_EmbeddedVersions(t *testing.T) {
	m, err := ParseCompareModel([]byte(`{
		"id":7,
		"modelVersions":[
			{"id":10,"availability":"Public","status":"Published","publishedAt":"2026-01-01T00:00:00Z"},
			{"id":11,"availability":null}
		]
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !m.Versions.HasValue() || len(m.Versions.Value) != 2 {
		t.Fatalf("embedded versions must decode, got %+v", m.Versions)
	}
	if !m.Versions.Value[0].Availability.HasValue() || m.Versions.Value[0].Availability.Value != "Public" {
		t.Fatalf("embedded availability must be retained: %+v", m.Versions.Value[0])
	}
	if m.Versions.Value[1].Availability.Presence != PresenceNull {
		t.Fatalf("embedded null availability must stay null: %+v", m.Versions.Value[1])
	}
	absent, err := ParseCompareModel([]byte(`{"id":7}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if absent.Versions.Presence != PresenceMissing {
		t.Fatalf("absent modelVersions presence = %q", absent.Versions.Presence)
	}
	empty, err := ParseCompareModel([]byte(`{"id":7,"modelVersions":[]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if empty.Versions.Presence != PresenceValue || empty.Versions.Value == nil {
		t.Fatalf("supplied empty modelVersions must stay a non-nil empty list: %+v", empty.Versions)
	}
}

func TestCompareParseImages_HasMetaPresence(t *testing.T) {
	v, err := ParseCompareVersion([]byte(`{
		"id":2,"modelId":7,
		"images":[
			{"hasMeta":true,"meta":null},
			{"hasMeta":false},
			{"hasMeta":null,"meta":{}},
			{"meta":{"prompt":"","negativePrompt":"neg","steps":8,"cfgScale":"1.5","sampler":"Euler","scheduler":"simple","seed":"42","width":{"nodes":{}},"height":512}},
			{"hasMeta":true,"meta":{"prompt":"p","width":null}}
		]
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !v.Images.HasValue() || len(v.Images.Value) != 5 {
		t.Fatalf("images must decode, got %+v", v.Images)
	}
	imgs := v.Images.Value
	if !imgs[0].HasMeta.HasValue() || !imgs[0].HasMeta.Value {
		t.Fatalf("hasMeta true must survive: %+v", imgs[0])
	}
	if imgs[0].Meta.Presence != PresenceNull {
		t.Fatalf("hasMeta true with null meta must stay null: %+v", imgs[0])
	}
	if !imgs[1].HasMeta.HasValue() || imgs[1].HasMeta.Value {
		t.Fatalf("hasMeta false must survive: %+v", imgs[1])
	}
	if imgs[1].Meta.Presence != PresenceMissing {
		t.Fatalf("absent meta must stay missing: %+v", imgs[1])
	}
	if imgs[2].HasMeta.Presence != PresenceNull {
		t.Fatalf("hasMeta null must stay null: %+v", imgs[2])
	}
	if !imgs[2].Meta.HasValue() {
		t.Fatalf("supplied empty meta object must stay supplied: %+v", imgs[2])
	}
	meta := imgs[3].Meta
	if !meta.HasValue() {
		t.Fatalf("metadata must decode: %+v", imgs[3])
	}
	if meta.Value.Prompt || !meta.Value.NegativePrompt || !meta.Value.Steps || !meta.Value.CFGScale ||
		!meta.Value.Sampler || !meta.Value.Scheduler || !meta.Value.Seed || !meta.Value.Width || !meta.Value.Height {
		t.Fatalf("non-empty recipe presence wrong: %+v", meta.Value)
	}
	if imgs[4].Meta.Value.Width {
		t.Fatalf("null width must not count as non-empty")
	}

	empty, err := ParseCompareVersion([]byte(`{"id":2,"modelId":7,"images":[]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if empty.Images.Presence != PresenceValue || empty.Images.Value == nil {
		t.Fatalf("supplied empty images must stay a non-nil empty list: %+v", empty.Images)
	}
	absent, err := ParseCompareVersion([]byte(`{"id":2,"modelId":7}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if absent.Images.Presence != PresenceMissing {
		t.Fatalf("absent images presence = %q", absent.Images.Presence)
	}
}

func TestCompareParse_DriftTolerance(t *testing.T) {
	m, err := ParseCompareModel([]byte(`{
		"id":"7",
		"type":"NovelType",
		"nsfwLevel":"7",
		"availability":"",
		"allowCommercialUse":[],
		"stats":{"downloadCount":"66832","thumbsUpCount":1934.0,"commentCount":null}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.ID.Presence != PresenceValue || m.ID.Value != 7 {
		t.Fatalf("numeric-string id must decode: %+v", m.ID)
	}
	if m.Type.Presence != PresenceValue || m.Type.Value != "NovelType" {
		t.Fatalf("unknown enum must be preserved: %+v", m.Type)
	}
	if m.NSFWLevel.Presence != PresenceValue || m.NSFWLevel.Value != int64(7) {
		t.Fatalf("numeric-string nsfwLevel must normalize to int64: %+v", m.NSFWLevel)
	}
	if m.Availability.Presence != PresenceValue || m.Availability.Value != "" {
		t.Fatalf("empty availability must stay supplied empty: %+v", m.Availability)
	}
	if m.AllowCommercialUse.Presence != PresenceValue || m.AllowCommercialUse.Value == nil {
		t.Fatalf("empty commercial-use list must stay supplied empty: %+v", m.AllowCommercialUse)
	}
	if !m.Downloads.HasValue() || m.Downloads.Value != 66832 {
		t.Fatalf("numeric-string stat must decode: %+v", m.Downloads)
	}
	if !m.ThumbsUp.HasValue() || m.ThumbsUp.Value != 1934 {
		t.Fatalf("integral float stat must decode: %+v", m.ThumbsUp)
	}
	if m.Comments.Presence != PresenceNull {
		t.Fatalf("null stat must stay null: %+v", m.Comments)
	}

	v, err := ParseCompareVersion([]byte(`{
		"id":2,"modelId":7,
		"files":[{"id":9,"name":"f","hashes":{"BLAKE3":"beef","NovelHash":"x","SHA256":"abc"},"metadata":null}]
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !v.Files.HasValue() || len(v.Files.Value) != 1 {
		t.Fatalf("files must decode: %+v", v.Files)
	}
	f := v.Files.Value[0]
	if !f.Hashes.HasValue() || len(f.Hashes.Value) != 3 {
		t.Fatalf("unknown hash algorithms must be preserved: %+v", f.Hashes)
	}
	if f.Hashes.Value[0].Algorithm != "BLAKE3" || f.Hashes.Value[1].Algorithm != "NovelHash" ||
		f.Hashes.Value[2].Algorithm != "SHA256" {
		t.Fatalf("hash algorithms must be sorted deterministically: %+v", f.Hashes.Value)
	}
	if f.Format.Presence != PresenceMissing {
		t.Fatalf("null metadata must leave format missing, got %+v", f.Format)
	}
}

func TestCompareParse_MalformedJSON(t *testing.T) {
	if _, err := ParseCompareModel([]byte(`{"id":`)); err == nil {
		t.Fatalf("malformed model body must fail")
	} else if !strings.Contains(err.Error(), "api: decode compare model") {
		t.Fatalf("error must be wrapped with context: %v", err)
	}
	if _, err := ParseCompareVersion([]byte(`[]`)); err == nil {
		t.Fatalf("non-object version body must fail")
	} else if !strings.Contains(err.Error(), "api: decode compare version") {
		t.Fatalf("error must be wrapped with context: %v", err)
	}
}

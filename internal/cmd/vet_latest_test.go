package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"civitai-cli/internal/api"
)

// vetTestVersion builds one embedded visible version with the fields the
// latest evaluator consumes.
func vetTestVersion(id int, name, base, publishedAt string) api.ModelVersion {
	return api.ModelVersion{ID: id, Name: name, BaseModel: base, PublishedAt: publishedAt}
}

// vetTestModel wraps an explicit version slice in an owning-model
// response; a nil slice keeps ModelVersions nil (field absent) while an
// empty non-nil slice means the API supplied [].
func vetTestModel(versions []api.ModelVersion) *api.Model {
	return &api.Model{ModelVersions: versions}
}

// vetTestCurrent builds the by-hash current version; only ID and
// BaseModel feed the latest context.
func vetTestCurrent(id int, base string) *api.ModelVersion {
	return &api.ModelVersion{ID: id, BaseModel: base}
}

// vetStrPtr returns a pointer for nullable JSON string expectations.
func vetStrPtr(s string) *string { return &s }

// vetWantCmp builds an expected comparison block.
func vetWantCmp(status, reason, baseChange string) vetLatestComparison {
	return vetLatestComparison{Status: status, Reason: reason, BaseModelChange: baseChange}
}

// vetWantKnown builds an expected unique-latest block; empty name/base
// become JSON null.
func vetWantKnown(id int, name, publishedAt, base string, cmp vetLatestComparison) vetLatestPublished {
	v := &vetLatestVersion{ID: id, PublishedAt: publishedAt}
	if name != "" {
		v.Name = vetStrPtr(name)
	}
	if base != "" {
		v.BaseModel = vetStrPtr(base)
	}
	return vetLatestPublished{
		Status:         vetLatestStatusKnown,
		Reason:         vetReasonNone,
		Version:        v,
		TiedVersionIDs: []int{},
		Comparison:     cmp,
	}
}

// vetWantTied builds an expected tied-latest block.
func vetWantTied(ids []int, cmp vetLatestComparison) vetLatestPublished {
	return vetLatestPublished{
		Status:         vetLatestStatusTied,
		Reason:         vetReasonLatestTied,
		TiedVersionIDs: ids,
		Comparison:     cmp,
	}
}

// vetWantUnknown builds an expected unknown-latest block: no version, an
// empty tied array, and the latestUnavailable comparison.
func vetWantUnknown(reason string) vetLatestPublished {
	return vetLatestPublished{
		Status:         vetLatestStatusUnknown,
		Reason:         reason,
		TiedVersionIDs: []int{},
		Comparison:     vetWantCmp(vetRelationUnknown, vetReasonLatestUnavailable, vetBaseChangeUnknown),
	}
}

const (
	vetT1 = "2026-01-01T00:00:00Z"
	vetT2 = "2026-02-02T00:00:00Z"
	vetT3 = "2026-03-03T00:00:00Z"
)

func TestVetLatestDerive(t *testing.T) {
	cases := []struct {
		name    string
		current *api.ModelVersion
		model   *api.Model
		want    vetLatestPublished
	}{
		{
			name:    "unique current latest same base",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "Anima",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged)),
		},
		{
			name:    "older current unique later version base changed",
			current: vetTestCurrent(10, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "SDXL", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "SDXL",
				vetWantCmp(vetRelationNewerPublished, vetReasonNone, vetBaseChangeChanged)),
		},
		{
			name:    "older current base unchanged",
			current: vetTestCurrent(10, "SDXL"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "SDXL", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "SDXL",
				vetWantCmp(vetRelationNewerPublished, vetReasonNone, vetBaseChangeUnchanged)),
		},
		{
			name:    "case-only base difference is changed",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "anima", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "anima",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeChanged)),
		},
		{
			name:    "empty current base makes change unknown",
			current: vetTestCurrent(20, ""),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "Anima",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnknown)),
		},
		{
			name:    "empty latest base keeps identity and makes change unknown",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnknown)),
		},
		{
			name:    "nil model",
			current: vetTestCurrent(20, "Anima"),
			model:   nil,
			want:    vetWantUnknown(vetReasonModelUnavailable),
		},
		{
			name:    "nil version list",
			current: vetTestCurrent(20, "Anima"),
			model:   vetTestModel(nil),
			want:    vetWantUnknown(vetReasonVersionsUnavailable),
		},
		{
			name:    "supplied empty version list",
			current: vetTestCurrent(20, "Anima"),
			model:   vetTestModel([]api.ModelVersion{}),
			want:    vetWantUnknown(vetReasonNoVisibleVersions),
		},
		{
			name:    "current absent unique latest base unchanged",
			current: vetTestCurrent(99, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "Anima",
				vetWantCmp(vetRelationUnknown, vetReasonCurrentNotVisible, vetBaseChangeUnchanged)),
		},
		{
			name:    "current absent unique latest base changed",
			current: vetTestCurrent(99, "SDXL"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantKnown(20, "v2", vetT2, "Anima",
				vetWantCmp(vetRelationUnknown, vetReasonCurrentNotVisible, vetBaseChangeChanged)),
		},
		{
			name:    "missing timestamp on lower entry",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", ""),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantUnknown(vetReasonTimestampUnavailable),
		},
		{
			name:    "missing timestamp on maximum candidate",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", ""),
			}),
			want: vetWantUnknown(vetReasonTimestampUnavailable),
		},
		{
			name:    "malformed timestamp on lower entry",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", "not-a-date"),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantUnknown(vetReasonTimestampUnavailable),
		},
		{
			name:    "malformed timestamp on maximum candidate",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", "2026-13-99T99:99"),
			}),
			want: vetWantUnknown(vetReasonTimestampUnavailable),
		},
		{
			name:    "maximum tie including current",
			current: vetTestCurrent(10, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(30, "v3", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
				vetTestVersion(10, "v1", "Anima", vetT2),
			}),
			want: vetWantTied([]int{10, 20},
				vetWantCmp(vetRelationTiedLatest, vetReasonLatestTied, vetBaseChangeUnknown)),
		},
		{
			name:    "maximum tie excluding current",
			current: vetTestCurrent(30, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT2),
				vetTestVersion(20, "v2", "Anima", vetT2),
				vetTestVersion(30, "v3", "Anima", vetT1),
			}),
			want: vetWantTied([]int{10, 20},
				vetWantCmp(vetRelationUnknown, vetReasonLatestTied, vetBaseChangeUnknown)),
		},
		{
			name:    "maximum tie with current absent",
			current: vetTestCurrent(99, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT2),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantTied([]int{10, 20},
				vetWantCmp(vetRelationUnknown, vetReasonCurrentNotVisible, vetBaseChangeUnknown)),
		},
		{
			name:    "offsets compared by instant not text",
			current: vetTestCurrent(10, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				// 2026-02-02T00:00:00+02:00 is 2026-02-01T22:00:00Z, so the
				// lexicographically smaller Z string is the later instant.
				vetTestVersion(10, "v1", "Anima", "2026-02-02T00:00:00+02:00"),
				vetTestVersion(20, "v2", "Anima", "2026-02-01T23:00:00Z"),
			}),
			want: vetWantKnown(20, "v2", "2026-02-01T23:00:00Z", "Anima",
				vetWantCmp(vetRelationNewerPublished, vetReasonNone, vetBaseChangeUnchanged)),
		},
		{
			name:    "equivalent offsets tie at the same instant",
			current: vetTestCurrent(10, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", "2026-02-02T02:00:00+02:00"),
				vetTestVersion(20, "v2", "Anima", "2026-02-02T00:00:00Z"),
			}),
			want: vetWantTied([]int{10, 20},
				vetWantCmp(vetRelationTiedLatest, vetReasonLatestTied, vetBaseChangeUnknown)),
		},
		{
			name:    "fractional seconds order instants",
			current: vetTestCurrent(10, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", "2026-02-02T00:00:00Z"),
				vetTestVersion(20, "v2", "Anima", "2026-02-02T00:00:00.5Z"),
			}),
			want: vetWantKnown(20, "v2", "2026-02-02T00:00:00.5Z", "Anima",
				vetWantCmp(vetRelationNewerPublished, vetReasonNone, vetBaseChangeUnchanged)),
		},
		{
			name:    "zero id",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(0, "v0", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantUnknown(vetReasonIdentityUnavailable),
		},
		{
			name:    "negative id",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(-3, "v1", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
			}),
			want: vetWantUnknown(vetReasonIdentityUnavailable),
		},
		{
			name:    "duplicate id beats tie precedence",
			current: vetTestCurrent(10, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT2),
				vetTestVersion(10, "v1b", "Anima", vetT2),
			}),
			want: vetWantUnknown(vetReasonIdentityUnavailable),
		},
		{
			name:    "empty latest name stays null with other facts known",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(10, "v1", "Anima", vetT1),
				vetTestVersion(20, "", "Anima", vetT2),
			}),
			want: vetWantKnown(20, "", vetT2, "Anima",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged)),
		},
		{
			name:    "response order does not select the latest",
			current: vetTestCurrent(20, "Anima"),
			model: vetTestModel([]api.ModelVersion{
				vetTestVersion(30, "v3", "Anima", vetT1),
				vetTestVersion(20, "v2", "Anima", vetT2),
				vetTestVersion(10, "v1", "Anima", vetT1),
			}),
			want: vetWantKnown(20, "v2", vetT2, "Anima",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged)),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveVetLatestPublished(tc.current, tc.model)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("deriveVetLatestPublished = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestVetLatestDerive_TieDeterministic proves response order never picks a
// winner: reversed tied input must produce byte-identical JSON.
func TestVetLatestDerive_TieDeterministic(t *testing.T) {
	cur := vetTestCurrent(10, "Anima")
	forward := vetTestModel([]api.ModelVersion{
		vetTestVersion(30, "v3", "Anima", vetT1),
		vetTestVersion(20, "v2", "Anima", vetT2),
		vetTestVersion(10, "v1", "Anima", vetT2),
	})
	reversed := vetTestModel([]api.ModelVersion{
		vetTestVersion(10, "v1", "Anima", vetT2),
		vetTestVersion(20, "v2", "Anima", vetT2),
		vetTestVersion(30, "v3", "Anima", vetT1),
	})
	j1, err := json.Marshal(deriveVetLatestPublished(cur, forward))
	if err != nil {
		t.Fatalf("marshal forward: %v", err)
	}
	j2, err := json.Marshal(deriveVetLatestPublished(cur, reversed))
	if err != nil {
		t.Fatalf("marshal reversed: %v", err)
	}
	if !bytes.Equal(j1, j2) {
		t.Errorf("tied result must be order-independent:\n%s\n%s", j1, j2)
	}
}

// TestVetLatestJSONShape pins the additive block's structural contract:
// all keys always emitted, version null for tied/unknown, nullable
// name/base, and tiedVersionIds always an array (never null).
func TestVetLatestJSONShape(t *testing.T) {
	cur := vetTestCurrent(20, "Anima")
	knownEmpty := deriveVetLatestPublished(cur, vetTestModel([]api.ModelVersion{
		vetTestVersion(10, "v1", "Anima", vetT1),
		vetTestVersion(20, "", "", vetT2),
	}))
	b, err := json.Marshal(knownEmpty)
	if err != nil {
		t.Fatalf("marshal known: %v", err)
	}
	for _, want := range []string{
		`"status":"known"`, `"reason":"none"`,
		`"id":20`, `"name":null`, `"publishedAt":"2026-02-02T00:00:00Z"`, `"baseModel":null`,
		`"tiedVersionIds":[]`,
		`"comparison":{"status":"currentLatest","reason":"none","baseModelChange":"unknown"}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("known block missing %s:\n%s", want, b)
		}
	}

	tied := deriveVetLatestPublished(vetTestCurrent(10, "Anima"), vetTestModel([]api.ModelVersion{
		vetTestVersion(10, "v1", "Anima", vetT2),
		vetTestVersion(20, "v2", "Anima", vetT2),
	}))
	b, err = json.Marshal(tied)
	if err != nil {
		t.Fatalf("marshal tied: %v", err)
	}
	for _, want := range []string{
		`"status":"tied"`, `"reason":"latestTied"`, `"version":null`, `"tiedVersionIds":[10,20]`,
		`"comparison":{"status":"tiedLatest","reason":"latestTied","baseModelChange":"unknown"}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("tied block missing %s:\n%s", want, b)
		}
	}

	unknown := deriveVetLatestPublished(cur, nil)
	b, err = json.Marshal(unknown)
	if err != nil {
		t.Fatalf("marshal unknown: %v", err)
	}
	for _, want := range []string{
		`"status":"unknown"`, `"reason":"modelUnavailable"`, `"version":null`, `"tiedVersionIds":[]`,
		`"comparison":{"status":"unknown","reason":"latestUnavailable","baseModelChange":"unknown"}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("unknown block missing %s:\n%s", want, b)
		}
	}
}

// TestVetLatestCompare_OrderIndeterminate exercises the defensive
// residual relationship state directly: valid maximum evaluation can
// never reach it (a different visible current entry cannot postdate the
// unique maximum), so it is constructed by hand here.
func TestVetLatestCompare_OrderIndeterminate(t *testing.T) {
	latest := vetWantKnown(20, "v2", vetT1, "Anima",
		vetWantCmp(vetRelationUnknown, vetReasonLatestUnavailable, vetBaseChangeUnknown))
	instants := map[int]time.Time{
		10: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		20: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	got := vetCompareToLatest(latest, vetTestCurrent(10, "Anima"), instants, instants[20])
	want := vetWantCmp(vetRelationUnknown, vetReasonOrderIndeterminate, vetBaseChangeUnchanged)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("vetCompareToLatest = %+v, want %+v", got, want)
	}
}

func TestVetLatestLineText(t *testing.T) {
	cases := []struct {
		name   string
		latest vetLatestPublished
		want   string
	}{
		{
			name: "current latest unchanged",
			latest: vetWantKnown(3263843, "turbo-v1.1", "2026-08-24T23:42:31.929Z", "Anima",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged)),
			want: "turbo-v1.1 (id 3263843, 2026-08-24)   base: Anima   relation: current latest; base unchanged",
		},
		{
			name: "newer published changed",
			latest: vetWantKnown(20, "v2", vetT2, "SDXL",
				vetWantCmp(vetRelationNewerPublished, vetReasonNone, vetBaseChangeChanged)),
			want: "v2 (id 20, 2026-02-02)   base: SDXL   relation: newer published; base changed",
		},
		{
			name: "current not visible",
			latest: vetWantKnown(20, "v2", vetT2, "Anima",
				vetWantCmp(vetRelationUnknown, vetReasonCurrentNotVisible, vetBaseChangeUnchanged)),
			want: "v2 (id 20, 2026-02-02)   base: Anima   relation: unknown (current version not visible); base unchanged",
		},
		{
			name: "tied including current",
			latest: vetWantTied([]int{10, 20},
				vetWantCmp(vetRelationTiedLatest, vetReasonLatestTied, vetBaseChangeUnknown)),
			want: "unknown (latest tied; version ids 10,20)   relation: tied latest; base change unknown",
		},
		{
			name: "tied excluding current",
			latest: vetWantTied([]int{10, 20},
				vetWantCmp(vetRelationUnknown, vetReasonLatestTied, vetBaseChangeUnknown)),
			want: "unknown (latest tied; version ids 10,20)   relation: unknown (latest tied); base change unknown",
		},
		{
			name:   "model unavailable",
			latest: vetWantUnknown(vetReasonModelUnavailable),
			want:   "unknown (model details unavailable)   relation: unknown; base change unknown",
		},
		{
			name:   "no visible versions",
			latest: vetWantUnknown(vetReasonNoVisibleVersions),
			want:   "unknown (no visible versions)   relation: unknown; base change unknown",
		},
		{
			name: "empty latest name and base",
			latest: vetWantKnown(20, "", vetT2, "",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnknown)),
			want: "unknown (id 20, 2026-02-02)   base: unknown   relation: current latest; base change unknown",
		},
		{
			name: "timestamp order indeterminate",
			latest: vetWantKnown(20, "v2", vetT2, "Anima",
				vetWantCmp(vetRelationUnknown, vetReasonOrderIndeterminate, vetBaseChangeUnchanged)),
			want: "v2 (id 20, 2026-02-02)   base: Anima   relation: unknown (timestamp order indeterminate); base unchanged",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := vetLatestLineText(tc.latest, 80); got != tc.want {
				t.Errorf("vetLatestLineText = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestVetLatestLineText_TruncatesOnlyName keeps IDs, dates, and statuses
// intact when a long display name must give way to the console width.
func TestVetLatestLineText_TruncatesOnlyName(t *testing.T) {
	longName := strings.Repeat("verylongversionname", 6) // 114 runes
	latest := vetWantKnown(20, longName, vetT2, "Anima",
		vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged))
	line := vetLatestLineText(latest, 80)
	if strings.Contains(line, longName) {
		t.Errorf("long name must be truncated: %q", line)
	}
	for _, want := range []string{"…", "(id 20, 2026-02-02)", "base: Anima", "relation: current latest; base unchanged"} {
		if !strings.Contains(line, want) {
			t.Errorf("truncated line missing %q: %q", want, line)
		}
	}
}

// TestVetLatestLineText_NoAdviceVocabulary pins the informational scope:
// the renderer has no outdated/upgrade/recommended/best/rollback/trend
// wording in any state.
func TestVetLatestLineText_NoAdviceVocabulary(t *testing.T) {
	latests := []vetLatestPublished{
		vetWantKnown(20, "v2", vetT2, "SDXL", vetWantCmp(vetRelationNewerPublished, vetReasonNone, vetBaseChangeChanged)),
		vetWantKnown(20, "v2", vetT2, "Anima", vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged)),
		vetWantKnown(20, "v2", vetT2, "Anima", vetWantCmp(vetRelationUnknown, vetReasonCurrentNotVisible, vetBaseChangeUnchanged)),
		vetWantTied([]int{10, 20}, vetWantCmp(vetRelationTiedLatest, vetReasonLatestTied, vetBaseChangeUnknown)),
		vetWantUnknown(vetReasonModelUnavailable),
		vetWantUnknown(vetReasonTimestampUnavailable),
	}
	for _, latest := range latests {
		line := strings.ToLower(vetLatestLineText(latest, 80))
		for _, banned := range []string{"outdated", "upgrade", "recommend", "best", "rollback", "trend"} {
			if strings.Contains(line, banned) {
				t.Errorf("latest line must not carry %q wording: %q", banned, line)
			}
		}
	}
}

package api

import (
	"encoding/json"
	"slices"
	"testing"

	"civitai-cli/internal/testutil"
)

func TestMapSort(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"hot", "Hot", true},
		{"MOST-DOWNLOADED", "Most Downloaded", true},
		{"Highest-Rated", "Highest Rated", true},
		{"recently-added", "Recently Added", true},
		{"bogus", "", false},
	}
	for _, tc := range cases {
		got, ok := MapSort(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("MapSort(%q) = %q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMapPeriod(t *testing.T) {
	if got, ok := MapPeriod("ALL"); !ok || got != "AllTime" {
		t.Errorf("MapPeriod(all) = %q,%v", got, ok)
	}
	if _, ok := MapPeriod("decade"); ok {
		t.Errorf("unknown period must not map")
	}
}

func TestMapType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"lora", "LORA"},
		{"LORA", "LORA"},
		{"ti", "TextualInversion"},
		{"hypernet", "Hypernetwork"},
		{"checkpoint", "Checkpoint"},
		{"locon", "LoCon"},
		{"dora", "DoRA"},
		{"vae", "VAE"},
	}
	for _, tc := range cases {
		got, ok := MapType(tc.in)
		if !ok || got != tc.want {
			t.Errorf("MapType(%q) = %q,%v want %q", tc.in, got, ok, tc.want)
		}
	}
	if _, ok := MapType("gan"); ok {
		t.Errorf("unknown type must not map")
	}
}

func TestCanonicalBaseModel(t *testing.T) {
	if got, ok := CanonicalBaseModel("ILLUSTRIOUS"); !ok || got != "Illustrious" {
		t.Errorf("CanonicalBaseModel(ILLUSTRIOUS) = %q,%v", got, ok)
	}
	if got, ok := CanonicalBaseModel("sdxl 1.0"); !ok || got != "SDXL 1.0" {
		t.Errorf("CanonicalBaseModel(sdxl 1.0) = %q,%v", got, ok)
	}
	if _, ok := CanonicalBaseModel("Anima 2"); ok {
		t.Errorf("unregistered value must report ok=false (warn, not block)")
	}
}

// TestBaseModelWhitelist_PinnedToLiveEvidence keeps the constant table in
// lockstep with the 2026-11-22 probe evidence. Re-fetch the evidence and
// update the table deliberately when the platform registers new bases.
func TestBaseModelWhitelist_PinnedToLiveEvidence(t *testing.T) {
	var evidence struct {
		Values []string `json:"values"`
	}
	if err := json.Unmarshal(testutil.LoadFixture(t, "probes/base_model_verified.json"), &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	got := BaseModelChoices()
	if !slices.Equal(got, evidence.Values) {
		t.Errorf("whitelist drifted from probe evidence:\n got %v\nwant %v", got, evidence.Values)
	}
}

// TestSortTable_PinnedToLiveZodLeak pins the sort mapping against the raw
// ZodError values array captured live.
func TestSortTable_PinnedToLiveZodLeak(t *testing.T) {
	var probe struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(testutil.LoadFixture(t, "probes/enum_sort.json"), &probe); err != nil {
		t.Fatalf("decode probe: %v", err)
	}
	var issues []struct {
		Values []string `json:"values"`
	}
	if err := json.Unmarshal([]byte(probe.Error.Message), &issues); err != nil || len(issues) == 0 {
		t.Fatalf("decode zod issues: %v", err)
	}
	var mapped []string
	for _, k := range SortChoices() {
		v, _ := MapSort(k)
		mapped = append(mapped, v)
	}
	slices.Sort(mapped)
	want := slices.Clone(issues[0].Values)
	slices.Sort(want)
	if !slices.Equal(mapped, want) {
		t.Errorf("sort table drifted from Zod leak:\n got %v\nwant %v", mapped, want)
	}
}

func TestTypeTable_CoversLiveZodLeak(t *testing.T) {
	var probe struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(testutil.LoadFixture(t, "probes/enum_types.json"), &probe); err != nil {
		t.Fatalf("decode probe: %v", err)
	}
	var issues []zodIssue
	if err := json.Unmarshal([]byte(probe.Error.Message), &issues); err != nil || len(issues) == 0 {
		t.Fatalf("decode zod issues: %v", err)
	}
	// invalid_union: the enum leak is nested in errors[0][0].values.
	leak := issues[0].Errors[0][0].Values
	var mapped []string
	for _, k := range TypeChoices() {
		v, _ := MapType(k)
		mapped = append(mapped, v)
	}
	for _, v := range leak {
		if !slices.Contains(mapped, v.(string)) {
			t.Errorf("type table missing live enum value %v", v)
		}
	}
}

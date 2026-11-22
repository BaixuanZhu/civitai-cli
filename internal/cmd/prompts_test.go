package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/testutil"
)

func TestRenderPrompts_TextFixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "version_3263843.json")
	v, err := api.ParseVersion(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	var buf bytes.Buffer
	if err := renderPrompts(&buf, v.Images, false); err != nil {
		t.Fatalf("renderPrompts: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"PROMPTS (9)",
		"[1]", "[9]",
		"prompt: masterpiece, best quality",
		"negativePrompt: worst quality, low quality",
		"steps: 8",
		"cfgScale: 1",
		"sampler: Euler",
		"scheduler: simple",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("fixture recipe block missing %q:\n%s", want, out)
		}
	}
	// All nine fixture images carry meta, so nothing is skipped.
	if strings.Contains(out, "skipped") {
		t.Errorf("fixture has meta on every image; must not report skips:\n%s", out)
	}
	// Plain-text recipe values carry no surrounding quotes or bullets.
	if strings.Contains(out, `"masterpiece`) || strings.Contains(out, "- prompt:") {
		t.Errorf("recipe values must be unquoted, unbulleted:\n%s", out)
	}
}

func TestRenderPrompts_NullMetaSkippedAndMultiline(t *testing.T) {
	images := []api.Image{
		{URL: "u1", Meta: &api.ImageMeta{
			Prompt:         "line1\nline2\nline3",
			NegativePrompt: "neg",
			Steps:          json.Number("20"),
			CFGScale:       json.Number("7.5"),
			Sampler:        "Euler a",
			Scheduler:      "karras",
		}},
		{URL: "u2", Meta: nil}, // skipped
		{URL: "u3", Meta: &api.ImageMeta{Prompt: "only prompt"}},
	}
	var buf bytes.Buffer
	if err := renderPrompts(&buf, images, false); err != nil {
		t.Fatalf("renderPrompts: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"PROMPTS (2)", // two recipes, one skipped
		"[1]", "[2]",
		"prompt: line1\nline2\nline3\n", // multi-line preserved verbatim
		"negativePrompt: neg",
		"steps: 20",
		"cfgScale: 7.5",
		"sampler: Euler a",
		"scheduler: karras",
		"prompt: only prompt",
		"skipped 1 images without meta",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("recipe block missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[3]") {
		t.Errorf("null-meta image must not consume an index:\n%s", out)
	}
	// The second recipe has only a prompt: absent fields are skipped, so
	// "steps:" appears exactly once (from the first recipe).
	if n := strings.Count(out, "steps:"); n != 1 {
		t.Errorf("absent fields must be skipped; steps: count = %d, want 1:\n%s", n, out)
	}
}

func TestRenderPrompts_JSON(t *testing.T) {
	images := []api.Image{
		{URL: "u1", Meta: &api.ImageMeta{
			Prompt:   "p1",
			Steps:    json.Number("20"),
			CFGScale: json.Number("7.5"),
			Sampler:  "Euler",
		}},
		{URL: "u2", Meta: nil},
	}
	var buf bytes.Buffer
	if err := renderPrompts(&buf, images, true); err != nil {
		t.Fatalf("renderPrompts json: %v", err)
	}
	var r promptsReport
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatalf("json must parse: %v\n%s", err, buf.String())
	}
	if r.Skipped != 1 || len(r.Prompts) != 1 {
		t.Fatalf("json shape = %+v, want 1 recipe + 1 skipped", r)
	}
	p := r.Prompts[0]
	if p.Index != 1 || p.URL != "u1" || p.Prompt != "p1" || p.Steps != json.Number("20") || p.CFGScale != json.Number("7.5") || p.Sampler != "Euler" {
		t.Errorf("json recipe mismatch: %+v", p)
	}
	// Absent fields are omitted; numeric fields marshal as numbers.
	if strings.Contains(buf.String(), `"negativePrompt"`) || strings.Contains(buf.String(), `"scheduler"`) {
		t.Errorf("absent fields must be omitted:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"steps": 20`) || !strings.Contains(buf.String(), `"cfgScale": 7.5`) {
		t.Errorf("steps/cfgScale must marshal as numbers:\n%s", buf.String())
	}
}

func TestRenderPrompts_EmptyImages(t *testing.T) {
	var buf bytes.Buffer
	if err := renderPrompts(&buf, nil, true); err != nil {
		t.Fatalf("renderPrompts: %v", err)
	}
	var r promptsReport
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatalf("json must parse: %v", err)
	}
	if r.Skipped != 0 || len(r.Prompts) != 0 {
		t.Errorf("empty images = %+v", r)
	}
	// The array must serialize as [] (not null) for stable scripting.
	if !strings.Contains(buf.String(), `"prompts": []`) {
		t.Errorf("empty prompts must be []:\n%s", buf.String())
	}
}

func TestShow_PromptsAppendsRecipes(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/3263843": "version_3263843.json",
	}))
	if err := runShowCmd(t, env, "2458426", "-v", "3263843", "--prompts"); err != nil {
		t.Fatalf("show -v --prompts: %v", err)
	}
	out := env.buf.String()
	// The version detail is still rendered, then the recipe block appended.
	for _, want := range []string{"turbo-v1.1", "TRIGGER WORDS", "PROMPTS (9)", "prompt: masterpiece", "sampler: Euler"} {
		if !strings.Contains(out, want) {
			t.Errorf("show --prompts missing %q:\n%s", want, out)
		}
	}
}

func TestShow_PromptsJSON(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/3263843": "version_3263843.json",
	}))
	if err := runShowCmd(t, env, "2458426", "-v", "3263843", "--prompts", "--json"); err != nil {
		t.Fatalf("show -v --prompts --json: %v", err)
	}
	var r promptsReport
	if err := json.Unmarshal(env.buf.Bytes(), &r); err != nil {
		t.Fatalf("json must parse: %v\n%s", err, env.buf.String())
	}
	if len(r.Prompts) != 9 || r.Skipped != 0 {
		t.Errorf("show --prompts --json = %d recipes, %d skipped; want 9/0", len(r.Prompts), r.Skipped)
	}
}

func TestShow_PromptsRequiresVersion(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models/2458426": "models_2458426.json",
	}))
	err := runShowCmd(t, env, "2458426", "--prompts")
	if err == nil || !strings.Contains(err.Error(), "requires -v") {
		t.Fatalf("--prompts without -v must be a usage error, got %v", err)
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("--prompts without -v must map to exit 1, got %d", exitCodeFor(err, false))
	}
}

func TestVet_PromptsAppendsRecipes(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, vetRoutes()))
	if err := runVetCmd(t, env, animaSHA256, "--prompts"); err != nil {
		t.Fatalf("vet --prompts: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{"EXAMPLES  9 (mature 0)", "PROMPTS (9)", "prompt: masterpiece", "scheduler: simple"} {
		if !strings.Contains(out, want) {
			t.Errorf("vet --prompts missing %q:\n%s", want, out)
		}
	}
}

func TestVet_PromptsJSON(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, vetRoutes()))
	if err := runVetCmd(t, env, animaSHA256, "--prompts", "--json"); err != nil {
		t.Fatalf("vet --prompts --json: %v", err)
	}
	var r promptsReport
	if err := json.Unmarshal(env.buf.Bytes(), &r); err != nil {
		t.Fatalf("json must parse: %v\n%s", err, env.buf.String())
	}
	if len(r.Prompts) != 9 || r.Skipped != 0 {
		t.Errorf("vet --prompts --json = %d recipes, %d skipped; want 9/0", len(r.Prompts), r.Skipped)
	}
}

package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"civitai-cli/internal/api"
)

func TestShow_ModelHeaderAndVersionTable(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models/2458426": "models_2458426.json",
	}))
	if err := runShowCmd(t, env, "2458426"); err != nil {
		t.Fatalf("runShow: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{
		"Anima", "circlestone_labs", "Checkpoint", "LICENSE",
		"credit: yes", "derivatives: yes", "relicensing: no",
		"VERSIONS (8)", "3263843", "turbo-v1.1",
		"https://civitai.com/models/2458426",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q:\n%s", want, out)
		}
	}
}

func TestShow_JSONModel(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/models/2458426": "models_2458426.json",
	}))
	if err := runShowCmd(t, env, "2458426", "--json"); err != nil {
		t.Fatalf("runShow --json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(env.buf.Bytes(), &m); err != nil {
		t.Fatalf("--json must parse: %v", err)
	}
	if m["id"] != float64(2458426) {
		t.Errorf("json id = %v", m["id"])
	}
}

func TestShow_VersionDrill(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/3263843": "version_3263843.json",
	}))
	if err := runShowCmd(t, env, "2458426", "-v", "3263843"); err != nil {
		t.Fatalf("runShow -v: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{
		"turbo-v1.1", "base: Anima", "FILES", "SafeTensor",
		"FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB",
		"AutoV2", "TRIGGER WORDS (0)", "Success",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("version detail missing %q:\n%s", want, out)
		}
	}
	if env.lastPath != "/model-versions/3263843" {
		t.Errorf("drill must hit the version endpoint, got %q", env.lastPath)
	}
}

func TestShow_VerAlias(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/3263843": "version_3263843.json",
	}))
	if err := runShowCmd(t, env, "2458426", "--ver", "3263843"); err != nil {
		t.Fatalf("--ver alias: %v", err)
	}
	if !strings.Contains(env.buf.String(), "turbo-v1.1") {
		t.Errorf("--ver must behave like -v")
	}
}

func TestShow_JSONVersion(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/3263843": "version_3263843.json",
	}))
	if err := runShowCmd(t, env, "2458426", "-v", "3263843", "--json"); err != nil {
		t.Fatalf("runShow -v --json: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(env.buf.Bytes(), &v); err != nil {
		t.Fatalf("--json must parse: %v", err)
	}
	if v["id"] != float64(3263843) {
		t.Errorf("json version id = %v", v["id"])
	}
}

func TestShow_TriggerWordsPlainLines(t *testing.T) {
	body := `{"id":11,"modelId":22,"name":"v1","baseModel":"SD 1.5","trainedWords":["foo bar","baz"],"files":[],"images":[]}`
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
	if err := runShowCmd(t, env, "22", "-v", "11"); err != nil {
		t.Fatalf("runShow: %v", err)
	}
	out := env.buf.String()
	if !strings.Contains(out, "TRIGGER WORDS (2)\nfoo bar\nbaz\n") {
		t.Errorf("trigger words must be plain copyable lines:\n%s", out)
	}
	if strings.Contains(out, `"foo bar"`) || strings.Contains(out, "- foo bar") {
		t.Errorf("trigger words must carry no quotes or bullets")
	}
}

func TestShow_VersionModelMismatch(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/3263843": "version_3263843.json",
	}))
	err := runShowCmd(t, env, "999", "-v", "3263843")
	if err == nil || !strings.Contains(err.Error(), "belongs to model") {
		t.Fatalf("mismatched model id must error, got: %v", err)
	}
}

func TestShow_NotFoundMapsToUsageExit(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil)) // every path 404s
	err := runShowCmd(t, env, "999999999")
	var nfe *api.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
	if exitCodeFor(err, false) != 1 {
		t.Errorf("M1 show maps 404 to exit 1 (3 is reserved for vet), got %d", exitCodeFor(err, false))
	}
}

func TestShow_InvalidID(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	for _, arg := range []string{"abc", "0"} {
		if err := runShowCmd(t, env, arg); err == nil {
			t.Errorf("id %q must be rejected", arg)
		}
	}
	if err := runShowCmd(t, env); err == nil {
		t.Errorf("missing id must be rejected")
	}
}

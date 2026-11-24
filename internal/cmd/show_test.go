package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/testutil"
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
	out := env.buf.String()
	var envl struct {
		SchemaVersion string         `json:"schemaVersion"`
		Kind          string         `json:"kind"`
		Model         map[string]any `json:"model"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &envl); err != nil {
		t.Fatalf("--json must parse: %v\n%s", err, out)
	}
	if envl.SchemaVersion != "civ.show/v1" || envl.Kind != "modelSummary" {
		t.Errorf("envelope = %q/%q", envl.SchemaVersion, envl.Kind)
	}
	if envl.Model["id"] != float64(2458426) || envl.Model["name"] != "Anima" {
		t.Errorf("model = %v/%v", envl.Model["id"], envl.Model["name"])
	}
	versions, ok := envl.Model["versions"].([]any)
	if !ok || len(versions) != 8 {
		t.Errorf("versions = %v, want all 8 summaries", envl.Model["versions"])
	}
	for _, excluded := range []string{`"description"`, `"images"`, `"files"`, `"downloadUrl"`} {
		if strings.Contains(out, excluded) {
			t.Errorf("compact model show leaks %s", excluded)
		}
	}
}

func TestShow_JSONViewFullModel(t *testing.T) {
	fixture := testutil.LoadFixture(t, "models_2458426.json")
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	if err := runShowCmd(t, env, "2458426", "--json", "--view", "full"); err != nil {
		t.Fatalf("runShow --json --view full: %v", err)
	}
	m, err := api.ParseModel(fixture)
	if err != nil {
		t.Fatalf("legacy parse: %v", err)
	}
	want, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy model: %v", err)
	}
	if env.buf.String() != string(want)+"\n" {
		t.Errorf("--view full must reproduce the legacy decoded model exactly")
	}
	// The legacy shape is a plain object (no compact envelope).
	var legacy map[string]any
	if err := json.Unmarshal(env.buf.Bytes(), &legacy); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if legacy["id"] != float64(2458426) || legacy["description"] == nil {
		t.Errorf("legacy model fields missing: id=%v description=%v", legacy["id"], legacy["description"] != nil)
	}
	if _, ok := legacy["schemaVersion"]; ok {
		t.Errorf("full view must not carry the compact envelope")
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
	out := env.buf.String()
	var envl struct {
		SchemaVersion string         `json:"schemaVersion"`
		Kind          string         `json:"kind"`
		Version       map[string]any `json:"version"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &envl); err != nil {
		t.Fatalf("--json must parse: %v\n%s", err, out)
	}
	if envl.SchemaVersion != "civ.show/v1" || envl.Kind != "versionSummary" {
		t.Errorf("envelope = %q/%q", envl.SchemaVersion, envl.Kind)
	}
	if envl.Version["id"] != float64(3263843) || envl.Version["modelId"] != float64(2458426) {
		t.Errorf("version ids = %v/%v", envl.Version["id"], envl.Version["modelId"])
	}
	files, ok := envl.Version["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("files = %v, want the one visible file", envl.Version["files"])
	}
	if examples, ok := envl.Version["examples"].(map[string]any); !ok || examples["total"] != float64(9) {
		t.Errorf("examples = %v", envl.Version["examples"])
	}
	for _, excluded := range []string{`"images"`, `"comfy"`, `"downloadUrl"`, `"prompt"`, `"description"`} {
		if strings.Contains(out, excluded) {
			t.Errorf("compact version show leaks %s", excluded)
		}
	}
}

func TestShow_JSONViewFullVersion(t *testing.T) {
	fixture := testutil.LoadFixture(t, "version_3263843.json")
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	if err := runShowCmd(t, env, "2458426", "-v", "3263843", "--json", "--view", "full"); err != nil {
		t.Fatalf("runShow -v --json --view full: %v", err)
	}
	v, err := api.ParseVersion(fixture)
	if err != nil {
		t.Fatalf("legacy parse: %v", err)
	}
	want, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy version: %v", err)
	}
	if env.buf.String() != string(want)+"\n" {
		t.Errorf("--view full must reproduce the legacy decoded version exactly")
	}
	var legacy map[string]any
	if err := json.Unmarshal(env.buf.Bytes(), &legacy); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if legacy["id"] != float64(3263843) || legacy["images"] == nil {
		t.Errorf("legacy version fields missing (id=%v, images present=%v)", legacy["id"], legacy["images"] != nil)
	}
	if _, ok := legacy["schemaVersion"]; ok {
		t.Errorf("full view must not carry the compact envelope")
	}
}

func TestShow_JSONExplicitCompactMatchesDefault(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"model", []string{"2458426", "--json"}},
		{"version", []string{"2458426", "-v", "3263843", "--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, fixtureHandler(t, map[string]string{
				"/models/2458426":         "models_2458426.json",
				"/model-versions/3263843": "version_3263843.json",
			}))
			if err := runShowCmd(t, env, tc.args...); err != nil {
				t.Fatalf("default compact: %v", err)
			}
			defaultOut := env.buf.String()
			env.buf.Reset()
			explicit := append(append([]string{}, tc.args...), "--view", "compact")
			if err := runShowCmd(t, env, explicit...); err != nil {
				t.Fatalf("explicit compact: %v", err)
			}
			if env.buf.String() != defaultOut {
				t.Errorf("explicit --view compact must equal the default output")
			}
		})
	}
}

func TestShow_ViewValidation(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"view requires json", []string{"2458426", "--view", "compact"}, "--view requires --json"},
		{"view conflicts with prompts", []string{"2458426", "-v", "3263843", "--json", "--prompts", "--view", "compact"}, "--view cannot be combined with --prompts"},
		{"unknown view", []string{"2458426", "--json", "--view", "bogus"}, "valid values: compact, full"},
		{"uppercase view", []string{"2458426", "--json", "--view", "COMPACT"}, `unknown --view "COMPACT"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, fixtureHandler(t, map[string]string{
				"/models/2458426":         "models_2458426.json",
				"/model-versions/3263843": "version_3263843.json",
			}))
			err := runShowCmd(t, env, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
			if exitCodeFor(err, false) != 1 {
				t.Errorf("exit code = %d, want 1", exitCodeFor(err, false))
			}
			if env.lastPath != "" {
				t.Errorf("usage error must not send a request, hit %q", env.lastPath)
			}
			if env.buf.Len() != 0 {
				t.Errorf("usage error must keep stdout empty, got %q", env.buf.String())
			}
		})
	}
}

func TestShow_ViewRequestParity(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantPath string
	}{
		{"model", []string{"2458426"}, "/models/2458426"},
		{"version", []string{"2458426", "-v", "3263843"}, "/model-versions/3263843"},
	}
	for _, tc := range cases {
		for _, view := range []string{"compact", "full"} {
			t.Run(tc.name+"/"+view, func(t *testing.T) {
				hits := 0
				env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
					hits++
					switch r.URL.Path {
					case "/models/2458426":
						w.Write(testutil.LoadFixture(t, "models_2458426.json"))
					case "/model-versions/3263843":
						w.Write(testutil.LoadFixture(t, "version_3263843.json"))
					default:
						w.WriteHeader(http.StatusNotFound)
						w.Write([]byte(`{"error":"Model not found"}`))
					}
				})
				args := append(append([]string{}, tc.args...), "--json", "--view", view)
				if err := runShowCmd(t, env, args...); err != nil {
					t.Fatalf("runShow --view %s: %v", view, err)
				}
				if hits != 1 {
					t.Errorf("requests = %d, want exactly 1", hits)
				}
				if env.lastPath != tc.wantPath {
					t.Errorf("path = %q, want %q", env.lastPath, tc.wantPath)
				}
			})
		}
	}
}

func TestShow_JSONCompactDecodeFailureNoFallback(t *testing.T) {
	// The legacy decoder tolerates a null root (zero-value model), so the
	// full view succeeds on this body; the compact contract rejects a
	// non-object root and must not fall back to full output.
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`null`))
	})
	if err := runShowCmd(t, env, "2458426", "--json", "--view", "full"); err != nil {
		t.Fatalf("the legacy full path must tolerate the body: %v", err)
	}
	if env.buf.Len() == 0 {
		t.Fatalf("legacy full path produced no output")
	}
	env.buf.Reset()
	if err := runShowCmd(t, env, "2458426", "--json"); err == nil {
		t.Fatalf("a non-object root must be a compact decode error")
	}
	if env.buf.Len() != 0 {
		t.Errorf("decode failure must not fall back to full output, got %q", env.buf.String())
	}
}

func TestShow_JSONCompactNotFound(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	err := runShowCmd(t, env, "999999999", "--json")
	var nfe *api.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
	if env.buf.Len() != 0 {
		t.Errorf("failed compact request must keep stdout empty, got %q", env.buf.String())
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

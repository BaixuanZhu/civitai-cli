package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"civitai-cli/internal/api"
)

// animaSHA256 is the pinned live SHA256 of Anima turbo-v1.1 (appendix D
// of the plan; testdata/byhash_anima_turbo.json).
const animaSHA256 = "FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB"

// vetRoutes maps the two endpoints a full vet hit consumes: by-hash for
// the version and /models/{id} for creator/license context.
func vetRoutes() map[string]string {
	return map[string]string{
		"/model-versions/by-hash/" + animaSHA256: "byhash_anima_turbo.json",
		"/models/2458426":                        "models_2458426.json",
	}
}

func runVetCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	var o vetOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runVet(context.Background(), &env.g, pos, &o)
}

func TestClassifyInput(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		kind vetInputKind
		hash string
	}{
		{"sha256 uppercase", animaSHA256, vetInputSHA256, animaSHA256},
		{"sha256 lowercase", strings.ToLower(animaSHA256), vetInputSHA256, animaSHA256},
		{"sha256 mixed case", "f" + animaSHA256[1:], vetInputSHA256, animaSHA256},
		{"autov2 uppercase", "FBA1195327", vetInputAutoV2, "FBA1195327"},
		{"autov2 lowercase", "fba1195327", vetInputAutoV2, "FBA1195327"},
		{"8 hex falls to path", "DEADBEEF", vetInputPath, "DEADBEEF"},
		{"63 hex falls to path", strings.Repeat("a", 63), vetInputPath, strings.Repeat("a", 63)},
		{"65 hex falls to path", strings.Repeat("a", 65), vetInputPath, strings.Repeat("a", 65)},
		{"10 non-hex chars", "zzzzzzzzzz", vetInputPath, "zzzzzzzzzz"},
		{"64 chars with non-hex g", strings.Repeat("g", 64), vetInputPath, strings.Repeat("g", 64)},
		{"filename", "foo.safetensors", vetInputPath, "foo.safetensors"},
		{"windows path", `I:\models\foo.safetensors`, vetInputPath, `I:\models\foo.safetensors`},
		{"empty argument", "", vetInputPath, ""},
	}
	for _, tc := range cases {
		kind, hash := classifyInput(tc.arg)
		if kind != tc.kind || hash != tc.hash {
			t.Errorf("%s: classifyInput(%q) = (%d, %q), want (%d, %q)", tc.name, tc.arg, kind, hash, tc.kind, tc.hash)
		}
	}
}

func TestVetExitCode(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		hasToken bool
		want     int
	}{
		{"success", nil, false, 0},
		{"not found", &api.NotFoundError{}, false, 3},
		{"not found with token", &api.NotFoundError{}, true, 3},
		{"wrapped not found", fmt.Errorf("vet: %w", &api.NotFoundError{}), false, 3},
		{"network", &api.NetworkError{Attempts: 4, Err: errors.New("x")}, false, 2},
		{"unauthorized with token", &api.UnauthorizedError{}, true, 4},
		{"unauthorized anonymous", &api.UnauthorizedError{}, false, 1},
		{"api rejection", &api.ApiError{Status: 400}, false, 1},
		{"generic", errors.New("x"), false, 1},
	}
	for _, tc := range cases {
		if got := vetExitCode(tc.err, tc.hasToken); got != tc.want {
			t.Errorf("%s: vetExitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns
// what was written. Hint lines are far below the pipe buffer size, so
// fn never blocks. Tests in this package run sequentially.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = old
	w.Close()
	b, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(b)
}

func TestVetErrorHint(t *testing.T) {
	out := captureStderr(t, func() { vetErrorHint(&api.NotFoundError{Detail: "Model not found"}, false) })
	if !strings.Contains(out, "not registered on Civitai") || !strings.Contains(out, "modified") {
		t.Errorf("404 hint must explain non-Civitai origin / modified file, got: %s", out)
	}
	if strings.Contains(out, "check the id") {
		t.Errorf("vet must not print the generic id hint, got: %s", out)
	}
	// Non-404 classes delegate to the shared hints unchanged.
	out = captureStderr(t, func() { vetErrorHint(&api.UnauthorizedError{}, true) })
	if !strings.Contains(out, "rejected the token") {
		t.Errorf("401 hint must delegate to the shared guidance, got: %s", out)
	}
}

func TestVet_ReportFixture(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, vetRoutes()))
	if err := runVetCmd(t, env, animaSHA256); err != nil {
		t.Fatalf("runVet: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{
		"MODEL     Anima   https://civitai.com/models/2458426",
		"VERSION   turbo-v1.1 (2026-08-24)   base: Anima",
		"CREATOR   circlestone_labs   DL 66.8k   ratio 1934:1   comments 852",
		"TRIGGER WORDS (0)",
		"FILES (1)",
		"anima-turbo-v1.1.safetensors",
		"3.9 GB",
		"SafeTensor bf16",
		"primary",
		"sha256   " + animaSHA256,
		"autov2   FBA1195327",
		"pickle: Success",
		"virus: Success",
		"LICENSE",
		"credit: yes",
		"commercial: Image,RentCivit",
		"derivatives: yes",
		"relicensing: no",
		"EXAMPLES  9 (mature 0)",
		"--prompts for example recipes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("vet report missing %q:\n%s", want, out)
		}
	}
	// The owning-model fetch is the second (last) request of a full hit.
	if env.lastPath != "/models/2458426" {
		t.Errorf("vet must fetch the owning model for creator/license, last path = %q", env.lastPath)
	}
}

func TestVet_AutoV2(t *testing.T) {
	for _, arg := range []string{"FBA1195327", "fba1195327"} {
		env := newTestEnv(t, fixtureHandler(t, map[string]string{
			"/model-versions/by-hash/FBA1195327": "byhash_anima_turbo.json",
			"/models/2458426":                    "models_2458426.json",
		}))
		if err := runVetCmd(t, env, arg); err != nil {
			t.Fatalf("AutoV2 %q: %v", arg, err)
		}
		if !strings.Contains(env.buf.String(), "turbo-v1.1") {
			t.Errorf("AutoV2 %q must resolve the same version:\n%s", arg, env.buf.String())
		}
	}
}

func TestVet_LocalFile(t *testing.T) {
	// The route key is the independently computed digest: if the
	// streaming hash disagreed with crypto/sha256, by-hash would 404.
	content := []byte("civ vet local file test\n")
	sum := sha256.Sum256(content)
	hash := strings.ToUpper(hex.EncodeToString(sum[:]))
	path := filepath.Join(t.TempDir(), "local.safetensors")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/by-hash/" + hash: "byhash_anima_turbo.json",
		"/models/2458426":                 "models_2458426.json",
	}))
	if err := runVetCmd(t, env, path); err != nil {
		t.Fatalf("vet local file: %v", err)
	}
	if !strings.Contains(env.buf.String(), "turbo-v1.1") {
		t.Errorf("local file must vet through its computed hash:\n%s", env.buf.String())
	}
}

func TestVet_NotFoundMapsToExit3(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil)) // every path 404s
	err := runVetCmd(t, env, animaSHA256)
	var nfe *api.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
	if got := vetExitCode(err, false); got != 3 {
		t.Errorf("vet 404 must map to exit 3, got %d", got)
	}
	// The global mapping stays untouched for search/show/tags.
	if got := exitCodeFor(err, false); got != 1 {
		t.Errorf("global exitCodeFor must keep 404 at 1, got %d", got)
	}
}

func TestVet_JSON(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, vetRoutes()))
	if err := runVetCmd(t, env, animaSHA256, "--json"); err != nil {
		t.Fatalf("runVet --json: %v", err)
	}
	var r struct {
		Hash    string `json:"hash"`
		Version struct {
			ID    int `json:"id"`
			Files []struct {
				Hashes map[string]string `json:"hashes"`
			} `json:"files"`
		} `json:"version"`
		Model *struct {
			ID      int    `json:"id"`
			Creator string `json:"creator"`
			License struct {
				AllowNoCredit      bool     `json:"allowNoCredit"`
				AllowCommercialUse []string `json:"allowCommercialUse"`
			} `json:"license"`
		} `json:"model"`
		Examples struct {
			Total    int `json:"total"`
			Mature   int `json:"mature"`
			WithMeta int `json:"withMeta"`
		} `json:"examples"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &r); err != nil {
		t.Fatalf("--json must parse: %v\n%s", err, env.buf.String())
	}
	if r.Hash != animaSHA256 {
		t.Errorf("json hash = %q", r.Hash)
	}
	if r.Version.ID != 3263843 || len(r.Version.Files) != 1 || r.Version.Files[0].Hashes["SHA256"] != animaSHA256 {
		t.Errorf("json version/files mismatch: %+v", r.Version)
	}
	if r.Model == nil || r.Model.ID != 2458426 || r.Model.Creator != "circlestone_labs" {
		t.Fatalf("json model summary mismatch: %+v", r.Model)
	}
	if !r.Model.License.AllowNoCredit || strings.Join(r.Model.License.AllowCommercialUse, ",") != "Image,RentCivit" {
		t.Errorf("json license mismatch: %+v", r.Model.License)
	}
	if r.Examples.Total != 9 || r.Examples.Mature != 0 || r.Examples.WithMeta != 9 {
		t.Errorf("json examples mismatch: %+v", r.Examples)
	}
}

func TestVet_ModelFetchDegrades(t *testing.T) {
	// by-hash hits but the model endpoint 404s: the report must still
	// render (exit 0) with placeholders and a stderr warning — a hit
	// must never turn into exit 3 because of the context fetch.
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/by-hash/" + animaSHA256: "byhash_anima_turbo.json",
	}))
	if err := runVetCmd(t, env, animaSHA256); err != nil {
		t.Fatalf("degraded model fetch must not fail vet: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{"CREATOR   -", "LICENSE   unknown", "DL 66.8k", "turbo-v1.1"} {
		if !strings.Contains(out, want) {
			t.Errorf("degraded report missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(env.logs.String(), "unavailable") {
		t.Errorf("degradation must warn on stderr, logs: %s", env.logs.String())
	}
}

func TestVet_JSONModelNullWhenDegraded(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/by-hash/" + animaSHA256: "byhash_anima_turbo.json",
	}))
	if err := runVetCmd(t, env, animaSHA256, "--json"); err != nil {
		t.Fatalf("runVet --json: %v", err)
	}
	var r struct {
		Model json.RawMessage `json:"model"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &r); err != nil {
		t.Fatalf("--json must parse: %v", err)
	}
	if string(r.Model) != "null" {
		t.Errorf("degraded model must serialize as null, got %s", r.Model)
	}
}

func TestVet_ExamplesAndTriggerWords(t *testing.T) {
	// Synthetic version: nsfwLevel 4 and 8 count as mature; 16 (unknown
	// bit) and "Soft" (string drift) do not. One image carries meta.
	body := `{"id":11,"modelId":22,"name":"v1","baseModel":"SD 1.5","publishedAt":"2026-01-02T03:04:05Z",` +
		`"trainedWords":["foo bar","baz"],"stats":{"downloadCount":10,"thumbsUpCount":5,"thumbsDownCount":1},` +
		`"files":[{"name":"m.safetensors","sizeKB":2048,"primary":true,"pickleScanResult":"Success","virusScanResult":"Success",` +
		`"metadata":{"format":"SafeTensor","fp":"fp16"},"hashes":{"SHA256":"ABCD","AutoV2":"ABCD"}}],` +
		`"images":[{"nsfwLevel":1,"meta":null},{"nsfwLevel":4,"meta":{"prompt":"x"}},{"nsfwLevel":8},{"nsfwLevel":2},{"nsfwLevel":"Soft"},{"nsfwLevel":16}]}`
	modelBody := `{"id":22,"name":"Mini","type":"LORA","creator":{"username":"u"},` +
		`"allowNoCredit":false,"allowCommercialUse":["None"],"allowDerivatives":false,"allowDifferentLicense":false,` +
		`"stats":{"commentCount":3}}`
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/model-versions/by-hash/0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF":
			w.Write([]byte(body))
		case "/models/22":
			w.Write([]byte(modelBody))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Model not found"}`))
		}
	})
	if err := runVetCmd(t, env, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("runVet: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{
		"EXAMPLES  6 (mature 2)",
		"TRIGGER WORDS (2)\nfoo bar\nbaz\n",
		"comments 3",
		"ratio 5:1",
		"2.0 MB",
		"credit: no",
		"commercial: none",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("synthetic report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `"foo bar"`) || strings.Contains(out, "- foo bar") {
		t.Errorf("trigger words must carry no quotes or bullets")
	}
}

func TestVet_UsageErrors(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, nil))
	missing := filepath.Join(t.TempDir(), "nope.safetensors")
	cases := []struct {
		name string
		args []string
	}{
		{"no argument", nil},
		{"two arguments", []string{"a", "b"}},
		{"missing path", []string{missing}},
		{"directory", []string{t.TempDir()}},
		{"8-hex invalid length", []string{"DEADBEEF"}},
		{"63-hex invalid length", []string{strings.Repeat("A", 63)}},
		{"65-hex invalid length", []string{strings.Repeat("A", 65)}},
	}
	for _, tc := range cases {
		err := runVetCmd(t, env, tc.args...)
		if err == nil {
			t.Errorf("%s: must be rejected", tc.name)
			continue
		}
		if got := vetExitCode(err, false); got != 1 {
			t.Errorf("%s: usage-class error must map to exit 1, got %d", tc.name, got)
		}
	}
}

func TestVet_PromptsFlagRegistered(t *testing.T) {
	// M3 wires --prompts into vet (the M2 placeholder asserted its absence).
	fs := flag.NewFlagSet("vet", flag.ContinueOnError)
	var g globals
	registerGlobalFlags(fs, &g)
	var o vetOptions
	o.register(fs)
	if fs.Lookup("prompts") == nil {
		t.Errorf("--prompts must be registered for vet in M3")
	}
}

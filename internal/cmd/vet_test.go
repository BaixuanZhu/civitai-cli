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
	"reflect"
	"strings"
	"sync"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/testutil"
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
		"LATEST    turbo-v1.1 (id 3263843, 2026-08-24)   base: Anima   relation: current latest; base unchanged",
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
	// Exactly one LATEST line, inserted between VERSION and CREATOR.
	if n := strings.Count(out, "LATEST"); n != 1 {
		t.Errorf("report must carry exactly one LATEST line, got %d:\n%s", n, out)
	}
	iV, iL, iC := strings.Index(out, "VERSION"), strings.Index(out, "LATEST"), strings.Index(out, "CREATOR")
	if !(iV < iL && iL < iC) {
		t.Errorf("LATEST must sit between VERSION and CREATOR (VERSION %d, LATEST %d, CREATOR %d)", iV, iL, iC)
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
		LatestPublished vetLatestPublished `json:"latestPublished"`
		Examples        struct {
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
	// The pinned Anima model's unique latest is the vetted version itself,
	// with the original full RFC3339 timestamp preserved.
	wantLatest := vetWantKnown(3263843, "turbo-v1.1", "2026-08-24T23:42:31.929Z", "Anima",
		vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged))
	if !reflect.DeepEqual(r.LatestPublished, wantLatest) {
		t.Errorf("json latestPublished = %+v, want %+v", r.LatestPublished, wantLatest)
	}
	if !strings.Contains(env.buf.String(), `"tiedVersionIds": []`) {
		t.Errorf("empty tied ids must serialize as []:\n%s", env.buf.String())
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
	for _, want := range []string{"CREATOR   -", "LICENSE   unknown", "DL 66.8k", "turbo-v1.1",
		"LATEST    unknown (model details unavailable)   relation: unknown; base change unknown"} {
		if !strings.Contains(out, want) {
			t.Errorf("degraded report missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(env.logs.String(), "unavailable") {
		t.Errorf("degradation must warn on stderr, logs: %s", env.logs.String())
	}
	// The existing model-fetch warning is the only one: unknown latest
	// context is a report state, not a new log event.
	if n := strings.Count(env.logs.String(), "[WARN]"); n != 1 {
		t.Errorf("degraded vet must keep exactly the one existing warning, got %d: %s", n, env.logs.String())
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
		Model           json.RawMessage    `json:"model"`
		LatestPublished vetLatestPublished `json:"latestPublished"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &r); err != nil {
		t.Fatalf("--json must parse: %v", err)
	}
	if string(r.Model) != "null" {
		t.Errorf("degraded model must serialize as null, got %s", r.Model)
	}
	// Model degradation keeps the hit but makes every latest fact unknown.
	want := vetWantUnknown(vetReasonModelUnavailable)
	if !reflect.DeepEqual(r.LatestPublished, want) {
		t.Errorf("degraded latestPublished = %+v, want %+v", r.LatestPublished, want)
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

// vetRouteNotFound marks a recorded route that must answer with the API
// 404 shape (deliberate degradation scenarios), distinguishing it from an
// unregistered route, which fails the test as unexpected.
const vetRouteNotFound = "\x00not-found"

// vetRouteDrop marks a recorded route whose connection is hijacked and
// closed without a response, injecting a transport failure.
const vetRouteDrop = "\x00drop"

// vetRouteRecorder is a vet-local, invocation-scoped route handler: it
// records ordered request paths, serves inline bodies, and fails the test
// on any route outside the expected set (compare-prefixed helpers and the
// shared testEnv stay untouched). Every response carries Connection:
// close so each request uses a fresh connection, which keeps injected
// transport failures deterministic (no transport-level retry on a reused
// connection).
type vetRouteRecorder struct {
	t     *testing.T
	mu    sync.Mutex
	paths []string
}

// handler serves the routes map and records each request path in order.
func (r *vetRouteRecorder) handler(routes map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.paths = append(r.paths, req.URL.Path)
		r.mu.Unlock()
		body, ok := routes[req.URL.Path]
		if !ok {
			r.t.Errorf("unexpected route %s", req.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Model not found"}`))
			return
		}
		if body == vetRouteDrop {
			hj, ok := w.(http.Hijacker)
			if !ok {
				r.t.Errorf("hijack unsupported by test server")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				r.t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
			return
		}
		w.Header().Set("Connection", "close")
		if body == vetRouteNotFound {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Model not found"}`))
			return
		}
		w.Write([]byte(body))
	}
}

// snapshot returns a copy of the recorded request paths in order.
func (r *vetRouteRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

// vetRecordEnv builds a test environment whose handler is a fresh
// vetRouteRecorder for the given routes.
func vetRecordEnv(t *testing.T, routes map[string]string) (*testEnv, *vetRouteRecorder) {
	t.Helper()
	rec := &vetRouteRecorder{t: t}
	return newTestEnv(t, rec.handler(routes)), rec
}

// vetAssertPaths asserts the exact ordered request path sequence.
func vetAssertPaths(t *testing.T, rec *vetRouteRecorder, want []string) {
	t.Helper()
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("request paths = %v, want %v", got, want)
	}
}

// vetLatestLine extracts the single LATEST line from human vet output.
func vetLatestLine(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "LATEST") {
			return line
		}
	}
	t.Fatalf("no LATEST line in output:\n%s", out)
	return ""
}

// TestVet_LatestMatrix runs the shared synthetic scenario matrix through
// both normal output views and asserts the complete corresponding facts
// in each: the exact human LATEST line and the full latestPublished JSON
// block must agree, data-level unknown/tied states must stay silent on
// stderr, and no state may use advice wording.
func TestVet_LatestMatrix(t *testing.T) {
	const hash = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"
	byHashPath := "/model-versions/by-hash/" + hash
	modelPath := "/models/42"

	byHash := func(id int, base string) string {
		return fmt.Sprintf(`{"id":%d,"modelId":42,"name":"cur","baseModel":%q,"publishedAt":"2026-01-01T00:00:00Z","files":[],"images":[]}`, id, base)
	}
	model := func(versions string) string {
		return fmt.Sprintf(`{"id":42,"name":"M","type":"Checkpoint","creator":{"username":"u"},"modelVersions":[%s]}`, versions)
	}
	mv := func(id int, name, base, publishedAt string) string {
		return fmt.Sprintf(`{"id":%d,"name":%q,"baseModel":%q,"publishedAt":%q}`, id, name, base, publishedAt)
	}
	baseModelBody := `{"id":42,"name":"M","type":"Checkpoint","creator":{"username":"u"}}`

	cases := []struct {
		name     string
		byHash   string
		model    string
		wantLine string
		want     vetLatestPublished
		wantWarn bool
	}{
		{
			name:     "current latest",
			byHash:   byHash(20, "Anima"),
			model:    model(mv(10, "v1", "Anima", vetT1) + "," + mv(20, "v2", "Anima", vetT3)),
			wantLine: "LATEST    v2 (id 20, 2026-03-03)   base: Anima   relation: current latest; base unchanged",
			want: vetWantKnown(20, "v2", vetT3, "Anima",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnchanged)),
		},
		{
			name:     "older current base changed",
			byHash:   byHash(10, "Anima"),
			model:    model(mv(10, "v1", "Anima", vetT1) + "," + mv(20, "v2", "SDXL", vetT3)),
			wantLine: "LATEST    v2 (id 20, 2026-03-03)   base: SDXL   relation: newer published; base changed",
			want: vetWantKnown(20, "v2", vetT3, "SDXL",
				vetWantCmp(vetRelationNewerPublished, vetReasonNone, vetBaseChangeChanged)),
		},
		{
			name:     "latest base missing makes change unknown",
			byHash:   byHash(20, "Anima"),
			model:    model(mv(10, "v1", "Anima", vetT1) + "," + mv(20, "v2", "", vetT3)),
			wantLine: "LATEST    v2 (id 20, 2026-03-03)   base: unknown   relation: current latest; base change unknown",
			want: vetWantKnown(20, "v2", vetT3, "",
				vetWantCmp(vetRelationCurrentLatest, vetReasonNone, vetBaseChangeUnknown)),
		},
		{
			name:     "supplied empty version list",
			byHash:   byHash(10, "Anima"),
			model:    model(""),
			wantLine: "LATEST    unknown (no visible versions)   relation: unknown; base change unknown",
			want:     vetWantUnknown(vetReasonNoVisibleVersions),
		},
		{
			name:     "version list field absent",
			byHash:   byHash(10, "Anima"),
			model:    baseModelBody,
			wantLine: "LATEST    unknown (version list unavailable)   relation: unknown; base change unknown",
			want:     vetWantUnknown(vetReasonVersionsUnavailable),
		},
		{
			name:     "current version not visible",
			byHash:   byHash(99, "Anima"),
			model:    model(mv(10, "v1", "Anima", vetT1) + "," + mv(20, "v2", "Anima", vetT3)),
			wantLine: "LATEST    v2 (id 20, 2026-03-03)   base: Anima   relation: unknown (current version not visible); base unchanged",
			want: vetWantKnown(20, "v2", vetT3, "Anima",
				vetWantCmp(vetRelationUnknown, vetReasonCurrentNotVisible, vetBaseChangeUnchanged)),
		},
		{
			name:     "missing timestamp",
			byHash:   byHash(10, "Anima"),
			model:    model(mv(10, "v1", "Anima", "") + "," + mv(20, "v2", "Anima", vetT3)),
			wantLine: "LATEST    unknown (timestamps unavailable or invalid)   relation: unknown; base change unknown",
			want:     vetWantUnknown(vetReasonTimestampUnavailable),
		},
		{
			name:     "malformed timestamp",
			byHash:   byHash(10, "Anima"),
			model:    model(mv(10, "v1", "Anima", "bogus") + "," + mv(20, "v2", "Anima", vetT3)),
			wantLine: "LATEST    unknown (timestamps unavailable or invalid)   relation: unknown; base change unknown",
			want:     vetWantUnknown(vetReasonTimestampUnavailable),
		},
		{
			name:     "tied maximum including current",
			byHash:   byHash(10, "Anima"),
			model:    model(mv(10, "v1", "Anima", vetT3) + "," + mv(20, "v2", "Anima", vetT3)),
			wantLine: "LATEST    unknown (latest tied; version ids 10,20)   relation: tied latest; base change unknown",
			want: vetWantTied([]int{10, 20},
				vetWantCmp(vetRelationTiedLatest, vetReasonLatestTied, vetBaseChangeUnknown)),
		},
		{
			name:     "model fetch degraded",
			byHash:   byHash(10, "Anima"),
			model:    vetRouteNotFound,
			wantLine: "LATEST    unknown (model details unavailable)   relation: unknown; base change unknown",
			want:     vetWantUnknown(vetReasonModelUnavailable),
			wantWarn: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routes := map[string]string{byHashPath: tc.byHash, modelPath: tc.model}

			envH, recH := vetRecordEnv(t, routes)
			if err := runVetCmd(t, envH, hash); err != nil {
				t.Fatalf("vet human: %v", err)
			}
			human := envH.buf.String()
			if line := vetLatestLine(t, human); line != tc.wantLine {
				t.Errorf("LATEST line = %q, want %q", line, tc.wantLine)
			}
			lower := strings.ToLower(human)
			for _, banned := range []string{"outdated", "upgrade", "recommend", "best", "rollback", "trend"} {
				if strings.Contains(lower, banned) {
					t.Errorf("human output must not carry %q wording:\n%s", banned, human)
				}
			}
			vetAssertPaths(t, recH, []string{byHashPath, modelPath})

			envJ, recJ := vetRecordEnv(t, routes)
			if err := runVetCmd(t, envJ, hash, "--json"); err != nil {
				t.Fatalf("vet json: %v", err)
			}
			var rep struct {
				LatestPublished vetLatestPublished `json:"latestPublished"`
			}
			if err := json.Unmarshal(envJ.buf.Bytes(), &rep); err != nil {
				t.Fatalf("vet json parse: %v\n%s", err, envJ.buf.String())
			}
			if !reflect.DeepEqual(rep.LatestPublished, tc.want) {
				t.Errorf("json latestPublished = %+v, want %+v", rep.LatestPublished, tc.want)
			}
			vetAssertPaths(t, recJ, []string{byHashPath, modelPath})

			logs := envH.logs.String() + envJ.logs.String()
			if tc.wantWarn {
				if !strings.Contains(logs, "[WARN]") {
					t.Errorf("model degradation must keep the existing warning, logs: %s", logs)
				}
			} else if strings.Contains(logs, "[WARN]") {
				t.Errorf("data-level unknown/tied states must not add warnings, logs: %s", logs)
			}
		})
	}
}

// TestVet_RequestCounts pins the zero-new-requests contract: every output
// mode and degradation path performs only the pre-existing by-hash and
// best-effort owning-model GETs, in order, with no supplementary calls.
func TestVet_RequestCounts(t *testing.T) {
	byHashPath := "/model-versions/by-hash/" + animaSHA256
	modelPath := "/models/2458426"
	byHashBody := string(testutil.LoadFixture(t, "byhash_anima_turbo.json"))
	modelBody := string(testutil.LoadFixture(t, "models_2458426.json"))
	fullRoutes := map[string]string{byHashPath: byHashBody, modelPath: modelBody}

	t.Run("human", func(t *testing.T) {
		env, rec := vetRecordEnv(t, fullRoutes)
		if err := runVetCmd(t, env, animaSHA256); err != nil {
			t.Fatalf("vet: %v", err)
		}
		vetAssertPaths(t, rec, []string{byHashPath, modelPath})
	})

	t.Run("json", func(t *testing.T) {
		env, rec := vetRecordEnv(t, fullRoutes)
		if err := runVetCmd(t, env, animaSHA256, "--json"); err != nil {
			t.Fatalf("vet --json: %v", err)
		}
		vetAssertPaths(t, rec, []string{byHashPath, modelPath})
	})

	t.Run("prompts json", func(t *testing.T) {
		env, rec := vetRecordEnv(t, fullRoutes)
		if err := runVetCmd(t, env, animaSHA256, "--prompts", "--json"); err != nil {
			t.Fatalf("vet --prompts --json: %v", err)
		}
		vetAssertPaths(t, rec, []string{byHashPath, modelPath})
	})

	t.Run("model 404", func(t *testing.T) {
		env, rec := vetRecordEnv(t, map[string]string{byHashPath: byHashBody, modelPath: vetRouteNotFound})
		if err := runVetCmd(t, env, animaSHA256); err != nil {
			t.Fatalf("model 404 must keep the hit: %v", err)
		}
		vetAssertPaths(t, rec, []string{byHashPath, modelPath})
	})

	t.Run("model undecodable", func(t *testing.T) {
		env, rec := vetRecordEnv(t, map[string]string{byHashPath: byHashBody, modelPath: "{not json"})
		if err := runVetCmd(t, env, animaSHA256); err != nil {
			t.Fatalf("undecodable model must keep the hit: %v", err)
		}
		vetAssertPaths(t, rec, []string{byHashPath, modelPath})
	})

	t.Run("missing modelId", func(t *testing.T) {
		noModelID := `{"id":3263843,"name":"turbo-v1.1","baseModel":"Anima","publishedAt":"2026-08-24T23:42:31.929Z","files":[],"images":[]}`
		env, rec := vetRecordEnv(t, map[string]string{byHashPath: noModelID})
		if err := runVetCmd(t, env, animaSHA256); err != nil {
			t.Fatalf("missing modelId must keep the hit: %v", err)
		}
		vetAssertPaths(t, rec, []string{byHashPath})
		if !strings.Contains(env.buf.String(), "unknown (model details unavailable)") {
			t.Errorf("missing modelId must degrade latest context:\n%s", env.buf.String())
		}
	})

	t.Run("by-hash 404", func(t *testing.T) {
		env, rec := vetRecordEnv(t, map[string]string{byHashPath: vetRouteNotFound})
		err := runVetCmd(t, env, animaSHA256)
		var nfe *api.NotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("expected NotFoundError, got %v", err)
		}
		if got := vetExitCode(err, false); got != 3 {
			t.Errorf("by-hash 404 must keep exit 3, got %d", got)
		}
		if env.buf.Len() != 0 {
			t.Errorf("by-hash 404 must emit no partial report: %s", env.buf.String())
		}
		vetAssertPaths(t, rec, []string{byHashPath})
	})

	t.Run("model transport failure", func(t *testing.T) {
		env, rec := vetRecordEnv(t, map[string]string{byHashPath: byHashBody, modelPath: vetRouteDrop})
		// No backoffs: the injected transport failure fails on the first
		// attempt instead of waiting through the default retry ladder,
		// proving attempts come solely from the pre-existing client policy.
		old := newAPIClient
		newAPIClient = func(g *globals) *api.Client {
			return api.NewClient(api.WithBaseURL(env.srv.URL), api.WithToken(g.resolveToken()),
				api.WithTimeout(g.Timeout), api.WithBackoffs())
		}
		t.Cleanup(func() { newAPIClient = old })

		if err := runVetCmd(t, env, animaSHA256); err != nil {
			t.Fatalf("model transport failure must keep the hit: %v", err)
		}
		vetAssertPaths(t, rec, []string{byHashPath, modelPath})
		if !strings.Contains(env.buf.String(), "unknown (model details unavailable)") {
			t.Errorf("transport failure must degrade latest context:\n%s", env.buf.String())
		}
	})
}

// TestVet_LatestHashFileEquivalence proves a local file and its full
// SHA256 input produce identical latest context (human line and complete
// JSON block) over the same response bodies, with the same request shape.
func TestVet_LatestHashFileEquivalence(t *testing.T) {
	content := []byte("civ vet latest equivalence\n")
	sum := sha256.Sum256(content)
	fileHash := strings.ToUpper(hex.EncodeToString(sum[:]))
	path := filepath.Join(t.TempDir(), "eq.safetensors")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	byHashBody := string(testutil.LoadFixture(t, "byhash_anima_turbo.json"))
	modelBody := string(testutil.LoadFixture(t, "models_2458426.json"))
	routes := map[string]string{
		"/model-versions/by-hash/" + animaSHA256: byHashBody,
		"/model-versions/by-hash/" + fileHash:    byHashBody,
		"/models/2458426":                        modelBody,
	}

	run := func(arg string, jsonMode bool) (string, vetLatestPublished, []string) {
		t.Helper()
		env, rec := vetRecordEnv(t, routes)
		args := []string{arg}
		if jsonMode {
			args = append(args, "--json")
		}
		if err := runVetCmd(t, env, args...); err != nil {
			t.Fatalf("vet %q: %v", arg, err)
		}
		if jsonMode {
			var rep struct {
				LatestPublished vetLatestPublished `json:"latestPublished"`
			}
			if err := json.Unmarshal(env.buf.Bytes(), &rep); err != nil {
				t.Fatalf("vet json parse: %v", err)
			}
			return "", rep.LatestPublished, rec.snapshot()
		}
		return vetLatestLine(t, env.buf.String()), vetLatestPublished{}, rec.snapshot()
	}

	assertShape := func(paths []string) {
		t.Helper()
		if len(paths) != 2 || !strings.HasPrefix(paths[0], "/model-versions/by-hash/") || paths[1] != "/models/2458426" {
			t.Errorf("request shape = %v, want [by-hash, /models/2458426]", paths)
		}
	}

	hLine, _, hPaths := run(animaSHA256, false)
	fLine, _, fPaths := run(path, false)
	if hLine != fLine {
		t.Errorf("human LATEST line differs:\nhash: %s\nfile: %s", hLine, fLine)
	}
	assertShape(hPaths)
	assertShape(fPaths)

	_, hLatest, _ := run(animaSHA256, true)
	_, fLatest, _ := run(path, true)
	if !reflect.DeepEqual(hLatest, fLatest) {
		t.Errorf("latestPublished differs:\nhash: %+v\nfile: %+v", hLatest, fLatest)
	}
}

// TestVet_PromptsHumanAppendsAfterLatest keeps the human --prompts
// contract: the normal report (including the LATEST line) is followed by
// the unchanged recipe blocks.
func TestVet_PromptsHumanAppendsAfterLatest(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, vetRoutes()))
	if err := runVetCmd(t, env, animaSHA256, "--prompts"); err != nil {
		t.Fatalf("vet --prompts: %v", err)
	}
	out := env.buf.String()
	iL, iP := strings.Index(out, "LATEST"), strings.Index(out, "PROMPTS (9)")
	if iL < 0 || iP < 0 || iL > iP {
		t.Errorf("human --prompts must render the normal report (incl. LATEST) before recipes (LATEST %d, PROMPTS %d):\n%s", iL, iP, out)
	}
}

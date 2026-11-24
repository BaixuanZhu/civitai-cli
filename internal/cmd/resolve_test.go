package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"civitai-cli/internal/api"
	"civitai-cli/internal/resolver"
)

// resolveTestFile is one file in a synthetic /model-versions body.
type resolveTestFile struct {
	ID      int               `json:"id"`
	Name    string            `json:"name"`
	Type    string            `json:"type,omitempty"`
	Primary bool              `json:"primary,omitempty"`
	Hashes  map[string]string `json:"hashes"`
}

// resolveTestModel is the owning-model summary of a synthetic version.
type resolveTestModel struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// resolveTestVersion is a synthetic /model-versions body carrying exactly
// the fields resolve projects (identity, AIR, base metadata, files).
type resolveTestVersion struct {
	ID            int               `json:"id"`
	ModelID       int               `json:"modelId"`
	Name          string            `json:"name,omitempty"`
	Air           string            `json:"air,omitempty"`
	BaseModel     string            `json:"baseModel,omitempty"`
	BaseModelType string            `json:"baseModelType,omitempty"`
	Model         *resolveTestModel `json:"model,omitempty"`
	Files         []resolveTestFile `json:"files"`
}

// resolveServer serves synthetic versions by id and by hash, counting
// every request so tests can assert lookup dedupe and zero-I/O usage
// errors.
func resolveServer(t *testing.T, versions map[int]resolveTestVersion, byHash map[string]int) (*testEnv, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		// Resolve is read-only: every request must be a GET on a
		// model-versions metadata path (no POST, download, or other call).
		if r.Method != http.MethodGet {
			t.Errorf("resolve must only issue GETs, got %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.URL.Path, "/model-versions/") {
			t.Errorf("resolve must only request model-versions metadata, got %s", r.URL.Path)
		}
		path := r.URL.Path
		if hash, ok := strings.CutPrefix(path, "/model-versions/by-hash/"); ok {
			id, found := byHash[hash]
			if !found {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"Model not found"}`))
				return
			}
			json.NewEncoder(w).Encode(versions[id])
			return
		}
		if idStr, ok := strings.CutPrefix(path, "/model-versions/"); ok {
			id, err := strconv.Atoi(idStr)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"Model not found"}`))
				return
			}
			v, found := versions[id]
			if !found {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"Model not found"}`))
				return
			}
			json.NewEncoder(w).Encode(v)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Model not found"}`))
	})
	return env, &count
}

// writeResolveFile writes content to dir/name and returns its path plus
// the uppercase SHA256 the registry fixture must carry.
func writeResolveFile(t *testing.T, dir, name string, content []byte) (string, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	sum := sha256.Sum256(content)
	return path, strings.ToUpper(hex.EncodeToString(sum[:]))
}

// runResolveCmd mirrors the runner's flag handling without os.Exit.
func runResolveCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	pos := parseForTest(t, &env.g, nil, args)
	return runResolve(context.Background(), &env.g, pos)
}

// resolveFixture is the ready two-resource registry fixture: one Anima
// checkpoint and one compatible Anima LORA, each with one file.
func resolveFixture(ckptHash, loraHash string) (map[int]resolveTestVersion, map[string]int) {
	versions := map[int]resolveTestVersion{
		100: {
			ID: 100, ModelID: 10, Name: "ckpt-v1",
			Air:       "urn:air:anima:checkpoint:civitai:10@100",
			BaseModel: "Anima", BaseModelType: "Anima",
			Model: &resolveTestModel{Name: "Anima", Type: "Checkpoint"},
			Files: []resolveTestFile{{
				ID: 1000, Name: "ckpt.safetensors", Type: "Model", Primary: true,
				Hashes: map[string]string{"SHA256": ckptHash},
			}},
		},
		200: {
			ID: 200, ModelID: 20, Name: "lora-v1",
			Air:       "urn:air:anima:lora:civitai:20@200",
			BaseModel: "Anima", BaseModelType: "Anima",
			Model: &resolveTestModel{Name: "Detail LoRA", Type: "LORA"},
			Files: []resolveTestFile{{
				ID: 2000, Name: "detail.safetensors", Type: "Model", Primary: true,
				Hashes: map[string]string{"SHA256": loraHash},
			}},
		},
	}
	byHash := map[string]int{ckptHash: 100, loraHash: 200}
	return versions, byHash
}

// resolveIdentityJSON is the identity projection of one resource.
type resolveIdentityJSON struct {
	ModelID       int    `json:"modelId"`
	VersionID     int    `json:"versionId"`
	AIR           string `json:"air"`
	FileID        int    `json:"fileId"`
	SHA256        string `json:"sha256"`
	ModelType     string `json:"modelType"`
	BaseModel     string `json:"baseModel"`
	BaseModelType string `json:"baseModelType"`
}

// resolveLocalJSON is the local-presence record of one resource.
type resolveLocalJSON struct {
	State          string `json:"state"`
	Path           string `json:"path"`
	ComputedSHA256 string `json:"computedSha256"`
	MatchedFileID  int    `json:"matchedFileId"`
}

// resolveResourceJSON is one evaluated resource.
type resolveResourceJSON struct {
	InputIndex   int                 `json:"inputIndex"`
	RequestedRef string              `json:"requestedRef"`
	Resolution   string              `json:"resolution"`
	Identity     resolveIdentityJSON `json:"identity"`
	Local        resolveLocalJSON    `json:"local"`
}

// resolveCompatJSON is one compatibility result.
type resolveCompatJSON struct {
	LeftIndex  int    `json:"leftIndex"`
	RightIndex int    `json:"rightIndex"`
	Status     string `json:"status"`
	RuleID     string `json:"ruleId"`
	Left       struct {
		BaseModel string `json:"baseModel"`
	} `json:"left"`
	Right struct {
		BaseModel string `json:"baseModel"`
	} `json:"right"`
}

// resolveEvidenceJSON is the structured evidence of one finding.
type resolveEvidenceJSON struct {
	Path     string               `json:"path"`
	Expected *resolveIdentityJSON `json:"expected"`
	Actual   *resolveIdentityJSON `json:"actual"`
}

// resolveFindingJSON is one stable finding.
type resolveFindingJSON struct {
	Code            string              `json:"code"`
	Blocking        bool                `json:"blocking"`
	ResourceIndexes []int               `json:"resourceIndexes"`
	Message         string              `json:"message"`
	RuleID          string              `json:"ruleId"`
	Evidence        resolveEvidenceJSON `json:"evidence"`
}

// resolveReportJSON is the machine shape the tests pin.
type resolveReportJSON struct {
	SchemaVersion int                   `json:"schemaVersion"`
	PolicyVersion string                `json:"policyVersion"`
	Ready         bool                  `json:"ready"`
	Resources     []resolveResourceJSON `json:"resources"`
	Compatibility []resolveCompatJSON   `json:"compatibility"`
	Findings      []resolveFindingJSON  `json:"findings"`
	Summary       struct {
		ResourceCount        int `json:"resourceCount"`
		BlockingFindingCount int `json:"blockingFindingCount"`
	} `json:"summary"`
}

// parseResolveReport decodes captured stdout.
func parseResolveReport(t *testing.T, env *testEnv) resolveReportJSON {
	t.Helper()
	var rep resolveReportJSON
	if err := json.Unmarshal(env.buf.Bytes(), &rep); err != nil {
		t.Fatalf("--json must parse: %v\n%s", err, env.buf.String())
	}
	return rep
}

// TestResolve_ReadyJSON pins AC2's machine report: READY, per-resource
// model/version/file ids, exact SHA256, AIR, local present, and the
// compatible rule id.
func TestResolve_ReadyJSON(t *testing.T) {
	dir := t.TempDir()
	ckptPath, ckptHash := writeResolveFile(t, dir, "ckpt.safetensors", []byte("checkpoint bytes"))
	loraPath, loraHash := writeResolveFile(t, dir, "detail.safetensors", []byte("lora bytes"))
	versions, byHash := resolveFixture(ckptHash, loraHash)
	env, count := resolveServer(t, versions, byHash)

	if err := runResolveCmd(t, env, ckptPath, loraPath, "--json"); err != nil {
		t.Fatalf("runResolve: %v", err)
	}
	rep := parseResolveReport(t, env)
	if !rep.Ready || rep.SchemaVersion != 1 || rep.PolicyVersion != resolver.PolicyVersionV1 {
		t.Fatalf("report header/verdict = %+v", rep)
	}
	if len(rep.Resources) != 2 || rep.Summary.ResourceCount != 2 || rep.Summary.BlockingFindingCount != 0 {
		t.Fatalf("resources/summary = %+v %+v", rep.Resources, rep.Summary)
	}
	r := rep.Resources[0]
	if r.Resolution != "resolved" || r.Local.State != "present" {
		t.Errorf("checkpoint state = %+v", r)
	}
	if r.Identity.ModelID != 10 || r.Identity.VersionID != 100 || r.Identity.FileID != 1000 {
		t.Errorf("checkpoint identity = %+v", r.Identity)
	}
	if r.Identity.SHA256 != ckptHash || r.Local.ComputedSHA256 != ckptHash || r.Local.MatchedFileID != 1000 {
		t.Errorf("checkpoint hash evidence = %+v", r)
	}
	if r.Identity.AIR != "urn:air:anima:checkpoint:civitai:10@100" {
		t.Errorf("checkpoint AIR = %q", r.Identity.AIR)
	}
	if len(rep.Compatibility) != 1 || rep.Compatibility[0].Status != "compatible" ||
		rep.Compatibility[0].RuleID != resolver.RuleCompatibleBaseModel {
		t.Errorf("compatibility = %+v", rep.Compatibility)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("ready report must have no findings: %+v", rep.Findings)
	}
	if got := count.Load(); got != 2 {
		t.Errorf("requests = %d, want one by-hash GET per path", got)
	}
	if strings.Contains(env.logs.String(), "WARN") {
		t.Errorf("ready path must not warn: %s", env.logs.String())
	}
}

// TestResolve_ReadyHuman pins the human rendering: verdict first, full
// ids/hashes, and the same facts as JSON.
func TestResolve_ReadyHuman(t *testing.T) {
	dir := t.TempDir()
	ckptPath, ckptHash := writeResolveFile(t, dir, "ckpt.safetensors", []byte("checkpoint bytes"))
	loraPath, loraHash := writeResolveFile(t, dir, "detail.safetensors", []byte("lora bytes"))
	versions, byHash := resolveFixture(ckptHash, loraHash)
	env, count := resolveServer(t, versions, byHash)

	if err := runResolveCmd(t, env, ckptPath, loraPath); err != nil {
		t.Fatalf("runResolve: %v", err)
	}
	out := env.buf.String()
	if !strings.HasPrefix(out, "READY\n") {
		t.Errorf("human output must start with READY:\n%s", out)
	}
	for _, want := range []string{
		"policy resolver-v1",
		"RESOURCES (2)",
		"identity: model 10 / version 100 / file 1000",
		"urn:air:anima:checkpoint:civitai:10@100",
		ckptHash,
		"local: present",
		"[0] Checkpoint (base Anima) -> [1] LORA (base Anima): compatible",
		"rule " + resolver.RuleCompatibleBaseModel,
		"FINDINGS (0)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human report missing %q:\n%s", want, out)
		}
	}
	// Human and JSON make the same number of registry requests.
	if got := count.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// TestResolve_NotReadyExit5 pins the completed-not-ready contract: the
// report is emitted and the sentinel maps to exit 5 while the shared
// mapping stays untouched.
func TestResolve_NotReadyExit5(t *testing.T) {
	dir := t.TempDir()
	ckptPath, ckptHash := writeResolveFile(t, dir, "ckpt.safetensors", []byte("checkpoint bytes"))
	versions, byHash := resolveFixture(ckptHash, strings.Repeat("B", 64))
	env, _ := resolveServer(t, versions, byHash)
	missing := filepath.Join(dir, "missing.safetensors")

	err := runResolveCmd(t, env, ckptPath, missing, "--json")
	var nr *resolveNotReadyError
	if !errors.As(err, &nr) {
		t.Fatalf("not-ready error = %T (%v), want *resolveNotReadyError", err, err)
	}
	if got := resolveExitCode(err, false); got != 5 {
		t.Errorf("resolveExitCode = %d, want 5", got)
	}
	if got := exitCodeFor(err, false); got != 1 {
		t.Errorf("shared exitCodeFor must not change for resolve, got %d", got)
	}
	rep := parseResolveReport(t, env)
	if rep.Ready || rep.Summary.BlockingFindingCount == 0 {
		t.Errorf("report must be not ready with findings: %+v", rep)
	}
	var missingFinding *resolveFindingJSON
	for i := range rep.Findings {
		if rep.Findings[i].Code == resolver.CodeMissingResource {
			missingFinding = &rep.Findings[i]
			break
		}
	}
	if missingFinding == nil {
		t.Fatalf("expected missing_resource, got %+v", rep.Findings)
	}
	if !missingFinding.Blocking || missingFinding.Evidence.Path != missing {
		t.Errorf("missing finding = %+v", missingFinding)
	}
}

// TestResolve_UsageErrorsNoIO pins AC6: rejected forms fail before any
// hashing or registry request.
func TestResolve_UsageErrorsNoIO(t *testing.T) {
	oldHash := computeHash
	computeHash = func(string) (string, error) {
		t.Errorf("usage errors must not hash local files")
		return "", errors.New("unexpected hash")
	}
	t.Cleanup(func() { computeHash = oldHash })

	env, count := resolveServer(t, nil, nil)
	cases := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"bare numeric id", []string{"2458426"}},
		{"display name", []string{"anima"}},
		{"autov2 short hash", []string{"FBA1195327"}},
		{"air without version", []string{"urn:air:anima:checkpoint:civitai:2458426"}},
		{"version zero", []string{"version:0"}},
		{"63-char hash", []string{strings.Repeat("A", 63)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runResolveCmd(t, env, tc.args...)
			if err == nil {
				t.Fatalf("must be rejected")
			}
			var ue *resolver.UsageError
			if !errors.As(err, &ue) {
				t.Fatalf("error = %T (%v), want *resolver.UsageError", err, err)
			}
			if got := resolveExitCode(err, false); got != 1 {
				t.Errorf("usage error exit = %d, want 1", got)
			}
			if got := count.Load(); got != 0 {
				t.Errorf("usage errors must make no registry request, got %d", got)
			}
			if env.buf.Len() != 0 {
				t.Errorf("usage errors must write no report: %s", env.buf.String())
			}
		})
	}
}

// TestResolve_DirectoryIsUsage pins the non-regular target rule.
func TestResolve_DirectoryIsUsage(t *testing.T) {
	dir := t.TempDir()
	env, count := resolveServer(t, nil, nil)
	err := runResolveCmd(t, env, dir)
	var ue *resolver.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("directory error = %T (%v), want *resolver.UsageError", err, err)
	}
	if count.Load() != 0 || env.buf.Len() != 0 {
		t.Errorf("directory usage error must not touch the registry or stdout")
	}
}

// TestResolve_ExitMapping pins AC11: 0/5/1/2/4 stay distinguishable and
// vet's 3 is untouched.
func TestResolve_ExitMapping(t *testing.T) {
	notReady := &resolveNotReadyError{blocking: 2}
	cases := []struct {
		name     string
		err      error
		hasToken bool
		want     int
	}{
		{"ready", nil, false, 0},
		{"completed not ready", notReady, false, 5},
		{"usage", errors.New("usage"), false, 1},
		{"network", &api.NetworkError{Attempts: 1, Err: errors.New("x")}, false, 2},
		{"token rejected", &api.UnauthorizedError{}, true, 4},
		{"anonymous unauthorized", &api.UnauthorizedError{}, false, 1},
		{"api rejection", &api.ApiError{Status: 400}, false, 1},
		{"wrapped not ready", fmt.Errorf("resolve: %w", notReady), false, 5},
		{"registry 404 is not vet 3", &api.NotFoundError{}, false, 1},
	}
	for _, tc := range cases {
		if got := resolveExitCode(tc.err, tc.hasToken); got != tc.want {
			t.Errorf("%s: resolveExitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
	// Vet keeps exclusive use of exit 3 for its unknown-hash semantics.
	if got := vetExitCode(&api.NotFoundError{}, false); got != 3 {
		t.Errorf("vet 404 must stay exit 3, got %d", got)
	}
}

// TestResolve_ErrorHint pins the hint policy: no extra hint for a
// completed not-ready verdict, shared guidance for operational failures.
func TestResolve_ErrorHint(t *testing.T) {
	out := captureStderr(t, func() { resolveErrorHint(&resolveNotReadyError{blocking: 1}, false) })
	if out != "" {
		t.Errorf("not-ready hint must be silent, got %q", out)
	}
	out = captureStderr(t, func() { resolveErrorHint(&api.UnauthorizedError{}, true) })
	if !strings.Contains(out, "rejected the token") {
		t.Errorf("operational hints must delegate to the shared guidance, got %q", out)
	}
}

// TestResolve_UnresolvedHashNotReady pins AC7: an unknown full SHA256 is
// a completed unresolved/not-ready report, not an operational failure.
func TestResolve_UnresolvedHashNotReady(t *testing.T) {
	unknown := strings.Repeat("D", 64)
	env, count := resolveServer(t, nil, nil)
	err := runResolveCmd(t, env, unknown, "--json")
	var nr *resolveNotReadyError
	if !errors.As(err, &nr) {
		t.Fatalf("error = %T (%v), want *resolveNotReadyError", err, err)
	}
	rep := parseResolveReport(t, env)
	if rep.Ready || rep.Resources[0].Resolution != "unresolved" ||
		rep.Resources[0].Local.State != "unverified" {
		t.Errorf("unresolved hash resource = %+v", rep.Resources[0])
	}
	found := false
	for _, f := range rep.Findings {
		if f.Code == resolver.CodeUnresolvedResource && f.Evidence.Path == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unresolved_resource with the reference, got %+v", rep.Findings)
	}
	if got := count.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
	if got := resolveExitCode(err, false); got != 5 {
		t.Errorf("exit = %d, want 5", got)
	}
}

// TestResolve_NetworkFailureExit2 pins R11's operational half: a network
// exhaustion aborts before any report and maps to exit 2.
func TestResolve_NetworkFailureExit2(t *testing.T) {
	env, count := resolveStatusServer(t, http.StatusInternalServerError)
	// No retries: the 500 below fails on the first attempt instead of
	// waiting through the default 3.5s backoff ladder.
	old := newAPIClient
	newAPIClient = func(g *globals) *api.Client {
		return api.NewClient(api.WithBaseURL(env.srv.URL), api.WithToken(g.resolveToken()),
			api.WithTimeout(g.Timeout), api.WithBackoffs())
	}
	t.Cleanup(func() { newAPIClient = old })

	err := runResolveCmd(t, env, strings.Repeat("D", 64))
	var ne *api.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("error = %T (%v), want *api.NetworkError", err, err)
	}
	if got := resolveExitCode(err, false); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if env.buf.Len() != 0 {
		t.Errorf("operational failure must leave stdout empty: %s", env.buf.String())
	}
	if count.Load() == 0 {
		t.Errorf("expected at least one request attempt")
	}
}

// resolveStatusServer starts a test environment whose handler always
// answers with status, returning the shared env plus a request counter.
func resolveStatusServer(t *testing.T, status int) (*testEnv, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(status)
	})
	return env, &count
}

// TestResolve_TokenRejectedExit4 pins the token class: no report, exit 4
// with a token, exit 1 anonymously.
func TestResolve_TokenRejectedExit4(t *testing.T) {
	env, count := resolveStatusServer(t, http.StatusUnauthorized)
	hash := strings.Repeat("D", 64)
	err := runResolveCmd(t, env, hash, "--token", "bad-key")
	var ue *api.UnauthorizedError
	if !errors.As(err, &ue) {
		t.Fatalf("error = %T (%v), want *api.UnauthorizedError", err, err)
	}
	if got := resolveExitCode(err, true); got != 4 {
		t.Errorf("exit = %d, want 4", got)
	}
	if env.buf.Len() != 0 {
		t.Errorf("operational failure must leave stdout empty: %s", env.buf.String())
	}
	if count.Load() != 1 {
		t.Errorf("requests = %d, want 1 (401 is not retried)", count.Load())
	}
}

// TestResolve_AIRRef pins the versioned-AIR reference path: the exact
// numeric @version drives GET /model-versions/{id}, AIR is reported as
// metadata, and the resource stays unverified/not-ready without a path.
func TestResolve_AIRRef(t *testing.T) {
	ckptHash := strings.Repeat("A", 64)
	loraHash := strings.Repeat("B", 64)
	versions, byHash := resolveFixture(ckptHash, loraHash)
	env, count := resolveServer(t, versions, byHash)

	air := "urn:air:anima:lora:civitai:20@200"
	err := runResolveCmd(t, env, air, "--json")
	var nr *resolveNotReadyError
	if !errors.As(err, &nr) {
		t.Fatalf("error = %T (%v), want *resolveNotReadyError", err, err)
	}
	rep := parseResolveReport(t, env)
	r := rep.Resources[0]
	if r.Resolution != "resolved" || r.Local.State != "unverified" {
		t.Errorf("AIR resource = %+v", r)
	}
	if r.Identity.VersionID != 200 || r.Identity.ModelID != 20 || r.Identity.AIR != air {
		t.Errorf("AIR identity = %+v", r.Identity)
	}
	if count.Load() != 1 {
		t.Errorf("requests = %d, want one by-id GET", count.Load())
	}
}

// TestResolve_LocalIOFailureExit1 pins local read/hash failures as
// operational errors: no report, exit 1, no registry traffic.
func TestResolve_LocalIOFailureExit1(t *testing.T) {
	dir := t.TempDir()
	path, _ := writeResolveFile(t, dir, "broken.safetensors", []byte("bytes"))
	old := computeHash
	computeHash = func(string) (string, error) { return "", errors.New("permission denied") }
	t.Cleanup(func() { computeHash = old })

	env, count := resolveServer(t, nil, nil)
	err := runResolveCmd(t, env, path)
	if err == nil {
		t.Fatalf("local I/O failure must be operational")
	}
	if got := resolveExitCode(err, false); got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if count.Load() != 0 {
		t.Errorf("local I/O failure must not reach the registry, got %d requests", count.Load())
	}
	if env.buf.Len() != 0 {
		t.Errorf("operational failure must leave stdout empty: %s", env.buf.String())
	}
}

// TestResolve_DecodeFailureExit1 pins malformed upstream data as an
// operational failure, never a ready verdict or a not-ready report.
func TestResolve_DecodeFailureExit1(t *testing.T) {
	hash := strings.Repeat("D", 64)
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":`)) // truncated JSON: decode failure
	})
	err := runResolveCmd(t, env, hash)
	if err == nil {
		t.Fatalf("decode failure must be operational")
	}
	if got := resolveExitCode(err, false); got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if env.buf.Len() != 0 {
		t.Errorf("operational failure must leave stdout empty: %s", env.buf.String())
	}
}

// TestResolve_WindowsPathWithSpaces pins the path grammar under a
// directory name containing spaces: both human and JSON run the same
// lookups and reach READY.
func TestResolve_WindowsPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	ckptPath, ckptHash := writeResolveFile(t, dir, filepath.Join("my models", "checkpoint file.safetensors"), []byte("ckpt bytes"))
	loraPath, loraHash := writeResolveFile(t, dir, filepath.Join("my models", "lora file.safetensors"), []byte("lora bytes"))
	versions, byHash := resolveFixture(ckptHash, loraHash)

	env, count := resolveServer(t, versions, byHash)
	if err := runResolveCmd(t, env, ckptPath, loraPath); err != nil {
		t.Fatalf("runResolve with spaces: %v", err)
	}
	if !strings.HasPrefix(env.buf.String(), "READY") {
		t.Errorf("expected READY:\n%s", env.buf.String())
	}
	humanCount := count.Load()

	envJSON, jsonRequests := resolveServer(t, versions, byHash)
	if err := runResolveCmd(t, envJSON, ckptPath, loraPath, "--json"); err != nil {
		t.Fatalf("runResolve --json with spaces: %v", err)
	}
	if !parseResolveReport(t, envJSON).Ready {
		t.Errorf("JSON run must be ready")
	}
	jsonCount := jsonRequests.Load()
	if humanCount != jsonCount {
		t.Errorf("human/JSON request counts differ: %d vs %d", humanCount, jsonCount)
	}
	if humanCount != 2 {
		t.Errorf("requests = %d, want 2", humanCount)
	}
}

// TestResolve_HelpPage pins the curated page's required content before
// the parent help-map wiring: four ref forms, a --json example, and no
// copy of the shared global block.
func TestResolve_HelpPage(t *testing.T) {
	for _, want := range []string{
		"Usage:", "Examples:", "<sha256>", "<air>", "version:<id>",
		"civ resolve C:\\models\\checkpoint.safetensors --json",
		"5 not ready",
	} {
		if !strings.Contains(resolveHelp, want) {
			t.Errorf("resolveHelp missing %q", want)
		}
	}
	if strings.Contains(resolveHelp, "Global flags") {
		t.Errorf("resolveHelp must not embed the shared global block")
	}
	if strings.Contains(resolveHelp, "Options:") {
		t.Errorf("resolve has no command-specific flags, so no Options section")
	}
}

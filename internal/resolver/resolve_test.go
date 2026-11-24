package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeNotFound is the registry 404 marker of the in-memory environment.
var fakeNotFound = errors.New("fake: not found")

// fakeEnv is an in-memory Environment: no HTTP, no filesystem. Absent
// version/hash entries model registry 404s; the error maps inject
// operational failures. It records calls so tests can assert dedupe.
type fakeEnv struct {
	stat      map[string]FileStatus
	statErrs  map[string]error
	hashes    map[string]string
	hashErrs  map[string]error
	versions  map[int]VersionMetadata
	idErrs    map[int]error
	byHash    map[string]int
	byHashErr map[string]error

	idCalls   []int
	hashCalls []string
}

// env projects the fake state into the resolver's function-value seams.
func (f *fakeEnv) env() Environment {
	return Environment{
		Stat: func(path string) (FileStatus, error) {
			if err := f.statErrs[path]; err != nil {
				return FileStatus{}, err
			}
			return f.stat[path], nil
		},
		HashFile: func(path string) (string, error) {
			if err := f.hashErrs[path]; err != nil {
				return "", err
			}
			h, ok := f.hashes[path]
			if !ok {
				return "", fmt.Errorf("fake: no content for %s", path)
			}
			return h, nil
		},
		VersionByID: func(_ context.Context, id int) (VersionMetadata, error) {
			f.idCalls = append(f.idCalls, id)
			if err := f.idErrs[id]; err != nil {
				return VersionMetadata{}, err
			}
			v, ok := f.versions[id]
			if !ok {
				return VersionMetadata{}, fakeNotFound
			}
			return v, nil
		},
		VersionByHash: func(_ context.Context, hash string) (VersionMetadata, error) {
			f.hashCalls = append(f.hashCalls, hash)
			if err := f.byHashErr[hash]; err != nil {
				return VersionMetadata{}, err
			}
			id, ok := f.byHash[hash]
			if !ok {
				return VersionMetadata{}, fakeNotFound
			}
			v, ok := f.versions[id]
			if !ok {
				return VersionMetadata{}, fakeNotFound
			}
			return v, nil
		},
		IsNotFound: func(err error) bool { return errors.Is(err, fakeNotFound) },
	}
}

// Test hashes (valid 64-character hex strings, uppercase).
const (
	hashCKPT = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	hashLORA = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	hashVAE  = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	hashBAD  = "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"
)

// mkFile builds one visible file.
func mkFile(id int, name, typ string, primary bool, sha string) FileMetadata {
	return FileMetadata{FileID: id, Name: name, Type: typ, Primary: primary, SHA256: sha}
}

// mkVersion builds resolver-owned registry metadata.
func mkVersion(modelID, versionID int, name, modelName, modelType, baseModel, baseModelType, air string, files ...FileMetadata) VersionMetadata {
	return VersionMetadata{
		ModelID:       modelID,
		VersionID:     versionID,
		AIR:           air,
		Name:          name,
		BaseModel:     baseModel,
		BaseModelType: baseModelType,
		ModelName:     modelName,
		ModelType:     modelType,
		Files:         files,
	}
}

// checkpointVersion is the canonical Anima checkpoint used by most tests.
func checkpointVersion() VersionMetadata {
	return mkVersion(10, 100, "ckpt-v1", "Anima", "Checkpoint", "Anima", "Anima",
		"urn:air:anima:checkpoint:civitai:10@100",
		mkFile(1000, "ckpt.safetensors", "Model", true, hashCKPT))
}

// loraVersion is a compatible Anima LORA.
func loraVersion() VersionMetadata {
	return mkVersion(20, 200, "lora-v1", "Detail LoRA", "LORA", "Anima", "Anima",
		"urn:air:anima:lora:civitai:20@200",
		mkFile(2000, "detail.safetensors", "Model", true, hashLORA))
}

// readyEnv builds the all-pass two-resource environment.
func readyEnv() *fakeEnv {
	return &fakeEnv{
		stat: map[string]FileStatus{
			"/m/ckpt.safetensors":   {Exists: true, Regular: true},
			"/m/detail.safetensors": {Exists: true, Regular: true},
		},
		hashes: map[string]string{
			"/m/ckpt.safetensors":   hashCKPT,
			"/m/detail.safetensors": hashLORA,
		},
		versions: map[int]VersionMetadata{100: checkpointVersion(), 200: loraVersion()},
		byHash:   map[string]int{hashCKPT: 100, hashLORA: 200},
	}
}

// mustRequests parses refs or fails the test.
func mustRequests(t *testing.T, refs ...string) []Request {
	t.Helper()
	reqs, err := ParseRequests(refs)
	if err != nil {
		t.Fatalf("ParseRequests(%v): %v", refs, err)
	}
	return reqs
}

// mustResolve runs Resolve with the default policy or fails the test.
func mustResolve(t *testing.T, reqs []Request, env Environment) Report {
	t.Helper()
	rep, err := Resolve(context.Background(), reqs, DefaultPolicy(), env)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return rep
}

// findingByCode returns the first finding with code.
func findingByCode(rep Report, code string) (Finding, bool) {
	for _, f := range rep.Findings {
		if f.Code == code {
			return f, true
		}
	}
	return Finding{}, false
}

// findingCodes lists all finding codes in order.
func findingCodes(rep Report) []string {
	codes := make([]string, len(rep.Findings))
	for i, f := range rep.Findings {
		codes[i] = f.Code
	}
	return codes
}

// TestParseRef_ApprovedForms pins the four accepted reference forms and
// their normalized request fields.
func TestParseRef_ApprovedForms(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		want Request
	}{
		{
			"relative path",
			"./models/foo.safetensors",
			Request{Kind: RequestPath, Ref: "./models/foo.safetensors", Path: "./models/foo.safetensors"},
		},
		{
			"windows absolute path with spaces",
			`C:\My Models\foo.safetensors`,
			Request{Kind: RequestPath, Ref: `C:\My Models\foo.safetensors`, Path: `C:\My Models\foo.safetensors`},
		},
		{
			"bare filename with extension",
			"foo.safetensors",
			Request{Kind: RequestPath, Ref: "foo.safetensors", Path: "foo.safetensors"},
		},
		{
			"full sha256 lowercase normalized",
			strings.ToLower(hashCKPT),
			Request{Kind: RequestSHA256, Ref: strings.ToLower(hashCKPT), SHA256: hashCKPT},
		},
		{
			"air with exact version",
			"urn:air:anima:checkpoint:civitai:2458426@3263843",
			Request{Kind: RequestAirVersion, Ref: "urn:air:anima:checkpoint:civitai:2458426@3263843", VersionID: 3263843, AIR: "urn:air:anima:checkpoint:civitai:2458426@3263843"},
		},
		{
			"air with file id and format suffix",
			"urn:air:anima:checkpoint:civitai:2458426@3263843+3147349.safetensors",
			Request{Kind: RequestAirVersion, Ref: "urn:air:anima:checkpoint:civitai:2458426@3263843+3147349.safetensors", VersionID: 3263843, AIR: "urn:air:anima:checkpoint:civitai:2458426@3263843+3147349.safetensors"},
		},
		{
			"version id",
			"version:3263843",
			Request{Kind: RequestVersionID, Ref: "version:3263843", VersionID: 3263843},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRef(3, tc.ref)
			if err != nil {
				t.Fatalf("ParseRef(%q): %v", tc.ref, err)
			}
			tc.want.InputIndex = 3
			if got != tc.want {
				t.Errorf("ParseRef(%q) = %+v, want %+v", tc.ref, got, tc.want)
			}
		})
	}
}

// TestParseRef_RejectsUsageForms pins AC6: unapproved forms fail before
// any I/O as usage errors.
func TestParseRef_RejectsUsageForms(t *testing.T) {
	cases := []struct {
		name string
		ref  string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"bare numeric id", "2458426"},
		{"zero", "0"},
		{"display name", "anima"},
		{"name with spaces", "Anima Turbo"},
		{"autov2 short hash", "FBA1195327"},
		{"63-char hash", strings.Repeat("A", 63)},
		{"65-char hash", strings.Repeat("A", 65)},
		{"64 chars with non-hex", strings.Repeat("G", 64)},
		{"air without version", "urn:air:anima:checkpoint:civitai:2458426"},
		{"air with empty version", "urn:air:anima:checkpoint:civitai:2458426@"},
		{"air with non-numeric version", "urn:air:anima:checkpoint:civitai:2458426@latest"},
		{"non-air urn", "urn:foo:bar"},
		{"version zero", "version:0"},
		{"version non-numeric", "version:abc"},
		{"version leading zeros", "version:007"},
		{"version negative", "version:-1"},
		{"version overflow", "version:999999999999999999999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseRef(0, tc.ref); err == nil {
				t.Fatalf("ParseRef(%q) must be rejected", tc.ref)
			} else {
				var ue *UsageError
				if !errors.As(err, &ue) {
					t.Fatalf("ParseRef(%q) error = %T, want *UsageError", tc.ref, err)
				}
			}
		})
	}
}

// TestParseRequests_DedupeAndOrder pins idempotent exact duplicates and
// first-appearance ordering.
func TestParseRequests_DedupeAndOrder(t *testing.T) {
	reqs := mustRequests(t, "version:200", "./a.pt", "version:200", "version:100")
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3 (exact duplicate dropped)", len(reqs))
	}
	want := []RequestKind{RequestVersionID, RequestPath, RequestVersionID}
	for i, r := range reqs {
		if r.InputIndex != i || r.Kind != want[i] {
			t.Errorf("request %d = %+v, want kind %q at index %d", i, r, want[i], i)
		}
	}
	if _, err := ParseRequests(nil); err == nil {
		t.Errorf("empty set must be a usage error")
	}
}

// TestLooksLikePath pins the pure path-shape classifier used to reject
// bare names and ids without touching the filesystem.
func TestLooksLikePath(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{"./foo", true},
		{"../foo", true},
		{`C:\models\foo.pt`, true},
		{"/models/foo", true},
		{"foo.safetensors", true},
		{"foo", false},
		{"anima", false},
		{"2458426", false},
		{"FBA1195327", false},
	}
	for _, tc := range cases {
		if got := LooksLikePath(tc.ref); got != tc.want {
			t.Errorf("LooksLikePath(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}

// TestResolve_Ready pins AC2: two explicit paths, exact hashes, one
// checkpoint anchor, and an exact-base dependent produce READY with full
// identity, AIR, local state, and the compatible rule id.
func TestResolve_Ready(t *testing.T) {
	env := readyEnv()
	rep := mustResolve(t, mustRequests(t, "/m/ckpt.safetensors", "/m/detail.safetensors"), env.env())
	if !rep.Ready {
		t.Fatalf("expected READY, findings: %v", findingCodes(rep))
	}
	if rep.SchemaVersion != 1 || rep.PolicyVersion != PolicyVersionV1 {
		t.Errorf("report header = schema %d policy %q", rep.SchemaVersion, rep.PolicyVersion)
	}
	if rep.Summary.ResourceCount != 2 || rep.Summary.BlockingFindingCount != 0 || len(rep.Findings) != 0 {
		t.Errorf("summary = %+v findings = %v", rep.Summary, findingCodes(rep))
	}
	if len(rep.Resources) != 2 {
		t.Fatalf("resources = %d", len(rep.Resources))
	}
	ckpt := rep.Resources[0]
	if ckpt.Resolution != ResolutionResolved || ckpt.Local.State != LocalPresent {
		t.Errorf("checkpoint state = %+v", ckpt)
	}
	if ckpt.Identity.ModelID != 10 || ckpt.Identity.VersionID != 100 || ckpt.Identity.FileID != 1000 {
		t.Errorf("checkpoint identity = %+v", ckpt.Identity)
	}
	if ckpt.Identity.SHA256 != hashCKPT || ckpt.Local.MatchedFileID != 1000 || ckpt.Local.ComputedSHA256 != hashCKPT {
		t.Errorf("checkpoint hash/file evidence = %+v", ckpt.Local)
	}
	if ckpt.Identity.AIR != "urn:air:anima:checkpoint:civitai:10@100" {
		t.Errorf("checkpoint AIR = %q", ckpt.Identity.AIR)
	}
	lora := rep.Resources[1]
	if lora.Identity.ModelType != "LORA" || lora.Local.State != LocalPresent {
		t.Errorf("lora resource = %+v", lora)
	}
	if len(rep.Compatibility) != 1 {
		t.Fatalf("compatibility = %+v", rep.Compatibility)
	}
	c := rep.Compatibility[0]
	if c.LeftIndex != 0 || c.RightIndex != 1 || c.Status != StatusCompatible || c.RuleID != RuleCompatibleBaseModel {
		t.Errorf("compatibility = %+v", c)
	}
	if c.Left.BaseModel != "Anima" || c.Right.BaseModel != "Anima" {
		t.Errorf("compatibility base metadata = %+v / %+v", c.Left, c.Right)
	}
	// The environment is queried once per distinct content hash.
	if len(env.hashCalls) != 2 {
		t.Errorf("hash lookups = %v, want 2", env.hashCalls)
	}
}

// TestResolve_NonPrimaryFile pins AC3: a non-primary file is bound by its
// exact hash, never by the primary flag.
func TestResolve_NonPrimaryFile(t *testing.T) {
	v := mkVersion(10, 100, "ckpt-v1", "Anima", "Checkpoint", "Anima", "Anima", "",
		mkFile(1000, "primary.safetensors", "Model", true, hashCKPT),
		mkFile(1001, "secondary.safetensors", "Model", false, hashLORA))
	env := &fakeEnv{
		stat:     map[string]FileStatus{"/m/secondary.safetensors": {Exists: true, Regular: true}},
		hashes:   map[string]string{"/m/secondary.safetensors": hashLORA},
		versions: map[int]VersionMetadata{100: v},
		byHash:   map[string]int{hashLORA: 100},
	}
	rep := mustResolve(t, mustRequests(t, "/m/secondary.safetensors"), env.env())
	if !rep.Ready {
		t.Fatalf("expected READY, findings: %v", findingCodes(rep))
	}
	r := rep.Resources[0]
	if r.Identity.FileID != 1001 || r.Local.MatchedFileID != 1001 {
		t.Errorf("non-primary file must bind id 1001, got %+v", r.Identity)
	}
	if r.Names.File != "secondary.safetensors" {
		t.Errorf("file name = %q", r.Names.File)
	}
	if len(r.VisibleFiles) != 2 || r.VisibleFiles[0].FileID != 1000 {
		t.Errorf("visible files must stay ordered and complete: %+v", r.VisibleFiles)
	}
}

// TestResolve_SameNameDifferentBytes pins AC3's negative half: identical
// display names never establish presence; unregistered content stays
// unresolved and not present.
func TestResolve_SameNameDifferentBytes(t *testing.T) {
	env := readyEnv()
	env.hashes["/m/ckpt.safetensors"] = hashBAD
	rep := mustResolve(t, mustRequests(t, "/m/ckpt.safetensors"), env.env())
	if rep.Ready {
		t.Fatalf("unregistered bytes must not be ready")
	}
	r := rep.Resources[0]
	if r.Local.State == LocalPresent {
		t.Errorf("same-named different bytes must not be present: %+v", r.Local)
	}
	if f, ok := findingByCode(rep, CodeUnresolvedResource); !ok {
		t.Errorf("expected unresolved_resource, got %v", findingCodes(rep))
	} else if f.Evidence.Path != "/m/ckpt.safetensors" {
		t.Errorf("finding evidence path = %q", f.Evidence.Path)
	}
}

// TestResolve_RemoteOnlyCannotReady pins AC4: bare version, AIR, and full
// hash refs expose canonical metadata but stay locally unverified and
// cannot make a set ready.
func TestResolve_RemoteOnlyCannotReady(t *testing.T) {
	env := readyEnv()
	rep := mustResolve(t, mustRequests(t,
		"version:100",
		"urn:air:anima:lora:civitai:20@200",
		hashCKPT,
	), env.env())
	if rep.Ready {
		t.Fatalf("remote-only refs must never be ready")
	}
	for i, r := range rep.Resources {
		if r.Resolution != ResolutionResolved {
			t.Errorf("resource %d must resolve canonical identity: %+v", i, r)
		}
		if r.Local.State != LocalUnverified {
			t.Errorf("resource %d local state = %q, want unverified", i, r.Local.State)
		}
	}
	if n := countCodes(rep.Findings, CodeLocalPresenceUnverified); n != 3 {
		t.Errorf("local_presence_unverified count = %d, want 3 (%v)", n, findingCodes(rep))
	}
	if len(rep.Compatibility) != 1 || rep.Compatibility[0].Status != StatusCompatible {
		t.Errorf("identity-only compatibility should still evaluate: %+v", rep.Compatibility)
	}
	// The AIR request keeps the registry version's AIR metadata.
	if rep.Resources[1].Identity.AIR != "urn:air:anima:lora:civitai:20@200" {
		t.Errorf("AIR metadata = %q", rep.Resources[1].Identity.AIR)
	}
	// The full-hash ref binds file identity but stays unverified locally.
	if rep.Resources[2].Identity.FileID != 1000 {
		t.Errorf("hash ref must bind the visible file: %+v", rep.Resources[2].Identity)
	}
}

// TestResolve_MissingPath pins the completed missing-resource report.
func TestResolve_MissingPath(t *testing.T) {
	env := readyEnv()
	rep := mustResolve(t, mustRequests(t, "/m/ckpt.safetensors", "/m/nope.safetensors"), env.env())
	if rep.Ready {
		t.Fatalf("missing path must not be ready")
	}
	r := rep.Resources[1]
	if r.Local.State != LocalMissing || r.Local.Path != "/m/nope.safetensors" {
		t.Errorf("missing resource local = %+v", r.Local)
	}
	f, ok := findingByCode(rep, CodeMissingResource)
	if !ok || f.Evidence.Path != "/m/nope.safetensors" {
		t.Errorf("missing_resource finding = %+v (found %v)", f, ok)
	}
	// The valid checkpoint keeps its identity evidence (one bad member does
	// not erase the others).
	if rep.Resources[0].Identity.VersionID != 100 {
		t.Errorf("valid member identity lost: %+v", rep.Resources[0])
	}
}

// TestResolve_UnresolvedRegistry pins AC7's registry-404 half.
func TestResolve_UnresolvedRegistry(t *testing.T) {
	env := readyEnv()
	rep := mustResolve(t, mustRequests(t, "version:999", hashBAD), env.env())
	if rep.Ready {
		t.Fatalf("unresolved refs must not be ready")
	}
	for i, r := range rep.Resources {
		if r.Resolution != ResolutionUnresolved {
			t.Errorf("resource %d resolution = %q, want unresolved", i, r.Resolution)
		}
	}
	if n := countCodes(rep.Findings, CodeUnresolvedResource); n != 2 {
		t.Errorf("unresolved_resource count = %d (%v)", n, findingCodes(rep))
	}
}

// TestResolve_WrongVersion pins AC5/R5: content resolving to a different
// version than the expected one is wrong_version with both identities.
func TestResolve_WrongVersion(t *testing.T) {
	env := readyEnv()
	req := Request{
		InputIndex:        0,
		Kind:              RequestPath,
		Ref:               "/m/ckpt.safetensors",
		Path:              "/m/ckpt.safetensors",
		ExpectedVersionID: 100, // expected the checkpoint, content is the LORA
	}
	// Route the checkpoint path's bytes to the LORA version to model a
	// content/expected-version disagreement.
	env.hashes["/m/ckpt.safetensors"] = hashLORA
	env.byHash[hashLORA] = 200
	rep := mustResolve(t, []Request{req}, env.env())
	if rep.Ready {
		t.Fatalf("wrong version must not be ready")
	}
	r := rep.Resources[0]
	if r.Resolution != ResolutionMismatch || r.Local.State != LocalMismatch {
		t.Errorf("wrong-version state = %+v", r)
	}
	if r.Local.ComputedSHA256 != hashLORA {
		t.Errorf("computed hash = %q", r.Local.ComputedSHA256)
	}
	f, ok := findingByCode(rep, CodeWrongVersion)
	if !ok {
		t.Fatalf("expected wrong_version, got %v", findingCodes(rep))
	}
	if f.Evidence.Expected == nil || f.Evidence.Expected.VersionID != 100 {
		t.Errorf("expected identity = %+v", f.Evidence.Expected)
	}
	if f.Evidence.Actual == nil || f.Evidence.Actual.VersionID != 200 || f.Evidence.Actual.ModelID != 20 {
		t.Errorf("actual identity = %+v", f.Evidence.Actual)
	}
	if f.Evidence.Actual.SHA256 != hashLORA {
		t.Errorf("actual hash evidence = %+v", f.Evidence.Actual)
	}
	if f.Evidence.Expected.ModelID != 10 {
		t.Errorf("expected model id should come from the expected version, got %+v", f.Evidence.Expected)
	}
}

// TestResolve_CompatibilityTable pins AC8: one compatible tuple, one
// reviewed known conflict, one uncovered/unknown tuple, plus the
// baseModelType and missing-metadata handling.
func TestResolve_CompatibilityTable(t *testing.T) {
	cases := []struct {
		name         string
		ckptBase     string
		ckptType     string
		depBase      string
		depType      string
		depModelType string
		wantStatus   CompatibilityStatus
		wantRule     string
		wantFinding  string
	}{
		{
			name: "exact compatible", ckptBase: "Anima", ckptType: "Anima",
			depBase: "Anima", depType: "Anima", depModelType: "LORA",
			wantStatus: StatusCompatible, wantRule: RuleCompatibleBaseModel,
		},
		{
			name: "known base conflict", ckptBase: "Anima", ckptType: "Anima",
			depBase: "SDXL 1.0", depType: "Anima", depModelType: "LORA",
			wantStatus: StatusConflict, wantRule: RuleConflictBaseModel, wantFinding: CodeBaseModelConflict,
		},
		{
			name: "unknown missing base", ckptBase: "Anima", ckptType: "Anima",
			depBase: "", depType: "Anima", depModelType: "LORA",
			wantStatus: StatusUnknown, wantRule: RuleUnknownMissingBaseModel, wantFinding: CodeUnknownCompatibility,
		},
		{
			name: "unknown model type", ckptBase: "Anima", ckptType: "Anima",
			depBase: "Anima", depType: "Anima", depModelType: "Other",
			wantStatus: StatusUnknown, wantRule: RuleUnknownModelType, wantFinding: CodeUnknownCompatibility,
		},
		{
			name: "known type conflict", ckptBase: "Anima", ckptType: "Anima",
			depBase: "Anima", depType: "Illustrious", depModelType: "LORA",
			wantStatus: StatusConflict, wantRule: RuleConflictBaseModelType, wantFinding: CodeBaseModelConflict,
		},
		{
			name: "one-sided baseModelType is compatible", ckptBase: "Anima", ckptType: "",
			depBase: "Anima", depType: "Anima", depModelType: "LORA",
			wantStatus: StatusCompatible, wantRule: RuleCompatibleBaseModel,
		},
		{
			name: "missing anchor base is unknown", ckptBase: "", ckptType: "",
			depBase: "Anima", depType: "Anima", depModelType: "LORA",
			wantStatus: StatusUnknown, wantRule: RuleUnknownMissingBaseModel, wantFinding: CodeUnknownCompatibility,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ckpt := mkVersion(10, 100, "c", "Anima", "Checkpoint", tc.ckptBase, tc.ckptType, "")
			dep := mkVersion(20, 200, "d", "Dep", tc.depModelType, tc.depBase, tc.depType, "")
			env := &fakeEnv{
				versions: map[int]VersionMetadata{100: ckpt, 200: dep},
			}
			rep := mustResolve(t, mustRequests(t, "version:100", "version:200"), env.env())
			if rep.Ready {
				t.Fatalf("remote-only set must not be ready")
			}
			if len(rep.Compatibility) != 1 {
				t.Fatalf("compatibility = %+v", rep.Compatibility)
			}
			c := rep.Compatibility[0]
			if c.Status != tc.wantStatus || c.RuleID != tc.wantRule {
				t.Errorf("compatibility = status %q rule %q, want %q / %q", c.Status, c.RuleID, tc.wantStatus, tc.wantRule)
			}
			if c.LeftIndex != 0 || c.RightIndex != 1 {
				t.Errorf("pair indexes = %d/%d", c.LeftIndex, c.RightIndex)
			}
			if c.Left.BaseModel != tc.ckptBase || c.Right.BaseModel != tc.depBase {
				t.Errorf("left/right base evidence = %q / %q", c.Left.BaseModel, c.Right.BaseModel)
			}
			if tc.wantFinding == "" {
				if _, ok := findingByCode(rep, CodeUnknownCompatibility); ok {
					t.Errorf("compatible tuple must not emit a compatibility finding")
				}
				if _, ok := findingByCode(rep, CodeBaseModelConflict); ok {
					t.Errorf("compatible tuple must not emit a conflict finding")
				}
				return
			}
			f, ok := findingByCode(rep, tc.wantFinding)
			if !ok {
				t.Fatalf("expected finding %q, got %v", tc.wantFinding, findingCodes(rep))
			}
			if f.RuleID != tc.wantRule {
				t.Errorf("finding rule id = %q, want %q", f.RuleID, tc.wantRule)
			}
			if f.Evidence.Left == nil || f.Evidence.Right == nil {
				t.Errorf("finding must carry both sides: %+v", f.Evidence)
			}
			// Known conflicts and unknowns cannot be collapsed: exactly one
			// of the two codes appears.
			if tc.wantFinding == CodeBaseModelConflict {
				if _, ok := findingByCode(rep, CodeUnknownCompatibility); ok {
					t.Errorf("known conflict must not also be unknown")
				}
			}
		})
	}
}

// TestResolve_AnchorRules pins D3: missing, conflicting, and idempotent
// duplicate anchors.
func TestResolve_AnchorRules(t *testing.T) {
	t.Run("no checkpoint", func(t *testing.T) {
		env := &fakeEnv{versions: map[int]VersionMetadata{200: loraVersion()}}
		rep := mustResolve(t, mustRequests(t, "version:200"), env.env())
		if _, ok := findingByCode(rep, CodeMissingAnchor); !ok {
			t.Errorf("expected missing_anchor, got %v", findingCodes(rep))
		}
		if len(rep.Compatibility) != 0 {
			t.Errorf("no anchor means no compatibility pairs: %+v", rep.Compatibility)
		}
	})
	t.Run("two distinct checkpoints", func(t *testing.T) {
		other := mkVersion(11, 101, "ckpt-v2", "Anima", "Checkpoint", "Anima", "Anima", "",
			mkFile(1001, "other.safetensors", "Model", true, hashBAD))
		env := &fakeEnv{versions: map[int]VersionMetadata{100: checkpointVersion(), 101: other}}
		rep := mustResolve(t, mustRequests(t, "version:100", "version:101"), env.env())
		f, ok := findingByCode(rep, CodeConflictingAnchor)
		if !ok {
			t.Fatalf("expected conflicting_anchor, got %v", findingCodes(rep))
		}
		if len(f.ResourceIndexes) != 2 || f.ResourceIndexes[0] != 0 || f.ResourceIndexes[1] != 1 {
			t.Errorf("conflicting indexes = %v", f.ResourceIndexes)
		}
	})
	t.Run("identical duplicate checkpoint is idempotent", func(t *testing.T) {
		env := readyEnv()
		env.stat["/m/copy.safetensors"] = FileStatus{Exists: true, Regular: true}
		env.hashes["/m/copy.safetensors"] = hashCKPT
		rep := mustResolve(t, mustRequests(t, "/m/ckpt.safetensors", "/m/copy.safetensors"), env.env())
		if !rep.Ready {
			t.Fatalf("identical canonical checkpoint must be idempotent, findings: %v", findingCodes(rep))
		}
		if _, ok := findingByCode(rep, CodeConflictingAnchor); ok {
			t.Errorf("identical checkpoint must not conflict")
		}
		// The identical GET is deduplicated in memory.
		if len(env.hashCalls) != 1 {
			t.Errorf("hash lookups = %v, want 1 (deduped)", env.hashCalls)
		}
	})
}

// TestResolve_FileIdentityEdges pins the no-fallback file identity rules.
func TestResolve_FileIdentityEdges(t *testing.T) {
	t.Run("ambiguous duplicate hashes", func(t *testing.T) {
		v := mkVersion(10, 100, "c", "Anima", "Checkpoint", "Anima", "Anima", "",
			mkFile(1000, "a.safetensors", "Model", true, hashCKPT),
			mkFile(1001, "b.safetensors", "Model", false, hashCKPT))
		env := &fakeEnv{
			stat:     map[string]FileStatus{"/m/a.safetensors": {Exists: true, Regular: true}},
			hashes:   map[string]string{"/m/a.safetensors": hashCKPT},
			versions: map[int]VersionMetadata{100: v},
			byHash:   map[string]int{hashCKPT: 100},
		}
		rep := mustResolve(t, mustRequests(t, "/m/a.safetensors"), env.env())
		r := rep.Resources[0]
		if r.Resolution != ResolutionAmbiguousFile || r.Identity.FileID != 0 || r.Local.MatchedFileID != 0 {
			t.Errorf("ambiguous file must not bind an id: %+v", r)
		}
		if _, ok := findingByCode(rep, CodeAmbiguousFileIdentity); !ok {
			t.Errorf("expected ambiguous_file_identity, got %v", findingCodes(rep))
		}
	})
	t.Run("no visible hash evidence", func(t *testing.T) {
		v := mkVersion(10, 100, "c", "Anima", "Checkpoint", "Anima", "Anima", "",
			mkFile(1000, "a.safetensors", "Model", true, ""))
		env := &fakeEnv{
			stat:     map[string]FileStatus{"/m/a.safetensors": {Exists: true, Regular: true}},
			hashes:   map[string]string{"/m/a.safetensors": hashCKPT},
			versions: map[int]VersionMetadata{100: v},
			byHash:   map[string]int{hashCKPT: 100},
		}
		rep := mustResolve(t, mustRequests(t, "/m/a.safetensors"), env.env())
		if _, ok := findingByCode(rep, CodeUnresolvedFileIdentity); !ok {
			t.Errorf("expected unresolved_file_identity, got %v", findingCodes(rep))
		}
	})
	t.Run("visible hash mismatch", func(t *testing.T) {
		v := mkVersion(10, 100, "c", "Anima", "Checkpoint", "Anima", "Anima", "",
			mkFile(1000, "a.safetensors", "Model", true, hashVAE))
		env := &fakeEnv{
			stat:     map[string]FileStatus{"/m/a.safetensors": {Exists: true, Regular: true}},
			hashes:   map[string]string{"/m/a.safetensors": hashCKPT},
			versions: map[int]VersionMetadata{100: v},
			byHash:   map[string]int{hashCKPT: 100},
		}
		rep := mustResolve(t, mustRequests(t, "/m/a.safetensors"), env.env())
		if _, ok := findingByCode(rep, CodeLocalHashMismatch); !ok {
			t.Errorf("expected local_hash_mismatch, got %v", findingCodes(rep))
		}
		if rep.Resources[0].Local.State == LocalPresent {
			t.Errorf("hash mismatch must not be present")
		}
	})
}

// TestResolve_MixedFailuresRetainFacts pins AC9: one bad member does not
// erase valid member details, and no numeric score is emitted.
func TestResolve_MixedFailuresRetainFacts(t *testing.T) {
	env := readyEnv()
	rep := mustResolve(t, mustRequests(t,
		"/m/ckpt.safetensors",
		"version:999", // unresolved dependent
		"/m/detail.safetensors",
	), env.env())
	if rep.Ready {
		t.Fatalf("mixed failures must not be ready")
	}
	if rep.Resources[0].Identity.VersionID != 100 || rep.Resources[2].Identity.VersionID != 200 {
		t.Errorf("valid member facts lost: %+v", rep.Resources)
	}
	if rep.Summary.BlockingFindingCount != len(rep.Findings) || len(rep.Findings) == 0 {
		t.Errorf("summary/finding mismatch: %+v %v", rep.Summary, findingCodes(rep))
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	for _, banned := range []string{"score", "confidence", "percentage", "rank"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("report must not contain %q: %s", banned, b)
		}
	}
}

// TestResolve_Deterministic pins byte-identical repeated reports.
func TestResolve_Deterministic(t *testing.T) {
	run := func() []byte {
		rep := mustResolve(t, mustRequests(t, "/m/ckpt.safetensors", "/m/detail.safetensors", "version:999"), readyEnv().env())
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	first := run()
	second := run()
	if string(first) != string(second) {
		t.Errorf("repeated evaluations must be byte-identical:\n%s\n---\n%s", first, second)
	}
	// Empty slices must serialize as [] (not null) for machine consumers:
	// a single-checkpoint set has no compatibility pairs.
	single := mustResolve(t, mustRequests(t, "/m/ckpt.safetensors"), readyEnv().env())
	b, err := json.Marshal(single)
	if err != nil {
		t.Fatalf("marshal single: %v", err)
	}
	if !strings.Contains(string(b), `"compatibility":[]`) {
		t.Errorf("empty compatibility must serialize as []:\n%s", b)
	}
}

// TestResolve_OperationalErrors pins R11: local I/O and registry failures
// are operational errors with no report, never a not-ready verdict.
func TestResolve_OperationalErrors(t *testing.T) {
	t.Run("stat failure", func(t *testing.T) {
		env := readyEnv()
		env.statErrs = map[string]error{"/m/ckpt.safetensors": errors.New("access denied")}
		if _, err := Resolve(context.Background(), mustRequests(t, "/m/ckpt.safetensors"), DefaultPolicy(), env.env()); err == nil {
			t.Fatalf("stat failure must be operational")
		}
	})
	t.Run("hash failure", func(t *testing.T) {
		env := readyEnv()
		env.hashErrs = map[string]error{"/m/ckpt.safetensors": errors.New("read error")}
		if _, err := Resolve(context.Background(), mustRequests(t, "/m/ckpt.safetensors"), DefaultPolicy(), env.env()); err == nil {
			t.Fatalf("hash failure must be operational")
		}
	})
	t.Run("registry network failure", func(t *testing.T) {
		env := readyEnv()
		network := errors.New("network down")
		env.byHashErr = map[string]error{hashCKPT: network}
		_, err := Resolve(context.Background(), mustRequests(t, "/m/ckpt.safetensors"), DefaultPolicy(), env.env())
		if err == nil || !errors.Is(err, network) {
			t.Fatalf("registry failure must propagate: %v", err)
		}
	})
	t.Run("directory is usage error", func(t *testing.T) {
		env := &fakeEnv{stat: map[string]FileStatus{"/m/dir": {Exists: true, Regular: false}}}
		_, err := Resolve(context.Background(), mustRequests(t, "/m/dir"), DefaultPolicy(), env.env())
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("directory error = %T, want *UsageError", err)
		}
	})
	t.Run("empty set", func(t *testing.T) {
		_, err := Resolve(context.Background(), nil, DefaultPolicy(), readyEnv().env())
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("empty set error = %T, want *UsageError", err)
		}
	})
	t.Run("incomplete environment", func(t *testing.T) {
		if _, err := Resolve(context.Background(), mustRequests(t, "version:100"), DefaultPolicy(), Environment{}); err == nil {
			t.Fatalf("incomplete environment must fail")
		}
	})
}

// TestResolve_InvalidRequestStructs pins the programmatic validation
// guard: combinations the CLI grammar cannot produce are still rejected.
func TestResolve_InvalidRequestStructs(t *testing.T) {
	env := readyEnv().env()
	cases := []struct {
		name string
		req  Request
	}{
		{"path with version", Request{Kind: RequestPath, Ref: "x", Path: "/m/x", VersionID: 1}},
		{"hash with path", Request{Kind: RequestSHA256, Ref: "x", SHA256: hashCKPT, Path: "/m/x"}},
		{"air with path", Request{Kind: RequestAirVersion, Ref: "x", VersionID: 1, AIR: "urn:air:a:b:c:1@1", Path: "/m/x"}},
		{"version with air", Request{Kind: RequestVersionID, Ref: "x", VersionID: 1, AIR: "urn:air:a:b:c:1@1"}},
		{"short hash", Request{Kind: RequestSHA256, Ref: "x", SHA256: "ABCD"}},
		{"unknown kind", Request{Kind: "name", Ref: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ue *UsageError
			if _, err := Resolve(context.Background(), []Request{tc.req}, DefaultPolicy(), env); !errors.As(err, &ue) {
				t.Fatalf("error = %T (%v), want *UsageError", err, err)
			}
		})
	}
}

// countCodes counts findings with a code.
func countCodes(findings []Finding, code string) int {
	n := 0
	for _, f := range findings {
		if f.Code == code {
			n++
		}
	}
	return n
}

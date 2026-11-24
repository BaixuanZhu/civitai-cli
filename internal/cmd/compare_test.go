package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"civitai-cli/internal/api"
)

// compareRouteCounter records request paths so tests can prove the command
// hits only the expected official endpoints, in the expected order.
type compareRouteCounter struct {
	mu    sync.Mutex
	paths []string
}

// addPath appends one observed request path.
func (c *compareRouteCounter) addPath(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paths = append(c.paths, p)
}

// snapshot returns a copy of the observed paths.
func (c *compareRouteCounter) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.paths...)
}

// count returns how many times path was observed.
func (c *compareRouteCounter) count(path string) int {
	n := 0
	for _, p := range c.snapshot() {
		if p == path {
			n++
		}
	}
	return n
}

// compareRoutingHandler serves inline JSON bodies by path and records each
// request; unmapped paths get the API 404 shape.
func compareRoutingHandler(counter *compareRouteCounter, routes map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		counter.addPath(r.URL.Path)
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Write([]byte(body))
	}
}

// compareRunCmd mirrors execute's flag handling without os.Exit.
func compareRunCmd(t *testing.T, env *testEnv, args ...string) error {
	t.Helper()
	var o compareOptions
	pos := parseForTest(t, &env.g, o.register, args)
	return runCompare(context.Background(), &env.g, pos, &o)
}

func TestCompareValidationBeforeHTTP(t *testing.T) {
	counter := &compareRouteCounter{}
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		counter.addPath(r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	cases := [][]string{
		{},
		{"models"},
		{"faces", "1", "2"},
		{"models", "1"},
		{"models", "1", "2", "3", "4", "5", "6"},
		{"models", "1", "1"},
		{"models", "0", "2"},
		{"models", "abc", "2"},
		{"models", "0x10", "2"},
		{"models", "999999999999999999999", "2"},
		{"Models", "1", "2"},
		{"versions", "1", "2", "--diff", "--json"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			env.buf.Reset()
			err := compareRunCmd(t, env, args...)
			if err == nil {
				t.Fatalf("args %v must fail", args)
			}
			if exitCodeFor(err, false) != 1 {
				t.Fatalf("usage error must map to exit 1, got %d (%v)", exitCodeFor(err, false), err)
			}
			if env.buf.Len() != 0 {
				t.Fatalf("usage error must write no stdout: %q", env.buf.String())
			}
		})
	}
	// A negative ID reaches the same validator when it bypasses flag parsing
	// (the real CLI rejects a leading-dash token as an unknown flag, also
	// usage-class exit 1 before any request).
	env.buf.Reset()
	err := runCompare(context.Background(), &env.g, []string{"models", "-3", "2"}, &compareOptions{})
	if err == nil || !strings.Contains(err.Error(), "invalid model id") {
		t.Fatalf("negative id must be rejected, got %v", err)
	}
	if exitCodeFor(err, false) != 1 || env.buf.Len() != 0 {
		t.Fatalf("negative id must be a usage-class failure with empty stdout")
	}
	if got := counter.snapshot(); len(got) != 0 {
		t.Fatalf("validation failures must make no requests, got %v", got)
	}
}

func TestCompareModelRoutesAndInputOrder(t *testing.T) {
	counter := &compareRouteCounter{}
	env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
		"/models/1": `{"id":1,"name":"One","type":"Checkpoint","creator":{"username":"a"},"modelVersions":[]}`,
		"/models/2": `{"id":2,"name":"Two","type":"LORA","creator":{"username":"b"},"modelVersions":[]}`,
	}))
	if err := compareRunCmd(t, env, "models", "2", "1", "--json"); err != nil {
		t.Fatalf("runCompare: %v", err)
	}
	if got := counter.snapshot(); len(got) != 2 || got[0] != "/models/2" || got[1] != "/models/1" {
		t.Fatalf("models must be fetched in input order via /models/{id}, got %v", got)
	}
	for _, p := range counter.snapshot() {
		if !strings.HasPrefix(p, "/models/") && !strings.HasPrefix(p, "/model-versions/") {
			t.Fatalf("unexpected side-effect request %q", p)
		}
	}
	var doc struct {
		Kind     string `json:"kind"`
		Subjects []struct {
			ModelID int64 `json:"modelId"`
		} `json:"subjects"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if doc.Kind != "model" || len(doc.Subjects) != 2 || doc.Subjects[0].ModelID != 2 || doc.Subjects[1].ModelID != 1 {
		t.Fatalf("subject order/identity wrong: %+v", doc)
	}
}

func TestCompareVersionParentDeduplication(t *testing.T) {
	counter := &compareRouteCounter{}
	env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
		"/model-versions/11": `{"id":11,"modelId":7,"name":"v11","baseModel":"Anima","trainedWords":[]}`,
		"/model-versions/12": `{"id":12,"modelId":7,"name":"v12","baseModel":"Anima","trainedWords":[]}`,
		"/models/7":          `{"id":7,"name":"Parent","type":"Checkpoint","creator":{"username":"u"}}`,
	}))
	if err := compareRunCmd(t, env, "versions", "11", "12", "--json"); err != nil {
		t.Fatalf("runCompare: %v", err)
	}
	want := []string{"/model-versions/11", "/models/7", "/model-versions/12"}
	got := counter.snapshot()
	if len(got) != len(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("requests = %v, want %v", got, want)
		}
	}
	if n := counter.count("/models/7"); n != 1 {
		t.Fatalf("shared parent fetched %d times, want 1", n)
	}
	var doc struct {
		Kind     string `json:"kind"`
		Subjects []struct {
			ModelID   int64 `json:"modelId"`
			VersionID int64 `json:"versionId"`
		} `json:"subjects"`
	}
	if err := json.Unmarshal(env.buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if doc.Kind != "version" || len(doc.Subjects) != 2 ||
		doc.Subjects[0].VersionID != 11 || doc.Subjects[1].VersionID != 12 ||
		doc.Subjects[0].ModelID != 7 || doc.Subjects[1].ModelID != 7 {
		t.Fatalf("subject order/identity wrong: %+v", doc)
	}
}

func TestCompareVersionCrossParentFetchesBothParents(t *testing.T) {
	counter := &compareRouteCounter{}
	env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
		"/model-versions/11": `{"id":11,"modelId":7,"name":"v11","baseModel":"Anima","trainedWords":[]}`,
		"/model-versions/12": `{"id":12,"modelId":8,"name":"v12","baseModel":"SDXL","trainedWords":[]}`,
		"/models/7":          `{"id":7,"name":"P7","type":"Checkpoint","creator":{"username":"u"}}`,
		"/models/8":          `{"id":8,"name":"P8","type":"LORA","creator":{"username":"v"}}`,
	}))
	if err := compareRunCmd(t, env, "versions", "11", "12", "--json"); err != nil {
		t.Fatalf("runCompare: %v", err)
	}
	if counter.count("/models/7") != 1 || counter.count("/models/8") != 1 {
		t.Fatalf("cross-parent versions must fetch each parent once: %v", counter.snapshot())
	}
	if counter.count("/model-versions/11") != 1 || counter.count("/model-versions/12") != 1 {
		t.Fatalf("selected versions must be fetched once: %v", counter.snapshot())
	}
}

func TestCompareVersionMissingOwningModelID(t *testing.T) {
	counter := &compareRouteCounter{}
	env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
		"/model-versions/11": `{"id":11,"name":"v11","baseModel":"Anima"}`,
	}))
	err := compareRunCmd(t, env, "versions", "11", "12")
	if err == nil || !strings.Contains(err.Error(), "no owning model id") {
		t.Fatalf("missing owning model id must fail, got %v", err)
	}
	if env.buf.Len() != 0 {
		t.Fatalf("failure must write no stdout: %q", env.buf.String())
	}
}

func TestCompareFetchFailuresAreAtomic(t *testing.T) {
	t.Run("selected_404", func(t *testing.T) {
		counter := &compareRouteCounter{}
		env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
			"/model-versions/11": `{"id":11,"modelId":7,"name":"v11"}`,
		}))
		err := compareRunCmd(t, env, "versions", "11", "12")
		var nfe *api.NotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("expected NotFoundError, got %v", err)
		}
		if exitCodeFor(err, false) != 1 {
			t.Fatalf("compare 404 must map to exit 1, got %d", exitCodeFor(err, false))
		}
		if env.buf.Len() != 0 {
			t.Fatalf("failure must write no stdout: %q", env.buf.String())
		}
	})
	t.Run("required_parent_404", func(t *testing.T) {
		counter := &compareRouteCounter{}
		env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
			"/model-versions/11": `{"id":11,"modelId":7,"name":"v11"}`,
			"/model-versions/12": `{"id":12,"modelId":7,"name":"v12"}`,
		}))
		err := compareRunCmd(t, env, "versions", "11", "12")
		var nfe *api.NotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("expected parent NotFoundError, got %v", err)
		}
		if !strings.Contains(err.Error(), "parent model 7 for version 11") {
			t.Fatalf("parent error must carry subject context: %v", err)
		}
		if env.buf.Len() != 0 {
			t.Fatalf("failure must write no stdout: %q", env.buf.String())
		}
	})
	t.Run("decode_error", func(t *testing.T) {
		counter := &compareRouteCounter{}
		env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
			"/models/1": `{"id":`,
		}))
		err := compareRunCmd(t, env, "models", "1", "2")
		if err == nil || !strings.Contains(err.Error(), "compare: model 1") {
			t.Fatalf("decode failure must be wrapped with subject context, got %v", err)
		}
		if env.buf.Len() != 0 {
			t.Fatalf("failure must write no stdout: %q", env.buf.String())
		}
	})
	t.Run("network", func(t *testing.T) {
		counter := &compareRouteCounter{}
		env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
			counter.addPath(r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var o compareOptions
		pos := parseForTest(t, &env.g, o.register, []string{"models", "1", "2"})
		err := runCompare(ctx, &env.g, pos, &o)
		var ne *api.NetworkError
		if !errors.As(err, &ne) {
			t.Fatalf("expected NetworkError, got %v", err)
		}
		if exitCodeFor(err, false) != 2 {
			t.Fatalf("network failure must map to exit 2, got %d", exitCodeFor(err, false))
		}
		if len(counter.snapshot()) != 0 {
			t.Fatalf("canceled comparison must not issue requests: %v", counter.snapshot())
		}
		if env.buf.Len() != 0 {
			t.Fatalf("failure must write no stdout: %q", env.buf.String())
		}
	})
	t.Run("token_rejected", func(t *testing.T) {
		counter := &compareRouteCounter{}
		env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
			counter.addPath(r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Unauthorized"}`))
		})
		err := compareRunCmd(t, env, "models", "1", "2", "--token", "bad")
		var ue *api.UnauthorizedError
		if !errors.As(err, &ue) {
			t.Fatalf("expected UnauthorizedError, got %v", err)
		}
		if exitCodeFor(err, true) != 4 {
			t.Fatalf("rejected token must map to exit 4, got %d", exitCodeFor(err, true))
		}
		if env.buf.Len() != 0 {
			t.Fatalf("failure must write no stdout: %q", env.buf.String())
		}
	})
}

func TestCompareJSONDeterministicAndCurated(t *testing.T) {
	counter := &compareRouteCounter{}
	env := newTestEnv(t, compareRoutingHandler(counter, map[string]string{
		"/model-versions/11": `{"id":11,"modelId":7,"name":"v11","baseModel":"Anima","description":"SENTINEL-DESC",
			"downloadUrl":"SENTINEL-DL","trainedWords":["b","a"],
			"files":[{"id":9,"name":"f","type":"Model","downloadUrl":"SENTINEL-DL","hashes":{"SHA256":"AA","AutoV2":"a2"}}],
			"images":[{"url":"SENTINEL-IMG","hasMeta":true,"meta":{"prompt":"SENTINEL-PROMPT"}}]}`,
		"/model-versions/12": `{"id":12,"modelId":7,"name":"v12","baseModel":"Anima","trainedWords":["a","b"],
			"files":[{"id":9,"name":"f","type":"Model","hashes":{"AutoV2":"a2","SHA256":"AA"}}],
			"images":[{"url":"SENTINEL-IMG","hasMeta":true,"meta":{"prompt":"SENTINEL-PROMPT"}}]}`,
		"/models/7": `{"id":7,"name":"Parent","type":"Checkpoint","creator":{"username":"u"},"description":"SENTINEL-DESC"}`,
	}))
	if err := compareRunCmd(t, env, "versions", "11", "12", "--json"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := env.buf.String()
	env.buf.Reset()
	if err := compareRunCmd(t, env, "versions", "11", "12", "--json"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second := env.buf.String()
	if first != second {
		t.Fatalf("identical responses must produce byte-identical JSON")
	}
	for _, banned := range []string{"SENTINEL-DESC", "SENTINEL-DL", "SENTINEL-IMG", "SENTINEL-PROMPT",
		"generatedAt", `"rank"`, `"score"`, `"winner"`} {
		if strings.Contains(first, banned) {
			t.Fatalf("JSON must not contain %q:\n%s", banned, first)
		}
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(first), &doc); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	for _, key := range []string{"schemaVersion", "kind", "baseline", "subjects", "fields"} {
		if _, ok := doc[key]; !ok {
			t.Fatalf("top-level key %q missing", key)
		}
	}
	t.Logf("compare json:\n%s", first)
}

func TestCompareHumanFullOutput(t *testing.T) {
	env := newTestEnv(t, fixtureHandler(t, map[string]string{
		"/model-versions/3108589": "version_3263843.json",
		"/model-versions/3263843": "version_3263843.json",
		"/models/2458426":         "models_2458426.json",
	}))
	if err := compareRunCmd(t, env, "versions", "3108589", "3263843"); err != nil {
		t.Fatalf("runCompare: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{
		"COMPARE version", "Baseline: subject 1",
		"version 3108589", "version 3263843", "FIELDS",
		"availability", "Public", "[embeddedVersion]",
		"trainedWords", "[]",
		"files", "hash SHA256", "FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB",
		"recipeCoverage", "images 9", "meta 9",
		"modelName", "Anima", "creator", "circlestone_labs",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("full human output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "api/download") || strings.Contains(out, "masterpiece") {
		t.Fatalf("human output must not leak download URLs or prompts:\n%s", out)
	}
	t.Logf("human compare output:\n%s", out)
}

func TestCompareHumanFiveSubjectsStableAndASCII(t *testing.T) {
	routes := map[string]string{}
	for id := 1; id <= 5; id++ {
		n := strconv.Itoa(id)
		routes["/models/"+n] = `{"id":` + n + `,"name":"M` + n + `","type":"Checkpoint","creator":{"username":"u"},"modelVersions":[]}`
	}
	env := newTestEnv(t, compareRoutingHandler(&compareRouteCounter{}, routes))
	if err := compareRunCmd(t, env, "models", "1", "2", "3", "4", "5"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := env.buf.String()
	env.buf.Reset()
	if err := compareRunCmd(t, env, "models", "5", "4", "3", "2", "1"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second := env.buf.String()
	if first == second {
		t.Fatalf("human output must keep input subject order")
	}
	if !strings.Contains(first, "<missing>") {
		t.Fatalf("missing values must be explicit in the human view:\n%s", first)
	}
	for _, r := range first {
		if r > 127 {
			t.Fatalf("human structural output must be ASCII-only, found %q", r)
		}
	}
	env.buf.Reset()
	if err := compareRunCmd(t, env, "models", "1", "2", "3", "4", "5"); err != nil {
		t.Fatalf("repeat run: %v", err)
	}
	if env.buf.String() != first {
		t.Fatalf("identical snapshots must produce identical human output")
	}
}

func TestCompareDiffNoObservedDifferences(t *testing.T) {
	versionBody := `{"id":1,"modelId":7,"name":"same","baseModel":"Anima","baseModelType":"Standard",
		"publishedAt":"2026-01-01T00:00:00Z","status":"Published","availability":"Public","nsfwLevel":1,
		"trainedWords":[],"stats":{"downloadCount":1,"thumbsUpCount":1,"thumbsDownCount":0},
		"files":[],"images":[]}`
	parentBody := `{"id":7,"name":"Parent","type":"Checkpoint","creator":{"username":"u"},
		"allowNoCredit":true,"allowCommercialUse":["Image"],"allowDerivatives":true,"allowDifferentLicense":false}`
	env := newTestEnv(t, compareRoutingHandler(&compareRouteCounter{}, map[string]string{
		"/model-versions/1": versionBody,
		"/model-versions/2": strings.Replace(versionBody, `"id":1`, `"id":2`, 1),
		"/models/7":         parentBody,
	}))
	if err := compareRunCmd(t, env, "versions", "1", "2", "--diff"); err != nil {
		t.Fatalf("runCompare --diff: %v", err)
	}
	out := env.buf.String()
	if !strings.Contains(out, "No observed differences.") {
		t.Fatalf("all-equal facts must report no observed differences:\n%s", out)
	}
	if strings.Contains(out, "CHANGES") || strings.Contains(out, "INDETERMINATE") {
		t.Fatalf("no-difference diff must not print change sections:\n%s", out)
	}
}

func TestCompareDiffShowsChangesAndIndeterminate(t *testing.T) {
	env := newTestEnv(t, compareRoutingHandler(&compareRouteCounter{}, map[string]string{
		"/model-versions/1": `{"id":1,"modelId":7,"name":"v1","baseModel":"A","availability":"Public",
			"trainedWords":["x"],"files":[],"images":[]}`,
		"/model-versions/2": `{"id":2,"modelId":7,"name":"v2","baseModel":"B",
			"trainedWords":["x"],"files":[],"images":[]}`,
		"/models/7": `{"id":7,"name":"Parent","type":"Checkpoint","creator":{"username":"u"},
			"allowNoCredit":true,"allowCommercialUse":["Image"],"allowDerivatives":true,"allowDifferentLicense":false}`,
	}))
	// --diff after positionals exercises the shared permuteArgs behavior.
	if err := compareRunCmd(t, env, "versions", "1", "2", "--diff"); err != nil {
		t.Fatalf("runCompare --diff: %v", err)
	}
	out := env.buf.String()
	for _, want := range []string{"CHANGES", "baseModel", "A -> B", "INDETERMINATE", "availability", "<missing>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("diff output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "  creator  [equal]") {
		t.Fatalf("diff must hide confirmed-equal fields:\n%s", out)
	}

	env.buf.Reset()
	if err := compareRunCmd(t, env, "versions", "1", "2"); err != nil {
		t.Fatalf("runCompare full: %v", err)
	}
	full := env.buf.String()
	if !strings.Contains(full, "  creator  [equal]") {
		t.Fatalf("full view must expose aligned equal fields:\n%s", full)
	}
}

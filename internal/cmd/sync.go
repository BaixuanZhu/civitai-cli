package cmd

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"civitai-cli/internal/api"
	"civitai-cli/internal/app"
	"civitai-cli/internal/hashcache"
	"civitai-cli/internal/report"
)

// sync behavior constants (plan §4.4, parent design D8).
const (
	hashWorkers            = 4 // phase-1 local hashing goroutines
	defaultSyncConcurrency = 3 // phase-2 API goroutines / client gate
	maxSyncConcurrency     = 8 // --concurrency hard cap
	defaultSyncExts        = ".safetensors,.pt,.pth,.ckpt,.bin"
)

// computeHash is the streaming SHA256 seam. Tests wrap it to count calls
// (proving cache hits skip recomputation) or inject failures.
var computeHash = hashcache.ComputeSHA256

// newSyncClient builds the phase-2 API client with the requested
// concurrency gate. Tests replace it to point at an httptest server,
// mirroring the newAPIClient seam.
var newSyncClient = func(g *globals, concurrency int) *api.Client {
	return api.NewClient(
		api.WithToken(g.resolveToken()),
		api.WithTimeout(g.Timeout),
		api.WithConcurrency(concurrency),
	)
}

// syncOptions carries the sync-specific flags.
type syncOptions struct {
	ext         string
	format      string
	concurrency int
	rehash      bool
}

// register binds the sync flags to fs.
func (o *syncOptions) register(fs *flag.FlagSet) {
	fs.StringVar(&o.ext, "ext", defaultSyncExts, "comma-separated file extensions to scan (leading dot optional)")
	fs.StringVar(&o.format, "format", "table", "output format: table, csv, json (global --json forces json)")
	fs.IntVar(&o.concurrency, "concurrency", defaultSyncConcurrency, "API concurrency (1-8)")
	fs.BoolVar(&o.rehash, "rehash", false, "recompute every hash, ignoring cache hits")
}

// Sync runs the sync command: civ sync <dir>. It recursively scans a
// directory for model files, hashes them (with a size/mtime cache), looks
// each hash up in the Civitai registry, and prints an audit row per file.
// Exit codes (plan §4.4): 0 when every API call succeeded (not_found is a
// data row, not an error), 2 on any network failure, 1 on a usage error.
func Sync(args []string) {
	var o syncOptions
	execute("sync", args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runSync(ctx, g, pos, &o)
	})
}

// syncRow is one audit line. Status is found/not_found/error; Dupe marks
// files sharing a registry version with another local file. Detail carries
// the failure reason for error rows (stderr warning + JSON "error" field).
// VersionID drives dupe aggregation and is exposed in JSON for scripts.
type syncRow struct {
	File      string `json:"file"`
	Status    string `json:"status"`
	Dupe      bool   `json:"dupe,omitempty"`
	Model     string `json:"model,omitempty"`
	Version   string `json:"version,omitempty"`
	Base      string `json:"base,omitempty"`
	Triggers  int    `json:"triggers"`
	Pickle    string `json:"pickle,omitempty"`
	Virus     string `json:"virus,omitempty"`
	ModelURL  string `json:"modelUrl,omitempty"`
	VersionID int    `json:"versionId,omitempty"`
	Detail    string `json:"error,omitempty"`
}

// syncHeader is the fixed column contract (plan §4.4).
var syncHeader = []string{"FILE", "STATUS", "MODEL", "VERSION", "BASE", "TRIGGERS", "PICKLE", "VIRUS", "MODEL_URL"}

// runSync is the pure sync implementation (testable without os.Exit):
// validate flags, collect files, hash them in phase 1 (saving the cache
// before any network work), look them up in phase 2, aggregate dupes, and
// render. not_found rows never fail the command; the first non-404 API
// error is returned so the exit code reflects network/auth failures.
func runSync(ctx context.Context, g *globals, pos []string, o *syncOptions) error {
	if len(pos) != 1 {
		return errors.New(`sync requires exactly one directory argument, e.g. civ sync I:\models\loras`)
	}
	dir := pos[0]

	format := o.format
	if g.JSON {
		format = "json" // global --json is equivalent to --format json
	}
	switch format {
	case "table", "csv", "json":
	default:
		return fmt.Errorf("unknown --format %q; valid values: table, csv, json", o.format)
	}
	if o.concurrency < 1 || o.concurrency > maxSyncConcurrency {
		return fmt.Errorf("--concurrency must be 1-%d (got %d)", maxSyncConcurrency, o.concurrency)
	}
	exts := parseExts(o.ext)
	if len(exts) == 0 {
		return errors.New("--ext must list at least one file extension")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%q is not a directory; sync scans a directory tree", dir)
	}

	files, err := collectFiles(dir, exts)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		app.Infof("sync: no files matched the --ext filter under %s", dir)
	}

	// CIV_NO_CACHE (non-empty) disables the cache entirely: a nil *Cache
	// makes Get miss and Put/Save no-ops. The cmd layer owns this env check
	// so hashcache stays a pure leaf package (parent design D8).
	var cache *hashcache.Cache
	if os.Getenv("CIV_NO_CACHE") == "" {
		cache = hashcache.Load()
	}

	// Phase 1: local hashing. Save immediately after so a phase-2 network
	// failure (or Ctrl+C) never discards hashing labor — the rerun hits the
	// cache and skips recomputation (the "second run <= 5s" acceptance).
	hashes := hashFiles(ctx, files, cache, o.rehash)
	if err := cache.Save(); err != nil {
		app.Warnf("sync: hash cache save failed: %v", err)
	}
	if err := ctx.Err(); err != nil {
		return &api.NetworkError{Err: err}
	}

	// Phase 2: registry lookups through the shared client concurrency gate.
	c := newSyncClient(g, o.concurrency)
	rows, apiErr := queryFiles(ctx, c, hashes, o.concurrency, syncProgress(len(hashes)))
	if ctx.Err() != nil {
		for i := range rows {
			if rows[i].Status == "" {
				rows[i].Status = "error"
				rows[i].Detail = "canceled"
			}
		}
	}
	markDupes(rows)
	if err := renderSync(g.stdout(), rows, format); err != nil {
		return err
	}
	if apiErr != nil {
		return apiErr
	}
	if err := ctx.Err(); err != nil {
		return &api.NetworkError{Err: err}
	}
	return nil
}

// parseExts canonicalizes the comma-separated --ext list into a set of
// lowercase, dot-prefixed extensions. Empty parts are ignored; a missing
// leading dot is added so "safetensors" and ".safetensors" both work.
func parseExts(s string) map[string]bool {
	m := make(map[string]bool)
	for _, part := range strings.Split(s, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, ".") {
			part = "." + part
		}
		m[part] = true
	}
	return m
}

// collectFiles walks root recursively and returns the absolute paths of
// files whose extension is in exts, sorted for deterministic output. A
// per-entry access error warns on stderr and is skipped (the scan never
// aborts on one unreadable file/subdirectory — plan §4.4).
func collectFiles(root string, exts map[string]bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			app.Warnf("sync: cannot access %s: %v", path, err)
			return nil // skip this entry, keep walking
		}
		if d.IsDir() {
			return nil
		}
		if !exts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			abs = path
		}
		files = append(files, abs)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sync: walk %s: %w", root, err)
	}
	sort.Strings(files)
	return files, nil
}

// fileHash is the phase-1 result for one file: the computed/cached hash,
// or err when stat/hashing failed (rendered as a STATUS=error row).
type fileHash struct {
	path string
	hash string
	err  error
}

// hashFiles runs phase 1: a fixed pool of hashWorkers goroutines consumes
// file indices from a jobs channel. Cache hits skip recomputation unless
// rehash is set. Each result slot is written by exactly one worker (no
// shared-index races); the cache is mutex-safe. ctx cancellation stops the
// workers and feeder; already-computed hashes remain in the cache for the
// caller to Save.
func hashFiles(ctx context.Context, files []string, cache *hashcache.Cache, rehash bool) []fileHash {
	results := make([]fileHash, len(files))
	for i, p := range files {
		results[i] = fileHash{path: p}
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < hashWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case i, ok := <-jobs:
					if !ok {
						return
					}
					results[i] = hashOne(ctx, files[i], cache, rehash)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range files {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	return results
}

// hashOne stats, cache-checks, and hashes a single file. A stat or hash
// failure is returned as fileHash.err (an error row, never a fatal abort).
func hashOne(ctx context.Context, path string, cache *hashcache.Cache, rehash bool) fileHash {
	if err := ctx.Err(); err != nil {
		return fileHash{path: path, err: err}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fileHash{path: path, err: err}
	}
	if !rehash {
		if h, ok := cache.Get(path, fi); ok {
			return fileHash{path: path, hash: h}
		}
	}
	h, err := computeHash(path)
	if err != nil {
		return fileHash{path: path, err: err}
	}
	cache.Put(path, fi, h)
	return fileHash{path: path, hash: h}
}

// queryFiles runs phase 2: a pool of conc goroutines looks each hash up via
// by-hash. 404 becomes a not_found data row; hash failures and other API
// errors become error rows (with a stderr warning). The first non-404 API
// error is returned for exit-code mapping. progress is called once per
// finalized row. ctx cancellation stops the workers.
func queryFiles(ctx context.Context, c *api.Client, hashes []fileHash, conc int, progress func(int)) ([]syncRow, error) {
	rows := make([]syncRow, len(hashes))
	for i, fh := range hashes {
		rows[i] = syncRow{File: fh.path}
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var done atomic.Int64
	recordErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case i, ok := <-jobs:
					if !ok {
						return
					}
					rows[i] = lookupOne(ctx, c, hashes[i], recordErr)
					progress(int(done.Add(1)))
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range hashes {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	return rows, firstErr
}

// lookupOne classifies one file's registry lookup into a syncRow. Hash
// failures short-circuit to an error row without an API call.
func lookupOne(ctx context.Context, c *api.Client, fh fileHash, recordErr func(error)) syncRow {
	row := syncRow{File: fh.path}
	if fh.err != nil {
		row.Status = "error"
		row.Detail = fh.err.Error()
		app.Warnf("sync: hash failed for %s: %v", fh.path, fh.err)
		return row
	}
	raw, err := c.GetVersionByHash(ctx, fh.hash)
	if err != nil {
		var nfe *api.NotFoundError
		if errors.As(err, &nfe) {
			row.Status = "not_found" // a data row, not a command failure
			return row
		}
		row.Status = "error"
		row.Detail = err.Error()
		app.Warnf("sync: lookup failed for %s: %v", fh.path, err)
		recordErr(err)
		return row
	}
	v, err := api.ParseVersion(raw)
	if err != nil {
		row.Status = "error"
		row.Detail = err.Error()
		app.Warnf("sync: decode failed for %s: %v", fh.path, err)
		recordErr(err)
		return row
	}
	row.Status = "found"
	row.fillFromVersion(v)
	return row
}

// fillFromVersion populates a found row from the by-hash version. Scan
// results come from the primary file (or the first file when none is
// flagged primary); the model name/URL come from the owning-model summary.
func (r *syncRow) fillFromVersion(v *api.ModelVersion) {
	r.VersionID = v.ID
	r.Version = v.Name
	r.Base = v.BaseModel
	r.Triggers = len(v.TrainedWords)
	if v.Model != nil {
		r.Model = v.Model.Name
	}
	if v.ModelID > 0 {
		r.ModelURL = fmt.Sprintf("https://civitai.com/models/%d", v.ModelID)
	}
	var f *api.ModelFile
	for i := range v.Files {
		if v.Files[i].Primary {
			f = &v.Files[i]
			break
		}
	}
	if f == nil && len(v.Files) > 0 {
		f = &v.Files[0]
	}
	if f != nil {
		r.Pickle = f.PickleScanResult
		r.Virus = f.VirusScanResult
	}
}

// markDupes flags found rows whose registry version is shared by more than
// one local file (plan §4.4: same version hit by multiple files → dupe).
func markDupes(rows []syncRow) {
	counts := make(map[int]int)
	for _, r := range rows {
		if r.Status == "found" && r.VersionID > 0 {
			counts[r.VersionID]++
		}
	}
	for i := range rows {
		if rows[i].Status == "found" && counts[rows[i].VersionID] > 1 {
			rows[i].Dupe = true
		}
	}
}

// syncDisplayStatus renders the STATUS cell for table/CSV, appending the
// dupe annotation. JSON keeps status pure and carries dupe as a bool.
func syncDisplayStatus(r syncRow) string {
	if r.Dupe {
		return r.Status + " dupe"
	}
	return r.Status
}

// renderSync writes the audit rows in the requested format. CSV uses
// encoding/csv (proper quoting); JSON is the row array; table is an
// aligned tabwriter with long names truncated for the console.
func renderSync(w io.Writer, rows []syncRow, format string) error {
	switch format {
	case "json":
		return report.WriteIndent(w, rows)
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write(syncHeader); err != nil {
			return fmt.Errorf("sync: write csv header: %w", err)
		}
		for _, r := range rows {
			rec := []string{
				r.File, syncDisplayStatus(r), r.Model, r.Version, r.Base,
				strconv.Itoa(r.Triggers), r.Pickle, r.Virus, r.ModelURL,
			}
			if err := cw.Write(rec); err != nil {
				return fmt.Errorf("sync: write csv row: %w", err)
			}
		}
		cw.Flush()
		return cw.Error()
	default: // table
		width := report.ConsoleWidth()
		tw := report.NewTableWriter(w)
		fmt.Fprintln(tw, strings.Join(syncHeader, "\t"))
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
				r.File,
				syncDisplayStatus(r),
				report.Truncate(r.Model, max(width-60, 16)),
				report.Truncate(r.Version, max(width-64, 12)),
				report.Truncate(r.Base, 12),
				r.Triggers,
				r.Pickle,
				r.Virus,
				r.ModelURL,
			)
		}
		return tw.Flush()
	}
}

// syncProgress returns a throttled stderr progress reporter ([INFO] n/total,
// at most ~20 updates plus the final one). Progress never touches stdout so
// it cannot pollute the data stream (parent design D7). It is always-on
// (batch feedback), logged through the standard logger configured in main.
func syncProgress(total int) func(int) {
	step := total / 20
	if step < 1 {
		step = 1
	}
	return func(n int) {
		if n == total || n%step == 0 {
			log.Printf("[INFO] sync %d/%d", n, total)
		}
	}
}

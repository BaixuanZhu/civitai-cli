# Error Handling

> How errors are classified, propagated, mapped to exit codes, and degraded in
> `civitai-cli`.

---

## Overview

Errors flow one way: `internal/api` classifies HTTP/transport failures into a
small set of typed errors; `internal/cmd` maps them to process exit codes via a
shared helper (with per-command overrides) and prints an actionable hint to
stderr. Wrapping uses `fmt.Errorf("...: %w", err)` with a package prefix;
classification uses `errors.As`. **Degradation is never fatal** — a broken
cache, corrupt config, or unavailable secondary fetch warns and continues.

Design contract: design.md D3 (error classes + exit codes), D2 (retry), D4
(tolerant decode).

---

## Error Types

Defined in `internal/api/errors.go`. Four exported classes plus one internal
retry marker:

| Type | Trigger | Meaning |
|---|---|---|
| `ApiError{Status, Detail}` | HTTP 400 (ZodError) and other non-2xx except 401/404 | Deterministic request rejection; usually a CLI-side bug or upstream API drift. `Detail` is a Zod issue summary or a raw body snippet. |
| `UnauthorizedError{Detail}` | HTTP 401 | With a token: invalid/expired (exit 4). Anonymous 401 only happens for personal-state params (`favorites`/`hidden`) and is usage-class (exit 1, needs a token). |
| `NotFoundError{Detail}` | HTTP 404 | Unknown model/version/hash. `vet` maps it to exit 3; other commands to exit 1; `sync` to a `not_found` data row. |
| `NetworkError{Attempts, Err}` | Transient failures after retries exhausted, or context canceled | Transport error, empty 200 body, 429, 5xx. Implements `Unwrap()`. Exit 2. |
| `transientError{err}` (internal) | One retryable attempt | Marks retryable causes inside the retry loop; implements `Unwrap()`. Callers see `NetworkError` once retries are exhausted. |

`classifyHTTPError(status, body)` maps a non-2xx response to its typed error.
`errorDetail(body)` extracts a display summary from **all observed shapes**
(verified live 2026-11-22):

```
{"error":"Model not found"}                       -> string form
{"error":{"name":"ZodError","message":"[{...}]"}} -> Zod summary (first issue message/values + path)
{"message":"Recently Added sort requires ..."}    -> plain message form
anything else (HTML, empty)                       -> 300-byte snippet (rune-boundary safe)
```

`zodSummary` condenses the first Zod issue (descending into `invalid_union`
nested issues, which carry the concrete enum leak). `truncateDetail` caps raw
snippets at `maxDetailBytes = 300` so a CDN HTML error page cannot flood the
terminal.

---

## Error Handling Patterns

### Wrapping and classification

```go
// wrap with a package prefix, preserve the cause
return fmt.Errorf("hashcache: open %s: %w", path, err)

// classify with errors.As (works through wraps)
var nfe *api.NotFoundError
if errors.As(err, &nfe) { return 3 }   // vet semantics
```

### Retry classification (api/client.go)

`doOnce` returns a `fetchOutcome{body, status, retryAfter, err, fatal}`:

- **Retryable** (`transientError`): transport error, empty 200 body (the
  silent-empty-response pitfall), 429, 5xx.
- **Deterministic / fatal** (return immediately): 400/401/403/404, request
  construction failure, body overflow (`io.LimitReader` 5 MB + 1).
- Retry loop = **initial attempt + `len(backoffs)` retries**; default backoffs
  `{500ms, 1s, 2s}` with ±20% jitter, capped at 8s; a valid `Retry-After`
  (seconds or HTTP-date) overrides the wait, capped at 30s. Every wait honors
  `ctx.Done()`. Exhaustion yields `NetworkError{Attempts, Err}`.

### Exit-code mapping (cmd/globals.go)

The shared `exitCodeFor(err, hasToken)`:

```go
nil                  -> 0
*api.NetworkError    -> 2
*api.UnauthorizedError -> 4 if hasToken else 1   // anonymous 401 = usage-class
everything else      -> 1                          // ApiError, NotFoundError, generic
```

Per-command overrides via `executeWithExitCode(..., exitMap, hint)`:

- `vetExitCode`: `*api.NotFoundError -> 3` (hash not registered), else
  `exitCodeFor`. `vetErrorHint` prints vet-specific 404 guidance.
- `loginExitCode`: `*api.UnauthorizedError -> 4` **regardless of `hasToken`**
  (login always probes WITH the pasted key, so a 401 means that key is invalid).

`execute`/`executeWithExitCode` print `civ: <err>` to stderr, call the hint
function, then `os.Exit(code)`. Flag-parse failures and unknown commands use
`app.Fail(err)` (stderr + exit 1). `-h`/`--help` on a subcommand exits 0.

### Degradation is never fatal

| Situation | Behavior |
|---|---|
| Config missing/corrupt/unreadable | `config.Load` warns (`app.Warnf`) and returns zero Config -> anonymous; never blocks a read command |
| Hash cache corrupt/unreadable | `hashcache.Load` warns and rebuilds empty, marks dirty so the next Save heals it |
| `vet` owning-model fetch fails/404/undecodable | `fetchVetModel` warns and returns nil; the by-hash hit stays exit 0 with creator/license omitted (a model-endpoint 404 MUST NOT turn a hit into exit 3) |
| `sync` per-file hash/lookup/decode failure | becomes a `STATUS=error` row + stderr warning; the first non-404 API error is returned for exit mapping |
| `sync` per-file 404 | `not_found` **data row**, command still exits 0 |
| `sync` walk entry access error | warns and skips the entry, keeps walking |
| PATH bootstrap failure | `main` logs `[WARN]` and continues; bootstrap is never fatal |

---

## API Error Responses

The CLI is a client, not a server: it does not produce API error responses, it
consumes them. The user-facing contract is the **exit code** (see README "Exit
codes") plus a stderr error line and hint. `--json` output carries no error
envelope on success; on failure the process exits non-zero and diagnostics go to
stderr (stdout stays clean for piping).

---

## Common Mistakes

- **Retrying a 4xx.** 400/401/403/404 are deterministic answers — return
  immediately, never retry. Only transport/empty-body/429/5xx are retryable.
- **Turning a `vet` hit into exit 3.** The by-hash hit is the source of truth;
  a secondary model-endpoint 404 must degrade (warn + omit), not fail.
- **Treating `sync` `not_found` as an error.** It is a data row; only non-404
  API failures change the exit code.
- **Mapping anonymous 401 to exit 4.** Without a token it is usage-class (1)
  with a "needs a token" hint; only a rejected token is 4.
- **Flooding the terminal with a raw HTML error body.** Always cap detail at
  300 bytes (`truncateDetail`).
- **Swallowing errors silently.** Degrade with a `[WARN]` on stderr; do not
  pretend success.
- **Hand-concatenating URLs.** Enum values contain spaces; a bare space causes
  a silent empty response. Always build query strings via `url.Values.Encode()`
  (the empty body is then treated as retryable).
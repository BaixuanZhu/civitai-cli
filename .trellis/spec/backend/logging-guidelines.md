# Logging Guidelines

> How logging is done in `civitai-cli` (the `civ` CLI).

---

## Overview

Logging uses the **standard library `log`** package, configured once in
`main.go`:

```go
log.SetOutput(os.Stderr)
log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
```

**stdout is reserved for command results only** (the human-readable table or the
`--json` data). Every log, progress line, warning, prompt, and error hint goes to
**stderr**, so `civ ... --json | python ...` and `civ sync ... --format csv >
audit.csv` never get polluted. Levels are hand-written prefixes (`[INFO]`,
`[DEBUG]`, `[WARN]`); there is no third-party logging library.

Design contract: design.md D7 (output contract). Helpers live in
`internal/app/log.go`.

---

## Log Levels

| Prefix | Helper | When | Gated by |
|---|---|---|---|
| `[DEBUG]` | `app.Debugf` | Fine-grained diagnostics: each outbound `GET <url>`, computed SHA256 | `--verbose` only |
| `[INFO]` | `app.Infof` | Notable progress: retry waits, "hashing <path>", `sync` progress | `--verbose` only (except `sync` progress, always-on) |
| `[WARN]` | `app.Warnf` | Non-fatal degradation: unregistered baseModel, first-only base filter, corrupt config/cache, unavailable model details, PATH bootstrap failure | **Always** emitted |
| fatal | `app.Fail(err)` | Usage-class failure: prints `civ: <err>` to stderr and `os.Exit(1)` | Always; never returns |

`app.SetVerbose(bool)` sets an `atomic.Bool` once during command startup (before
request goroutines exist); `Infof`/`Debugf` check it, `Warnf` ignores it. The
atomic keeps `-race` runs clean. The global `--verbose` flag has **no short
alias** (design D6: `show -v` carries version semantics).

```go
// internal/app/log.go
func Infof(format string, args ...any)  { if verboseEnabled.Load() { log.Printf("[INFO] "+format, args...) } }
func Debugf(format string, args ...any) { if verboseEnabled.Load() { log.Printf("[DEBUG] "+format, args...) } }
func Warnf(format string, args ...any)  { log.Printf("[WARN] "+format, args...) }   // always
```

Command failures print `civ: <err>` then a hint to stderr (via
`printErrorHint` / `vetErrorHint` / `loginErrorHint`), then exit with the mapped
code — these are not log-level calls but share the stderr discipline.

---

## Structured Logging

- Format: standard `log` with `Ldate|Ltime|Lshortfile`, e.g.
  `2026-11-22 15:52:56 sync.go:509: [INFO] sync 1/1`. The file:line locator is
  intentional (debuggability).
- The level prefix (`[INFO]`/`[DEBUG]`/`[WARN]`) is part of the message string,
  prepended by the helper.
- All log text is **English** (zh-CN console mojibake lesson).
- `--json` turns off human decoration on **stdout**; it does not silence stderr
  warnings (degradation must stay visible). `sync` progress is always-on
  `[INFO] n/total` to stderr, throttled to ~20 updates plus the final one
  (`syncProgress`), regardless of `--verbose`.

---

## What to Log

- Outbound request URLs at `[DEBUG]` (`api.Client.Get`).
- Retry attempts and waits at `[INFO]` (`retry n/N in <dur>: <cause>`).
- Local hashing start at `[INFO]` and the computed digest at `[DEBUG]` (`vet`).
- `sync` batch progress at `[INFO] n/total` (always-on, stderr).
- Degradations at `[WARN]`: unregistered/first-only baseModel filter, corrupt or
  unreadable config/cache, unavailable owning-model details, per-file sync
  hash/lookup/decode failures, walk access errors, PATH bootstrap failure.
- The `login` prompt `Paste API key:` goes to **stderr** (so a piped key on stdin
  and the saved-key confirmation on stdout stay separate).

---

## What NOT to Log

- **The API token / key — never.** `login` reads the key from stdin and never
  echoes it to stdout or logs it; the key is only sent as the `Authorization:
  Bearer` header and written to the 0600 config file. Do not log `--token`,
  `CIVITAI_API_KEY`, or config contents.
- No PII beyond what the public registry already returns (creator usernames are
  public model metadata, rendered in results, not logged as diagnostics).
- **Nothing diagnostic on stdout.** No `fmt.Println` for progress/warnings;
  stdout carries only the command result. A stray stdout log breaks `--json`
  piping and CSV redirection.
- Do not log full raw error bodies (HTML pages); error detail is capped at 300
  bytes by `api.truncateDetail` before it reaches the error line.

---

## Common Mistakes

- Writing progress/warnings to stdout (pollutes `--json` / CSV). Use
  `app.Warnf`/`app.Infof` or `log.Printf` (stderr-configured).
- Emitting `[INFO]`/`[DEBUG]` without the verbose gate (noisy default output).
- Logging the token or config contents.
- Using `fmt.Println` for diagnostics instead of the leveled stderr helpers.
- Non-English log text (console mojibake on zh-CN Windows).
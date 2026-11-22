# civ login one-line token input

Lightweight task (PRD-only). Independent of `11-22-proxy-support`; can ship
first. Stdlib-only, Windows-only, all-English, offline httptest tests.

## Goal

Let users log in with a single command — `civ login --key <token>`,
`civ login -t <token>`, or `civ login <token>` — instead of the interactive
"Paste API key:" stdin prompt, which today is the ONLY way and rejects a
positional token. Keep the interactive/pipe fallback for backward compatibility.

## Background / Confirmed Facts (repository evidence)

- **Positional token is rejected.** `runLogin` returns a usage error for any
  positional arg: `if len(pos) != 0 { ... "login takes no arguments" }`
  ([internal/cmd/login.go:31](../../../internal/cmd/login.go)). So
  `civ login <token>` exits 1.
- **Stdin-only; global --token ignored.** `Login` registers no command-specific
  flags (`executeWithExitCode("login", args, nil, ...)`,
  [internal/cmd/login.go:23](../../../internal/cmd/login.go)). The candidate key
  comes only from the stdin scanner
  ([internal/cmd/login.go:34-45](../../../internal/cmd/login.go)); `g.Token`
  (global `--token`) is never used as the candidate, so `civ login --token X`
  still prompts. A stdin pipe (`echo <token> | civ login`) already works.
- **Save overwrites the whole config (cross-feature hazard).**
  `config.Save(config.Config{Token: key})`
  ([internal/cmd/login.go:60](../../../internal/cmd/login.go)) writes a fresh
  Config with only Token. Once `11-22-proxy-support` adds `config.Proxy`, every
  login would WIPE the saved proxy. Must become load-modify-save.
- **Probe + exit codes are correct and stay.** Candidate key is probed via
  `GET /models?favorites=true&limit=1` with a globals copy carrying the key
  ([internal/cmd/login.go:50-58](../../../internal/cmd/login.go));
  `loginExitCode` maps 401→4 (nothing saved), network→2
  ([internal/cmd/login.go:71](../../../internal/cmd/login.go)). Key never echoed
  to stdout; success prints the config path
  ([internal/cmd/login.go:63](../../../internal/cmd/login.go)).
- **config.Load/Save are atomic, 0600, degrade-to-zero**
  ([internal/config/config.go:40](../../../internal/config/config.go),
  [:66](../../../internal/config/config.go)). Load-modify-save reuses them.
- **Global flag plumbing.** `globals.Token` + `--token` already parsed
  ([internal/cmd/globals.go:39](../../../internal/cmd/globals.go));
  `permuteArgs` handles value flags after the command name. `-t` is unused
  (show uses `-v` for version), so it is free as a login short flag.
- **Command pattern.** Pure `run*` + `execute*` runner; command-specific flags
  via a `register` func (see skill.go/login.go). Tests drive `runLogin` with a
  `strings.Reader` stdin and swap `newAPIClient`
  ([internal/cmd/login_test.go](../../../internal/cmd/login_test.go)).

## Key Decisions

- D1 (user). Separate lightweight task (not folded into proxy); ships
  independently. The config.Save overwrite fix lives here.
- D2. Candidate token sources, precedence: `--key`/`-t` flag > positional arg >
  global `--token` > interactive stdin prompt. A flag value of `-` means "read
  the key from stdin" (history-safe one-liner, gh/curl convention).
- D3. Save becomes load-modify-save: `c,_ := config.Load(); c.Token = key;
  config.Save(c)` — preserves Proxy and any future fields.
- D4. Probe validation, exit codes (0/1/2/4), no-echo, and success message
  unchanged. Interactive stdin prompt retained when no candidate is given.
- D5. Document the CLI-token exposure tradeoff (shell history / process list)
  and the safer alternatives (`echo <token> | civ login`, `civ login --key -`).

## Requirements

- R1. `civ login --key <token>` and `civ login -t <token>` save the token
  non-interactively (register a login `--key` flag with `-t` alias).
- R2. `civ login <token>` (exactly one positional) saves the token; more than
  one positional is a usage error (exit 1).
- R3. Global `--token <token>` is also accepted as the login candidate.
- R4. Precedence per D2; with no candidate, the interactive stdin prompt and
  pipe behavior are unchanged (backward compatible).
- R5. `--key -` / `-t -` reads the key from stdin (no echo), for a history-safe
  one-liner.
- R6. Empty/whitespace candidate (from any source) → exit 1, no save.
- R7. Save is load-modify-save (D3); login never clears other config fields.
- R8. Probe validation + exit codes + no-echo + success message unchanged
  (401→4 no save, network→2).
- R9. README login section + `civ help` document the new inputs, precedence,
  the history-exposure caveat, and the pipe / `--key -` alternatives.
- R10. Tests: table-driven runLogin for flag/positional/--token/stdin/`-`/empty/
  multi-positional; load-modify-save preserves a pre-set config.Proxy; existing
  login tests updated and green; offline httptest only.

## Acceptance Criteria (observable)

- [ ] AC1. `civ login --key <valid>`, `civ login -t <valid>`, `civ login <valid>`
  each save without a stdin prompt (httptest probe 200) and exit 0.
- [ ] AC2. `civ login --token <valid>` saves (global flag reused).
- [ ] AC3. No candidate → interactive prompt still works; `echo <valid> | civ
  login` still works; `civ login --key -` reads stdin.
- [ ] AC4. Empty/whitespace candidate or >1 positional → exit 1, config unchanged.
- [ ] AC5. Invalid key (probe 401) → exit 4, nothing saved; network failure → 2.
- [ ] AC6. With a pre-set `config.Proxy`, login preserves it (load-modify-save).
- [ ] AC7. Key never appears on stdout; `go vet ./...`, `go test ./...`,
  `go test -race ./...` green; existing tests unaffected.
- [ ] AC8. README + `civ help` updated per R9.

## Out of Scope

- `--json` output shape for login (keep the current human message) — deferred.
- Changing `logout`.
- Token encryption/obfuscation at rest or OS keychain integration (config stays
  0600 plaintext as today).
- The proxy feature itself (separate task `11-22-proxy-support`); only the
  login-side Save preservation is handled here.
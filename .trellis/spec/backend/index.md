# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

This directory contains the backend (Go CLI) guidelines for `civitai-cli` (the
`civ` command). They document the project's **actual conventions**, distilled
from the global design contract
(`.trellis/tasks/archive/2026-11/11-22-civitai-cli-go/design.md`, D1–D9) and
the implemented code under `internal/`. The project is
standard-library-only, all-English, offline-testable, and reproducibly
buildable.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Design Principles](./design-principles.md) | High cohesion / low coupling, Go-style OO idioms, sanctioned seams, extensibility recipes (new subcommand / endpoint / option / format), maintainability contracts | Filled |
| [Directory Structure](./directory-structure.md) | Layering (main -> cmd -> api/resolver/compare/report/hashcache/config + app/env/upgrade), dependency direction, consumer-side seams | Filled |
| [Error Handling](./error-handling.md) | Four api error types, exitCodeFor mapping (0/1/2/3/4), vet/login/resolve overrides (resolve adds 5 = completed not-ready), %w wrapping, non-fatal degradation | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Zero deps, all-English, GoDoc, httptest offline tests, table-driven, CIV_TEST_REGISTRY CI-only gate, reproducible-build trio | Filled |
| [Logging Guidelines](./logging-guidelines.md) | stdlib log -> stderr, [INFO]/[WARN]/[DEBUG] prefixes, app.SetVerbose, stdout-only-results discipline | Filled |
| [Database Guidelines](./database-guidelines.md) | Not applicable — no database; persistence is JSON files (config.json, hashes.json) | N/A |

---

**Language**: All documentation should be written in **English**.

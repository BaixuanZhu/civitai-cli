# Database Guidelines

> **Not applicable to this project.** `civitai-cli` is a stateless CLI client; it
> has no database, ORM, queries, migrations, or transactions.

---

## Overview

There is no relational/NoSQL database. The only persistence is two small JSON
files under the user's OS directories, plus read-only fixtures used by tests.
This file is kept as an explicit "N/A" so the guideline index is complete and no
one assumes a database layer exists.

---

## Actual Persistence (JSON files, not a database)

| Store | Path | Owner package | Notes |
|---|---|---|---|
| Config (API token) | `os.UserConfigDir()/civ/config.json` (`%APPDATA%\civ\config.json`) | `internal/config` | `{"token":"..."}`; 0600 best-effort; atomic write (temp + rename); missing/corrupt degrades to anonymous |
| Hash cache | `os.UserCacheDir()/civ/cache/hashes.json` (`%LOCALAPPDATA%\civ\cache\hashes.json`) | `internal/hashcache` | map[absPath]{size,mtime,sha256}; size+mtime invalidation; atomic write; `CIV_NO_CACHE` bypasses; `--rehash` forces recompute |
| Test fixtures | `testdata/*.json` (+ `testdata/probes/`) | tests only | verbatim live API bodies; read-only; manifest in `testdata/FIXTURES.md` |

Conventions for these stores (atomic temp+rename writes, non-fatal degradation
on corrupt/unreadable files, mutex-safe concurrent cache access) are documented
in [error-handling.md](./error-handling.md) and
[directory-structure.md](./directory-structure.md). The remote source of truth
is the Civitai REST API, accessed only through `internal/api` `Client.Get` (see
[directory-structure.md](./directory-structure.md)).

---

## Query Patterns / Migrations / Naming Conventions

Not applicable — no database. If a future version adds embedded storage, revisit
this guideline then; v1 deliberately keeps persistence to two JSON files.
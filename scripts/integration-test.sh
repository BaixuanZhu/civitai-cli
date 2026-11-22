#!/usr/bin/env bash
# Online integration smoke test for civ (civitai-cli).
#
# Runs the real binary against the live Civitai API (anonymous) and asserts
# exit codes plus --json response shapes for every command. This is the
# canary for upstream API drift: the fixture-based unit tests stay green when
# Civitai changes a field or enum, only a live call catches that.
#
# Isolation: CIV_NO_BOOTSTRAP=1 and CIV_NO_CACHE=1 are exported unconditionally
# below, so the binary under test never writes the user PATH/registry (even a
# release-flavored exe) and never serves a warm local hash cache. The only
# files written are repo-local scratch dirs, removed on exit; nothing outside
# the repo is modified.
#
# Usage:
#   bash scripts/integration-test.sh [exe]
#   Default exe: dist/amd64/civ.exe (build it first: make build).
#
# Paths are repo-relative on purpose: Git Bash /tmp mounts are not resolvable
# by the native Windows exe or by Windows python, so scratch dirs live under
# the working directory (the repo root) where all three agree.

set -euo pipefail

EXE="${1:-dist/amd64/civ.exe}"

if [ ! -f "$EXE" ]; then
  echo "[integration] exe not found: $EXE (build it first: make build)" >&2
  exit 1
fi
if ! command -v python >/dev/null 2>&1; then
  echo "[integration] python is required for the JSON assertions" >&2
  exit 1
fi

# Hard isolation: never touch PATH/registry, never serve a cached response.
export CIV_NO_BOOTSTRAP=1
export CIV_NO_CACHE=1

WORK=".integration-work"
MODELS=".integration-models"
rm -rf "$WORK" "$MODELS"
mkdir -p "$WORK" "$MODELS"
trap 'rm -rf "$WORK" "$MODELS"' EXIT

# Two synthetic local files for the sync not-found path (anonymous by-hash 404).
printf 'integration-a' > "$MODELS/f1.safetensors"
printf 'integration-bb' > "$MODELS/f2.ckpt"

# Known registered hash (Anima turbo-v1.1, plan appendix D) and its AutoV2.
ANIMA_SHA256="FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB"
ANIMA_AUTOV2="FBA1195327"

echo "[integration] exe: $EXE (live Civitai API, anonymous)"

# run <label> <args...>: capture stdout/stderr/exit-code under set -e.
run() {
  local label="$1"; shift
  local got=0
  "$EXE" "$@" >"$WORK/$label.out" 2>"$WORK/$label.err" || got=$?
  printf '%s' "$got" > "$WORK/$label.exit"
  echo "[integration] ran $label (exit $got)"
}

run version          version
run search_json      search anima --json --limit 3
run search_table     search stocking --type lora --limit 5
run show_json        show 2458426 --json
run show_version     show 2458426 -v 3263843 --json
run tags_json        tags anima --json --limit 5
run vet_hit          vet "$ANIMA_SHA256" --json
run vet_autov2       vet "$ANIMA_AUTOV2"
run vet_miss         vet DEADBEEF00
run vet_badinput     vet DEADBEEF
run sync             sync "$MODELS" --format csv
run usage_error      search anima --limit 101

echo "[integration] asserting responses..."
python - "$WORK" <<'PYEOF'
import json
import os
import sys

WORK = sys.argv[1]
failures = []


def check(cond, label):
    print(('  ok   ' if cond else '  FAIL ') + label)
    if not cond:
        failures.append(label)


def exit_of(label):
    with open(os.path.join(WORK, label + '.exit'), encoding='utf-8') as fh:
        return int(fh.read().strip())


def out_of(label):
    with open(os.path.join(WORK, label + '.out'), encoding='utf-8') as fh:
        return fh.read()


def json_of(label):
    # An upstream outage that returns non-JSON FAILs cleanly here instead of
    # crashing the whole assertion run.
    try:
        return json.loads(out_of(label))
    except Exception as exc:
        check(False, '%s: stdout is valid JSON (%s)' % (label, exc))
        return None


# version
check(exit_of('version') == 0 and out_of('version').startswith('civ '),
      'version: exit 0 and prints "civ <version>"')

# search --json: raw items array passthrough with core fields
items = json_of('search_json')
items = items if isinstance(items, list) else []
check(exit_of('search_json') == 0 and len(items) > 0
      and all('id' in i and 'name' in i and 'type' in i for i in items[:3]),
      'search --json: non-empty items array carrying id/name/type')

# search table: human header present, stdout non-empty
check(exit_of('search_table') == 0 and 'NAME' in out_of('search_table'),
      'search table: exit 0 and header row present')

# show --json: structured model object
model = json_of('show_json')
check(exit_of('show_json') == 0 and isinstance(model, dict)
      and ('id' in model or 'name' in model),
      'show --json: exit 0 and model object with id/name')

# show version drill --json: parses (version detail object)
ver = json_of('show_version')
check(exit_of('show_version') == 0 and ver is not None,
      'show -v --json: exit 0 and valid JSON version detail')

# tags --json: name array
tags = json_of('tags_json')
check(exit_of('tags_json') == 0 and isinstance(tags, list) and len(tags) > 0
      and all(isinstance(t, str) for t in tags),
      'tags --json: non-empty array of names')

# vet hit --json: report object with version block
report = json_of('vet_hit')
check(exit_of('vet_hit') == 0 and isinstance(report, dict) and 'version' in report,
      'vet known SHA256 --json: exit 0 and report carries version')

# vet AutoV2 hit
check(exit_of('vet_autov2') == 0,
      'vet AutoV2: exit 0 (same version hit)')

# vet unknown hash -> exit 3 (not registered)
check(exit_of('vet_miss') == 3,
      'vet unknown hash: exit 3 (not registered)')

# vet bad input (8 hex -> path semantics, missing file) -> exit 1
check(exit_of('vet_badinput') == 1,
      'vet bad input: exit 1 (usage-class)')

# sync: not_found data rows, overall exit 0
check(exit_of('sync') == 0 and 'not_found' in out_of('sync'),
      'sync synthetic dir: exit 0 with not_found rows')

# usage error: limit over 100 rejected CLI-side -> exit 1
check(exit_of('usage_error') == 1,
      'search --limit 101: exit 1 (CLI-side cap)')

print()
if failures:
    sys.exit('[integration] %d check(s) FAILED' % len(failures))
print('[integration] all checks passed')
PYEOF
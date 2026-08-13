#!/usr/bin/env bash
# Fail if this library's goose migrations would be unsafe in a consumer app.
#
# Consumer apps flatten THIS library's migrations together with their own module
# migrations and every other cto-up library into ONE goose namespace (see the
# consumer's RunGlobalMigrations / NewMapFS). A duplicate version_id there makes
# goose either panic at startup ("duplicate version") or, worse, treat the losing
# migration as already applied and SILENTLY SKIP it — the schema change never
# lands and the first failing query reports a missing column or relation.
#
# Numbering scheme (adopted 2026-08-13, after coreapp v0.2.29 collided with a
# skeells module migration on version 20260810120000):
#
#   new migrations : 16 digits = YYYYMMDDHHMMSS + 2-digit SOURCE ID
#                    01 core-be-lib  02 cron-lib  03 outbox-lib  04 lcgo-lib
#   consumer apps  : bare 14-digit YYYYMMDDHHMMSS
#
# A 16-digit version is ~100x larger than any 14-digit one, so a library version
# can never equal an app version, and the source ID keeps the libraries apart
# from each other. No cross-repo coordination is needed at release time.
#
# Existing 14-digit files here predate the scheme and are grandfathered by
# LEGACY_MAX. Anything NEWER than that must use the 16-digit form.
set -euo pipefail

SOURCE_ID="01"
LEGACY_MAX="20260810120000"
MIGRATION_DIR="pkg/core/db/migration"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

# Pass 1 — collect "version<TAB>path". Kept free of validation so the loop is
# not part of a pipeline: a subshell would swallow the fail flag set in pass 2.
while IFS= read -r f; do
  base="$(basename "$f")"
  printf '%s\t%s\n' "${base%%_*}" "$f"
done < <(find "$MIGRATION_DIR" -name '*.sql' -type f 2>/dev/null) | sort > "$tmp"

if [ ! -s "$tmp" ]; then
  echo "ERROR: no migrations found under $MIGRATION_DIR" >&2
  exit 1
fi

# Pass 2 — validate each version against the scheme.
fail=0
while IFS=$'\t' read -r version f; do
  if ! printf '%s' "$version" | grep -qE '^[0-9]{14}([0-9]{2})?$'; then
    echo "ERROR: $f" >&2
    echo "       version '$version' is neither a 14-digit legacy timestamp nor a" >&2
    echo "       16-digit timestamp + source id. Use 'make new-migration NAME=<name>'." >&2
    fail=1
  elif [ "${#version}" -eq 14 ]; then
    # Both operands are 14 digits, so string > is numeric >.
    if [[ "$version" > "$LEGACY_MAX" ]]; then
      echo "ERROR: $f" >&2
      echo "       new 14-digit migration (newer than legacy cutoff $LEGACY_MAX)." >&2
      echo "       Library migrations must be 16 digits ending in source id $SOURCE_ID." >&2
      echo "       Use 'make new-migration NAME=<name>'." >&2
      fail=1
    fi
  elif [ "${version: -2}" != "$SOURCE_ID" ]; then
    echo "ERROR: $f" >&2
    echo "       16-digit version ends in '${version: -2}', expected source id '$SOURCE_ID'." >&2
    fail=1
  fi
done < "$tmp"

# Pass 3 — duplicates within this repository.
dups="$(cut -f1 "$tmp" | uniq -d || true)"
if [ -n "$dups" ]; then
  echo "" >&2
  echo "ERROR: duplicate migration version_id(s) within this repository:" >&2
  for v in $dups; do
    echo "  version $v is used by:" >&2
    grep -E "^${v}"$'\t' "$tmp" | cut -f2 | sed 's/^/    - /' >&2
  done
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo "" >&2
  exit 1
fi

echo "OK: $(wc -l < "$tmp" | tr -d ' ') migrations, no duplicates, source id $SOURCE_ID enforced"

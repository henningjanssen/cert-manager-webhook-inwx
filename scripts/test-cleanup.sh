#!/usr/bin/env bash
# Self-check for cleanup-pr.sh: only versions wholly owned by the PR are selected.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
source scripts/cleanup-pr.sh

v() { printf '{"id":%s,"metadata":{"container":{"tags":%s}}}' "$1" "$2"; }
# Two arrays, as `gh api --paginate` emits for two pages.
images="[$(v 1 '["pr-5","pr-5-abc123"]'),$(v 2 '["pr-5-def456"]'),$(v 3 '["pr-5","1.2.3"]'),$(v 4 '["pr-5","main"]'),$(v 5 '["pr-5","dev"]'),$(v 6 '["pr-5","latest"]'),$(v 7 '[]')]
[$(v 8 '["pr-51"]'),$(v 9 '["pr-51-abc"]'),$(v 10 '["pr-5","pr-6"]'),$(v 11 '["1.2.3-rc.1"]'),$(v 12 '["1.2.3","1.2","1","latest"]'),$(v 13 '["pr-5-XYZ"]')]"
charts="[$(v 21 '["0.0.0-pr5_abc1234"]'),$(v 22 '["0.0.0-pr51_abc1234"]'),$(v 23 '["0.0.0-main","main"]'),$(v 24 '["0.0.0-pr5_abc","1.0.0"]'),$(v 25 '["0.0.0-pr5"]'),$(v 26 '["0.0.0-dev","dev"]'),$(v 27 '[]')]"

fails=0
check() { local got; got="$(select_versions "$2" <<<"$3" | xargs)"; if [ "$got" = "$1" ]; then echo "ok   selected: $got"; else echo "FAIL want '$1' got '$got'"; fails=$((fails + 1)); fi; }
check '1 2' '^pr-5(-[0-9a-f]+)?$' "$images"
check '21 25' '^0\.0\.0-pr5([+_.-].*)?$' "$charts"
check '8 9' '^pr-51(-[0-9a-f]+)?$' "$images"
check '' '^pr-99(-[0-9a-f]+)?$' "$images"
[ "$fails" -eq 0 ] || { echo "$fails failed"; exit 1; }
echo "all passed"

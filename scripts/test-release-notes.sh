#!/usr/bin/env bash
# Self-check for release-notes.sh against a throwaway git repository.
set -euo pipefail
script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release-notes.sh"
repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT
cd "$repo"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.com GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.com
git init -q
mkdir -p tools/a tools/b

n=0
commit() { # <path> <message>
  n=$((n + 1)); echo "$n" >"$1/f"; git add .; git commit -qm "$2"
}

fails=0
ok()   { echo "ok   $1"; }
fail() { echo "FAIL $1"; fails=$((fails + 1)); }
has()  { if grep -qF -- "$2" <<<"$notes"; then ok "$1"; else fail "$1: missing '$2' in:"$'\n'"$notes"; fi; }
hasnt(){ if grep -qF -- "$2" <<<"$notes"; then fail "$1: unexpected '$2' in:"$'\n'"$notes"; else ok "$1"; fi; }

commit tools/a 'feat(a): first feature'
git tag a@1.0.0

commit tools/a 'fix(a): old bug'
commit tools/b 'feat(b): other tool'
commit tools/a 'feat(a)!: drop the old flag'
git tag a@2.0.0-rc.1
commit tools/a $'refactor: new config\n\nLonger body.\n\nBREAKING CHANGE: rename foo\nto bar\nRefs: #1'
commit tools/a 'Bump golang from 1 to 2'
git tag a@2.0.0

notes="$(bash "$script" a@2.0.0)"
first="$(head -1 <<<"$notes")"
if [ "$first" = '## ⚠️ Breaking changes' ]; then ok 'breaking section comes first'; else fail "first line is: $first"; fi
has   'bang is breaking'           'drop the old flag'
has   'footer is breaking'         'new config'
has   'multi-line footer text'     '  rename foo to bar'
hasnt 'next footer not included'   'Refs'
has   'stable covers prereleases'  'old bug'
hasnt 'previous release excluded'  'first feature'
hasnt 'other tool excluded'        'other tool'
has   'plain subject in other'     'Bump golang from 1 to 2'
hasnt 'own scope dropped'          'a: '
[ "$(grep -c 'drop the old flag' <<<"$notes")" = 1 ] && ok 'breaking listed once' || fail 'breaking listed more than once'

notes="$(bash "$script" a@2.0.0-rc.1)"
has   'rc: since last stable'      'old bug'
notes="$(bash "$script" a@1.0.0)"
has   'first release: all history' 'first feature'
hasnt 'no empty breaking section'  'Breaking'

git tag a@2.0.0-rc.2 a@2.0.0-rc.1
commit tools/a 'fix(scope): patch'
git tag a@1.0.1
notes="$(bash "$script" a@1.0.1)"
hasnt 'patch of old line starts at 1.0.0' 'first feature'
has   'other scope kept'           'scope: patch'

git tag b@1.0.0 HEAD~1
commit tools/a 'docs: only a'
git tag b@1.0.1
notes="$(bash "$script" b@1.0.1)"
has   'release without changes'    'No changes to b'

bash "$script" a@9.9.9 2>/dev/null && fail 'unknown tag accepted' || ok 'unknown tag rejected'

[ "$fails" -eq 0 ] || { echo "$fails failed"; exit 1; }
echo "all passed"

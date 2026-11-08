#!/usr/bin/env bash
# Self-check for compute_aliases and the publish mode guard.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
MOON_PROJECT_ID=x MOON_PROJECT_ROOT=/nonexistent source scripts/lib.sh

fails=0
check() { # <description> <expected> <version> <existing tags...>
  local d="$1" want="$2" v="$3" got; shift 3
  got="$(printf '%s\n' "$@" | compute_aliases "$v" | xargs)"
  if [ "$got" = "$want" ]; then echo "ok   $d"; else echo "FAIL $d: want '$want' got '$got'"; fails=$((fails + 1)); fi
}
check 'first release'                 '1.0 1 latest' 1.0.0
check 'new highest'                   '1.3 1 latest' 1.3.0 1.2.9 1.2 1 latest
check 'patch of old line, newest there' '1.2 1'      1.2.9 1.2.8 2.0.0 main
check 'old minor keeps major'         '1.2'          1.2.9 1.2.8 1.3.0 2.0.0
check 'lower patch moves nothing'     ''             1.2.3 1.2.4 1.3.0
check '1.10.0 beats 1.9.0'            '1.10 1 latest' 1.10.0 1.9.0
check '1.9.1 loses to 1.10.0'         '1.9'          1.9.1 1.9.0 1.10.0
check 'prefix: 1.1 not 1.10'          '1.1'          1.1.5 1.1.4 1.10.0
check 'prefix: 1 not 10'              '1.1 1'        1.1.5 10.0.0
check 'rc moves nothing'              ''             1.3.0-rc.1 1.2.0
check 'alpha moves nothing'           ''             2.0.0-alpha.1
check 'prereleases do not compete'    '1.3 1 latest' 1.3.0 1.3.1-rc.1 2.0.0-beta.1 1.2.0
check 'junk tags ignored'             '0.2 0 latest' 0.2.0 pr-9 pr-9-abc sha-abc 0.0 0.0.0-main_x 99.99 latest

expect_kind() { # <want|fail> VAR=value...
  local want="$1" got; shift
  got="$(env -u RELEASE_VERSION -u PR_NUMBER -u BRANCH_TAG "$@" MOON_PROJECT_ID=x MOON_PROJECT_ROOT=/nonexistent bash -c 'source scripts/lib.sh; publish_kind' 2>/dev/null || echo fail)"
  if [ "$got" = "$want" ]; then echo "ok   kind $* -> $want"; else echo "FAIL kind $*: want $want got $got"; fails=$((fails + 1)); fi
}
expect_kind release RELEASE_VERSION=1.2.3
expect_kind release RELEASE_VERSION=1.2.3-rc.1
expect_kind release RELEASE_VERSION=1.2.3-alpha.0
expect_kind release RELEASE_VERSION=1.2.3-beta.10
for bad in v1.2.3 1.2 1.2.3-rc 1.2.3-rc.01 1.2.3-foo.1 1.02.3 1.2.3+b 1.2.3-rc.1.2; do expect_kind fail RELEASE_VERSION=$bad; done
expect_kind pr PR_NUMBER=12
expect_kind fail PR_NUMBER=abc
expect_kind branch BRANCH_TAG=main
expect_kind branch BRANCH_TAG=dev
expect_kind fail BRANCH_TAG=feature
expect_kind fail
expect_kind fail RELEASE_VERSION=1.2.3 PR_NUMBER=1

[ "$fails" -eq 0 ] || { echo "$fails failed"; exit 1; }
echo "all passed"

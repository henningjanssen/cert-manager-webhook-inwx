#!/usr/bin/env bash
# Prints the changelog of a release tag as markdown, from the commits that touch
# tools/<tool> since the previous release of that tool. Breaking changes (`type!:`
# or a BREAKING CHANGE footer) come first.
#
#   scripts/release-notes.sh <tool>@<version>
set -euo pipefail

tag="${1:?usage: release-notes.sh <tool>@<version>}"
tool="${tag%@*}"
version="${tag##*@}"
git rev-parse -q --verify "refs/tags/$tag" >/dev/null || { echo "no such tag: $tag" >&2; exit 1; }

# A stable release covers everything since the last stable one, including its
# prereleases. `~` makes sort -V order 1.0.0-rc.1 before 1.0.0.
current="${version/-/\~}"
versions="$(git tag -l "$tool@*" | sed 's/.*@//; s/-/~/')"
[[ "$version" == *-* ]] || versions="$(grep -v '~' <<<"$versions" || true)"
previous="$(printf '%s\n%s\n' "$versions" "$current" | grep -v '^$' | sort -Vu | grep -Fx -B1 -- "$current" | head -1)"
range="refs/tags/$tag"
[ "$previous" = "$current" ] || range="refs/tags/$tool@${previous/\~/-}..$range"

breaking='' features='' fixes='' other=''
while IFS=$'\x1f' read -r -d '' sha subject body; do
  type='' scope='' bang='' text="$subject"
  if [[ "$subject" =~ ^([a-z]+)(\(([^\)]+)\))?(!)?:[[:space:]]+(.+)$ ]]; then
    type="${BASH_REMATCH[1]}" scope="${BASH_REMATCH[3]}" bang="${BASH_REMATCH[4]}" text="${BASH_REMATCH[5]}"
  fi
  [ -z "$scope" ] || [ "$scope" = "$tool" ] || text="$scope: $text"
  entry="- $text (\`$sha\`)"$'\n'

  # A footer runs until a blank line or the next footer.
  detail="$(awk '
    /^BREAKING[ -]CHANGE(: | #)/ { sub(/^[^:#]*(: | #)/, ""); on = 1; printf "%s%s", sep, $0; sep = " "; next }
    on && (/^$/ || /^[A-Za-z-]+(: | #)/) { on = 0 }
    on { printf " %s", $0 }
  ' <<<"$body")"

  if [ -n "$bang" ] || [ -n "$detail" ]; then
    breaking+="$entry"
    [ -z "$detail" ] || breaking+="  $detail"$'\n'
  elif [ "$type" = feat ]; then
    features+="$entry"
  elif [ "$type" = fix ]; then
    fixes+="$entry"
  else
    other+="$entry"
  fi
done < <(git log -z --no-merges --format='%h%x1f%s%x1f%b' "$range" -- "tools/$tool")

section() { if [ -n "$2" ]; then printf '## %s\n\n%s\n' "$1" "$2"; fi; }
section '⚠️ Breaking changes' "$breaking"
section 'Features' "$features"
section 'Bug fixes' "$fixes"
section 'Other changes' "$other"
if [ -z "$breaking$features$fixes$other" ]; then printf 'No changes to %s in this release.\n\n' "$tool"; fi

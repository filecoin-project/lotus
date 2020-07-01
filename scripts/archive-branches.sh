#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

org=filecoin-project
repo=lotus
arch_repo="$org/lotus-archived"
api_repo="repos/$org/$repo"

exclusions=(
	'master'
)

gh_api_next() {
	links=$(grep '^Link:' | sed -e 's/Link: //' -e 's/, /\n/g')
	echo "$links" | grep '; rel="next"' >/dev/null || return
	link=$(echo "$links" | grep '; rel="next"' | sed -e 's/^<//' -e 's/>.*//')

	curl -n -f -sD >(gh_api_next) "$link"
}

gh_api() {
	curl -n -f -sD >(gh_api_next) "https://api.github.com/$1" | jq -s '[.[] | .[]]'
}

pr_branches() {
	gh_api "$api_repo/pulls" |  jq -r '.[].head.label | select(test("^'"$org"':"))' \
		| sed 's/^'"$org"'://'
}

origin_refs() {
	format=${1-'%(refname:short)'}

	git for-each-ref --format "$format" refs/remotes/origin | sed 's|^origin/||'
}

active_branches() {
	origin_refs '%(refname:short) %(committerdate:unix)' |awk \
'	BEGIN { monthAgo = systime() - 31*24*60*60 }
	{ if ($2 > monthAgo) print $1 }
'
}

git remote add archived "git@github.com:$arch_repo.git" || true

branches_to_move="$(cat <(active_branches) <(pr_branches) <((IFS=$'\n'; echo "${exclusions[*]}")) | sort -u | comm - <(origin_refs | sort) -13)"

echo "================"
printf "%s\n" "$branches_to_move"
echo "================"

echo "Please confirm move of above branches [y/N]:"

read -r line
case "$line" in
  [Yy]|[Yy][Ee][Ss]) ;;
  *) exit 1 ;;
esac


printf "%s\n" "$branches_to_move" | \
while read -r ref; do
		git push archived "origin/$ref:refs/heads/$ref/$(date --rfc-3339=date)"
		git push origin --delete "$ref"
	done


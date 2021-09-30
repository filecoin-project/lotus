#!/usr/bin/env bash
set -ex

# Validate lotus version matches the current tag
# $1 - lotus path to execute
# $2 - lotus git tag for this release
function validate_lotus_version_matches_tag(){
  # sanity checks
  if [[ $# != 2 ]]; then
    echo "expected 2 args for validate_lotus_version, got ${$#}"
    exit 100
  fi

  # extract version from `lotus --version` response
  lotus_path=$1
  # get version
  lotus_raw_version=`${lotus_path} --version`
  # grep for version string
  lotus_actual_version=`echo ${lotus_raw_version} | grep -oE '[0-9]+\.[0-9]+\.[0-9]+'`

  # trim leading 'v'
  tag=${2#v}
  # trim possible -rc[0-9]
  expected_version=${tag%-*}

  # check the versions are consistent
  if [[ ${expected_version} != ${lotus_actual_version} ]]; then
    echo "lotus version does not match build tag"
    exit 101
  fi
}

_lotus_path=$1

if [[ ! -z "${CIRCLE_TAG}" ]]; then
  validate_lotus_version_matches_tag "${_lotus_path}" "${CIRCLE_TAG}"
else
  echo "No CI tag found. Skipping version check."
fi

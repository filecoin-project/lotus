#!/usr/bin/env bash
set -exo

pushd dist

# make sure we have a token set, api requests won't work otherwise
if [ -z "${GITHUB_TOKEN}" ]; then
  echo "\${GITHUB_TOKEN} not set, publish failed"
  exit 1
fi

REQUIRED=(
    "jq"
    "curl"
)
for REQUIRE in "${REQUIRED[@]}"
do
    command -v "${REQUIRE}" >/dev/null 2>&1 || echo >&2 "'${REQUIRE}' must be installed"
done

#see if the release already exists by tag
RELEASE_RESPONSE=`
  curl \
    --fail \
    --header "Authorization: token ${GITHUB_TOKEN}" \
    "https://api.github.com/repos/${CIRCLE_PROJECT_USERNAME}/${CIRCLE_PROJECT_REPONAME}/releases/tags/${CIRCLE_TAG}"
`
RELEASE_ID=`echo "${RELEASE_RESPONSE}" | jq '.id'`

if [ "${RELEASE_ID}" = "null" ]; then
  echo "creating release"

  COND_CREATE_DISCUSSION=""
  PRERELEASE=true
  if [[ ${CIRCLE_TAG} =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    COND_CREATE_DISCUSSION="\"discussion_category_name\": \"announcement\","
    PRERELEASE=false
  fi

  RELEASE_DATA="{
    \"tag_name\": \"${CIRCLE_TAG}\",
    \"target_commitish\": \"${CIRCLE_SHA1}\",
    ${COND_CREATE_DISCUSSION}
    \"name\": \"${CIRCLE_TAG}\",
    \"body\": \"\",
    \"prerelease\": ${PRERELEASE}
  }"

  # create it if it doesn't exist yet
  RELEASE_RESPONSE=`
    curl \
        --fail \
        --request POST \
        --header "Authorization: token ${GITHUB_TOKEN}" \
        --header "Content-Type: application/json" \
        --data "${RELEASE_DATA}" \
        "https://api.github.com/repos/$CIRCLE_PROJECT_USERNAME/${CIRCLE_PROJECT_REPONAME}/releases"
  `
else
  echo "release already exists"
fi

RELEASE_UPLOAD_URL=`echo "${RELEASE_RESPONSE}" | jq -r '.upload_url' | cut -d'{' -f1`
echo "Preparing to send artifacts to ${RELEASE_UPLOAD_URL}"

for CHECKSUM_FILE in *.{cid,sha512}
do
  echo "Uploading ${CHECKSUM_FILE}..."
  curl \
    --fail \
    --request POST \
    --header "Authorization: token ${GITHUB_TOKEN}" \
    --header "Content-Type: application/octet-stream" \
    --data-binary "@${CHECKSUM_FILE}" \
    "$RELEASE_UPLOAD_URL?name=$(basename "${CHECKSUM_FILE}")"

  echo "Uploaded ${CHECKSUM_FILE}"
done

popd

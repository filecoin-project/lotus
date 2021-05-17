#!/usr/bin/env bash
set -e

pushd bundle

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

artifacts=(
  "lotus_${CIRCLE_TAG}_linux-amd64.tar.gz"
  "lotus_${CIRCLE_TAG}_linux-amd64.tar.gz.cid"
  "lotus_${CIRCLE_TAG}_linux-amd64.tar.gz.sha512"
  "lotus_${CIRCLE_TAG}_darwin-amd64.tar.gz"
  "lotus_${CIRCLE_TAG}_darwin-amd64.tar.gz.cid"
  "lotus_${CIRCLE_TAG}_darwin-amd64.tar.gz.sha512"
)

for RELEASE_FILE in "${artifacts[@]}"
do
  echo "Uploading ${RELEASE_FILE}..."
  curl \
    --request POST \
    --header "Authorization: token ${GITHUB_TOKEN}" \
    --header "Content-Type: application/octet-stream" \
    --data-binary "@${RELEASE_FILE}" \
    "$RELEASE_UPLOAD_URL?name=$(basename "${RELEASE_FILE}")"

  echo "Uploaded ${RELEASE_FILE}"
done

popd

miscellaneous=(
  "README.md"
  "LICENSE-MIT"
  "LICENSE-APACHE"
)
for MISC in "${miscellaneous[@]}"
do
  echo "Uploading release bundle: ${MISC}"
  curl \
    --request POST \
    --header "Authorization: token ${GITHUB_TOKEN}" \
    --header "Content-Type: application/octet-stream" \
    --data-binary "@${MISC}" \
    "$RELEASE_UPLOAD_URL?name=$(basename "${MISC}")"

  echo "Release bundle uploaded: ${MISC}"
done

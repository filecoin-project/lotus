#!/usr/bin/env bash
set -exo

pushd dist

# make sure we have a token set, api requests won't work otherwise
if [ -z "${GITHUB_TOKEN}" ]; then
  echo "\${GITHUB_TOKEN} not set, publish failed"
  exit 1
fi

if [ -z "${TAG}" ]; then
  echo "\${TAG} not set, publish failed"
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
    "https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/tags/${TAG}"
`
RELEASE_ID=`echo "${RELEASE_RESPONSE}" | jq '.id'`

if [ "${RELEASE_ID}" = "null" ]; then
  echo "https://github.com/repos/${GITHUB_REPOSITORY}/releases/tags/${TAG} does not exist, publish failed"
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

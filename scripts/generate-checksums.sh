#!/usr/bin/env bash
set -euxo pipefail

command -v sha512sum >/dev/null 2>&1 || { echo >&2 "'sha512sum' must be installed"; exit 1; }

# generate checksums
for FILE in dist/*.tar.gz
do
  sha512sum "${FILE}" > "${FILE}.sha512"
done

#!/bin/bash

# this script runs jupyter inside a docker container and copies
# plan manifests from the user's local filesystem into a temporary
# directory that's bind-mounted into the container.

image_name="iptestground/composer"
image_tag="latest"
image_full_name="$image_name:$image_tag"
tg_home=${TESTGROUND_HOME:-$HOME/testground}
container_plans_dir="/testground/plans"
jupyter_port=${JUPYTER_PORT:-8888}

poll_interval=30

exists() {
  command -v "$1" >/dev/null 2>&1
}

require_cmds() {
  for cmd in $@; do
    exists $cmd || { echo "This script requires the $cmd command. Please install it and try again." >&2; exit 1; }
  done
}

get_manifest_paths() {
  find -L $tg_home/plans -name manifest.toml
}

update_manifests() {
  local dest_dir=$1
  mkdir -p $dest_dir

  for m in $(get_manifest_paths); do
    local plan=$(basename $(dirname $m))
    local dest="$dest_dir/$plan/manifest.toml"
    mkdir -p "$dest_dir/$plan"

    # only copy if source manifest is newer than dest (or dest doesn't exist)
    if [[ ! -e $dest || $m -nt $dest ]]; then
      cp $m $dest
    fi
  done
}

watch_manifests() {
  local manifest_dest=$1
  while true; do
    update_manifests ${manifest_dest}
    sleep $poll_interval
  done
}

open_url() {
  local url=$1
  if exists cmd.exe; then
    cmd.exe /c start ${url} >/dev/null 2>&1
  elif exists xdg-open; then
     xdg-open ${url} >/dev/null 2>&1 &
  elif exists open; then
    open ${url}
  else
    echo "unable to automatically open url. copy/paste this into a browser: $url"
  fi
}

# delete temp dir and stop docker container
cleanup () {
  if [[ "$container_id" != "" ]]; then
    docker stop ${container_id} >/dev/null
  fi

  if [[ -d "$temp_manifest_dir" ]]; then
    rm -rf ${temp_manifest_dir}
  fi

  if [[ -d "$temp_jupyter_runtime_dir" ]]; then
    rm -rf ${temp_jupyter_runtime_dir}
  fi
}

# run cleanup on exit
trap "{ cleanup; }" EXIT

# make sure we have the commands we need
require_cmds jq docker

# make temp dir for manifests
temp_base="/tmp"
if [[ "$TEMP" != "" ]]; then
  temp_base=$TEMP
fi

temp_manifest_dir="$(mktemp -d ${temp_base}/testground-composer-XXXX)"
temp_jupyter_runtime_dir="$(mktemp -d ${temp_base}/testground-composer-jupyter-XXXX)"
echo "temp manifest dir: $temp_manifest_dir"
echo "temp jupyter dir: $temp_jupyter_runtime_dir"

# copy the manifests to the temp dir
update_manifests ${temp_manifest_dir}

# run the container in detached mode and grab the id
container_id=$(docker run -d --user $(id -u):$(id -g) -p ${jupyter_port}:8888 -v ${temp_manifest_dir}:${container_plans_dir}:ro -v ${temp_jupyter_runtime_dir}:/jupyter/runtime $image_full_name)

echo "container $container_id started"
# print the log output
docker logs -f ${container_id} &


while [[ ! -f ${temp_jupyter_runtime_dir}/nbserver-1.json ]]; do
  sleep 1
done

token=$(jq -r '.token' ${temp_jupyter_runtime_dir}/nbserver-1.json)
jupyter_url="http://localhost:${jupyter_port}/?token=${token}"

echo "Jupyter url: $jupyter_url"
open_url $jupyter_url


# poll & check for manifest changes every few seconds
watch_manifests ${temp_manifest_dir}

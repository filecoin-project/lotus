#!/bin/bash

# this script runs jupyter inside a docker container and copies
# plan manifests from the user's local filesystem into a temporary
# directory that's bind-mounted into the container.

set -o errexit
set -o pipefail

set -e

err_report() {
    echo "Error on line $1"
}

trap 'err_report $LINENO' ERR


image_name="iptestground/composer"
image_tag="latest"
image_full_name="$image_name:$image_tag"
tg_home=${TESTGROUND_HOME:-$HOME/testground}
container_plans_dir="/testground/plans"
jupyter_port=${JUPYTER_PORT:-8888}
panel_port=${PANEL_PORT:-5006}

poll_interval=30

exists() {
  command -v "$1" >/dev/null 2>&1
}

require_cmds() {
  for cmd in $@; do
    exists $cmd || { echo "This script requires the $cmd command. Please install it and try again." >&2; exit 1; }
  done
}

update_plans() {
  local dest_dir=$1
  rsync -avzh --quiet --copy-links "${tg_home}/plans/" ${dest_dir}
}

watch_plans() {
  local plans_dest=$1
  while true; do
    update_plans ${plans_dest}
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

  if [[ -d "$temp_plans_dir" ]]; then
    rm -rf ${temp_plans_dir}
  fi
}

get_host_ip() {
  # get interface of default route
  local net_if=$(netstat -rn | awk '/^0.0.0.0/ {thif=substr($0,74,10); print thif;} /^default.*UG/ {thif=substr($0,65,10); print thif;}')
  # use ifconfig to get addr of that interface
  detected_host_ip=`ifconfig ${net_if} | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*' | grep -v '127.0.0.1'`

  if [ -z "$detected_host_ip" ]
  then
    detected_host_ip="host.docker.internal"
  fi

  echo $detected_host_ip
}

# run cleanup on exit
trap "{ cleanup; }" EXIT

# make sure we have the commands we need
require_cmds jq docker rsync

if [[ "$SKIP_BUILD" == "" ]]; then
  echo "Building latest docker image. Set SKIP_BUILD env var to any value to bypass."
  require_cmds make
  make docker
fi

# make temp dir for manifests
temp_base="/tmp"
if [[ "$TEMP" != "" ]]; then
  temp_base=$TEMP
fi

temp_plans_dir="$(mktemp -d ${temp_base}/testground-composer-XXXX)"
echo "temp plans dir: $temp_plans_dir"

# copy testplans from $TESTGROUND_HOME/plans to the temp dir
update_plans ${temp_plans_dir}

# run the container in detached mode and grab the id
container_id=$(docker run -d \
  -e TESTGROUND_DAEMON_HOST=$(get_host_ip) \
  --user $(id -u):$(id -g) \
  -p ${panel_port}:5006 \
  -v ${temp_plans_dir}:${container_plans_dir}:ro \
  $image_full_name)

echo "container $container_id started"
# print the log output
docker logs -f ${container_id} &

# sleep for a couple seconds to let the server start up
sleep 2

# open a browser to the app url
panel_url="http://localhost:${panel_port}"
open_url $panel_url

# poll & sync testplan changes every few seconds
watch_plans ${temp_plans_dir}

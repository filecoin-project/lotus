#!/usr/bin/env bash

session="lotus-interop"
wdaemon="daemon"
wminer="miner"
wsetup="setup"
wpledging="pledging"
wcli="cli"
faucet="http://t01000.miner.interopnet.kittyhawk.wtf"

PLEDGE_COUNT="${1:-20}"
BRANCH="interopnet"
BASEDIR=$(mktemp -d -t "lotus-interopnet.XXXX")

git clone --branch "$BRANCH" https://github.com/filecoin-project/lotus.git "${BASEDIR}/build"

mkdir -p "${BASEDIR}/scripts"
mkdir -p "${BASEDIR}/bin"

cat > "${BASEDIR}/scripts/build.bash" <<EOF
#!/usr/bin/env bash
set -x

SCRIPTDIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
pushd \$SCRIPTDIR/../build

pwd
make clean deps lotus lotus-storage-miner lotus-shed
cp lotus lotus-storage-miner lotus-shed ../bin/

popd
EOF

cat > "${BASEDIR}/scripts/env.fish" <<EOF
set -x PATH ${BASEDIR}/bin \$PATH
set -x LOTUS_PATH ${BASEDIR}/.lotus
set -x LOTUS_STORAGE_PATH ${BASEDIR}/.lotusstorage
EOF

cat > "${BASEDIR}/scripts/env.bash" <<EOF
export PATH=${BASEDIR}/bin:\$PATH
export LOTUS_PATH=${BASEDIR}/.lotus
export LOTUS_STORAGE_PATH=${BASEDIR}/.lotusstorage
EOF

cat > "${BASEDIR}/scripts/create_miner.bash" <<EOF
#!/usr/bin/env bash
set -x

owner=\$(lotus wallet new bls)
result=\$(curl -D - -XPOST -F "sectorSize=536870912" -F "address=\$owner" $faucet/mkminer | grep Location)
query_string=\$(grep -o "\bf=.*\b" <<<\$(echo \$result))

declare -A param
while IFS='=' read -r -d '&' key value && [[ -n "\$key" ]]; do
    param["\$key"]=\$value
done <<<"\${query_string}&"

lotus state wait-msg "\${param[f]}"

maddr=\$(curl "$faucet/msgwaitaddr?cid=\${param[f]}" | jq -r '.addr')

lotus-storage-miner init --actor=\$maddr --owner=\$owner
EOF

cat > "${BASEDIR}/scripts/pledge_sectors.bash" <<EOF
#!/usr/bin/env bash

set -x

while [ ! -d ${BASEDIR}/.lotusstorage ]; do
  sleep 5
done

while [ ! -f ${BASEDIR}/.lotusstorage/api ]; do
  sleep 5
done

sleep 30

sector=\$(lotus-storage-miner sectors list | tail -n1 | awk '{print \$1}' | tr -d ':')
current="\$sector"

while true; do
  if (( \$(lotus-storage-miner sectors list | wc -l) > ${PLEDGE_COUNT} )); then
    break
  fi

  while true; do
    state=\$(lotus-storage-miner sectors list | tail -n1 | awk '{print \$2}')

    if [ -z "\$state" ]; then
      break
    fi

    case \$state in
      Packing | Unsealed | PreCommitting | Committing | CommitWait | FinalizeSector ) sleep 30 ;;
      WaitSeed | Proving ) break ;;
      * ) echo "Unknown Sector State: \$state"
          lotus-storage-miner sectors status --log \$current
          break ;;
    esac
  done

  lotus-storage-miner pledge-sector

  while [ "\$current" == "\$sector" ]; do
    sector=\$(lotus-storage-miner sectors list | tail -n1 | awk '{print \$1}' | tr -d ':')
    sleep 5
  done

  current="\$sector"
done
EOF

cat > "${BASEDIR}/scripts/monitor.bash" <<EOF

while true; do
  clear
  lotus sync status

  echo
  echo
  echo Storage Miner Info
  lotus-storage-miner info

  echo
  echo
  echo Sector List
  lotus-storage-miner sectors list | tail -n4

  sleep 25
done
EOF

chmod +x "${BASEDIR}/scripts/build.bash"
chmod +x "${BASEDIR}/scripts/create_miner.bash"
chmod +x "${BASEDIR}/scripts/pledge_sectors.bash"
chmod +x "${BASEDIR}/scripts/monitor.bash"

bash "${BASEDIR}/scripts/build.bash"

tmux new-session -d -s $session -n $wsetup

tmux set-environment -t $session BASEDIR "$BASEDIR"

tmux new-window -t $session -n $wcli
tmux new-window -t $session -n $wdaemon
tmux new-window -t $session -n $wminer
tmux new-window -t $session -n $wpledging

tmux kill-window -t $session:$wsetup

case $(basename $SHELL) in
  fish ) shell=fish ;;
  *    ) shell=bash ;;
esac

tmux send-keys -t $session:$wdaemon   "source ${BASEDIR}/scripts/env.$shell" C-m
tmux send-keys -t $session:$wminer    "source ${BASEDIR}/scripts/env.$shell" C-m
tmux send-keys -t $session:$wcli      "source ${BASEDIR}/scripts/env.$shell" C-m
tmux send-keys -t $session:$wpledging "source ${BASEDIR}/scripts/env.$shell" C-m

tmux send-keys -t $session:$wdaemon "lotus daemon --api 48010 daemon 2>&1 | tee -a ${BASEDIR}/daemon.log" C-m

sleep 30

tmux send-keys -t $session:$wminer   "${BASEDIR}/scripts/create_miner.bash" C-m
tmux send-keys -t $session:$wminer   "lotus-storage-miner run --api 48020 | tee -a ${BASEDIR}/miner.log" C-m
tmux send-keys -t $session:$wcli     "${BASEDIR}/scripts/monitor.bash" C-m
tmux send-keys -t $session:$wpleding "${BASEDIR}/scripts/pledge_sectors.bash" C-m

tmux select-window -t $session:$wcli

tmux attach-session -t $session


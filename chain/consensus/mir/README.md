# Eudico with Mir Consensus

This is an experimental code for internal research purposes. It shouldn't be used in production.

## Requirements
Eudico and [Mir](https://github.com/filecoin-project/mir) requirements must be satisfied.
The most important one is Go1.17+.

## Install

### Eudico
```
git clone git@github.com:filecoin-project/eudico.git
cd eudico
git submodule update --init --recursive
make eudico
```

## Run


### Mir in the root network
```
./scripts/mir/eud-mir-root.sh

```

### Tmux

To stop a demo running via tmux:
```
tmux kill-session -t mir
```

All other Tmux commands can be found in the [Tmux Cheat Sheet](https://tmuxcheatsheet.com/).

### Mir in subnet

To create a deployment run the following script:
```
./scripts/mir/eud-mir-subnet.sh
```

Then run the following commands:
```
 ./eudico subnet add --consensus mir --name mir --min-validators 2
 ./eudico subnet join --subnet=/root/t01001 -val-addr=127.0.0.1:10000 10 
 ./eudico subnet mine  --subnet=/root/t01003 --log-file=mir_miner_00.log --log-level=debug
 ./eudico subnet mine  --subnet=/root/t01003 --log-file=mir_miner_01.log --log-level=debug
 ./eudico subnet mine  --subnet=/root/t01003 --log-file=mir_miner_02.log --log-level=debug
 ./eudico --subnet-api=/root/t01003 wallet list
 ./eudico subnet fund --from=X --subnet=/root/t01002 11
 ./eudico --subnet-api=/root/t01001 send <addr> 10
 ./eudico subnet list-subnets
 ./eudico subnet release --from=X --subnet=/root/t01002
```

If the minimum number of validators for the specified consensus is 4
then you must add at least 4 validators before running the consensus in the subnet:

```
./eudico subnet join --subnet=/root/t01003 -val-addr=/ip4/127.0.0.1/tcp/10000/p2p/12D3KooWJhKBXvytYgPCAaiRtiNLJNSFG5jreKDu2jiVpJetzvVJ 10
./eudico subnet join --subnet=/root/t01003 -val-addr=/ip4/127.0.0.1/tcp/10001/p2p/12D3KooWKPASbibHcHMCEuUk5qx4AQbbPiNgot7F4A4VPeEV6srp 10
./eudico subnet join --subnet=/root/t01003 -val-addr=/ip4/127.0.0.1/tcp/10002/p2p/12D3KooWNuDQaGuwVLPyroJ4FZyNkiFcH2Qi61bNGehK2Mhgq3TK 10
./eudico subnet join --subnet=/root/t01002 -val-addr=/ip4/127.0.0.1/tcp/10003/p2p/12D3KooWRF48VL58mRkp5DmysHp2wLwWyycJ6df2ocEHPvRxMrLs 10
```

Don't forget provide different addresses if you use Mir nodes on the same machine: 
```
./eudico subnet join --subnet=/root/t01002 -val-addr=127.0.0.1:10001 10 
```

`t01001` name is just an example, in your setup the real subnet name may be different.

# project lotus - 莲

Lotus is an experimental implementation of the Filecoin Distributed Storage
Network. For more details, check out the
[spec](https://github.com/filecoin-project/specs).

## Development

All work is tracked via issues. An attempt at keeping an up-to-date view on
remaining work is in the [lotus testnet github project
board](https://github.com/filecoin-project/lotus/projects/1).


## Building

*Dependencies:*
- go1.12 or higher
- gcc
- git
- bzr (some go dependency needs this)
- b2sum
- jq
- pkg-config

*Building:*
```
$ make
```

## Devnet

### Node setup

Start full node daemon
```sh
$ lotus daemon
```

Connect to the network:
```sh
$ lotus net connect /ip4/147.75.80.29/tcp/1347/p2p/12D3KooWGThG7Ct5aX4tTRkgvjr3pT2JyCyyvK77GhXVQ9Cfjzj2
$ lotus net connect /ip4/147.75.80.17/tcp/1347/p2p/12D3KooWRNm4a6ESBr9bbTpSC2CfLfoWKRpABJi7FR3GhHw7usKW
```

[wait for the chain to finish syncing]

You can see current chain height with
```sh
lotus chain getblock $(lotus chain head) | jq .Height
```

### Basics

Create new address
```sh
$ lotus wallet new bls
t3...
```

Grab some funds from faucet - go to http://147.75.80.29:777/, paste the address
you just created, and press Send

See wallet balance:
```sh
$ lotus wallet balance [optional address (t3...)]
```
(NOTE: If you see an error like `actor not found` after executing this command,
it means that either there are no transactions to this address on chain - using
faucet should 'fix' this, or your node isn't fully synced)

### Mining

Ensure that at least one BLS address (`t3..`) in your wallet has enough funds to
cover pledge collateral:
```sh
$ lotus state pledge-collateral
1234
$ lotus wallet balance t3...
8999
```
(Balance must be higher than the returned pledge collateral for the next step to work)

Initialize storage miner:
```sh
$ lotus-storage-miner init --owner=t3...  
```
This command should return successfully after miner is setup on-chain (30-60s)

Start mining:
```sh
$ lotus-storage-miner run
```

Seal random data to start producing PoSts:
```sh
$ lotus-storage-miner store-garbage
```

### Making deals

TODO: see `$ lotus client` commands

### Pond UI

Build:
```
$ make pond
```

Run:
```
$ ./pond run
Listening on http://127.0.0.1:2222
```

Now go to http://127.0.0.1:2222, basic usage should be rather intuitive

Note: don't leave unattended for long periods of time (10h+), the web-ui tends to
eventually consume all the available RAM

### Troubleshooting

* Turn it off
* `rm -rf ~/.lotus ~/.lotusstorage/`
* "Turn it on" - Start at the top
* If that didn't help, open a new issue

## Architecture
Lotus is architected modularly, and aims to keep clean api boundaries between
everything, even if they are in the same process. Notably, the 'lotus full node'
software, and the 'lotus storage miner' software are two separate programs.

The lotus storage miner is intended to be run on the machine that manages a
single storage miner instance, and is meant to communicate with the full node
via the websockets api for all of its chain interaction needs. This way, a
mining operation may easily run one or many storage miners, connected to one or
many full node instances.

## Notable Modules

### Api
The systems api is defined in here. The rpc maps directly to the api defined
here using the jsonrpc package in `lib/jsonrpc`.

### Chain/Types
Implementation of data structures used by Filecoin and their serializations.

### Chain/Store
The chainstore manages all local chain state, including block headers,
messages, and state.

### Chain/State
A package for dealing with the filecoin state tree. Wraps the
[HAMT](https://github.com/ipfs/go-hamt-ipld).

### Chain/Actors
Implementations of the builtin Filecoin network actors.

### Chain/Vm
The filecoin state machine 'vm'. Implemented here are utilities to invoke
filecoin actor methods.


### Miner
The block producer logic. This package interfaces with the full node through
the api, despite currently being implented in the same process (very likely to
be extracted as its own separate process in the near future).

### Storage
The storage miner logic. This package also interfaces with the full node
through a subset of the api. This code is used to implement the
lotus-storage-miner process.

## Pond
Pond is a graphical testbed for lotus. It can be used to spin up nodes, connect
them in a given topology, start them mining, and observe how they function over
time.

To try it out, run `make pond`, then run `./pond run`.
Once it is running, visit localhost:2222 in your browser.

## Tracing
Lotus has tracing built into many of its internals. To view the traces, first
[download jaeger](https://www.jaegertracing.io/download/) (Choose the
'all-in-one' binary). Then run it somewhere, start up the lotus daemon, and
open up localhost:16686 in your browser.

For more details, see [this document](./docs/tracing.md).

## License
MIT + Apache

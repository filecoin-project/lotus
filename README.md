# project lotus - 莲

Lotus is an experimental implementation of the Filecoin Distributed Storage Network. For more details about Filecoin, check out the [Filecoin Spec](https://github.com/filecoin-project/specs).

## Development

All work is tracked via issues. An attempt at keeping an up-to-date view on remaining work is in the [lotus testnet github project board](https://github.com/filecoin-project/lotus/projects/1).


## Building

*Dependencies:*

- go (1.12 or higher)
- gcc (7.4.0 or higher)
- git
- bzr (some go dependency needs this)
- b2sum (In Linux in coreutils)
- jq
- pkg-config

*Building:*
```
$ make
```

## Devnet

### Node setup

If you have run lotus before and want to remove all previous data: `rm -rf ~/.lotus ~/.lotusstorage`

[You can copy `lotus` and `lotus-storage-miner` to your `$GOPATH/bin` or `$PATH`, or reference all lotus commands below from your local build directory with `./lotus`]

The following sections describe how to use the lotus CLI. Alternately you can running nodes and miners using the [Pond GUI](#pond).

### Start Daemon

```sh
$ lotus daemon
```

In another window check that you are connected to the network:
```sh
$ lotus net peers | wc -l
2 # number of peers
```

[wait for the chain to finish syncing]

You can see current chain height with
```sh
$ lotus sync status
```
or use
```sh
$ lotus chain getblock $(lotus chain head) | jq .Height
```

### Basics

Create new address
```sh
$ lotus wallet new bls
t3...
```

Grab some funds from faucet - go to http://147.75.80.29:777/, paste the address
you just created, and press Send.

See wallet balance:
```sh
$ lotus wallet balance [optional address (t3...)]
```
(NOTE: If you see an error like `actor not found` after executing this command,
it means that either there are no transactions to this address on chain - using
faucet should 'fix' this, or your node isn't fully synced).

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

In the Miner's startup log will be the miner id used for deals: 
e.g.  `Registering miner 't0111' with full node.` 

Seal random data to start producing PoSts:

```sh
$ lotus-storage-miner store-garbage
```

You can check Miner power and sector usage with the miner id:

```sh
# Total Power of the network
$ lotus-storage-miner state power

$ lotus-storage-miner state power <miner>

$ lotus-storage-miner state sectors <miner>
```

### Stage Data

Import some data:

```sh
# Create a simple file
$ echo "Hi my name is $USER" > hello.txt

# Import the file into lotus & get a Data CID
$ lotus client import ./hello.txt
<Data CID>

# List imported files by CID, name, size, status
$ lotus client local
```

(CID is short for Content Identifier, a self describing content address used throughout the IPFS ecosystem. It is a cryptographic hash that uniquely maps to the data and verifies it has not changed.)

### Make a deal

(It is possible for a Client to make a deal with a Miner on the same lotus Node.)

```sh
# List asks proposed by a miner
$ lotus client query-ask <miner>

# Propose a deal with a miner
$ lotus client deal <Data CID> <miner> <price> <duration>
```

For example `$ lotus client deal bafkre...qvtjsi t0111 36000 12` proposes a deal to store CID `bafkre...qvtjsi` with miner `t0111` at price `36000` for a duration of `12` blocks.

### Search & Retrieval

If you've stored data with a miner in the network, you can search for it by CID:

```sh
# Search for data by CID
$ lotus client find <Data CID>
LOCAL
RETRIEVAL <miner>@<miner peerId>-<deal funds>-<size>
```

To retrieve data from a miner:

```sh
$ lotus client retrieve <Data CID> <outfile>
```

This will initiate a retrieval deal and write the data to the outfile. (This process may take some time.)



### Pond UI

-----

As an alternative to the CLI you can use Pond, a graphical testbed for lotus. It can be used to spin up nodes, connect them in a given topology, start them mining, and observe how they function over time.

Build:

```
$ make pond
```

Run:
```
$ ./pond run
Listening on http://127.0.0.1:2222
```

Now go to http://127.0.0.1:2222. 

**Things to try:**

- The `Spawn Node` button starts a new lotus Node in a new draggable window.
- Click `[Spawn Storage Miner]` to start mining (make sure the Node's wallet has funds).
- Click on `[Client]` to open the Node's client interface and propose a deal with an existing Miner. If successful you'll see a payment channel open up with that Miner.

> Note: Don't leave Pond unattended for long periods of time (10h+), the web-ui tends to
> eventually consume all the available RAM.

### Troubleshooting

* Turn it off and on - Start at the top
* `rm -rf ~/.lotus ~/.lotusstorage/`
* Verify you have the correct versions of dependencies
* If that didn't help, open a new issue, ask in the [Community Forum](https://discuss.filecoin.io) or reach out via [Community chat](https://github.com/filecoin-project/community#chat).



## Architecture

Lotus is architected modularly, and aims to keep clean API boundaries between everything, even if they are in the same process. Notably, the 'lotus full node' software, and the 'lotus storage miner' software are two separate programs.

The lotus storage miner is intended to be run on the machine that manages a single storage miner instance, and is meant to communicate with the full node via the websockets api for all of its chain interaction needs. This way, a mining operation may easily run one or many storage miners, connected to one or many full node instances.

## Notable Modules

### API
The systems API is defined in here. The RPC maps directly to the API defined here using the JSON RPC package in `lib/jsonrpc`. Initial API documentation in [docs/API.md](docs/API.md).

### Chain/Types
Implementation of data structures used by Filecoin and their serializations.

### Chain/Store
The chainstore manages all local chain state, including block headers, messages, and state.

### Chain/State
A package for dealing with the Filecoin state tree. Wraps the [HAMT](https://github.com/ipfs/go-hamt-ipld).

### Chain/Actors
Implementations of the builtin Filecoin network actors.

### Chain/Vm
The Filecoin state machine 'vm'. Implemented here are utilities to invoke Filecoin actor methods.


### Miner
The block producer logic. This package interfaces with the full node through the API, despite currently being implemented in the same process (very likely to be extracted as its own separate process in the near future).

### Storage
The storage miner logic. This package also interfaces with the full node through a subset of the api. This code is used to implement the `lotus-storage-miner` process.

## Pond
Pond is a graphical testbed for lotus. It can be used to spin up nodes, connect them in a given topology, start them mining, and observe how they function over time.

To try it out, run `make pond`, then run `./pond run`. 
Once it is running, visit localhost:2222 in your browser.

## Tracing
Lotus has tracing built into many of its internals. To view the traces, first download jaeger](https://www.jaegertracing.io/download/) (Choose the 'all-in-one' binary). Then run it somewhere, start up the lotus daemon, and open up localhost:16686 in your browser.

For more details, see [this document](./docs/tracing.md).

## License
Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)

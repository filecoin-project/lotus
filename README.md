![Lotus](docs/images/lotus_logo_h.png)

# project lotus - 莲

Lotus is an experimental implementation of the Filecoin Distributed Storage Network. For more details about Filecoin, check out the [Filecoin Spec](https://github.com/filecoin-project/specs).

## Development

All work is tracked via issues. An attempt at keeping an up-to-date view on remaining work is in the [lotus testnet github project board](https://github.com/filecoin-project/lotus/projects/1).


## Building

We currently only provide the option to build lotus from source. Binary installation options are coming soon!

In order to run lotus, please do the following:
1. Make sure you have these dependencies installed:
- go (1.13 or higher)
- gcc (7.4.0 or higher)
- git (version 2 or higher)
- bzr (some go dependency needs this)
- jq
- pkg-config
- opencl-icd-loader
- opencl driver (like nvidia-opencl on arch) (for GPU acceleration) 
- opencl-headers (build)
- rustup (proofs build)
- llvm (proofs build)
- clang (proofs build)

Arch (run):
```sh
sudo pacman -Syu opencl-icd-loader
```

Arch (build):
```sh
sudo pacman -Syu go gcc git bzr jq pkg-config opencl-icd-loader opencl-headers
```

Ubuntu / Debian (run):
```sh
sudo apt update
sudo apt install mesa-opencl-icd ocl-icd-opencl-dev
```

Ubuntu (build):
```sh
sudo add-apt-repository ppa:longsleep/golang-backports
sudo apt update
sudo apt install golang-go gcc git bzr jq pkg-config mesa-opencl-icd ocl-icd-opencl-dev
```

2. Clone this repo & `cd` into it
```
$ git clone https://github.com/filecoin-project/lotus.git
$ cd lotus/
```

3. Build and install the source code
```
$ make
$ sudo make install
```

Now, you should be able to perform the commands listed below.

## Devnet

### Node setup

If you have run lotus before and want to remove all previous data: `rm -rf ~/.lotus ~/.lotusstorage`

The following sections describe how to use the lotus CLI. Alternately you can run lotus nodes and miners using the [Pond GUI](#pond).

### Genesis & Bootstrap

The current lotus build will automatically join the lotus Devnet using the genesis and bootstrap files in the `build/` directory. No configuration is needed.

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

You can follow sync status with:
```sh
$ watch lotus sync status
```

then view latest block height along with other network metrics at the https://lotus-metrics.kittyhawk.wtf/chain.

[It may take a few minutes for the chain to finish syncing. You will see `Height: 0` until the full chain is synced and validated.]

### Basics

Create a new address:
```sh
$ lotus wallet new bls
t3...
```

Grab some funds from faucet - go to https://lotus-faucet.kittyhawk.wtf/, paste the address
you just created, and press Send.

Check the wallet balance (balance is listed in attoFIL, where 1 attoFIL = 10^-18 FIL):
```sh
$ lotus wallet balance [optional address (t3...)]
```

(NOTE: If you see an error like `actor not found` after executing this command, it means that either your node isn't fully synced or there are no transactions to this address yet on chain. If the latter, using the faucet should 'fix' this).

### Mining

Ensure that at least one BLS address (`t3..`) in your wallet exists
```sh
$ lotus wallet list
t3...
```
With this address, go to https://lotus-faucet.kittyhawk.wtf/miner.html, and
click `Create Miner`

Wait for a page telling you the address of the newly created storage miner to
appear - It should be saying: `New storage miners address is: t0..`

Initialize storage miner:
```sh
$ lotus-storage-miner init --actor=t01.. --owner=t3....  
```
This command should return successfully after miner is setup on-chain (30-60s)

Start mining:
```sh
$ lotus-storage-miner run
```

To view the miner id used for deals: 

```sh
$ lotus-storage-miner info
```

e.g. miner id `t0111`

Seal random data to start producing PoSts:

```sh
$ lotus-storage-miner store-garbage
```

You can check miner power and sector usage with the miner id:

```sh
# Total power of the network
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
# List all miners in the system. Choose one to make a deal with.
$ lotus state list-miners

# List asks proposed by a miner
$ lotus client query-ask <miner>

# Propose a deal with a miner. Price is in attoFIL/byte/block. Duration is # of blocks.
$ lotus client deal <Data CID> <miner> <price> <duration>
```

For example `$ lotus client deal bafkre...qvtjsi t0111 36000 12` proposes a deal to store CID `bafkre...qvtjsi` with miner `t0111` at price `36000` for a duration of `12` blocks. If successful, the `client deal` command will return a deal CID.

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

### Monitoring Dashboard

To see the latest network activity, including chain block height, blocktime, total network power, largest miners, and more, check out the monitoring dashboard at https://lotus-metrics.kittyhawk.wtf.

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
* If stuck on a bad fork, try `lotus chain sethead --genesis`
* If that didn't help, open a new issue, ask in the [Community forum](https://discuss.filecoin.io) or reach out via [Community chat](https://github.com/filecoin-project/community#chat).



## Architecture

Lotus is architected modularly, and aims to keep clean API boundaries between everything, even if they are in the same process. Notably, the 'lotus full node' software, and the 'lotus storage miner' software are two separate programs.

The lotus storage miner is intended to be run on the machine that manages a single storage miner instance, and is meant to communicate with the full node via the websockets jsonrpc api for all of its chain interaction needs. This way, a mining operation may easily run one or many storage miners, connected to one or many full node instances.

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

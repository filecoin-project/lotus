# Frequently Asked Questions

Here are some FAQs concerning the Lotus implementation and participation in 
Testnet.
For questions concerning the broader Filecoin project, please 
go [here](https://filecoin.io/faqs/).

## Introduction to Lotus

### What is Lotus?

Lotus is an implementation of the **Filecoin Distributed Storage Network**, written in Go. 
It is designed to be modular and interoperable with any other implementation of the Filecoin Protocol.
More information about Lotus can be found [here](https://lotu.sh/).

### What are the components of Lotus?

Lotus is composed of two separate pieces that can talk to each other:

The Lotus Node can sync the blockchain, validating all blocks, transfers, and deals
along the way. It can also facilitate the creation of new storage deals. If you are not 
interested in providing your own storage to the network, and do not want to produce blocks
yourself, then the Lotus Node is all you need!

The Lotus Storage Miner does everything you need for the registration of storage, and the
production of new blocks. The Lotus Storage Miner communicates with the network
by talking to a Lotus Node over the JSON-RPC API.

## Setting up a Lotus Node

### How do I set up a Lotus Node?

Follow the instructions found [here](https://docs.lotu.sh/en+getting-started).

### Where can I get the latest version of Lotus?

Download the binary tagged as the `Latest Release` from the
 [Lotus Github repo](https://github.com/filecoin-project/lotus/releases).
 
### What operating systems can Lotus run on?

Lotus can build and run on most Linux and MacOS systems with at least 
8GB of RAM. Windows is not yet supported.

### How can I update to the latest version of Lotus?

To update Lotus, follow the instructions [here](https://lotu.sh/en+updating-lotus).

### How do I prepare a fresh installation of Lotus?

Stop the Lotus daemon, and delete all related files, including sealed and chain data by 
running `rm ~/.lotus ~/.lotusstorage`.

Then, install Lotus afresh by following the instructions 
found [here](https://docs.lotu.sh/en+getting-started).

## Interacting with a Lotus Node

### How can I communicate with a Lotus Node?

Lotus Nodes have a command-line interface, as well as a JSON-RPC API.

### What are the commands I can send using the command-line interface? 

The command-line interface is self-documenting, try running `lotus --help` from the `lotus` home 
directory for more.

### How can I send a request over the JSON-RPC API?

Information on how to send a `cURL` request to the JSON-RPC API can be found
[here](https://lotu.sh/en+api). A JavaScript client is under development.

### What are the requests I can send over the JSON-RPC API?

Please have a look at the 
[source code](https://github.com/filecoin-project/lotus/blob/master/api/api_common.go) 
for a list of methods supported by the JSON-RPC API.
## The Test Network

### What is Testnet?

Testnet is a live network of Lotus Nodes run by the 
community for testing purposes.
 It has 2 PiB of storage (and growing!) dedicated to it.

### Is FIL on the Testnet worth anything?

Nothing at all! Real-world incentives may be provided in a future phase of Testnet, but this is 
yet to be confirmed.

### Will there be future phases of Testnet?

Yes, there will be at least one more phase of Testnet. We plan on introducing interoperable
[go-filecoin nodes](https://github.com/filecoin-project/go-filecoin#filecoin-go-filecoin)
in a future phase.

### How can I see the status of Testnet?

The [dashboard](https://stats.testnet.filecoin.io/) displays the status of the network as 
well as a ton
of other metrics you might find interesting.

## Mining with a Lotus Node on Testnet

### How do I get started mining with Lotus?

Follow the instructions found [here](https://lotu.sh/en+mining).

### What are the minimum hardware requirements?

An example test configuration, and minimum hardware requirements can be found 
[here](https://lotu.sh/en+hardware-mining). 

Note that these might NOT be the minimum requirements for mining on Mainnet.

### What are some GPUs that have been tested?

A list of benchmarked GPUs can be found [here](https://lotu.sh/en+hardware-mining#benchmarked-gpus-7393).

## Advanced questions

### Is there a Docker image for lotus?

Community-contributed Docker and Docker Compose examples are available 
[here](https://github.com/filecoin-project/lotus/tree/master/tools/dockers/docker-examples).

### How can I run two miners on the same machine?

You can do so by changing the storage path variable for the second miner, e.g.,
`LOTUS_STORAGE_PATH=~/.lotusstorage2`. You will also need to make sure that no ports collide.

### How do I setup my own local devnet?     

Follow the instructions found [here](https://lotu.sh/en+setup-local-dev-net).
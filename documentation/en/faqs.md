# Frequently Asked Questions

Here are some FAQs concerning the Lotus implementation and participation in 
Testnet.
For questions concerning the broader Filecoin project, please 
go [here](https://filecoin.io/faqs/).

## Introduction to Lotus

### What is Lotus?

Lotus is an implementation of the **Filecoin Distributed Storage Network**, written in Go. 
It is designed to be modular and interoperable with any other implementation of the Filecoin Protocol.

### What are the components of Lotus?

Lotus is composed of two separate pieces that can talk to each other:

The Lotus Node can sync the blockchain, validating all blocks, transfers, and deals
along the way. It can also facilitate the creation of new storage deals. If you are not 
interested in providing your own storage to the network, and do not want to produce blocks
yourself, then the Lotus Node is all you need!

The Lotus Miner does everything you need for the registration of storage, and the
production of new blocks. The Lotus Miner communicates with the network by talking 
to a Lotus Node over the JSON-RPC API.

## Setting up a Lotus Node

### How do I set up a Lotus Node?

Follow the instructions found [here](en+install) and [here](en+setup).

### Where can I get the latest version of Lotus?

Download the binary tagged as the `Latest Release` from the [Lotus Github repo](https://github.com/filecoin-project/lotus/releases) or checkout the `master` branch of the source repository.
 
### What operating systems can Lotus run on?

Lotus can build and run on most Linux and MacOS systems with [at least 8GB of RAM](en+install#hardware-requirements-1). Windows is not yet supported.

### How can I update to the latest version of Lotus?

To update Lotus, follow the instructions [here](en+update).

### How do I prepare a fresh installation of Lotus?

Stop the Lotus daemon, and delete all related files, including sealed and chain data by 
running `rm ~/.lotus ~/.lotusminer`.

Then, install Lotus afresh by following the instructions 
found [here](en+install).

### Can I configure where the node's config and data goes?

Yes! The `LOTUS_PATH` variable sets the path for where the Lotus node's data is written.
The `LOTUS_MINER_PATH` variable does the same for miner-specific information.

## Interacting with a Lotus Node

### How can I communicate with a Lotus Node?

Lotus Nodes have a command-line interface, as well as a JSON-RPC API.

### What are the commands I can send using the command-line interface? 

The command-line interface is self-documenting, try running `lotus --help` from the `lotus` home 
directory for more.

### How can I send a request over the JSON-RPC API?

Information on how to send a `cURL` request to the JSON-RPC API can be found
[here](en+api).

### What are the requests I can send over the JSON-RPC API?

Please have a look [here](en+api).


## The Test Network

### What is Testnet?

Testnet is a live network of Lotus Nodes run by the 
community for testing purposes.

### Is FIL on the Testnet worth anything?

Nothing at all!

### How can I see the status of Testnet?

The [dashboard](https://stats.testnet.filecoin.io/) displays the status of the network as 
well as a ton of other metrics you might find interesting.

## Mining with a Lotus Node on Testnet

### How do I get started mining with Lotus?

Follow the instructions found [here](en+mining).

### What are the minimum hardware requirements?

An example test configuration, and minimum hardware requirements can be found 
[here](en+install#hardware-requirements-8).

Note that these might NOT be the minimum requirements for mining on Mainnet.

### What are some GPUs that have been tested?

See previous question.

### Why is my GPU not being used when sealing a sector?

Sealing a sector does not involve constant GPU operations. It's possible
that your GPU simply isn't necessary at the moment you checked.

## Advanced questions

### Is there a Docker image for lotus?

Community-contributed Docker and Docker Compose examples are available 
[here](https://github.com/filecoin-project/lotus/tree/master/tools/dockers/docker-examples).

### How can I run two miners on the same machine?

You can do so by changing the storage path variable for the second miner, e.g.,
`LOTUS_MINER_PATH=~/.lotusminer2`. You will also need to make sure that no ports collide.

### How do I setup my own local devnet?     

Follow the instructions found [here](en+local-devnet).

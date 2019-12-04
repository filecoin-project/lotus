# Lotus

Lotus is an alternative implementation of the **Filecoin Distributed Storage Network**. It is the only implementation you will be able to use when the **Filecoin TestNet** launches.

For more details about Filecoin, check out the [Filecoin Spec](https://github.com/filecoin-project/specs).

## What can I learn here?

- How to install Lotus on [Arch Linux](https://docs.lotu.sh/install-lotus-arch), [Ubuntu](https://docs.lotu.sh/install-lotus-ubuntu), or [MacOS](https://docs.lotu.sh/install-lotus-macos).
- [Storing](https://docs.lotu.sh/storing-data) or [retrieving](https://docs.lotu.sh/retrieving-data) data.
- Joining the **Lotus DevNet** using your [CLI](https://docs.lotu.sh/join-devnet-cli).
- Test Lotus in a seperate local network using [Pond UI](https://docs.lotu.sh/testing-with-gui).
- Mining Filecoin using the **Lotus Storage Miner** in your [CLI](https://docs.lotu.sh/mining).

## What makes Lotus different?

Lotus is architected modularly to keep clean API boundaries while using the same process. Installing Lotus will include two seperate programs:

- The **Lotus Node** 
- The **Lotus Storage Miner**

The **Lotus Storage Miner** is intended to be run on the machine that manages a single storage miner instance, and is meant to communicate with the **Lotus Node** via the websockets **JSON RPC** API for all of the chain interaction needs.

This way, a mining operation may easily run a **Lotus Storage Miner** or many of them, connected to one or many **Lotus Node** instances.

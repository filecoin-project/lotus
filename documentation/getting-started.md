# Lotus

Lotus is an experimental implementation of the **Filecoin Distributed Storage Network**. For more details about Filecoin, check out the [Filecoin Spec](https://github.com/filecoin-project/specs).

## What can I learn here?

- If you want to [store](https://docs.lotu.sh/storing-data) or [retrieve](https://docs.lotu.sh/retrieving-data) data, you can install Lotus on [Arch Linux](https://docs.lotu.sh/install-lotus-arch), [Ubuntu](https://docs.lotu.sh/install-lotus-ubuntu), or [MacOS](https://docs.lotu.sh/install-lotus-macos) and get started.
- Learn how to join the DevNet with the [command line interface](https://docs.lotu.sh/join-devnet-cli).
- Learn how to test Lotus in a seperate local network using a [GUI](https://docs.lotu.sh/testing-with-gui).
- Learn how to mine Filecoin using the `lotus-storage-miner` in the [CLI](https://docs.lotu.sh/mining)

## What makes Lotus different?

Lotus is architected modularly to keep clean API boundaries while using the same process. The **Lotus Full Node** and the **Lotus Storage Miner** are two separate programs.

The **Lotus Storage Miner** is intended to be run on the machine that manages a single storage miner instance, and is meant to communicate with the full node via the websockets JSON RPC API for all of its chain interaction needs.

This way, a mining operation may easily run one or many storage miners, connected to one or many full node instances.

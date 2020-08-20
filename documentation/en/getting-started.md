# Lotus

Lotus is an implementation of the **Filecoin Distributed Storage Network**. You can run the Lotus software client to join the **Filecoin Testnet**.

For more details about Filecoin, check out the [Filecoin Docs](https://docs.filecoin.io) and [Filecoin Spec](https://filecoin-project.github.io/specs/).

## What can I learn here?

- How to install Lotus on [Arch Linux](https://docs.lotu.sh/en+install-lotus-arch), [Ubuntu](https://docs.lotu.sh/en+install-lotus-ubuntu), or [MacOS](https://docs.lotu.sh/en+install-lotus-macos).
- Joining the [Lotus Testnet](https://docs.lotu.sh/en+join-testnet).
- [Storing](https://docs.lotu.sh/en+storing-data) or [retrieving](https://docs.lotu.sh/en+retrieving-data) data.
- Mining Filecoin using the **Lotus Miner** in your [CLI](https://docs.lotu.sh/en+mining).

## How is Lotus designed?

Lotus is architected modularly to keep clean API boundaries while using the same process. Installing Lotus will include two separate programs:

- The **Lotus Node**
- The **Lotus Miner**

The **Lotus Miner** is intended to be run on the machine that manages a single miner instance, and is meant to communicate with the **Lotus Node** via the websocket **JSON-RPC** API for all of the chain interaction needs.

This way, a mining operation may easily run a **Lotus Miner** or many of them, connected to one or many **Lotus Node** instances.

# Project Lotus - 莲

Lotus is an experimental implementation of the **Filecoin Distributed Storage Network**. For more details about Filecoin, check out the [Filecoin Spec](https://github.com/filecoin-project/specs).

## Background

Lotus is architected modularly, and aims to keep clean API boundaries between everything, even if they are in the same process. Notably, the 'lotus full node' software, and the 'lotus storage miner' software are two separate programs.

The **Lotus Storage Miner** is intended to be run on the machine that manages a single storage miner instance, and is meant to communicate with the full node via the websockets JSON RPC API for all of its chain interaction needs. This way, a mining operation may easily run one or many storage miners, connected to one or many full node instances.

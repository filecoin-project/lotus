# Protocol Labs Standard Testing Configuration

> This documentation page describes the standard testing configuration the Protocol Labs team has used to test **Lotus Storage Miner**s on Lotus. There is no guarantee this testing configuration will be suitable for Filecoin storage mining at MainNet launch. If you need to buy new hardware to join the Filecoin Testnet, we recommend to buy no more hardware than you require for testing. To learn more please read this [Protocol Labs Standard Testing Configuration post](https://filecoin.io/blog/filecoin-testnet-mining/).

**Sector sizes** and **minimum pledged storage** required to mine blocks are two very important Filecoin Testnet parameters that impact hardware decisions. We will continue to refine all parameters during Testnet.

BECAUSE OF THIS, OUR STANDARD TESTING CONFIGURATION FOR FILECOIN MAINNET CAN AND WILL CHANGE. YOU HAVE BEEN WARNED.

## Example configuration

The setup below is a minimal example for sealing 32 GiB sectors on Lotus:

- 2 TB of hard drive space.
- 8 core CPU
- 128 GiB of RAM

Note that 1GB sectors don't require as high of specs, but are likely to be removed as we improve the performance of 32GB sector sealing.

## Testnet discoveries

- If you only have 128GiB of ram, enabling 256GB of **NVMe** swap on an SSD will help you avoid out-of-memory issues while mining.

## Benchmarked GPUs

GPUs are a must for getting **block rewards**. Here are a few that have been confirmed to generate **SNARKs** quickly enough to successfully mine blocks on the Lotus Testnet.

- GeForce RTX 2080 Ti
- GeForce RTX 2080 SUPER
- GeForce RTX 2080
- GeForce GTX 1080 Ti
- GeForce GTX 1080
- GeForce GTX 1060

## Testing other GPUs

If you want to test a GPU that is not explicitly supported, use the following global**environment variable**:

```sh
BELLMAN_CUSTOM_GPU="<NAME>:<NUMBER_OF_CORES>"
```

Here is an example of trying a GeForce GTX 1660 Ti with 1536 cores.

```sh
BELLMAN_CUSTOM_GPU="GeForce GTX 1660 Ti:1536"
```

To get the number of cores for your GPU, you will need to check your card’s specifications.

## Benchmarking

Here is a [benchmarking tool](https://github.com/filecoin-project/lotus/tree/testnet-staging/cmd/lotus-bench) and a [GitHub issue thread](https://github.com/filecoin-project/lotus/issues/694) for those who wish to experiment with and contribute hardware setups for the **Filecoin Testnet**.

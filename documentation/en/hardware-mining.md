# Mining Hardware

> This documentation page describes the standard testing configuration the Protocol Labs team has used to test **Lotus Storage Miner**s on Lotus. There is no guarantee this testing configuration will be suitable for Filecoin storage mining at MainNet launch. If you need to buy new hardware to join the Filecoin TestNet, we recommend to buy no more hardware than you require for testing. To learn more please read this [hardware blog post](https://filecoin.io/blog/filecoin-testnet-mining/)

**Sector sizes** and **minimum pledged storage** required to mine blocks are two very important Filecoin TestNet parameters that impact hardware decisions. We will continue to refine all parameters during TestNet. 

BECAUSE OF THIS, OUR STANDARD TESTING CONFIGURATION FOR FILECOIN MAINNET CAN AND WILL CHANGE. YOU HAVE BEEN WARNED.

## Example configuration

The setup below is a minimal example for sealing 32 GiB sectors on Lotus:

* 3 TB of hard drive space.
* 8 core CPU
* 128 GB of RAM

## TestNet discoveries

* 256GB **NVMe** Swap on an SSD for anyone that has 128GB RAM to avoid out of memory issues while mining.

## Benchmarked GPUs

GPUs are a must for getting **block rewards**. Here are a few that have been tried in the past:

* GeForce RTX 2080 Ti
* GeForce RTX 2080 SUPER
* GeForce RTX 2080
* GeForce GTX 1080 Ti
* GeForce GTX 1080
* GeForce GTX 1060

## Testing other GPUs

If you want to test other GPUs, such as the GeForce GTX 1660, you can use the following configuration flag:

```sh
BELLMAN_CUSTOM_GPU="<NAME>:<NUMBER_OF_CORES>"
```

Here is an example of trying a GeForce GTX 1660 ti with 1536 cores.

```sh
BELLMAN_CUSTOM_GPU="GeForce GTX 1660 Ti:1536"
```

To get the number of cores for your GPU, you will need to check your cards specifications.

## Benchmarking

Here is a [benchmarking tool](https://github.com/filecoin-project/lotus/tree/testnet-staging/cmd/lotus-bench) and a [GitHub issue thread](https://github.com/filecoin-project/lotus/issues/694) for those who wish to experiment with and contribute hardware setups for the **Filecoin TestNet**.
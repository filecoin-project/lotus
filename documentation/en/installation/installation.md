# Installation

Lotus can be installed in [Linux](en-install-linux) and [MacOS](en-install-macos) machines by building it from source. Windows is not supported yet.

This section contains guides to install Lotus in the supported platforms.

Lotus is made of 3 binaries:

* `lotus`: the main [Lotus node](en+setup) (Lotus client)
* `lotus-miner`: an application specifically for [Filecoin mining](en+miner-setup)
* `lotus-worker`: an additional [application to offload some heavy-processing tasks](en+lotus-worker) from the Lotus Miner.

These applications are written in Go, but also import several Rust libraries. Lotus does not distribute
pre-compiled builds.

## Hardware requirements

### For client nodes

* 8GiB of RAM
* Recommended for syncing speed: CPU with support for *Intel SHA Extensions* (AMD since Zen microarchitecture, Intel since Ice Lake).
* Recommended for speed: SSD hard drive (the bigger the better)

### For miners

The following correspond to the latest testing configuration:

* 2 TB of hard drive space
* 8 core CPU
* 128 GiB of RAM with 256 GiB of NVMe SSD storage for swap (or simply, more RAM).
* Recommended for speed: CPU with support for *Intel SHA Extensions* (AMD since Zen microarchitecture, Intel since Ice Lake).
* GPU for block mining. The following have been [confirmed to be fast enough](en+gpus):

- GeForce RTX 2080 Ti
- GeForce RTX 2080 SUPER
- GeForce RTX 2080
- GeForce GTX 1080 Ti
- GeForce GTX 1080
- GeForce GTX 1060

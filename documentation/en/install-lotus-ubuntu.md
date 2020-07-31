# Ubuntu Instructions

These steps will install the following dependencies:

- go (1.14 or higher)
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

### Install dependencies

```sh
sudo apt update
sudo apt install mesa-opencl-icd ocl-icd-opencl-dev gcc git bzr jq pkg-config curl
sudo apt upgrade
```

### Install Go 1.14

Install the latest version of Go by following [the docs on their website](https://golang.org/doc/install).

### Clone the Lotus repository

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

### Build the Lotus binaries from source and install

```sh
make clean && make all
sudo make install
```

After installing Lotus, you can run the `lotus` command directly from your CLI to see usage documentation. Next, you can join the [Lotus Testnet](https://docs.lotu.sh/en+join-testnet).

### Interopnet

If you seek a smaller network to test, you can join the `interopnet`. Please note that this network is meant for developers - it resets much more often, and is much smaller. To join this network, checkout the branch `interopnet` instead of `master` before building and installing;
```
git checkout interopnet
```

Please also note that this documentation (if viewed on the website) might not be up to date with the interopnet. For the latest documentation on the interopnet branch, see the [Lotus Documentation Interopnet Branch on GitHub](https://github.com/filecoin-project/lotus/tree/interopnet/documentation/en)

# Ubuntu Instructions

These steps will install the following dependencies:

- go (1.13 or higher)
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

Run

```sh
sudo apt update
sudo apt install mesa-opencl-icd ocl-icd-opencl-dev
```

Build

```sh
sudo add-apt-repository ppa:longsleep/golang-backports
sudo apt update
sudo apt install golang-go gcc git bzr jq pkg-config mesa-opencl-icd ocl-icd-opencl-dev
```

Clone

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

Install

```sh
make clean && make all
sudo make install
```

After installing Lotus, you can run the `lotus` command directly from your CLI to see usage documentation. Next, you can join the [Lotus Testnet](https://docs.lotu.sh/en+join-testnet).

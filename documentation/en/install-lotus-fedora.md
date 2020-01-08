# Fedora Instructions

> tested on 30

**NOTE:** If you have an AMD GPU the opencl instructions may be incorrect...

These steps will install the following dependencies:

- go (1.13 or higher)
- gcc (7.4.0 or higher)
- git (version 2 or higher)
- bzr (some go dependency needs this)
- jq
- pkg-config
- rustup (proofs build)
- llvm (proofs build)
- clang (proofs build)

Run

```sh
$ sudo dnf -y update
$ sudo dnf -y install go gcc git bzr jq pkgconfig mesa-libOpenCL mesa-libOpenCL-devel opencl-headers ocl-icd ocl-icd-devel clang llvm
$ curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

Clone

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

Install

```sh
$ make clean && make all
$ sudo make install
```

After installing Lotus, you can run the `lotus` command directly from your CLI to see usage documentation. Next, you can join the [Lotus TestNet](https://docs.lotu.sh/en+join-testnet).

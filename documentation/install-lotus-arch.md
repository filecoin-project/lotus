# Installing Lotus on Arch Linux

Install these dependencies for Arch Linux.

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

Arch (run):

```sh
sudo pacman -Syu opencl-icd-loader
```

Arch (build):

```sh
sudo pacman -Syu go gcc git bzr jq pkg-config opencl-icd-loader opencl-headers
```

Clone

```sh
$ git clone https://github.com/filecoin-project/lotus.git
$ cd lotus/
```

Install

```sh
$ make clean all
$ sudo make install
```

Now you can use the command `lotus` in the command line.

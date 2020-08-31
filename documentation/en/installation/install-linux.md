# Linux installation

This page will show you the steps to build and install Lotus in your Linux computer.

## Dependencies

### System dependencies

First of all, building Lotus will require installing some system dependencies, usually provided by your distribution.

For Arch Linux:

```sh
sudo pacman -Syu opencl-icd-loader gcc git bzr jq pkg-config opencl-icd-loader opencl-headers
```

For Ubuntu:

```sh
sudo apt update
sudo apt install mesa-opencl-icd ocl-icd-opencl-dev gcc git bzr jq pkg-config curl
sudo apt upgrade
```

For Fedora:

```sh
sudo dnf -y update
sudo dnf -y install gcc git bzr jq pkgconfig mesa-libOpenCL mesa-libOpenCL-devel opencl-headers ocl-icd ocl-icd-devel clang llvm
```

For OpenSUSE:

```sh
sudo zypper in gcc git jq make libOpenCL1 opencl-headers ocl-icd-devel clang llvm
sudo ln -s /usr/lib64/libOpenCL.so.1 /usr/lib64/libOpenCL.so
```

### Rustup

Lotus needs [rustup](https://rustup.rs/):

```sh
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

Please make sure your `$PATH` variable is correctly configured after the rustup installation so that `cargo` and `rustc` are found in their rustup-configured locations.

### Go

To build lotus you will need a working installation of **[Go1.14](https://golang.org/dl/)**. Follow the [installation instructions](https://golang.org/doc/install), which generally amount to:

```sh
# Example! Check the installation instructions.
wget -c https://dl.google.com/go/go1.14.7.linux-amd64.tar.gz -O - | sudo tar -xz -C /usr/local
```

## Build and install Lotus

With all the above, you are ready to build and install the Lotus suite (`lotus`, `lotus-miner` and `lotus-worker`):

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

__IF YOU ARE IN CHINA__, set `export GOPROXY=https://goproxy.cn` before building

Now, choose the network that you will be joining:

* For `testnet`: `git checkout master`
* For `nerpa`: `git checkout ntwk-nerpa`
* For `butterfly`: `git checkout ntwk-butterfly`

Once on the right branch, do:

```sh
make clean install
sudo make install
```

This will put `lotus`, `lotus-miner` and `lotus-worker` in `/usr/local/bin`. `lotus` will use the `$HOME/.lotus` folder by default for storage (configuration, chain data, wallets...). `lotus-miner` will use `$HOME/.lotusminer` respectively. See the *environment variables* section below for how to customize these.

> Remeber to [move your Lotus folder](en+update) if you are switching between different networks, or there has been a network reset.


### Native Filecoin FFI

Some newer processors (AMD Zen (and later), Intel Ice Lake) have support SHA extensions. To make full use of your processor's capabilities, make sure you set the following variables BEFORE building from source (as described above):

```sh
export RUSTFLAGS="-C target-cpu=native -g"
export FFI_BUILD_FROM_SOURCE=1
```

> __NOTE__: This method of building does not produce portable binaries! Make sure you run the binary in the same machine as you built it.

### systemd service files

Lotus provides Systemd service files. They can be installed with:

```sh
make install-daemon-service
make install-miner-service
```

After that, you should be able to control Lotus using `systemctl`.

## Troubleshooting

This section mentions some of the common pitfalls for building Lotus. Check the [getting started](en+getting-started) section for more tips on issues when running the lotus daemon.

### Build errors

Please check the build logs closely. If you have a dirty state in your git branch make sure to:

```sh
git checkout <desired_branch>
git reset origin/<desired_branch> --hard
make clean
```

### Slow builds from China

Users from China can speed up their builds by setting:

```sh
export GOPROXY=https://goproxy.cn
```

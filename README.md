<p align="center">
  <a href="https://lotus.filecoin.io/" title="Filecoin Docs">
    <img src="documentation/images/lotus_logo_h.png" alt="Project Lotus Logo" width="244" />
  </a>
</p>

<h1 align="center">Project Lotus - 莲</h1>

<p align="center">
  <a href="https://circleci.com/gh/filecoin-project/lotus"><img src="https://circleci.com/gh/filecoin-project/lotus.svg?style=svg"></a>
  <a href="https://codecov.io/gh/filecoin-project/lotus"><img src="https://codecov.io/gh/filecoin-project/lotus/branch/master/graph/badge.svg"></a>
  <a href="https://goreportcard.com/report/github.com/filecoin-project/lotus"><img src="https://goreportcard.com/badge/github.com/filecoin-project/lotus" /></a>  
  <a href=""><img src="https://img.shields.io/badge/golang-%3E%3D1.18.8-blue.svg" /></a>
  <br>
</p>

Lotus is an implementation of the Filecoin Distributed Storage Network. For more details about Filecoin, check out the [Filecoin Spec](https://spec.filecoin.io).

## Building & Documentation

> Note: The default `master` branch is the dev branch, please use with caution. For the latest stable version, checkout the most recent [`Latest release`](https://github.com/filecoin-project/lotus/releases).
 
For complete instructions on how to build, install and setup lotus, please visit [https://lotus.filecoin.io](https://lotus.filecoin.io/lotus/install/prerequisites/#supported-platforms). Basic build instructions can be found further down in this readme.

## Reporting a Vulnerability

Please send an email to security@filecoin.org. See our [security policy](SECURITY.md) for more details.

## Related packages

These repos are independent and reusable modules, but are tightly integrated into Lotus to make up a fully featured Filecoin implementation:

- [go-fil-markets](https://github.com/filecoin-project/go-fil-markets) which has its own [kanban work tracker available here](https://app.zenhub.com/workspaces/markets-shared-components-5daa144a7046a60001c6e253/board)
- [builtin-actors](https://github.com/filecoin-project/builtin-actors)

## Contribute

Lotus is a universally open project and welcomes contributions of all kinds: code, docs, and more. However, before making a contribution, we ask you to heed these recommendations:

1. If the proposal entails a protocol change, please first submit a [Filecoin Improvement Proposal](https://github.com/filecoin-project/FIPs).
2. If the change is complex and requires prior discussion, [open an issue](github.com/filecoin-project/lotus/issues) or a [discussion](https://github.com/filecoin-project/lotus/discussions) to request feedback before you start working on a pull request. This is to avoid disappointment and sunk costs, in case the change is not actually needed or accepted.
3. Please refrain from submitting PRs to adapt existing code to subjective preferences. The changeset should contain functional or technical improvements/enhancements, bug fixes, new features, or some other clear material contribution. Simple stylistic changes are likely to be rejected in order to reduce code churn.

When implementing a change:

1. Adhere to the standard Go formatting guidelines, e.g. [Effective Go](https://golang.org/doc/effective_go.html). Run `go fmt`.
2. Stick to the idioms and patterns used in the codebase. Familiar-looking code has a higher chance of being accepted than eerie code. Pay attention to commonly used variable and parameter names, avoidance of naked returns, error handling patterns, etc.
3. Comments: follow the advice on the [Commentary](https://golang.org/doc/effective_go.html#commentary) section of Effective Go.
4. Minimize code churn. Modify only what is strictly necessary. Well-encapsulated changesets will get a quicker response from maintainers.
5. Lint your code with [`golangci-lint`](https://golangci-lint.run) (CI will reject your PR if unlinted).
6. Add tests.
7. Title the PR in a meaningful way and describe the rationale and the thought process in the PR description.
8. Write clean, thoughtful, and detailed [commit messages](https://chris.beams.io/posts/git-commit/). This is even more important than the PR description, because commit messages are stored _inside_ the Git history. One good rule is: if you are happy posting the commit message as the PR description, then it's a good commit message.

## Basic Build Instructions
**System-specific Software Dependencies**:

Building Lotus requires some system dependencies, usually provided by your distribution.

Ubuntu/Debian:
```
sudo apt install mesa-opencl-icd ocl-icd-opencl-dev gcc git bzr jq pkg-config curl clang build-essential hwloc libhwloc-dev wget -y && sudo apt upgrade -y
```

Fedora:
```
sudo dnf -y install gcc make git bzr jq pkgconfig mesa-libOpenCL mesa-libOpenCL-devel opencl-headers ocl-icd ocl-icd-devel clang llvm wget hwloc hwloc-devel
```

For other distributions you can find the required dependencies [here.](https://lotus.filecoin.io/lotus/install/prerequisites/#supported-platforms) For instructions specific to macOS, you can find them [here.](https://lotus.filecoin.io/lotus/install/macos/)

#### Go

To build Lotus, you need a working installation of [Go 1.18.8 or higher](https://golang.org/dl/):

```bash
wget -c https://golang.org/dl/go1.18.8.linux-amd64.tar.gz -O - | sudo tar -xz -C /usr/local
```

**TIP:**
You'll need to add `/usr/local/go/bin` to your path. For most Linux distributions you can run something like:

```shell
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.bashrc && source ~/.bashrc
```

See the [official Golang installation instructions](https://golang.org/doc/install) if you get stuck.

### Build and install Lotus

Once all the dependencies are installed, you can build and install the Lotus suite (`lotus`, `lotus-miner`, and `lotus-worker`).

1. Clone the repository:

   ```sh
   git clone https://github.com/filecoin-project/lotus.git
   cd lotus/
   ```
   
Note: The default branch `master` is the dev branch where the latest new features, bug fixes and improvement are in. However, if you want to run lotus on Filecoin mainnet and want to run a production-ready lotus, get the latest release[ here](https://github.com/filecoin-project/lotus/releases).

2. To join mainnet, checkout the [latest release](https://github.com/filecoin-project/lotus/releases).

   If you are changing networks from a previous Lotus installation or there has been a network reset, read the [Switch networks guide](https://lotus.filecoin.io/lotus/manage/switch-networks/) before proceeding.

   For networks other than mainnet, look up the current branch or tag/commit for the network you want to join in the [Filecoin networks dashboard](https://network.filecoin.io), then build Lotus for your specific network below.

   ```sh
   git checkout <tag_or_branch>
   # For example:
   git checkout <vX.X.X> # tag for a release
   ```

   Currently, the latest code on the _master_ branch corresponds to mainnet.

3. If you are in China, see "[Lotus: tips when running in China](https://lotus.filecoin.io/lotus/configure/nodes-in-china/)".
4. This build instruction uses the prebuilt proofs binaries. If you want to build the proof binaries from source check the [complete instructions](https://lotus.filecoin.io/lotus/install/prerequisites/). Note, if you are building the proof binaries from source, [installing rustup](https://lotus.filecoin.io/lotus/install/linux/#rustup) is also needed.

5. Build and install Lotus:

   ```sh
   make clean all #mainnet

   # Or to join a testnet or devnet:
   make clean calibnet # Calibration with min 32GiB sectors

   sudo make install
   ```

   This will put `lotus`, `lotus-miner` and `lotus-worker` in `/usr/local/bin`.

   `lotus` will use the `$HOME/.lotus` folder by default for storage (configuration, chain data, wallets, etc). See [advanced options](https://lotus.filecoin.io/lotus/configure/defaults/#environment-variables) for information on how to customize the Lotus folder.

6. You should now have Lotus installed. You can now [start the Lotus daemon and sync the chain](https://lotus.filecoin.io/lotus/install/linux/#start-the-lotus-daemon-and-sync-the-chain).

## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)

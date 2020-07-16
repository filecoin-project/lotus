# MacOS Instructions

With xcode and brew installed and up to date on your machine run the following:

```sh
# install deps
brew install go bzr jq pkg-config rustup

# run rustup-init - required after first install of rustup.
rustup-init

# clone the repo
git clone https://github.com/filecoin-project/lotus.git

# clean and build lotus, run from the project root
cd lotus/
make clean && make all

# optional: install the lotus command on your PATH
sudo make install
```

After installing Lotus, you can run the `lotus` command directly from your CLI to see usage documentation. Next, you can join the [Lotus Testnet](https://docs.lotu.sh/en+join-testnet).

The rest of this document goes into more detail on the steps listed above.

## Get XCode Command Line Tools

To check if you already have the XCode Command Line Tools installed via the CLI, run:

```sh
xcode-select -p
```

If this command returns a path, you can move on to the next step. Otherwise, to install via the CLI, run:

```sh
xcode-select --install
```

To update, run:

```sh
sudo rm -rf /Library/Developer/CommandLineTools
xcode-select --install
```

## Get HomeBrew

We recommend that MacOS users use [HomeBrew](https://brew.sh) to install each the necessary packages.

Check if you have HomeBrew:

```sh
brew -v
```

This command returns a version number if you have HomeBrew installed and nothing otherwise.

In your terminal, enter this command to install Homebrew:

```sh
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
```

## Install dependencies via HomeBrew

Use the command `brew install` to install the rest of the tools needed to build the lotus binary:

```sh
brew install go bzr jq pkg-config rustup
```

If you are installing `rustup` for the first time, you will need to run `rustup-init` once to set up the rust toolchain.

```sh
rustup-init
```

Clone the repo:

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

Run the `make` tasks to clean and build lotus, then install it on your PATH:

```sh
make clean && make all
sudo make install
```

## Common issues

### `failed to fetch https://github.com/rust-lang/crates.io-index`

If you hit an error during the `make` step where [`cargo` is unable to fetch repos](https://github.com/rust-lang/cargo/issues/2078), a workaround is to change your cargo config to use the git cli to fetch repos, including the following in `~/.cargo/config`
```ini
[net]
git-fetch-with-cli = true
```

See: https://github.com/rust-lang/cargo/issues/2078


## Next steps

After installing Lotus, you can run the `lotus` command directly from your CLI to see usage documentation. Next, you can join the [Lotus Testnet](https://docs.lotu.sh/en+join-testnet).

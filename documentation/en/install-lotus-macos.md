# MacOS Instructions

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

Use the command `brew install` to install the following packages:

```sh
brew install go bzr jq pkg-config rustup
```

Clone

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

Build

```sh
make clean && make all
sudo make install
```

After installing Lotus, you can run the `lotus` command directly from your CLI to see usage documentation. Next, you can join the [Lotus Testnet](https://docs.lotu.sh/en+join-testnet).

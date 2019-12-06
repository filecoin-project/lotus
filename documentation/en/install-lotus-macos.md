# MacOS Instructions

We recommend for MacOS users to use [HomeBrew](https://brew.sh/) to install each package.

## HomeBrew installation

Run `terminal.app` and enter this command:

```sh
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
```

Use the command `brew install` to install the following packages:

```sh
brew install go
brew install bzr
brew install jq
brew install pkg-config
brew install rustup
```

## Clone

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

## Build

```sh
make clean all
sudo make install
```

Now you can use the command `lotus` in the CLI and join the [Lotus DevNet](https://docs.lotu.sh/en+join-devnet).

## Troubleshooting

You may run into problems with running `lotus daemon`, here are some common cases:

```sh
WARN  peermgr peermgr/peermgr.go:131  failed to connect to bootstrap peer: failed to dial : all dials failed
  * [/ip4/147.75.80.17/tcp/1347] failed to negotiate security protocol: connected to wrong peer
```

* Try running the build steps again and make sure that you have the latest code from GitHub.

```sh
ERROR hello hello/hello.go:81 other peer has different genesis!
```

* Try deleting your file system's `~/.lotus` directory. Check that it exists with `ls ~/.lotus`.

## Explanations

Some errors will occur that do not prevent Lotus from working:

```sh
ERROR chainstore  store/store.go:564  get message get failed: <Data CID>: blockstore: block not found

```

* Someone is requesting a **Data CID** from you that you don't have.
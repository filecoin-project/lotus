# Setting up Lotus

Your Lotus binaries have been installed and you are ready to start participating in the Filecoin network.

## Selecting the right network

You should have built the Lotus binaries from the right Github branch and Lotus will be fully setup to join the matching [Filecoin network](https://docs.filecoin.io/how-to/networks/). For more information on switching networks, check the [updating Lotus section](en+update).

## Starting the daemon

To start the daemon simply run:

```sh
lotus daemon
```

or if you are using the provided systemd service files, do:

```sh
systemctl start lotus-daemon
```

__If you are using Lotus from China__, make sure you set the following environment variable before running Lotus:

```
export IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```


During the first start, Lotus:

* Will setup its data folder at `~/.lotus`
* Will download the necessary parameters
* Start syncing the Lotus chain

If you started lotus using systemd, the logs will appear in `/var/log/lotus/daemon.log` (not in journalctl as usual), otherwise you will see them in your screen.

Do not be appalled by the amount of warnings and sometimes errors showing in the logs, there are usually part of the usual functioning of the daemon as part of a distributed network.

## Waiting to sync

After the first start, the chain will start syncing until it has reached the tip. You can check how far the syncing process is with:

```sh
lotus sync status
```

You can also interactively wait for the chain to be fully synced with:

```sh
lotus sync wait
```

## Interacting with the Lotus daemon

As shown above, the `lotus` command allows to interact with the running daemon. You will see it getting used in many of the documentation examples.

This command-line-interface is self-documenting:

```sh
# Show general help
lotus --help
# Show specific help for the "client" subcommand
lotus client --help
```

For example, after your Lotus daemon has been running for a few minutes, use `lotus` to check the number of other peers that it is connected to in the Filecoin network:

```sh
lotus net peers
```

## Controlling the logging level

```sh
lotus log set-level
```
This command can be used to toggle the logging levels of the different
systems of a Lotus node. In decreasing order
of logging detail, the levels are `debug`, `info`, `warn`, and `error`. 

As an example,
to set the `chain` and `blocksync` to log at the `debug` level, run 
`lotus log set-level --system chain --system blocksync debug`. 

To see the various logging system, run `lotus log list`.


## Configuration

### Configuration file

The Lotus daemon stores a configuration file in `~/.lotus/config.toml`. Note that by default all settings are commented. Here is an example configuration:

```toml
[API]
  # Binding address for the Lotus API
  ListenAddress = "/ip4/127.0.0.1/tcp/1234/http"
  # Not used by lotus daemon
  RemoteListenAddress = ""
  # General network timeout value
  Timeout = "30s"

# Libp2p provides connectivity to other Filecoin network nodes
[Libp2p]
  # Binding address swarm - 0 means random port.
  ListenAddresses = ["/ip4/0.0.0.0/tcp/0", "/ip6/::/tcp/0"]
  # Insert any addresses you want to explicitally
  # announce to other peers here. Otherwise, they are
  # guessed.
  AnnounceAddresses = []
  # Insert any addresses to avoid announcing here.
  NoAnnounceAddresses = []
  # Connection manager settings, decrease if your
  # machine is overwhelmed by connections.
  ConnMgrLow = 150
  ConnMgrHigh = 180
  ConnMgrGrace = "20s"

# Pubsub is used to broadcast information in the network
[Pubsub]
  Bootstrapper = false
  RemoteTracer = "/dns4/pubsub-tracer.filecoin.io/tcp/4001/p2p/QmTd6UvR47vUidRNZ1ZKXHrAFhqTJAD27rKL9XYghEKgKX"

# This section can be used to enable adding and retriving files from IPFS
[Client]
  UseIpfs = false
  IpfsMAddr = ""
  IpfsUseForRetrieval = false

# Metrics configuration
[Metrics]
  Nickname = ""
  HeadNotifs = false
```

### Ensuring connectivity to your Lotus daemon

Usually your lotus daemon will establish connectivity with others in the network and try to make itself diallable using uPnP. If you wish to manually ensure that your daemon is reachable:

* Set a fixed port of your choice in the `ListenAddresses` in the Libp2p section (i.e. 6665).
* Open a port in your router that is forwarded to this port. This is usually called featured as "Port forwarding" and the instructions differ from router model to model but there are many guides online.
* Add your public IP/port to `AnnounceAddresses`. i.e. `/ip4/<yourIP>/tcp/6665/`.

Note that it is not a requirement to use Lotus as a client to the network to be fully reachable, as your node already connects to others directly.


### Environment variables

Common to most Lotus binaries:

* `LOTUS_FD_MAX`: Sets the file descriptor limit for the process
* `LOTUS_JAEGER`: Sets the Jaeger URL to send traces. See TODO.
* `LOTUS_DEV`: Any non-empty value will enable more verbose logging, useful only for developers.

Specific to the *Lotus daemon*:

* `LOTUS_PATH`: Location to store Lotus data (defaults to `~/.lotus`).
* `LOTUS_SKIP_GENESIS_CHECK=_yes_`: Set only if you wish to run a lotus network with a different genesis block.
* `LOTUS_CHAIN_TIPSET_CACHE`: Sets the size for the chainstore tipset cache. Defaults to `8192`. Increase if you perform frequent arbitrary tipset lookups.
* `LOTUS_CHAIN_INDEX_CACHE`: Sets the size for the epoch index cache. Defaults to `32768`. Increase if you perform frequent deep chain lookups for block heights far from the latest height.
* `LOTUS_BSYNC_MSG_WINDOW`: Set the initial maximum window size for message fetching blocksync request. Set to 10-20 if you have an internet connection with low bandwidth.

Specific to the *Lotus miner*:

* `LOTUS_MINER_PATH`: Location for the miner's on-disk repo. Defaults to `./lotusminer`.
* A number of environment variables are respected for configuring the behaviour of the Filecoin proving subsystem. [See here](en+miner-setup).



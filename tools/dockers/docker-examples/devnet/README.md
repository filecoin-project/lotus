# Private DevNet Quickstart

Use this to quickly spin up a minimal private development network.
It includes a seed node and storage miner with 4KB debug parameters.

Requirements:
 - Docker
 - A machine with a public IP address

## Setup

#### Building the Docker image

```shell script
cd tools/dockers/docker-examples/devnet
make build
```

#### Starting up the network.

```shell script
make run
```

#### Retrieving login parameters

If you just started the network, you might see errors before all services are running.

```shell script
make login
```

#### Connecting to the network

To connect another (external) node to the DevNet,
a few things need to be configured.

##### Setting the bootstrap peer

The node needs to be instructed to connect to the DevNet.
The bootstrap multiaddr is listed in the `make login` script:

```shell script
lotus/tools/dockers/docker-examples/devnet $ make login

...
Seed Peer (up=true)
FULLNODE_API_INFO="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.BOQBl27LjMFgfgKeB-TJgBL7tsEW8oLycHzQ-YpWASo:/ip4/127.0.0.1/tcp/1234/http"
# P2P Connection: /ip4/127.0.0.1/tcp/5678
# Net ID: 12D3KooWFmRZcXx6rqWPsDAZMYxphMYpcjXBPY8kAbZDbX2voswt
# multiaddr: /ip4/127.0.0.1/tcp/5678/p2p/12D3KooWFmRZcXx6rqWPsDAZMYxphMYpcjXBPY8kAbZDbX2voswt
...
```

If the Docker container is on a different host than the external node,
the IP in the multiaddr needs to be replaced.

Then, set it up in the Lotus config file like so:

```toml
[Libp2p]
BootstrapPeers = ["/ip4/127.0.0.1/tcp/5678/p2p/12D3KooWFmRZcXx6rqWPsDAZMYxphMYpcjXBPY8kAbZDbX2voswt"]
```

##### Getting the genesis block

The generated genesis can be downloaded using `docker cp`:

```shell script
lotus $ docker cp filecoin-devnet:/lotus/dev.gen dev.gen
lotus $ ls ./dev.gen
```

##### Starting the node

```shell script
# Compile a Lotus node in debug mode
lotus $ make debug
...
lotus $ ./lotus daemon --genesis=dev.gen
```

#### Other commands

 - `make build`: Rebuilds the `filecoin-devnet` image.
 - `make stop`: Stops the network container.
 - `make restart`: Restarts the started or stopped container.
 - `make kill`: Kills the network container.
 - `make clean`: Deletes the network container.
 - `make clean-all`: Deletes everything, including images and volumes.
 - `docker exec -it filecoin-devnet bash`: Opens a shell inside the container.

# Lotus Seal Worker

The **Lotus Seal Worker** is an extra process that can offload heavy processing tasks from your **Lotus Storage Miner**. It can be run on the same machine as your `lotus-storage-miner`, or on another machine communicating over a fast network.

## Get Started

Make sure that the `lotus-seal-worker` is installed by running:

```sh
make lotus-seal-worker
```

## Running Alongside Storage Miner

You may wish to run the **Lotus Seal Worker** on the same computer as the **Lotus Storage Miner**. This allows you to easily set the process priority of the sealing tasks to be lower than the priority of your more important storage miner process.

To do this, simply run `lotus-seal-worker run`, and the seal worker will automatically pick up the correct authentication tokens from the `LOTUS_STORAGE_PATH` miner repository.

To check that the **Lotus Seal Worker** is properly connected to your storage miner, run `lotus-storage-miner info` and check that the remote worker count has increased.

```sh
why@computer ~/lotus> lotus-storage-miner info
Miner: t0103
Sector Size: 16.0 MiB
Power: 0 B / 16.0 MiB (0%)
Worker use:
        Local: 0 / 2 (+1 reserved)
        **Remote: 0 / 1**
PoSt Submissions: Not Proving
Sectors:  map[Committing:0 Proving:0 Total:0]
```

## Running Over the Network

Warning: This setup is a little more complex than running it locally.

To use an entirely separate computer for sealing tasks, you will want to run the `lotus-seal-worker` on a separate machine, connected to your **Lotus Storage Miner** via the local area network.

First, you will need to ensure your `lotus-storage-miner`'s API is accessible over the network.

To do this, open up `~/.lotusstorage/config.toml` (Or if you manually set `LOTUS_STORAGE_PATH`, look under that directory) and look for the API field.

Default config:

```toml
[API]
ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
```

To make your node accessible over the local area network, you will need to determine your machines IP on the LAN, and change the `127.0.0.1` in the file to that address.

A more permissive and less secure option is to change it to `0.0.0.0`. This will allow anyone who can connect to your computer on that port to access the [API](https://docs.lotu.sh/en+api). They will still need an auth token.

Next, you will need to [create an authentication token](https://docs.lotu.sh/en+api-scripting-support#generate-a-jwt-46). All Lotus APIs require authentication tokens to ensure your processes are as secure against attackers attempting to make unauthenticated requests to them.

### Connect the Lotus Seal Worker

On the machine that will run `lotus-seal-worker`, set the `STORAGE_API_INFO` environment variable to `TOKEN:STORAGE_NODE_MULTIADDR`. Where `TOKEN` is the token we created above, and `STORAGE_NODE_MULTIADDR` is the `multiaddr` of the **Lotus Storage Miner** API that was set in `config.toml`.

Once this is set, run:

```sh
lotus-seal-worker run
```

To check that the **Lotus Seal Worker** is connected to your **Lotus Storage Miner**, run `lotus-storage-miner info` and check that the remote worker count has increased.

```sh
why@computer ~/lotus> lotus-storage-miner info
Miner: t05749
Sector Size: 1 GiB
Power: 0 B / 136 TiB (0.0000%)
  Committed: 1 GiB
  Proving: 1 GiB
Worker use:
  Local: 0 / 1 (+1 reserved)
  **Remote: 0 / 1**
Sectors:  map[Proving:1 Total:1]
```

# Lotus Seal Worker

The `lotus-seal-worker` is an extra process that can offload heavy processing tasks from your `lotus-storage-miner`. It can be run on the same machine as your `lotus-storage-miner`, or on a different machine communicating over a fast network.

## Get Started
Make sure that the `lotus-seal-worker` is installed by running:

```sh
make lotus-seal-worker
```

## Running Alongside Storage Miner
You may wish to run the lotus seal worker on the same computer as the storage miner. This allows you to easily set the process priority of the sealing tasks to be lower than the priority of your more important storage miner process.

To do this, simply run `lotus-seal-worker run`, and the seal worker will automatically pick up the correct authentication tokens from the `LOTUS_STORAGE_PATH` miner repository.

To check that the seal worker is properly connected to your storage miner, run `lotus-storage-miner info` and check that the remote worker count has increased.

```
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
To use an entirely separate computer for sealing tasks, you will want to run the `lotus-seal-worker` on a separate machine, connected to your storage miner via the local area network.

This setup is a little more complex than running it locally.

First, you will need to ensure your `lotus-storage-miner`'s API is accessible over the network.

To do this, open up `~/.lotusstorage/config.toml` (Or if you manually set `LOTUS_STORAGE_PATH`, look under that directory) and look for the API field.

By default it should look something like:
```toml
[API]
ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
```

To make your node accessible over the local area network, you will need to determine your machines IP on the LAN, and change the `127.0.0.1` in the file to that address. A less secure, but more permissive option is to change it to `0.0.0.0`. This will allow anyone who can connect to your computer on that port to access the API (though they will still need an auth token, as we will discuss next).

Next, you will need to get an authentication token for the seal worker. All lotus APIs require authentication tokens to ensure your processes are as secure against attackers attempting to make unauthenticated requests to them. To create a token, run `lotus-storage-miner auth create-token --perm admin`. This will create a token with `admin` permissions. (TODO: does the seal worker need admin? or can we get away with less?) (if it does need admin powers, insert a warning here about how powerful this token is)

This token will look something like this:
```sh
why@WhyNet ~> lotus-storage-miner auth create-token --perm admin
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.KWWdh1jOVP_5YMAp8x5wNomFGgKS75ucOtj1ah5iP7k
```

Now that you have allowed the storage miner to be connected to, and have created an auth token, its time to connect up the seal worker.

On the machine that you will be running the `lotus-seal-worker` on, you will need to set the `STORAGE_API_INFO` environment variable to `TOKEN:STORAGE_NODE_MULTIADDR`. Where `TOKEN` is the token we created above, and `STORAGE_NODE_MULTIADDR` is the multiaddr of the storage miners api that we set in the config file.

Once this is set, you should be able to just run `lotus-seal-worker run`.

To check that the seal worker is properly connected to your storage miner, run `lotus-storage-miner info` and check that the remote worker count has increased.

TODO: sample output

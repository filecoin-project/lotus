# Lotus Worker

The **Lotus Worker** is an extra process that can offload heavy processing tasks from your **Lotus Miner**. The sealing process automatically runs in the **Lotus Miner** process, but you can use the Worker on another machine communicating over a fast network to free up resources on the machine running the mining process.

## Note: Using the Lotus Worker from China

If you are trying to use `lotus-worker` from China. You should set this **environment variable** on your machine:

```sh
export IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```

## Get Started

Make sure that the `lotus-worker` is compiled and installed by running:

```sh
make lotus-worker
```

## Setting up the Miner

First, you will need to ensure your `lotus-miner`'s API is accessible over the network.

To do this, open up `~/.lotusminer/config.toml` (Or if you manually set `LOTUS_MINER_PATH`, look under that directory) and look for the API field.

Default config:

```toml
[API]
ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
RemoteListenAddress = "127.0.0.1:2345"
```

To make your node accessible over the local area network, you will need to determine your machines IP on the LAN, and change the `127.0.0.1` in the file to that address.

A more permissive and less secure option is to change it to `0.0.0.0`. This will allow anyone who can connect to your computer on that port to access the [API](https://lotu.sh/en+api). They will still need an auth token.

`RemoteListenAddress` must be set to an address which other nodes on your network will be able to reach.

Next, you will need to [create an authentication token](https://lotu.sh/en+api-scripting-support#generate-a-jwt-46). All Lotus APIs require authentication tokens to ensure your processes are as secure against attackers attempting to make unauthenticated requests to them.

### Connect the Lotus Worker

On the machine that will run `lotus-worker`, set the `MINER_API_INFO` environment variable to `TOKEN:MINER_NODE_MULTIADDR`. Where `TOKEN` is the token we created above, and `NIMER_NODE_MULTIADDR` is the `multiaddr` of the **Lotus Miner** API that was set in `config.toml`.

Once this is set, run:

```sh
lotus-worker run
```

If you are running multiple workers on the same host, you will need to specify the `--listen` flag and ensure each worker is on a different port.

To check that the **Lotus Worker** is connected to your **Lotus Miner**, run `lotus-miner sealing workers` and check that the remote worker count has increased.

```sh
why@computer ~/lotus> lotus-miner sealing workers
Worker 0, host computer
        CPU:  [                                                                ] 0 core(s) in use
        RAM:  [||||||||||||||||||                                              ] 28% 18.1 GiB/62.7 GiB
        VMEM: [||||||||||||||||||                                              ] 28% 18.1 GiB/62.7 GiB
        GPU: GeForce RTX 2080, not used

Worker 1, host othercomputer
        CPU:  [                                                                ] 0 core(s) in use
        RAM:  [||||||||||||||                                                  ] 23% 14 GiB/62.7 GiB
        VMEM: [||||||||||||||                                                  ] 23% 14 GiB/62.7 GiB
        GPU: GeForce RTX 2080, not used
```

### Running locally for manually managing process priority

You can also run the **Lotus Worker** on the same machine as your **Lotus Miner**, so you can manually manage the process priority.
To do so you have to first __disable all seal task types__ in the miner config. This is important to prevent conflicts between the two processes.

You can then run the miner on your local-loopback interface; 

```sh
lotus-worker run
```

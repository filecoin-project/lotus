# Lotus Seal Worker

The **Lotus Seal Worker** is an extra process that can offload heavy processing tasks from your **Lotus Storage Miner**. The sealing process automatically runs in the **Lotus Storage Miner** process, but you can use the Seal Worker on another machine communicating over a fast network to free up resources on the machine running the mining process.

## Note: Using the Lotus Seal Worker from China

If you are trying to use `lotus-seal-worker` from China. You should set this **environment variable** on your machine:

```sh
IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```

## Get Started

Make sure that the `lotus-seal-worker` is compiled and installed by running:

```sh
make lotus-seal-worker
```

## Setting up the Storage Miner

First, you will need to ensure your `lotus-storage-miner`'s API is accessible over the network.

To do this, open up `~/.lotusstorage/config.toml` (Or if you manually set `LOTUS_STORAGE_PATH`, look under that directory) and look for the API field.

Default config:

```toml
[API]
ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
RemoteListenAddress = "127.0.0.1:2345"
```

To make your node accessible over the local area network, you will need to determine your machines IP on the LAN, and change the `127.0.0.1` in the file to that address.

A more permissive and less secure option is to change it to `0.0.0.0`. This will allow anyone who can connect to your computer on that port to access the [API](https://docs.lotu.sh/en+api). They will still need an auth token.

`RemoteListenAddress` must be set to an address which other nodes on your network will be able to reach.

Next, you will need to [create an authentication token](https://docs.lotu.sh/en+api-scripting-support#generate-a-jwt-46). All Lotus APIs require authentication tokens to ensure your processes are as secure against attackers attempting to make unauthenticated requests to them.

### Connect the Lotus Seal Worker

On the machine that will run `lotus-seal-worker`, set the `STORAGE_API_INFO` environment variable to `TOKEN:STORAGE_NODE_MULTIADDR`. Where `TOKEN` is the token we created above, and `STORAGE_NODE_MULTIADDR` is the `multiaddr` of the **Lotus Storage Miner** API that was set in `config.toml`.

Once this is set, run:

```sh
lotus-seal-worker run --address 192.168.2.10:2345
```

Replace `192.168.2.10:2345` with the proper IP and port.

To check that the **Lotus Seal Worker** is connected to your **Lotus Storage Miner**, run `lotus-storage-miner workers list` and check that the remote worker count has increased.

```sh
why@computer ~/lotus> lotus-storage-miner workers list
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

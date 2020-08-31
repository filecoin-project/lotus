# Lotus Worker

The **Lotus Worker** is an extra process that can offload heavy processing tasks from your **Lotus Miner**. The sealing process automatically runs in the **Lotus Miner** process, but you can use the Worker on another machine communicating over a fast network to free up resources on the machine running the mining process.

## Installation

The `lotus-worker` application is installed along with the others when running `sudo make install` as shown in the [Installation section](en+install-linux). For simplicity, we recommend following the same procedure in the machines that will run the Lotus Workers (even if the Lotus miner and the Lotus daemon are not used there).

## Setting up the Miner

### Allow external connections to the miner API

First, you will need to ensure your `lotus-miner`'s API is accessible over the network.

To do this, open up `~/.lotusminer/config.toml` (Or if you manually set `LOTUS_MINER_PATH`, look under that directory) and look for the API field.

Default config:

```toml
[API]
ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
RemoteListenAddress = "127.0.0.1:2345"
```

To make your node accessible over the local area network, you will need to determine your machine's IP on the LAN (`ip a`), and change the `127.0.0.1` in the file to that address.

A more permissive and less secure option is to change it to `0.0.0.0`. This will allow anyone who can connect to your computer on that port to access the miner's API, though they will still need an auth token.

`RemoteListenAddress` must be set to an address which other nodes on your network will be able to reach.

### Create an authentication token

Write down the output of:

```sh
lotus-miner auth api-info --perm admin
```

The Lotus Workers will need this token to connect to the miner.

## Connecting the Lotus Workers

On each machine that will run the `lotus-worker` application you will need to define the following *environment variable*:

```sh
export MINER_API_INFO:<TOKEN>:/ip4/<miner_api_address>/tcp/2345`
```

If you are trying to use `lotus-worker` from China. You should additionally set:

```sh
export IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```


Once that is done, you can run the Worker with:

```sh
lotus-worker run
```

> If you are running multiple workers on the same host, you will need to specify the `--listen` flag and ensure each worker is on a different port.

On your Lotus miner, check that the workers are correctly connected:

```sh
lotus-miner sealing workers
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

## Running locally for manually managing process priority

You can also run the **Lotus Worker** on the same machine as your **Lotus Miner**, so you can manually manage the process priority.

To do so you have to first __disable all seal task types__ in the miner config. This is important to prevent conflicts between the two processes:

```toml
[Storage]
  AllowPreCommit1 = false
  AllowPreCommit2 = false
  AllowCommit = false
  AllowUnseal = false
```

You can then run the miner on your local-loopback interface; 

```sh
lotus-worker run
```

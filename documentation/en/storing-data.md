# Storing Data

> There are recent bug reports with these instructions. If you happen to encounter any problems, please create a [GitHub issue](https://github.com/filecoin-project/lotus/issues/new) and a maintainer will address the problem as soon as they can.

Here are instructions for how to store data on the **Lotus DevNet**.

## Adding a file

```sh
lotus client import ./your-example-file.txt
```

Upon success, this command will return a **Data CID**.

## List local files

The command to see a list of files by `CID`, `name`, `size` in bytes, and `status`:

```sh
lotus client local
```

An example of the output:

```sh
bafkreierupr5ioxn4obwly4i2a5cd2rwxqi6kwmcyyylifxjsmos7hrgpe Development/sample-1.txt 2332 ok
bafkreieuk7h4zs5alzpdyhlph4lxkefowvwdho3a3pml6j7dam5mipzaii Development/sample-2.txt 30618 ok
```

## Make a Miner Deal on DevNet

Get a list of all miners that can store data:

```sh
lotus state list-miners
```

Get the requirements of a miner you wish to store data with:

```sh
lotus client query-ask <miner>
```

Store a **Data CID** with a miner:

```sh
lotus client deal <Data CID> <miner> <price> <duration>
# price is in attoFIL/byte/block
# duration is number of blocks.
```

Upon success, this command will return a **Deal CID**. 

From now on the **Data CID** is [retrievable](https://docs.lotu.sh/en+retrieving-data) from the **Lotus Storage Miner**.

## Troubleshooting

```sh
WARN  main  lotus/main.go:72  routing: not found
```

* This miner is offline.

```sh
WARN  main  lotus/main.go:72  failed to start deal: computing commP failed: generating CommP: Piece must be at least 127 bytes
```

* There is a minimum file size of 127 bytes.
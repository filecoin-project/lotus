# Retrieving Data

> There are recent bug reports with these instructions. If you happen to encounter any problems, please create a [GitHub issue](https://github.com/filecoin-project/lotus/issues/new) and a maintainer will address the problem as soon as they can.

Here are the operations you can perform after you have stored a **Data CID** with the **Lotus Storage Miner** in the network.

If you would like to learn how to store a **Data CID** on a miner, read the instructions [here](https://docs.lotu.sh/en+storing-data).

## Find by Data CID

```sh
lotus client find <Data CID>
# LOCAL
# RETRIEVAL <miner>@<miner peerId>-<deal funds>-<size>
```

## Retrieve by Data CID

```sh
lotus client retrieve <Data CID> <outfile>
```

This command will initiate a **retrieval deal** and write the data to your computer. This process may take 2 to 10 minutes.

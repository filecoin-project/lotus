# Managing deals


While the Lotus Miner is running as a daemon, the `lotus-miner` application can be used to manage and configure the miner:


```sh
lotus-miner storage-deals --help
```

Running the above command will show the different options related to deals. For example, `lotus-miner storage-deals set-ask` allows to set the price for storage that your miner uses to respond ask requests from clients.

If deals are ongoing, you can check the data transfers with:

```sh
lotus-miner data-transfers list
```

Make sure you explore the `lotus-miner` CLI. Every command is self-documented and takes a `--help` flag that offers specific information about it.

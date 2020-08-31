# Making storage deals

## Adding a file to Lotus

Before sending data to a Filecoin miner for storage, the data needs to be correctly formatted and packed. This can be achieved by locally importing the data into Lotus with:

```sh
lotus client import ./your-example-file.txt
```

Upon success, this command will return a **Data CID**. This is a very important piece of information, as it will be used to make deals to both store and retrieve the data in the future.

You can list the data CIDs of the files you locally imported with:

```sh
lotus client local
```

## Storing data in the network

To store data in the network you will need to:

* Find a Filecoin miner willing to store it
* Make a deal with the miner agreeing on the price to pay and the duration for which the data should be stored.

You can obtain a list of all miners in the network with:

```sh
lotus state list-miners
t0xxxx
t0xxxy
t0xxxz
...
```

This will print a list of miner IDs. In order to ask for the terms offered by a particular miner, you can then run:

```sh
lotus client query-ask <miner>
```

If you are satisfied with the terms, you can proceed to propose a deal to the miner, using the **Data CID** that you obtained during the import step:


```sh
lotus client deal
```

This command will interactively ask you for the CID, miner ID and duration in days for the deal. You can also call it with arguments:

```sh
lotus client deal <data CID> <miner> <price> <duration>
```

where the `duration` is expressed in blocks (1 block is equivalent to 30s).

## Checking the status of the deals

You can list deals with:

```sh
lotus client list-deals
```

Among other things, this will give you information about the current state on your deals, whether they have been published on chain (by the miners) and whether the miners have been slashed for not honoring them.

For a deal to succeed, the miner needs to be correctly configured and running, accept the deal and *seal* the file correctly. Otherwise, the deal will appear in error state.

You can make deals with multiple miners for the same data.

Once a deal is sucessful and the data is *sealed*, it can be [retrieved](en+retrieving).

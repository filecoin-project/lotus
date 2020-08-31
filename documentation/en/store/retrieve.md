# Retrieving Data

Once data has been succesfully [stored](en+making-deals) and sealed by a Filecoin miner, it can be retrieved.

In order to do this we will need to create a **retrieval deal**.

## Finding data by CID

In order to retrieve some data you will need the **Data CID** that was used to create the storage deal.

You can find who is storing the data by running:

```sh
lotus client find <Data CID>
```

## Making a retrieval deal

You can then make a retrieval deal with:

```sh
lotus client retrieve <Data CID> <outfile>
```

This commands take other optional flags (check `--help`).

If the outfile does not exist it will be created in the Lotus repository directory. This process may take 2 to 10 minutes.

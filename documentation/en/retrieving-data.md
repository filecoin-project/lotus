# Retrieving Data

If you have stored data with a **Lotus Storage Miner** in the network, you can search for it by **Data CID**

```sh
$ lotus client find <Data CID>
LOCAL
RETRIEVAL <miner>@<miner peerId>-<deal funds>-<size>
```

Retrieve data from a **Lotus Storage Miner**.

```sh
$ lotus client retrieve <Data CID> <outfile>
```

This will initiate a **retrieval deal** and write the data to the outfile. This process may take some time.

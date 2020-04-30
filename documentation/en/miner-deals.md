# Information for Miners 

Here is how a miner can get set up to accept storage deals. The first step is
to install a Lotus node and sync to the top of the chain.

## Set up an ask

```
lotus-storage-miner set-price <price>
```

This command will set up your miner to accept deal proposals that meet the input price.
The price is inputted in FIL per GiB per epoch, and the default is 0.0000000005. 

<!-- TODO: Add info about setting min piece size, max piece size, duration -->

## Ensure you can be discovered

Clients need to be able to find you in order to make storage deals with you. 
While there isn't necessarily anything you need to do to become discoverable, here are some things you can
try to check that people can connect to you. 

To start off, make sure you are connected to at least some peers, and your port is 
open and working.

### Connect to your own node

If you are in contact with someone else running Lotus, you can ask them to try connecting
to your node. To do so, provide them your peer ID, which you can get by running `lotus net id` on
your node.

They can then try running `lotus net findpeer <peerID>` to get your address(es), and can then
run `lotus net connect <address>` to connect to you. If successful, your node will now
appear on their peers list (run `lotus net peers` to check).

You can also check this by running a second instance of Lotus yourself.

### Query your own ask

A client should be able to find your ask by running `lotus client query-ask <minerID>`. If 
someone is not able to retrieve your ask by doing so, then there is an issue with your node.
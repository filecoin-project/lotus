# Pond UI

Pond is a graphical testbed for [Lotus](https://lotu.sh). Using it will setup a separate local network which is helpful for debugging. Pond will spin up nodes, connect them in a given topology, start them mining, and observe how they function over time.

## Build

```sh
make pond
```

## Run

```sh
./pond run
```

Now go to `http://127.0.0.1:2222`.

## What can I test?

- The `Spawn Node` button starts a new **Lotus Node** in a new draggable window.
- Click `[Spawn Miner]` to start a **Lotus Miner**. This requires the node's wallet to have funds.
- Click on `[Client]` to open the **Lotus Node**'s client interface and propose a deal with an existing Miner. If successful you'll see a payment channel open up with that Miner.

Don't leave Pond unattended for more than 10 hours, the web client will eventually consume all available RAM.

## Troubleshooting

- Turn it off and on - Start at the top
- `rm -rf ~/.lotus ~/.lotusminer/`, this command will delete chain sync data, stored wallets, and other configurations so be careful.
- Verify you have the correct versions of dependencies
- If stuck on a bad fork, try `lotus chain sethead --genesis`

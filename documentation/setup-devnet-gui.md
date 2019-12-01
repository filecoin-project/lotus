# Setup Lotus Devnet using a GUI

As an alternative to the CLI you can use Pond, a graphical testbed for lotus. It can be used to spin up nodes, connect them in a given topology, start them mining, and observe how they function over time.

Build:

```sh
$ make pond
```

Run:

```sh
$ ./pond run
Listening on http://127.0.0.1:2222
```

Now go to http://127.0.0.1:2222.

## Thing to try

- The `Spawn Node` button starts a new lotus Node in a new draggable window.
- Click `[Spawn Storage Miner]` to start mining (make sure the Node's wallet has funds).
- Click on `[Client]` to open the Node's client interface and propose a deal with an existing Miner. If successful you'll see a payment channel open up with that Miner.

Don't leave Pond unattended for long periods of time (10h+), the web-ui tends to eventually consume all the available RAM.

## Troubleshooting

- Turn it off and on - Start at the top
- `rm -rf ~/.lotus ~/.lotusstorage/`
- Verify you have the correct versions of dependencies
- If stuck on a bad fork, try `lotus chain sethead --genesis`
- If that didn't help, open a new issue, ask in the [Community forum](https://discuss.filecoin.io) or reach out via [Community chat](https://github.com/filecoin-project/community#chat).

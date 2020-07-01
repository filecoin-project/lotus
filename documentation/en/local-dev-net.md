# Setup Local Devnet

Build the Lotus Binaries in debug mode, This enables the use of 2048 byte sectors.

```sh
make 2k
```

Download the 2048 byte parameters:
```sh
./lotus fetch-params 2048
```

Pre-seal some sectors:

```sh
./lotus-seed pre-seal --sector-size 2KiB --num-sectors 2
```

Create the genesis block and start up the first node:

```sh
./lotus-seed genesis new localnet.json
./lotus-seed genesis add-miner localnet.json ~/.genesis-sectors/pre-seal-t01000.json
./lotus daemon --lotus-make-genesis=dev.gen --genesis-template=localnet.json --bootstrap=false
```

Then, in another console, import the genesis miner key:

```sh
./lotus wallet import ~/.genesis-sectors/pre-seal-t01000.key
```

Set up the genesis miner:

```sh
./lotus-storage-miner init --genesis-miner --actor=t01000 --sector-size=2KiB --pre-sealed-sectors=~/.genesis-sectors --pre-sealed-metadata=~/.genesis-sectors/pre-seal-t01000.json --nosync
```

Now, finally, start up the miner:

```sh
./lotus-storage-miner run --nosync
```

If all went well, you will have your own local Lotus Devnet running.

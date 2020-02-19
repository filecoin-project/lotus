# Setup Local Devnet

Build the Lotus Binaries in debug mode, This enables the use of 1024 byte sectors.

```sh
make debug
```

Download the 1024 byte parameters:
```sh
./lotus fetch-params --proving-params 1024
```

Pre-seal some sectors:

```sh
./lotus-seed pre-seal --sector-size 1024 --num-sectors 2
```

Create the genesis block and start up the first node:

```sh
./lotus daemon --lotus-make-random-genesis=dev.gen --genesis-presealed-sectors=~/.genesis-sectors/pre-seal-t0101.json --bootstrap=false
```

Set up the genesis miner:

```sh
./lotus-storage-miner init --genesis-miner --actor=t0101 --sector-size=1024 --pre-sealed-sectors=~/.genesis-sectors --pre-sealed-metadata=~/.genesis-sectors/pre-seal-t0101.json --nosync
```

Now, finally, start up the miner:

```sh
./lotus-storage-miner run --nosync
```

If all went well, you will have your own local Lotus Devnet running.

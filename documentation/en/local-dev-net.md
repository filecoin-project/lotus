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
./lotus-seed genesis new localnet.json
./lotus-seed genesis add-miner localnet.json ~/.genesis-sectors/pre-seal-t01000.json
./lotus daemon --lotus-make-genesis=dev.gen --genesis-template=localnet.json --bootstrap=false
# TODO Key import
```

Set up the genesis miner:

```sh
./lotus-storage-miner init --genesis-miner --actor=t0101 --sector-size=1024 --pre-sealed-sectors=~/.genesis-sectors --nosync
```

Now, finally, start up the miner:

```sh
./lotus-storage-miner run --nosync
```

If all went well, you will have your own local Lotus Devnet running.

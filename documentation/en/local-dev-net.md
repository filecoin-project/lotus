# Running a local developer network

Build the lotus binaries in debug mode, This enables the use of 1024 byte
sectors.

```
make debug
```

Pre-seal some sectors:

```
./lotus-seed pre-seal --sector-size 1024 --num-sectors 2
```

Create the genesis block and start up the first node:

```
./lotus daemon --lotus-make-random-genesis=dev.gen --genesis-presealed-sectors
=~/.genesis-sectors/pre-seal-t0101.json --bootstrap=false
```

Set up the genesis miner:

```
./lotus-storage-miner init --genesis-miner --actor=t0101 --sector-size=1024 --pre-sealed-sectors=~/.genesis-sectors --nosync
```

Now, finally, start up the miner:

```
./lotus-storage-miner run --nosync
```

If all went well, you will have your own local lotus network running!

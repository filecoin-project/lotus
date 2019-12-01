# Mining

Ensure that at least one BLS address (`t3..`) in your wallet exists

```sh
$ lotus wallet list
t3...
```

With this address, go to https://lotus-faucet.kittyhawk.wtf/miner.html, and
click `Create Miner`

Wait for a page telling you the address of the newly created storage miner to
appear - It should be saying: `New storage miners address is: t0..`

Initialize storage miner:

```sh
$ lotus-storage-miner init --actor=t01.. --owner=t3....
```

This command should return successfully after miner is setup on-chain (30-60s)

Start mining:

```sh
$ lotus-storage-miner run
```

To view the miner id used for deals:

```sh
$ lotus-storage-miner info
```

e.g. miner id `t0111`

Seal random data to start producing PoSts:

```sh
$ lotus-storage-miner store-garbage
```

You can check miner power and sector usage with the miner id:

```sh
# Total power of the network
$ lotus-storage-miner state power

$ lotus-storage-miner state power <miner>

$ lotus-storage-miner state sectors <miner>
```

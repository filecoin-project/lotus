# Lotus changelog

# 0.5.0 / 2020-08-20

This version of Lotus will be used for the incentivized testnet Space Race competition,
and can be considered mainnet-ready code. It includes some protocol
changes, upgrades of core dependencies, and various bugfixes and UX/performance improvements.

## Highlights

Among the highlights included in this release are:

- Gas changes: We implemented EIP-1559 and introduced real gas values.
- Deal-making: We now support "Committed Capacity" sectors, "fast-retrieval" deals,
and the packing of multiple deals into a single sector.
- Renamed features: We renamed some of the binaries, environment variables, and default
paths associated with a Lotus node.

### Gas changes

We made some significant changes to the mechanics of gas in this release.

#### Network fee

We implemented something similar to 
[Ethereum's EIP-1559](https://github.com/ethereum/EIPs/blob/master/EIPS/eip-1559.md).
The `Message` structure had three changes:
- The `GasPrice` field has been removed
- A new `GasFeeCap` field has been added, which controls the maximum cost
the sender incurs for the message
- A new `GasPremium` field has been added, which controls the reward a miner
earns for including the message

A sender will never be charged more than `GasFeeCap * GasLimit`. 
A miner will typically earn `GasPremium * GasLimit` as a reward.

The `Blockheader` structure has one new field, called `ParentBaseFee`. 
Informally speaking,the `ParentBaseFee`
is increased when blocks are densely packed with messages, and decreased otherwise.

The `ParentBaseFee` is used when calculating how much a sender burns when executing a message. _Burning_ simply refers to sending attoFIL to a dedicated, unreachable account.
A message causes `ParentBaseFee * GasUsed` attoFIL to be burnt.

#### Real gas values

This release also includes our first "real" gas costs for primitive operations.
The costs were designed to account for both the _time_ that message execution takes,
as well as the _space_ a message adds to the state tree.

## Deal-making changes

There are three key changes to the deal-making process.

#### Committed Capacity sectors

Miners can now pledge "Committed Capacity" (CC) sectors, which are explicitly
stated as containing junk data, and must not include any deals. Miners can do this
to increase their storage power, and win block rewards from this pledged storage.

They can mark these sectors as "upgradable" with `lotus-miner sectors mark-for-upgrade`.
If the miner receives and accepts one or more storage deals, the sector that includes
those deals will _replace_ the CC sector. This is intended to maximize the amount of useful
storage on the Filecoin network.

#### Fast-retrieval deals

Clients can now include a `fast-retrieval` flag when proposing deals with storage miners.
If set to true, the miner will include an extra copy of the deal data. This
data can be quickly served in a retrieval deal, since it will not need to be unsealed.

#### Multiple deals per sector

Miners can now pack multiple deals into a single sector, so long as all the deals
fit into the sector capacity. This should increase the packing efficiency of miners.

### Renamed features

To improve the user experience, we updated several names to mainatin
standard prefixing, and to better reflect the meaning of the features being referenced.

In particular, the Lotus miner binary is now called `lotus-miner`, the default
path for miner data is now `~/.lotusminer`, and the environment variable
that sets the path for miner data is now `$LOTUS_MINER_PATH`. A full list of renamed
features can be found [here](https://github.com/filecoin-project/lotus/issues/2304).

## Changelog

#### Downstream upgrades
- Upgrades markets to v0.5.6 (https://github.com/filecoin-project/lotus/pull/3058)
- Upgrades specs-actors to v0.9.3 (https://github.com/filecoin-project/lotus/pull/3151)

#### Core protocol
- Introduces gas values, replacing placeholders (https://github.com/filecoin-project/lotus/pull/2343)
- Implements EIP-1559, introducing a network base fee, message gas fee cap, and message gas fee premium (https://github.com/filecoin-project/lotus/pull/2874)
- Implements Poisson Sortition for elections (https://github.com/filecoin-project/lotus/pull/2084)

#### Deal-making lifecycle
- Introduces "Committed Capacity" sectors (https://github.com/filecoin-project/lotus/pull/2220)
- Introduces "fast-retrieval" flag for deals (https://github.com/filecoin-project/lotus/pull/2323
- Supports packing multiple deals into one sector (https://github.com/filecoin-project/storage-fsm/pull/38)

#### Enhancements

- Optimized message pool selection logic (https://github.com/filecoin-project/lotus/pull/2838)
- Window-based scheduling of sealing tasks (https://github.com/filecoin-project/sector-storage/pull/67)
- Faster window PoSt (https://github.com/filecoin-project/lotus/pull/2209/files)
- Refactors the payment channel manager (https://github.com/filecoin-project/lotus/pull/2640)
- Refactors blocksync (https://github.com/filecoin-project/lotus/pull/2715/files)

#### UX

- Provide status updates for data-transfer (https://github.com/filecoin-project/lotus/pull/3162, https://github.com/filecoin-project/lotus/pull/3191)
- Miners can customise asks (https://github.com/filecoin-project/lotus/pull/2046)
- Miners can toggle auto-acceptance of deals (https://github.com/filecoin-project/lotus/pull/1994)
- Miners can maintain a blocklist of piece CIDs (https://github.com/filecoin-project/lotus/pull/2069)

## Contributors

The following contributors had 10 or more commits go into this release.
We are grateful for every contribution!

| Contributor        | Commits | Lines ±       |
|--------------------|---------|---------------|
| magik6k            | 361     | +13197/-6136  |
| Kubuxu             | 227     | +5670/-2587   |
| arajasek           | 120     | +2916/-1264   |
| whyrusleeping      | 112     | +3979/-1089   |
| vyzo               | 99      | +3343/-1305   |
| dirkmc             | 68      | +8732/-3621   |
| laser              | 45      | +1489/-501    |
| hannahhoward       | 43      | +2654/-990    |
| frrist             | 37      | +6630/-4338   |
| schomatis          | 28      | +3016/-1368   |
| placer14           | 27      | +824/-350     |
| raulk              | 25      | +28718/-29849 |
| mrsmkl             | 22      | +560/-368     |
| travisperson       | 18      | +1354/-314    |
| nonsense           | 16      | +2956/-2842   |
| ingar              | 13      | +331/-123     |
| daviddias          | 11      | +311/-11      |
| Stebalien          | 11      | +1204/-980    |
| RobQuistNL         | 10      | +69/-74       |

# 0.1.0 / 2019-12-11

We are very excited to release **lotus** 0.1.0. This is our testnet release. To install lotus and join the testnet, please visit [lotu.sh](lotu.sh). Please file bug reports as [issues](https://github.com/filecoin-project/lotus/issues).

A huge thank you to all contributors for this testnet release!

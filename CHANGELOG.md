# Lotus changelog

> **Historical Note**
> Previous changelog entries are archived in:
> * [CHANGELOG_0.x.md](./documentation/changelog/CHANGELOG_0.x.md) - v0.1.0 to v0.9.1
> * [CHANGELOG_1.0x.md](./documentation/changelog/CHANGELOG_1.0x.md) - v1.0.0 to v1.9.0
> * [CHANGELOG_1.1x.md](./documentation/changelog/CHANGELOG_1.1x.md) - v1.10.0 to v1.19.0
> * [CHANGELOG_1.2x.md](./documentation/changelog/CHANGELOG_1.2x.md) - v1.20.0 to v1.29.2

# UNRELEASED

- feat!: actors bundle v16.0.1 & special handling for calibnet ([filecoin-project/lotus#13006](https://github.com/filecoin-project/lotus/pull/13006))

# Node and Miner v1.32.1 / 2025-03-28

The Lotus v1.32.1 release is a **MANDATORY patch release**, which will deliver the Filecoin network version 25, codenamed “Teep” 🦵. This release sets the upgrade epoch for the Mainnet to **Epoch 4867320 - 2025-04-10T23:00:00Z**, and correctly sets the F3 activationcontract address to `0xA19080A1Bcb82Bb61bcb9691EC94653Eb5315716`. You can find more details about how the F3 activation on Mainnet will be executed in the [F3 Activation Procedure](https://github.com/filecoin-project/go-f3/issues/920#issuecomment-2761448485).

## ☢️ Upgrade Warnings ☢️
- The Lotus v1.32.0 release had an issue where the F3 activation contract address was not set correctly. This release corrects that issue.
- If you are running the v1.30.0 version of Lotus, please go through the Upgrade Warnings section for the [v1.31.0 releases](https://github.com/filecoin-project/lotus/releases/tag/v1.31.0) and [v1.31.1](https://github.com/filecoin-project/lotus/releases/tag/v1.31.1) before upgrading to this release.
- The minimum supported Golang version is now `1.23.6` ([filecoin-project/lotus#12910](https://github.com/filecoin-project/lotus/pull/12910)).
- The `SupportedProofTypes` field has been removed from the `Filecoin.StateGetNetworkParams` method because it was frequently overlooked during proof type updates and did not accurately reflect the FVM's supported proofs ([filecoin-project/lotus#12881](https://github.com/filecoin-project/lotus/pull/12881)).
- Introduced `Agent` field to the `Filecoin.Version` response. Note that this change may be breaking, depending on the clients deserialization capabilities. ([filecoin-project/lotus#12904](https://github.com/filecoin-project/lotus/pull/12904)).
- The `--only-cc` option has been removed from the `lotus-miner sectors extend` command. 

## 🏛️ Filecoin network version 25 FIPs

- [FIP-0097: Add Support for EIP-1153 (Transient Storage) in the FEVM](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0097.md)
- [FIP-0098: Simplify termination fee calculation to a fixed percentage of initial pledge](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0098.md)
- [FIP-0100: Removing Batch Balancer, Replacing It With a Per-sector Fee and Removing Gas-limited Constraints](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0100.md)
- [F3 Mainnet Activation](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0086.md)

## 📦 v16 Builtin Actor Bundle

This release candidate uses the [v16.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v16.0.0)

## 🚚 Migration
All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 120 epochs before the network upgrade. The migration for the NV25 upgrade is expected to be medium with a bit longer pre-migration compared to the two previous network upgrade.

Pre-Migration is expected to take between 4 to 8 minutes on a SplitStore node. The migration on the upgrade epoch is expected to take 30 seconds on a node with a NVMe-drive and a newer CPU. For nodes running on slower disks/CPU, it is still expected to take around 1 minute. We recommend node operators (who haven't enabled splitstore discard mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

For certain node operators, such as full archival nodes or systems that need to keep large amounts of state (RPC providers), we recommend skipping the pre-migration and run the non-cached migration (i.e., just running the migration at the network upgrade epoch), and schedule for some additional downtime. Operators of such nodes can read the [How to disable premigration in network upgrade tutorial](https://lotus.filecoin.io/kb/disable-premigration/).

## New Features highlight
- feat!: [FIP-0100](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0100.md) and [FIP-0098](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0098.md) implementation.
  - Adds a scheduled nv26 "Tock" upgrade exactly 90 days after nv25 to signal the end of the sector extensions grace period for FIP-0100. This grace period is 7 days for calibnet.
  - Deadlines on the public API now have a `DailyFee` field
  - `DealIDs` has now been removed from the public API's `SectorOnChainInfo` (was deprecated in FIP-0079)
  - Removed `--only-cc` from `spcli sectors extend` command
  - Change circulating supply calculation for calibnet, butterflynet and 2k for nv25 upgrade; see ([filecoin-project/lotus#12938](https://github.com/filecoin-project/lotus/pull/12938)) for more information.
- feat: integrate & test FIP-0098 additions ([filecoin-project/lotus#12968](https://github.com/filecoin-project/lotus/pull/12968))
- feat(cli): the `lotus state sectors` command now supports the `--show-partitions` flag and printing CSV output. ([filecoin-project/lotus#12834](https://github.com/filecoin-project/lotus/pull/12834)).
- feat: handle non-existing actors gracefully in F3 power proportion CLI. ([filecoin-project/lotus#12840](https://github.com/filecoin-project/lotus/pull/12840))
- feat: check ETH events indexed in range ([filecoin-project/lotus#12728](https://github.com/filecoin-project/lotus/pull/12728))
- feat: exposed `StateGetNetworkParams` in the Lotus Gateway API ([filecoin-project/lotus#12881](https://github.com/filecoin-project/lotus/pull/12881))
- feat: add `--csv` option to the `lotus send` command ([filecoin-project/lotus#12892](https://github.com/filecoin-project/lotus/pull/12892))
- feat: add `GenesisTimestamp` to `StateGetNetworkParams` response ([filecoin-project/lotus#12925](https://github.com/filecoin-project/lotus/pull/12925))
- feat: add `ChainGetMessagesInTipset` to Lotus Gateway API ([filecoin-project/lotus#12947](https://github.com/filecoin-project/lotus/pull/12947))
- feat(f3): Implement contract based parameter setting as for FRC-0099 ([filecoin-project/lotus#12861](https://github.com/filecoin-project/lotus/pull/12861))
- feat(miner): remove batch balancer-related functionality ([filecoin-project/lotus#12919](https://github.com/filecoin-project/lotus/pull/12919))
- feat(market): expose access to ProviderSectors on the market actor abstraction ([filecoin-project/lotus#12978](https://github.com/filecoin-project/lotus/pull/12978))
- feat: expose market ProviderSectors access on state-types abstraction ([filecoin-project/lotus#12978](https://github.com/filecoin-project/lotus/pull/12978))
- feat(shed): lotus-shed miner-fees - to inspect FIP-0100 fees for a miner ([filecoin-project/lotus#12980](https://github.com/filecoin-project/lotus/pull/12980))
- Set the F3 contract address on mainnet ([filecoin-project/lotus#12994](https://github.com/filecoin-project/lotus/pull/12994))

## Improvements
- refactor(eth): attach ToFilecoinMessage converter to EthCall ([filecoin-project/lotus#12844](https://github.com/filecoin-project/lotus/pull/12844))
- feat: automatically detect if genesis CAR is compressed when using the `--genesis` flag in the `lotus daemon` command ([filecoin-project/lotus#12885](https://github.com/filecoin-project/lotus/pull/12885))
- fix: In addition to existing network, also publish 2k docker images ([filecoin-project/lotus#12911](https://github.com/filecoin-project/lotus/pull/12911))
- docs(api): document 10% overestimation in collateral/pledge APIs ([filecoin-project/lotus#12922](https://github.com/filecoin-project/lotus/pull/12922))
- fix(drand): add null HistoricalBeaconClient for old beacons ([filecoin-project/lotus#12830](https://github.com/filecoin-project/lotus/pull/12830))
- chore: reduce participation log verbosity when F3 isn't read ([filecoin-project/lotus#12937](https://github.com/filecoin-project/lotus/pull/12937))
- fix: allow users to optionally configure node startup even if index reconciliation fails ([filecoin-project/lotus#12930](https://github.com/filecoin-project/lotus/pull/12930))
- feat: add a `LOTUS_DISABLE_F3_ACTIVATION` enviroment variable allowing disabling F3 activation for a specific contract address or epoch ([filecoin-project/lotus#12920](https://github.com/filecoin-project/lotus/pull/12920)).
- chore: switch to pure-go zstd decoder for snapshot imports.  ([filecoin-project/lotus#12857](https://github.com/filecoin-project/lotus/pull/12857))
- chore: upgrade go-state-types with big.Int{} change that means an empty big.Int is now treated as zero for all operations ([filecoin-project/lotus#12936](https://github.com/filecoin-project/lotus/pull/12936))
- chore(eth): make `EthGetBlockByNumber` & `EthGetBlockByHash` share the same cache and be impacted by `EthBlkCacheSize` config settings ([filecoin-project/lotus#12979](https://github.com/filecoin-project/lotus/pull/12979))
- chore(deps): bump go-state-types to v0.16.0-rc8 ([filecoin-project/lotus#12973](https://github.com/filecoin-project/lotus/pull/12973))  
- chore: set Mainnet nv25 upgrade epoch and update deps ([filecoin-project/lotus#12986](https://github.com/filecoin-project/lotus/pull/12986))
- chore(eth): make EthGetBlockByNumber & EthGetBlockByHash share cache code ([filecoin-project/lotus#12979](https://github.com/filecoin-project/lotus/pull/12979))

## Bug Fixes
- fix(eth): minor improvements to event range checking ([filecoin-project/lotus#12867](https://github.com/filecoin-project/lotus/pull/12867))
- fix(wallet): allow delegated wallet import ([filecoin-project/lotus#12876](https://github.com/filecoin-project/lotus/pull/12876))
- fix: use the correct environment variable (`FIL_PROOFS_PARAMETER_CACHE`) for proof params path ([filecoin-project/lotus#12891](https://github.com/filecoin-project/lotus/pull/12891))

## 📝 Changelog

For the set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/v1.31.1...v1.32.1
- Miner: https://github.com/filecoin-project/lotus/compare/v1.31.1...miner/v1.32.1

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Rod Vagg | 46 | +16240/-12784 | 286 |
| Masih H. Derkani | 76 | +5697/-2175 | 290 |
| Jakub Sztandera | 38 | +2048/-1652 | 244 |
| Aryan Tikarya | 2 | +1931/-1444 | 43 |
| Phi-rjan | 19 | +1777/-1251 | 69 |
| Piotr Galar | 4 | +1052/-261 | 14 |
| Mikers | 2 | +664/-149 | 12 |
| Steven Allen | 8 | +325/-148 | 31 |
| dependabot[bot] | 15 | +190/-208 | 30 |
| Phi | 4 | +214/-156 | 12 |
| Viraj Bhartiya | 2 | +190/-49 | 13 |
| Aarsh Shah | 1 | +104/-47 | 6 |
| caseylove | 1 | +71/-67 | 1 |
| asamuj | 2 | +39/-43 | 14 |
| ZenGround0 | 1 | +64/-0 | 1 |
| Krishang Shah | 1 | +30/-30 | 2 |
| tediou5 | 1 | +38/-15 | 14 |
| dockercui | 1 | +19/-19 | 19 |
| XiaoBei | 2 | +15/-15 | 7 |
| Hubert | 1 | +21/-5 | 9 |
| wmjae | 2 | +9/-9 | 7 |
| taozui472 | 1 | +9/-9 | 6 |
| Yash Jagtap | 1 | +7/-7 | 5 |
| Peter Cover | 1 | +6/-6 | 4 |
| Andi | 1 | +6/-6 | 2 |
| root | 1 | +5/-5 | 4 |
| growfrow | 1 | +3/-3 | 1 |
| Łukasz Magiera | 1 | +4/-0 | 2 |
| wgyt | 1 | +2/-2 | 1 |
| web3-bot | 2 | +2/-2 | 2 |
| parthshah1 | 1 | +2/-2 | 1 |
| leo | 1 | +2/-2 | 2 |
| futreall | 1 | +2/-2 | 2 |
| Pranav Konde | 1 | +2/-2 | 1 |
| Steve Loeppky | 1 | +2/-0 | 1 |
| LexLuthr | 1 | +2/-0 | 1 |

# Node and Miner v1.32.0 / 2025-03-27

This is the stable release of the **upcoming MANDATORY Lotus v1.32.0 release**, which will deliver the Filecoin network version 25, codenamed “Teep” 🦵. This release candidate sets the upgrade epoch for the Mainnet to **Epoch 4867320 - 2025-04-10T23:00:00Z**.

## ☢️ Upgrade Warnings ☢️
- If you are running the v1.30.0 version of Lotus, please go through the Upgrade Warnings section for the [v1.31.0 releases](https://github.com/filecoin-project/lotus/releases/tag/v1.31.0) and [v1.31.1](https://github.com/filecoin-project/lotus/releases/tag/v1.31.1) before upgrading to this release.
- The minimum supported Golang version is now `1.23.6` ([filecoin-project/lotus#12910](https://github.com/filecoin-project/lotus/pull/12910)).
- The `SupportedProofTypes` field has been removed from the `Filecoin.StateGetNetworkParams` method because it was frequently overlooked during proof type updates and did not accurately reflect the FVM's supported proofs ([filecoin-project/lotus#12881](https://github.com/filecoin-project/lotus/pull/12881)).
- Introduced `Agent` field to the `Filecoin.Version` response. Note that this change may be breaking, depending on the clients deserialization capabilities. ([filecoin-project/lotus#12904](https://github.com/filecoin-project/lotus/pull/12904)).
- The `--only-cc` option has been removed from the `lotus-miner sectors extend` command. 

## 🏛️ Filecoin network version 25 FIPs

- [FIP-0097: Add Support for EIP-1153 (Transient Storage) in the FEVM](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0097.md)
- [FIP-0098: Simplify termination fee calculation to a fixed percentage of initial pledge](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0098.md)
- [FIP-0100: Removing Batch Balancer, Replacing It With a Per-sector Fee and Removing Gas-limited Constraints](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0100.md)
- [F3 Mainnet Activation](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0086.md)

## 📦 v16 Builtin Actor Bundle

This release candidate uses the [v16.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v16.0.0)

## 🚚 Migration
All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 120 epochs before the network upgrade. The migration for the NV25 upgrade is expected to be medium with a bit longer pre-migration compared to the two previous network upgrade.

Pre-Migration is expected to take between 4 to 8 minutes on a SplitStore node. The migration on the upgrade epoch is expected to take 30 seconds on a node with a NVMe-drive and a newer CPU. For nodes running on slower disks/CPU, it is still expected to take around 1 minute. We recommend node operators (who haven't enabled splitstore discard mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

For certain node operators, such as full archival nodes or systems that need to keep large amounts of state (RPC providers), we recommend skipping the pre-migration and run the non-cached migration (i.e., just running the migration at the network upgrade epoch), and schedule for some additional downtime. Operators of such nodes can read the [How to disable premigration in network upgrade tutorial](https://lotus.filecoin.io/kb/disable-premigration/).

## New Features highlight
- feat!: [FIP-0100](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0100.md) and [FIP-0098](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0098.md) implementation.
  - Adds a scheduled nv26 "Tock" upgrade exactly 90 days after nv25 to signal the end of the sector extensions grace period for FIP-0100. This grace period is 7 days for calibnet.
  - Deadlines on the public API now have a `DailyFee` field
  - `DealIDs` has now been removed from the public API's `SectorOnChainInfo` (was deprecated in FIP-0079)
  - Removed `--only-cc` from `spcli sectors extend` command
  - Change circulating supply calculation for calibnet, butterflynet and 2k for nv25 upgrade; see ([filecoin-project/lotus#12938](https://github.com/filecoin-project/lotus/pull/12938)) for more information.
- feat: integrate & test FIP-0098 additions ([filecoin-project/lotus#12968](https://github.com/filecoin-project/lotus/pull/12968))
- feat(cli): the `lotus state sectors` command now supports the `--show-partitions` flag and printing CSV output. ([filecoin-project/lotus#12834](https://github.com/filecoin-project/lotus/pull/12834)).
- feat: handle non-existing actors gracefully in F3 power proportion CLI. ([filecoin-project/lotus#12840](https://github.com/filecoin-project/lotus/pull/12840))
- feat: check ETH events indexed in range ([filecoin-project/lotus#12728](https://github.com/filecoin-project/lotus/pull/12728))
- feat: exposed `StateGetNetworkParams` in the Lotus Gateway API ([filecoin-project/lotus#12881](https://github.com/filecoin-project/lotus/pull/12881))
- feat: add `--csv` option to the `lotus send` command ([filecoin-project/lotus#12892](https://github.com/filecoin-project/lotus/pull/12892))
- feat: add `GenesisTimestamp` to `StateGetNetworkParams` response ([filecoin-project/lotus#12925](https://github.com/filecoin-project/lotus/pull/12925))
- feat: add `ChainGetMessagesInTipset` to Lotus Gateway API ([filecoin-project/lotus#12947](https://github.com/filecoin-project/lotus/pull/12947))
- feat(f3): Implement contract based parameter setting as for FRC-0099 ([filecoin-project/lotus#12861](https://github.com/filecoin-project/lotus/pull/12861))
- feat(miner): remove batch balancer-related functionality ([filecoin-project/lotus#12919](https://github.com/filecoin-project/lotus/pull/12919))
- feat(market): expose access to ProviderSectors on the market actor abstraction ([filecoin-project/lotus#12978](https://github.com/filecoin-project/lotus/pull/12978))
- feat: expose market ProviderSectors access on state-types abstraction ([filecoin-project/lotus#12978](https://github.com/filecoin-project/lotus/pull/12978))
- feat(shed): lotus-shed miner-fees - to inspect FIP-0100 fees for a miner ([filecoin-project/lotus#12980](https://github.com/filecoin-project/lotus/pull/12980))

## Improvements
- refactor(eth): attach ToFilecoinMessage converter to EthCall ([filecoin-project/lotus#12844](https://github.com/filecoin-project/lotus/pull/12844))
- feat: automatically detect if genesis CAR is compressed when using the `--genesis` flag in the `lotus daemon` command ([filecoin-project/lotus#12885](https://github.com/filecoin-project/lotus/pull/12885))
- fix: In addition to existing network, also publish 2k docker images ([filecoin-project/lotus#12911](https://github.com/filecoin-project/lotus/pull/12911))
- docs(api): document 10% overestimation in collateral/pledge APIs ([filecoin-project/lotus#12922](https://github.com/filecoin-project/lotus/pull/12922))
- fix(drand): add null HistoricalBeaconClient for old beacons ([filecoin-project/lotus#12830](https://github.com/filecoin-project/lotus/pull/12830))
- chore: reduce participation log verbosity when F3 isn't read ([filecoin-project/lotus#12937](https://github.com/filecoin-project/lotus/pull/12937))
- fix: allow users to optionally configure node startup even if index reconciliation fails ([filecoin-project/lotus#12930](https://github.com/filecoin-project/lotus/pull/12930))
- feat: add a `LOTUS_DISABLE_F3_ACTIVATION` enviroment variable allowing disabling F3 activation for a specific contract address or epoch ([filecoin-project/lotus#12920](https://github.com/filecoin-project/lotus/pull/12920)).
- chore: switch to pure-go zstd decoder for snapshot imports.  ([filecoin-project/lotus#12857](https://github.com/filecoin-project/lotus/pull/12857))
- chore: upgrade go-state-types with big.Int{} change that means an empty big.Int is now treated as zero for all operations ([filecoin-project/lotus#12936](https://github.com/filecoin-project/lotus/pull/12936))
- chore(eth): make `EthGetBlockByNumber` & `EthGetBlockByHash` share the same cache and be impacted by `EthBlkCacheSize` config settings ([filecoin-project/lotus#12979](https://github.com/filecoin-project/lotus/pull/12979))
- chore(deps): bump go-state-types to v0.16.0-rc8 ([filecoin-project/lotus#12973](https://github.com/filecoin-project/lotus/pull/12973))  
- chore: set Mainnet nv25 upgrade epoch and update deps ([filecoin-project/lotus#12986](https://github.com/filecoin-project/lotus/pull/12986))
- chore(eth): make EthGetBlockByNumber & EthGetBlockByHash share cache code ([filecoin-project/lotus#12979](https://github.com/filecoin-project/lotus/pull/12979))

## Bug Fixes
- fix(eth): minor improvements to event range checking ([filecoin-project/lotus#12867](https://github.com/filecoin-project/lotus/pull/12867))
- fix(wallet): allow delegated wallet import ([filecoin-project/lotus#12876](https://github.com/filecoin-project/lotus/pull/12876))
- fix: use the correct environment variable (`FIL_PROOFS_PARAMETER_CACHE`) for proof params path ([filecoin-project/lotus#12891](https://github.com/filecoin-project/lotus/pull/12891))

## 📝 Changelog

For the set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/v1.31.1...v1.32.1
- Miner: https://github.com/filecoin-project/lotus/compare/v1.31.1...miner/v1.32.1

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Rod Vagg | 46 | +16240/-12784 | 286 |
| Masih H. Derkani | 76 | +5697/-2175 | 290 |
| Jakub Sztandera | 38 | +2048/-1652 | 244 |
| Aryan Tikarya | 2 | +1931/-1444 | 43 |
| Phi-rjan | 19 | +1777/-1251 | 69 |
| Piotr Galar | 4 | +1052/-261 | 14 |
| Mikers | 2 | +664/-149 | 12 |
| Steven Allen | 8 | +325/-148 | 31 |
| dependabot[bot] | 15 | +190/-208 | 30 |
| Phi | 4 | +214/-156 | 12 |
| Viraj Bhartiya | 2 | +190/-49 | 13 |
| Aarsh Shah | 1 | +104/-47 | 6 |
| caseylove | 1 | +71/-67 | 1 |
| asamuj | 2 | +39/-43 | 14 |
| ZenGround0 | 1 | +64/-0 | 1 |
| Krishang Shah | 1 | +30/-30 | 2 |
| tediou5 | 1 | +38/-15 | 14 |
| dockercui | 1 | +19/-19 | 19 |
| XiaoBei | 2 | +15/-15 | 7 |
| Hubert | 1 | +21/-5 | 9 |
| wmjae | 2 | +9/-9 | 7 |
| taozui472 | 1 | +9/-9 | 6 |
| Yash Jagtap | 1 | +7/-7 | 5 |
| Peter Cover | 1 | +6/-6 | 4 |
| Andi | 1 | +6/-6 | 2 |
| root | 1 | +5/-5 | 4 |
| growfrow | 1 | +3/-3 | 1 |
| Łukasz Magiera | 1 | +4/-0 | 2 |
| wgyt | 1 | +2/-2 | 1 |
| web3-bot | 2 | +2/-2 | 2 |
| parthshah1 | 1 | +2/-2 | 1 |
| leo | 1 | +2/-2 | 2 |
| futreall | 1 | +2/-2 | 2 |
| Pranav Konde | 1 | +2/-2 | 1 |
| Steve Loeppky | 1 | +2/-0 | 1 |
| LexLuthr | 1 | +2/-0 | 1 |

# Node v1.31.1 / 2025-01-27

This Lotus release introduces several new features and improvements, including JSON output for tipsets in `lotus chain list` cmd, enhanced logging during network upgrade migrations, and additional Bootstrap nodes. It also includes a refactored Ethereum API implementation into smaller, more manageable modules in a new `github.com/filecoin-project/lotus/node/impl/eth` package, as well as adding network name as a tag in most metrics - making it easier to create Graphana Dashboards for multiple networks. Please review the upgrade warnings and documentation for any important changes affecting RPC providers, node operators, and storage providers.

## ☢️ Upgrade Warnings ☢️
- If you are running the v1.30.x version of Lotus, please go through the Upgrade Warnings section for the [v1.31.0](https://github.com/filecoin-project/lotus/releases/tag/v1.31.0) before upgrading to this release.

## ⭐ Feature/Improvement Highlights:
- Add json output of tipsets to `lotus chain list`. ([filecoin-project/lotus#12691](https://github.com/filecoin-project/lotus/pull/12691))
- During a network upgrade, log migration progress every 2 seconds so they are more helpful and informative. The `LOTUS_MIGRATE_PROGRESS_LOG_SECONDS` environment variable can be used to change this if needed. ([filecoin-project/lotus#12732](https://github.com/filecoin-project/lotus/pull/12732))
- Add Magik's Bootstrap node. ([filecoin-project/lotus#12792](https://github.com/filecoin-project/lotus/pull/12792))
- Lotus now reports the network name as a tag in most metrics. ([filecoin-project/lotus#12733](https://github.com/filecoin-project/lotus/pull/12733))
- Add a new cron queue inspection utility. ([filecoin-project/lotus#12825](https://github.com/filecoin-project/lotus/pull/12825))
- Add a new utility to the `lotus-shed msg` tool which pretty-prints gas summaries to tables, broken down into compute and storage gas totals and percentages ([filecoin-project/lotus#12817](https://github.com/filecoin-project/lotus/pull/12817))
- Generate the cli docs directly from the code instead compiling and executing binaries' `help` output. ([filecoin-project/lotus#12717](https://github.com/filecoin-project/lotus/pull/12717))
- Refactored Ethereum API implementation into smaller, more manageable modules in a new `github.com/filecoin-project/lotus/node/impl/eth` package. ([filecoin-project/lotus#12796](https://github.com/filecoin-project/lotus/pull/12796))
- Add F3GetCertificate & F3GetLatestCertificate to the gateway. ([filecoin-project/lotus#12778](https://github.com/filecoin-project/lotus/pull/12778))
- Add `StateMarketProposalPending` API / `lotus state market proposal-pending` CLI. ([filecoin-project/lotus#12724](https://github.com/filecoin-project/lotus/pull/12724))

## 🐛 Bug Fix Highlights
- Remove IPNI advertisement relay over pubsub via Lotus node as it now has been deprecated. ([filecoin-project/lotus#12768](https://github.com/filecoin-project/lotus/pull/12768)
- Make `EthTraceFilter` / `trace_filter` skip null rounds instead of erroring. ([filecoin-project/lotus#12702](https://github.com/filecoin-project/lotus/pull/12702))
- Event APIs (`GetActorEventsRaw`, `SubscribeActorEventsRaw`, `eth_getLogs`, `eth_newFilter`, etc.) will now return an error when a request matches more than `MaxFilterResults` (default: 10,000) rather than silently truncating the results. Also apply an internal event matcher for `eth_getLogs` (etc.) to avoid builtin actor events on database query so as not to include them in `MaxFilterResults` calculation. ([filecoin-project/lotus#12671](https://github.com/filecoin-project/lotus/pull/12671))
- `ChainIndexer#GetMsgInfo` returns an `ErrNotFound` when there are no rows. ([filecoin-project/lotus#12680](https://github.com/filecoin-project/lotus/pull/12680))
- Gracefully handle EAM CreateAccount failures in `EthTraceBlock` (`trace_block`) and `EthTraceTransaction` (`trace_transaction`) calls. ([filecoin-project/lotus#12730](https://github.com/filecoin-project/lotus/pull/12730))
- Make f3 gen power command being non-deterministic ([filecoin-project/lotus#12764](https://github.com/filecoin-project/lotus/pull/12764))
- Resolve a bug in sync by preventing checkpoint expansion ([filecoin-project/lotus#12747](https://github.com/filecoin-project/lotus/pull/12747))
- Fix issue in backfillIndex where error handling could lead to a potential panic ([filecoin-project/lotus#12813](https://github.com/filecoin-project/lotus/pull/12813))

## 📝 Changelog

For the full set of changes since the last stable node release:

https://github.com/filecoin-project/lotus/compare/v1.31.0...v1.31.1

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Rod Vagg | 26 | +13687/-11008 | 146 |
| Masih H. Derkani | 19 | +2492/-1506 | 59 |
| Aryan Tikarya | 2 | +2120/-1407 | 45 |
| Krishang Shah | 1 | +3214/-117 | 66 |
| Steven Allen | 4 | +1317/-1632 | 22 |
| Jakub Sztandera | 10 | +935/-1203 | 176 |
| Łukasz Magiera | 2 | +949/-467 | 33 |
| Phi-rjan | 9 | +369/-339 | 43 |
| Piotr Galar | 4 | +586/-106 | 12 |
| Viraj Bhartiya | 3 | +219/-63 | 16 |
| caseylove | 1 | +71/-67 | 1 |
| asamuj | 2 | +39/-43 | 14 |
| ZenGround0 | 2 | +73/-1 | 3 |
| XiaoBei | 2 | +15/-15 | 7 |
| wmjae | 2 | +9/-9 | 7 |
| taozui472 | 1 | +9/-9 | 6 |
| dependabot[bot] | 2 | +9/-9 | 4 |
| huajin tong | 1 | +6/-6 | 6 |
| Phi | 1 | +6/-6 | 6 |
| Andi | 1 | +6/-6 | 2 |
| root | 1 | +5/-5 | 4 |
| chuangjinglu | 1 | +3/-3 | 3 |
| wgyt | 1 | +2/-2 | 1 |
| parthshah1 | 1 | +2/-2 | 1 |
| leo | 1 | +2/-2 | 2 |
| pinglanlu | 1 | +1/-1 | 1 |

# Node and Miner v1.31.0 / 2024-12-02

The Lotus v1.31.0 release introduces the new `ChainIndexer` subsystem, enhancing the indexing of Filecoin chain state for improved RPC performance. Several bug fixes in the block production loop are also included. Please review the upgrade warnings and documentation for any important changes affecting RPC providers, node operators and storage providers.

## ⭐ New Feature Highlights:
- New ChainIndexer subsystem to index Filecoin chain state such as tipsets, messages, events and ETH transactions for accurate and faster RPC responses. The `ChainIndexer` replaces the existing `MsgIndex`, `EthTxHashLookup` and `EventIndex` implementations in Lotus, which [suffer from a multitude of known problems](https://github.com/filecoin-project/lotus/issues/12293).  If you are an RPC provider or a node operator who uses or exposes Ethereum and/or events APIs, please refer to the [ChainIndexer documentation for operators](./documentation/en/chain-indexer-overview-for-operators.md) for information on how to enable, configure and use the new Indexer.  While there is no automated data migration and one can upgrade and downgrade without backups, there are manual steps that need to be taken to backfill data when upgrading to this Lotus version, or downgrading to the previous version without ChainIndexer. Please be aware that that this feature removes some options in the Lotus configuration file, if these have been set, Lotus will report an error when starting. See the documentation for more information
- `lotus chain head` now supports a `--height` flag to print just the epoch number of the current chain head ([filecoin-project/lotus#12609](https://github.com/filecoin-project/lotus/pull/12609))
- Implement `EthGetTransactionByBlockNumberAndIndex` (`eth_getTransactionByBlockNumberAndIndex`) and `EthGetTransactionByBlockHashAndIndex` (`eth_getTransactionByBlockHashAndIndex`) methods. ([filecoin-project/lotus#12618](https://github.com/filecoin-project/lotus/pull/12618))
- `lotus-shed indexes inspect-indexes` now performs a comprehensive comparison of the event index data for each message by comparing the AMT root CID from the message receipt with the root of a reconstructed AMT. Previously `inspect-indexes` simply compared event counts.  Comparing AMT roots instead confirms all the event data is byte-perfect. ([filecoin-project/lotus#12570](https://github.com/filecoin-project/lotus/pull/12570))
- Return a "data" field on the "error" returned from RPC when `eth_call` and `eth_estimateGas` APIs encounter `execution reverted` errors. This is a standard expectation of Ethereum RPC tooling and may improve compatibility in some cases. ([filecoin-project/lotus#12553](https://github.com/filecoin-project/lotus/pull/12553))
- Improve ETH-filter performance for nodes serving many clients. ([filecoin-project/lotus#12603](https://github.com/filecoin-project/lotus/pull/12603))
- Implement F3 utility CLIs to list the power table for a given instance and sum the proportional power of a set of actors that participate in a given instance. ([filecoin-project/lotus#12698](https://github.com/filecoin-project/lotus/pull/12698))

## 🐛 Bug Fix Highlights
- Add logic to check if the miner's owner address is delegated (f4 address). If it is delegated, the `lotus-shed sectors termination-estimate` command now sends the termination state call using the worker ID. This fix resolves the issue where termination-estimate did not function correctly for miners with delegated owner addresses. ([filecoin-project/lotus#12569](https://github.com/filecoin-project/lotus/pull/12569))
- The Lotus Miner will now always mine on the latest chain head returned by lotus, even if that head has less "weight" than the previously seen head. This is necessary because F3 may end up finalizing a tipset with a lower weight, although this situation should be rare on the Filecoin mainnet. ([filecoin-project/lotus#12659](https://github.com/filecoin-project/lotus/pull/12659)) and ([filecoin-project/lotus#12690](https://github.com/filecoin-project/lotus/pull/12690))
- Make the ordering of event output for `eth_` APIs and `GetActorEventsRaw` consistent, sorting ascending on: epoch, message index, event index and original event entry order. ([filecoin-project/lotus#12623](https://github.com/filecoin-project/lotus/pull/12623))
- Return a consistent error when encountering null rounds in ETH RPC method calls. ([filecoin-project/lotus#12655](https://github.com/filecoin-project/lotus/pull/12655))
- Correct erroneous sector QAP-calculation upon sector extension in lotus-miner cli. ([filecoin-project/lotus#12720](https://github.com/filecoin-project/lotus/pull/12720))
- Return error if logs or events within range are not indexed. ([filecoin-project/lotus#12728](https://github.com/filecoin-project/lotus/pull/12728))


## 📝 Changelog

For the full set of changes since the last stable release:

* Node: https://github.com/filecoin-project/lotus/compare/v1.30.0...v1.31.0
* Miner: https://github.com/filecoin-project/lotus/compare/v1.30.0...miner/v1.31.0

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Aarsh Shah | 2 | +6725/-5410 | 84 |
| Masih H. Derkani | 13 | +1924/-867 | 61 |
| Viraj Bhartiya | 6 | +2048/-703 | 41 |
| Steven Allen | 25 | +1394/-404 | 53 |
| Rod Vagg | 13 | +502/-272 | 39 |
| Phi-rjan | 8 | +175/-64 | 20 |
| Jakub Sztandera | 7 | +107/-66 | 15 |
| aarshkshah1992 | 1 | +61/-30 | 5 |
| Steve Loeppky | 1 | +78/-2 | 4 |
| Krishang Shah | 1 | +7/-17 | 1 |
| Łukasz Magiera | 1 | +9/-10 | 3 |
| Phi | 1 | +9/-9 | 8 |
| Danial Ahn | 1 | +14/-1 | 2 |
| hanabi1224 | 1 | +7/-6 | 1 |
| web3-bot | 1 | +1/-1 | 1 |
| asamuj | 1 | +1/-1 | 1 |
| Andrew Jackson (Ajax) | 1 | +2/-0 | 1 |

# Node and Miner v1.30.0 / 2024-11-06

This is the final release of the MANDATORY Lotus v1.30.0 release, which delivers the Filecoin network version 24, codenamed Tuk Tuk 🛺. **This release sets the Mainnet to upgrade at epoch `4461240`, corresponding to `2024-11-20T23:00:00Z`.**

- If you are running the v1.28.x version of Lotus, please go through the Upgrade Warnings section for the v1.28.* releases and v1.29.*, before upgrading to this release.
- This release requires a minimum Go version of v1.22.7 or higher.
- The `releases` branch has been deprecated with the 202408 split of 'Lotus Node' and 'Lotus Miner'. See https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#why-is-the-releases-branch-deprecated-and-what-are-alternatives for more info and alternatives for getting the latest release for both the 'Lotus Node' and 'Lotus Miner' based on the Branch and Tag Strategy.
   - To get the latest Lotus Node tag: git tag -l 'v*' | sort -V -r | head -n 1
   - To get the latest Lotus Miner tag: git tag -l 'miner/v*' | sort -V -r | head -n 1

## 🏛️ Filecoin network version 24 FIPs

- [FIP-0081: Introduce lower bound for sector initial pledge](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0081.md)
- [FIP-0094: Add Support for EIP-5656 (MCOPY Opcode) in the FEVM](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0094.md)
- [FIP-0095: Add FEVM precompile to fetch beacon digest from chain history](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0095.md)

*⚠️ The activation of F3 (Fast Finality) has been postponed for mainnet* due to unresolved issues in the Client/SP code and the F3 protocol itself. These issues require further testing and resolution before we can safely deploy F3 on the mainnet. Read the full [post here](https://github.com/filecoin-project/community/discussions/74?sort=new#discussioncomment-11164349).

## 📦 v15 Builtin Actor Bundle

The [v15.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v15.0.0) actor bundle is used for supporting this upgrade. Make sure that your Lotus actor bundle matches the v15 actors manifest by running the following cli after upgrading to this release:

```
lotus state actor-cids --network-version=24
Network Version: 24
Actor Version: 15
Manifest CID: bafy2bzaceakwje2hyinucrhgtsfo44p54iw4g6otbv5ghov65vajhxgntr53u

Actor             CID
account           bafk2bzacecia5zacqt4gvd4z7275lnkhgraq75shy63cphakphhw6crf4joii
cron              bafk2bzacecbyx7utt3tkvhqnfk64kgtlt5jlvv56o2liwczikgzfowk2cvqvk
datacap           bafk2bzacecrypcpyzidphfl3sf3vhrjbiwzu7w3hoole45wsk2bqpverw4tni
eam               bafk2bzacebybq7keb45l6isqfaiwxy5oi5wlpknhggjheut7q6xwp7mbxxku4
ethaccount        bafk2bzaceajdy72edg3t2zcb6qwv2wgdsysfwdtczcklxcp4hlwh7pkxekja4
evm               bafk2bzaceandffodu45eyro7jr7bizxw7ibipaiskt36xbp4vpvsxtrpkyjfm
init              bafk2bzaceb5mjmy56ediswt2hvwqdfs2xzi4qw3cefkufoat57yyt3iwkg7kw
multisig          bafk2bzaced3csl3buj7chpunsubrhwhchtskx674fpukfen4u6pbpkcheueya
paymentchannel    bafk2bzacea3dpsfxw7cnj6zljmjnnaubp43a5kvuausigztmukektesg2flei
placeholder       bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro
reward            bafk2bzaceapkgue3gcxmwx7bvypn33okppa2nwpelcfp7oyo5yln3brixpjpm
storagemarket     bafk2bzaceaqrnikbxymygwhwa2rsvhnqj5kfch75pn5xawnx243brqlfglsl6
storageminer      bafk2bzacecnl2hqe3nozwo7al7kdznqgdrv2hbbbmpcbcwzh3yl4trog433hc
storagepower      bafk2bzacecb3tvvppxmktll3xehjc7mqbfilt6bd4gragbdwxn77hm5frkuac
system            bafk2bzacecvcqje6kcfqeayj66hezlwzfznytwqkxgw7p64xac5f5lcwjpbwe
verifiedregistry  bafk2bzacecudaqwbz6dukmdbfok7xuxcpjqighnizhxun4spdqvnqgftkupp2
```

## 🚚 Migration

All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 120 epochs before the network upgrade. The migration for the NV24 upgrade is expected to be light with no heavy pre-migrations:

- Pre-Migration is expected to take less then 1 minute.
- The migration on the upgrade epoch is expected to take less than 30 seconds on a node with a NVMe-drive and a newer CPU. For nodes running on slower disks/CPU, it is still expected to take less then 1 minute.
- RAM usages is expected to be under 20GiB RAM for both the pre-migration and migration.

We recommend node operators (who haven't enabled splitstore discard mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

For certain node operators, such as full archival nodes or systems that need to keep large amounts of state (RPC providers), we recommend skipping the pre-migration and run the non-cached migration (i.e., just running the migration at the network upgrade epoch), and schedule for some additional downtime. Operators of such nodes can read the [How to disable premigration in network upgrade tutorial](https://lotus.filecoin.io/kb/disable-premigration/).

## 📝 Changelog

For the set of changes since the last stable release:

* Node: https://github.com/filecoin-project/lotus/compare/v1.29.2...v1.30.0
* Miner: https://github.com/filecoin-project/lotus/compare/v1.28.3...miner/v1.30.0

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Krishang | 2 | +34106/-0 | 109 |
| Rod Vagg | 86 | +10643/-8291 | 456 |
| Masih H. Derkani | 59 | +7700/-4725 | 298 |
| Steven Allen | 55 | +6113/-3169 | 272 |
| kamuik16 | 7 | +4618/-1333 | 285 |
| Jakub Sztandera | 10 | +3995/-1226 | 94 |
| Peter Rabbitson | 26 | +2313/-2718 | 275 |
| Viraj Bhartiya | 5 | +2624/-580 | 50 |
| Phi | 7 | +1337/-1519 | 257 |
| Mikers | 1 | +1274/-455 | 23 |
| Phi-rjan | 29 | +736/-600 | 92 |
| Andrew Jackson (Ajax) | 3 | +732/-504 | 75 |
| LexLuthr | 3 | +167/-996 | 8 |
| Aarsh Shah | 12 | +909/-177 | 47 |
| web3-bot | 40 | +445/-550 | 68 |
| Piotr Galar | 6 | +622/-372 | 15 |
| aarshkshah1992 | 18 | +544/-299 | 40 |
| Steve Loeppky | 14 | +401/-196 | 22 |
| Frrist | 1 | +403/-22 | 5 |
| Łukasz Magiera | 4 | +266/-27 | 13 |
| winniehere | 1 | +146/-144 | 3 |
| Jon | 1 | +209/-41 | 4 |
| Aryan Tikarya | 2 | +183/-8 | 7 |
| adlrocha | 2 | +123/-38 | 21 |
| dependabot[bot] | 11 | +87/-61 | 22 |
| Jiaying Wang | 8 | +61/-70 | 12 |
| Ian Davis | 2 | +60/-38 | 5 |
| Aayush Rajasekaran | 2 | +81/-3 | 3 |
| hanabi1224 | 4 | +46/-4 | 5 |
| Laurent Senta | 1 | +44/-1 | 2 |
| jennijuju | 6 | +21/-20 | 17 |
| parthshah1 | 1 | +23/-13 | 1 |
| Brendan O'Brien | 1 | +25/-10 | 2 |
| Jennifer Wang | 4 | +24/-8 | 6 |
| Matthew Rothenberg | 3 | +10/-18 | 6 |
| riskrose | 1 | +8/-8 | 7 |
| linghuying | 1 | +5/-5 | 5 |
| fsgerse | 2 | +3/-7 | 3 |
| PolyMa | 1 | +5/-5 | 5 |
| zhangguanzhang | 1 | +3/-3 | 2 |
| luozexuan | 1 | +3/-3 | 3 |
| Po-Chun Chang | 1 | +6/-0 | 2 |
| Kevin Martin | 1 | +4/-1 | 2 |
| simlecode | 1 | +2/-2 | 2 |
| ZenGround0 | 1 | +2/-2 | 2 |
| GFZRZK | 1 | +2/-1 | 1 |
| DemoYeti | 1 | +2/-1 | 1 |
| qwdsds | 1 | +1/-1 | 1 |
| Samuel Arogbonlo | 1 | +2/-0 | 2 |
| Elias Rad | 1 | +1/-1 | 1 |

# Lotus changelog

> **Historical Note**
> Previous changelog entries are archived in:
> * [CHANGELOG_0.x.md](./documentation/changelog/CHANGELOG_0.x.md) - v0.1.0 to v0.9.1
> * [CHANGELOG_1.0x.md](./documentation/changelog/CHANGELOG_1.0x.md) - v1.0.0 to v1.9.0
> * [CHANGELOG_1.1x.md](./documentation/changelog/CHANGELOG_1.1x.md) - v1.10.0 to v1.19.0
> * [CHANGELOG_1.2x.md](./documentation/changelog/CHANGELOG_1.2x.md) - v1.20.0 to v1.29.2

# UNRELEASED

## 🚀 Features
- EIP-7702: Add support for EIP-7702 transactions and integration.
  - Implements the "Set EOA account code" standard via a new transaction type `0x04`.
  - Enables atomic authorization and execution via `EthAccount.ApplyAndCall`.
  - Adds support for parsing `0x04` transactions and `authorizationList` in `ethtypes`.
  - Extends JSON-RPC types and receipts to include `authorizationList` and `delegatedTo` fields.
  - *Note: The send-path is currently gated by the `eip7702_enabled` build tag.*

## 👌 Improvements
- docs: fix outdated link in documentation ([#13436](https://github.com/filecoin-project/lotus/pull/13436))
- docs: fix dead link in documentation ([#13437](https://github.com/filecoin-project/lotus/pull/13437))

# Node v1.34.3 / 2025-12-03

This is a patch release addressing Docker image glibc compatibility errors reported in v1.34.2. This update is only necessary for users running Lotus via Docker who encountered `GLIBC_2.32/2.33/2.34 not found` errors.

## Bug Fixes

- fix(docker): upgrade base image from ubuntu:20.04 to ubuntu:22.04 ([filecoin-project/lotus#13441](https://github.com/filecoin-project/lotus/pull/13441))
  - The build stage uses golang:1.24.7-bookworm (glibc 2.36), but the runtime base was ubuntu:20.04 (glibc 2.31), causing GLIBC_2.32/2.33/2.34 errors when running lotus binaries.

## 📝 Changelog

For the set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/release/v1.34.2...release/v1.34.3

# Node and Miner v1.34.2 / 2025-12-01

The Lotus and Lotus-Miner v1.34.2 release includes numerous bug fixes, CLI enhancements, and dependency updates. These improvements, along with updated dependencies, enhance the stability and usability of Lotus for both node operators and storage providers.

## ☢️ Upgrade Warnings ☢️

- The minimum supported Golang version is now `1.24.7`

## Features and Bug Fixes

- feat(gateway): expose StateGetRandomnessDigestFromBeacon ([filecoin-project/lotus#13339](https://github.com/filecoin-project/lotus/pull/13339))
- fix(cli): add deposit-margin-factor to the new miner commands ([filecoin-project/lotus#13365](https://github.com/filecoin-project/lotus/pull/13365))
- feat(spcli): add a `deposit-margin-factor` option to `lotus-miner actor new` and `lotus-shed miner create` so the sent deposit still covers the on-chain requirement if it rises between lookup and execution ([filecoin-project/lotus#13407](https://github.com/filecoin-project/lotus/pull/13407))
- feat(cli): lotus evm deploy prints message CID ([filecoin-project/lotus#13241](https://github.com/filecoin-project/lotus/pull/13241))
- fix(miner): ensure sender account exists ([filecoin-project/lotus#13348](https://github.com/filecoin-project/lotus/pull/13348))
- fix(eth): properly return vm error in all gas estimation methods ([filecoin-project/lotus#13389](https://github.com/filecoin-project/lotus/pull/13389))
- chore: all actor cmd support --actor ([filecoin-project/lotus#13391](https://github.com/filecoin-project/lotus/pull/13391))

## 📝 Changelog

For the set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/release/v1.34.1...release/v1.34.2
- Miner: https://github.com/filecoin-project/lotus/compare/release/v1.34.1...release/miner/v1.34.2

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Phi-rjan | 11 | +39018/-254 | 240 |
| Rod Vagg | 12 | +793/-656 | 45 |
| dependabot[bot] | 33 | +483/-415 | 69 |
| Jintu Kumar Das | 1 | +372/-372 | 24 |
| Adin Schmahmann | 1 | +525/-53 | 6 |
| Mikers | 1 | +519/-0 | 18 |
| TippyFlits | 6 | +248/-160 | 22 |
| Piotr Galar | 3 | +57/-44 | 14 |
| aceppaluni | 1 | +48/-34 | 3 |
| Block Wizard | 5 | +37/-36 | 18 |
| tediou5 | 2 | +58/-6 | 4 |
| Phi | 2 | +37/-17 | 12 |
| Luca Moretti | 4 | +24/-24 | 18 |
| cui | 1 | +22/-25 | 5 |
| beck | 1 | +13/-22 | 4 |
| Aryan Tikarya | 1 | +21/-14 | 2 |
| parthshah1 | 1 | +11/-23 | 3 |
| 0x5459 | 1 | +28/-4 | 4 |
| fengyuchuanshen | 1 | +7/-7 | 7 |
| web3-bot | 4 | +6/-7 | 5 |
| Steve Loeppky | 1 | +7/-5 | 1 |
| Snezhkko | 1 | +6/-6 | 5 |
| Krishang Shah | 1 | +6/-5 | 1 |
| Lee | 1 | +5/-5 | 1 |
| stemlaud | 1 | +4/-4 | 4 |
| asttool | 1 | +4/-4 | 4 |
| Jakub Sztandera | 1 | +0/-8 | 1 |
| Hubert | 1 | +4/-3 | 3 |
| suranmiao | 1 | +2/-2 | 2 |
| reddaisyy | 1 | +2/-2 | 1 |
| joemicky | 1 | +2/-2 | 1 |
| efcking | 1 | +2/-2 | 1 |
| CertiK | 1 | +2/-1 | 1 |
| wyrapeseed | 1 | +1/-1 | 1 |
| letreturn | 1 | +1/-1 | 1 |
| juejinyuxitu | 1 | +1/-1 | 1 |
| cargoedit | 1 | +1/-1 | 1 |
| asamuj | 1 | +1/-1 | 1 |
| spuradage | 1 | +0/-1 | 1 |

# Node and Miner v1.34.1 / 2025-09-15

This is a non-critical patch release that fixes an issue with the Lotus `v1.34.0` release where the incorrect version of filecoin-ffi was included.  Lotus `v1.34.0` used filecoin-ffi `v1.34.0-dev` when it should have used `v1.34.0`.  This isn’t critical since it’s the same filecoin-ffi version used during the nv27 Calibration network upgrade, but for consistency with other Node implementations like Forest, we are creating this release.  This ensures the inclusion of ref-fvm `v4.7.3` update that was missing in v1.34.0.  All users of v1.34.0 are encouraged to upgrade to v1.34.1.

# Node and Miner v1.34.0 / 2025-09-11

This is a **MANDATORY Lotus v1.34.0 release**, which will deliver the Filecoin network version 27, codenamed “Golden Week” 🏮. This release candidate sets the upgrade epoch for the Mainnet network to **Epoch 5348280:  2025-09-24T23:00:00Z**.  (See the [local time for other timezones](https://www.worldtimebuddy.com/?qm=1&lid=100,5128581,5368361,1816670&h=100&date=2025-9-24&sln=23-24&hf=1&c=1196).)

## ☢️ Upgrade Warnings ☢️
- All Lotus node and Storage Provider (SP) operators must upgrade to v1.34.x before the specified date for the Mainnet network.
- The `/v1` Ethereum APIs have "F3 awareness" for all Ethereum calls where `"finalized"` or `"safe"` are supplied.  Nodes will likely return different (and likely more recent) results in v1.34.x+ than previous versions when these tags are used.  See more info below.

## 🏛️ Filecoin network version 27 FIPs and FRCs

- [FIP-0105: BLS12-381 Precompiles for FEVM (EIP-2537)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0105.md)
- [FIP-0109: Smart contract notifications for Direct Data Onboarding (DDO)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0109.md)
- [FIP-0077: Add deposit requirement for new miner creation](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0077.md)
- [FIP-0103: Remove ExtendSectorExpiration method](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0103.md)
- [FIP-0106: Remove ProveReplicaUpdates method](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0106.md)
- [FIP-0101: Remove ProveCommitAggregate method](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0101.md)
- [FRC-0108: F3-compatible snapshots](https://github.com/filecoin-project/FIPs/blob/master/FRCs/frc-0108.md)

## 📦 v17 Builtin Actor Bundle

This release candidate uses [v17.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v17.0.0).

## 🚚 Migration
All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 120 epochs before the network upgrade. The migration for the NV27 upgrade is expected to be light with no heavy pre-migrations:
- Pre-Migration is expected to take less then 1 minute.
- The migration on the upgrade epoch is expected to take less than 30 seconds on a node with a NVMe-drive and a newer CPU. For nodes running on slower disks/CPU, it is still expected to take less then 1 minute.
- RAM usages is expected to be under 20GiB RAM for both the pre-migration and migration.

We recommend node operators (who haven’t enabled splitstore discard mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

For certain node operators, such as full archival nodes or systems that need to keep large amounts of state (RPC providers), we recommend skipping the pre-migration and run the non-cached migration (i.e., just running the migration at the network upgrade epoch), and schedule for some additional downtime. Operators of such nodes can read the [How to disable premigration in network upgrade tutorial](https://lotus.filecoin.io/kb/disable-premigration/).

## ⭐ New Features highlight

- feat(eth): use F3 for "finalized" and "safe" resolution in v1 APIs. This switches the /v1 Ethereum APIs to have the same resolution rules as /v2, enabling F3 awareness for all Ethereum calls where `"finalized"` or `"safe"` is supplied. See [F3-aware Ethereum APIs via `/v2` endpoint and improvements to existing `/v1` APIs](#f3-aware-ethereum-apis-via-v2-endpoint-and-improvements-to-existing-v1-apis) below for details of how the /v2 APIs work as introduced in the 1.33.0 release. Set the environment variable `LOTUS_ETH_V1_DISABLE_F3_FINALITY_RESOLUTION` to `1` to revert this behaviour but note that the option to revert will likely be removed in a future release ([tracking issue](https://github.com/filecoin-project/lotus/issues/13315)). ([filecoin-project/lotus#13298](https://github.com/filecoin-project/lotus/pull/13298))
- feat(f3): expose simple ChainGetFinalizedTipSet API on v1 (and gateway) that just returns the latest F3 finalized tipset, or falls back to EC finality if F3 is not operational on the node or if the F3 finalized tipset is further back than EC finalized tipset. This API can be used for follow-up state calls that clamp to a specific tipset to have assurance of state finality. ([filecoin-project/lotus#13299](https://github.com/filecoin-project/lotus/pull/13299))
- feat: support for F3-aware snapshot v2 format per [FRC-0108](https://github.com/filecoin-project/FIPs/blob/master/FRCs/frc-0108.md) ([filecoin-project/lotus#13282](https://github.com/filecoin-project/lotus/pull/13282))
  - snapshot export now defaults to v2 format with embedded F3 finality certificates, dramatically reducing F3 catchup time from ~8 hours
  - transparently imports both v1 and v2 snapshot formats
  - to export v1 snapshots, use `lotus chain export --skip-old-msgs --recent-stateroots=2001 --snapshot-version=1 <filename>`
- feat(net): add LOTUS_ENABLE_MESSAGE_FETCH_INSTRUMENTATION=1 to turn on metrics and debugging for local vs bitswap message fetching during block validation ([filecoin-project/lotus#13221](https://github.com/filecoin-project/lotus/pull/13221))

## 👌 Improvements
- chore(docs): mark v0 API as "deprecated" and v1 as "stable" ([filecoin-project/lotus#13264](https://github.com/filecoin-project/lotus/pull/13264))
- feat(api): add StateMinerCreationDeposit API method for FIP-0077 - calculates the deposit required for creating a new miner ([filecoin-project/lotus#13308](https://github.com/filecoin-project/lotus/pull/13308))
- feat(spcli): correctly handle the batch logic of `lotus-miner actor settle-deal` ([#13189](https://github.com/filecoin-project/lotus/pull/13189))
- feat(spcli): add `--all-deals` to `lotus-miner actor settle-deal` ([#13243](https://github.com/filecoin-project/lotus/pull/13243))

## 🐛 Bug Fixes
- fix: properly handle all RPC API retry errors ([#13279](https://github.com/filecoin-project/lotus/pull/13279))
- fix(api): `eth_getCode` and `eth_getStorageAt` now return state after the specified block rather than before it ([filecoin-project/lotus#13274](https://github.com/filecoin-project/lotus/pull/13274))
- fix(api): `eth_getTransactionCount` now returns state after the specified block rather than before it ([filecoin-project/lotus#13275](https://github.com/filecoin-project/lotus/pull/13275))
- fix: handle partial reads in UnpadReader for non-power-of-2 pieces ([filecoin-project/lotus#13306](https://github.com/filecoin-project/lotus/pull/13306))

## 📝 Changelog

For the set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/release/v1.33.1...release/v1.34.0
- Miner: https://github.com/filecoin-project/lotus/compare/release/v1.33.1...release/miner/v1.34.0

### Changes since RC2
- Updated to use final release versions of key dependencies, including builtin-actors.  See [filecoin-project/lotus#13337](https://github.com/filecoin-project/lotus/pull/13337)).

## 👨‍👩‍👧‍👦 Contributors

| **Contributor** | **Commits** | **Lines ±** | **Files Changed** |
|-----------------|-------------|-------------|-------------------|
| Rod Vagg | 12 | +1856/-720 | 49 |
| TippyFlits | 6 | +1312/-897 | 60 |
| tediou5 | 1 | +610/-22 | 14 |
| Phi-rjan | 11 | +455/-169 | 29 |
| chris-4chain | 1 | +222/-23 | 7 |
| Steven Allen | 1 | +142/-68 | 3 |
| beck | 2 | +141/-52 | 8 |
| dependabot[bot] | 12 | +81/-86 | 24 |
| Steve Loeppky | 5 | +90/-42 | 22 |
| hanabi1224 | 2 | +91/-24 | 3 |
| raul0ligma | 1 | +88/-4 | 5 |
| William Morriss | 3 | +41/-13 | 7 |
| Copilot | 1 | +46/-0 | 1 |
| deepdring | 1 | +6/-6 | 6 |
| Block Wizard | 3 | +6/-6 | 5 |
| wmypku | 1 | +4/-4 | 2 |
| queryfast | 1 | +4/-4 | 4 |
| minxinyi | 1 | +4/-4 | 4 |
| web3-bot | 2 | +3/-3 | 3 |
| tzchenxixi | 1 | +3/-3 | 3 |
| haouvw | 1 | +3/-3 | 2 |
| TimberLake | 1 | +3/-3 | 2 |
| Jakub Sztandera | 2 | +3/-3 | 3 |
| Micke | 1 | +2/-2 | 2 |
| longhutianjie | 1 | +1/-1 | 1 |
| Piotr Galar | 1 | +1/-1 | 1 |
| Phi | 1 | +1/-1 | 1 |

# Node v1.33.1 / 2025-07-31
This is the Lotus v1.33.1 release, which introduces performance improvements and operational enhancements. This release focuses on improving F3 subsystem performance, and enhancing CLI tools for better storage provider operations. Notable improvements include up to 6-10x performance gains in F3 power table calculations, ensuring that PreCommit and ProveCommit operations are aggregating to get optimal gas usage after FIP-100, and a enhanced sector management tool with CSV output support. These improvements collectively enhance the stability and efficiency of Lotus operations for both node operators and storage providers.

## ☢️ Upgrade Warnings ☢️
- There are no upgrade warnings for this release candidate.

## ⭐ Feature/Improvement Highlights:
- fix(cli): fix `lotus state sector` command to display DealIDs correctly post-FIP-0076 by querying market actor's ProviderSectors HAMT while maintaining backward compatibility with DeprecatedDealIDs field ([filecoin-project/lotus#13140](https://github.com/filecoin-project/lotus/pull/13140))
- feat(spcli): make settle-deal optionally take deal id ranges ([filecoin-project/lotus#13146](https://github.com/filecoin-project/lotus/pull/13146))
- feat(miner): Adjust PreCommit & ProveCommit logic for optimal nv25 gas usage: ([filecoin-project/lotus#13049](https://github.com/filecoin-project/lotus/pull/13049))
  - remove deprecated pre-nv25 code, including batch balancer calculations
  - default to PreCommit batching
  - default to ProveCommit aggregation
  - remove config options: AggregateCommits, AggregateAboveBaseFee, BatchPreCommitAboveBaseFee
- feat(paych): add EnablePaymentChannelManager config option and disable payment channel manager by default ([filecoin-project/lotus#13139](https://github.com/filecoin-project/lotus/pull/13139))
- feat(gateway): add CORS headers if --cors is provided ([filecoin-project/lotus#13145](https://github.com/filecoin-project/lotus/pull/13145))
- feat(f3): move go-f3 datastore to separate leveldb instance ([filecoin-project/lotus#13174](https://github.com/filecoin-project/lotus/pull/13174))
- `lotus state active-sectors` now outputs CSV format and supports an optional `--show-partitions` to list active sector deadlines and partitions. ([filecoin-project/lotus#13152](https://github.com/filecoin-project/lotus/pull/13152))
- feat: ExpectedRewardForPower builtin utility function and `lotus-shed miner expected-reward` CLI command ([filecoin-project/lotus#13138](https://github.com/filecoin-project/lotus/pull/13138))
- feat(api): update go-f3 to 0.8.8, add F3GetPowerTableByInstance to the API ([filecoin-project/lotus#13201](https://github.com/filecoin-project/lotus/pull/13201))
- feat(f3): integrate cached MapReduce from go-hamt-ipld, which improves performance of F3 power table calculation by 6-10x ([filecoin-project/lotus#13134](https://github.com/filecoin-project/lotus/pull/13134))

## 🐛 Bug Fix Highlights
- fix(cli): correctly construct the TerminateSectors params ([filecoin-project/lotus#13207](https://github.com/filecoin-project/lotus/pull/13207))
- fix(spcli): send SettleDealPayments msg to f05 for `lotus-miner actor settle-deal` ([filecoin-project/lotus#13142](https://github.com/filecoin-project/lotus/pull/13142))
- fix(cli): handle disabled payment channel manager gracefully in lotus info command ([filecoin-project/lotus#13198](https://github.com/filecoin-project/lotus/pull/13198))
- chore: disable F3 participation via gateway ([filecoin-project/lotus#13123](https://github.com/filecoin-project/lotus/pull/13123)
- fix(cli): make lotus-miner sectors extend command resilient to higher gas ([filecoin-project/lotus#11928](https://github.com/filecoin-project/lotus/pull/11928))
- fix(cli): use F3GetPowerTableByInstance to resolve F3 power tables by default, `--by-tipset` flag can be used to restore old behavior ([filecoin-project/lotus#13201](https://github.com/filecoin-project/lotus/pull/13201))
- fix(f3): properly wire up eth v2 APIs for f3 ([filecoin-project/lotus#13149](https://github.com/filecoin-project/lotus/pull/13149))
- chore: increase the F3 GMessage buffer size to 1024 ([filecoin-project/lotus#13126](https://github.com/filecoin-project/lotus/pull/13126))
- chore: bump the pubsub validation queue length to 256 ([filecoin-project/lotus#13176](https://github.com/filecoin-project/lotus/pull/13176))
- chore(deps): update of critical underlying dependencies with go-libp2p to v0.42.0 (filecoin-project/lotus#13190) and boxo to v0.32.0 ([filecoin-project/lotus#13202](https://github.com/filecoin-project/lotus/pull/13202)) and boxo v0.33.0([filecoin-project/lotus#13226](https://github.com/filecoin-project/lotus/pull/13226))
- feat(spcli): correctly handle the batch logic of `lotus-miner actor settle-deal`; replace the dealid data source ([filecoin-project/lotus#13189](https://github.com/filecoin-project/lotus/pull/13189))
- feat(spcli): add `--all-deals` to `lotus-miner actor settle-deal`. By default, only expired deals are processed ([filecoin-project/lotus#13243](https://github.com/filecoin-project/lotus/pull/13243))

## 📝 Changelog

For the full set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/release/v1.33.0...release/v1.33.1

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| TippyFlits | 6 | +38440/-212 | 201 |
| Masih H. Derkani | 32 | +10241/-3770 | 171 |
| Jakub Sztandera | 33 | +2823/-1753 | 146 |
| Rod Vagg | 17 | +2089/-238 | 67 |
| Steven Allen | 7 | +885/-741 | 15 |
| Steve Loeppky | 8 | +389/-395 | 29 |
| hanabi1224 | 1 | +533/-8 | 5 |
| Phi-rjan | 8 | +346/-169 | 37 |
| Sarkazein | 3 | +364/-53 | 14 |
| tediou5 | 4 | +48/-318 | 29 |
| Barbara Peric | 5 | +315/-24 | 12 |
| beck | 3 | +116/-89 | 8 |
| dependabot[bot] | 21 | +92/-90 | 42 |
| Łukasz Magiera | 1 | +173/-0 | 2 |
| Copilot | 2 | +106/-66 | 5 |
| Krishang Shah | 1 | +113/-0 | 5 |
| Piotr Galar | 3 | +47/-3 | 6 |
| Phi | 1 | +12/-12 | 11 |
| terry.hung | 1 | +15/-6 | 1 |
| Degen Dev | 2 | +8/-8 | 6 |
| bytesingsong | 1 | +7/-7 | 6 |
| Anna Smith | 1 | +7/-7 | 4 |
| Aida Syoko | 1 | +6/-6 | 5 |
| cuithon | 1 | +5/-5 | 5 |
| shandongzhejiang | 1 | +4/-4 | 4 |
| geekvest | 1 | +4/-4 | 3 |
| Block Wizard | 1 | +4/-4 | 3 |
| bytetigers | 1 | +3/-3 | 3 |
| ZenGround0 | 1 | +2/-3 | 1 |
| web3-bot | 2 | +2/-2 | 2 |
| findmyhappy | 1 | +2/-2 | 1 |
| emmmm | 1 | +2/-2 | 2 |
| dumikau | 1 | +2/-2 | 2 |
| David Klank | 1 | +2/-2 | 1 |
| gopherorg | 1 | +1/-1 | 1 |
| VolodymyrBg | 1 | +1/-1 | 1 |
| Hubert | 1 | +1/-1 | 1 |
| GarmashAlex | 1 | +1/-1 | 1 |
| James Niken | 1 | +0/-1 | 1 |

# Node v1.33.0 / 2025-05-08
The Lotus v1.33.0 release introduces experimental v2 APIs with F3 awareness, featuring a new TipSet selection mechanism that significantly enhances how applications interact with the Filecoin blockchain. This release candidate also adds F3-aware Ethereum APIs via the /v2 endpoint.  All of the /v2 APIs implement intelligent fallback mechanisms between F3 and Expected Consensus and are exposed through the Lotus Gateway.

Please review the detailed documentation for these experimental APIs, as they are subject to change and have important operational considerations for node operators and API providers.

## ☢️ Upgrade Warnings ☢️
- There are no upgrade warnings for this release candidate.

## ⭐ Feature/Improvement Highlights:

### Experimental v2 APIs with F3 awareness

The Lotus V2 APIs introduce a powerful new TipSet selection mechanism that significantly enhances how applications interact with the Filecoin blockchain. The design reduces API footprint, seamlessly handles both traditional Expected Consensus (EC) and the new F3 protocol, and provides graceful fallbacks.

> [!NOTE]
> V2 APIs are highly experimental and subject to change without notice.

See [Filecoin v2 APIs docs](https://filoznotebook.notion.site/Filecoin-V2-APIs-1d0dc41950c1808b914de5966d501658) for an in-depth overview. /v2 APIs are exposed through Lotus Gateway.

This work was primarily done in ([filecoin-project/lotus#13003](https://github.com/filecoin-project/lotus/pull/13003)), ([filecoin-project/lotus#13027](https://github.com/filecoin-project/lotus/pull/13027)), ([filecoin-project/lotus#13034](https://github.com/filecoin-project/lotus/pull/13034)), ([filecoin-project/lotus#13075](https://github.com/filecoin-project/lotus/pull/13075)), ([filecoin-project/lotus#13066](https://github.com/filecoin-project/lotus/pull/13066))

### F3-aware Ethereum APIs via `/v2` endpoint and improvements to existing `/v1` APIs

Lotus now offers two versions of its Ethereum-compatible APIs (`eth_*`, `trace_*`, `net_*`, `web3_*` and associated `Filecoin.*` APIs including Filecoin-specific functions such as `Filecoin.EthAddressToFilecoinAddress` and `Filecoin.FilecoinAddressToEthAddress`) with different finality handling:
* **`/v2` APIs (New & Experimental):** These APIs consult the F3 subsystem (if enabled) for finality information.
  * `"finalized"` tag maps to the F3 finalized epoch.
  * `"safe"` tag maps to the F3 finalized epoch or 200 epochs behind head, whichever is more recent.
* **`/v1` APIs (Existing & Recommended):** These maintain behavior closer to pre-F3 Lotus (EC finality) for compatibility.
  * `"finalized"` tag continues to use a fixed 900-epoch delay from the head (EC finality).
  * `"safe"` tag uses a 30-epoch delay from the head.
  * _One or both of these tags may be adjusted in a future upgrade to take advantage of F3 finality._
* **Note:** Previously, `"finalized"` and `"safe"` tags referred to epochs `N-1`. This `-1` offset has been removed in both V1 and V2.
* Additional improvements affecting **both `/v1` and `/v2`** Ethereum APIs:
  * `eth_getBlockTransactionCountByNumber` now accepts standard Ethereum block specifiers (hex numbers _or_ tags like `"latest"`, `"safe"`, `"finalized"`).
  * Methods accepting `BlockNumberOrHash` now support all standard tags (`"pending"`, `"latest"`, `"safe"`, `"finalized"`). This includes `eth_estimateGas`, `eth_call`, `eth_getCode`, `eth_getStorageAt`, `eth_getBalance`, `eth_getTransactionCount`, and `eth_getBlockReceipts`.
  * Removed internal `Eth*Limited` methods (e.g., `EthGetTransactionByHashLimited`) from the supported gateway API surface.
  * Improved error handling: block selection endpoints now consistently return `ErrNullRound` (and corresponding JSONRPC errors) for null tipsets.

This work was done in ([filecoin-project/lotus#13026](https://github.com/filecoin-project/lotus/pull/13026)), ([filecoin-project/lotus#13070](https://github.com/filecoin-project/lotus/pull/13070)).

### Others
- feat: add gas to application metric reporting `vm/applyblocks_early_gas`, `vm/applyblocks_messages_gas`, `vm/applyblocks_cron_gas` ([filecoin-project/lotus#13030](https://github.com/filecoin-project/lotus/pull/13030))
- feat(metrics): capture total gas metric from ApplyBlocks ([filecoin-project/lotus#13037](https://github.com/filecoin-project/lotus/pull/13037))
- feat: add F3 Grafana Dashboard Template ([filecoin-project/lotus#12934](https://github.com/filecoin-project/lotus/pull/12934))
- fix(f3): limit the concurrency of F3 power table calculation ([filecoin-project/lotus#13085](https://github.com/filecoin-project/lotus/pull/13085))
- feat(f3): remove dynnamic manifest functionality and use static manifest ([filecoin-project/lotus#13074](https://github.com/filecoin-project/lotus/pull/13074))

## 🐛 Bug Fix Highlights
- fix(eth): apply limit in EthGetBlockReceiptsLimited ([filecoin-project/lotus#12883](https://github.com/filecoin-project/lotus/pull/12883))
- fix(eth): always return nil for eth transactions not found ([filecoin-project/lotus#12999](https://github.com/filecoin-project/lotus/pull/12999))
- fix(deps): fix Ledger hardware wallet support ([filecoin-project/lotus#13048](https://github.com/filecoin-project/lotus/pull/13048))

## 📝 Changelog

For the full set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/release/v1.32.3...release/v1.33.0

## 👨‍👩‍👧‍👦 Contributors

Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Rod Vagg | 19 | +13805/-3639 | 129 |
| Masih H. Derkani | 19 | +11910/-2369 | 119 |
| Jakub Sztandera | 14 | +2528/-202 | 32 |
| Phi-rjan | 12 | +1707/-79 | 42 |
| Steve Loeppky | 3 | +1287/-32 | 8 |
| Piotr Galar | 2 | +298/-3 | 4 |
| Barbara Peric | 3 | +182/-73 | 5 |
| ZenGround0 | 1 | +191/-0 | 4 |
| CoolCu | 1 | +15/-49 | 6 |
| Volker Mische | 1 | +18/-31 | 5 |
| Phi | 3 | +32/-14 | 10 |
| dependabot[bot] | 1 | +15/-15 | 2 |
| Amit Gaikwad | 1 | +19/-2 | 2 |
| tom | 1 | +0/-14 | 2 |
| xixishidibei | 1 | +2/-11 | 1 |
| Tomass | 1 | +4/-4 | 2 |
| tsinghuacoder | 1 | +3/-2 | 1 |
| dropbigfish | 1 | +1/-1 | 1 |
| James Niken | 1 | +1/-1 | 1 |
| Hubert | 1 | +1/-0 | 1 |
| Steven Allen | 1 | +0/-0 | 2 |

# Node v1.32.3 / 2025-04-29

This Node v1.32.3 patch release contains a critical update for all node operators. This release ensures that the F3 initial power table CID is correctly set in your Lotus node now that F3 is enabled on Mainnet. All node operators must upgrade to this release before their next node restart to ensure proper F3 functionality.

## ☢️ Upgrade Warnings ☢️
- All node operators must upgrade to this release before their next node restart to ensure proper F3 functionality. Storage providers only needs to upgrade their Lotus chain node to this release.

## 📝 Changelog

- feat: set F3 initial power table for mainnet ([filecoin-project/lotus#13077](https://github.com/filecoin-project/lotus/pull/13077))

For the set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/v1.31.2...v1.32.3

# Node and Miner v1.32.2 / 2025-04-04

This Lotus v1.32.2 release is a **MANDATORY patch release**. After the Calibration network upgraded to nv25, a bug was discovered in the ref-fvm KAMT library affecting ERC-20 token minting operations. You can read the full technical breakdown of the issue [here](https://github.com/filecoin-project/builtin-actors/pull/1667).

This patch release includes the following updates:
- Schedules a mandatory Calibration upgrade, happening on `2025-04-07T23:00:00Z`, to fix the ERC-20 token minting bug on the Calibration network.
- Postpones the mandatory Mainnet nv25 upgrade by 4 days, to `2025-04-14T23:00:00Z`

## ☢️ Upgrade Warnings ☢️
- All Lotus node and Storage Provider (SP) operators must upgrade to this patch release before the specified dates for the Calibration and Mainnet networks.
- Please check the upgrade warning section for the [v1.32.1 release](https://github.com/filecoin-project/lotus/releases/tag/v1.32.1) for more upgrade warnings if you are upgrading from a version prior to v1.32.0.

## 🏛️ Filecoin network version 25 FIPs

- [FIP-0097: Add Support for EIP-1153 (Transient Storage) in the FEVM](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0097.md)
- [FIP-0098: Simplify termination fee calculation to a fixed percentage of initial pledge](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0098.md)
- [FIP-0100: Removing Batch Balancer, Replacing It With a Per-sector Fee and Removing Gas-limited Constraints](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0100.md)
- [F3 Mainnet Activation](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0086.md)

## 📦 v16 Builtin Actor Bundle

This release candidate uses the [v16.0.1](https://github.com/filecoin-project/builtin-actors/releases/tag/v16.0.1)

## 🚚 Migration
All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 120 epochs before the network upgrade. The migration for the NV25 upgrade is expected to be medium with a bit longer pre-migration compared to the two previous network upgrade.

Pre-Migration is expected to take between 4 to 8 minutes on a SplitStore node. The migration on the upgrade epoch is expected to take 30 seconds on a node with a NVMe-drive and a newer CPU. For nodes running on slower disks/CPU, it is still expected to take around 1 minute. We recommend node operators (who haven't enabled splitstore discard mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

For certain node operators, such as full archival nodes or systems that need to keep large amounts of state (RPC providers), we recommend skipping the pre-migration and run the non-cached migration (i.e., just running the migration at the network upgrade epoch), and schedule for some additional downtime. Operators of such nodes can read the [How to disable premigration in network upgrade tutorial](https://lotus.filecoin.io/kb/disable-premigration/).

## Bug Fixes and Chores
- feat!: actors bundle v16.0.1 & special handling for calibnet ([filecoin-project/lotus#13006](https://github.com/filecoin-project/lotus/pull/13006)).
- chore: update new Mainnet nv25 date to 2025-04-14T23:00:00Z ([filecoin-project/lotus#13007](https://github.com/filecoin-project/lotus/pull/13007)).
- chore: make TockFix epoch for 2k network configurable ([filecoin-project/lotus#13008](https://github.com/filecoin-project/lotus/pull/13008)).
- chore(deps): update filecoin-ffi ([filecoin-project/lotus#13011](https://github.com/filecoin-project/lotus/pull/13011)).

## 📝 Changelog

For the set of changes since the last stable release:

- Node: https://github.com/filecoin-project/lotus/compare/v1.31.1...v1.32.2
- Miner: https://github.com/filecoin-project/lotus/compare/v1.31.1...miner/v1.32.2

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
- feat: add a `LOTUS_DISABLE_F3_ACTIVATION` environment variable allowing disabling F3 activation for a specific contract address or epoch ([filecoin-project/lotus#12920](https://github.com/filecoin-project/lotus/pull/12920)).
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
- feat: add a `LOTUS_DISABLE_F3_ACTIVATION` environment variable allowing disabling F3 activation for a specific contract address or epoch ([filecoin-project/lotus#12920](https://github.com/filecoin-project/lotus/pull/12920)).
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
- New ChainIndexer subsystem to index Filecoin chain state such as tipsets, messages, events and ETH transactions for accurate and faster RPC responses. The `ChainIndexer` replaces the existing `MsgIndex`, `EthTxHashLookup` and `EventIndex` implementations in Lotus, which [suffer from a multitude of known problems](https://github.com/filecoin-project/lotus/issues/12293).  If you are an RPC provider or a node operator who uses or exposes Ethereum and/or events APIs, please refer to the [ChainIndexer documentation for operators](./documentation/en/chain-indexer-overview-for-operators.md) for information on how to enable, configure and use the new Indexer.  While there is no automated data migration and one can upgrade and downgrade without backups, there are manual steps that need to be taken to backfill data when upgrading to this Lotus version, or downgrading to the previous version without ChainIndexer. Please be aware that this feature removes some options in the Lotus configuration file, if these have been set, Lotus will report an error when starting. See the documentation for more information
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

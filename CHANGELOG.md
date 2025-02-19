# Lotus changelog

> **Historical Note**  
> Previous changelog entries are archived in:
> * [CHANGELOG_0.x.md](./documentation/changelog/CHANGELOG_0.x.md) - v0.1.0 to v0.9.1
> * [CHANGELOG_1.0x.md](./documentation/changelog/CHANGELOG_1.0x.md) - v1.0.0 to v1.9.0
> * [CHANGELOG_1.1x.md](./documentation/changelog/CHANGELOG_1.1x.md) - v1.10.0 to v1.19.0
> * [CHANGELOG_1.2x.md](./documentation/changelog/CHANGELOG_1.2x.md) - v1.20.0 to v1.29.2

# UNRELEASED

- Exposed `StateGetNetworkParams` in the Lotus Gateway API ([filecoin-project/lotus#12881](https://github.com/filecoin-project/lotus/pull/12881))
- **BREAKING**: Removed `SupportedProofTypes` from `StateGetNetworkParams` response as it was unreliable and didn't match FVM's actual supported proofs ([filecoin-project/lotus#12881](https://github.com/filecoin-project/lotus/pull/12881))
- refactor(eth): attach ToFilecoinMessage converter to EthCall method for improved package/module import structure. This change also exports the converter as a public method, enhancing usability for developers utilizing Lotus as a library. ([filecoin-project/lotus#12844](https://github.com/filecoin-project/lotus/pull/12844))
- chore: switch to pure-go zstd decoder for snapshot imports.  ([filecoin-project/lotus#12857](https://github.com/filecoin-project/lotus/pull/12857))
- feat: automatically detect if the genesis is zstd compressed. ([filecoin-project/lotus#12885](https://github.com/filecoin-project/lotus/pull/12885)
- `lotus send` now supports `--csv` option for sending multiple transactions. ([filecoin-project/lotus#12892](https://github.com/filecoin-project/lotus/pull/12892))

- chore: upgrade to the latest go-f3 and allow F3 chain exchange topics ([filecoin-project/lotus#12893](https://github.com/filecoin-project/lotus/pull/12893)

- chore: upgrade drand client, this increases the Go minimum version to v1.22.10.


# UNRELEASED v.1.32.0

See https://github.com/filecoin-project/lotus/blob/release/v1.32.0/CHANGELOG.md

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

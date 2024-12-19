# Lotus changelog v1.0.0 - v1.9.0

* [1.9.0 / 2021-05-17](#190--2021-05-17)
* [1.8.0 / 2021-04-05](#180--2021-04-05)
* [1.6.0 / 2021-04-05](#160--2021-04-05)
* [1.5.3 / 2021-03-24](#153--2021-03-24)
* [1.5.2 / 2021-03-11](#152--2021-03-11)
* [1.5.1 / 2021-03-10](#151--2021-03-10)
* [1.5.0 / 2021-02-23](#150--2021-02-23)
* [1.4.2 / 2021-02-17](#142--2021-02-17)
* [1.4.1 / 2021-01-20](#141--2021-01-20)
* [1.4.0 / 2020-12-19](#140--2020-12-19)
* [1.3.0 / 2020-12-16](#130--2020-12-16)
* [1.2.3 / 2020-12-15](#123--2020-12-15)
* [1.2.2 / 2020-12-03](#122--2020-12-03)
* [1.2.1 / 2020-11-20](#121--2020-11-20)
* [1.2.0 / 2020-11-18](#120--2020-11-18)
* [1.1.3 / 2020-11-13](#113--2020-11-13)
* [1.1.2 / 2020-10-24](#112--2020-10-24)
* [1.1.1 / 2020-10-24](#111--2020-10-24)
* [1.1.0 / 2020-10-20](#110--2020-10-20)
* [1.0.0 / 2020-10-19](#100--2020-10-19)

# 1.9.0 / 2021-05-17

This is an optional Lotus release that introduces various improvements to the sealing, mining, and deal-making processes.

## Highlights

- OpenRPC Support (https://github.com/filecoin-project/lotus/pull/5843)
- Take latency into account when making interactive deals (https://github.com/filecoin-project/lotus/pull/5876)
- Update go-commp-utils for >10x faster client commp calculation (https://github.com/filecoin-project/lotus/pull/5892)
- add `lotus client cancel-retrieval` cmd to lotus CLI (https://github.com/filecoin-project/lotus/pull/5871)
- add `inspect-deal` command to `lotus client` (https://github.com/filecoin-project/lotus/pull/5833)
- Local retrieval support (https://github.com/filecoin-project/lotus/pull/5917)
- go-fil-markets v1.1.9 -> v1.2.5
  - For a detailed changelog see https://github.com/filecoin-project/go-fil-markets/blob/master/CHANGELOG.md
- rust-fil-proofs v5.4.1 -> v7.0.1
  - For a detailed changelog see https://github.com/filecoin-project/rust-fil-proofs/blob/master/CHANGELOG.md

## Changes
- storagefsm: Apply global events even in broken states (https://github.com/filecoin-project/lotus/pull/5962)
- Default the AlwaysKeepUnsealedCopy flag to true (https://github.com/filecoin-project/lotus/pull/5743)
- splitstore: compact hotstore prior to garbage collection (https://github.com/filecoin-project/lotus/pull/5778)
- ipfs-force bootstrapper update (https://github.com/filecoin-project/lotus/pull/5799)
- better logging when unsealing fails (https://github.com/filecoin-project/lotus/pull/5851)
- perf: add cache for gas permium estimation (https://github.com/filecoin-project/lotus/pull/5709)
- backupds: Compact log on restart (https://github.com/filecoin-project/lotus/pull/5875)
- backupds: Improve truncated log handling (https://github.com/filecoin-project/lotus/pull/5891)
- State CLI improvements (State CLI improvements)
- API proxy struct codegen (https://github.com/filecoin-project/lotus/pull/5854)
- move DI stuff for paychmgr into modules (https://github.com/filecoin-project/lotus/pull/5791)
- Implement Event observer and Settings for 3rd party dep injection (https://github.com/filecoin-project/lotus/pull/5693)
- Export developer and network commands for consumption by derivatives of Lotus (https://github.com/filecoin-project/lotus/pull/5864)
- mock sealer: Simulate randomness sideeffects (https://github.com/filecoin-project/lotus/pull/5805)
- localstorage: Demote reservation stat error to debug (https://github.com/filecoin-project/lotus/pull/5976)
- shed command to unpack miner info dumps (https://github.com/filecoin-project/lotus/pull/5800)
- Add two utils to Lotus-shed (https://github.com/filecoin-project/lotus/pull/5867)
- add shed election estimate command  (https://github.com/filecoin-project/lotus/pull/5092)
- Add --actor flag in lotus-shed sectors terminate (https://github.com/filecoin-project/lotus/pull/5819)
- Move lotus mpool clear to lotus-shed (https://github.com/filecoin-project/lotus/pull/5900)
- Centralize everything on ipfs/go-log/v2 (https://github.com/filecoin-project/lotus/pull/5974)
- expose NextID from nice market actor interface (https://github.com/filecoin-project/lotus/pull/5850)
- add available options for perm on error (https://github.com/filecoin-project/lotus/pull/5814)
- API docs clarification: Document StateSearchMsg replaced message behavior (https://github.com/filecoin-project/lotus/pull/5838)
- api: Document StateReplay replaced message behavior (https://github.com/filecoin-project/lotus/pull/5840)
- add godocs to miner objects (https://github.com/filecoin-project/lotus/pull/2184)
- Add description to the client deal CLI command (https://github.com/filecoin-project/lotus/pull/5999)
- lint: don't skip builtin (https://github.com/filecoin-project/lotus/pull/5881)
- use deal duration from actors (https://github.com/filecoin-project/lotus/pull/5270)
- remote calc winningpost proof (https://github.com/filecoin-project/lotus/pull/5884)
- packer: other network images (https://github.com/filecoin-project/lotus/pull/5930)
- Convert the chainstore lock to RW (https://github.com/filecoin-project/lotus/pull/5971)
- Remove CachedBlockstore (https://github.com/filecoin-project/lotus/pull/5972)
- remove messagepool CapGasFee duplicate code (https://github.com/filecoin-project/lotus/pull/5992)
- Add a mining-heartbeat INFO line at every epoch (https://github.com/filecoin-project/lotus/pull/6183)
- chore(ci): Enable build on RC tags (https://github.com/filecoin-project/lotus/pull/6245)
- Upgrade nerpa to actor v4 and bump the version to rc4 (https://github.com/filecoin-project/lotus/pull/6249)
## Fixes
- return buffers after canceling badger operation (https://github.com/filecoin-project/lotus/pull/5796)
- avoid holding a lock while calling the View callback (https://github.com/filecoin-project/lotus/pull/5792)
- storagefsm: Trigger input processing when below limits (https://github.com/filecoin-project/lotus/pull/5801)
- After importing a previously deleted key, be able to delete it again (https://github.com/filecoin-project/lotus/pull/4653)
- fix StateManager.Replay on reward actor (https://github.com/filecoin-project/lotus/pull/5804)
- make sure atomic 64bit fields are 64bit aligned (https://github.com/filecoin-project/lotus/pull/5794)
- Import secp sigs in paych tests (https://github.com/filecoin-project/lotus/pull/5879)
- fix ci build-macos (https://github.com/filecoin-project/lotus/pull/5934)
- Fix creation of remainder account when it's not a multisig (https://github.com/filecoin-project/lotus/pull/5807)
- Fix fallback chainstore (https://github.com/filecoin-project/lotus/pull/6003)
- fix 4857: show help for set-addrs (https://github.com/filecoin-project/lotus/pull/5943)
- fix health report (https://github.com/filecoin-project/lotus/pull/6011)
- fix(ci): Use recent ubuntu LTS release; Update release params ((https://github.com/filecoin-project/lotus/pull/6011))

# 1.8.0 / 2021-04-05

This is a mandatory release of Lotus that upgrades the network to version 12, which introduces various performance improvements to the cron processing of the power actor. The network will upgrade at height 712320, which is 2021-04-29T06:00:00Z.

## Changes

- v4 specs-actors integration, nv12 migration (https://github.com/filecoin-project/lotus/pull/6116)

# 1.6.0 / 2021-04-05

This is a mandatory release of Lotus that upgrades the network to version 11, which implements [FIP-0014](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0014.md). The network will upgrade at height 665280, which is 2021-04-12T22:00:00Z.

## v1 sector extension CLI

This release also expands the `lotus-miner sectors extend` CLI, with a new option that automatically extends all extensible v1 sectors. The option can be run using `lotus-miner sectors extend --v1-sectors`.

- The `tolerance` flag can be passed to indicate what durations aren't "worth" extending. It defaults to one week, which means that sectors whose current lifetime's are within one week of the maximum possible lifetime will not be extended.

- The `expiration-cutoff` flag can be passed to skip sectors whose expiration is past a certain point from the current head. It defaults to infinity (no cutoff), but if, say, 28800 was specified, then only sectors expiring in the next 10 days would be extended (2880 epochs in 1 day).

## Changes

- Util for miners to extend all v1 sectors (https://github.com/filecoin-project/lotus/pull/5924)
- Upgrade the butterfly network (https://github.com/filecoin-project/lotus/pull/5929)
- Introduce the v11 network upgrade (https://github.com/filecoin-project/lotus/pull/5904)
- Debug mode: Make upgrade heights controllable by an envvar (https://github.com/filecoin-project/lotus/pull/5919)

# 1.5.3 / 2021-03-24

This is a patch release of Lotus that introduces small fixes to the Storage FSM.

## Changes

- storagefsm: Fix double unlock with ready WaitDeals sectors (https://github.com/filecoin-project/lotus/pull/5783)
- backupds: Allow larger values in write log (https://github.com/filecoin-project/lotus/pull/5776)
- storagefsm: Don't log the SectorRestart event (https://github.com/filecoin-project/lotus/pull/5779)

# 1.5.2 / 2021-03-11

This is an hotfix release of Lotus that fixes a critical bug introduced in v1.5.1 in the miner windowPoSt logic. This upgrade is only affecting miner nodes.

## Changes
- fix window post rand check (https://github.com/filecoin-project/lotus/pull/5773)
- wdpost: Always use head tipset to get randomness (https://github.com/filecoin-project/lotus/pull/5774)

# 1.5.1 / 2021-03-10

This is an optional release of Lotus that introduces an important fix to the WindowPoSt computation process. The change is to wait for some confidence before drawing beacon randomness for the proof. Without this, invalid proofs might be generated as the result of a null tipset.

## Splitstore

This release also introduces the splitstore, a new optional blockstore that segregates the monolithic blockstore into cold and hot regions. The hot region contains objects from the last 4-5 finalities plus all reachable objects from two finalities away. All other objects are moved to the cold region using a compaction process that executes every finality, once 5 finalities have elapsed.

The splitstore allows us to separate the two regions quite effectively, using two separate badger blockstores. The separation
means that the live working set is much smaller, which results in potentially significant performance improvements. In addition, it means that the coldstore can be moved to a separate (bigger, slower, cheaper) disk without loss of performance.

The design also allows us to use different implementations for the two blockstores; for example, an append-only blockstore could be used for coldstore and a faster memory mapped blockstore could be used for the hotstore (eg LMDB). We plan to experiment with these options in the future.

Once the splitstore has been enabled, the existing monolithic blockstore becomes the coldstore. On the first head change notification, the splitstore will warm up the hotstore by copying all reachable objects from the current tipset into the hotstore.  All new writes go into the hotstore, with the splitstore tracking the write epoch. Once 5 finalities have elapsed, and every finality thereafter, the splitstore compacts by moving cold objects into the coldstore. There is also experimental support for garbage collection, whereby nunreachable objects are simply discarded.

To enable the splitstore, add the following to config.toml:

```
[Chainstore]
  EnableSplitstore = true
```

## Highlights

Other highlights include:

- Improved deal data handling - now multiple deals can be adding to sectors in parallel
- Rewriten sector pledging - it now actually cares about max sealing sector limits
- Better handling for sectors stuck in the RecoverDealIDs state
- lotus-miner sectors extend command
- Optional configurable storage path size limit
- Config to disable owner/worker fallback from control addresses (useful when owner is a key on a hardware wallet)
- A write log for node metadata, which can be restored as a backup when the metadata leveldb becomes corrupted (e.g. when you run out of disk space / system crashes in some bad way)

## Changes

- avoid use mp.cfg directly to avoid race (https://github.com/filecoin-project/lotus/pull/5350)
- Show replacing message CID is state search-msg cli (https://github.com/filecoin-project/lotus/pull/5656)
- Fix riceing by importing the main package (https://github.com/filecoin-project/lotus/pull/5675)
- Remove sectors with all deals expired in RecoverDealIDs (https://github.com/filecoin-project/lotus/pull/5658)
- storagefsm: Rewrite input handling (https://github.com/filecoin-project/lotus/pull/5375)
- reintroduce Refactor send command for better testability (https://github.com/filecoin-project/lotus/pull/5668)
- Improve error message with importing a chain (https://github.com/filecoin-project/lotus/pull/5669)
- storagefsm: Cleanup CC sector creation (https://github.com/filecoin-project/lotus/pull/5612)
- chain list --gas-stats display capacity (https://github.com/filecoin-project/lotus/pull/5676)
- Correct some logs (https://github.com/filecoin-project/lotus/pull/5694)
- refactor blockstores (https://github.com/filecoin-project/lotus/pull/5484)
- Add idle to sync stage's String() (https://github.com/filecoin-project/lotus/pull/5702)
- packer provisioner (https://github.com/filecoin-project/lotus/pull/5604)
- add DeleteMany to Blockstore interface (https://github.com/filecoin-project/lotus/pull/5703)
- segregate chain and state blockstores (https://github.com/filecoin-project/lotus/pull/5695)
- fix(multisig): The format of the amount is not correct in msigLockApp (https://github.com/filecoin-project/lotus/pull/5718)
- Update butterfly network (https://github.com/filecoin-project/lotus/pull/5627)
- Collect worker task metrics (https://github.com/filecoin-project/lotus/pull/5648)
- Correctly format disputer log (https://github.com/filecoin-project/lotus/pull/5716)
- Log block CID in the large delay warning (https://github.com/filecoin-project/lotus/pull/5704)
- Move api client builders to a cliutil package (https://github.com/filecoin-project/lotus/pull/5728)
- Implement net peers --extended (https://github.com/filecoin-project/lotus/pull/5734)
- Command to extend sector expiration (https://github.com/filecoin-project/lotus/pull/5666)
- garbage collect hotstore after compaction (https://github.com/filecoin-project/lotus/pull/5744)
- tune badger gc to repeatedly gc the value log until there is no rewrite (https://github.com/filecoin-project/lotus/pull/5745)
- Add configuration option for pubsub IPColocationWhitelist subnets (https://github.com/filecoin-project/lotus/pull/5735)
- hot/cold blockstore segregation (aka. splitstore) (https://github.com/filecoin-project/lotus/pull/4992)
- Customize verifreg root key and remainder account when making genesis (https://github.com/filecoin-project/lotus/pull/5730)
- chore: update go-graphsync to 0.6.0 (https://github.com/filecoin-project/lotus/pull/5746)
- Add connmgr metadata to NetPeerInfo (https://github.com/filecoin-project/lotus/pull/5749)
- test: attempt to make the splitstore test deterministic (https://github.com/filecoin-project/lotus/pull/5750)
- Feat/api no dep build (https://github.com/filecoin-project/lotus/pull/5729)
- Fix bootstrapper profile setting (https://github.com/filecoin-project/lotus/pull/5756)
- Check liveness of sectors when processing termination batches (https://github.com/filecoin-project/lotus/pull/5759)
- Configurable storage path storage limit (https://github.com/filecoin-project/lotus/pull/5624)
- miner: Config to disable owner/worker address fallback (https://github.com/filecoin-project/lotus/pull/5620)
- Fix TestUnpadReader on Go 1.16 (https://github.com/filecoin-project/lotus/pull/5761)
- Metadata datastore log (https://github.com/filecoin-project/lotus/pull/5755)
- Remove the SR2 stats, leave just the network totals (https://github.com/filecoin-project/lotus/pull/5757)
- fix: wait a bit before starting to compute window post proofs (https://github.com/filecoin-project/lotus/pull/5764)
- fix: retry proof when randomness changes (https://github.com/filecoin-project/lotus/pull/5768)


# 1.5.0 / 2021-02-23

This is a mandatory release of Lotus that introduces the fifth upgrade to the Filecoin network. The network upgrade occurs at height 550321, before which time all nodes must have updated to this release (or later). At this height, [v3 specs-actors](https://github.com/filecoin-project/specs-actors/releases/tag/v3.0.0) will take effect, which in turn implements the following two FIPs:

- [FIP-0007 h/amt-v3](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0007.md) which improves the performance of the Filecoin HAMT and AMT.
- [FIP-0010 off-chain Window PoSt Verification](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0010.md) which reduces the gas consumption of `SubmitWindowedPoSt` messages significantly by optimistically accepting Window PoSt proofs without verification, and allowing them to be disputed later by off-chain verifiers.

Note that the integration of v3 actors was already completed in 1.4.2, this upgrade simply sets the epoch for the upgrade to occur.

## Disputer

FIP-0010 introduces the ability to dispute bad Window PoSts. Node operators are encouraged to run the new Lotus disputer alongside their Lotus daemons. For more information, see the announcement [here](https://github.com/filecoin-project/lotus/discussions/5617#discussioncomment-387333).

## Changes

- [#5341](https://github.com/filecoin-project/lotus/pull/5341)  Add a  `LOTUS_DISABLE_V3_ACTOR_MIGRATION` envvar
  - Setting this envvar to 1 disables the v3 actor migration, should only be used in the event of a failed migration

# 1.4.2 / 2021-02-17

This is a large, and highly recommended, optional release with new features and improvements for lotus miner and deal-making UX. The release also integrates [v3 specs-actors](https://github.com/filecoin-project/specs-actors/releases/tag/v3.0.0), which implements two FIPs:

- [FIP-0007 h/amt-v3](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0007.md) which improves the performance of the Filecoin HAMT and AMT.
- [FIP-0010 off-chain Window PoSt Verification](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0010.md) which reduces the gas consumption of `SubmitWindowedPoSt` messages significantly by optimistically accepting Window PoSt proofs without verification, and allowing them to be disputed later by off-chain verifiers.

Note that this release does NOT set an upgrade epoch for v3 actors to take effect. That will be done in the upcoming 1.5.0 release.

## New Features

- [#5341](https://github.com/filecoin-project/lotus/pull/5341)  Added sector termination API and CLI
  - Run `lotus-miner sectors terminate`
- [#5342](https://github.com/filecoin-project/lotus/pull/5342) Added CLI for using a multisig wallet as miner's owner address
  - See how to set it up [here](https://github.com/filecoin-project/lotus/pull/5342#issue-554009129)
- [#5363](https://github.com/filecoin-project/lotus/pull/5363), [#5418](https://github.com/filecoin-project/lotus/pull/), [#5476](https://github.com/filecoin-project/lotus/pull/5476), [#5459](https://github.com/filecoin-project/lotus/pull/5459) Integrated [spec-actor v3](https://github.com/filecoin-pro5418ject/specs-actors/releases/tag/v3.0.0)
  - [#5472](https://github.com/filecoin-project/lotus/pull/5472) Generate actor v3 methods for pond
- [#5379](https://github.com/filecoin-project/lotus/pull/5379) Added WindowPoSt disputer
  - This is to support [FIP-0010 off-chian Window PoSt verification](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0010.md)
  - See how to run a disputer [here](https://github.com/filecoin-project/lotus/pull/5379#issuecomment-776482445)
- [#5309](https://github.com/filecoin-project/lotus/pull/5309) Batch multiple deals in one `PublishStorageMessages`
  - [#5411](https://github.com/filecoin-project/lotus/pull/5411) Handle batch `PublishStorageDeals` message in sealing recovery
  - [#5505](https://github.com/filecoin-project/lotus/pull/5505) Exclude expired deals from batching in `PublishStorageDeals` messages
  - Added `PublishMsgPeriod` and `MaxDealsPerPublishMsg` to miner `Dealmaking` [configuration](https://lotus.filecoin.io/storage-providers/advanced-configurations/market/#dealmaking-section). See how they work [here](https://lotus.filecoin.io/storage-providers/advanced-configurations/market/#publishing-several-deals-in-one-message).
  - [#5538](https://github.com/filecoin-project/lotus/pull/5538), [#5549](https://github.com/filecoin-project/lotus/pull/5549) Added a command to list pending deals and force publish messages.
    - Run `lotus-miner market pending-publish`
  - [#5428](https://github.com/filecoin-project/lotus/pull/5428) Moved waiting for `PublishStorageDeals` messages' receipt from markets to lotus
- [#5510](https://github.com/filecoin-project/lotus/pull/5510) Added `nerpanet` build option
  - To build `nerpanet`, run `make nerpanet`
- [#5433](https://github.com/filecoin-project/lotus/pull/5433) Added `AlwaysKeepUnsealedCopy` option to the miner configuration
- [#5520](https://github.com/filecoin-project/lotus/pull/5520) Added `MsigGetPending` to get pending transactions for multisig wallets
- [#5219](https://github.com/filecoin-project/lotus/pull/5219) Added interactive mode for lotus-wallet
- [5529](https://github.com/filecoin-project/lotus/pull/5529) Added support for minder nodes in `lotus-shed rpc` util

## Bug Fixes

- [#5210](https://github.com/filecoin-project/lotus/pull/5210) Miner should not dial client on restart
- [#5403](https://github.com/filecoin-project/lotus/pull/5403) When estimating GasLimit only apply prior messages up to the nonce
- [#5410](https://github.com/filecoin-project/lotus/pull/510) Fix the calibnet build option
- [#5492](https://github.com/filecoin-project/lotus/pull/5492) Fixed `has` for ipfsbstore for non-existing blocks
- [#5361](https://github.com/filecoin-project/lotus/pull/5361) Fixed retrieval hangs when using `IpfsOnlineMode=true`
- [#5493](https://github.com/filecoin-project/lotus/pull/5493) Fixed retrieval failure when price-per-byte is zero
- [#5506](https://github.com/filecoin-project/lotus/pull/5506) Fixed contexts in the storage adpater
- [#5515](https://github.com/filecoin-project/lotus/pull/5515) Properly wire up `StateReadState` on gateway API
- [#5582](https://github.com/filecoin-project/lotus/pull/5582) Fixed error logging format strings
- [#5614](https://github.com/filecoin-project/lotus/pull/5614) Fixed websocket reconnecting handling


## Improvements

- [#5389](https://github.com/filecoin-project/lotus/pull/5389) Show verified indicator for `./lotus-miner storage-deals list`
- [#5229](https://github.com/filecoin-project/lotus/pull/5220) Show power for verified deals in `./lotus-miner setocr list`
- [#5407](https://github.com/filecoin-project/lotus/pull/5407) Added explicit check of the miner address protocol
- [#5399](https://github.com/filecoin-project/lotus/pull/5399) watchdog: increase heapprof capture threshold to 90%
- [#5398](https://github.com/filecoin-project/lotus/pull/5398) storageadapter: Look at precommits on-chain since deal publish msg
- [#5470](https://github.com/filecoin-project/lotus/pull/5470) Added `--no-timing` option for `./lotus state compute-state --html`
- [#5417](https://github.com/filecoin-project/lotus/pull/5417) Storage Manager: Always unseal full sectors
- [#5393](https://github.com/filecoin-project/lotus/pull/5393) Switched to [filecoin-ffi bls api ](https://github.com/filecoin-project/filecoin-ffi/pull/159)for bls signatures
- [#5380](https://github.com/filecoin-project/lotus/pull/5210) Refactor deals API tests
- [#5397](https://github.com/filecoin-project/lotus/pull/5397) Fixed a flake in the sync manager edge case test
- [#5406](https://github.com/filecoin-project/lotus/pull/5406) Added a test to ensure a correct window post cannot be disputed
- [#5294](https://github.com/filecoin-project/lotus/pull/5394) Added jobs to build Lotus docker image and push it to AWS ECR
- [#5387](https://github.com/filecoin-project/lotus/pull/5387) Added network info(mainnet|calibnet) in version
- [#5497](https://github.com/filecoin-project/lotus/pull/5497) Export metric for lotus-gateaway
- [#4950](https://github.com/filecoin-project/lotus/pull/4950) Removed bench policy
- [#5047](https://github.com/filecoin-project/lotus/pull/5047) Improved the UX for `./lotus-shed bitfield enc`
- [#5282](https://github.com/filecoin-project/lotus/pull/5282) Snake a context through the chian blockstore creation
- [#5350](https://github.com/filecoin-project/lotus/pull/5350) Avoid using `mp.cfg` directrly to prevent race condition
- [#5449](https://github.com/filecoin-project/lotus/pull/5449) Documented the block-header better
- [#5404](https://github.com/filecoin-project/lotus/pull/5404) Added retrying proofs if an incorrect one is generated
- [#4545](https://github.com/filecoin-project/lotus/pull/4545) Made state tipset usage consistent in the API
- [#5540](https://github.com/filecoin-project/lotus/pull/5540) Removed unnecessary database reads in validation check
- [#5554](https://github.com/filecoin-project/lotus/pull/5554) Fixed `build lotus-soup` CI job
- [#5552](https://github.com/filecoin-project/lotus/pull/5552) Updated CircleCI to halt gracefully
- [#5555](https://github.com/filecoin-project/lotus/pull/5555) Cleanup and add docstrings of node builder
- [#5564](https://github.com/filecoin-project/lotus/pull/5564) Stopped depending on gocheck with gomod
- [#5574](https://github.com/filecoin-project/lotus/pull/5574) Updated CLI UI
- [#5570](https://github.com/filecoin-project/lotus/pull/5570) Added code CID to `StateReadState` return object
- [#5565](https://github.com/filecoin-project/lotus/pull/5565) Added storageadapter.PublishMsgConfig to miner in testkit for lotus-soup testplan
- [#5571](https://github.com/filecoin-project/lotus/pull/5571) Added `lotus-seed gensis car` to generate lotus block for devnets
- [#5613](https://github.com/filecoin-project/lotus/pull/5613) Check format in client commP util
- [#5507](https://github.com/filecoin-project/lotus/pull/5507) Refactored coalescing logic into its own function and take both cancellation sets into account
- [#5592](https://github.com/filecoin-project/lotus/pull/5592) Verify FFI version before building

## Dependency Updates
- [#5296](https://github.com/filecoin-project/lotus/pull/5396) Upgraded to [raulk/go-watchdog@v1.0.1](https://github.com/raulk/go-watchdog/releases/tag/v1.0.1)
- [#5450](https://github.com/filecoin-project/lotus/pull/5450) Dependency updates
- [#5425](https://github.com/filecoin-project/lotus/pull/5425) Fixed stale imports in testplans/lotus-soup
- [#5535](https://github.com/filecoin-project/lotus/pull/5535) Updated to [go-fil-markets@v1.1.7](https://github.com/filecoin-project/go-fil-markets/releases/tag/v1.1.7)
- [#5616](https://github.com/filecoin-project/lotus/pull/5600) Updated to [filecoin-ffi@b6e0b35fb49ed0fe](https://github.com/filecoin-project/filecoin-ffi/releases/tag/b6e0b35fb49ed0fe)
- [#5599](https://github.com/filecoin-project/lotus/pull/5599) Updated to [go-bitfield@v0.2.4](https://github.com/filecoin-project/go-bitfield/releases/tag/v0.2.4)
- [#5614](https://github.com/filecoin-project/lotus/pull/5614), , [#5621](https://github.com/filecoin-project/lotus/pull/5621) Updated to [go-jsonrpc@v0.1.3](https://github.com/filecoin-project/go-jsonrpc/releases/tag/v0.1.3)
- [#5459](https://github.com/filecoin-project/lotus/pull/5459) Updated to [spec-actors@v3.0.1](https://github.com/filecoin-project/specs-actors/releases/tag/v3.0.1)


## Network Version v10 Upgrade
- [#5473](https://github.com/filecoin-project/lotus/pull/5473) Merged staging branch for v1.5.0
- [#5603](https://github.com/filecoin-project/lotus/pull/5603) Set nerpanet's upgrade epochs up to v3 actors
- [#5471](https://github.com/filecoin-project/lotus/pull/5471), [#5456](https://github.com/filecoin-project/lotus/pull/5456) Set calibration net actor v3 migration epochs for testing
- [#5434](https://github.com/filecoin-project/lotus/pull/5434) Implemented pre-migration framework
- [#5476](https://github.com/filecoin-project/lotus/pull/5477) Tune migration

# 1.4.1 / 2021-01-20

This is an optional Lotus release that introduces various improvements to the sealing, mining, and deal-making processes. In particular, [#5341](https://github.com/filecoin-project/lotus/pull/5341) introduces the ability for Lotus miners to terminate sectors.

## Changes

#### Core Lotus

- fix(sync): enforce ForkLengthThreshold for synced chain (https://github.com/filecoin-project/lotus/pull/5182)
- introduce memory watchdog; LOTUS_MAX_HEAP (https://github.com/filecoin-project/lotus/pull/5101)
- Skip bootstrapping if no peers specified (https://github.com/filecoin-project/lotus/pull/5301)
- Chainxchg write response timeout (https://github.com/filecoin-project/lotus/pull/5254)
- update NewestNetworkVersion (https://github.com/filecoin-project/lotus/pull/5277)
- fix(sync): remove checks bypass when we submit the block (https://github.com/filecoin-project/lotus/pull/4192)
- chore: export vm.ShouldBurn (https://github.com/filecoin-project/lotus/pull/5355)
- fix(sync): enforce fork len when changing head (https://github.com/filecoin-project/lotus/pull/5244)
- Use 55th percentile instead of median for gas-price (https://github.com/filecoin-project/lotus/pull/5369)
- update go-libp2p-pubsub to v0.4.1 (https://github.com/filecoin-project/lotus/pull/5329)

#### Sealing

- Sector termination support (https://github.com/filecoin-project/lotus/pull/5341)
- update weight canSeal and canStore when attach (https://github.com/filecoin-project/lotus/pull/5242/files)
- sector-storage/mock: improve mocked readpiece (https://github.com/filecoin-project/lotus/pull/5208)
- Fix deadlock in runWorker in sched_worker.go (https://github.com/filecoin-project/lotus/pull/5251)
- Skip checking terminated sectors provable (https://github.com/filecoin-project/lotus/pull/5217)
- storagefsm: Fix unsealedInfoMap.lk init race (https://github.com/filecoin-project/lotus/pull/5319)
- Multicore AddPiece CommP (https://github.com/filecoin-project/lotus/pull/5320)
- storagefsm: Send correct event on ErrExpiredTicket in CommitFailed (https://github.com/filecoin-project/lotus/pull/5366)
- expose StateSearchMessage on gateway (https://github.com/filecoin-project/lotus/pull/5382)
- fix FileSize to return correct disk usage recursively (https://github.com/filecoin-project/lotus/pull/5384)

#### Dealmaking

- Better error message when withdrawing funds (https://github.com/filecoin-project/lotus/pull/5293)
- add verbose for list transfers (https://github.com/filecoin-project/lotus/pull/5259)
- cli - rename `client info` to `client balances` (https://github.com/filecoin-project/lotus/pull/5304)
- Better CLI for wallet market withdraw and client info (https://github.com/filecoin-project/lotus/pull/5303)

#### UX

- correct flag usages for replace cmd (https://github.com/filecoin-project/lotus/pull/5255)
- lotus state call will panic (https://github.com/filecoin-project/lotus/pull/5275)
- fix get sector bug (https://github.com/filecoin-project/lotus/pull/4976)
- feat: lotus wallet market add (adds funds to storage market actor) (https://github.com/filecoin-project/lotus/pull/5300)
- Fix client flag parsing in client balances cli (https://github.com/filecoin-project/lotus/pull/5312)
- delete slash-consensus miner (https://github.com/filecoin-project/lotus/pull/4577)
- add fund sufficient check in send (https://github.com/filecoin-project/lotus/pull/5252)
- enable parse and shorten negative FIL values (https://github.com/filecoin-project/lotus/pull/5315)
- add limit and rate for chain noise (https://github.com/filecoin-project/lotus/pull/5223)
- add bench env print (https://github.com/filecoin-project/lotus/pull/5222)
- Implement full-node restore option (https://github.com/filecoin-project/lotus/pull/5362)
- add color for token amount (https://github.com/filecoin-project/lotus/pull/5352)
- correct log in maybeUseAddress (https://github.com/filecoin-project/lotus/pull/5359)
- add slash-consensus from flag (https://github.com/filecoin-project/lotus/pull/5378)

#### Testing

- tvx extract: more tipset extraction goodness (https://github.com/filecoin-project/lotus/pull/5258)
- Fix race in blockstore test suite (https://github.com/filecoin-project/lotus/pull/5297)


#### Build & Networks

- Remove LOTUS_DISABLE_V2_ACTOR_MIGRATION envvar (https://github.com/filecoin-project/lotus/pull/5289)
- Create a calibnet build option (https://github.com/filecoin-project/lotus/pull/5288)
- Calibnet: Set Orange epoch (https://github.com/filecoin-project/lotus/pull/5325)

#### Management

- Update SECURITY.md (https://github.com/filecoin-project/lotus/pull/5246)
- README: Contribute section (https://github.com/filecoin-project/lotus/pull/5330)
- README: refine Contribute section (https://github.com/filecoin-project/lotus/pull/5331)
- Add misc tooling to codecov ignore list (https://github.com/filecoin-project/lotus/pull/5347)

# 1.4.0 / 2020-12-19

This is a MANDATORY hotfix release of Lotus that resolves a chain halt at height 336,459 caused by nondeterminism in specs-actors. The fix is to update actors to 2.3.3 in order to incorporate this fix https://github.com/filecoin-project/specs-actors/pull/1334.

# 1.3.0 / 2020-12-16

This is a mandatory release of Lotus that introduces the third post-liftoff upgrade to the Filecoin network. The network upgrade occurs at height 343200, before which time all nodes must have updated to this release (or later). The change that breaks consensus is an implementation of FIP-0009(https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0009.md).

## Changes

- Disable gas burning for window post messages (https://github.com/filecoin-project/lotus/pull/5200)
- fix lock propose (https://github.com/filecoin-project/lotus/pull/5197)

# 1.2.3 / 2020-12-15

This is an optional Lotus release that introduces many performance improvements, bugfixes, and UX improvements.

## Changes

- When waiting for deal commit messages, ignore unsuccessful messages (https://github.com/filecoin-project/lotus/pull/5189)
- Bigger copy buffer size for stores (https://github.com/filecoin-project/lotus/pull/5177)
- Print MinPieceSize when querying ask (https://github.com/filecoin-project/lotus/pull/5178)
- Optimize miner info & sectors list loading (https://github.com/filecoin-project/lotus/pull/5176)
- Allow miners to filter (un)verified deals (https://github.com/filecoin-project/lotus/pull/5094)
- Fix curSealing out of MaxSealingSectors limit (https://github.com/filecoin-project/lotus/pull/5166)
- Add mpool pending from / to filter (https://github.com/filecoin-project/lotus/pull/5169)
- Add metrics for delayed blocks (https://github.com/filecoin-project/lotus/pull/5171)
- Fix PushUntrusted publishing -- the message is local (https://github.com/filecoin-project/lotus/pull/5173)
- Avoid potential hang in events API when starting event listener (https://github.com/filecoin-project/lotus/pull/5159)
- Show data transfer ID in list-deals (https://github.com/filecoin-project/lotus/pull/5150)
- Fix events API mutex locking (https://github.com/filecoin-project/lotus/pull/5160)
- Message pool refactors (https://github.com/filecoin-project/lotus/pull/5162)
- Fix lotus-shed cid output (https://github.com/filecoin-project/lotus/pull/5072)
- Use FundManager to withdraw funds, add MarketWithdraw API (https://github.com/filecoin-project/lotus/pull/5112)
- Add keygen outfile (https://github.com/filecoin-project/lotus/pull/5118)
- Update sr2 stat aggregation (https://github.com/filecoin-project/lotus/pull/5114)
- Fix miner control address lookup (https://github.com/filecoin-project/lotus/pull/5119)
- Fix send with declared nonce 0 (https://github.com/filecoin-project/lotus/pull/5111)
- Introduce memory watchdog; LOTUS_MAX_HEAP (https://github.com/filecoin-project/lotus/pull/5101)
- Miner control address config for (pre)commits (https://github.com/filecoin-project/lotus/pull/5103)
- Delete repeated call func (https://github.com/filecoin-project/lotus/pull/5099)
- lotus-shed ledger show command (https://github.com/filecoin-project/lotus/pull/5098)
- Log a message when there aren't enough peers for sync (https://github.com/filecoin-project/lotus/pull/5105)
- Miner code cleanup (https://github.com/filecoin-project/lotus/pull/5107)

# 1.2.2 / 2020-12-03

This is an optional Lotus release that introduces various improvements to the mining logic and deal-making workflow, as well as several new UX features.

## Changes

- Set lower feecap on PoSt messages with low balance (https://github.com/filecoin-project/lotus/pull/4217)
- Add options to set BlockProfileRate and MutexProfileFraction (https://github.com/filecoin-project/lotus/pull/4140)
- Shed/post find (https://github.com/filecoin-project/lotus/pull/4355)
- tvx extract: make it work with secp messages.(https://github.com/filecoin-project/lotus/pull/4583)
- update go from 1.14 to 1.15 (https://github.com/filecoin-project/lotus/pull/4909)
- print multiple blocks from miner cid (https://github.com/filecoin-project/lotus/pull/4767)
- Connection Gater support (https://github.com/filecoin-project/lotus/pull/4849)
- just return storedask.NewStoredAsk to reduce unuseful code (https://github.com/filecoin-project/lotus/pull/4902)
- add go main version (https://github.com/filecoin-project/lotus/pull/4910)
- Use version0 when pre-sealing (https://github.com/filecoin-project/lotus/pull/4911)
- optimize code UpgradeTapeHeight and go fmt (https://github.com/filecoin-project/lotus/pull/4913)
- CLI to get network version (https://github.com/filecoin-project/lotus/pull/4914)
- Improve error for ActorsVersionPredicate (https://github.com/filecoin-project/lotus/pull/4915)
- upgrade to go-fil-markets 1.0.5 (https://github.com/filecoin-project/lotus/pull/4916)
- bug:replace with func recordFailure (https://github.com/filecoin-project/lotus/pull/4919)
- Remove unused key (https://github.com/filecoin-project/lotus/pull/4924)
- change typeV7 make len (https://github.com/filecoin-project/lotus/pull/4943)
- emit events for peer disconnections and act upon them in the blocksync tracker (https://github.com/filecoin-project/lotus/pull/4754)
- Fix lotus bench error (https://github.com/filecoin-project/lotus/pull/4305)
- Reduce badger ValueTreshold to 128 (https://github.com/filecoin-project/lotus/pull/4629)
- Downgrade duplicate nonce logs to debug (https://github.com/filecoin-project/lotus/pull/4933)
- readme update golang version from 1.14.7 to 1.15.5 (https://github.com/filecoin-project/lotus/pull/4974)
- add data transfer logging (https://github.com/filecoin-project/lotus/pull/4975)
- Remove all temp file generation for deals (https://github.com/filecoin-project/lotus/pull/4929)
- fix get sector bug (https://github.com/filecoin-project/lotus/pull/4976)
- fix nil pointer in StateSectorPreCommitInfo (https://github.com/filecoin-project/lotus/pull/4082)
- Add logging on data-transfer to miner (https://github.com/filecoin-project/lotus/pull/4980)
- bugfix: fixup devnet script (https://github.com/filecoin-project/lotus/pull/4956)
- modify for unsafe (https://github.com/filecoin-project/lotus/pull/4024)
- move testground/lotus-soup testplan from oni to lotus (https://github.com/filecoin-project/lotus/pull/4727)
- Setup remainder msig signers when parsing genesis template (https://github.com/filecoin-project/lotus/pull/4904)
- Update JSON RPC server to enforce a maximum request size (https://github.com/filecoin-project/lotus/pull/4923)
- New SR-specific lotus-shed cmd (https://github.com/filecoin-project/lotus/pull/4971)
- update index to sectorNumber (https://github.com/filecoin-project/lotus/pull/4987)
- storagefsm: Fix expired ticket retry loop (https://github.com/filecoin-project/lotus/pull/4876)
- add .sec scale to measurements; humanize for metric tags (https://github.com/filecoin-project/lotus/pull/4989)
- Support seal proof type switching (https://github.com/filecoin-project/lotus/pull/4873)
- fix log format (https://github.com/filecoin-project/lotus/pull/4984)
- Format workerID as string (https://github.com/filecoin-project/lotus/pull/4973)
- miner: Winning PoSt Warmup (https://github.com/filecoin-project/lotus/pull/4824)
- Default StartDealParams's fast retrieval field to true over JSON (https://github.com/filecoin-project/lotus/pull/4998)
- Fix actor not found in chain inspect-usage (https://github.com/filecoin-project/lotus/pull/5010)
- storagefsm: Improve new deal sector logic (https://github.com/filecoin-project/lotus/pull/5007)
- Configure simultaneous requests (https://github.com/filecoin-project/lotus/pull/4996)
- miner: log winningPoSt duration separately (https://github.com/filecoin-project/lotus/pull/5005)
- fix wallet dead lock (https://github.com/filecoin-project/lotus/pull/5002)
- Update go-jsonrpc to v0.1.2 (https://github.com/filecoin-project/lotus/pull/5015)
- markets - separate watching for pre-commit from prove-commit (https://github.com/filecoin-project/lotus/pull/4945)
- storagefsm: Add missing planners (https://github.com/filecoin-project/lotus/pull/5016)
- fix wallet delete address where address is default (https://github.com/filecoin-project/lotus/pull/5019)
- worker: More robust remote checks (https://github.com/filecoin-project/lotus/pull/5008)
- Add new booststrappers (https://github.com/filecoin-project/lotus/pull/4007)
- add a tooling to make filecoin accounting a little easier (https://github.com/filecoin-project/lotus/pull/5025)
- fix: start a new line in print miner-info to avoid ambiguous display (https://github.com/filecoin-project/lotus/pull/5029)
- Print gas limit sum in mpool stat (https://github.com/filecoin-project/lotus/pull/5035)
- Fix chainstore tipset leak (https://github.com/filecoin-project/lotus/pull/5037)
- shed rpc: Allow calling with args (https://github.com/filecoin-project/lotus/pull/5036)
- Make --gas-limit optional in mpool replace cli (https://github.com/filecoin-project/lotus/pull/5059)
- client list-asks --by-ping (https://github.com/filecoin-project/lotus/pull/5060)
- Ledger signature verification (https://github.com/filecoin-project/lotus/pull/5068)
- Fix helptext for verified-deal default in client deal (https://github.com/filecoin-project/lotus/pull/5074)
- worker: Support setting task types at runtime (https://github.com/filecoin-project/lotus/pull/5023)
- Enable Callers tracing when GasTracing is enabled (https://github.com/filecoin-project/lotus/pull/5080)
- Cancel transfer cancels storage deal (https://github.com/filecoin-project/lotus/pull/5032)
- Sector check command (https://github.com/filecoin-project/lotus/pull/5041)
- add commp-to-cid base64 decode (https://github.com/filecoin-project/lotus/pull/5079)
- miner info cli improvements (https://github.com/filecoin-project/lotus/pull/5083)
- miner: Add slow mode to proving check (https://github.com/filecoin-project/lotus/pull/5086)
- Error out deals that are not activated by proposed deal start epoch (https://github.com/filecoin-project/lotus/pull/5061)

# 1.2.1 / 2020-11-20

This is a very small release of Lotus that fixes an issue users are experiencing when importing snapshots. There is no need to upgrade unless you experience an issue with creating a new datastore directory in the Lotus repo.

## Changes

- fix blockstore directory not created automatically (https://github.com/filecoin-project/lotus/pull/4922)
- WindowPoStScheduler.checkSectors() delete useless judgment (https://github.com/filecoin-project/lotus/pull/4918)


# 1.2.0 / 2020-11-18

This is a mandatory release of Lotus that introduces the second post-liftoff upgrade to the Filecoin network. The network upgrade occurs at height 265200, before which time all nodes must have updated to this release (or later). This release also bumps the required version of Go to 1.15.

The changes that break consensus are:

- Upgrading to sepcs-actors 2.3.2 (https://github.com/filecoin-project/specs-actors/releases/tag/v2.3.2)
- Introducing proofs v5.4.0 (https://github.com/filecoin-project/rust-fil-proofs/releases/tag/storage-proofs-v5.4.0), and switching between the proof types (https://github.com/filecoin-project/lotus/pull/4873)
- Don't use terminated sectors for winning PoSt (https://github.com/filecoin-project/lotus/pull/4770)
- Various small VM-level edge-case handling (https://github.com/filecoin-project/lotus/pull/4783)
- Correction of the VM circulating supply calculation (https://github.com/filecoin-project/lotus/pull/4862)
- Retuning gas costs (https://github.com/filecoin-project/lotus/pull/4830)
- Avoid sending messages to the zero BLS address (https://github.com/filecoin-project/lotus/pull/4888)

## Other Changes

- delayed pubsub subscribe for messages topic (https://github.com/filecoin-project/lotus/pull/3646)
- add chain base64 decode params (https://github.com/filecoin-project/lotus/pull/4748)
- chore(dep): update bitswap to fix an initialization race that could panic (https://github.com/filecoin-project/lotus/pull/4855)
- Chore/blockstore nits (https://github.com/filecoin-project/lotus/pull/4813)
- Print Consensus Faults in miner info (https://github.com/filecoin-project/lotus/pull/4853)
- Truncate genesis file before generating (https://github.com/filecoin-project/lotus/pull/4851)
- miner: Winning PoSt Warmup (https://github.com/filecoin-project/lotus/pull/4824)
- Fix init actor address map diffing (https://github.com/filecoin-project/lotus/pull/4875)
- Bump API versions to 1.0.0 (https://github.com/filecoin-project/lotus/pull/4884)
- Fix cid recording issue (https://github.com/filecoin-project/lotus/pull/4874)
- Speed up worker key retrieval (https://github.com/filecoin-project/lotus/pull/4885)
- Add error codes to worker return (https://github.com/filecoin-project/lotus/pull/4890)
- Update go to 1.15.5 (https://github.com/filecoin-project/lotus/pull/4896)
- Fix MaxSealingSectrosForDeals getting reset to 0 (https://github.com/filecoin-project/lotus/pull/4879)
- add sanity check for maximum block size (https://github.com/filecoin-project/lotus/pull/3171)
- Check (pre)commit receipt before other checks in failed states (https://github.com/filecoin-project/lotus/pull/4712)
- fix badger double open on daemon --import-snapshot; chainstore lifecycle (https://github.com/filecoin-project/lotus/pull/4872)
- Update to ipfs-blockstore 1.0.3 (https://github.com/filecoin-project/lotus/pull/4897)
- break loop when found warm up sector (https://github.com/filecoin-project/lotus/pull/4869)
- Tweak handling of bad beneficaries in DeleteActor (https://github.com/filecoin-project/lotus/pull/4903)
- cap maximum number of messages per block in selection (https://github.com/filecoin-project/lotus/pull/4905)
- Set Calico epoch (https://github.com/filecoin-project/lotus/pull/4889)

# 1.1.3 / 2020-11-13

This is an optional release of Lotus that upgrades Lotus dependencies, and includes many performance enhancements, bugfixes, and UX improvements.

## Highlights

- Refactored much of the miner code (https://github.com/filecoin-project/lotus/pull/3618), improving its recovery from restarts and overall sector success rate
- Updated [proofs](https://github.com/filecoin-project/rust-fil-proofs) to v5.3.0, which brings significant performance improvements
- Updated [markets](https://github.com/filecoin-project/go-fil-markets/releases/tag/v1.0.4) to v1.0.4, which reduces failures due to reorgs (https://github.com/filecoin-project/lotus/pull/4730) and uses the newly refactored fund manager (https://github.com/filecoin-project/lotus/pull/4736)

## Changes

#### Core Lotus

- polish: add Equals method to MinerInfo shim (https://github.com/filecoin-project/lotus/pull/4604)
- Fix messagepool accounting (https://github.com/filecoin-project/lotus/pull/4668)
- Prep for gas balancing (https://github.com/filecoin-project/lotus/pull/4651)
- Reduce badger ValueThreshold to 128 (https://github.com/filecoin-project/lotus/pull/4629)
- Config for default max gas fee (https://github.com/filecoin-project/lotus/pull/4652)
- bootstrap: don't return early when one drand resolution fails (https://github.com/filecoin-project/lotus/pull/4626)
- polish: add ClaimsChanged and DiffClaims method to power shim (https://github.com/filecoin-project/lotus/pull/4628)
- Simplify chain event Called API (https://github.com/filecoin-project/lotus/pull/4664)
- Cache deal states for most recent old/new tipset (https://github.com/filecoin-project/lotus/pull/4623)
- Add miner available balance and power info to state miner info (https://github.com/filecoin-project/lotus/pull/4618)
- Call GetHeaviestTipSet() only once when syncing (https://github.com/filecoin-project/lotus/pull/4696)
- modify runtime gasUsed printf (https://github.com/filecoin-project/lotus/pull/4704)
- Rename builtin actor generators (https://github.com/filecoin-project/lotus/pull/4697)
- Move gas multiplier as property of pricelist (https://github.com/filecoin-project/lotus/pull/4728)
- polish: add msig pendingtxn diffing and comp (https://github.com/filecoin-project/lotus/pull/4719)
- Optional chain Bitswap (https://github.com/filecoin-project/lotus/pull/4717)
- rewrite sync manager (https://github.com/filecoin-project/lotus/pull/4599)
- async connect to bootstrappers (https://github.com/filecoin-project/lotus/pull/4785)
- head change coalescer (https://github.com/filecoin-project/lotus/pull/4688)
- move to native badger blockstore; leverage zero-copy View() to deserialize in-place (https://github.com/filecoin-project/lotus/pull/4681)
- badger blockstore: minor improvements (https://github.com/filecoin-project/lotus/pull/4811)
- Do not fail wallet delete because of pre-existing trashed key (https://github.com/filecoin-project/lotus/pull/4589)
- Correctly delete the default wallet address (https://github.com/filecoin-project/lotus/pull/4705)
- Reduce badger ValueTreshold to 128 (https://github.com/filecoin-project/lotus/pull/4629)
- predicates: Fast StateGetActor wrapper (https://github.com/filecoin-project/lotus/pull/4835)

#### Mining

- worker key should change when set sender found key not equal with the value on chain (https://github.com/filecoin-project/lotus/pull/4595)
- extern/sector-storage: fix GPU usage overwrite bug (https://github.com/filecoin-project/lotus/pull/4627)
- sectorstorage: Fix manager restart edge-case (https://github.com/filecoin-project/lotus/pull/4645)
- storagefsm: Fix GetTicket loop when the sector is already precommitted (https://github.com/filecoin-project/lotus/pull/4643)
- Debug flag to force running sealing scheduler (https://github.com/filecoin-project/lotus/pull/4662)
- Fix worker reenabling, handle multiple restarts in worker (https://github.com/filecoin-project/lotus/pull/4666)
- keep retrying the proof until we run out of sectors to skip (https://github.com/filecoin-project/lotus/pull/4633)
- worker: Commands to pause/resume task processing (https://github.com/filecoin-project/lotus/pull/4615)
- struct name incorrect (https://github.com/filecoin-project/lotus/pull/4699)
- optimize code replace strings with constants (https://github.com/filecoin-project/lotus/pull/4769)
- optimize pledge sector (https://github.com/filecoin-project/lotus/pull/4765)
- Track sealing processes across lotus-miner restarts (https://github.com/filecoin-project/lotus/pull/3618)
- Fix scheduler lockups after storage is freed (https://github.com/filecoin-project/lotus/pull/4778)
- storage: Track worker hostnames with work (https://github.com/filecoin-project/lotus/pull/4779)
- Expand sched-diag; Command to abort sealing calls (https://github.com/filecoin-project/lotus/pull/4804)
- miner: Winning PoSt Warmup (https://github.com/filecoin-project/lotus/pull/4824)
- docsgen: Support miner/worker (https://github.com/filecoin-project/lotus/pull/4817)
- miner: Basic storage cleanup command (https://github.com/filecoin-project/lotus/pull/4834)

#### Markets and Data Transfer

- Flesh out data transfer features (https://github.com/filecoin-project/lotus/pull/4572)
- Fix memory leaks in data transfer (https://github.com/filecoin-project/lotus/pull/4619)
- Handle deal id changes in OnDealSectorCommitted (https://github.com/filecoin-project/lotus/pull/4730)
- Refactor FundManager (https://github.com/filecoin-project/lotus/pull/4736)
- refactor: integrate new FundManager (https://github.com/filecoin-project/lotus/pull/4787)
- Fix race in paych manager when req context is cancelled (https://github.com/filecoin-project/lotus/pull/4803)
- fix race in paych manager add funds (https://github.com/filecoin-project/lotus/pull/4597)
- Fix panic in FundManager (https://github.com/filecoin-project/lotus/pull/4808)
- Fix: dont crash on startup if funds migration fails (https://github.com/filecoin-project/lotus/pull/4827)

#### UX

- Make EarlyExpiration in sectors list less scary (https://github.com/filecoin-project/lotus/pull/4600)
- Add commands to change the worker key (https://github.com/filecoin-project/lotus/pull/4513)
- Expose ClientDealSize via CLI (https://github.com/filecoin-project/lotus/pull/4569)
- client deal: Cache CommD when creating multiple deals (https://github.com/filecoin-project/lotus/pull/4535)
- miner sectors list: flags for events/seal time (https://github.com/filecoin-project/lotus/pull/4649)
- make IPFS online mode configurable (https://github.com/filecoin-project/lotus/pull/4650)
- Add sync status to miner info command (https://github.com/filecoin-project/lotus/pull/4669)
- Add a StateDecodeParams method (https://github.com/filecoin-project/lotus/pull/4105)
- sched: Interactive RPC Shell (https://github.com/filecoin-project/lotus/pull/4692)
- Add api for getting status given a code (https://github.com/filecoin-project/lotus/pull/4210)
- Update lotus-stats with a richer cli (https://github.com/filecoin-project/lotus/pull/4718)
- Use TSK passed to GasEstimateGasLimit (https://github.com/filecoin-project/lotus/pull/4739)
- match data type for reward state api (https://github.com/filecoin-project/lotus/pull/4745)
- Add `termination-estimate` to get an estimation for how much a termination penalty will be (https://github.com/filecoin-project/lotus/pull/4617)
- Restrict `ParseFIL` input length (https://github.com/filecoin-project/lotus/pull/4780)
- cmd sectors commitIDs len debug (https://github.com/filecoin-project/lotus/pull/4786)
- Add client deal-stats CLI (https://github.com/filecoin-project/lotus/pull/4788)
- Modify printf format (https://github.com/filecoin-project/lotus/pull/4795)
- Updated msig inspect (https://github.com/filecoin-project/lotus/pull/4533)
- Delete the duplicate output (https://github.com/filecoin-project/lotus/pull/4819)
- miner: Storage list sectors command (https://github.com/filecoin-project/lotus/pull/4831)
- drop a few logs down to debug (https://github.com/filecoin-project/lotus/pull/4832)

#### Testing and Tooling

- refactor: share code between CLI tests (https://github.com/filecoin-project/lotus/pull/4598)
- Fix flaky TestCLIDealFlow (https://github.com/filecoin-project/lotus/pull/4608)
- Fix flaky testMiningReal (https://github.com/filecoin-project/lotus/pull/4609)
- Add election run-dummy command (https://github.com/filecoin-project/lotus/pull/4498)
- Fix .gitmodules (https://github.com/filecoin-project/lotus/pull/4713)
- fix metrics wiring.(https://github.com/filecoin-project/lotus/pull/4691)
- shed: Util for creating ID CIDs (https://github.com/filecoin-project/lotus/pull/4726)
- Run kumquat upgrade on devnets (https://github.com/filecoin-project/lotus/pull/4734)
- Make pond work again (https://github.com/filecoin-project/lotus/pull/4775)
- lotus-stats: fix influx flags (https://github.com/filecoin-project/lotus/pull/4810)
- 2k sync BootstrapPeerThreshold (https://github.com/filecoin-project/lotus/pull/4797)
- test for FundManager panic to ensure it is fixed (https://github.com/filecoin-project/lotus/pull/4825)
- Stop mining at the end of tests (https://github.com/filecoin-project/lotus/pull/4826)
- Make some logs quieter (https://github.com/filecoin-project/lotus/pull/4709)

#### Dependencies

- update filecoin-ffi in go mod (https://github.com/filecoin-project/lotus/pull/4584)
- Update FFI (https://github.com/filecoin-project/lotus/pull/4613)
- feat: integrate new optional blst backend and verification optimizations from proofs (https://github.com/filecoin-project/lotus/pull/4630)
- Use https for blst submodule (https://github.com/filecoin-project/lotus/pull/4710)
- Update go-bitfield (https://github.com/filecoin-project/lotus/pull/4756)
- Update Yamux (https://github.com/filecoin-project/lotus/pull/4758)
- Update to latest go-bitfield (https://github.com/filecoin-project/lotus/pull/4793)
- Update to latest go-address (https://github.com/filecoin-project/lotus/pull/4798)
- update libp2p for stream interface changes (https://github.com/filecoin-project/lotus/pull/4814)

# 1.1.2 / 2020-10-24

This is a patch release of Lotus that builds on the fixes involving worker keys that was introduced in v1.1.1. Miners and node operators should update to this release as soon as possible in order to ensure their blocks are propagated and validated.

## Changes

- Handle worker key changes correctly in runtime (https://github.com/filecoin-project/lotus/pull/4579)

# 1.1.1 / 2020-10-24

This is a patch release of Lotus that addresses some issues caused by when miners change their worker keys. Miners and node operators should update to this release as soon as possible, especially any miner who has changed their worker key recently.

## Changes

- Miner finder for interactive client deal CLI (https://github.com/filecoin-project/lotus/pull/4504)
- Disable blockstore bloom filter (https://github.com/filecoin-project/lotus/pull/4512)
- Add api for getting status given a code (https://github.com/filecoin-project/lotus/pull/4210)
- add batch api for push messages (https://github.com/filecoin-project/lotus/pull/4236)
- add measure datastore wrapper around bench chain datastore (https://github.com/filecoin-project/lotus/pull/4302)
- Look at block base fee for PCR (https://github.com/filecoin-project/lotus/pull/4313)
- Add a shed util to determine % of power that has won a block (https://github.com/filecoin-project/lotus/pull/4318)
- Shed/borked cmd (https://github.com/filecoin-project/lotus/pull/4339)
- optimize mining code (https://github.com/filecoin-project/lotus/pull/4379)
- heaviestTipSet reurning nil is a ok (https://github.com/filecoin-project/lotus/pull/4523)
- Remove most v0 actor imports (https://github.com/filecoin-project/lotus/pull/4383)
- Small chain export optimization (https://github.com/filecoin-project/lotus/pull/4536)
- Add block list to pcr (https://github.com/filecoin-project/lotus/pull/4314)
- Fix circ supply default in conformance (https://github.com/filecoin-project/lotus/pull/4449)
- miner: fix init --create-worker-key (https://github.com/filecoin-project/lotus/pull/4475)
- make push and addLocal atomic (https://github.com/filecoin-project/lotus/pull/4500)
- add some methods that oni needs (https://github.com/filecoin-project/lotus/pull/4501)
- MinerGetBaseInfo: if miner is not found in lookback, check current (https://github.com/filecoin-project/lotus/pull/4508)
- Delete wallet from local wallet cache (https://github.com/filecoin-project/lotus/pull/4526)
- Fix lotus-shed ledger list (https://github.com/filecoin-project/lotus/pull/4521)
- Manage sectors by size instead of proof type (https://github.com/filecoin-project/lotus/pull/4511)
- Feat/api request metrics wrapper (https://github.com/filecoin-project/lotus/pull/4516)
- Fix chain sync stopping to sync (https://github.com/filecoin-project/lotus/pull/4541)
- Use the correct lookback for the worker key when creating blocks (https://github.com/filecoin-project/lotus/pull/4539)
- Cleanup test initialization and always validate VRFs in tests (https://github.com/filecoin-project/lotus/pull/4538)
- Add a market WithdrawBalance CLI (https://github.com/filecoin-project/lotus/pull/4524)
- wallet list: Add market balance and ID address flags (https://github.com/filecoin-project/lotus/pull/4555)
- tvx simulate command; tvx extract --ignore-sanity-checks (https://github.com/filecoin-project/lotus/pull/4554)
- lotus-lite: CLI tests for `lotus client` commands (https://github.com/filecoin-project/lotus/pull/4497)
- lite-mode - market storage and retrieval clients (https://github.com/filecoin-project/lotus/pull/4263)
- Chore: update drand to v1.2.0 (https://github.com/filecoin-project/lotus/pull/4420)
- Fix random test failures (https://github.com/filecoin-project/lotus/pull/4559)
- Fix flaky TestTimedBSSimple (https://github.com/filecoin-project/lotus/pull/4561)
- Make wallet market withdraw usable with miner addresses (https://github.com/filecoin-project/lotus/pull/4556)
- Fix flaky TestChainExportImportFull (https://github.com/filecoin-project/lotus/pull/4564)
- Use older randomness for the PoSt commit on specs-actors version 2 (https://github.com/filecoin-project/lotus/pull/4563)
- shed: Commad to decode messages (https://github.com/filecoin-project/lotus/pull/4565)
- Fetch worker key from correct block on sync (https://github.com/filecoin-project/lotus/pull/4573)

# 1.1.0 / 2020-10-20

This is a mandatory release that introduces the first post-liftoff upgrade to the Filecoin network. The changes that break consensus are an upgrade to specs-actors v2.2.0 at epoch 170000.

## Changes

- Introduce Network version 6 (https://github.com/filecoin-project/lotus/pull/4506)
- Update markets v1.0.0 (https://github.com/filecoin-project/lotus/pull/4505)
- Add some extra logging to try and debug sync issues (https://github.com/filecoin-project/lotus/pull/4486)
- Circle: Run tests for some subsystems separately (https://github.com/filecoin-project/lotus/pull/4496)
- Add a terminate sectors command to lotus-shed (https://github.com/filecoin-project/lotus/pull/4507)
- Add a comment to BlockMessages to address #4446 (https://github.com/filecoin-project/lotus/pull/4491)

# 1.0.0 / 2020-10-19

It's 1.0.0! This is an optional release of Lotus that introduces some UX improvements to the 0.10 series.

This very small release is largely cosmetic, and intended to flag the code that the Filecoin mainnet was launched with.

## API changes

- `StateMsgGasCost` has been removed. The equivalent information can be gained by calling `StateReplay`.
- A `GasCost` field has been added to the `InvocResult` type, meaning detailed gas costs will be returned when calling `StateReplay`, `StateCompute`, and `StateCall`.
- The behaviour of `StateReplay` in response to an empty tipset key has been changed. Instead of simply using the heaviest tipset (which is almost guaranteed to be an unsuccessful replay), we search now search the chain for the tipset that included the message, and replay the message in that tipset (we fail if no such tipset is found).

## Changes

- Increase code coverage! (https://github.com/filecoin-project/lotus/pull/4410)
- Mpool: Don't block node startup loading messages (https://github.com/filecoin-project/lotus/pull/4411)
- Improve the UX of multisig approves (https://github.com/filecoin-project/lotus/pull/4398)
- Use build.BlockDelaySecs for deal start buffer (https://github.com/filecoin-project/lotus/pull/4415)
- Conformance: support multiple protocol versions (https://github.com/filecoin-project/lotus/pull/4393)
- Ensure msig inspect cli works with lotus-lite (https://github.com/filecoin-project/lotus/pull/4421)
- Add command to (slowly) prune lotus chain datastore (https://github.com/filecoin-project/lotus/pull/3876)
- Add WalletVerify to lotus-gateway (https://github.com/filecoin-project/lotus/pull/4373)
- Improve StateMsg APIs (https://github.com/filecoin-project/lotus/pull/4429)
- Add endpoints needed by spacegap (https://github.com/filecoin-project/lotus/pull/4426)
- Make audit balances capable of printing robust addresses (https://github.com/filecoin-project/lotus/pull/4423)
- Custom filters for retrieval deals (https://github.com/filecoin-project/lotus/pull/4424)
- Fix message list api (https://github.com/filecoin-project/lotus/pull/4422)
- Replace bootstrap peers (https://github.com/filecoin-project/lotus/pull/4447)
- Don't overwrite previously-configured maxPieceSize for a persisted ask (https://github.com/filecoin-project/lotus/pull/4480)
- State: optimize state snapshot address cache (https://github.com/filecoin-project/lotus/pull/4481)

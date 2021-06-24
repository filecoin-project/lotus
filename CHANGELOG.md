# Lotus changelog

# 1.10.0 / 2021-06-23

This is a mandatory release of Lotus that introduces Filecoin network v13, codenamed the HyperDrive upgrade. The 
Filecoin mainnet will upgrade, which is epoch 892800, on 2021-06-30T22:00:00Z. The network upgrade introduces the 
following FIPs:

- [FIP-0008](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0008.md): Add miner batched sector pre-commit method
- [FIP-0011](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0011.md): Remove reward auction from reporting consensus faults
- [FIP-0012](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0012.md): DataCap Top up for FIL+ Client Addresses
- [FIP-0013](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0013.md): Add ProveCommitSectorAggregated method to reduce on-chain congestion
- [FIP-0015](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0015.md): Revert FIP-0009(Exempt Window PoSts from BaseFee burn)

Note that this release is built on top of Lotus v1.9.0. Enterprising users can use the `master` branch of Lotus to get the latest functionality, including all changes in this release candidate.

## Proof batching and aggregation

FIPs [0008](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0008.md) and [0013](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0013.md) combine to allow for a significant increase in the rate of onboarding storage on the Filecoin network. This aims to lead to more useful data being stored on the network, reduced network congestion, and lower network base fee.

**Check out the documentation [here](https://docs.filecoin.io/mine/lotus/miner-configuration/#precommitsectorsbatch) for details on the new Lotus miner sealing config options, [here](https://docs.filecoin.io/mine/lotus/miner-configuration/#fees-section) for fee config options, and explanations of the new features.**

Note: 
  - We recommend to keep `PreCommitSectorsBatch` as 1.
  - We recommend miners to set `PreCommitBatchWait` lower than 30 hours.
  - We recommend miners to set a longer `CommitBatchSlack` and  `PreCommitBatchSlack` to prevent message failures 
    due to expirations.

### Projected state tree growth

In order to validate the Hyperdrive changes, we wrote a simulation to seal as many sectors as quickly as possible, assuming the same number and mix of 32GiB and 64GiB miners as the current network.

Given these assumptions:

- We'd expect a network storage growth rate of around 530PiB per day. 😳 🎉 🥳 😅
- We'd expect network bandwidth dedicated to `SubmitWindowedPoSt` to grow by about 0.02% per day.
- We'd expect the [state-tree](https://spec.filecoin.io/#section-systems.filecoin_vm.state_tree) (and therefore [snapshot](https://docs.filecoin.io/get-started/lotus/chain/#lightweight-snapshot)) size to grow by 1.16GiB per day.
   - Nearly all of the state-tree growth is expected to come from new sector metadata.
- We'd expect the daily lotus datastore growth rate to increase by about 10-15% (from current ~21GiB/day).
   - Most "growth" of the lotus datastore is due to "churn", historical data that's no longer referenced by the latest state-tree.

### Future improvements

Various Lotus improvements are planned moving forward to mitigate the effects of the growing state tree size. The primary improvement is the [Lotus splitstore](https://github.com/filecoin-project/lotus/discussions/5788), which will soon be enabled by default. The feature allows for [online garbage collection](https://github.com/filecoin-project/lotus/issues/6577) for nodes that do not seek to maintain full chain and state history, thus eliminating the need for users to delete their datastores and sync from snapshots.

Other improvements including better compressed snapshots, faster pre-migrations, and improved chain exports are in the roadmap.

## WindowPost base fee burn

Included in the HyperDrive upgrade is [FIP-0015](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0015.md) which eliminates the special-case gas treatment of `SubmitWindowedPoSt` messages that was introduced in [FIP-0009](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0009.md). Although `SubmitWindowedPoSt` messages will be relatively cheap, thanks to the introduction of optimistic acceptance of these proofs in [FIP-0010](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0010.md), storage providers should pay attention to their `MaxWindowPoStGasFee` config option: too low and PoSts may not land on chain; too high and they may cost an exorbitant amount!

## Changelog

### New Features

- Implement FIP-0015 ([filecoin-project/lotus#6361](https://github.com/filecoin-project/lotus/pull/6361))
- Integrate FIP0013 and FIP0008 ([filecoin-project/lotus#6235](https://github.com/filecoin-project/lotus/pull/6235))
  - [Configuration docs and cli examples](https://docs.filecoin.io/mine/lotus/miner-configuration/#precommitsectorsbatch)
  - [cli docs](https://github.com/filecoin-project/lotus/blob/master/documentation/en/cli-lotus-miner.md#lotus-miner-sectors-batching)
  - Introduce gas prices for aggregate verifications ([filecoin-project/lotus#6347](https://github.com/filecoin-project/lotus/pull/6347))
- Introduce v5 actors ([filecoin-project/lotus#6195](https://github.com/filecoin-project/lotus/pull/6195))
- Robustify commit batcher ([filecoin-project/lotus#6367](https://github.com/filecoin-project/lotus/pull/6367))
- Always flush when timer goes off ([filecoin-project/lotus#6563](https://github.com/filecoin-project/lotus/pull/6563))
- Update default fees for aggregates ([filecoin-project/lotus#6548](https://github.com/filecoin-project/lotus/pull/6548))
- sealing: Early finalization option ([filecoin-project/lotus#6452](https://github.com/filecoin-project/lotus/pull/6452))
  - `./lotus-miner/config.toml/[Sealing.FinalizeEarly]`: default to false. Enable if you want to FinalizeSector before commiting 
- Add filplus utils to CLI ([filecoin-project/lotus#6351](https://github.com/filecoin-project/lotus/pull/6351))
  - cli doc can be found [here](https://github.com/filecoin-project/lotus/blob/master/documentation/en/cli-lotus.md#lotus-filplus)
- Add miner-side MaxDealStartDelay config ([filecoin-project/lotus#6576](https://github.com/filecoin-project/lotus/pull/6576))

  
### Bug Fixes
- chainstore: Don't take heaviestLk with backlogged reorgCh ([filecoin-project/lotus#6526](https://github.com/filecoin-project/lotus/pull/6526))
- Backport #6041 - storagefsm: Fix batch deal packing behavior  ([filecoin-project/lotus#6519](https://github.com/filecoin-project/lotus/pull/6519))
- backport: pick the correct partitions-per-post limit ([filecoin-project/lotus#6503](https://github.com/filecoin-project/lotus/pull/6503)) 
- failed sectors should be added into res correctly ([filecoin-project/lotus#6472](https://github.com/filecoin-project/lotus/pull/6472))
- sealing: Fix restartSectors race ([filecoin-project/lotus#6491](https://github.com/filecoin-project/lotus/pull/6491))
- Fund miners with the aggregate fee when ProveCommitting ([filecoin-project/lotus#6428](https://github.com/filecoin-project/lotus/pull/6428)) 
- Commit and Precommit batcher cannot share a getSectorDeadline method ([filecoin-project/lotus#6416](https://github.com/filecoin-project/lotus/pull/6416))
- Fix supported proof type manipulations for v5 actors ([filecoin-project/lotus#6366](https://github.com/filecoin-project/lotus/pull/6366))
- events: Fix handling of multiple matched events per epoch ([filecoin-project/lotus#6362](https://github.com/filecoin-project/lotus/pull/6362))
- Fix randomness fetching around null blocks ([filecoin-project/lotus#6240](https://github.com/filecoin-project/lotus/pull/6240))
  
### Improvements 
- Appimage v1.10.0 rc3 ([filecoin-project/lotus#6492](https://github.com/filecoin-project/lotus/pull/6492))
- Expand on Drand change testing ([filecoin-project/lotus#6500](https://github.com/filecoin-project/lotus/pull/6500))
- Backport Fix logging around mineOne ([filecoin-project/lotus#6499](https://github.com/filecoin-project/lotus/pull/6499))
- mpool: Add more metrics ([filecoin-project/lotus#6453](https://github.com/filecoin-project/lotus/pull/6453)) 
- Merge backported PRs into v1.10 release branch ([filecoin-project/lotus#6436](https://github.com/filecoin-project/lotus/pull/6436))
- Fix tests ([filecoin-project/lotus#6371](https://github.com/filecoin-project/lotus/pull/6371))
- Extend the default deal start epoch delay ([filecoin-project/lotus#6350](https://github.com/filecoin-project/lotus/pull/6350))
- sealing: Wire up context to batchers ([filecoin-project/lotus#6497](https://github.com/filecoin-project/lotus/pull/6497))
- Improve address resolution for messages ([filecoin-project/lotus#6364](https://github.com/filecoin-project/lotus/pull/6364))
  
### Dependency Updates
- Proofs v8.0.2 ([filecoin-project/lotus#6524](https://github.com/filecoin-project/lotus/pull/6524))
- Update to fixed Bellperson ([filecoin-project/lotus#6480](https://github.com/filecoin-project/lotus/pull/6480))
- Update to go-praamfetch with fslocks ([filecoin-project/lotus#6473](https://github.com/filecoin-project/lotus/pull/6473))
- Update ffi with fixed multicore sdr support ([filecoin-project/lotus#6471](https://github.com/filecoin-project/lotus/pull/6471))
- github.com/filecoin-project/go-paramfetch (v0.0.2-0.20200701152213-3e0f0afdc261 -> v0.0.2-0.20210614165157-25a6c7769498)
- github.com/filecoin-project/specs-actors/v5 (v5.0.0-20210512015452-4fe3889fff57 -> v5.0.0)
- github.com/filecoin-project/go-hamt-ipld/v3 (v3.0.1 -> v3.1.0)
- github.com/ipfs/go-log/v2 (v2.1.2-0.20200626104915-0016c0b4b3e4 -> v2.1.3)
- github.com/filecoin-project/go-amt-ipld/v3 (v3.0.0 -> v3.1.0)
 
### Network Version v13 HyperDrive Upgrade
- Set HyperDrive upgrade epoch ([filecoin-project/lotus#6565](https://github.com/filecoin-project/lotus/pull/6565))
- version bump to lotus v1.10.0-rc6 ([filecoin-project/lotus#6529](https://github.com/filecoin-project/lotus/pull/6529))
- Upgrade epochs for calibration reset ([filecoin-project/lotus#6528](https://github.com/filecoin-project/lotus/pull/6528))
- Lotus version 1.10.0-rc5 ([filecoin-project/lotus#6504](https://github.com/filecoin-project/lotus/pull/6504))
- Merge releases into v1.10 release ([filecoin-project/lotus#6494](https://github.com/filecoin-project/lotus/pull/6494))
- update lotus to v1.10.0-rc3 ([filecoin-project/lotus#6481](https://github.com/filecoin-project/lotus/pull/6481))
- updated configuration comments for docs
- Lotus version 1.10.0-rc2 ([filecoin-project/lotus#6443](https://github.com/filecoin-project/lotus/pull/6443))
- Set ntwk v13 HyperDrive Calibration upgrade epoch ([filecoin-project/lotus#6442](https://github.com/filecoin-project/lotus/pull/6442))


## Contributors

💙Thank you to all the contributors!

| Contributor        | Commits | Lines ±     | Files Changed |
|--------------------|---------|-------------|---------------|
| @magik6k    | 81      | +9606/-1536 | 361           |
| @arajasek  | 41      | +6543/-679  | 189           |
| @ZenGround0         | 11      | +4074/-727  | 110           |
| @anorth                | 10      | +2035/-1177 | 55            |
| @iand           | 1       | +779/-12    | 5             |
| @frrist             | 2       | +722/-6     | 6             |
| @Stebalien       | 6       | +368/-24    | 15            |
| @jennijuju      | 11      | +204/-111   | 19            |
| @vyzo               | 6       | +155/-66    | 13            |
| @coryschwartz      | 10      | +171/-27    | 14            |
| @Kubuxu    | 4       | +177/-13    | 7             |
| @ribasushi    | 4       | +65/-42     | 5             |
| @travisperson      | 2       | +11/-11     | 4             |
| @kirk-baird | 1       | +1/-5       | 1             |
| @wangchao           | 2       | +3/-2       | 2             |


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

# 1.9.0-rc4 / 2021-05-13

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


# 1.9.0-rc2 / 2021-04-30

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
- rust-fil-proofs v5.4.1 -> v7
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
  - Added `PublishMsgPeriod` and `MaxDealsPerPublishMsg` to miner `Dealmaking` [configuration](https://docs.filecoin.io/mine/lotus/miner-configuration/#dealmaking-section). See how they work [here](https://docs.filecoin.io/mine/lotus/miner-configuration/#publishing-several-deals-in-one-message).
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

# 0.10.2 / 2020-10-14

This is an optional release of Lotus that updates markets to 0.9.1, which fixes an issue affecting deals that were mid-transfer when the node was upgraded to 0.9.0. This release also includes some tweaks to default gas values and minor performance improvements.

## Changes

- Use updated stored ask API (https://github.com/filecoin-project/lotus/pull/4384)
- tvx: trace puts to blockstore for inclusion in CAR. (https://github.com/filecoin-project/lotus/pull/4278)
- Add propose remove (https://github.com/filecoin-project/lotus/pull/4311)
- Update to 0.9.1 bugfix release (https://github.com/filecoin-project/lotus/pull/4402)
- Update drand endpoints (https://github.com/filecoin-project/lotus/pull/4125)
- fix: return true when deadlines changed (https://github.com/filecoin-project/lotus/pull/4403)
- sync wait --watch (https://github.com/filecoin-project/lotus/pull/4396)
- reduce garbage in blockstore (https://github.com/filecoin-project/lotus/pull/4406)
- give the TimeCacheBS tests a bit more time (https://github.com/filecoin-project/lotus/pull/4407)
- Improve gas defaults (https://github.com/filecoin-project/lotus/pull/4408)
- Change default gas premium to for 10 block inclusion (https://github.com/filecoin-project/lotus/pull/4222)

# 0.10.1 / 2020-10-14

This is an optional release of Lotus that updates markets to 0.9.0, which adds the ability to restart data transfers. This release also introduces Ledger support, and various UX improvements.

## Changes

- Test the tape upgrade (https://github.com/filecoin-project/lotus/pull/4328)
- Adding in Ledger support (https://github.com/filecoin-project/lotus/pull/4290)
- Improve the UX for lotus-miner sealing workers (https://github.com/filecoin-project/lotus/pull/4329)
- Add a CLI tool for miner's to repay debt (https://github.com/filecoin-project/lotus/pull/4319)
- Rename params_testnet to params_mainnet (https://github.com/filecoin-project/lotus/pull/4336)
- Use seal-duration in calculating the earliest StartEpoch (https://github.com/filecoin-project/lotus/pull/4337)
- Reject deals that are > 7 days in the future in the BasicDealFilter (https://github.com/filecoin-project/lotus/pull/4173)
- Add an API endpoint to calculate the exact circulating supply (https://github.com/filecoin-project/lotus/pull/4148)
- lotus-pcr: ignore all other market messages (https://github.com/filecoin-project/lotus/pull/4341)
- Add message CID to InvocResult (https://github.com/filecoin-project/lotus/pull/4382)
- types: Add CID fields to messages in json marshalers (https://github.com/filecoin-project/lotus/pull/4338)
- fix(sync state): set state height to actual tipset height (https://github.com/filecoin-project/lotus/pull/4347)
- Fix off by one tipset in searchBackForMsg (https://github.com/filecoin-project/lotus/pull/4367)
- fix a panic on startup when we fail to load the tipset (https://github.com/filecoin-project/lotus/pull/4376)
- Avoid having the same message CID show up in execution traces (https://github.com/filecoin-project/lotus/pull/4350)
- feat(markets): update markets 0.9.0 and add data transfer restart (https://github.com/filecoin-project/lotus/pull/4363)

# 0.10.0 / 2020-10-12

This is a consensus-breaking hotfix that addresses an issue in specs-actors v2.0.3 that made it impossible to pledge new 32GiB sectors. The change in Lotus is to update to actors v2.1.0, behind the new network version 5.

## Changes

- make pledge test pass with the race detector (https://github.com/filecoin-project/lotus/pull/4291)
- fix a race in tipset cache usage (https://github.com/filecoin-project/lotus/pull/4282)
- add an api for removing multisig signers (https://github.com/filecoin-project/lotus/pull/4274)
- cli: Don't output errors to stdout (https://github.com/filecoin-project/lotus/pull/4298)
- Fix panic in wallet export when key is not found (https://github.com/filecoin-project/lotus/pull/4299)
- Dump the block validation cache whenever we perform an import (https://github.com/filecoin-project/lotus/pull/4287)
- Fix two races (https://github.com/filecoin-project/lotus/pull/4301)
- sync unmark-bad --all (https://github.com/filecoin-project/lotus/pull/4296)
- decode parameters for multisig transactions in inspect (https://github.com/filecoin-project/lotus/pull/4312)
- Chain is love (https://github.com/filecoin-project/lotus/pull/4321)
- lotus-stats: optmize getting miner power (https://github.com/filecoin-project/lotus/pull/4315)
- implement tape upgrade (https://github.com/filecoin-project/lotus/pull/4322)

# 0.9.1 / 2020-10-10

This release fixes an issue which may cause the actors v2 migration to compute the state incorrectly when more than one migration is running in parallel.

## Changes

- Make concurrent actor migrations safe (https://github.com/filecoin-project/lotus/pull/4293)
- Remote wallet backends (https://github.com/filecoin-project/lotus/pull/3583)
- Track funds in FundMgr correctly in case of AddFunds failing (https://github.com/filecoin-project/lotus/pull/4273)
- Partial lite-node mode (https://github.com/filecoin-project/lotus/pull/4095)
- Fix potential infinite loop in GetBestMiningCandidate (https://github.com/filecoin-project/lotus/pull/3444)
- sync wait: Handle processed message offset (https://github.com/filecoin-project/lotus/pull/4253)
- Add some new endpoints for querying Msig info (https://github.com/filecoin-project/lotus/pull/4250)
- Update markets v0.7.1 (https://github.com/filecoin-project/lotus/pull/4254)
- Optimize SearchForMessage and GetReceipt (https://github.com/filecoin-project/lotus/pull/4246)
- Use FIL instead of attoFIL in CLI more consistently (https://github.com/filecoin-project/lotus/pull/4249)
- fix: clash between daemon --api flag and cli tests (https://github.com/filecoin-project/lotus/pull/4241)
- add more info to chain sync lookback failure (https://github.com/filecoin-project/lotus/pull/4245)
- Add message counts to inspect chain output (https://github.com/filecoin-project/lotus/pull/4230)

# 0.9.0 / 2020-10-07

This consensus-breaking release of Lotus upgrades the actors version to v2.0.0. This requires migrating actor state from v0 to v2. The changes that break consensus are:

- Introducing v2 actors and its migration (https://github.com/filecoin-project/lotus/pull/3936)
- Runtime's Receiver() should only return ID addresses  (https://github.com/filecoin-project/lotus/pull/3589)
- Update miner eligibility checks for v2 actors (https://github.com/filecoin-project/lotus/pull/4188)
- Add funds that have left FilReserve to circ supply (https://github.com/filecoin-project/lotus/pull/4160)
- Set WinningPoStSectorSetLookback to finality post-v2 actors (https://github.com/filecoin-project/lotus/pull/4190)
- fix: error when actor panics directly (https://github.com/filecoin-project/lotus/pull/3697)

## Changes

#### Dependencies

- Update go-bitfield (https://github.com/filecoin-project/lotus/pull/4171)
- update the AMT implementation (https://github.com/filecoin-project/lotus/pull/4194)
- Update to actors v0.2.1 (https://github.com/filecoin-project/lotus/pull/4199)

#### Core Lotus

- Paych: fix voucher amount verification (https://github.com/filecoin-project/lotus/pull/3821)
- Cap market provider messages (https://github.com/filecoin-project/lotus/pull/4141)
- Run fork function after cron for null block safety (https://github.com/filecoin-project/lotus/pull/4114)
- use bitswap sessions when fetching messages, and cancel them (https://github.com/filecoin-project/lotus/pull/4142)
- relax pubsub IPColocationFactorThreshold to 5 (https://github.com/filecoin-project/lotus/pull/4183)
- Support addresses with mainnet prefixes (https://github.com/filecoin-project/lotus/pull/4186)
- fix: make message signer nonce generation transactional (https://github.com/filecoin-project/lotus/pull/4165)
- build: Env var to keep test address output (https://github.com/filecoin-project/lotus/pull/4213)
- make vm.EnableGasTracing public (https://github.com/filecoin-project/lotus/pull/4214)
- introduce separate state-tree versions (https://github.com/filecoin-project/lotus/pull/4197)
- reject explicit "calls" at the upgrade height (https://github.com/filecoin-project/lotus/pull/4231)
- return an illegal actor error when we see an unsupported actor version (https://github.com/filecoin-project/lotus/pull/4232)
- Set head should unmark blocks as valid (https://gist.github.com/travisperson/3c7cddd77a33979a519ccef4e6515f20)

#### Mining

- Increased ExpectedSealDuration and and WaitDealsDelay (https://github.com/filecoin-project/lotus/pull/3743)
- Miner backup/restore commands (https://github.com/filecoin-project/lotus/pull/4133)
- lotus-miner: add more help text to storage / attach (https://github.com/filecoin-project/lotus/pull/3961)
- Reject deals that are > 7 days in the future in the BasicDealFilter (https://github.com/filecoin-project/lotus/pull/4173)
- feat(miner): add miner deadline diffing logic (https://github.com/filecoin-project/lotus/pull/4178)

#### UX

- Improve the UX for replacing messages (https://github.com/filecoin-project/lotus/pull/4134)
- Add verified flag to interactive deal creation (https://github.com/filecoin-project/lotus/pull/4145)
- Add command to (slowly) prune lotus chain datastore (https://github.com/filecoin-project/lotus/pull/3876)
- Some helpers for verifreg work (https://github.com/filecoin-project/lotus/pull/4124)
- Always use default 720h for setask duration and hide the duration param option (https://github.com/filecoin-project/lotus/pull/4077)
- Convert ID addresses to key addresses before checking wallet (https://github.com/filecoin-project/lotus/pull/4122)
- add a command to view block space utilization (https://github.com/filecoin-project/lotus/pull/4176)
- allow usage inspection on a chain segment (https://github.com/filecoin-project/lotus/pull/4177)
- Add mpool stats for base fee (https://github.com/filecoin-project/lotus/pull/4170)
- Add verified status to api.DealInfo (https://github.com/filecoin-project/lotus/pull/4153)
- Add a CLI command to set a miner's owner address (https://github.com/filecoin-project/lotus/pull/4189)

#### Tooling and validation

- Lotus-pcr: add recover-miners command (https://github.com/filecoin-project/lotus/pull/3714)
- MpoolPushUntrusted API for gateway (https://github.com/filecoin-project/lotus/pull/3915)
- Test lotus-miner info all (https://github.com/filecoin-project/lotus/pull/4166)
- chain export: Error with unfinished exports (https://github.com/filecoin-project/lotus/pull/4179)
- add printf in TestWindowPost (https://github.com/filecoin-project/lotus/pull/4043)
- add trace wdpost (https://github.com/filecoin-project/lotus/pull/4020)
- Fix noncefix (https://github.com/filecoin-project/lotus/pull/4202)
- Lotus-pcr: Limit the fee cap of messages we will process, refund gas fees for windowed post and storage deals (https://github.com/filecoin-project/lotus/pull/4198)
- Fix pond (https://github.com/filecoin-project/lotus/pull/4203)
- allow manual setting of noncefix fee cap (https://github.com/filecoin-project/lotus/pull/4205)
- implement command to get execution traces of any message (https://github.com/filecoin-project/lotus/pull/4200)
- conformance: minor driver refactors (https://github.com/filecoin-project/lotus/pull/4211)
- lotus-pcr: ignore all other messages (https://github.com/filecoin-project/lotus/pull/4218)
- lotus-pcr: zero refund (https://github.com/filecoin-project/lotus/pull/4229)

## Contributors

The following contributors had 5 or more commits go into this release.
We are grateful for every contribution!

| Contributor        | Commits | Lines ±       |
|--------------------|---------|---------------|
| Stebalien          | 84       | +3425/-2287  |
| magik6k            | 41       | +2121/-506   |
| arajasek           | 39       | +2467/-424   |
| Kubuxu             | 25       | +2344/-775   |
| raulk              | 21       | +287/-196    |
| whyrusleeping      | 13       | +727/-71     |
| hsanjuan           | 13       | +5886/-7956  |
| dirkmc             | 11       | +2634/-576   | 
| travisperson       | 8        | +923/-202    |
| ribasushi          | 6        | +188/-128    |
| zgfzgf             | 5        | +21/-17      |

# 0.8.1 / 2020-09-30

This optional release of Lotus introduces a new version of markets which switches to CBOR-map encodings, and allows datastore migrations. The release also introduces several improvements to the mining process, a few performance optimizations, and a battery of UX additions and enhancements.

## Changes

#### Dependencies

- Markets 0.7.0 with updated data stores (https://github.com/filecoin-project/lotus/pull/4089)
- Update ffi to code with blst fixes (https://github.com/filecoin-project/lotus/pull/3998)

#### Core Lotus

- Fix GetPower with no miner address (https://github.com/filecoin-project/lotus/pull/4049)
- Refactor: Move nonce generation out of mpool (https://github.com/filecoin-project/lotus/pull/3970)

#### Performance

- Implement caching syscalls for import-bench (https://github.com/filecoin-project/lotus/pull/3888)
- Fetch tipset blocks in parallel (https://github.com/filecoin-project/lotus/pull/4074)
- Optimize Tipset equals() (https://github.com/filecoin-project/lotus/pull/4056)
- Make state transition in validation async (https://github.com/filecoin-project/lotus/pull/3868)

#### Mining

- Add trace window post (https://github.com/filecoin-project/lotus/pull/4020)
- Use abstract types for Dont recompute post on revert (https://github.com/filecoin-project/lotus/pull/4022)
- Fix injectNulls logic in test miner (https://github.com/filecoin-project/lotus/pull/4058)
- Fix potential panic in FinalizeSector (https://github.com/filecoin-project/lotus/pull/4092)
- Don't recompute post on revert (https://github.com/filecoin-project/lotus/pull/3924)
- Fix some failed precommit handling (https://github.com/filecoin-project/lotus/pull/3445)
- Add --no-swap flag for worker (https://github.com/filecoin-project/lotus/pull/4107)
- Allow some single-thread tasks to run in parallel with PC2/C2 (https://github.com/filecoin-project/lotus/pull/4116)

#### UX

- Add an envvar to set address network version (https://github.com/filecoin-project/lotus/pull/4028)
- Add logging to chain export (https://github.com/filecoin-project/lotus/pull/4030)
- Add JSON output to state compute (https://github.com/filecoin-project/lotus/pull/4038)
- Wallet list CLI: Print balances/nonces (https://github.com/filecoin-project/lotus/pull/4088)
- Added an option to show or not show sector info for `lotus-miner info` (https://github.com/filecoin-project/lotus/pull/4003)
- Add a command to import an ipld object into the chainstore (https://github.com/filecoin-project/lotus/pull/3434)
- Improve the lotus-shed dealtracker (https://github.com/filecoin-project/lotus/pull/4051)
- Docs review and re-organization (https://github.com/filecoin-project/lotus/pull/3431)
- Fix wallet list (https://github.com/filecoin-project/lotus/pull/4104)
- Add an endpoint to validate whether a string is a well-formed address (https://github.com/filecoin-project/lotus/pull/4106)
- Add an option to set config path (https://github.com/filecoin-project/lotus/pull/4103)
- Add printf in TestWindowPost (https://github.com/filecoin-project/lotus/pull/4043)
- Improve miner sectors list UX (https://github.com/filecoin-project/lotus/pull/4108)

#### Tooling

- Move policy change to seal bench (https://github.com/filecoin-project/lotus/pull/4032)
- Add back network power to stats (https://github.com/filecoin-project/lotus/pull/4050)
- Conformance: Record and feed circulating supply (https://github.com/filecoin-project/lotus/pull/4078)
- Snapshot import progress bar, add HTTP support (https://github.com/filecoin-project/lotus/pull/4070)
- Add lotus shed util to validate a tipset (https://github.com/filecoin-project/lotus/pull/4065)
- tvx: a test vector extraction and execution tool (https://github.com/filecoin-project/lotus/pull/4064)

#### Bootstrap

- Add new bootstrappers (https://github.com/filecoin-project/lotus/pull/4007)
- Add Glif node to bootstrap peers (https://github.com/filecoin-project/lotus/pull/4004)
- Add one more node located in China (https://github.com/filecoin-project/lotus/pull/4041)
- Add ipfsmain bootstrapper (https://github.com/filecoin-project/lotus/pull/4067)

# 0.8.0 / 2020-09-26

This consensus-breaking release of Lotus introduces an upgrade to the network. The changes that break consensus are:

- Upgrading to specs-actors v0.9.11, which reduces WindowPoSt faults per [FIP 0002](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0002.md) to reduce cost for honest miners with occasional faults (see https://github.com/filecoin-project/specs-actors/pull/1181)
- Revisions to some cryptoeconomics and network params

This release also updates go-fil-markets to fix an incompatibility issue between v0.7.2 and earlier versions.

## Changes

#### Dependencies

- Update spec actors to 0.9.11 (https://github.com/filecoin-project/lotus/pull/4039)
- Update markets to 0.6.3 (https://github.com/filecoin-project/lotus/pull/4013)

#### Core Lotus

- Network upgrade (https://github.com/filecoin-project/lotus/pull/4039)
- Fix AddSupportedProofTypes (https://github.com/filecoin-project/lotus/pull/4033)
- Return an error when we fail to find a sector when checking sector expiration (https://github.com/filecoin-project/lotus/pull/4026)
- Batch blockstore copies after block validation (https://github.com/filecoin-project/lotus/pull/3980)
- Remove a misleading miner actor abstraction (https://github.com/filecoin-project/lotus/pull/3977)
- Fix out-of-bounds when loading all sector infos (https://github.com/filecoin-project/lotus/pull/3976)
- Fix break condition in the miner (https://github.com/filecoin-project/lotus/pull/3953)

#### UX

- Correct helptext around miners setting ask (https://github.com/filecoin-project/lotus/pull/4009)
- Make sync wait nicer (https://github.com/filecoin-project/lotus/pull/3991)

#### Tooling and validation

- Small adjustments following network upgradability changes (https://github.com/filecoin-project/lotus/pull/3996)
- Add some more big pictures stats to stateroot stat (https://github.com/filecoin-project/lotus/pull/3995)
- Add some actors policy setters for testing (https://github.com/filecoin-project/lotus/pull/3975)

## Contributors

The following contributors had 5 or more commits go into this release.
We are grateful for every contribution!

| Contributor        | Commits | Lines ±       |
|--------------------|---------|---------------|
| arajasek           | 66       | +3140/-1261  |
| Stebalien          | 64       | +3797/-3434  |
| magik6k            | 48       | +1892/-976   |
| raulk              | 40       | +2412/-1549  |
| vyzo               | 22       | +287/-196    |
| alanshaw           | 15       | +761/-146    |
| whyrusleeping      | 15       | +736/-52     |
| hannahhoward       | 14       | +1237/-837   | 
| anton              | 6        | +32/-8       |
| travisperson       | 5        | +502/-6      |
| Frank              | 5        | +78/-39      |
| Jennifer           | 5        | +148/-41     |

# 0.7.2 / 2020-09-23

This optional release of Lotus introduces a major refactor around how a Lotus node interacts with code from the specs-actors repo. We now use interfaces to read the state of actors, which is required to be able to reason about different versions of actors code at the same time.

Additionally, this release introduces various improvements to the sync process, as well as changes to better the overall UX experience.

## Changes

#### Core Lotus

- Network upgrade support (https://github.com/filecoin-project/lotus/pull/3781)
- Upgrade markets to `v0.6.2` (https://github.com/filecoin-project/lotus/pull/3974)
- Validate chain sync response indices when fetching messages (https://github.com/filecoin-project/lotus/pull/3939)
- Add height diff to sync wait (https://github.com/filecoin-project/lotus/pull/3926)
- Replace Requires with Wants (https://github.com/filecoin-project/lotus/pull/3898)
- Update state diffing for market actor (https://github.com/filecoin-project/lotus/pull/3889)
- Parallel fetch for sync (https://github.com/filecoin-project/lotus/pull/3887)
- Fix SectorState (https://github.com/filecoin-project/lotus/pull/3881)

#### User Experience

- Add basic deal stats api server for spacerace slingshot (https://github.com/filecoin-project/lotus/pull/3963)
- When doing `sectors update-state`, show a list of existing states if user inputs an invalid one (https://github.com/filecoin-project/lotus/pull/3944)
- Fix `lotus-miner storage find` error (https://github.com/filecoin-project/lotus/pull/3927)
- Log shutdown method for lotus daemon and miner (https://github.com/filecoin-project/lotus/pull/3925)
- Update build and setup instruction link (https://github.com/filecoin-project/lotus/pull/3919)
- Add an option to hide removed sectors from `sectors list` output (https://github.com/filecoin-project/lotus/pull/3903)

#### Testing and validation

- Add init.State#Remove() for testing (https://github.com/filecoin-project/lotus/pull/3971)
- lotus-shed: add consensus check command (https://github.com/filecoin-project/lotus/pull/3933)
- Add keyinfo verify and jwt token command to lotus-shed (https://github.com/filecoin-project/lotus/pull/3914)
- Fix conformance gen (https://github.com/filecoin-project/lotus/pull/3892)

# 0.7.1 / 2020-09-17

This optional release of Lotus introduces some critical fixes to the window PoSt process. It also upgrades some core dependencies, and introduces many improvements to the mining process, deal-making cycle, and overall User Experience.

## Changes

#### Some notable improvements:

- Correctly construct params for `SubmitWindowedPoSt` messages (https://github.com/filecoin-project/lotus/pull/3909)
- Skip sectors correctly for Window PoSt (https://github.com/filecoin-project/lotus/pull/3839)
- Split window PoST submission into multiple messages (https://github.com/filecoin-project/lotus/pull/3689)
- Improve journal coverage (https://github.com/filecoin-project/lotus/pull/2455)
- Allow retrievals while sealing (https://github.com/filecoin-project/lotus/pull/3778)
- Don't prune locally published messages (https://github.com/filecoin-project/lotus/pull/3772)
- Add get-ask, set-ask retrieval commands (https://github.com/filecoin-project/lotus/pull/3886)
- Consistently name winning and window post in logs (https://github.com/filecoin-project/lotus/pull/3873))
- Add auto flag to mpool replace (https://github.com/filecoin-project/lotus/pull/3752))

#### Dependencies

- Upgrade markets to `v0.6.1` (https://github.com/filecoin-project/lotus/pull/3906)
- Upgrade specs-actors to `v0.9.10` (https://github.com/filecoin-project/lotus/pull/3846)
- Upgrade badger (https://github.com/filecoin-project/lotus/pull/3739)

# 0.7.0 / 2020-09-10

This consensus-breaking release of Lotus is designed to test a network upgrade on the space race testnet. The changes that break consensus are:

- Upgrading the Drand network used from the test Drand network to the League of Entropy main drand network. This is the same Drand network that will be used in the Filecoin mainnet.
- Upgrading to specs-actors v0.9.8, which adds a new method to the Multisig actor.

## Changes

#### Core Lotus

- Fix IsAncestorOf (https://github.com/filecoin-project/lotus/pull/3717)
- Update to specs-actors v0.9.8 (https://github.com/filecoin-project/lotus/pull/3725)
- Increase chain throughput by 20% (https://github.com/filecoin-project/lotus/pull/3732)
- Updare to go-libp2p-pubsub `master` (https://github.com/filecoin-project/lotus/pull/3735)
- Drand upgrade (https://github.com/filecoin-project/lotus/pull/3670)
- Multisig API additions (https://github.com/filecoin-project/lotus/pull/3590)

#### Storage Miner

- Increase the number of times precommit2 is attempted before moving back to precommit1 (https://github.com/filecoin-project/lotus/pull/3720)

#### Message pool

- Relax mpool add strictness checks for local pushes (https://github.com/filecoin-project/lotus/pull/3724)


#### Maintenance

- Fix devnets (https://github.com/filecoin-project/lotus/pull/3712)
- Fix(chainwatch): compare prev miner with cur miner (https://github.com/filecoin-project/lotus/pull/3715)
- CI: fix statediff build; make optional (https://github.com/filecoin-project/lotus/pull/3729)
- Feat: Chaos abort (https://github.com/filecoin-project/lotus/pull/3733)

## Contributors

The following contributors had commits go into this release.
We are grateful for every contribution!

| Contributor        | Commits | Lines ±       |
|--------------------|---------|---------------|
| arajasek           | 28      | +1144/-239    |
| Kubuxu             | 19      | +452/-261     |
| whyrusleeping      | 13      | +456/-87      |
| vyzo               | 11      | +318/-20      |
| raulk              | 10      | +1289/-350    |
| magik6k            | 6       | +188/-55      |
| dirkmc             | 3       | +31/-8        |
| alanshaw           | 3       | +176/-37      |
| Stebalien          | 2       | +9/-12        |
| lanzafame          | 1       | +1/-1         |
| frrist             | 1       | +1/-1         |
| mishmosh           | 1       | +1/-1         |
| nonsense           | 1       | +1/-0         |

# 0.6.2 / 2020-09-09

This release introduces some critical fixes to message selection and gas estimation logic. It also adds the ability for nodes to mark a certain tipset as checkpointed, as well as various minor improvements and bugfixes.

## Changes

#### Messagepool

- Warn when optimal selection fails to pack a block and we fall back to random selection (https://github.com/filecoin-project/lotus/pull/3708)
- Add basic command for printing gas performance of messages in the mpool (https://github.com/filecoin-project/lotus/pull/3701)
- Adjust optimal selection to always try to fill blocks (https://github.com/filecoin-project/lotus/pull/3685)
- Fix very minor bug in repub baseFeeLowerBound (https://github.com/filecoin-project/lotus/pull/3663)
- Add an auto flag to mpool replace (https://github.com/filecoin-project/lotus/pull/3676)
- Fix mpool optimal selection packing failure (https://github.com/filecoin-project/lotus/pull/3698)

#### Core Lotus

- Don't use latency as initital estimate for blocksync (https://github.com/filecoin-project/lotus/pull/3648)
- Add niceSleep 1 second when drand errors (https://github.com/filecoin-project/lotus/pull/3664)
- Fix isChainNearSync check in block validator (https://github.com/filecoin-project/lotus/pull/3650)
- Add peer to peer manager before fetching the tipset (https://github.com/filecoin-project/lotus/pull/3667)
- Add StageFetchingMessages to sync status (https://github.com/filecoin-project/lotus/pull/3668)
- Pass tipset through upgrade logic (https://github.com/filecoin-project/lotus/pull/3673)
- Allow nodes to mark tipsets as checkpointed (https://github.com/filecoin-project/lotus/pull/3680)
- Remove hard-coded late-fee in window PoSt (https://github.com/filecoin-project/lotus/pull/3702)
- Gas: Fix median calc (https://github.com/filecoin-project/lotus/pull/3686)

#### Storage

- Storage manager: bail out with an error if unsealed cid is undefined (https://github.com/filecoin-project/lotus/pull/3655)
- Storage: return true from Sealer.ReadPiece() on success (https://github.com/filecoin-project/lotus/pull/3657)

#### Maintenance

- Resolve lotus, test-vectors, statediff dependency cycle (https://github.com/filecoin-project/lotus/pull/3688)
- Paych: add docs on how to use paych status (https://github.com/filecoin-project/lotus/pull/3690)
- Initial CODEOWNERS (https://github.com/filecoin-project/lotus/pull/3691)

# 0.6.1 / 2020-09-08

This optional release introduces a minor improvement to the sync process, ensuring nodes don't fall behind and then resync.

## Changes

- Update `test-vectors` (https://github.com/filecoin-project/lotus/pull/3645)
- Revert "only subscribe to pubsub topics once we are synced" (https://github.com/filecoin-project/lotus/pull/3643)

# 0.6.0 / 2020-09-07

This consensus-breaking release of Lotus is designed to test a network upgrade on the space race testnet. The changes that break consensus are:

- Tweaking of some cryptoecon parameters in specs-actors 0.9.7 (https://github.com/filecoin-project/specs-actors/releases/tag/v0.9.7)
- Rebalancing FIL distribution to make testnet FIL scarce, which prevents base fee spikes and sets better expectations for mainnet

This release also introduces many improvements to Lotus! Among them are a new version of go-fil-markets that supports non-blocking retrieval, various spam reduction measures in the messagepool and p2p logic, and UX improvements to payment channels, dealmaking, and state inspection.

## Changes

#### Core Lotus and dependencies

- Implement faucet funds reallocation logic (https://github.com/filecoin-project/lotus/pull/3632)
- Network upgrade: Upgrade to correct fork threshold (https://github.com/filecoin-project/lotus/pull/3628)
- Update to specs 0.9.7 and markets 0.6.0 (https://github.com/filecoin-project/lotus/pull/3627)
- Network upgrade: Perform base fee tamping (https://github.com/filecoin-project/lotus/pull/3623)
- Chain events: if cache best() is nil, return chain head (https://github.com/filecoin-project/lotus/pull/3611)
- Update to specs actors v0.9.6 (https://github.com/filecoin-project/lotus/pull/3603)

#### Messagepool

- Temporarily allow negative chains (https://github.com/filecoin-project/lotus/pull/3625)
- Improve publish/republish logic (https://github.com/filecoin-project/lotus/pull/3592)
- Fix selection bug; priority messages were not included if other chains were negative (https://github.com/filecoin-project/lotus/pull/3580)
- Add defensive check for minimum GasFeeCap for inclusion within the next 20 blocks (https://github.com/filecoin-project/lotus/pull/3579)
- Add additional info about gas premium (https://github.com/filecoin-project/lotus/pull/3578)
- Fix GasPremium capping logic  (https://github.com/filecoin-project/lotus/pull/3552)

#### Payment channels

- Get available funds by address or by from/to (https://github.com/filecoin-project/lotus/pull/3547)
- Create `lotus paych status` command (https://github.com/filecoin-project/lotus/pull/3523)
- Rename CLI command from "paych get" to "paych add-funds" (https://github.com/filecoin-project/lotus/pull/3520)

#### Peer-to-peer

- Only subscribe to pubsub topics once we are synced (https://github.com/filecoin-project/lotus/pull/3602)
- Reduce mpool add failure log spam (https://github.com/filecoin-project/lotus/pull/3562)
- Republish messages even if the chains have negative performance(https://github.com/filecoin-project/lotus/pull/3557)
- Adjust gossipsub gossip factor (https://github.com/filecoin-project/lotus/pull/3556)
- Integrate pubsub Random Early Drop (https://github.com/filecoin-project/lotus/pull/3518)

#### Miscellaneous

- Fix panic in OnDealExpiredSlashed (https://github.com/filecoin-project/lotus/pull/3553)
- Robustify state manager against holes in actor method numbers (https://github.com/filecoin-project/lotus/pull/3538)

#### UX

- VM: Fix an error message (https://github.com/filecoin-project/lotus/pull/3608)
- Documentation: Batch replacement,update lotus-storage-miner to lotus-miner (https://github.com/filecoin-project/lotus/pull/3571)
- CLI: Robust actor lookup (https://github.com/filecoin-project/lotus/pull/3535)
- Add agent flag to net peers (https://github.com/filecoin-project/lotus/pull/3534)
- Add watch option to storage-deals list (https://github.com/filecoin-project/lotus/pull/3527)

#### Testing & tooling

- Decommission chain-validation (https://github.com/filecoin-project/lotus/pull/3606)
- Metrics: add expected height metric (https://github.com/filecoin-project/lotus/pull/3586)
- PCR: Use current tipset during refund (https://github.com/filecoin-project/lotus/pull/3570)
- Lotus-shed: Add math command (https://github.com/filecoin-project/lotus/pull/3568)
- PCR: Add tipset aggergation (https://github.com/filecoin-project/lotus/pull/3565)- Fix broken paych tests (https://github.com/filecoin-project/lotus/pull/3551)
- Make chain export ~1000x times faster (https://github.com/filecoin-project/lotus/pull/3533)
- Chainwatch: Stop SyncIncomingBlocks from leaking into chainwatch processing; No panics during processing (https://github.com/filecoin-project/lotus/pull/3526)
- Conformance: various changes (https://github.com/filecoin-project/lotus/pull/3521)

# 0.5.10 / 2020-09-03

This patch includes a crucial fix to the message pool selection logic, strongly disfavouring messages that might cause a miner penalty.

## Changes

- Fix calculation of GasReward in messagepool (https://github.com/filecoin-project/lotus/pull/3528)

# 0.5.9 / 2020-09-03

This patch includes a hotfix to the `GasEstimateFeeCap` method, capping the estimated fee to a reasonable level by default.

## Changes

- Added target height to sync wait (https://github.com/filecoin-project/lotus/pull/3502)
- Disable codecov annotations (https://github.com/filecoin-project/lotus/pull/3514)
- Cap fees to reasonable level by default (https://github.com/filecoin-project/lotus/pull/3516)
- Add APIs and command to inspect bandwidth usage (https://github.com/filecoin-project/lotus/pull/3497)
- Track expected nonce in mpool, ignore messages with large nonce gaps (https://github.com/filecoin-project/lotus/pull/3450)

# 0.5.8 / 2020-09-02

This patch includes some bugfixes to the sector sealing process, and updates go-fil-markets. It also improves the performance of blocksync, adds a method to export chain state trees, and improves chainwatch.

## Changes

- Upgrade markets to v0.5.9 (https://github.com/filecoin-project/lotus/pull/3496)
- Improve blocksync to load fewer messages: (https://github.com/filecoin-project/lotus/pull/3494)
- Fix a panic in the ffi-wrapper's `ReadPiece` (https://github.com/filecoin-project/lotus/pull/3492/files)
- Fix a deadlock in the sealing scheduler (https://github.com/filecoin-project/lotus/pull/3489)
- Add test vectors for tipset tests (https://github.com/filecoin-project/lotus/pull/3485/files)
- Improve the advance-block debug command (https://github.com/filecoin-project/lotus/pull/3476)
- Add toggle for message processing to Lotus PCR (https://github.com/filecoin-project/lotus/pull/3470)
- Allow exporting recent chain state trees (https://github.com/filecoin-project/lotus/pull/3463)
- Remove height from chain rand (https://github.com/filecoin-project/lotus/pull/3458)
- Disable GC on chain badger datastore (https://github.com/filecoin-project/lotus/pull/3457)
- Account for `GasPremium` in `GasEstimateFeeCap` (https://github.com/filecoin-project/lotus/pull/3456)
- Update go-libp2p-pubsub to `master` (https://github.com/filecoin-project/lotus/pull/3455)
- Chainwatch improvements (https://github.com/filecoin-project/lotus/pull/3442)

# 0.5.7 / 2020-08-31

This patch release includes some bugfixes and enhancements to the sector lifecycle and message pool logic.

## Changes

- Rebuild unsealed infos on miner restart (https://github.com/filecoin-project/lotus/pull/3401)
- CLI to attach storage paths to workers (https://github.com/filecoin-project/lotus/pull/3405)
- Do not select negative performing message chains for inclusion (https://github.com/filecoin-project/lotus/pull/3392)
- Remove a redundant error-check (https://github.com/filecoin-project/lotus/pull/3421)
- Correctly move unsealed sectors in `FinalizeSectors` (https://github.com/filecoin-project/lotus/pull/3424)
- Improve worker selection logic (https://github.com/filecoin-project/lotus/pull/3425)
- Don't use context to close bitswap (https://github.com/filecoin-project/lotus/pull/3430)
- Correctly estimate gas premium when there is only one message on chain (https://github.com/filecoin-project/lotus/pull/3428)

# 0.5.6 / 2020-08-29

Hotfix release that fixes a panic in the sealing scheduler (https://github.com/filecoin-project/lotus/pull/3389).

# 0.5.5

This patch release introduces a large number of improvements to the sealing process.
It also updates go-fil-markets to
[version 0.5.8](https://github.com/filecoin-project/go-fil-markets/releases/tag/v0.5.8),
and go-libp2p-pubsub to [v0.3.5](https://github.com/libp2p/go-libp2p-pubsub/releases/tag/v0.3.5).

#### Downstream upgrades

- Upgrades markets to v0.5.8 (https://github.com/filecoin-project/lotus/pull/3384)
- Upgrades go-libp2p-pubsub to v0.3.5 (https://github.com/filecoin-project/lotus/pull/3305)

#### Sector sealing

- The following improvements were introduced in https://github.com/filecoin-project/lotus/pull/3350.

  - Allow `lotus-miner sectors remove` to remove a sector in any state.
  - Create a separate state in the storage FSM dedicated to submitting the Commit message.
  - Recovery for when the Deal IDs of deals in a sector get changed in a reorg.
  - Auto-retry sending Precommit and Commit messages if they run out of gas
  - Auto-retry sector remove tasks when they fail
  - Compact worker windows, and allow their tasks to be executed in any order

- Don't simply skip PoSt for bad sectors (https://github.com/filecoin-project/lotus/pull/3323)

#### Message Pool

- Spam Protection: Track required funds for pending messages (https://github.com/filecoin-project/lotus/pull/3313)

#### Chainwatch

- Add more power and reward metrics (https://github.com/filecoin-project/lotus/pull/3367)
- Fix raciness in sector deal table (https://github.com/filecoin-project/lotus/pull/3275)
- Parallelize miner processing (https://github.com/filecoin-project/lotus/pull/3380)
- Accept Lotus API and token (https://github.com/filecoin-project/lotus/pull/3337)

# 0.5.4

A patch release, containing a few nice bugfixes and improvements:

- Fix parsing of peer ID in `lotus-miner actor set-peer-id` (@whyrusleeping)
- Update dependencies, fixing several bugs (@Stebalien)
- Fix remaining linter warnings (@Stebalien)
- Use safe string truncation (@Ingar)
- Allow tweaking of blocksync message window size (@whyrusleeping)
- Add some additional gas stats to metrics (@Kubuxu)
- Fix an edge case bug in message selection, add many tests (@vyzo)

# 0.5.3

Yet another hotfix release.
A lesson for readers, having people who have been awake for 12+ hours review
your hotfix PR is not a good idea. Find someone who has enough slept recently
enough to give you good code review, otherwise you'll end up quickly bumping
versions again.

- Fixed a bug in the mempool that was introduced in v0.5.2

# 0.5.2 / 2020-08-24

This is a hotfix release.

- Fix message selection to not include messages that are invalid for block
  inclusion.
- Improve SelectMessage handling of the case where the message pools tipset
  differs from our mining base.

# 0.5.1 / 2020-08-24

The Space Race release!
This release contains the genesis car file and bootstrap peers for the space
race network.

Additionally, we included two small fixes to genesis creation:
- Randomize ticket value in genesis generation
- Correctly set t099 (burnt funds actor) to have valid account actor state

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

# Lotus changelog

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
| hannahhoward       | 14       | +1237/837-   | 
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

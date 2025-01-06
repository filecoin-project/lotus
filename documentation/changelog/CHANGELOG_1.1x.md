# Lotus changelog v1.10.0 - v1.19.0

* [1.19.0 / 2022-12-07](#1190--2022-12-07)
* [1.18.2 / 2022-12-10](#1182--2022-12-10)
* [1.18.1 / 2022-11-28](#1181--2022-11-28)
* [1.18.0 / 2022-11-15](#1180--2022-11-15)
* [v1.17.2 / 2022-10-05](#v1172--2022-10-05)
* [v1.17.1 / 2022-09-06](#v1171--2022-09-06)
* [v1.17.0 / 2022-08-02](#v1170--2022-08-02)
* [1.16.1 / 2022-07-07](#1161--2022-07-07)
* [1.16.0 / 2022-06-24](#1160--2022-06-24)
* [1.15.3 / 2022-05-31](#1153--2022-05-31)
* [1.15.2 / 2022-05-06](#1152--2022-05-06)
* [1.15.1 / 2022-04-07](#1151--2022-04-07)
* [1.15.0 / 2022-03-09](#1150--2022-03-09)
* [1.14.4 / 2022-03-03](#1144--2022-03-03)
* [v1.14.3 / 2022-02-28](#v1143--2022-02-28)
* [1.14.2 / 2022-02-24](#1142--2022-02-24)
* [1.14.1 / 2022-02-18](#1141--2022-02-18)
* [1.14.0 / 2022-02-17](#1140--2022-02-17)
* [v1.13.2 /  2022-01-09](#v1132---2022-01-09)
* [v1.13.1 / 2021-11-26](#v1131--2021-11-26)
* [v1.13.0 / 2021-10-18](#v1130--2021-10-18)
* [v1.12.0 / 2021-10-12](#v1120--2021-10-12)
* [v1.11.3 / 2021-09-29](#v1113--2021-09-29)
* [v1.11.2 / 2021-09-06](#v1112--2021-09-06)
* [1.11.1 / 2021-08-16](#1111--2021-08-16)
* [1.11.0 / 2021-07-22](#1110--2021-07-22)
* [1.10.1 / 2021-07-05](#1101--2021-07-05)
* [1.10.1 / 2021-07-05](#1101--2021-07-05-1)
* [1.10.0 / 2021-06-23](#1100--2021-06-23)

# 1.19.0 / 2022-12-07

This is an optional feature release of Lotus. This feature release includes the SplitStore beta, the experimental Lotus node cluster feature, as well as numerous enhancments and bugfixes.

## Highlights
### 🟢 SplitStore v2（Beta) 🟢

Splitstore aims to reduce the node performance impact that's caused by the Filecoin's very large, and continuously growing chain datastore by having a hot and cold blockstore. You can find more about the Splitstore implementation [here](https://github.com/filecoin-project/lotus/blob/master/blockstore/splitstore/README.md).
Splitstore has three basic modes for node operators to configure according to your needs: 
- `discard`: hotstore only, automatically archive out-of-scope objects that are beyond 4 finalities(3600 epochs).
- `universal`:  stores all chain data that's beyond 4 finalities into coldstore.
- `messages`: only stores on-chain messages into coldstore.

The `EnableColdStoreAutoPrune=` configuration is being deprecated in this release, as there is only ever one compaction running. We welcome all node operators to try the new feature and let us know [here](https://github.com/filecoin-project/lotus/discussions/9179) if you have any feedback! 
There are more configuration one may set, you can read the full documentation about the SplitStoreV2 here: https://lotus.filecoin.io/lotus/configure/splitstore/.

### 🧪 Node Cluster (*EXPERIMENTAL.*) 🧪
The Lotus HA node cluster feature allows you to run multiple Lotus daemons for the same lotus-miner increasing resiliency. We welcome all Lotus users to join the early testing for this feature and provide your feedback. Please note that this feature is targeted towards more enterprise users of Lotus and requires at least 3 lotus nodes to be set up in a cluster.
Check out the documentation here: https://lotus.filecoin.io/lotus/configure/clusters/

### ⭐️ SnapDeals Enhancements ⭐️
Numerous SnapDeals related improvements and fixes made it into this release before the code freeze. Some the highlights of the issues that has been fixed in this feature release are:

- *Unable to snap-up a sector again if something went wrong.* - This has now been fixed ✅
- *Error messages on loop during an open deadline.* - This has now been fixed ✅

## New features
- feat:splitstore:single compaction that can handle prune aka two marksets one compaction (#9571) ([filecoin-project/lotus#9571](https://github.com/filecoin-project/lotus/pull/9571))
   - Introduces a new SplitStore-mode, `messages`, which will only store on-chain messages. Fixes previously issues with regards to `AutoPrune` not compacting the coldstore. [Link to documentation](https://lotus.filecoin.io/lotus/configure/splitstore/)
- feat: Raft consensus for lotus nodes in a cluster ([filecoin-project/lotus#9294](https://github.com/filecoin-project/lotus/pull/9294))
   - Adds the experimental node cluster feature.
- feat: storage: Force exit GenerateSingleVanillaProof on cancelled context ([filecoin-project/lotus#9613](https://github.com/filecoin-project/lotus/pull/9613))
   - `GenerateSingleVanillaProof` now respects context, which means that it will skip slow to read sectors :snail: and return a context error. Instead of being blocked forever if storage reads where blocked (e.g disconnected NFS).
- feat: wdpost: Configurable pre-check timeouts ([filecoin-project/lotus#9680](https://github.com/filecoin-project/lotus/pull/9680))
   - Adds configuration knobs for setting custom amount of time a proving pre-check can take before a sector and partition will be skipped. [Link to documentation](https://lotus.filecoin.io/storage-providers/advanced-configurations/proving/#pre-check-sector-timeout)
- feat: chain: future proof the from & to address protocols ([filecoin-project/lotus#9515](https://github.com/filecoin-project/lotus/pull/9515))
   - This lets us add new address protocols to go-address without implicitly accepting them in messages on the network.
- feat: Retrieval into remote blockstores ([filecoin-project/lotus#9565](https://github.com/filecoin-project/lotus/pull/9565))
   - Makes it possible to point retrievals at a network-backed blockstore.
- feat: Add node uptime rpc / output in info command ([filecoin-project/lotus#9436](https://github.com/filecoin-project/lotus/pull/9436))
   - Adds node uptime stats to the `lotus-miner info` and `lotus info` commands
- feat: wdpost: Add ability to only have single partition per msg for partitions with… ([filecoin-project/lotus#9413](https://github.com/filecoin-project/lotus/pull/9413))
  - Adds a configuration option to have a single partition per PoSt Message for partitions containing recovering sectors.
- feat: miner paramfetch: Don't fetch param files when not needed ([filecoin-project/lotus#9391](https://github.com/filecoin-project/lotus/pull/9391))
   - A Lotus-Miner processes that has disabled local PoSt / C2 / PR2 does not need the param-files. This makes node startup much faster, reducing downtime by a lot when restarts are needed.
- feat: client: Add retrieval deal ID and bytes transferred to retrieval output ([filecoin-project/lotus#9398](https://github.com/filecoin-project/lotus/pull/9398))
   - Appends retrieval deal ID and bytes transferred to the retrieval output.
- feat: dealpublisher: check for duplicate deals before adding ([filecoin-project/lotus#9365](https://github.com/filecoin-project/lotus/pull/9365))
- feat: Drop active retrieval check (#764) ([filecoin-project/go-fil-markets#764](https://github.com/filecoin-project/go-fil-markets/pull/764))
- feat(retrievalmarkets): expose GetDynamicAsk (#748) ([filecoin-project/go-fil-markets#748](https://github.com/filecoin-project/go-fil-markets/pull/748))
- feat: handle retrieval queries for unindexed identity payload CIDs (#747) ([filecoin-project/go-fil-markets#747](https://github.com/filecoin-project/go-fil-markets/pull/747))
- feat: add a method for validating an address for a network version (#115) ([filecoin-project/go-state-types#115](https://github.com/filecoin-project/go-state-types/pull/115))

## Improvements
- fix: miner-cli: Fix lotus-miner proving check ([filecoin-project/lotus#9643](https://github.com/filecoin-project/lotus/pull/9643))
   - Fixes the issue where the `lotus-miner proving check` command always outputted `Error: rg is nil`
- fix: sealing pipeline: Clear CreationTime when starting sector upgrade ([filecoin-project/lotus#9642](https://github.com/filecoin-project/lotus/pull/9642))
   - Fixes the issue where an aborted SnapDeal upgrade could no longer be retried with SnapDeals.
- fix:sealing-fsm:wait mutable fsm state for immutable sector upgrade error ([filecoin-project/lotus#9598](https://github.com/filecoin-project/lotus/pull/9598))
   - Creating a new WaitMutable state - now if the deadline is open and the sectors are trying finalize they will wait on the worker until the deadline has closed. Important to note that they will not finalize as soon as the deadline closes, they will wait 1h before continuing. Fixes the previous issue where upgraded Snap-sectors for an open deadline cause a lot of `error-messages` and `p_aux` issues
- fix: cli: add beneficiary info to lotus-miner actor control list ([filecoin-project/lotus#9632](https://github.com/filecoin-project/lotus/pull/9632))
   - Adds the beneficiary address to the `lotus-miner actor control list` output.
- fix: sealing pipeine: Release assigned deals on snapdeals abort ([filecoin-project/lotus#9601](https://github.com/filecoin-project/lotus/pull/9601))
- fix: docker: make compatible with arm platform ([filecoin-project/lotus#9363](https://github.com/filecoin-project/lotus/pull/9363))
   - Makes the `Dockerfile.lotus` compatible with ARM-platforms (e.g Mac M1).
- fix: post worker sched: Don't check worker session in a busy loop ([filecoin-project/lotus#9495](https://github.com/filecoin-project/lotus/pull/9495))
   - Fixes a looping pattern which could result in a flood of requests between `lotus-miner`<->`lotus-worker`, potentially exhausting resources needed to make http requests, that lead to all sorts of random RPC-related issues.
- fix: miner: init miner's with 32GiB sectors by default ([filecoin-project/lotus#9364](https://github.com/filecoin-project/lotus/pull/9364))
   - Makes the `lotus-miner init` defualt to 32GiB sectors.
- fix: store identity CIDs in CARs for online deals (#749) ([filecoin-project/go-fil-markets#749](https://github.com/filecoin-project/go-fil-markets/pull/749))
- fix: cliutil: Fix URL-based API endpoint parsing

## Dependencies
- deps: upgrade go-merkledag to 0.8.1 (#9717)
- deps: Update go-fil-markets to v1.25.0 ([filecoin-project/lotus#9633](https://github.com/filecoin-project/lotus/pull/9633))
- deps: upgrade go-merkledag to 0.8.0 ([filecoin-project/lotus#9455](https://github.com/filecoin-project/lotus/pull/9455))

## Others
- fix/build: Update Zondax/hid to 0.9.1
- refactor: sealing: minor refactor of FinalizeReplicaUpdate ([filecoin-project/lotus#9614](https://github.com/filecoin-project/lotus/pull/9614))
- update ffi to 280c4f8b94fd46dc (#9608) ([filecoin-project/lotus#9608](https://github.com/filecoin-project/lotus/pull/9608))
- fix: tvx: make it work with the FVM ([filecoin-project/lotus#9604](https://github.com/filecoin-project/lotus/pull/9604))
- fix: autobatch: remove potential deadlock when a block is missing ([filecoin-project/lotus#9602](https://github.com/filecoin-project/lotus/pull/9602))
- feat: shed: set control address: add dump bytes option ([filecoin-project/lotus#9572](https://github.com/filecoin-project/lotus/pull/9572))
- feat: shed: Online export-car ([filecoin-project/lotus#9590](https://github.com/filecoin-project/lotus/pull/9590))
- fix: chain: Update chain.go ([filecoin-project/lotus#9373](https://github.com/filecoin-project/lotus/pull/9373))
- fix: fvm: Allow setting local bundles for Debug FVM for av 9+ ([filecoin-project/lotus#9509](https://github.com/filecoin-project/lotus/pull/9509))
- fix: ux: Add outputs to wallet commands ([filecoin-project/lotus#9416](https://github.com/filecoin-project/lotus/pull/9416))
- fix: ux: specify arg in `net ping` cmd ([filecoin-project/lotus#9459](https://github.com/filecoin-project/lotus/pull/9459))
- fix: cli: renew --only-cc with sectorfile ([filecoin-project/lotus#9402](https://github.com/filecoin-project/lotus/pull/9402))
- fix: bstore: Handle codecs correctly in membstore Get ([filecoin-project/lotus#9471](https://github.com/filecoin-project/lotus/pull/9471))
- fix: not multiplied by the number of seconds ([filecoin-project/lotus#9460](https://github.com/filecoin-project/lotus/pull/9460))
- Makefile: Drop rarely used binaries from build-devnets (#9513) ([filecoin-project/lotus#9513](https://github.com/filecoin-project/lotus/pull/9513))
- _ci_ Remove unneeded homebrew deps ([filecoin-project/lotus#9559](https://github.com/filecoin-project/lotus/pull/9559))
- _ci_: Have apt-get wait until it can get a lock in packer builds ([filecoin-project/lotus#9534](https://github.com/filecoin-project/lotus/pull/9534))
- _ci_ Appimage go1.18.1 fix ([filecoin-project/lotus#9496](https://github.com/filecoin-project/lotus/pull/9496))
- _ci_: Fix failing Digital Ocean and Amazon Machine Image release process ([filecoin-project/lotus#9425](https://github.com/filecoin-project/lotus/pull/9425))
- _ci_: Don't publish new homebrew releases for RC builds ([filecoin-project/lotus#9350](https://github.com/filecoin-project/lotus/pull/9350))
- _ci_: Use golang 1.18.1 to build appimage ([filecoin-project/lotus#9386](https://github.com/filecoin-project/lotus/pull/9386))
- _ci_ Refactor release pipeline to better support m1 builds ([filecoin-project/lotus#9600](https://github.com/filecoin-project/lotus/pull/9600))
- _ci_: Rely on local env varibale instead of context ([filecoin-project/lotus#9740](https://github.com/filecoin-project/lotus/pull/9740))
- feat: shed: FIP0036 post poll result processing  ([filecoin-project/lotus#9387](https://github.com/filecoin-project/lotus/pull/9387))
- misc: github: Cleanup PR template ([filecoin-project/lotus#9472](https://github.com/filecoin-project/lotus/pull/9472))
- docs: release template: Mention codegen in release prep ([filecoin-project/lotus#9430](https://github.com/filecoin-project/lotus/pull/9430))
- chore: merge releases (v1.17.2) into master ([filecoin-project/lotus#9433](https://github.com/filecoin-project/lotus/pull/9433))
- chore: merge release/v1.18.0 into master ([filecoin-project/lotus#9597](https://github.com/filecoin-project/lotus/pull/9597))
- chore:shed: Teach shed/sim to understand --tipset=@nnn notation ([filecoin-project/lotus#9434](https://github.com/filecoin-project/lotus/pull/9434))
- _chore_: Upgrade `hid` ([filecoin-project/lotus#9406](https://github.com/filecoin-project/lotus/pull/9406))
- chore: release: Update `release_issue_template` ([filecoin-project/lotus#9150](https://github.com/filecoin-project/lotus/pull/9150))
- chore: update lotus version to 1.19.0-rc1
- chore: merge release into master ([filecoin-project/lotus#9657](https://github.com/filecoin-project/lotus/pull/9657))
- Backport: #9061 rpc errors ([filecoin-project/lotus#9384](https://github.com/filecoin-project/lotus/pull/9384))
- shed: util: get all msig ([filecoin-project/lotus#9322](https://github.com/filecoin-project/lotus/pull/9322))
- fix: test: simplify TestDeadlineToggling ([filecoin-project/lotus#9356](https://github.com/filecoin-project/lotus/pull/9356))
- fix: build: set PropagationDelaySecs correctly ([filecoin-project/lotus#9358](https://github.com/filecoin-project/lotus/pull/9358))
- fix: test: flaky TestDeadlineToggling around nulls (#9354) ([filecoin-project/lotus#9354](https://github.com/filecoin-project/lotus/pull/9354))
- fix: retrievals: price-per-byte calculation fix ([filecoin-project/lotus#9353](https://github.com/filecoin-project/lotus/pull/9353))
- fix: docs: update Go-badge in readme ([filecoin-project/lotus#9347](https://github.com/filecoin-project/lotus/pull/9347))
- docs: release template: clarify location of version.go ([filecoin-project/lotus#9338](https://github.com/filecoin-project/lotus/pull/9338))
- build: Bump version to v1.17.3-dev ([filecoin-project/lotus#9337](https://github.com/filecoin-project/lotus/pull/9337))
- github.com/filecoin-project/go-fil-markets (v1.24.0-v17 -> v1.25.0):
- Merge branch 'release/v1.24.3'
- Update ffi and update markets to v9 (#751) (#761) ([filecoin-project/go-fil-markets#761](https://github.com/filecoin-project/go-fil-markets/pull/761))
- chore: extract Provider piece logic to separate file (#750) ([filecoin-project/go-fil-markets#750](https://github.com/filecoin-project/go-fil-markets/pull/750))
- Merge branch 'release/v1.24.2'
- Feat/support custom metadata (#759) ([filecoin-project/go-fil-markets#759](https://github.com/filecoin-project/go-fil-markets/pull/759))
- Revert "Update ffi and update markets to v9 (#751)" (#755) ([filecoin-project/go-fil-markets#755](https://github.com/filecoin-project/go-fil-markets/pull/755))
- Update ffi and update markets to v9 (#751) ([filecoin-project/go-fil-markets#751](https://github.com/filecoin-project/go-fil-markets/pull/751))
- release: v1.24.0 ([filecoin-project/go-fil-markets#745](https://github.com/filecoin-project/go-fil-markets/pull/745))
- github.com/filecoin-project/go-state-types (v0.9.8 -> v0.9.9):

## lotus-market EOL notice 

As mentioned in [lotus v1.17.0 release notes](https://github.com/filecoin-project/lotus/releases/tag/v1.17.0), markets related features, enhancements and fixes is now lower priority for Lotus. We recommend our users to migrate to other deal making focused software, like [boost](https://boost.filecoin.io/) as soon as possible. That being said, the lotus maintainers will be:
-  Lotus maintainers will stop supporting lotus-market subcomponent/**storage** deal making related issues or enhancements on Jan 31, 2023.
- In Q2 2023, we will be deprecating/removing lotus-market related code from this repository.

If you have any questions or concerns, please raise them in [Lotus discussion](https://github.com/filecoin-project/lotus/discussions/categories/market)!

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Geoff Stuart | 69 | +4745/-19478 | 405 |
| Shrenuj Bansal | 39 | +5257/-2183 | 243 |
| Łukasz Magiera | 32 | +2763/-730 | 169 |
| Aayush | 47 | +1439/-1138 | 157 |
| Ian Davis | 21 | +556/-1065 | 41 |
| Rod Vagg | 5 | +657/-320 | 18 |
| jennijuju | 4 | +632/-317 | 6 |
| Aayush Rajasekaran | 13 | +700/-135 | 18 |
| Jennifer Wang | 14 | +740/-54 | 25 |
| ZenGround0 | 1 | +193/-195 | 14 |
| Hannah Howard | 4 | +138/-122 | 52 |
| Steven Allen | 4 | +105/-24 | 11 |
| zenground0 | 9 | +109/-16 | 14 |
| Peter Rabbitson | 1 | +27/-23 | 3 |
| hannahhoward | 2 | +49/-0 | 2 |
| Airenas Vaičiūnas | 2 | +31/-16 | 2 |
| simlecode | 6 | +19/-10 | 12 |
| Phi | 5 | +16/-10 | 7 |
| sectrgt | 2 | +18/-0 | 2 |
| Jiaying Wang | 2 | +4/-4 | 3 |
| Rob Quist | 1 | +3/-1 | 1 |
| Jakub Sztandera | 1 | +1/-1 | 1 |

# 1.18.2 / 2022-12-10

This is an OPTIONAL patch release that fixes a recently reported bug, where the miner process crashes due to a panic during an AddPiece process. More details can be found [here](https://github.com/filecoin-project/lotus/pull/9822).


# 1.18.1 / 2022-11-28

This is a small OPTIONAL patch release for the mandatory v1.18.0 release that supports the Filecoin nv17 Shark Upgrade. 
We highly recommend you to read the full [v1.18.0 release note](https://github.com/filecoin-project/lotus/releases/tag/v1.18.0) if you haven't already.

Note to SPs:
If you are running into issue with updating your miner node from an earlier release and is failing to restart your miner, check your `journalctl` and see if you noticed the following error:
```
New sector storage: <%d>
Nov 19 15:03:43 g0lotus01 lotus-miner[<id>]: ERROR: creating node: starting node: could not build arguments for function "reflect".makeFuncStub (/usr/local/go/src/reflect/asm_amd64.s:28): failed to build *paths.Local: received non-nil error from function "reflect".makeFuncStub (/usr/local/go/src/reflect/asm_amd64.s:28): opening path /media/data1/lotusstorage: path with ID <%d> already opened: '<path>'
```
If so, this check is introduced via [#9032](https://github.com/filecoin-project/lotus/pull/9032), precisely this [line](https://github.com/filecoin-project/lotus/blob/v1.18.1/storage/paths/local.go#L221~L223).
It's added to prevents double-attaching paths given it's now possible to manipulate paths at runtime. Verify storage.json configs if you encounter this error or remove the undesired duplicated path as you see fit accordingly.

## Bug Fixes

- fix: cli: check found before dereferencing SectorInfo #9703

# 1.18.0 / 2022-11-15

> ⚠️ **Please note that from Lotus v1.17.2&^ will require a Go-version of v1.18.1&^**

This is the final release of the upcoming MANDATORY release of Lotus that introduces [Filecoin network v17,
codenamed the Shark upgrade](https://github.com/filecoin-project/community/discussions/74?sort=top#discussioncomment-3825422). Shark upgrade delivers a wave of protocol refinements that will allow for useful smart contracts to be written using the FVM (eg. programmable markets, lending contracts, etc.).

**The Filecoin mainnet is scheduled to upgrade to nv17 at epoch 2383680, on Nov 30th on 2022-11-30T14:00:00Z. All node operators, including storage providers, must upgrade to this release before that time. Storage providers must update their daemons, miners, market and worker(s)/boost.**

The Shark upgrade introduces the following FIPs, delivered in [actors v9](https://github.com/filecoin-project/builtin-actors/releases/tag/v9.0.3):
- [FIP0029 Beneficiary Address for Storage Providers](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0029.md): step towards better lending market for SP
- [FIP0034 Fix PreCommit Deposit Independent of Sector Content](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0034.md): resolves a significant weakening of Filecoin PoRep’s security guarantees
  - ❗Pre-commit deposit will be calculated as the 20-day projection of expected reward earned by a sector with **a sector quality of 10 (i.e. full of verified deals)**, regardless of sector content. The Initial Pledge value, due when the sector is proven, is left **unchanged**.
- [FIP0041 Forward Compatibility for PreCommit](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0041.md): enables a cleaner and easier transition to Programmable Storage Markets
- [FIP0044 Standard Message Authentication](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0044.md): enable metadata authentication for user defined actor
- [FIP0045 Decoupling Fil+ from Marketplace](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0045.md): DataCap and the 10x QAP is now only associated with how long DATA is wanted to be stored on the network.
  - This is a transitional state to enabling far more efficient and dynamic storage markets on Filecoin network in the future.
  - ⭐️ First Fungible Token Contract - Datacap Actor, on Filecoin!  ([fungible token standard](https://github.com/filecoin-project/FIPs/blob/master/FRCs/frc-0046.md), [token contract library](https://github.com/helix-onchain/filecoin/tree/5455f4f831e0f3f20005a9a789623d7ad6dada15/frc46_token)).
  - For storage deal participants (clients and storage providers):
    - `PublishStorageDeals`/`ProveCommit(Aggregate)`/`ProveReplicaUpdates` message that includes verified deals will see a gas usage increase, more details can be found [here](https://github.com/filecoin-project/FIPs/blob/385f069b3b146c5fef4fdc1109a0e2f35f399e48/FIPS/fip-0045.md?plain=1#L784)
    -  `Term` is introduced for defining how long the DataCap is assigned to a piece of data. Anyone who cares about that piece of data may extend the _term_, which incentives SPs to store the data longer on the network without a new deal/resealing.
    - There is no  more diluted verified deal QAP due to deal/sector space time for new sectors that contains verified deals after this upgrade.
    - SPs may enjoy 90 days of extra QAP than deal duration by default, given `term_max` is always `deal duration + 90 days`.
❗ We highly recommend all lotus users, especially storage providers, developers and clients to read the FIPs in detail to understand the protocol changes and potential impact to network participants!
      
## Snapshots

The [#fil-infra](https://filecoinproject.slack.com/archives/C039RBG3RPC) team at PL has launched a brand new Lightweight Filecoin Chain Snapshots Service to support chain management needs for the node operators, check [here](https://www.notion.so/pl-strflt/Lightweight-Filecoin-Chain-Snapshots-17e4c386f35c44548f5863afb7b5e024) for the full detail. 
We are planning to switch [the snapshot service listed in lotus docs](https://lotus.filecoin.io/lotus/manage/chain-management/#lightweight-snapshot) to the new Lightweight Filecoin Chain Snapshots Service by EOY, and deprecate public support of the current snapshots production. We recommend all users to test and switch the new service ASAP, and if you run into any issue, please report them [here](https://github.com/filecoin-project/filecoin-chain-archiver/discussions/new?category=feedback) and the team would be happy to support you! For the main differences between the old & the new service, checkout the FAQ section [here](https://www.notion.so/pl-strflt/Lightweight-Filecoin-Chain-Snapshots-17e4c386f35c44548f5863afb7b5e024)

## Migration 

We are expecting a heavier than normal state migration for this upgrade due to the amount of the state changes introduced.
All node operators, including storage providers, should be aware that two pre-migrations are being scheduled. The first pre-migration will begin at 2022-11-30T12:00:00Z (120 minutes before the real upgrade), the second pre-migration will begin at 2022-11-30T13:45:00Z (7.5 minutes before the real upgrade). 
The first pre-migration will take up to 1.5hr, depending on the amount of the historical state in the node blockstore and the hardware specs the node is running on. During this time, expect slower block validation times, increased CPU and memory usage, and longer delays for API queries. 
We recommend node operators (who haven't enbabled splistore `universal` mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.
Note to full archival node operators: you may expect a migration that takes up to 20 min upon the upgrade, during this period your node will fall out of sync and your chain service may have some disruption. However, you can expect the node to catch up soon after the migration completes.

### v9 Built-in actor bundles

Bundles for all networks(mainnet, calibnet, and etc) are included in the lotus source tree (`build/actors/`) and embedded on build, for v9 actors you can find it [here](https://github.com/filecoin-project/lotus/blob/master/build/actors/v9.tar.zst). 
Reminder: Lotus verifies that the bundle CIDs are the right ones upon build & upgrade against the values in `build/builtin_actors_gen.go`, according to the network you are building. You may also check the bundle manifest CID matches the bundle gen-ed values by running `lotus state actor-cids --network-version 17`.

The manifest CID & full list of actor code CIDs for nv17 using [actor v9](https://github.com/filecoin-project/builtin-actors/releases/tag/v9.0.3) is:

```
"_manifest":        "bafy2bzaceb6j6666h36xnhksu3ww4kxb6e25niayfgkdnifaqi6m6ooc66i6i"
"account":          "bafk2bzacect2p7urje3pylrrrjy3tngn6yaih4gtzauuatf2jllk3ksgfiw2y"
"cron":             "bafk2bzacebcec3lffmos3nawm5cvwehssxeqwxixoyyfvejy7viszzsxzyu26"
"datacap":          "bafk2bzacebb6uy2ys7tapekmtj7apnjg7oyj4ia5t7tlkvbmwtxwv74lb2pug"
"init":             "bafk2bzacebtdq4zyuxk2fzbdkva6kc4mx75mkbfmldplfntayhbl5wkqou33i"
"multisig":         "bafk2bzacec4va3nmugyqjqrs3lqyr2ij67jhjia5frvx7omnh2isha6abxzya"
"paymentchannel":   "bafk2bzacebhdvjbjcgupklddfavzef4e4gnkt3xk3rbmgfmk7xhecszhfxeds"
"reward":           "bafk2bzacebezgbbmcm2gbcqwisus5fjvpj7hhmu5ubd37phuku3hmkfulxm2o"
"storagemarket":    "bafk2bzacec3j7p6gklk64stax5px3xxd7hdtejaepnd4nw7s2adihde6emkcu"
"storageminer":     "bafk2bzacedyux5hlrildwutvvjdcsvjtwsoc5xnqdjl73ouiukgklekeuyfl4"
"storagepower":     "bafk2bzacedsetphfajgne4qy3vdrpyd6ekcmtfs2zkjut4r34cvnuoqemdrtw"
"system":           "bafk2bzaceagvlo2jtahj7dloshrmwfulrd6e2izqev32qm46eumf754weec6c"
"verifiedregistry": "bafk2bzacecf3yodlyudzukumehbuabgqljyhjt5ifiv4vetcfohnvsxzynwga"
```

## New Features
- Integrate actor v9:
  - test: Add invariance checks to v17 migration test ([filecoin-project/lotus#9454](https://github.com/filecoin-project/lotus/pull/9454))
- Implement and support [FIP0045 Decoupling Fil+ from Marketplace](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0045.md):
  - fix: state: add datacap actor to actors registry ([filecoin-project/lotus#9476](https://github.com/filecoin-project/lotus/pull/9476))
  - feat: actors: Integrate builtin-actors changes for FIP-0045 ([filecoin-project/lotus#9355](https://github.com/filecoin-project/lotus/pull/9355))
  - feat: actors: Integrate datacap actor into lotus (#9348) ([filecoin-project/lotus#9348](https://github.com/filecoin-project/lotus/pull/9348))
  - feat: cli: Add commands for listing allocations and removing expired allocations ([filecoin-project/lotus#9468](https://github.com/filecoin-project/lotus/pull/9468))
  - feat: sealing pipeline: Prepare deal assigning logic for FIP-45 ([filecoin-project/lotus#9412](https://github.com/filecoin-project/lotus/pull/9412))
  - feat: add API methods to get allocations and claims ([filecoin-project/lotus#9437](https://github.com/filecoin-project/lotus/pull/9437))
- Implement and support [FIP0029 Beneficiary Address for Storage Providers](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0029.md)
  - feat: api/cli: beneficiary withdraw api and cli #9296
  - feat: api/cli: change beneficiary propose and confirm for actors and multisigs. #9307
  
## Improvements
- feat: wdpost: Add ability to only have single partition per msg for partitions with recovery sectors ([filecoin-project/lotus#9427](https://github.com/filecoin-project/lotus/pull/9427))
- feat: API:  support typed errors over RPC ([filecoin-project/lotus#9061](https://github.com/filecoin-project/lotus/pull/9061))
- feat: refactor: remove NewestNetworkVersion ([filecoin-project/lotus#9351](https://github.com/filecoin-project/lotus/pull/9351))
- chore: actors: Allow builtin-actors to return a map of methods (#9342) ([filecoin-project/lotus#9342](https://github.com/filecoin-project/lotus/pull/9342))

## Dependencies
- Update FFI ([filecoin-project/lotus#9484](https://github.com/filecoin-project/lotus/pull/9484))
- chore: deps: update go-state-types and builtin-actors for v9 release ([filecoin-project/lotus#9485](https://github.com/filecoin-project/lotus/pull/9485))
- deps: backport: #9455 ([filecoin-project/lotus#9463](https://github.com/filecoin-project/lotus/pull/9463))
- Deps: Update go-fil-markets to 1.24.0-v17 ([filecoin-project/lotus#9450](https://github.com/filecoin-project/lotus/pull/9450))
- github.com/filecoin-project/go-jsonrpc (v0.1.7 -> v0.1.8)
- github.com/filecoin-project/go-state-types (v0.1.12-beta -> v0.9.0):

## Others
- fix: upgrade: no splash banner for nv17 :( ([filecoin-project/lotus#9486](https://github.com/filecoin-project/lotus/pull/9486))
- chore: build: add calib upgrade param for shark ([filecoin-project/lotus#9483](https://github.com/filecoin-project/lotus/pull/9483))
- chore: update butterfly artifacts ([filecoin-project/lotus#9467](https://github.com/filecoin-project/lotus/pull/9467))
- chore: butterfly: update assets ([filecoin-project/lotus#9462](https://github.com/filecoin-project/lotus/pull/9462))
- Delete lotus-pond (#9352) ([filecoin-project/lotus#9352](https://github.com/filecoin-project/lotus/pull/9352))
- build: set version to v1.18.0-dev

## lotus-market EOL notice 

As mentioned in [lotus v1.17.0 release notes](https://github.com/filecoin-project/lotus/releases/tag/v1.17.0), markets related features, enhancements and fixes is now lower priority for Lotus. We recommend our users to migrate to other deal making focused software, like [boost](https://boost.filecoin.io/) as soon as possible. That being said, the lotus maintainers will be:
-  Lotus maintainers will stop supporting lotus-market subcomponent/**storage** deal making related issues or enhancements on Jan 31, 2023.
- In Q2 2023, we will be deprecating/removing lotus-market related code from this repository.
If you have any questions or concerns, please raise them in [Lotus discussion](https://github.com/filecoin-project/lotus/discussions/categories/market)!


## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @geoff-vball | 73 | +14533/-19712 | 509 |
| @arajasek | 16 | +2230/-303 | 49 |
| @arajasek | 29 | +701/-297 | 117 |
| @magik6k | 5 | +429/-135 | 45 |
| @Frrist | 1 | +246/-203 | 25 |
| @stebalien | 2 | +323/-2 | 6 |
| @shrenujbansal  | 3 | +176/-61 | 10 |
| @ZenGround0 | 2 | +78/-38 | 5 |
| @jennijuju | 8 | +97/-18 | 16 |
| @simlecode | 5 | +18/-9 | 11 |
| Kevin Li | 1 | +7/-0 | 1 |
| @zenground0 | 2 | +3/-3 | 3 |
| @jennijuju | 1 | +3/-3 | 2 |
| Rod Vagg | 1 | +3/-2 | 2 |
| @jennijuju | 1 | +2/-2 | 2 |
| Peter Rabbitson | 1 | +3/-0 | 1 |
| Jakub Sztandera | 1 | +1/-1 | 1 |

# v1.17.2 / 2022-10-05

This is an OPTIONAL release of Lotus. This feature release introduces new sector number management APIs in Lotus that enables all the Sealing-as-a-Service and Lotus interactions needed to function. The default propagation delay setting for storage providers has also been changed, as well as numerous other features and enhancements. Check out the sub-bullet points in the feature and enhancement section to get a short description about each feature and enhancements.

### Highlights

🦭 **SaaS** 🦭
New sector management APIs makes it possible to import partially sealed sectors into Lotus. This release implements all SaaS<->Lotus interactions needed for such services to work. Deep dive into the new APIs here: https://github.com/filecoin-project/lotus/discussions/9079#discussioncomment-3652044

⏳ **Propagation delay** ⌛️
In v1.17.2 the default PropagationDelay has been raised from 6 seconds -> 10 seconds, and you can tune this yourself with the `PROPAGATION_DELAY_SECS` environment variable. This means you will now wait for 10 seconds for other blocks to arrive from the network before computing a winningPoSt (if eligible). In your `lotus-miner` logs that means you will see this "baseDeltaSeconds": 10 as default.

⚠️ **Please note that Lotus v1.17.2 will require a Go-version of v1.18.1 or higher!**

## New features
- feat: sealing: Partially sealed sector import ([filecoin-project/lotus#9210](https://github.com/filecoin-project/lotus/pull/9210))
  - Implements support for importing (partially) sealed sectors which is needed for enabling external sealing services.
- feat: sealing: Use bitfields to manage sector numbers ([filecoin-project/lotus#9183](https://github.com/filecoin-project/lotus/pull/9183))
  - Needed for Sealing-as-a-Service. Move sector number assigning logic to use stored bitfields instead of stored counters. Makes it possible to reserve ranges of sector numbers, see the sector assigner state and view sector number reservations.
- feat: env: propagation delay ([filecoin-project/lotus#9290](https://github.com/filecoin-project/lotus/pull/9290))
  - The default propagation delay is raised to 10 seconds from 6 seconds. Ability to set it yourself with the `PROPAGATION_DELAY_SECS` environment variable.
- feat: cli: lotus info cmd ([filecoin-project/lotus#9233](https://github.com/filecoin-project/lotus/pull/9233))
  - A new `lotus info` command that prints useful node information in one place.
- feat: proving: Introduce manual sector fault recovery (#9144) ([filecoin-project/lotus#9144](https://github.com/filecoin-project/lotus/pull/9144))
  - Allow users to declare fault recovery messages manually with the `lotus-miner proving recover-faults` command, rather than waiting for it to happen automatically before windowPost.
- feat: api: Reintroduce StateActorManifestCID ([filecoin-project/lotus#9201](https://github.com/filecoin-project/lotus/pull/9201))
  - Adds ability to retrieve the Actor Manifest CID through the api.
- feat: message: Add uuid to mpool message sent to chain node from miner ([filecoin-project/lotus#9174](https://github.com/filecoin-project/lotus/pull/9174))
  - Adds a UUID to each message sent by the `lotus-miner` to the daemon. A requirement needed for https://github.com/filecoin-project/lotus/issues/9130
- feat: message: Add retries to mpool push message from lotus-miner ([filecoin-project/lotus#9177](https://github.com/filecoin-project/lotus/pull/9177))
  - Retries to mpool push message API in case of unavailability of the lotus chain node.

**Network 17 related features :**
- feat: network: add nv17 and integrate the corresponding go state type ([filecoin-project/lotus#9267](https://github.com/filecoin-project/lotus/pull/9267))
- feat: cli: print beneficiary info in state miner-info ([filecoin-project/lotus#9308](https://github.com/filecoin-project/lotus/pull/9308))
- feat: api/cli: change beneficiary propose and confirm for actors and multisigs. ([filecoin-project/lotus#9307](https://github.com/filecoin-project/lotus/pull/9307))
- feat: api/cli: beneficiary withdraw api and cli ([filecoin-project/lotus#9296](https://github.com/filecoin-project/lotus/pull/9296))

## Enhancements
- feat: sectors renew --only-cc ([filecoin-project/lotus#9184](https://github.com/filecoin-project/lotus/pull/9184))
  - Exlude extending deal-related sectors with the `--only-cc` option when using the `lotus-miner sectors renew`
- feat: miner: display updated & update-cache for storage list ([filecoin-project/lotus#9323](https://github.com/filecoin-project/lotus/pull/9323))
  - Show amount of `updated` & `update-cache` sectors in each storage path in the `lotus-miner storage list` output
- feat: add descriptive errors to markets event handler ([filecoin-project/lotus#9326](https://github.com/filecoin-project/lotus/pull/9326))
  - More descriptive market error logs
- feat: cli: Add option to terminate sectors from worker address ([filecoin-project/lotus#9291](https://github.com/filecoin-project/lotus/pull/9291))
  - Adds a flag to allow either owner address or worker address to send terminate sectors message.
- fix: cli: actor-cids cli command now defaults to current network ([filecoin-project/lotus#9321](https://github.com/filecoin-project/lotus/pull/9321))
  - Makes the command defaults to the current network.
- fix: ux: Output bytes in `lotus client commP` cmd ([filecoin-project/lotus#9189](https://github.com/filecoin-project/lotus/pull/9189))
  - Adds an additional line that outputs bytes in the `lotus client commP` command.
- fix: sealing: Add information on what worker a job was assigned to in logs ([filecoin-project/lotus#9151](https://github.com/filecoin-project/lotus/pull/9151))
  - Adds the worker hostname into the assignment logs.
- refactor: sealing pipeline: Remove useless storage adapter code ([filecoin-project/lotus#9142](https://github.com/filecoin-project/lotus/pull/9142)
  - Remove proxy code in `storage/miner.go` / `storage/miner_sealing.go`, call the pipeline directly instead.

## Bug fixes
- fix: ffiwrapper: Close readers in AddPiece ([filecoin-project/lotus#9328](https://github.com/filecoin-project/lotus/pull/9328))
- fix: sealing: Drop unused PreCommitInfo from pipeline.SectorInfo ([filecoin-project/lotus#9325](https://github.com/filecoin-project/lotus/pull/9325))
- fix: cli: fix panic in `lotus-miner actor control list` ([filecoin-project/lotus#9241](https://github.com/filecoin-project/lotus/pull/9241))
- fix: sealing: Abort upgrades in sectors with no deals ([filecoin-project/lotus#9310](https://github.com/filecoin-project/lotus/pull/9310))
- fix: sealing: Make DataCid resource env vars make more sense ([filecoin-project/lotus#9231](https://github.com/filecoin-project/lotus/pull/9231))
- fix: cli: Option to specify --from msg sender ([filecoin-project/lotus#9237](https://github.com/filecoin-project/lotus/pull/9237))
- fix: ux: better ledger rejection error ([filecoin-project/lotus#9242](https://github.com/filecoin-project/lotus/pull/9242))
- fix: ux: msg receipt for actor withdrawal ([filecoin-project/lotus#9155](https://github.com/filecoin-project/lotus/pull/9155))
- fix: ux: exclude negative available balance from spendable amount ([filecoin-project/lotus#9182](https://github.com/filecoin-project/lotus/pull/9182))
- fix: sealing: Avoid panicking in handleUpdateActivating on startup ([filecoin-project/lotus#9331](https://github.com/filecoin-project/lotus/pull/9331))
- fix: api: DataCid - ensure reader is closed ([filecoin-project/lotus#9230](https://github.com/filecoin-project/lotus/pull/9230))
- fix: verifreg: serialize RmDcProposalID as int, not tuple ([filecoin-project/lotus#9206](https://github.com/filecoin-project/lotus/pull/9206))
- fix: api: Ignore uuid check for messages with uuid not set ([filecoin-project/lotus#9303](https://github.com/filecoin-project/lotus/pull/9303))
- fix: cgroupV1: memory.memsw.usage_in_bytes: no such file or directory ([filecoin-project/lotus#9202](https://github.com/filecoin-project/lotus/pull/9202))
- fix: miner: init miner's with 32GiB sectors by default ([filecoin-project/lotus#9364](https://github.com/filecoin-project/lotus/pull/9364))
- fix: worker: Close all storage paths on worker shutdown ([filecoin-project/lotus#9153](https://github.com/filecoin-project/lotus/pull/9153))
- fix: build: set PropagationDelaySecs correctly ([filecoin-project/lotus#9358](https://github.com/filecoin-project/lotus/pull/9358))
- fix: renew --only-cc with sectorfile ([filecoin-project/lotus#9428](https://github.com/filecoin-project/lotus/pull/9428))

## Dependency updates
- github.com/filecoin-project/go-fil-markets (v1.23.1 -> v1.24.0)
- github.com/filecoin-project/go-jsonrpc (v0.1.5 -> v0.1.7)
- github.com/filecoin-project/go-state-types (v0.1.10 -> v0.1.12-beta)
- github.com/filecoin-project/go-commp-utils/nonffi (null -> v0.0.0-20220905160352-62059082a837)
- deps: go-libp2p-pubsub v0.8.0 ([filecoin-project/lotus#9229](https://github.com/filecoin-project/lotus/pull/9229))
- deps: libp2p v0.22 ([filecoin-project/lotus#9216](https://github.com/filecoin-project/lotus/pull/9216))
- deps: Use latest cbor-gen ([filecoin-project/lotus#9335](https://github.com/filecoin-project/lotus/pull/9335))
- chore: update bitswap and some libp2p packages ([filecoin-project/lotus#9279](https://github.com/filecoin-project/lotus/pull/9279))

## Others
- chore: merge releases into master after v1.17.1 release ([filecoin-project/lotus#9283](https://github.com/filecoin-project/lotus/pull/9283))
- chore: docs: Fix dead links to docs.filecoin.io ([filecoin-project/lotus#9304](https://github.com/filecoin-project/lotus/pull/9304))
- chore: deps: update FFI ([filecoin-project/lotus#9330](https://github.com/filecoin-project/lotus/pull/9330))
- chore: seed: add cmd for adding signers to rkh to genesis ([filecoin-project/lotus#9198](https://github.com/filecoin-project/lotus/pull/9198))
- chore: fix typo in comment ([filecoin-project/lotus#9161](https://github.com/filecoin-project/lotus/pull/9161))
- chore: cli: cleanup and standardize cli ([filecoin-project/lotus#9317](https://github.com/filecoin-project/lotus/pull/9317))
- chore: versioning: Bump version to v1.17.2-dev ([filecoin-project/lotus#9147](https://github.com/filecoin-project/lotus/pull/9147))
- chore: release: v1.17.2-rc1 ([filecoin-project/lotus#9339](https://github.com/filecoin-project/lotus/pull/9339))
- feat: shed: add a --max-size flag to vlog2car ([filecoin-project/lotus#9212](https://github.com/filecoin-project/lotus/pull/9212))
- fix: docsgen: revert rename of API Name to Num ([filecoin-project/lotus#9315](https://github.com/filecoin-project/lotus/pull/9315))
- fix: ffi: Revert accidental filecoin-ffi downgrade from #9144 ([filecoin-project/lotus#9277](https://github.com/filecoin-project/lotus/pull/9277))
- fix: miner: Call SyncBasefeeCheck from `lotus info` ([filecoin-project/lotus#9281](https://github.com/filecoin-project/lotus/pull/9281))
- fix: mock sealer: grab lock in ReadPiece ([filecoin-project/lotus#9207](https://github.com/filecoin-project/lotus/pull/9207))
- refactor: use `os.ReadDir` for lightweight directory reading ([filecoin-project/lotus#9282](https://github.com/filecoin-project/lotus/pull/9282))
- tests: cli: Don't panic with no providers in client retrieve ([filecoin-project/lotus#9232](https://github.com/filecoin-project/lotus/pull/9232))
- build: artifacts: butterfly ([filecoin-project/lotus#9027](https://github.com/filecoin-project/lotus/pull/9027))
- build: Use lotus snap (and fix typo) for packer builds ([filecoin-project/lotus#9152](https://github.com/filecoin-project/lotus/pull/9152))
- build: Update xcode version for macos builds ([filecoin-project/lotus#9170](https://github.com/filecoin-project/lotus/pull/9170))
- ci: build: Snap daemon autorun disable ([filecoin-project/lotus#9167](https://github.com/filecoin-project/lotus/pull/9167))
- ci: Use golang 1.18.1 to build appimage ([filecoin-project/lotus#9389](https://github.com/filecoin-project/lotus/pull/9389))
- ci: Don't publish new homebrew releases for RC builds ([filecoin-project/lotus#9350](https://github.com/filecoin-project/lotus/pull/9350))
- Merge branch 'deps/go-libp2p-v0.21'

Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Aayush Rajasekaran | 8 | +23010/-2122 | 109 |
| Aayush | 15 | +6168/-2679 | 360 |
| Łukasz Magiera | 69 | +6462/-2137 | 606 |
| Geoff Stuart | 19 | +3080/-1177 | 342 |
| Marco Munizaga | 16 | +543/-424 | 41 |
| Shrenuj Bansal | 30 | +485/-419 | 88 |
| LexLuthr | 3 | +498/-12 | 19 |
| Phi | 15 | +330/-70 | 17 |
| Jennifer Wang | 7 | +132/-12 | 11 |
| TippyFlitsUK | 1 | +43/-45 | 12 |
| Steven Allen | 1 | +18/-28 | 2 |
| Frrist | 1 | +19/-11 | 2 |
| Eng Zer Jun | 1 | +14/-11 | 6 |
| Dirk McCormick | 2 | +23/-1 | 3 |
| Ian Davis | 3 | +7/-9 | 3 |
| Masih H. Derkani | 1 | +11/-0 | 1 |
| Anton Evangelatov | 1 | +11/-0 | 1 |
| Yu | 2 | +4/-4 | 5 |
| Hannah Howard | 1 | +4/-4 | 1 |
| Phi-rjan | 1 | +1/-2 | 1 |
| Jiaying Wang | 1 | +3/-0 | 1 |
| nujz | 1 | +1/-1 | 1 |
| Rob Quist | 1 | +1/-1 | 1 |

# v1.17.1 / 2022-09-06

This is an optional release of Lotus. This release introduces the [Splitstore v2 - beta](https://github.com/filecoin-project/lotus/blob/master/blockstore/splitstore/README.md)(beta). Splitstore aims to reduce the node performance impact that's caused by the Filecoin's very large, and continuously growing datastore. Splitstore v2 introduces  the coldstore auto prune/GC feature & some improvements for the hotstore.  We welcome all lotus users to join the early testers and try the new Splitstore out, you can leave any feedback or report issues in [this discussion](https://github.com/filecoin-project/lotus/discussions/9179) or create an issue. As always,  multiple small bug fixes, new features & improvements are also included in this release.


## New features

- feat:chain:splitstore auto prune ([filecoin-project/lotus#9123](https://github.com/filecoin-project/lotus/pull/9123))
  - Trigger SplitStore chain prune on head events. [Link to the documentation](https://lotus.filecoin.io/lotus/manage/chain-management/#cold-store-garbage-collection)
- feat:chain:splitstore chain prune ([filecoin-project/lotus#9056](https://github.com/filecoin-project/lotus/pull/9056))
  - Adds `chain prune` command to trigger manual garbage collection. [Link to the documentation](https://lotus.filecoin.io/lotus/manage/chain-management/#cold-store-garbage-collection)
- feat: storage: Path type filters ([filecoin-project/lotus#9013](https://github.com/filecoin-project/lotus/pull/9013))
  - Adds new fields to `sectorstore.json` to allow file type filtering. [Link to the documentation](https://lotus.filecoin.io/storage-providers/operate/custom-storage-layout/#filter-sector-types-1)
- feat: sealing: storage redeclare/detach ([filecoin-project/lotus#9032](https://github.com/filecoin-project/lotus/pull/9032))
  - Adds new Lotus commands to detach and redeclare storage paths. [Link to the documentation](https://lotus.filecoin.io/storage-providers/operate/custom-storage-layout/#detach-storage-paths)
- feat: worker: Add stop cmd for lotus worker ([filecoin-project/lotus#9101](https://github.com/filecoin-project/lotus/pull/9101))
  - Adds new `lotus-worker stop` command. [Link to the documentation](https://lotus.filecoin.io/storage-providers/seal-workers/seal-workers/#stop-the-worker)
- feat: market: Add lotus-shed cmd to get total active deal storage ([filecoin-project/lotus#9113](https://github.com/filecoin-project/lotus/pull/9113))
  - `get-deals-total-storage` - View the total storage available in all active market deals
- feat: wdpost: Envvar for limiting recovering sectors ([filecoin-project/lotus#9106](https://github.com/filecoin-project/lotus/pull/9106))
  - Adds new envvar to limit the number of sectors declared in the recover message


## Improvements

- feat: sealing: Allow overriding worker hostname ([filecoin-project/lotus#9116](https://github.com/filecoin-project/lotus/pull/9116))
- feat: build: run fiximports on make actors-gen ([filecoin-project/lotus#9114](https://github.com/filecoin-project/lotus/pull/9114))
- feat: FVM: always enable tracing for user-triggered executions ([filecoin-project/lotus#9036](https://github.com/filecoin-project/lotus/pull/9036))
- feat: miner cli: proving deadline command enchantments ([filecoin-project/lotus#9109](https://github.com/filecoin-project/lotus/pull/9109))
- FVM: Use MaxInt64 for Implicit Message gas limits ([filecoin-project/lotus#9037](https://github.com/filecoin-project/lotus/pull/9037))
- lotus shed addr decode
- push lotus-gateway to docker hub ([filecoin-project/lotus#8969](https://github.com/filecoin-project/lotus/pull/8969))
- Review Response
- test: net: net and conngater tests ([filecoin-project/lotus#8084](https://github.com/filecoin-project/lotus/pull/8084))
- Update FFI ([filecoin-project/lotus#9139](https://github.com/filecoin-project/lotus/pull/9139))

## Bug Fixes

- backport: 9153: detach storage on worker shutdown ([filecoin-project/lotus#9127](https://github.com/filecoin-project/lotus/pull/9165))
- fix makegen
- fix: build: use GOCC when building lotus-fountain ([filecoin-project/lotus#9127](https://github.com/filecoin-project/lotus/pull/9127))
- fix: ci: Forgot a .sh on the end of a the new publish script ([filecoin-project/lotus#9088](https://github.com/filecoin-project/lotus/pull/9088))
- fix: cli: ./lotus-miner actor control list, if the owner is not account ([filecoin-project/lotus#9072](https://github.com/filecoin-project/lotus/pull/9072))
- fix: deps: update FFI to fix a slow memory leak ([filecoin-project/lotus#9042](https://github.com/filecoin-project/lotus/pull/9042))
- fix: FVM: record message applied metrics ([filecoin-project/lotus#9052](https://github.com/filecoin-project/lotus/pull/9052))
- fix: gas: estimate gas with a zero base-fee ([filecoin-project/lotus#8991](https://github.com/filecoin-project/lotus/pull/8991))
- fix: post: restrict recoveries per deadline ([filecoin-project/lotus#9111](https://github.com/filecoin-project/lotus/pull/9111))
- fix: sealing: Workaround for sealing bug ([filecoin-project/lotus#9043](https://github.com/filecoin-project/lotus/pull/9043))
- fix: storage: don't panic in getCommitCutoff when precommit is not found ([filecoin-project/lotus#9141](https://github.com/filecoin-project/lotus/pull/9141))
- fix: test: deflake TestQuotePriceForUnsealedRetrieval ([filecoin-project/lotus#9084](https://github.com/filecoin-project/lotus/pull/9084))

## Dependency Updates

- github.com/multiformats/go-multibase (v0.0.3 -> v0.1.1)

## Others

- chore: ci: Update xcode version for macos builds ([filecoin-project/lotus#9164)](https://github.com/filecoin-project/lotus/pull/9164))
- Merge branch 'docs/ysrotciv-desc'
- Merge branch 'feat/f8-worker-env'
- Merge branch 'LexLuthr-feat/minerWithdrawBalanceAPI'
- Merge branch 'LexLuthr-feat/SchedRemoveRequest'
- base256emoji ([filecoin-project/lotus#9038)](https://github.com/filecoin-project/lotus/pull/9038))
- chore: interop: update interop assets ([filecoin-project/lotus#9093)](https://github.com/filecoin-project/lotus/pull/9093))
- chore: merge: releases (v1.17.0) to master ([filecoin-project/lotus#9146)](https://github.com/filecoin-project/lotus/pull/9146))
- chore: sealer: Fixup typos ([filecoin-project/lotus#9040)](https://github.com/filecoin-project/lotus/pull/9040))
- chore:docs:remove readme reference to deprecated specs-actors ([filecoin-project/lotus#8984)](https://github.com/filecoin-project/lotus/pull/8984))
- ci : Change default shell options for snapcraft publish ([filecoin-project/lotus#9122)](https://github.com/filecoin-project/lotus/pull/9122))
- ci: More tweaks to snapcraft release process ([filecoin-project/lotus#9090)](https://github.com/filecoin-project/lotus/pull/9090))
- ci: Publish to both lotus and lotus-filecoin for snap ([filecoin-project/lotus#9119)](https://github.com/filecoin-project/lotus/pull/9119))
- ci: Run snap builds for lotus and lotus-filecoin in parallel ([filecoin-project/lotus#9133)](https://github.com/filecoin-project/lotus/pull/9133))
- ci: Switches goreleaser notes back to default (keep-existing) ([filecoin-project/lotus#9120)](https://github.com/filecoin-project/lotus/pull/9120))
- ci: update snapcraft and release flow logic ([filecoin-project/lotus#8994)](https://github.com/filecoin-project/lotus/pull/8994))
- ci: Use goreleaser to build macos universal binaries (including M1 macs) ([filecoin-project/lotus#9096)](https://github.com/filecoin-project/lotus/pull/9096))
- ci:testing:remove codecov ([filecoin-project/lotus#9062)](https://github.com/filecoin-project/lotus/pull/9062))


Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Łukasz Magiera | 34 | +2329/-317 | 163 |
| ZenGround0 | 2 | +1527/-89 | 38 |
| Ian Davis | 14 | +751/-232 | 30 |
| LexLuthr | 17 | +480/-225 | 63 |
| TheMenko | 4 | +323/-61 | 5 |
| Aayush | 10 | +285/-92 | 30 |
| beck | 3 | +143/-93 | 3 |
| Steven Allen | 4 | +95/-75 | 9 |
| zenground0 | 5 | +44/-116 | 9 |
| Shrenuj Bansal | 7 | +136/-7 | 16 |
| Patrick Deuse | 3 | +76/-57 | 3 |
| Jennifer Wang | 3 | +6/-52 | 11 |
| zl | 2 | +20/-16 | 2 |
| Aayush Rajasekaran | 2 | +6/-6 | 2 |
| Clint Armstrong | 1 | +7/-3 | 1 |
| Cory Schwartz | 2 | +9/-0 | 2 |
| Jorropo | 1 | +3/-2 | 2 |
| Geoff Stuart | 1 | +5/-0 | 1 |
| Frank Y | 1 | +2/-2 | 2 |
| Aloxaf | 1 | +2/-2 | 1 |

# v1.17.0 / 2022-08-02

This is an optional release of Lotus. This feature release introduces a lot of new sealing and scheduler improvements, and many other functionalities and bug fixes.

PSA: Markets related features, enhancements and fixes is now lower priority for Lotus, and is going to be in the hands of [Boost](https://boost.filecoin.io), built by the amazing [Bedrock team](https://www.notion.so/pl-strflt/Bedrock-2e956d5d8143432080a1d84435ccf0ff). You can find Lotus mission scope [here](https://www.notion.so/Lotus-8352bbb6c321431abd8790a7a3401ed3#805c645f592840ad893c272723362d3d)

## New features

- feat: worker: lotus-worker run --no-default ([filecoin-project/lotus#8672](https://github.com/filecoin-project/lotus/pull/8672))
  - Makes it very easy to spin up workers that make the compute tasks opt-in, instead of the default opt-out. Also makes it very easy to create storage-only workers. [Link to the documentation](https://lotus.filecoin.io/storage-providers/seal-workers/seal-workers/#run-the-worker)
- feat: sched: Per worker concurrent task count limits ([filecoin-project/lotus#8725](https://github.com/filecoin-project/lotus/pull/8725))
  - Set the maximum number of tasks running it parallel on workers by exporting env-variables: `[short task type]_[sector size]_MAX_CONCURRENT=[limit]`. [Link to documentation](https://lotus.filecoin.io/storage-providers/seal-workers/seal-workers/#limit-tasks-run-in-parallel)
- feat: sched: Finalize* move selectors ([filecoin-project/lotus#8710](https://github.com/filecoin-project/lotus/pull/8710))
  - Allows you to force all Finalize tasks to run on workers with local access to both long-term storage and the sealing path containing the sector.
- feat: sched: Add scheduler interfaces, configurable assigner ([filecoin-project/lotus#8700](https://github.com/filecoin-project/lotus/pull/8700)) 
  - Introduce a new simpler worker assigning logic which will attempt to assign tasks to as many workers as possible and ignore worker utilization. [Link to documentation](https://lotus.filecoin.io/storage-providers/advanced-configurations/sealing/#worker-assigning-logic)
- feat: bench: simple sealing operations commands ([filecoin-project/lotus#8373](https://github.com/filecoin-project/lotus/pull/8373))
  - Allows you to only test the performance of a single task. [Read the documentation](https://lotus.filecoin.io/storage-providers/operate/benchmarks/#single-task-benchmark).
- feat: miner cli: sealing data-cid command ([filecoin-project/lotus#8715](https://github.com/filecoin-project/lotus/pull/8715))
  - Makes it possible to compute data CIDs on lotus workers.
- feat: precommits info ([filecoin-project/lotus#8696](https://github.com/filecoin-project/lotus/pull/8696))
  - Check the on-chain precommit info with the `lotus-miner sectors precommits` command.
- feat: dagstore: add dagstore register-shard command ([filecoin-project/lotus#8645](https://github.com/filecoin-project/lotus/pull/8645))
  - If you have broken indexes in your market’s dagstore, you can try to recover it by using `lotus-miner dagstore register-shard`. [Link to documentation](https://lotus.filecoin.io/kb/dagstore-register/)
- feat: Implement cli command for compactPartitions ([filecoin-project/lotus#8637](https://github.com/filecoin-project/lotus/pull/8637))
  - Remove dead sectors from partitions and reduce the number of partitions used if possible with the `lotus-miner sectors compact-partitions`command. [Link to documentation](https://lotus.filecoin.io/storage-providers/operate/daily-chores/#compacting-partitions)
- feat: recovery: Config for maximum partition count per message ([filecoin-project/lotus#8986](https://github.com/filecoin-project/lotus/pull/8986)
  - Adds config for setting the maximum amount of partitions to declare in a DeclareFaultsRecovered message [Link to documentation](https://lotus.filecoin.io/storage-providers/seal-workers/post-workers/#multiple-partitions)
- feat: wdpost: Config for maximum partition count per message ([filecoin-project/lotus#8982](https://github.com/filecoin-project/lotus/pull/8982))
  - Adds config for setting the maximum amount of partitions to prove in a SubmitWindowPoSt message [Link to documentation](https://lotus.filecoin.io/storage-providers/seal-workers/post-workers/#multiple-partitions)
- feat: sealer: Config for disabling builtin PoSt / PoSt pre-checks ([filecoin-project/lotus#8959](https://github.com/filecoin-project/lotus/pull/8959))
  - Adds the ability to fully disable PoSt tasks on the `lotus-miner` and disabling windowPoSt pre-checks. [Link to documentation](https://lotus.filecoin.io/storage-providers/advanced-configurations/workers/#post-computations)
- feat: add create ledger wallet address by account index command ([filecoin-project/lotus#8657](https://github.com/filecoin-project/lotus/pull/8657))
 
## Improvements

- feat: wdpost: Ignore faults in lotus-miner proving compute window-post ([filecoin-project/lotus#8737](https://github.com/filecoin-project/lotus/pull/8737))
- feat: cli: Nicer net stat ([filecoin-project/lotus#8797](https://github.com/filecoin-project/lotus/pull/8797))
- feat: networking: add healthz and livez endpoints ([filecoin-project/lotus#8692](https://github.com/filecoin-project/lotus/pull/8692))
- feat: Snap Deals full unseal ([filecoin-project/lotus#8478](https://github.com/filecoin-project/lotus/pull/8478))
- feat: cli: Hide sector nums in 'proving deadline' by default ([filecoin-project/lotus#8952](https://github.com/filecoin-project/lotus/pull/8952))
- feat: Add rate limiting to the lotus gateway ([filecoin-project/lotus#8517](https://github.com/filecoin-project/lotus/pull/8517))
- feat: networking: disconnect cmd ([filecoin-project/lotus#8955](https://github.com/filecoin-project/lotus/pull/8955))
- feat: dagstore: Add DestroyShard() in dagstore wrapper ([filecoin-project/lotus#9010](https://github.com/filecoin-project/lotus/pull/9010))
- feat: shed: report the "user version" ([filecoin-project/lotus#8864](https://github.com/filecoin-project/lotus/pull/8864))
- feat: lotus-shed get remote peer hello message ([filecoin-project/lotus#8787](https://github.com/filecoin-project/lotus/pull/8787))
- feat: only enable rcmgr by default in full nodes ([filecoin-project/lotus#8769](https://github.com/filecoin-project/lotus/pull/8769))
- feat: refactor: actor bundling system (#8838) ([filecoin-project/lotus#8838](https://github.com/filecoin-project/lotus/pull/8838))
- feat: migration: Implement function to migrate actors with only code changes ([filecoin-project/lotus#8843](https://github.com/filecoin-project/lotus/pull/8843))
- feat: conformance & tvx: support ReportConsensusFault messages ([filecoin-project/lotus#8302](https://github.com/filecoin-project/lotus/pull/8302))
- feat: wdpost: Envvar for limiting recovering sectors ([filecoin-project/lotus#9106](https://github.com/filecoin-project/lotus/pull/9106))
- fix: post: restrict recoveries per deadline ([filecoin-project/lotus#9111](https://github.com/filecoin-project/lotus/pull/9111))
- ux: print absolute time for proving period start in proving cli ([filecoin-project/lotus#8954](https://github.com/filecoin-project/lotus/pull/8954))
- chore: storage refactors part 1 ([filecoin-project/lotus#8858](https://github.com/filecoin-project/lotus/pull/8858))
- chore: sealing pipeline: Remove adapter code (storage refactors part 2) ([filecoin-project/lotus#8871](https://github.com/filecoin-project/lotus/pull/8871))
- refactor: remove old BlockSyncProtocolID ([filecoin-project/lotus#8820](https://github.com/filecoin-project/lotus/pull/8820))

## Bug Fixes

- fix: format error log ([filecoin-project/lotus#8854](https://github.com/filecoin-project/lotus/pull/8854))
- fix: build: really make macos compatible (#8853) ([filecoin-project/lotus#8853](https://github.com/filecoin-project/lotus/pull/8853))
- fix: build: fix pack script and add calibrationnet to bundle ([filecoin-project/lotus#8852](https://github.com/filecoin-project/lotus/pull/8852))
- fix: build: fix 2k build params ([filecoin-project/lotus#8835](https://github.com/filecoin-project/lotus/pull/8835))
- Fix: PaychGetRestartAfterAddFundsMsg may stuck in forever waiting ([filecoin-project/lotus#8829](https://github.com/filecoin-project/lotus/pull/8829))
- fix: paych: Print "waiting for confirmation.." ([filecoin-project/lotus#8823](https://github.com/filecoin-project/lotus/pull/8823))
- fix: build: genesis miner network version ([filecoin-project/lotus#8756](https://github.com/filecoin-project/lotus/pull/8756))
- fix: bench: consistency in description ([filecoin-project/lotus#8777](https://github.com/filecoin-project/lotus/pull/8777))
- fix: worker: don't log normal storage stat calls ([filecoin-project/lotus#8744](https://github.com/filecoin-project/lotus/pull/8744))
- fix: worker: don't check params with --no-default when not needed ([filecoin-project/lotus#8741](https://github.com/filecoin-project/lotus/pull/8741))
- fix: Deduplicate parallel stat requests ([filecoin-project/lotus#8589](https://github.com/filecoin-project/lotus/pull/8589))
- fix: Delegate storage auth on market nodes ([filecoin-project/lotus#8978](https://github.com/filecoin-project/lotus/pull/8978))
- fix: post workers: check proving params on startup ([filecoin-project/lotus#8736](https://github.com/filecoin-project/lotus/pull/8736))
- fix: rpc: readd rpc.discover aliases; lotus-gateway openrpc ([filecoin-project/lotus#8738](https://github.com/filecoin-project/lotus/pull/8738))
- fix: change 1475 bootstrap peer ([filecoin-project/lotus#9008](https://github.com/filecoin-project/lotus/pull/9008))
- fix: verifreg: update deprecation log ([filecoin-project/lotus#8690](https://github.com/filecoin-project/lotus/pull/8690))
- fix: vm: support raw blocks in chain export ([filecoin-project/lotus#8691](https://github.com/filecoin-project/lotus/pull/8691))
- fix: deps: restore butterfly network genesis from v1.14.4 ([filecoin-project/lotus#8708](https://github.com/filecoin-project/lotus/pull/8708))
- fix: improve error message when maxPrice is too low ([filecoin-project/lotus#8818](https://github.com/filecoin-project/lotus/pull/8818))
- fix: msig cli: Check for existing signers in add-propose ([filecoin-project/lotus#8833](https://github.com/filecoin-project/lotus/pull/8833))
- Add new proofs version type ([filecoin-project/lotus#8848](https://github.com/filecoin-project/lotus/pull/8848))
- fix: Make cli deal command get Block Delay specific to build ([filecoin-project/lotus#8896](https://github.com/filecoin-project/lotus/pull/8896))
- fix: cli: Break out of retrieval if provider cancels ([filecoin-project/lotus#8912](https://github.com/filecoin-project/lotus/pull/8912))
- fix: appimage build ([filecoin-project/lotus#8931](https://github.com/filecoin-project/lotus/pull/8931))
- fix: incorrect usage of peer.IDFromString (should be peer.Decode) ([filecoin-project/lotus#8993](https://github.com/filecoin-project/lotus/pull/8993))
- fix: deps: update FFI to fix a slow memory leak ([filecoin-project/lotus#9044](https://github.com/filecoin-project/lotus/pull/9044))
- fix: sealing: hacky sealing fix backport ([filecoin-project/lotus#9055](https://github.com/filecoin-project/lotus/pull/9055))

## Dependency Updates

- github.com/filecoin-project/go-address (v0.0.6 -> v1.0.0)
- github.com/filecoin-project/go-fil-markets (v1.20.1 -> v1.23.1)
- github.com/filecoin-project/go-indexer-core (v0.2.8 -> v0.2.9)
- github.com/filecoin-project/go-data-transfer (v1.15.1 -> v1.15.2)
- github.com/filecoin-project/go-legs (v0.3.7 -> v0.4.4)
- github.com/filecoin-project/go-state-types (v0.1.8 -> v0.1.10)
- github.com/filecoin-project/index-provider (v0.5.0 -> v0.8.1)
- github.com/filecoin-project/specs-actors (v0.9.14 -> v0.9.15)
- github.com/filecoin-project/specs-actors/v3 (v3.1.1 -> v3.1.2)
- github.com/filecoin-project/specs-actors/v4 (v4.0.1 -> v4.0.2)
- github.com/filecoin-project/specs-actors/v5 (v5.0.4 -> v5.0.6)
- github.com/filecoin-project/specs-actors/v6 (v6.0.1 -> v6.0.2)
- github.com/filecoin-project/specs-actors/v7 (v7.0.0 -> v7.0.1)
- github.com/filecoin-project/specs-actors/v8 (null -> v8.0.1)
- github.com/filecoin-project/specs-storage (v0.2.4 -> v0.4.1)
- github.com/filecoin-project/storetheindex (v0.3.5 -> v0.4.17)
- deps: libp2p: update to the latest golibp2p tag ([filecoin-project/lotus#8704](https://github.com/filecoin-project/lotus/pull/8704))
- chore: update and fix libp2p ([filecoin-project/lotus#8996](https://github.com/filecoin-project/lotus/pull/8996))

## Others

- chore: fix imports conflict ([filecoin-project/lotus#8863](https://github.com/filecoin-project/lotus/pull/8863))
- chore: Fix imports ([filecoin-project/lotus#8859](https://github.com/filecoin-project/lotus/pull/8859))
- chore: backport: release v1.16.0 back to master ([filecoin-project/lotus#8855](https://github.com/filecoin-project/lotus/pull/8855))
- chore: bundle: remove wrongly committed bundle cars ([filecoin-project/lotus#8762](https://github.com/filecoin-project/lotus/pull/8762))
- docs:sealing:fix default miner config comments ([filecoin-project/lotus#8689](https://github.com/filecoin-project/lotus/pull/8689))
- ci: deps: Use testground-github-action from testground org ([filecoin-project/lotus#8490](https://github.com/filecoin-project/lotus/pull/8490))
 
Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Masih H. Derkani | 153 | +15515/-16832 | 660 |
| Łukasz Magiera | 92 | +10429/-8024 | 1580 |
| Andrew Gillis | 43 | +4149/-1765 | 208 |
| Jennifer Wang | 10 | +1441/-1138 | 34 |
| Geoff Stuart | 18 | +1348/-859 | 113 |
| dirkmc | 11 | +1827/-210 | 70 |
| Aayush | 21 | +1134/-894 | 69 |
| Steven Allen | 9 | +743/-889 | 66 |
| Marco Munizaga | 15 | +990/-252 | 36 |
| gammazero | 47 | +681/-411 | 104 |
| Will | 4 | +514/-246 | 29 |
| web3-bot | 15 | +409/-348 | 20 |
| Steven Fraser | 1 | +671/-0 | 36 |
| Cory Schwartz | 27 | +520/-89 | 36 |
| Hannah Howard | 3 | +318/-105 | 8 |
| Piotr Galar | 2 | +337/-59 | 7 |
| swift-mx | 14 | +264/-131 | 17 |
| vyzo | 7 | +357/-15 | 16 |
| Petar Maymounkov | 6 | +221/-23 | 14 |
| LexLuthr | 7 | +182/-21 | 14 |
| Aayush Rajasekaran | 5 | +97/-70 | 33 |
| Raúl Kripalani | 5 | +87/-45 | 7 |
| unknown | 1 | +114/-0 | 8 |
| sti-bot | 44 | +54/-60 | 44 |
| Aarsh Shah | 2 | +61/-50 | 8 |
| Lucas Molas | 1 | +74/-27 | 3 |
| zenground0 | 8 | +80/-18 | 14 |
| Dirk McCormick | 3 | +52/-33 | 8 |
| frank | 3 | +73/-7 | 3 |
| Will Scott | 4 | +45/-11 | 5 |
| kaola526 | 5 | +44/-11 | 5 |
| dependabot[bot] | 3 | +16/-10 | 8 |
| zl | 1 | +15/-4 | 4 |
| Phi | 5 | +12/-6 | 6 |
| Marcin Rataj | 1 | +11/-7 | 1 |
| github-actions[bot] | 7 | +8/-8 | 7 |
| Anton Evangelatov | 2 | +13/-0 | 4 |
| Nicolas Gimenez | 1 | +12/-0 | 1 |
| Marten Seemann | 2 | +5/-7 | 5 |
| Chris Harden | 1 | +10/-0 | 2 |
| jennijuju | 1 | +4/-4 | 7 |
| Travis Person | 2 | +2/-6 | 2 |
| Rod Vagg | 1 | +3/-3 | 2 |
| Rob Quist | 1 | +3/-3 | 1 |
| Jiaying Wang | 1 | +2/-3 | 2 |
| zengroundumbass | 1 | +3/-1 | 1 |
| lifei | 1 | +1/-1 | 1 |
| Mike | 1 | +2/-0 | 1 |
| Hubert | 1 | +1/-1 | 1 |
| Daniel N | 1 | +1/-1 | 1 |
| BMZ | 1 | +1/-1 | 1 |

# 1.16.1 / 2022-07-07

This is an OPTIONAL PATCH releases for storage providers who have failed to publish `SubmitWindowedPoSt` due to out of gas error. The error log looks like `/wdpost_run.go:xxx	estimating gas	{"error": "estimating gas used: message execution failed: exit SysErrOutOfGas(7)...`.

## New Features

- feat: declare fault recovery: Config for maximum partition count per message (#8988 / #8986)
  - configure `MaxPartitionsPerRecoveryMessage` in miner configuration setting.
- feat: wdpost: Config for maximum partition count per message (#8982 / #8986)
  - configure `MaxPartitionsPerPoStMessage` in miner configuration setting.
  
# 1.16.0 / 2022-06-24

This is a MANDATORY release of Lotus that introduces [Filecoin network v16,
codenamed the Skyr upgrade](https://github.com/filecoin-project/community/discussions/74?sort=new#discussioncomment-2392151).

The network is scheduled to upgrade to nv16 at epoch 1960320, on July 6th at 2022-07-06T14:00:00Z. All node operators, including storage providers, must upgrade to this release (or a later release) before that time. Storage providers must update their daemons, miners, market and worker(s).

Your lotus node will switch from the Legacy VM (that depended on go-based specs-actors) to Filecoin Virtual Machine FVM (that uses Rust-based builtin-actors) atomically upon the upgrade.

The Skyr upgrade introduces the following FIPs, enhancements and bug fixes, delivered in [built-actors v8](https://github.com/filecoin-project/builtin-actors/releases/tag/v8.0.0) and [ref-fvm v1.0.0](https://github.com/filecoin-project/ref-fvm/releases/tag/fvm%40v1.0.0):,

- [FIP-0030](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0030.md)
- [FIP-0031](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0031.md)
- [FIP-0032](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0032.md))
- [FIP-0027](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0027.md)
- [Bug fix for the ProveReplicaUpdates method](https://github.com/filecoin-project/builtin-actors/pull/138)
- [New proofs version for SnarkPack](https://github.com/filecoin-project/builtin-actors/pull/474/commits/3027c365f516e1cba6f156d4fb9dbd8c893d5b62)


## 🆕 Things you may wanna know

### Actor Code CIDs

As stated in [FIP-0031](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0031.md)- [structure of the code cid](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0031.md#structure-of-code-cids), system actors' code CIDs will be real content-addressing

For lotus users, we are making the change minimal for you. This means the `CODE` output when you run `lotus state get-actor` will now be the actual CID that represents the executable code for the actor, followed by wrapped synthetic id like the ones you've got before, i.e `fil/8/system`.

Moreover, this also means that in the future, whenever the actor code changes, the CID changes accordingly, and a network upgrade is needed for the network participants to have consensus over what executable code we should use for each system actor.

### Built-in actor bundles

As the network introduces FVM, it's also switching from spec-actor (written in GoLang) to [built-in actor](https://github.com/filecoin-project/builtin-actors) (written in rust), in which the latter comes with[ importable bundles](https://github.com/filecoin-project/builtin-actors#importable-bundle). This means, like filecoin proof parameters, node operators now also need to fetch the actor bundles according to the network versions for the nodes to remain operational.

Bundles for all networks(mainnet, calibnet, and etc) are included in the lotus source tree (`build/actors/`) and embedded on build. Lotus verifies that the bundle CIDs are the right ones upon build & upgrade against the values in `build/builtin_actors_gen.go`, according to the network you are building. You may also check the bundle manifest CID matches the bundle gen-ed values by running `lotus state actor-cids --network-version 16`.

The manifest CID & full list of actor code CIDs for nv16 using v8.0.0 is:

```
    "_manifest":         "bafy2bzacebogjbpiemi7npzxchgcjjki3tfxon4ims55obfyfleqntteljsea"
	"account":          "bafk2bzacedudbf7fc5va57t3tmo63snmt3en4iaidv4vo3qlyacbxaa6hlx6y"
	"cron":             "bafk2bzacecqb3eolfurehny6yp7tgmapib4ocazo5ilkopjce2c7wc2bcec62"
	"init":             "bafk2bzaceaipvjhoxmtofsnv3aj6gj5ida4afdrxa4ewku2hfipdlxpaektlw"
	"multisig":         "bafk2bzacebhldfjuy4o5v7amrhp5p2gzv2qo5275jut4adnbyp56fxkwy5fag"
	"paymentchannel":   "bafk2bzacebalad3f72wyk7qyilvfjijcwubdspytnyzlrhvn73254gqis44rq"
	"reward":           "bafk2bzacecwzzxlgjiavnc3545cqqil3cmq4hgpvfp2crguxy2pl5ybusfsbe"
	"storagemarket":    "bafk2bzacediohrxkp2fbsl4yj4jlupjdkgsiwqb4zuezvinhdo2j5hrxco62q"
	"storageminer":     "bafk2bzacecgnynvd3tene3bvqoknuspit56canij5bpra6wl4mrq2mxxwriyu"
	"storagepower":     "bafk2bzacebjvqva6ppvysn5xpmiqcdfelwbbcxmghx5ww6hr37cgred6dyrpm"
	"system":           "bafk2bzacedwq5uppsw7vp55zpj7jdieizirmldceehu6wvombw3ixq2tcq57w"
	"verifiedregistry": "bafk2bzaceb3zbkjz3auizmoln2unmxep7dyfcmsre64vnqfhdyh7rkqfoxlw4"
```

All bundles are also available at https://github.com/filecoin-project/builtin-actors/releases, thus you can also manually download the bundles and place them in the right path.

Note: use customized bundle will risk you to lose sync with the network!

To get Code CIDs:
- api:`StateActorCodeCIDs`
- cli: `lotus state actor-cids`
- cli: `lotus-shed cid inspect-bundle`

### Execution Trace

For developers that are dependent on lotus execution trace, you will need  to enable `LOTUS_VM_ENABLE_TRACING` envvar to get the exact execution trace response as before. Without the envvar enabled, `Subcall` details, Duration` and `GasCharges` fields will be missing from the new FVM trace.

### Deal Proposal Migration

All deal proposals with non-utf8 string as the label in the metadata store will perform a light migration to new format as defined in [v1.1.1](https://github.com/filecoin-project/go-fil-markets/pull/721).

## Others
- Resource manager is now only enabled by default on full daemon node. You can enable it manually for other nodes by setting env var `LOTUS_RCMGR` to `1`.
- Fix: drand: calculation of round from Filecoin epochs ([filecoin-project/lotus#8606](https://github.com/filecoin-project/lotus/pull/8606))

## Dependency Update
- chore: deps: update to go-libp2p v0.19.4 ([filecoin-project/lotus#8801](https://github.com/filecoin-project/lotus/pull/8801))
- github.com/filecoin-project/go-fil-markets (v1.20.1 -> v1.20.1-v16-2):
- github.com/filecoin-project/go-legs (v0.3.7 -> v0.3.10):
- github.com/filecoin-project/go-state-types (v0.1.8 -> v0.1.10):
- github.com/filecoin-project/specs-storage (v0.2.4 -> v0.4.1):

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @geoff-vball | 37 | +10565/-8454 | 150 |
| @arajasek | 33 | +7964/-6340 | 473 |
| @arajasek | 37 | +6220/-6976 | 457 |
| @vyzo | 135 | +7287/-5380 | 546 |
| @ZenGround0 | 19 | +5958/-2920 | 226 |
| @stebalien | 18 | +1566/-1101 | 116 |
| Alex | 5 | +323/-2304 | 32 |
| @zenground0 | 9 | +583/-358 | 56 |
| @jennijuju | 5 | +853/-27 | 19 |
| @jennijuju | 24 | +392/-201 | 60 |
| Marco Munizaga | 3 | +236/-83 | 5 |
| @raulk| 9 | +93/-15 | 15 |
| @travisperson | 3 | +37/-37 | 12 |
| @Kubuxu | 1 | +41/-5 | 1 |
| @koalacxr | 1 | +29/-13 | 3 |
| @gammazero | 2 | +18/-10 | 4 |
| Peter Rabbitson | 1 | +5/-3 | 1 |
| Steve Loeppky | 1 | +6/-0 | 1 |
| @masih | 1 | +3/-3 | 2 |
| @magik6k | 1 | +4/-0 | 1 |
| @jennijuju | 1 | +2/-2 | 1 |
| tian zhou | 1 | +1/-1 | 1 |

# 1.15.3 / 2022-05-31

This is an optional release of lotus that include new APIs, some improvements and bug fixes.

## New Features
- feat: api: add StateGetNetworkParams api ([filecoin-project/lotus#8546](https://github.com/filecoin-project/lotus/pull/8546))
- feat: api: Implement StateLookupRobustAddress ([filecoin-project/lotus#8486](https://github.com/filecoin-project/lotus/pull/8486))
- sealing: DataCid on workers ([filecoin-project/lotus#8557](https://github.com/filecoin-project/lotus/pull/8557))
- feat: FVM: Support exectraces ([filecoin-project/lotus#8514](https://github.com/filecoin-project/lotus/pull/8514))

## Improvements
- ux: wallet: update delete usage ([filecoin-project/lotus#8442](https://github.com/filecoin-project/lotus/pull/8442))
- chore: config: default-disable kvlog ([filecoin-project/lotus#8477](https://github.com/filecoin-project/lotus/pull/8477))
- chore: cli: Alias cli commands for uniformity ([filecoin-project/lotus#8587](https://github.com/filecoin-project/lotus/pull/8587))

## Bug Fixes
- fix: Make deal making logs much less noisy ([filecoin-project/lotus#8622](https://github.com/filecoin-project/lotus/pull/8622))
- fix: mpool: avoid deadlock on unsubscribe  ([filecoin-project/lotus#8635](https://github.com/filecoin-project/lotus/pull/8635))
- fix: update StatApplied when fvm apply message ([filecoin-project/lotus#8545](https://github.com/filecoin-project/lotus/pull/8545))
- fix: ci: build macos and linux assets on tagged releases ([filecoin-project/lotus#8597](https://github.com/filecoin-project/lotus/pull/8597))
- fix: Clean up vanilla proof fetching errors for proper display ([filecoin-project/lotus#8564](https://github.com/filecoin-project/lotus/pull/8564))
- fix: Make markets logger less noisy when doing retrievals ([filecoin-project/lotus#8604](https://github.com/filecoin-project/lotus/pull/8604))
- fix: test: Fix flaky TestGatewayWalletMsig ([filecoin-project/lotus#8601](https://github.com/filecoin-project/lotus/pull/8601))
- fix: lotus-wallet: correct network in version ([filecoin-project/lotus#8563](https://github.com/filecoin-project/lotus/pull/8563))
- fix: worker: Download proofs if PRU2 is enabled ([filecoin-project/lotus#8555](https://github.com/filecoin-project/lotus/pull/8555))
- fix: ux: update `lotus-wallet run` description ([filecoin-project/lotus#8528](https://github.com/filecoin-project/lotus/pull/8528))
- fix: market: Infer index provider topic from network name by default ([filecoin-project/lotus#8526](https://github.com/filecoin-project/lotus/pull/8526))
- fix: storiface: Make FSOverhead numbers more accurate ([filecoin-project/lotus#8481](https://github.com/filecoin-project/lotus/pull/8481))
- fix: node: fix comment for IndexProvider ([filecoin-project/lotus#8480](https://github.com/filecoin-project/lotus/pull/8480))
- fix: sealing: fail to add expired precommits to a batch ([filecoin-project/lotus#8479](https://github.com/filecoin-project/lotus/pull/8479))
- fix:sealing:check index out of bounds against correct param length not return length ([filecoin-project/lotus#8475](https://github.com/filecoin-project/lotus/pull/8475))
- fix: sealing: Finalize snap sectors before submitting proofs ([filecoin-project/lotus#8582](https://github.com/filecoin-project/lotus/pull/8582))
- chore: fix lint issue ([filecoin-project/lotus#8531](https://github.com/filecoin-project/lotus/pull/8531))
- feat: vm: add actor error backtraces to FVM ([filecoin-project/lotus#8524](https://github.com/filecoin-project/lotus/pull/8524))

## Dependency Updates
- github.com/filecoin-project/specs-storage (v0.2.2 -> v0.2.3-0.20220426183226-1a0a63c5990f):
- deps: update go-libp2p@v0.19 ([filecoin-project/lotus#8511](https://github.com/filecoin-project/lotus/pull/8511))
- chore: deps: Use tagged version of specs-storage #8764

## Others
- chore: merge releases back into master ([filecoin-project/lotus#8638](https://github.com/filecoin-project/lotus/pull/8638))
- chore:ci:make codecov quiet ([filecoin-project/lotus#8600](https://github.com/filecoin-project/lotus/pull/8600))
- chore: version: bump the version to v1.15.3-dev ([filecoin-project/lotus#8473](https://github.com/filecoin-project/lotus/pull/8473))
- chore: .gitignore: ignore built in actor bundles ([filecoin-project/lotus#8590](https://github.com/filecoin-project/lotus/pull/8590))
- ci: deps: macos build deps ([filecoin-project/lotus#8581](https://github.com/filecoin-project/lotus/pull/8581))
- chore:chore:chore remove the wrong TODO ([filecoin-project/lotus#8586](https://github.com/filecoin-project/lotus/pull/8586))
- add 1475 bootstrapper ([filecoin-project/lotus#8495](https://github.com/filecoin-project/lotus/pull/8495))
- Bump version for xtools ([filecoin-project/lotus#8494](https://github.com/filecoin-project/lotus/pull/8494))
- chore: bundle: remove wrongly committed bundle cars #8763
## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @stebalien | 4 | +607/-95 | 19 |
| @magik6k  | 9 | +550/-37 | 43 |
| @geoff-vball | 5 | +279/-219 | 27 |
| @simlecode | 1 | +306/-39 | 20 |
| @arajasek | 1 | +256/-34 | 10 |
| @zenground0 | 11 | +214/-66 | 31 |
| @arajasek | 2 | +149/-99 | 8 |
| @vyzo | 3 | +125/-81 | 4 |
| @Masih| 1 | +134/-15 | 7 |
| @travisperson | 3 | +24/-32 | 6 |
| @Rjan | 6 | +16/-16 | 9 |
| @jennijuju | 3 | +9/-8 | 15 |
| Rob Quist | 3 | +12/-4 | 3 |
| @Icarus9913 | 1 | +3/-3 | 3 |
| @swift-mx | 1 | +3/-0 | 1 |
| @Phi-rjan | 1 | +1/-1 | 1 |
| @lifei | 1 | +1/-0 | 1 |


# 1.15.2 / 2022-05-06

This is a highly recommended feature lotus release v1.15.2. This feature release introduces many new features and  for SPs, including PoSt workers, sealing scheduler, snap deal queue and so on.

Note: You need to be using go v1.18.1&up from this release onwards.

## Highlights
### ❣️❣️❣️ PoSt Workers ❣️❣️❣️
‼️️Attention - the long-awaited yet highly requested PoSt workers, they are here! And they come in as a combo: you may setup PoSt workers for both winningPoSt or/and windowPoSt worker. You can also setup any number of PoSt workers as long as you have the hardware resources!
For more details and learn how to set it up, see the docs [here](https://lotus.filecoin.io/storage-providers/seal-workers/post-workers/). You can also find early result of the PoSt workers performance by the community [here](https://github.com/filecoin-project/lotus/discussions/8375).
- feat: PoSt workers ([filecoin-project/lotus#7971](https://github.com/filecoin-project/lotus/pull/7971))
- feat: storage: Parallel proving checks ([filecoin-project/lotus#8391](https://github.com/filecoin-project/lotus/pull/8391))
  - adjust `ParallelCheckLimit` according to your resource setup to get pre-post checkers run faster!

In addition, we also added some handy toolings:
- feat: miner: API/CLI to compute window-post ([filecoin-project/lotus#8389](https://github.com/filecoin-project/lotus/pull/8389))
  - run `lotus-miner proving compute window-post` to manually trigger a full windowPoSt computation for a specific deadline for full sanity checks.
- feat: miner cli: Separate proving workers command ([filecoin-project/lotus#8379](https://github.com/filecoin-project/lotus/pull/8379))
  - run `lotus-miner proving workers` to list all the PoSt workers that's attached
- feat: miner cli: proving check --faulty, faults in storage list sectors ([filecoin-project/lotus#8349](https://github.com/filecoin-project/lotus/pull/8349))
  - run `lotus-miner proving check --faulty` to identify the sectors that might be bad.

### 🔥🔥🔥 Sealing Scheduler Enhancement 🔥🔥🔥

Have you ever got a couple workers but only a few of them are super packed, while the rest are idling waiting for jobs? Now the task can be distributed more evenly with:
- feat: sched: Improve worker assigning logic ([filecoin-project/lotus#8447](https://github.com/filecoin-project/lotus/pull/8447))

### 🌟🌟🌟 Snap Deal Enhancements 🌟🌟🌟

The Filecoin Network introduced Snap Deal with the network v15 OhSnap upgrade, and lotus shipped v1.14.0 with basic snappy support for SP to use this feature. Since then, we have received good ux feedbacks and bug reports from the community, and we are introducing a couple enhancement for SPs to better leverage this feature.

- feat: sealing: Sector upgrade queue ([filecoin-project/lotus#8330](https://github.com/filecoin-project/lotus/pull/8330))
  - Snap up CCs to be ready for deals automatically, learn more [here](https://lotus.filecoin.io/storage-providers/operate/snap-deals/#snap-deal-queue)
- feat: sealing: More SnapDeals config knobs ([filecoin-project/lotus#8343](https://github.com/filecoin-project/lotus/pull/8343))
  - SPs can now set `PreferNewSectorsForDeals` and `MaxUpgradingSectors` to better manage their sealing/dealing pipeline.
- feat: config: Move MakeNewSectorForDeals config into the Sealing sectoin ([filecoin-project/lotus#8378](https://github.com/filecoin-project/lotus/pull/8378))
- fix: sealing: Release unsealed sector files after snapdeals abort ([filecoin-project/lotus#8438](https://github.com/filecoin-project/lotus/pull/8438))
- fix: sealing: always do cooldown in handleSubmitReplicaUpdateFailed ([filecoin-project/lotus#8353](https://github.com/filecoin-project/lotus/pull/8353))
- fix: storagefsm: Fix error loop on bad event ([filecoin-project/lotus#8338](https://github.com/filecoin-project/lotus/pull/8338))
- fix: sealing: Remove sector copies from workers after snapdeals ([filecoin-project/lotus#8329](https://github.com/filecoin-project/lotus/pull/8329))

## New Features
- enable rcmgr by default ([filecoin-project/lotus#8470](https://github.com/filecoin-project/lotus/pull/8470))
- feat: cli: lotus client list-asks --protocols ([filecoin-project/lotus#8464](https://github.com/filecoin-project/lotus/pull/8464))
- feat: shed: Multi-file vlog2car ([filecoin-project/lotus#8426](https://github.com/filecoin-project/lotus/pull/8426))
- feat: worker: check fd limit on startup ([filecoin-project/lotus#8390](https://github.com/filecoin-project/lotus/pull/8390))
- feat: lotus-shed: add command to compute state over a range of tipsets. ([filecoin-project/lotus#8371](https://github.com/filecoin-project/lotus/pull/8371))
- feat: cli/net: implement 'net ping' command ([filecoin-project/lotus#8357](https://github.com/filecoin-project/lotus/pull/8357))
- feat: stmgr: call: use a buffered concurrent-access blockstore ([filecoin-project/lotus#8358](https://github.com/filecoin-project/lotus/pull/8358))
- feat: infra/ci: add `lotus-test` image as CI build step ([filecoin-project/lotus#7956](https://github.com/filecoin-project/lotus/pull/7956))
- feat:  multisig: lotus-sheed miner-multisig change-worker command. ([filecoin-project/lotus#8281](https://github.com/filecoin-project/lotus/pull/8281))
- feat: Add additional test annotations (#8272) ([filecoin-project/lotus#8272](https://github.com/filecoin-project/lotus/pull/8272))

## Improvements
- Revert appimage removal ([filecoin-project/lotus#8439](https://github.com/filecoin-project/lotus/pull/8439))
- sealing: Don't panic in ReleaseUnsealed with no ranges ([filecoin-project/lotus#8461](https://github.com/filecoin-project/lotus/pull/8461))
- testkit: give up on waiting for the RPC server to shutdown after 1s ([filecoin-project/lotus#8450](https://github.com/filecoin-project/lotus/pull/8450))
- chore: events: implement event observer deregister method ([filecoin-project/lotus#8441](https://github.com/filecoin-project/lotus/pull/8441))
- Thread safe piecereader ([filecoin-project/lotus#8397](https://github.com/filecoin-project/lotus/pull/8397))
- VM: Refactor pricelist to be based on network versions ([filecoin-project/lotus#8369](https://github.com/filecoin-project/lotus/pull/8369))
- --max-piece-size in set-ask is no longer required ([filecoin-project/lotus#8361](https://github.com/filecoin-project/lotus/pull/8361))
- refactor: convert RepoType from int to interface ([filecoin-project/lotus#8086](https://github.com/filecoin-project/lotus/pull/8086))
- Shed: fix error message ([filecoin-project/lotus#8340](https://github.com/filecoin-project/lotus/pull/8340))

## Bug Fixes
- fix: FVM: add finality check for consensus faults ([filecoin-project/lotus#8452](https://github.com/filecoin-project/lotus/pull/8452))
- fix: market: Reuse the market `PubSub` in index provider ([filecoin-project/lotus#8443](https://github.com/filecoin-project/lotus/pull/8443))
- fix: market: set all index provider options based on lotus config ([filecoin-project/lotus#8444](https://github.com/filecoin-project/lotus/pull/8444))
- release worker tracker lock when call cb func ([filecoin-project/lotus#8206](https://github.com/filecoin-project/lotus/pull/8206))
- fix: node: Fix market node startup ([filecoin-project/lotus#8425](https://github.com/filecoin-project/lotus/pull/8425))
- fix: sealing: Fix PR1 worker selection ([filecoin-project/lotus#8420](https://github.com/filecoin-project/lotus/pull/8420))
- fix: go: make Go 1.18 builds work ([filecoin-project/lotus#8410](https://github.com/filecoin-project/lotus/pull/8410))
- fix: sealing: Added error checking ([filecoin-project/lotus#8404](https://github.com/filecoin-project/lotus/pull/8404))
- fix: ux: Change Propose-worker msg ([filecoin-project/lotus#8384](https://github.com/filecoin-project/lotus/pull/8384))
- fix: miner: dead loop on removing sector ([filecoin-project/lotus#8386](https://github.com/filecoin-project/lotus/pull/8386))
- fix: worker: Fix default value handling ([filecoin-project/lotus#8380](https://github.com/filecoin-project/lotus/pull/8380))
- Revert "Update params for interopnet for fvm" ([filecoin-project/lotus#8374](https://github.com/filecoin-project/lotus/pull/8374))
- fix: cli: Reset miner/ask lists in interactive deal 'miner' step (#8155) ([filecoin-project/lotus#8155](https://github.com/filecoin-project/lotus/pull/8155))
- fix:snapup: Rename error message ([filecoin-project/lotus#8370](https://github.com/filecoin-project/lotus/pull/8370))
- fix: multisig: Print "waiting for confirmation.." ([filecoin-project/lotus#8368](https://github.com/filecoin-project/lotus/pull/8368))
- fix: lotus-wallet: pass correct repo type to repo.Init ([filecoin-project/lotus#8356](https://github.com/filecoin-project/lotus/pull/8356))
- fix: avoid racy memstores when estimating gas ([filecoin-project/lotus#8351](https://github.com/filecoin-project/lotus/pull/8351))
- fix: itests: Don't hang on exit in MineBlocksMustPost ([filecoin-project/lotus#8345](https://github.com/filecoin-project/lotus/pull/8345))
- fix: miner cli: Estimate deal weight in sector list when upgrading ([filecoin-project/lotus#8336](https://github.com/filecoin-project/lotus/pull/8336))
- fix: sealing: FinalizeSector doesn't need sealed replica access ([filecoin-project/lotus#8337](https://github.com/filecoin-project/lotus/pull/8337))
- fix: cli: add ArgsUsage field to clientGetDealCmd ([filecoin-project/lotus#8241](https://github.com/filecoin-project/lotus/pull/8241))
- fix: market: Infer index provider topic from network name by default #8533
- fix: deps: Update to FFI with logger bump #8588
- fix: sealing: Finalize snap sectors before submitting proofs #8588

## Dependency Updates
- deps: update go-libp2p@v0.19 #8533
- deps: ffi: update ffi that includes the log fix #8577
- deps: ffi: pull ffi that includes the latest fvm ([filecoin-project/lotus#8424](https://github.com/filecoin-project/lotus/pull/8424))
- Update to go-log 2.5.1 ([filecoin-project/lotus#8422](https://github.com/filecoin-project/lotus/pull/8422))
- chore(deps): update go-data-transfer with fixes (master edition) ([filecoin-project/lotus#8411](https://github.com/filecoin-project/lotus/pull/8411))
- deps: update ffi with actor v7.1.0 and fvm that uses the bundle that includes the new manifest ([filecoin-project/lotus#8402](https://github.com/filecoin-project/lotus/pull/8402))
- Update to specs-storage v0.2.2 ([filecoin-project/lotus#8400](https://github.com/filecoin-project/lotus/pull/8400))
- chore: ffi: the latest fvm release ([filecoin-project/lotus#8381](https://github.com/filecoin-project/lotus/pull/8381))
- Update params for interopnet for fvm ([filecoin-project/lotus#8119](https://github.com/filecoin-project/lotus/pull/8119))
- github.com/filecoin-project/specs-storage (v0.2.0 -> v0.2.2):
- ci: deps: macos build deps #8588

## Others
- chore: merge releases back to master ([filecoin-project/lotus#8468](https://github.com/filecoin-project/lotus/pull/8468))
- Packer publish copy orb ([filecoin-project/lotus#8413](https://github.com/filecoin-project/lotus/pull/8413))
- chore: Remove temp gomock reflect file ([filecoin-project/lotus#8372](https://github.com/filecoin-project/lotus/pull/8372))
- chore: FVM: log when fvm is used ([filecoin-project/lotus#8363](https://github.com/filecoin-project/lotus/pull/8363))
- lib: extract unixfs filestore into lib ([filecoin-project/lotus#8354](https://github.com/filecoin-project/lotus/pull/8354))
- test: use `T.TempDir` to create temporary test directory ([filecoin-project/lotus#8295](https://github.com/filecoin-project/lotus/pull/8295))
- Update Dockerfile.lotus
- chore:sealing:remove endpoint from cli ([filecoin-project/lotus#8215](https://github.com/filecoin-project/lotus/pull/8215))
- chore: build: bump the master version to v1.15.2-dev ([filecoin-project/lotus#8322](https://github.com/filecoin-project/lotus/pull/8322))
- chore: fix lint issue #8533

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @magik6k | 95 | +5147/-2922 | 401 |
| @mz-sirius | 3 | +1789/-546 | 48 |
| @nonsense  | 11 | +777/-567 | 121 |
| @arajasek | 11 | +336/-231 | 28 |
| Darko Brdareski | 1 | +463/-13 | 95 |
| @coryschwartz | 11 | +147/-217 | 13 |
| spark8899 | 2 | +300/-0 | 2 |
| @zenground0 | 2 | +6/-193 | 7 |
| Eng Zer Jun | 1 | +31/-158 | 11 |
| Kevin Li | 2 | +174/-0 | 14 |
| @arajasek | 5 | +85/-86 | 18 |
| @jennijuju | 1 | +0/-119 | 3 |
| @jennijuju | 1 | +0/-98 | 6 |
| @raulk | 1 | +60/-1 | 1 |
| @frrist | 1 | +56/-0 | 2 |
| @vyzo | 3 | +18/-16 | 5 |
| @Masih | 3 | +29/-4 | 3 |
| @jennijuju | 4 | +18/-11 | 11 |
| @hannahhoward | 1 | +13/-10 | 2 |
| @dirkmc | 1 | +21/-1 | 1 |
| koalacxr | 1 | +10/-11 | 4 |
| Aarsh Shah | 1 | +19/-1 | 1 |
| @Rjan | 6 | +10/-8 | 7 |
| @zl | 1 | +7/-1 | 1 |
| KAYUII | 1 | +3/-2 | 1 |
| @simlecode | 1 | +4/-0 | 1 |
| @dirkmc | 1 | +1/-3 | 1 |
| Jerry | 1 | +3/-0 | 1 |
| @steblian | 1 | +1/-1 | 1 |
| Geoff Stuart | 1 | +1/-0 | 1 |
| Florian Ruen | 1 | +0/-1 | 1 |

# 1.15.1 / 2022-04-07

This is a *HIGHLY recommended* feature release v1.15.1, especially for node operators and storage providers who want to be a part of the content addressing network of Filecoin and IPFS.
This feature release introduces Index Provider, GraphSync v2, and many other latest functionalities, improvements and bug fixes. More importantly, node operator can now enable the FVM(experimental) to sync mainnet!!

## Highlights

### 🔥🔥🔥 FVM (Experimental) 🔥🔥🔥
- feat: fvm: FVM integration  ([filecoin-project/lotus#8332](https://github.com/filecoin-project/lotus/pull/8332))
  The lotus team is excited to announce the launch of experimental non-programmable FVM on mainnet. By enabling `"LOTUS_USE_FVM_EXPERIMENTAL=1` envvar, the lotus daemon will be running the [WASM-compiled built-in actors](https://github.com/filecoin-project/builtin-actors) that is compatible with the existing chain(Network v15 OhSnap). If you are trying it out and having any questions or feedbacks, please leave a comment [here](https://github.com/filecoin-project/lotus/discussions/8334)!
  - chore: FVM: log when fvm is used([filecoin-project/lotus#8363](https://github.com/filecoin-project/lotus/pull/8363))
  - chore: ffi: the latest fvm release([filecoin-project/lotus#8382](https://github.com/filecoin-project/lotus/pull/8382))

### 🌟🌟🌟 Index Provider (Production Ready!) 🌟🌟🌟
- feat: markets: Integrate index ingest protocol and retrieve by any CID ([filecoin-project/lotus#7313](https://github.com/filecoin-project/lotus/pull/7313))

More and more useful data is being stored on Filecoin via deals made by clients to Storage Providers. The goal is that this content is discoverable when people need them. To achieve that goal, one of the projects [the Bedrock team](https://www.notion.so/pl-strflt/Bedrock-2e956d5d8143432080a1d84435ccf0ff) is working on is building an Indexer Ecosystem, a project that's focus on content addressing on Filecoin, then potentially have interoperability with IPFS in the future and eventually serve the retrieval market. The Indexer Ecosystem high level architecture overview diagram can be found [here](https://github.com/filecoin-project/storetheindex/blob/main/doc/indexer_ecosys.png) and a detailed write up about can be found [here](https://www.notion.so/pl-strflt/Introducing-Indexer-to-SP-90bf296794174a8281c121d4ce6747a0).

That being said, with this release, lotus Storage Providers can easily become an Index Provider and serve the Indexer Content Addressing System. Index Providers generate advertisements from the deals made by a storage provider and announces the data to the indexer nodes for further processing:
- To learn more about *what is an Index Provider and how to be an Index Provider*, read it [here](https://lotus.filecoin.io/storage-providers/operate/index-provider/) in lotus docuementation.
- An [one-off migration](https://lotus.filecoin.io/storage-providers/operate/index-provider/#first-time-migration) is needed in order for a Storage Provider to become an Index Provider and announce the proper formatted index. It's *highly recommended* for all Index Provider to do a [force bulk initialization](https://lotus.filecoin.io/storage-providers/configure/dagstore/#forcing-bulk-initialization) to enable index announcement on all existing deals.
  - Note that the Initialization places IO workload on your storage system. SP should set a proper `concurrency` based on your hardware or can stop/start initialization at their wish/convenience as proving deadlines approach and elapse, to avoid IOPS starvation or competition with window PoSt.
- After the first one-time migration, being an Index Provider barely puts any extra usage on SP's market system.
  - You can find the testing result by SPX fellows [here](https://github.com/filecoin-project/lotus/discussions/8087).

We recommend all Storage Providers that are serving deals in the Filecoin network to become a Index Provider, make the data you are storing discoverable for the retrieval market and retrieval clients!
- If you have any questions about becoming an index provider, or the indexer system in general, leave a comment [here](https://github.com/filecoin-project/lotus/discussions/8341).
- Follow the indexer project at https://github.com/filecoin-project/go-indexer-core.
- If you have any feature request or bug reports of running an index provider, create an issue in https://github.com/filecoin-project/index-provider.
- You may also join the #storetheindex channel in the Filecoin Slack to engage with the team & the community!

### ❗️❗️❗️ Dag Migration For New CAR index format in DagStore ❗️❗️❗️
The index provider leverages the latest CARv2 indexing format `MultihashIndexSorted`, which stores the multihash code as well as the digest of all CIDs in a CAR file. Thus, all Storage Providers SHOULD perform an one-off DAG mirgation to regenerate DagStore CARv2 indices. You have to do it to become an index provider, failing to do so may also impact your future deal making.
Follow the instruction [here](https://lotus.filecoin.io/storage-providers/operate/index-provider/) to perform the migration.

## New Features
- feat: sealing: Sector upgrade queue ([filecoin-project/lotus#8333](https://github.com/filecoin-project/lotus/pull/8333))
  - see more details in docs: [here](https://lotus.filecoin.io/storage-providers/operate/snap-deals/#snap-deal-queue)
- feat: market utils: Support unixfsnode in TraverseDag ([filecoin-project/lotus#8168](https://github.com/filecoin-project/lotus/pull/8168))
- feat: config: enable indexer providing by default ([filecoin-project/lotus#8314](https://github.com/filecoin-project/lotus/pull/8314))
- feat: api: Make ClientCalcCommP multithreaded ([filecoin-project/lotus#8276](https://github.com/filecoin-project/lotus/pull/8276))
- feat: config: Persistent subsystem log level config ([filecoin-project/lotus#8283](https://github.com/filecoin-project/lotus/pull/8283))
- feat: shed: blockstore/vlog to car export cmds ([filecoin-project/lotus#8265](https://github.com/filecoin-project/lotus/pull/8265))
- feat: shed: ItestD ([filecoin-project/lotus#8290](https://github.com/filecoin-project/lotus/pull/8290))
- feat: Make add piece idempotent ([filecoin-project/lotus#8160](https://github.com/filecoin-project/lotus/pull/8160))
- feat: paychmgr: Support paych funding (a.k.a. fast paid retrieval) ([filecoin-project/lotus#7883](https://github.com/filecoin-project/lotus/pull/7883))
- feat: ci: packer snap ([filecoin-project/lotus#7819](https://github.com/filecoin-project/lotus/pull/7819))
- feat: #6147: Include worker name in sealing errors ([filecoin-project/lotus#7844](https://github.com/filecoin-project/lotus/pull/7844))
- Feat: cli: Remove verified data cap ([filecoin-project/lotus#8175](https://github.com/filecoin-project/lotus/pull/8175))
- feat: gateway: add MsigGetVestingSchedule to gateway api ([filecoin-project/lotus#8104](https://github.com/filecoin-project/lotus/pull/8104))
- feat: itests: add itests ensemble mocknet getter ([filecoin-project/lotus#8157](https://github.com/filecoin-project/lotus/pull/8157))
- feat: lotus-miner sectors list --initial-pledge ([filecoin-project/lotus#8098](https://github.com/filecoin-project/lotus/pull/8098))
- Resource Manager Metrics ([filecoin-project/lotus#8089](https://github.com/filecoin-project/lotus/pull/8089))
- feat: cli: set current network version from params ([filecoin-project/lotus#8111](https://github.com/filecoin-project/lotus/pull/8111))
- feat: Snapdeals support in `storage find` CLI ([filecoin-project/lotus#8130](https://github.com/filecoin-project/lotus/pull/8130))

## Improvements
- improve resource manager integration ([filecoin-project/lotus#8318](https://github.com/filecoin-project/lotus/pull/8318))
- add check manual-stateless-deal with interactive deal making ([filecoin-project/lotus#7560](https://github.com/filecoin-project/lotus/pull/7560))
- test: cli: adding wallet tests ([filecoin-project/lotus#8079](https://github.com/filecoin-project/lotus/pull/8079))
- test: chain: unit tests for the syncer & sync manager ([filecoin-project/lotus#8072](https://github.com/filecoin-project/lotus/pull/8072))
- test: cli: unit tests for sync related commands ([filecoin-project/lotus#8080](https://github.com/filecoin-project/lotus/pull/8080))
- misc: wallet: wallet tests with annotations for system test matrix ([filecoin-project/lotus#7928](https://github.com/filecoin-project/lotus/pull/7928))
- test: Cli: add mempool tests ([filecoin-project/lotus#8162](https://github.com/filecoin-project/lotus/pull/8162))
- add a state-tree diff command to lotus shed ([filecoin-project/lotus#8081](https://github.com/filecoin-project/lotus/pull/8081))
- test: mempool: Add unit and integration tests ([filecoin-project/lotus#8017](https://github.com/filecoin-project/lotus/pull/8017))
- splistore cold object reification redux ([filecoin-project/lotus#8029](https://github.com/filecoin-project/lotus/pull/8029))
- test: cli: chain category unit tests ([filecoin-project/lotus#8048](https://github.com/filecoin-project/lotus/pull/8048))
- feat: config: Move MakeNewSectorForDeals config into the Sealing section([filecoin-project/lotus#8382](https://github.com/filecoin-project/lotus/pull/8382))

## Bug Fixes
- fix: FVM: add finality check for consensus faults #8452
- fix: market: Reuse the market PubSub in index provider #8451
- fix: market: set all index provider options based on lotus config #8444
- fix: sealing: Fix PR1 worker selection (#8421)
- fix: miner: dead loop on removing sector (#8421)
- fix: sealing: Remove sector copies from workers after snapdeals ([filecoin-project/lotus#8331](https://github.com/filecoin-project/lotus/pull/8331))
- fix: storagefsm: Fix error loop on bad event ([filecoin-project/lotus#8339](https://github.com/filecoin-project/lotus/pull/8339))
- fix: sealing: FinalizeSector doesn't need sealed replica access ([filecoin-project/lotus#8339](https://github.com/filecoin-project/lotus/pull/8339))
- fix: sealing: always do cooldown in handleSubmitReplicaUpdateFailed ([filecoin-project/lotus#8353](https://github.com/filecoin-project/lotus/pull/8353))
- fix: storage cli: Output primary sector status correctly ([filecoin-project/lotus#8320](https://github.com/filecoin-project/lotus/pull/8320))
- fix: sealing fsm: Handle inputLk correctly ([filecoin-project/lotus#8291](https://github.com/filecoin-project/lotus/pull/8291))
- fix: piece provider: Don't log CIDs as binary ([filecoin-project/lotus#8287](https://github.com/filecoin-project/lotus/pull/8287))
- fix:sealing:Log instead of error normal shutdown of state machine ([filecoin-project/lotus#8232](https://github.com/filecoin-project/lotus/pull/8232))
- fix:sealing:Handle finalize replica update failures in fsm ([filecoin-project/lotus#8229](https://github.com/filecoin-project/lotus/pull/8229))
- ci: appimage: re-install appimage CI ([filecoin-project/lotus#7943](https://github.com/filecoin-project/lotus/pull/7943))
- fix: sealing: PRU insufficient collateral  ([filecoin-project/lotus#8219](https://github.com/filecoin-project/lotus/pull/8219))
- fix: shed: diff command ([filecoin-project/lotus#8202](https://github.com/filecoin-project/lotus/pull/8202))
- Make `--lite` option visible in the lotus daemon help text  ([filecoin-project/lotus#8207](https://github.com/filecoin-project/lotus/pull/8207))
- fix:sealing:Less verbose sector manager logging ([filecoin-project/lotus#8213](https://github.com/filecoin-project/lotus/pull/8213))
- avoid panic ([filecoin-project/lotus#8205](https://github.com/filecoin-project/lotus/pull/8205))
- A package is vulnerable to Exposure of Sensitive Information  ([filecoin-project/lotus#8204](https://github.com/filecoin-project/lotus/pull/8204))
- fix: sealing: add flag usage ([filecoin-project/lotus#8190](https://github.com/filecoin-project/lotus/pull/8190))
- Fix the epoch used for gas in the message pool & validation ([filecoin-project/lotus#8163](https://github.com/filecoin-project/lotus/pull/8163))
- fix:sealing:really-do-it flag for abort upgrade ([filecoin-project/lotus#8181](https://github.com/filecoin-project/lotus/pull/8181))
- fix:proving:post check sector handles snap deals replica faults ([filecoin-project/lotus#8177](https://github.com/filecoin-project/lotus/pull/8177))
- fix: client: calculate commps for pieces bigger than 32GB ([filecoin-project/lotus#8179](https://github.com/filecoin-project/lotus/pull/8179))
- fix:cli:Continue instead of return error if no valid value is filled ([filecoin-project/lotus#8131](https://github.com/filecoin-project/lotus/pull/8131))
- fix: limit reification sizes ([filecoin-project/lotus#8149](https://github.com/filecoin-project/lotus/pull/8149))
- fix: state: Allow lotus-miner info to complete without admin permission ([filecoin-project/lotus#8057](https://github.com/filecoin-project/lotus/pull/8057))
- fix:paychan:deflake integration test ([filecoin-project/lotus#8088](https://github.com/filecoin-project/lotus/pull/8088))
- fix: worker: allow enable/disabling ReplicaUpdate tasks ([filecoin-project/lotus#8090](https://github.com/filecoin-project/lotus/pull/8090))
- don't fail reification on missing references ([filecoin-project/lotus#8128](https://github.com/filecoin-project/lotus/pull/8128))
- sealer: fix error message ([filecoin-project/lotus#8121](https://github.com/filecoin-project/lotus/pull/8121))
- don't track peer ids in rcmgr metrics ([filecoin-project/lotus#8099](https://github.com/filecoin-project/lotus/pull/8099))
- temporarily disable reification ([filecoin-project/lotus#8132](https://github.com/filecoin-project/lotus/pull/8132))
- [Describe]: when excute cmd "lotus-bench sealing" without "benchmark-… ([filecoin-project/lotus#8173](https://github.com/filecoin-project/lotus/pull/8173))

## Dependency Updates
- deps: update go-libp2p and go-libp2p-resource-manager ([filecoin-project/lotus#8289](https://github.com/filecoin-project/lotus/pull/8289))
- feat(deps): update to graphsync v0.13.0 with 2.0 protocol ([filecoin-project/lotus#8273](https://github.com/filecoin-project/lotus/pull/8273))
- dep: actor: get v7 ([filecoin-project/lotus#8194](https://github.com/filecoin-project/lotus/pull/8194))
- deps: update go-libp2p to v0.18 release  ([filecoin-project/lotus#8355](https://github.com/filecoin-project/lotus/pull/8355))
- github.com/filecoin-project/go-data-transfer (v1.14.1 -> v1.15.0):
- github.com/filecoin-project/go-fil-markets (v1.19.2 -> v1.20.1):
- deps: update go-libp2p to v0.18.0-rc5 ([filecoin-project/lotus#8169](https://github.com/filecoin-project/lotus/pull/8169))

## Others
- chore: build: backport releases ([filecoin-project/lotus#8192](https://github.com/filecoin-project/lotus/pull/8192))
- feat: build: bump the version to v1.15.1-dev ([filecoin-project/lotus#8073](https://github.com/filecoin-project/lotus/pull/8073))
- makefile: add make jen ([filecoin-project/lotus#8122](https://github.com/filecoin-project/lotus/pull/8122))
- chore: Merge releases into master ([filecoin-project/lotus#8156](https://github.com/filecoin-project/lotus/pull/8156))
- chore: ci: disable publish-packer #8451

# 1.15.0 / 2022-03-09

This is an optional release with retrieval improvements(client side), SP ux with unsealing, snap deals and regular deal making and many other new features, improvements and bug fixes.

## Highlights
- feat:sealing: StartEpochSealingBuffer triggers packing on time([filecoin-project/lotus#7905](https://github.com/filecoin-project/lotus/pull/7905))
  - use the `StartEpochSealingBuffer` configuration variable as a way to enforce that sectors are packed for sealing / updating no matter how many deals they have if the nearest deal start date is close enough to the present.
- feat: #6017 market: retrieval ask CLI command ([filecoin-project/lotus#7814](https://github.com/filecoin-project/lotus/pull/7814))
- feat(graphsync): allow setting of per-peer incoming requests for miners ([filecoin-project/lotus#7578](https://github.com/filecoin-project/lotus/pull/7578))
  - by setting `SimultaneousTransfersForStoragePerClient` in deal making configuration.
- Make retrieval even faster ([filecoin-project/lotus#7746](https://github.com/filecoin-project/lotus/pull/7746))
- feat: #7747 sealing: Adding conf variable for capping number of concurrent unsealing jobs (#7884) ([filecoin-project/lotus#7884](https://github.com/filecoin-project/lotus/pull/7884))
  - by setting `MaxConcurrentUnseals` in `DAGStoreConfig`

## New Features
- feat: mpool: Cache state nonces ([filecoin-project/lotus#8005](https://github.com/filecoin-project/lotus/pull/8005))
- chore: build: make the OhSnap epoch configurable by an envvar for devnets ([filecoin-project/lotus#7995](https://github.com/filecoin-project/lotus/pull/7995))
- Shed: Add a util to send a batch of messages ([filecoin-project/lotus#7667](https://github.com/filecoin-project/lotus/pull/7667))
- Add api for transfer diagnostics ([filecoin-project/lotus#7759](https://github.com/filecoin-project/lotus/pull/7759))
- Shed: Add a util to list terminated deals ([filecoin-project/lotus#7774](https://github.com/filecoin-project/lotus/pull/7774))
- Expose EnableGasTracing as an env_var ([filecoin-project/lotus#7750](https://github.com/filecoin-project/lotus/pull/7750))
- Command to list active sector locks ([filecoin-project/lotus#7735](https://github.com/filecoin-project/lotus/pull/7735))
- Initial switch to OpenTelemetry ([filecoin-project/lotus#7725](https://github.com/filecoin-project/lotus/pull/7725))

## Improvements
- splitstore sortless compaction ([filecoin-project/lotus#8008](https://github.com/filecoin-project/lotus/pull/8008))
- perf: chain: Make drand logs in daemon less noisy (#7955) ([filecoin-project/lotus#7955](https://github.com/filecoin-project/lotus/pull/7955))
- chore: shed: storage stats 2.0 ([filecoin-project/lotus#7941](https://github.com/filecoin-project/lotus/pull/7941))
- misc: api: Annotate lotus tests according to listed behaviors ([filecoin-project/lotus#7835](https://github.com/filecoin-project/lotus/pull/7835))
- some basic splitstore refactors ([filecoin-project/lotus#7999](https://github.com/filecoin-project/lotus/pull/7999))
- chore: sealer: quieten a log ([filecoin-project/lotus#7998](https://github.com/filecoin-project/lotus/pull/7998))
- tvx: supply network version when extracting messages. ([filecoin-project/lotus#7996](https://github.com/filecoin-project/lotus/pull/7996))
- chore: remove inaccurate comment in sealtasks ([filecoin-project/lotus#7977](https://github.com/filecoin-project/lotus/pull/7977))
- Refactor: VM: Remove the NetworkVersionGetter ([filecoin-project/lotus#7818](https://github.com/filecoin-project/lotus/pull/7818))
- refactor: state: Move randomness versioning out of the VM ([filecoin-project/lotus#7816](https://github.com/filecoin-project/lotus/pull/7816))
- updating to new datastore/blockstore code with contexts ([filecoin-project/lotus#7646](https://github.com/filecoin-project/lotus/pull/7646))
- Mempool msg selection should respect block message limits ([filecoin-project/lotus#7321](https://github.com/filecoin-project/lotus/pull/7321))
- Minor improvement for OpenTelemetry ([filecoin-project/lotus#7760](https://github.com/filecoin-project/lotus/pull/7760))
- Sort lotus-miner retrieval-deals by dealId ([filecoin-project/lotus#7749](https://github.com/filecoin-project/lotus/pull/7749))
- dagstore pieceReader: Always read full in ReadAt ([filecoin-project/lotus#7737](https://github.com/filecoin-project/lotus/pull/7737))

## Bug Fixes
- fix: sealing: Stop recovery attempts after fault ([filecoin-project/lotus#8014](https://github.com/filecoin-project/lotus/pull/8014))
- fix:snap: pay for the collateral difference needed if the miner available balance is insufficient ([filecoin-project/lotus#8234](https://github.com/filecoin-project/lotus/pull/8234))
- sealer: fix error message ([filecoin-project/lotus#8136](https://github.com/filecoin-project/lotus/pull/8136))
- typo in  variable name ([filecoin-project/lotus#8134](https://github.com/filecoin-project/lotus/pull/8134))
- fix: sealer: allow enable/disabling ReplicaUpdate tasks ([filecoin-project/lotus#8093](https://github.com/filecoin-project/lotus/pull/8093))
- chore: chain: fix log ([filecoin-project/lotus#7993](https://github.com/filecoin-project/lotus/pull/7993))
- Fix: chain: create a new VM for each epoch ([filecoin-project/lotus#7966](https://github.com/filecoin-project/lotus/pull/7966))
- fix: doc generation struct slice example value ([filecoin-project/lotus#7851](https://github.com/filecoin-project/lotus/pull/7851))
- fix: returned error not be accept correctly ([filecoin-project/lotus#7852](https://github.com/filecoin-project/lotus/pull/7852))
- fix: #7577 markets: When retrying Add Piece, first seek to start of reader ([filecoin-project/lotus#7812](https://github.com/filecoin-project/lotus/pull/7812))
- misc: n/a sealing: Fix grammatical error in a log warning message ([filecoin-project/lotus#7831](https://github.com/filecoin-project/lotus/pull/7831))
- sectors update-state checks if sector exists before changing its state ([filecoin-project/lotus#7762](https://github.com/filecoin-project/lotus/pull/7762))
- SplitStore: supress compaction near upgrades ([filecoin-project/lotus#7734](https://github.com/filecoin-project/lotus/pull/7734))

## Dependency Updates
- github.com/filecoin-project/go-commp-utils (v0.1.2 -> v0.1.3):
- github.com/filecoin-project/dagstore (v0.4.3 -> v0.4.4):
- github.com/filecoin-project/go-fil-markets (v1.13.4 -> v1.19.2):
- github.com/filecoin-project/go-statestore (v0.1.1 -> v0.2.0):
- github.com/filecoin-project/go-storedcounter (v0.0.0-20200421200003-1c99c62e8a5b -> v0.1.0):
- github.com/filecoin-project/specs-actors/v2 (v2.3.5 -> v2.3.6):
  - feat(deps): update markets stack ([filecoin-project/lotus#7959](https://github.com/filecoin-project/lotus/pull/7959))
  - Use go-libp2p-connmgr v0.3.1 ([filecoin-project/lotus#7957](https://github.com/filecoin-project/lotus/pull/7957))
  - dep/fix 7701 Dependency: update to ipld-legacy to v0.1.1 ([filecoin-project/lotus#7751](https://github.com/filecoin-project/lotus/pull/7751))

## Others
- chore: backport: release ([filecoin-project/lotus#8245](https://github.com/filecoin-project/lotus/pull/8245))
- Lotus release v1.15.0-rc3 ([filecoin-project/lotus#8236](https://github.com/filecoin-project/lotus/pull/8236))
- Lotus release v1.15.0-rc2 ([filecoin-project/lotus#8211](https://github.com/filecoin-project/lotus/pull/8211))
- Merge branch 'releases' into release/v1.15.0
- chore: build: backport releases ([filecoin-project/lotus#8193](https://github.com/filecoin-project/lotus/pull/8193))
- Merge branch 'releases' into release/v1.15.0
- bump the version to v1.15.0-rc1
- chore: build: v1.14.0 -> master ([filecoin-project/lotus#8053](https://github.com/filecoin-project/lotus/pull/8053))
- chore: merge release/v1.14.0 PRs into master ([filecoin-project/lotus#7979](https://github.com/filecoin-project/lotus/pull/7979))
- chore: update PR template ([filecoin-project/lotus#7918](https://github.com/filecoin-project/lotus/pull/7918))
- build: release: bump master version to v1.15.0-dev ([filecoin-project/lotus#7922](https://github.com/filecoin-project/lotus/pull/7922))
- misc: docs: remove issue number from the pr title ([filecoin-project/lotus#7902](https://github.com/filecoin-project/lotus/pull/7902))
- Snapcraft grade no develgrade ([filecoin-project/lotus#7802](https://github.com/filecoin-project/lotus/pull/7802))
- chore: create pull_request_template.md ([filecoin-project/lotus#7726](https://github.com/filecoin-project/lotus/pull/7726))
- Disable appimage ([filecoin-project/lotus#7707](https://github.com/filecoin-project/lotus/pull/7707))

## Contributors
| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @arajasek | 73 | +7232/-2778 | 386 |
| @zenground0 | 27 | +5604/-1049 | 219 |
| @vyzo | 118 | +4356/-1470 | 253 |
| @zl | 1 | +3725/-309 | 8 |
| @dirkmc | 7 | +1392/-1110 | 61 |
| arajasek | 37 | +221/-1329 | 90 |
| @magik6k | 33 | +1138/-336 | 101 |
| @whyrusleeping | 2 | +483/-585 | 28 |
| Darko Brdareski | 14 | +725/-276 | 154 |
| @rvagg | 2 | +43/-947 | 10 |
| @hannahhoward | 5 | +436/-335 | 31 |
| @hannahhoward | 12 | +507/-133 | 37 |
| @jennijuju | 27 | +333/-178 | 54 |
| @TheMenko | 8 | +237/-179 | 17 |
| c r | 2 | +227/-45 | 12 |
| @dirkmck | 12 | +188/-40 | 27 |
| @ribasushi | 3 | +128/-62 | 3 |
| @raulk | 6 | +128/-49 | 9 |
| @Whyrusleeping | 1 | +76/-70 | 8 |
| @Stebalien | 1 | +55/-37 | 1 |
| @jennijuju | 11 | +29/-16 | 11 |
| @aarshkshah1992 | 1 | +23/-19 | 5 |
| @travisperson | 1 | +0/-18 | 2 |
| @gstuart | 3 | +12/-1 | 3 |
| @coryschwartz | 4 | +5/-6 | 4 |
| @pefish | 1 | +4/-3 | 1 |
| @Kubuxu | 1 | +5/-2 | 2 |
| Colin Kennedy | 1 | +4/-2 | 1 |
| Rob Quist | 1 | +2/-2 | 1 |
| @shotcollin | 1 | +1/-1 | 1 |


# 1.14.4 / 2022-03-03

This is a *highly recommended* optional release for storage providers that are doing snap deals. This fix the bug
that causes some snap deal sectors are stuck in `FinalizeReplicaUpdate`. In addition, SPs should be able to force
update sectors status without getting blocked by `normal shutdown of state machine`.

# v1.14.3 / 2022-02-28

This is an **optional** release, that includes a fix to properly register the `--really-do-it` flag for abort-upgrade.

# 1.14.2 / 2022-02-24

This is an **optional** release of lotus, that's had a couple more improvements w.r.t Snap experience for storage providers in preparation of the[upcoming OhSnap upgrade](https://github.com/filecoin-project/community/discussions/74?sort=new#discussioncomment-1922550).

Note that the network is STILL scheduled to upgrade to v15 on March 1st at 2022-03-01T15:00:00Z. All node operators, including storage providers, must upgrade to at least Lotus v1.14.0 before that time. Storage providers must update their daemons, miners, and worker(s).

Wanna know how to Snap your deal? Check [this](https://github.com/filecoin-project/lotus/discussions/8141) out!

## Bug Fixes
- fix lotus-bench for sealing jobs (#8173)
- fix:sealing:really-do-it flag for abort upgrade (#8181)
- fix:proving:post check sector handles snap deals replica faults (#8177)
- fix: sealing: missing file type (#8180)

## Others
- Retract force-pushed v1.14.0 to work around stale gomod caches (#8159): We originally tagged v1.14.0 off the wrong
  commit and fixed that by a force push, in which is a really bad practise since it messes up the go mod. Therefore,
  we want to retract it and users may use v1.14.1&^.

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @zenground0 | 2 | +73/-58 | 12 |
| @eben.xie | 1 | +7/-0 | 1 |
| @jennijuju | 1 | +4/-0 | 1 |
| @jennijuju | 1 | +2/-1 | 1 |
| @ribasushi | 1 | +2/-0 | 1 |

# 1.14.1 / 2022-02-18

This is an **optional** release of lotus, that fixes the incorrect *comment* of network v15 OhSnap upgrade **date**. Note the actual upgrade epoch in [v1.14.0](https://github.com/filecoin-project/lotus/releases/tag/v1.14.0) was correct.

# 1.14.0 / 2022-02-17

This is a MANDATORY release of Lotus that introduces [Filecoin network v15,
codenamed the OhSnap upgrade](https://github.com/filecoin-project/community/discussions/74?sort=new#discussioncomment-1922550).

The network is scheduled to upgrade to v15 on March 1st at 2022-03-01T15:00:00Z. All node operators, including storage providers, must upgrade to this release (or a later release) before that time. Storage providers must update their daemons, miners, and worker(s).

The OhSnap upgrade introduces the following FIPs, delivered in [actors v7](https://github.com/filecoin-project/specs-actors/releases/tag/v7.0.0):
- [FIP-0019 Snap Deals](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0019.md)
- [FIP-0028 Remove Datacap from Verified clients](https://github.com/filecoin-project/FIPs/pull/226)

It is recommended that storage providers download the new params before updating their node, miner, and workers. To do so:

- Download Lotus v1.14.0 or later
- run `make lotus-shed`
- run `./lotus-shed fetch-params` with the appropriate `proving-params` flag
- Upgrade the Lotus daemon and miner **when the previous step is complete**

All node operators, including storage providers, should be aware that a pre-migration will begin at 2022-03-01T13:30:00Z (90 minutes before the real upgrade). The pre-migration will take between 20 and 50 minutes, depending on hardware specs. During this time, expect slower block validation times, increased CPU and memory usage, and longer delays for API queries.

## New Features and Changes
- Integrate actor v7-rc1:
  - Integrate v7 actors ([#7617](https://github.com/filecoin-project/lotus/pull/7617))
  - feat: state: Fast migration for v15 ([#7933](https://github.com/filecoin-project/lotus/pull/7933))
  - fix: blockstore: Add missing locks to autobatch::Get() [#7939](https://github.com/filecoin-project/lotus/pull/7939))
  - correctness fixes for the autobatch blockstore ([#7940](https://github.com/filecoin-project/lotus/pull/7940))
- Implement and support [FIP-0019 Snap Deals](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0019.md)
  - chore: deps: Integrate proof v11.0.0 ([#7923](https://github.com/filecoin-project/lotus/pull/7923))
  - Snap Deals Lotus Integration: FSM Posting and integration test ([#7810](https://github.com/filecoin-project/lotus/pull/7810))
  - Feat/sector storage unseal ([#7730](https://github.com/filecoin-project/lotus/pull/7730))
  - Feat/snap deals storage ([#7615](https://github.com/filecoin-project/lotus/pull/7615))
  - fix: sealing: Add more deal expiration checks during PRU pipeline ([#7871](https://github.com/filecoin-project/lotus/pull/7871))
  - chore: deps: Update go-paramfetch ([#7917](https://github.com/filecoin-project/lotus/pull/7917))
  - feat: #7880 gas: add gas charge for VerifyReplicaUpdate ([#7897](https://github.com/filecoin-project/lotus/pull/7897))
  - enhancement: sectors: disable existing cc upgrade path 2 days before the upgrade epoch ([#7900](https://github.com/filecoin-project/lotus/pull/7900))

## Improvements
- updating to new datastore/blockstore code with contexts ([#7646](https://github.com/filecoin-project/lotus/pull/7646))
- reorder transfer checks so as to ensure sending 2B FIL to yourself fails if you don't have that amount ([#7637](https://github.com/filecoin-project/lotus/pull/7637))
- VM: Circ supply should be constant per epoch ([#7811](https://github.com/filecoin-project/lotus/pull/7811))

## Bug Fixes
- Fix: state: circsuypply calc around null blocks ([#7890](https://github.com/filecoin-project/lotus/pull/7890))
- Mempool msg selection should respect block message limits ([#7321](https://github.com/filecoin-project/lotus/pull/7321))
  SplitStore: supress compaction near upgrades ([#7734](https://github.com/filecoin-project/lotus/pull/7734))

## Others
- chore: create pull_request_template.md ([#7726](https://github.com/filecoin-project/lotus/pull/7726))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Aayush Rajasekaran | 41 | +5538/-1205 | 189 |
| zenground0 | 11 | +3316/-524 | 124 |
| Jennifer Wang | 29 | +714/-599 | 68 |
| ZenGround0 | 3 | +263/-25 | 11 |
| c r | 2 | +198/-30 | 6 |
| vyzo | 4 | +189/-7 | 7 |
| Aayush | 11 | +146/-48 | 49 |
| web3-bot | 10 | +99/-17 | 10 |
| Steven Allen | 1 | +55/-37 | 1 |
| Jiaying Wang | 5 | +30/-8 | 5 |
| Jakub Sztandera | 2 | +8/-3 | 3 |
| Łukasz Magiera | 1 | +3/-3 | 2 |
| Travis Person | 1 | +2/-2 | 2 |
| Rod Vagg | 1 | +2/-2 | 2 |

# v1.13.2 /  2022-01-09

Lotus v1.13.2 is a *highly recommended* feature release with remarkable retrieval improvements, new features like
worker management, schedule enhancements and so on.

## Highlights
- 🚀🚀🚀Improve retrieval deal experience
  - Testing result with MinerX.3 shows the retrieval deal success rate has increased dramatically with faster transfer
    speed, you can join or follow along furthur performance testings [here](https://github.com/filecoin-project/lotus/discussions/7874). We recommend application developers to integrate with the new
    retrieval APIs to provide a better client experience.
  - 🌟🌟🌟 Reduce retrieval Time-To-First-Byte over 100x ([#7693](https://github.com/filecoin-project/lotus/pull/7693))
    - This change makes most free, small retrievals sub-second
  - 🌟🌟🌟 Partial retrieval ux improvements ([#7610](https://github.com/filecoin-project/lotus/pull/7610))
    - New retrieval commands for clients:
      - `lotus client ls`: retrieve and list desired object links
      - `lotus client cat`: retrieve and print the data from the network
    - 🌟🌟 The monolith `ClientRetrieve` method was broken into:
      - `ClientRetrieve` which retrieves data into the local repo (or into an IPFS node if ipfs integration is enabled)
      - `ClientRetrieveWait` which will wait for the retrieval to complete
      - `ClientExport` which will export data from the local node
      - Note: this change only applies to v1 API. v0 API remains unchanged.
    - 🌟 Support for full ipld selectors was added (for example making it possible to only retrieve list of directories in a deal, without fetching any file data)
      - To learn more, see [here](https://github.com/filecoin-project/lotus/blob/0523c946f984b22b3f5de8cc3003cc791389527e/api/types.go#L230-L264)
- 🚀🚀 Sealing scheduler enhancements ([#7703](https://github.com/filecoin-project/lotus/pull/7703),
  [#7269](https://github.com/filecoin-project/lotus/pull/7269)), [#7714](https://github.com/filecoin-project/lotus/pull/7714)
  - Workers are now aware of cgroup memory limits
  - Multiple tasks which use a GPU can be scheduled on a single worker
  - Workers can override default resource table through env vars
    - Default value list: https://gist.github.com/magik6k/c0e1c7cd73c1241a9acabc30bf469a43
- 🚀🚀 Sector storage groups ([#7453](https://github.com/filecoin-project/lotus/pull/7453))
  - Storage groups allow for better control of data flow between workers, for example, it makes it possible to define that data from PC1 on a given worker has to have it's PC2 step executed on the same worker
  - To set it up, follow the instructions under the `Sector Storage Group` section [here](https://lotus.filecoin.io/docs/storage-providers/seal-workers/#lotus-worker-co-location)

## New Features
- Add RLE dump code ([#7691](https://github.com/filecoin-project/lotus/pull/7691))
- Shed: Add a util to list miner faults ([#7605](https://github.com/filecoin-project/lotus/pull/7605))
- lotus-shed msg: Decode submessages/msig proposals ([#7639](https://github.com/filecoin-project/lotus/pull/7639))
- CLI: Add a lotus multisig cancel command ([#7645](https://github.com/filecoin-project/lotus/pull/7645))
- shed: simple wallet balancer util ([#7414](https://github.com/filecoin-project/lotus/pull/7414))
  - balancing token balance between multiple accounts

## Improvements
- Add verbose mode to `lotus-miner pieces list-cids` ([#7699](https://github.com/filecoin-project/lotus/pull/7699))
- retrieval: Only output matching nodes, MatchPath dagspec ([#7706](https://github.com/filecoin-project/lotus/pull/7706))
- Cleanup partial retrieval codepaths ( zero functional changes ) ([#7688](https://github.com/filecoin-project/lotus/pull/7688))
- storage: Use 1M buffers for Tar transfers ([#7681](https://github.com/filecoin-project/lotus/pull/7681))
- Chore/dm level tests plus merkle proof cars ([#7673](https://github.com/filecoin-project/lotus/pull/7673))
- Shed: Add a util to create miners more easily ([#7595](https://github.com/filecoin-project/lotus/pull/7595))
- add timeout flag to wait-api command ([#7592](https://github.com/filecoin-project/lotus/pull/7592))
- add log for restart windows post scheduler ([#7613](https://github.com/filecoin-project/lotus/pull/7613))
- remove jaeger envvars ([#7631](https://github.com/filecoin-project/lotus/pull/7631))
- remove api and jaeger env from docker file ([#7624](https://github.com/filecoin-project/lotus/pull/7624))
- Wdpost worker: Reduce challenge confidence to 1 epoch ([#7572](https://github.com/filecoin-project/lotus/pull/7572))
- add additional methods to lotus gateway ([#7644](https://github.com/filecoin-project/lotus/pull/7644))
- Add caches to lotus-stats and splitcode ([#7329](https://github.com/filecoin-project/lotus/pull/7329))
- remote store: Remove debug printf ([#7664](https://github.com/filecoin-project/lotus/pull/7664))
- docsgen-cli: Handle commands with no description correctly ([#7659](https://github.com/filecoin-project/lotus/pull/7659))

## Bug Fixes
- fix docker logic error ([#7709](https://github.com/filecoin-project/lotus/pull/7709))
- add missing NodeType tag ([#7559](https://github.com/filecoin-project/lotus/pull/7559))
- checkCommit should return SectorCommitFailed ([#7555](https://github.com/filecoin-project/lotus/pull/7555))
- ffiwrapper: Validate PC2 by calling C1 with random seeds ([#7710](https://github.com/filecoin-project/lotus/pull/7710))

## Dependency Updates
- Update go-graphsync v0.10.6 ([#7708](https://github.com/filecoin-project/lotus/pull/7708))
- update go-libp2p-pubsub to v0.5.6 ([#7581](https://github.com/filecoin-project/lotus/pull/7581))
- Update go-state-types ([#7591](https://github.com/filecoin-project/lotus/pull/7591))
- disable mplex stream muxer ([#7689](https://github.com/filecoin-project/lotus/pull/7689))
- Bump ws from 5.2.2 to 5.2.3 in /lotuspond/front ([#7660](https://github.com/filecoin-project/lotus/pull/7660))
- Bump color-string from 1.5.3 to 1.6.0 in /lotuspond/front ([#7658](https://github.com/filecoin-project/lotus/pull/7658))
- Bump postcss from 7.0.17 to 7.0.39 in /lotuspond/front ([#7657](https://github.com/filecoin-project/lotus/pull/7657))
- Bump path-parse from 1.0.6 to 1.0.7 in /lotuspond/front ([#7656](https://github.com/filecoin-project/lotus/pull/7656))
- Bump tmpl from 1.0.4 to 1.0.5 in /lotuspond/front ([#7655](https://github.com/filecoin-project/lotus/pull/7655))
- Bump url-parse from 1.4.7 to 1.5.3 in /lotuspond/front ([#7654](https://github.com/filecoin-project/lotus/pull/7654))
- github.com/filecoin-project/go-state-types (v0.1.1-0.20210915140513-d354ccf10379 -> v0.1.1):

## Others
- Update archive script ([#7690](https://github.com/filecoin-project/lotus/pull/7690))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @magik6k | 89 | +5200/-1818 | 232 |
| Travis Person | 5 | +1473/-953 | 38 |
| @arajasek | 6 | +550/-38 | 19 |
| @clinta | 4 | +393/-123 | 26 |
| @ribasushi | 3 | +334/-68 | 7 |
| @jennijuju| 13 | +197/-120 | 67 |
| @Kubuxu | 10 | +153/-30 | 10 |
| @coryschwartz | 6 | +18/-26 | 6 |
| Marten Seemann | 2 | +6/-34 | 5 |
| @vyzo | 1 | +3/-3 | 2 |
| @hannahhoward | 1 | +3/-3 | 2 |
| @zenground0 | 2 | +2/-2 | 2 |
| @yaohcn | 2 | +2/-2 | 2 |
| @jennijuju | 1 | +1/-1 | 1 |
| @hunjixin | 1 | +1/-0 | 1 |



# v1.13.1 / 2021-11-26

This is an optional Lotus v1.13.1 release.

## New Features
- Shed: Add a util to find miner based on peerid ([filecoin-project/lotus#7544](https://github.com/filecoin-project/lotus/pull/7544))
- Collect and expose graphsync metrics  ([filecoin-project/lotus#7542](https://github.com/filecoin-project/lotus/pull/7542))
- Shed: Add a util to find the most recent null tipset ([filecoin-project/lotus#7456](https://github.com/filecoin-project/lotus/pull/7456))

## Improvements
- Show prepared tasks in sealing jobs ([filecoin-project/lotus#7527](https://github.com/filecoin-project/lotus/pull/7527))
- To make Deep happy ([filecoin-project/lotus#7546](https://github.com/filecoin-project/lotus/pull/7546))
- Expose per-state sector counts on the prometheus endpoint ([filecoin-project/lotus#7541](https://github.com/filecoin-project/lotus/pull/7541))
- Add storage-id flag to proving check ([filecoin-project/lotus#7479](https://github.com/filecoin-project/lotus/pull/7479))
- FilecoinEC: Improve a log message ([filecoin-project/lotus#7499](https://github.com/filecoin-project/lotus/pull/7499))
- itests: retry deal when control addr is out of funds ([filecoin-project/lotus#7454](https://github.com/filecoin-project/lotus/pull/7454))
- Normlize selector use within lotus ([filecoin-project/lotus#7467](https://github.com/filecoin-project/lotus/pull/7467))
- sealing: Improve scheduling of ready work ([filecoin-project/lotus#7335](https://github.com/filecoin-project/lotus/pull/7335))
- Remove dead example code + dep ([filecoin-project/lotus#7466](https://github.com/filecoin-project/lotus/pull/7466))

## Bug Fixes
- fix  the withdrawn amount unit ([filecoin-project/lotus#7563](https://github.com/filecoin-project/lotus/pull/7563))
- rename vm#make{=>Account}Actor(). ([filecoin-project/lotus#7562](https://github.com/filecoin-project/lotus/pull/7562))
- Fix used sector space accounting after AddPieceFailed ([filecoin-project/lotus#7530](https://github.com/filecoin-project/lotus/pull/7530))
- Don't remove sector data when moving data into a shared path ([filecoin-project/lotus#7494](https://github.com/filecoin-project/lotus/pull/7494))
- fix: support node instantiation in external packages ([filecoin-project/lotus#7511](https://github.com/filecoin-project/lotus/pull/7511))
- Stop adding Jennifer's $HOME to lotus docs ([filecoin-project/lotus#7477](https://github.com/filecoin-project/lotus/pull/7477))
- Bugfix: Use correct startup network versions ([filecoin-project/lotus#7486](https://github.com/filecoin-project/lotus/pull/7486))
- Dep upgrade pass ([filecoin-project/lotus#7478](https://github.com/filecoin-project/lotus/pull/7478))
- Remove obsolete GS testplan - it now lives in go-graphsync ([filecoin-project/lotus#7469](https://github.com/filecoin-project/lotus/pull/7469))
- sealing: Recover sectors after failed AddPiece ([filecoin-project/lotus#7444](https://github.com/filecoin-project/lotus/pull/7444))

## Dependency Updates
- Update go-graphsync v0.10.1 ([filecoin-project/lotus#7457](https://github.com/filecoin-project/lotus/pull/7457))
- update to proof v10.1.0 ([filecoin-project/lotus#7564](https://github.com/filecoin-project/lotus/pull/7564))
- github.com/filecoin-project/specs-actors/v6 (v6.0.0 -> v6.0.1):
- github.com/filecoin-project/go-jsonrpc (v0.1.4-0.20210217175800-45ea43ac2bec -> v0.1.5):
- github.com/filecoin-project/go-fil-markets (v1.13.1 -> v1.13.3):
- github.com/filecoin-project/go-data-transfer (v1.11.1 -> v1.11.4):
- github.com/filecoin-project/go-crypto (v0.0.0-20191218222705-effae4ea9f03 -> v0.0.1):
- github.com/filecoin-project/go-commp-utils (v0.1.1-0.20210427191551-70bf140d31c7 -> v0.1.2):
- github.com/filecoin-project/go-cbor-util (v0.0.0-20191219014500-08c40a1e63a2 -> v0.0.1):
- github.com/filecoin-project/go-address (v0.0.5 -> v0.0.6):
- unpin the yamux dependency ([filecoin-project/lotus#7532](https://github.com/filecoin-project/lotus/pull/7532)
- peerstore@v0.2.9 was withdrawn, let's not depend on it directly ([filecoin-project/lotus#7481](https://github.com/filecoin-project/lotus/pull/7481))
- chore(deps): use tagged github.com/ipld/go-ipld-selector-text-lite ([filecoin-project/lotus#7464](https://github.com/filecoin-project/lotus/pull/7464))
- Stop indirectly depending on deprecated github.com/prometheus/common ([filecoin-project/lotus#7473](https://github.com/filecoin-project/lotus/pull/7473))

## Others
- fix the changelog ([filecoin-project/lotus#7594](https://github.com/filecoin-project/lotus/pull/7594))
- v1.13.1-rc2 prep ([filecoin-project/lotus#7593](https://github.com/filecoin-project/lotus/pull/7593))
- lotus v1.13.1-rc1 ([filecoin-project/lotus#7569](https://github.com/filecoin-project/lotus/pull/7569))
- misc: back-port v1.13.0 back to master ([filecoin-project/lotus#7537](https://github.com/filecoin-project/lotus/pull/7537))
- Inline codegen ([filecoin-project/lotus#7495](https://github.com/filecoin-project/lotus/pull/7495))
- releases -> master ([filecoin-project/lotus#7507](https://github.com/filecoin-project/lotus/pull/7507))
- Make chocolate back to master ([filecoin-project/lotus#7493](https://github.com/filecoin-project/lotus/pull/7493))
- restore filters for the build-macos job ([filecoin-project/lotus#7455](https://github.com/filecoin-project/lotus/pull/7455))
- bump master to v1.13.1-dev ([filecoin-project/lotus#7451](https://github.com/filecoin-project/lotus/pull/7451))

Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @magik6k | 27 | +1285/-531 | 76 |
| @ribasushi | 7 | +265/-1635 | 21 |
| @raulk | 2 | +2/-737 | 13 |
| @nonsens | 4 | +391/-21 | 19 |
| @arajasek | 6 | +216/-23 | 14 |
| @jennijuju| 8 | +102/-37 | 29 |
| Steven Allen | 2 | +77/-29 | 6 |
| @jennijuju | 4 | +19/-18 | 11 |
| @dirkmc | 2 | +9/-9 | 4 |
| @@coryschwartz | 1 | +16/-2 | 2 |
| @frrist | 1 | +12/-0 | 2 |
| @Kubuxu | 5 | +5/-5 | 5 |
| @hunjixin | 2 | +6/-3 | 2 |
| @vyzo | 1 | +3/-3 | 2 |
| @@rvagg | 1 | +3/-3 | 2 |
| @hannahhoward | 1 | +3/-2 | 2 |
| Marten Seemann | 1 | +3/-0 | 1 |
| @ZenGround0 | 1 | +1/-1 | 1 |


# v1.13.0 / 2021-10-18

Lotus v1.13.0 is a *highly recommended* feature release for all lotus users(i.e: storage providers, data brokers, application developers and so on) that supports the upcoming
[Network v14 Chocolate upgrade](https://github.com/filecoin-project/lotus/discussions/7431).
This feature release includes the latest functionalities and improvements, like data transfer rate-limiting for both storage and retrieval deals, proof v10 with CUDA support, etc. You can find more details in the Changelog below.

## Highlights
- Enable separate storage and retrieval transfer limits ([filecoin-project/lotus#7405](https://github.com/filecoin-project/lotus/pull/7405))
  - `SimultaneousTransfer` is now replaced by `SimultaneousTransfersForStorage` and `SimultaneousTransfersForRetrieval`, where users may set the amount of ongoing data transfer for storage and retrieval deals in parallel separately. The default value for both is set to 20.
  - If you are using the lotus client, these two configuration variables are under the `Client` section in `./lotus/config.toml`.
  - If you are a service provider, these two configuration variables should be set under the `Dealmaking` section in `/.lotusminer/config.toml`.
- Update proofs to v10.0.0 ([filecoin-project/lotus#7420](https://github.com/filecoin-project/lotus/pull/7420))
  - This version supports CUDA. To enable CUDA instead of openCL, build lotus with `FFI_USE_CUDA=1 FFI_BUILD_FROM_SOURCE=1 ...`.
  - You can find additional Nvidia driver installation instructions written by MinerX fellows [here](https://github.com/filecoin-project/lotus/discussions/7443#discussioncomment-1425274) and perf improvements result on PC2/C2/WindowPoSt computation on different profiles [here](https://github.com/filecoin-project/lotus/discussions/7443), most people observe a 30-50% decrease in computation time.

## New Features
- Feat/datamodel selector retrieval ([filecoin-project/lotus#6393](https://github.com/filecoin-project/lotus/pull/66393393))
  - This introduces a new RetrievalOrder-struct field and a CLI option that takes a string representation as understood by [https://pkg.go.dev/github.com/ipld/go-ipld-selector-text-lite#SelectorSpecFromPath](https://pkg.go.dev/github.com/ipld/go-ipld-selector-text-lite#SelectorSpecFromPath). This allows for partial retrieval of any sub-DAG of a deal provided the user knows the exact low-level shape of the deal contents.
    - For example, to retrieve the first entry of a UnixFS directory by executing, run `lotus client retrieve --miner f0XXXXX --datamodel-path-selector 'Links/0/Hash' bafyROOTCID ~/output`
- Expose storage stats on the metrics endpoint ([filecoin-project/lotus#7418](https://github.com/filecoin-project/lotus/pull/7418))
- feat: Catch panic to generate report and reraise ([filecoin-project/lotus#7341](https://github.com/filecoin-project/lotus/pull/7341))
  - Set `LOTUS_PANIC_REPORT_PATH` and `LOTUS_PANIC_JOURNAL_LOOKBACK` to get reports generated when a panic occurs on your daemon miner or workers.
- Add envconfig docs to the config ([filecoin-project/lotus#7412](https://github.com/filecoin-project/lotus/pull/7412))
  - You can now find supported env vars in [default-lotus-miner-config.toml](https://github.com/filecoin-project/lotus/blob/master/documentation/en/default-lotus-miner-config.toml).
- lotus shed: fr32 utils ([filecoin-project/lotus#7355](https://github.com/filecoin-project/lotus/pull/7355))
- Miner CLI: Allow trying to change owners of any miner actor ([filecoin-project/lotus#7328](https://github.com/filecoin-project/lotus/pull/7328))
- Add --unproven flag to the sectors list command ([filecoin-project/lotus#7308](https://github.com/filecoin-project/lotus/pull/7308))

## Improvements
- check for deal start epoch on SectorAddPieceToAny ([filecoin-project/lotus#7407](https://github.com/filecoin-project/lotus/pull/7407))
- Verify Voucher locks in VoucherValidUnlocked ([filecoin-project/lotus#5609](https://github.com/filecoin-project/lotus/pull/5609))
- Add more info to miner allinfo command ([filecoin-project/lotus#7384](https://github.com/filecoin-project/lotus/pull/7384))
- add `lotus-miner storage-deals list --format=json` with transfers ([filecoin-project/lotus#7312](https://github.com/filecoin-project/lotus/pull/7312))
- Fix formatting ([filecoin-project/lotus#7383](https://github.com/filecoin-project/lotus/pull/7383))
- GetCurrentDealInfo err: handle correctly err case ([filecoin-project/lotus#7346](https://github.com/filecoin-project/lotus/pull/7346))
- fix: Enforce verification key integrity check regardless of TRUST_PARAMS=1 ([filecoin-project/lotus#7327](https://github.com/filecoin-project/lotus/pull/7327))
- Show more deal states in miner info ([filecoin-project/lotus#7311](https://github.com/filecoin-project/lotus/pull/7311))
- Prep retrieval for selectors: no functional changes ([filecoin-project/lotus#7306](https://github.com/filecoin-project/lotus/pull/7306))
- Seed: improve helptext ([filecoin-project/lotus#7304](https://github.com/filecoin-project/lotus/pull/7304))
- Mempool: reduce size of sigValCache ([filecoin-project/lotus#7305](https://github.com/filecoin-project/lotus/pull/7305))
- Stop indirectly depending on deprecated github.com/prometheus/common ([filecoin-project/lotus#7474](https://github.com/filecoin-project/lotus/pull/7474))

## Bug Fixes
- StateSearchMsg: Correct usage of the allowReplaced flag ([filecoin-project/lotus#7450](https://github.com/filecoin-project/lotus/pull/7450))
- fix staging area path buildup ([filecoin-project/lotus#7363](https://github.com/filecoin-project/lotus/pull/7363))
- storagemgr: Cleanup workerLk around worker resources ([filecoin-project/lotus#7334](https://github.com/filecoin-project/lotus/pull/7334))
- fix: check padSector Cid ([filecoin-project/lotus#7310](https://github.com/filecoin-project/lotus/pull/7310))
- sealing: Recover sectors after failed AddPiece ([filecoin-project/lotus#7492](https://github.com/filecoin-project/lotus/pull/7492))
- fix: support node instantiation in external packages ([filecoin-project/lotus#7511](https://github.com/filecoin-project/lotus/pull/7511))
- Chore/backport cleanup withdrawn dependency ([filecoin-project/lotus#7482](https://github.com/filecoin-project/lotus/pull/7482))

## Dependency Updates
- github.com/filecoin-project/go-data-transfer (v1.10.1 -> v1.11.1):
- github.com/filecoin-project/go-fil-markets (v1.12.0 -> v1.13.1):
- github.com/filecoin-project/go-paramfetch (v0.0.2-0.20210614165157-25a6c7769498 -> v0.0.2):
- update go-libp2p to v0.15.0 ([filecoin-project/lotus#7362](https://github.com/filecoin-project/lotus/pull/7362))
- update to go-graphsync v0.10.1 ([filecoin-project/lotus#7359](https://github.com/filecoin-project/lotus/pull/7359))

## Others
- Chocolate to master ([filecoin-project/lotus#7440](https://github.com/filecoin-project/lotus/pull/7440))
- releases -> master ([filecoin-project/lotus#7403](https://github.com/filecoin-project/lotus/pull/7403))
- remove nerpanet related code  ([filecoin-project/lotus#7373](https://github.com/filecoin-project/lotus/pull/7373))
- sync branch main with master on updates ([filecoin-project/lotus#7366](https://github.com/filecoin-project/lotus/pull/7366))
- remove job to install jq ([filecoin-project/lotus#7309](https://github.com/filecoin-project/lotus/pull/7309))
- restore filters for the build-macos job ([filecoin-project/lotus#7455](https://github.com/filecoin-project/lotus/pull/7455))
- v1.13.0-rc2 ([filecoin-project/lotus#7458](https://github.com/filecoin-project/lotus/pull/7458))
- v1.13.0-rc1 ([filecoin-project/lotus#7452](https://github.com/filecoin-project/lotus/pull/7452))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @dirkmc | 8 | +845/-375 | 55 |
| @magik6k | 10 | +1056/-60 | 26 |
| @aarshkshah1992 | 6 | +813/-259 | 16 |
| @arajasek | 10 | +552/-251 | 43 |
| @ribasushi | 6 | +505/-78 | 22 |
| @jennijuju | 7 | +212/-323 | 34 |
| @nonsense | 10 | +335/-139 | 19 |
| @dirkmc | 8 | +149/-55 | 16 |
| @hannahhoward | 4 | +56/-32 | 17 |
| @rvagg | 4 | +61/-13 | 9 |
| @jennijuju | 2 | +0/-57 | 2 |
| @hannahhoward | 1 | +33/-18 | 7 |
| @Kubuxu | 8 | +27/-16 | 9 |
| @coryschwartz | 1 | +16/-2 | 2 |
| @travisperson | 1 | +14/-0 | 1 |
| @frrist | 1 | +12/-0 | 2 |
| @ognots | 1 | +0/-10 | 2 |
| @lanzafame  | 1 | +3/-3 | 1 |
| @jennijuju | 1 | +2/-2 | 1 |
| @swift-mx | 1 | +1/-1 | 1 |

# v1.12.0 / 2021-10-12

This is a mandatory release of Lotus that introduces [Filecoin Network v14](https://github.com/filecoin-project/community/discussions/74#discussioncomment-1398542), codenamed the Chocolate upgrade. The Filecoin mainnet will upgrade at epoch 1231620, on 2021-10-26T13:30:00Z.

The Chocolate upgrade introduces the following FIPs, delivered in [v6 actors](https://github.com/filecoin-project/specs-actors/releases/tag/v6.0.0)

- [FIP-0020](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0020.md): Add return value to `WithdrawBalance`
- [FIP-0021](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0021.md): Correct quality calculation on expiration
- [FIP-0022](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0022.md): Bad deals don't fail PublishStorageDeals
- [FIP-0023](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0023.md): Break ties between tipsets of equal weight
- [FIP-0024](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0024.md): BatchBalancer & BatchDiscount Post-HyperDrive Adjustment
- [FIP-0026](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0026.md): Extend sector faulty period from 2 weeks to 6 weeks

Note that this release is built on top of lotus v1.11.3. Enterprising users like storage providers, data brokers and others are recommended to use lotus v1.13.0 for latest new features, improvements and bug fixes.

## New Features and Changes
- Implement and support [FIP-0024](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0024.md) BatchBalancer & BatchDiscount Post-HyperDrive Adjustment:
  - Precommit batch balancer support/config ([filecoin-project/lotus#7410](https://github.com/filecoin-project/lotus/pull/7410))
    - Set `BatchPreCommitAboveBaseFee` to decide whether sending out a PreCommits in individual messages or in a batch.
    - The default value of `BatchPreCommitAboveBaseFee` and `AggregateAboveBaseFee` are now updated to 0.32nanoFIL.
- The amount of FIL withdrawn from `WithdrawBalance` from miner or market via lotus CLI is now printed out upon message landing on the chain.

## Improvements
- Implement [FIP-0023](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0023.md) (Break ties between tipsets of equal weight)
  - ChainStore: Add a tiebreaker rule for tipsets of equal weight ([filecoin-project/lotus#7378](https://github.com/filecoin-project/lotus/pull/7378))
- Randomness: Move getters from ChainAPI to StateAPI ([filecoin-project/lotus#7322](https://github.com/filecoin-project/lotus/pull/7322))

## Bug Fixes
- Fix Drand fetching around null tipsets ([filecoin-project/lotus#7376](https://github.com/filecoin-project/lotus/pull/7376))

## Dependency Updates
- Add [v6 actors](https://github.com/filecoin-project/specs-actors/releases/tag/v6.0.0)
  - **Protocol changes**
    - Multisig Approve only hashes when hash in params
    - FIP 0020 WithdrawBalance methods return withdrawn value
    - FIP 0021 Fix bug in power calculation when extending verified deals sectors
    - FIP 0022 PublishStorageDeals drops errors in batch
    - FIP 0024 BatchBalancer update and burn added to PreCommitBatch
    - FIP 0026 Add FaultMaxAge extension
    - Reduce calls to power and reward actors by passing values from power cron
    - Defensive programming hardening power cron against programmer error
  - **Implementation changes**
    - Move to xerrors
    - Improved logging: burn events are not logged with reasons and burned value.
- github.com/filecoin-project/go-state-types (v0.1.1-0.20210810190654-139e0e79e69e -> v0.1.1-0.20210915140513-d354ccf10379):

## Others
- v1.12.0-rc1 prep ([filecoin-project/lotus#7426](https://github.com/filecoin-project/lotus/pull/7426)
- Extend FaultMaxAge to 6 weeks for actors v6 on test networks only ([filecoin-project/lotus#7421](https://github.com/filecoin-project/lotus/pull/7421))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @ZenGround0 | 12 | +4202/-2752 | 187 |
| @arajasek | 25 | +4567/-854 | 190 |
| @laudiacay | 4 | +1276/-435 | 37 |
| @laudiacay | 12 | +1350/-209 | 43 |
| @magik6k |  1 | +171/-13 | 8 |
| @Stebalien | 2 | +115/-12 | 6 |
| @jennijuju | 7 | +73/-34 | 26 |
| @travisperson | 2 | +19/-19 | 7 |
| @coryschwartz | 1 | +16/-2 | 2 |
| @Kubuxu | 5 | +5/-5 | 5 |
| @ribasushi | 1 | +5/-3 | 1 |

# v1.11.3 / 2021-09-29

lotus v1.11.3 is a feature release that's **highly recommended to ALL lotus users to upgrade**, including node
operators, storage providers and clients. It includes many improvements and bug fixes that result in perf
improvements in different area, like deal making, sealing and so on.

## Highlights

- 🌟🌟Introduce `MaxStagingDealsBytes - reject new deals if our staging deals area is full ([filecoin-project/lotus#7276](https://github.com/filecoin-project/lotus/pull/7276))
  - Set `MaxStagingDealsBytes` under the [Dealmaking] section of the markets' subsystem's `config.toml` to reject new incoming deals when the `deal-staging` directory of market subsystem's repo gets too large.
- 🌟🌟miner: Command to list/remove expired sectors locally ([filecoin-project/lotus#7140](https://github.com/filecoin-project/lotus/pull/7140))
  - run `./lotus-miner sectors expired -h` for more details.
- 🚀update to ffi to update-bellperson-proofs-v9-0-2 ([filecoin-project/lotus#7369](https://github.com/filecoin-project/lotus/pull/7369))
  - MinerX fellows(early testers of lotus releases) have reported faster WindowPoSt computation!
- 🌟dealpublisher: Fully validate deals before publishing ([filecoin-project/lotus#7234](https://github.com/filecoin-project/lotus/pull/7234))
  - This excludes the expired deals before sending out a PSD message which reduces the chances of PSD message failure due to invalid deals.
- 🌟Simple alert system; FD limit alerts ([filecoin-project/lotus#7108](https://github.com/filecoin-project/lotus/pull/7108))

## New Features

- feat(ci): include version/cli checks in tagged releases ([filecoin-project/lotus#7331](https://github.com/filecoin-project/lotus/pull/7331))
- Show deal sizes is sealing sectors ([filecoin-project/lotus#7261](https://github.com/filecoin-project/lotus/pull/7261))
- config for disabling NAT port mapping ([filecoin-project/lotus#7204](https://github.com/filecoin-project/lotus/pull/7204))
- Add optional mined block list to miner info ([filecoin-project/lotus#7202](https://github.com/filecoin-project/lotus/pull/7202))
- Shed: Create a verifreg command for when VRK isn't a multisig ([filecoin-project/lotus#7099](https://github.com/filecoin-project/lotus/pull/7099))

## Improvements

- build macOS CI ([filecoin-project/lotus#7307](https://github.com/filecoin-project/lotus/pull/7307))
- itests: remove cid equality comparison ([filecoin-project/lotus#7292](https://github.com/filecoin-project/lotus/pull/7292))
- Add partition info to the 'sectors status' command ([filecoin-project/lotus#7246](https://github.com/filecoin-project/lotus/pull/7246))
- chain: Cleanup consensus logic ([filecoin-project/lotus#7255](https://github.com/filecoin-project/lotus/pull/7255))
- builder: Handle chainstore config in ConfigFullNode ([filecoin-project/lotus#7232](https://github.com/filecoin-project/lotus/pull/7232))
- gateway: check tipsets in ChainGetPath ([filecoin-project/lotus#7230](https://github.com/filecoin-project/lotus/pull/7230))
- Refactor events subsystem ([filecoin-project/lotus#7000](https://github.com/filecoin-project/lotus/pull/7000))
- test: re-enable disabled tests ([filecoin-project/lotus#7211](https://github.com/filecoin-project/lotus/pull/7211))
- Reduce lotus-miner startup spam ([filecoin-project/lotus#7205](https://github.com/filecoin-project/lotus/pull/7205))
- Catch deal slashed because sector was terminated ([filecoin-project/lotus#7201](https://github.com/filecoin-project/lotus/pull/7201))
- Insert miner and network power data as gibibytes to avoid int64 overflows ([filecoin-project/lotus#7194](https://github.com/filecoin-project/lotus/pull/7194))
- sealing: Check piece CIDs after AddPiece ([filecoin-project/lotus#7185](https://github.com/filecoin-project/lotus/pull/7185))
- markets: OnDealExpiredOrSlashed - get deal by proposal instead of deal ID ([filecoin-project/lotus#5431](https://github.com/filecoin-project/lotus/pull/5431))
- Incoming: improve a log message ([filecoin-project/lotus#7181](https://github.com/filecoin-project/lotus/pull/7181))
- journal: make current log file have a fixed named (#7112) ([filecoin-project/lotus#7112](https://github.com/filecoin-project/lotus/pull/7112))
- call string.Repeat always with positive int ([filecoin-project/lotus#7104](https://github. com/filecoin-project/lotus/pull/7104))
- itests: support larger sector sizes; add large deal test. ([filecoin-project/lotus#7148](https://github.com/filecoin-project/lotus/pull/7148))
- Ignore nil throttler ([filecoin-project/lotus#7169](https://github.com/filecoin-project/lotus/pull/7169))

## Bug Fixes

- fix: escape periods to match actual periods in version
- fix bug for CommittedCapacitySectorLifetime ([filecoin-project/lotus#7337](https://github.com/filecoin-project/lotus/pull/7337))
- fix a panic in HandleRecoverDealIDs ([filecoin-project/lotus#7336](https://github.com/filecoin-project/lotus/pull/7336))
- fix index out of range ([filecoin-project/lotus#7273](https://github.com/filecoin-project/lotus/pull/7273))
- fix: correctly handle null blocks when detecting an expensive fork ([filecoin-project/lotus#7210](https://github.com/filecoin-project/lotus/pull/7210))
- fix: make lotus soup use the correct dependencies ([filecoin-project/lotus#7221](https://github.com/filecoin-project/lotus/pull/7221))
- fix: init restore adds empty storage.json ([filecoin-project/lotus#7025](https://github.com/filecoin-project/lotus/pull/7025))
- fix: disable broken testground integration test ([filecoin-project/lotus#7187](https://github.com/filecoin-project/lotus/pull/7187))
- fix TestDealPublisher ([filecoin-project/lotus#7173](https://github.com/filecoin-project/lotus/pull/7173))
- fix: make TestTimedCacheBlockstoreSimple pass reliably ([filecoin-project/lotus#7174](https://github.com/filecoin-project/lotus/pull/7174))
- Fix throttling bug ([filecoin-project/lotus#7177](https://github.com/filecoin-project/lotus/pull/7177))
- sealing: Fix sector state accounting with FinalizeEarly ([filecoin-project/lotus#7256](https://github.com/filecoin-project/lotus/pull/7256))
- docker entrypoint.sh missing variable escape character ([filecoin-project/lotus#7291](https://github.com/filecoin-project/lotus/pull/7291))
- sealing: Fix retry loop in SubmitCommitAggregate ([filecoin-project/lotus#7245](https://github.com/filecoin-project/lotus/pull/7245))
- sectors expired: Handle precomitted and unproven sectors correctly ([filecoin-project/lotus#7236](https://github.com/filecoin-project/lotus/pull/7236))
- stores: Fix reserved disk usage log spam ([filecoin-project/lotus#7233](https://github.com/filecoin-project/lotus/pull/7233))


## Dependency Updates

- github.com/filecoin-project/go-fil-markets (v1.8.1 -> v1.12.0):
- github.com/filecoin-project/go-data-transfer (v1.7.8 -> v1.10.1):
- update to ffi to update-bellperson-proofs-v9-0-2 ([filecoin-project/lotus#7369](https://github.com/filecoin-project/lotus/pull/7369))
- fix(deps): use go-graphsync v0.9.3 with hotfix
- Update to unified go-graphsync v0.9.0 ([filecoin-project/lotus#7197](https://github.com/filecoin-project/lotus/pull/7197))

## Others

- v1.11.3-rc2 ([filecoin-project/lotus#7371](https://github.com/filecoin-project/lotus/pull/7371))
- v1.11.3-rc1 ([filecoin-project/lotus#7299](https://github.com/filecoin-project/lotus/pull/7299))
- Increase threshold from 0.5% to 1% ([filecoin-project/lotus#7262](https://github.com/filecoin-project/lotus/pull/7262))
- ci: exclude cruft from code coverage ([filecoin-project/lotus#7189](https://github.com/filecoin-project/lotus/pull/7189))
- Bump version to v1.11.3-dev ([filecoin-project/lotus#7180](https://github.com/filecoin-project/lotus/pull/7180))
- test: disable flaky TestBatchDealInput ([filecoin-project/lotus#7176](https://github.com/filecoin-project/lotus/pull/7176))
- Turn off patch ([filecoin-project/lotus#7172](https://github.com/filecoin-project/lotus/pull/7172))
- test: disable flaky TestSimultaneousTransferLimit ([filecoin-project/lotus#7153](https://github.com/filecoin-project/lotus/pull/7153))


## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @magik6k | 39 | +3311/-1825 | 179 |
| @Stebalien | 23 | +1935/-1417 | 84 |
| @dirkmc | 12 | +921/-732 | 111 |
| @dirkmc | 12 | +663/-790 | 30 |
| @hannahhoward | 3 | +482/-275 | 46 |
| @travisperson | 1 | +317/-65 | 5 |
| @jennijuju | 11 | +223/-126 | 24 |
| @hannahhoward | 7 | +257/-55 | 16 |
| @nonsense| 9 | +258/-37 | 19 |
| @raulk | 4 | +127/-36 | 13 |
| @raulk | 1 | +43/-60 | 15 |
| @arajasek | 4 | +74/-8 | 10 |
| @Frank | 2 | +68/-8 | 3 |
| @placer14|  2 | +52/-1 | 4 |
| @ldoublewood | 2 | +15/-13 | 3 |
| @lanzafame | 1 | +16/-2 | 1 |
| @aarshkshah1992 | 2 | +11/-6 | 2 |
| @ZenGround0 | 2 | +7/-6 | 2 |
| @ognots | 1 | +0/-10 | 2 |
| @KAYUII | 2 | +4/-4 | 2 |
| @lanzafame | 1 | +6/-0 | 1 |
| @jacobheun | 1 | +3/-3 | 1 |
| @frank | 1 | +4/-0 | 1 |


# v1.11.2 / 2021-09-06

lotus v1.11.2 is a feature release that's **highly recommended ALL lotus users to upgrade**, including node operators,
storage providers and clients.

## Highlights
- 🌟🌟🌟 Introduce Dagstore and CARv2 for deal-making (#6671) ([filecoin-project/lotus#6671](https://github.com/filecoin-project/lotus/pull/6671))
  - **[lotus miner markets' Dagstore](https://lotus.filecoin.io/storage-providers/operate/dagstore/)** is a
    component of the `markets` subsystem in lotus-miner. It is a sharded store to hold large IPLD graphs efficiently,
    packaged as  location-transparent attachable CAR files and it replaces the former Badger staging blockstore. It
    is designed to provide high efficiency and throughput, and minimize resource utilization during deal-making operations.  
    The dagstore also leverages the indexing features of [CARv2](https://github.com/ipld/ipld/blob/master/specs/transport/car/carv2/index.md) to enable plan CAR files to act as read and write
    blockstores, which are served as the direct medium for data exchanges in markets for both storage and retrieval
    deal making without requiring intermediate buffers.
  - In the future, lotus will leverage and interact with Dagstore a lot for new features and improvements for deal
    making, therefore, it's highly recommended to lotus users to go through [Lotus Miner: About the markets dagstore](https://lotus.filecoin.io/storage-providers/operate/dagstore/) thoroughly to learn more about Dagstore's
    conceptual overview, terminology, directory structure, configuration and so on.
  - **Note**:
    - When you first start your lotus-miner or market subsystem with this release, a one-time/first-time **dagstore migration** will be triggered which replaces the former Badger staging blockstore with dagstore. We highly
      recommend storage providers to read this [section](https://lotus.filecoin.io/storage-providers/operate/dagstore/#first-time-migration) to learn more about
      what the process does, what to expect and how monitor it.
    - It is highly recommended to **wait all ongoing data transfer to finish or cancel inbound storage deals that
      are still transferring**, using the `lotus-miner data-transfers cancel` command before upgrade your market nodes. Reason being that the new dagstore changes attributes in the internal deal state objects, and the paths to the staging CARs where the deal data was being placed will be lost.
    - ‼️Having your dags initialized will become important in the near feature for you to provide a better storage
      and retrieval service. We'd suggest you to start [forced bulk initialization] soon if possible as this process
      places relatively high IP workload on your storage system and is better to be carried out gradually and over a
      longer timeframe. Read how to do properly perform a force bulk initialization [here](https://lotus.filecoin.io/storage-providers/operate/dagstore/#forcing-bulk-initialization).
    - ⏮ Rollback Alert(from v1.11.2-rcX to any version lower): If a storages deal is initiated with M1/v1.11.2(-rcX)
      release, it needs to get to the `StorageDealAwaitingPrecommit` state before you can do a version rollback or the markets process may panic.
  - 💙 **Special thanks to [MinerX fellows for testing and providing valuable feedbacks](https://github.com/filecoin-project/lotus/discussions/6852) for Dagstore in the past month!**
- 🌟🌟 rpcenc: Support reader redirect ([filecoin-project/lotus#6952](https://github.com/filecoin-project/lotus/pull/6952))
  - This allows market processes to send piece bytes directly to workers involved on `AddPiece`.
- Extending sectors: more practical and flexible tools ([filecoin-project/lotus#6097](https://github.com/filecoin-project/lotus/pull/6097))
  - `lotus-miner sectors check-expire` to inspect expiring sectors.
  - `lotus-miner sectors renew` for renewing expiring sectors, see the command help menu for customizable option
    like `extension`, `new-expiration` and so on.
- ‼️ MpoolReplaceCmd ( lotus mpool replace`) now takes FIL for fee-limit ([filecoin-project/lotus#6927](https://github.com/filecoin-project/lotus/pull/6927))
- Drop townhall/chainwatch ([filecoin-project/lotus#6912](https://github.com/filecoin-project/lotus/pull/6912))
  - ChainWatch is no longer supported by lotus.
- Configurable CC Sector Expiration ([filecoin-project/lotus#6803](https://github.com/filecoin-project/lotus/pull/6803))
  - Set `CommittedCapacitySectorLifetime` in lotus-miner/config.toml to specify the default expiration for a new CC
    sector, value must be between 180-540 days inclusive.

## New Features
- api/command for encoding actor params ([filecoin-project/lotus#7150](https://github.com/filecoin-project/lotus/pull/7150))
- shed: Support raw encoding in cid id ([filecoin-project/lotus#7149](https://github.com/filecoin-project/lotus/pull/7149))
- feat(miner deals): create subdir to miner repo for staged deals ([filecoin-project/lotus#6853](https://github.com/filecoin-project/lotus/pull/6853))
- Support --actor in miner actor control list ([filecoin-project/lotus#7027](https://github.com/filecoin-project/lotus/pull/7027))
- Shed: Include network name in genesis-verify ([filecoin-project/lotus#7019](https://github.com/filecoin-project/lotus/pull/7019))
- feat: add ChainGetTipSetAfterHeight ([filecoin-project/lotus#6990](https://github.com/filecoin-project/lotus/pull/6990))
- lotus-shed splitstore clear command ([filecoin-project/lotus#6967](https://github.com/filecoin-project/lotus/pull/6967))

## Improvements
- improve get api error messages ([filecoin-project/lotus#7088](https://github.com/filecoin-project/lotus/pull/7088))
- Strict major minor version checking on v0 and v1 apis ([filecoin-project/lotus#7038](https://github.com/filecoin-project/lotus/pull/7038))
- make lotus-miner net commands hit markets subsystem. ([filecoin-project/lotus#7042](https://github.com/filecoin-project/lotus/pull/7042))
- Test with latest actors version ([filecoin-project/lotus#6998](https://github.com/filecoin-project/lotus/pull/6998))
- Reduce splitstore memory usage during chain walks ([filecoin-project/lotus#6949](https://github.com/filecoin-project/lotus/pull/6949))
- Remove forgotten non-functioning config from the pre-mainnet days ([filecoin-project/lotus#6970](https://github.com/filecoin-project/lotus/pull/6970))
- add explicit error msg if repo dir does not exist ([filecoin-project/lotus#6909](https://github.com/filecoin-project/lotus/pull/6909))
- Test/pledge batching msg prop ([filecoin-project/lotus#6537](https://github.com/filecoin-project/lotus/pull/6537))
- reasonable max value for initial sector expiration ([filecoin-project/lotus#6099](https://github.com/filecoin-project/lotus/pull/6099))
- support MARKETS_API_INFO env var, and markets-repo, markets-api-url CLI flags. ([filecoin-project/lotus#6936](https://github.com/filecoin-project/lotus/pull/6936))
- Improve formatting of workers CLI ([filecoin-project/lotus#6942](https://github.com/filecoin-project/lotus/pull/6942))
- make: set default GOCC earlier ([filecoin-project/lotus#6932](https://github.com/filecoin-project/lotus/pull/6932))
- Moving GC Followup ([filecoin-project/lotus#6905](https://github.com/filecoin-project/lotus/pull/6905))
- Log more call context during errors ([filecoin-project/lotus#6918](https://github.com/filecoin-project/lotus/pull/6918))
- polish(errors): better state tree errors ([filecoin-project/lotus#6923](https://github.com/filecoin-project/lotus/pull/6923))
- adding an RuntimeSubsystems API to storage miner; fix `lotus-miner info` ([filecoin-project/lotus#6906](https://github.com/filecoin-project/lotus/pull/6906))
- Reduce entropy in the chain package ([filecoin-project/lotus#6889](https://github.com/filecoin-project/lotus/pull/6889))
- make: Allow setting Go compiler with GOCC ([filecoin-project/lotus#6911](https://github.com/filecoin-project/lotus/pull/6911))

## Bug Fixes
- sealing: Fix RecoverDealIDs loop with changed PieceCID ([filecoin-project/lotus#7117](https://github.com/filecoin-project/lotus/pull/7117))
- Fix error handling in SectorAddPieceToAny api impl ([filecoin-project/lotus#7135](https://github.com/filecoin-project/lotus/pull/7135))
- add rice box to required binaries ([filecoin-project/lotus#7125](https://github.com/filecoin-project/lotus/pull/7125))
- fix(miner): always create miner deal staging directory (#7098) ([filecoin-project/lotus#7098](https://github.com/filecoin-project/lotus/pull/7098))
- fix build after merging #6097. (#7096) ([filecoin-project/lotus#7096](https://github.com/filecoin-project/lotus/pull/7096))
- fix: don't check for t_aux when proving ([filecoin-project/lotus#7011](https://github.com/filecoin-project/lotus/pull/7011))
- fix: vet actors shims ([filecoin-project/lotus#6999](https://github.com/filecoin-project/lotus/pull/6999))
- fix: more logging in maybeStartBatch error ([filecoin-project/lotus#6996](https://github.com/filecoin-project/lotus/pull/6996))
- fix flaky TestDealPublisher and re-enable ([filecoin-project/lotus#6991](https://github.com/filecoin-project/lotus/pull/6991))
- fix skipCount ([filecoin-project/lotus#6940](https://github.com/filecoin-project/lotus/pull/6940))
- fix bug in MpoolPending message exclusion ([filecoin-project/lotus#6945](https://github.com/filecoin-project/lotus/pull/6945))
- PreCommitPolicy: Don't try to align expirations on proving period boundaries ([filecoin-project/lotus#7018](https://github.com/filecoin-project/lotus/pull/7018))
- make: fix version check when using gotip ([filecoin-project/lotus#6916](https://github.com/filecoin-project/lotus/pull/6916))
- fix ticket check ([filecoin-project/lotus#6882](https://github.com/filecoin-project/lotus/pull/6882))

## Dependency Updates
- github.com/filecoin-project/go-data-transfer (v1.7.2 -> v1.7.8):
- github.com/filecoin-project/go-fil-markets (v1.6.2 -> v1.8.1):
- update go-libp2p-pubsub to v0.5.4 ([filecoin-project/lotus#6958](https://github.com/filecoin-project/lotus/pull/6958))
- integrate the proof patch: tag proofs-v9-revert-deps-hotfix
- Update markets, dt and graphsync ([filecoin-project/lotus#7160](https://github.com/filecoin-project/lotus/pull/7160))
- Remove replace directive for multihash dep (#7113) ([filecoin-project/lotus#7113](https://github.com/filecoin-project/lotus/pull/7113))
- upgrade upstream dependencies. ([filecoin-project/lotus#7115](https://github.com/filecoin-project/lotus/pull/7115))
- Update to latest FFI ([filecoin-project/lotus#7110](https://github.com/filecoin-project/lotus/pull/7110))
- Update state machine deps for logging ([filecoin-project/lotus#6941](https://github.com/filecoin-project/lotus/pull/6941))
- Update deps for more logging in data transfer and markets ([filecoin-project/lotus#6937](https://github.com/filecoin-project/lotus/pull/6937))
- Update to branches with improved logging ([filecoin-project/lotus#6919](https://github.com/filecoin-project/lotus/pull/6919))
- update go-libp2p-pubsub to v0.5.3 ([filecoin-project/lotus#6907](https://github.com/filecoin-project/lotus/pull/6907))

## Others
- Fix nits and see if codecov works now with auto ([filecoin-project/lotus#7151](https://github.com/filecoin-project/lotus/pull/7151))
- Codecov Projects ([filecoin-project/lotus#7147](https://github.com/filecoin-project/lotus/pull/7147))
- remove m1 templates and make area selection multi-optionable ([filecoin-project/lotus#7121](https://github.com/filecoin-project/lotus/pull/7121))
- release -> master ([filecoin-project/lotus#7105](https://github.com/filecoin-project/lotus/pull/7105))
- Lotus release process  - how we make releases ([filecoin-project/lotus#6944](https://github.com/filecoin-project/lotus/pull/6944))
- codecov: fix mock name ([filecoin-project/lotus#7039](https://github.com/filecoin-project/lotus/pull/7039))
- codecov: fix regexes ([filecoin-project/lotus#7037](https://github.com/filecoin-project/lotus/pull/7037))
- chore: disable flaky test ([filecoin-project/lotus#6957](https://github.com/filecoin-project/lotus/pull/6957))
- set buildtype in nerpa and butterfly ([filecoin-project/lotus#6085](https://github.com/filecoin-project/lotus/pull/6085))
- release v1.11.1 backport -> master ([filecoin-project/lotus#6929](https://github.com/filecoin-project/lotus/pull/6929))
- chore: fixup issue templates ([filecoin-project/lotus#6899](https://github.com/filecoin-project/lotus/pull/6899))
- bump master version to v1.11.2-dev ([filecoin-project/lotus#6903](https://github.com/filecoin-project/lotus/pull/6903))
- releases -> master for v1.11.0 ([filecoin-project/lotus#6894](https://github.com/filecoin-project/lotus/pull/6894))


Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @magik6k | 23 | +5040/-8389 | 114 |
| @aarshkshah1992 | 11 | +4859/-1078 | 101 |
| @raulk | 5 | +4170/-1662 | 104 |
| @vyzo | 30 | +1092/-702 | 49 |
| @nonsense | 6 | +630/-472 | 19 |
| @ZenGround0 | 31 | +556/-274 | 74 |
| @He Weidong | 16 | +680/-128 | 16 |
| @raulk | 16 | +444/-277 | 49 |
| @Stebalien | 11 | +403/-259 | 48 |
| @jennijuju| 17 | +276/-281 | 42 |
| @dirkmc | 5 | +204/-138 | 20 |
| @placer14 | 7 | +178/-77 | 17 |
| @BlocksOnAChain | 1 | +138/-0 | 1 |
| @Frrist | 1 | +63/-56 | 2 |
| @arajasek | 7 | +74/-42 | 13 |
| @frrist | 2 | +67/-6 | 6 |
| @hannahhoward | 2 | +13/-11 | 3 |
| @coryschwartz | 1 | +16/-6 | 3 |
| @whyrusleeping | 1 | +7/-7 | 1 |
| @hunjixin | 1 | +8/-6 | 1 |
| @aarshkshah1992 | 1 | +6/-6 | 2 |
| @dirkmc | 2 | +8/-0 | 2 |
| @mx | 2 | +6/-1 | 2 |
| @travisperson | 1 | +3/-2 | 1 |
| @jennijuju | 2 | +2/-2 | 2 |
| @ribasushi | 1 | +1/-2 | 2 |

# 1.11.1 / 2021-08-16

> Note: for discussion about this release, please comment [here](https://github.com/filecoin-project/lotus/discussions/6904)

This is a  **highly recommended** but optional Lotus v1.11.1 release that introduces many deal making and datastore improvements and new features along with other bug fixes.

## Highlights
- ⭐️⭐️⭐️[**lotus-miner market subsystem**](https://lotus.filecoin.io/storage-providers/advanced-configurations/split-markets-miners/) is introduced in this release! It is **highly recommended** for storage providers to run markets processes on a separate machine! Doing so, only this machine needs to exposes public ports for deal making. This also means that the other miner operations can now be completely isolated by from the deal making processes and storage providers can stop and restarts the markets process without affecting an ongoing Winning/Window PoSt!
  - More details on the concepts, architecture and how to split the market process can be found [here](https://lotus.filecoin.io/storage-providers/advanced-configurations/split-markets-miners/#concepts).
  - Base on your system setup(running on separate machines, same machine and so on), please see the suggested practice by community members [here](https://github.com/filecoin-project/lotus/discussions/7047#discussion-3515335).
    - Note: if you are running lotus-worker on a different machine, you will need to set `MARKETS_API_INFO` for certain CLI to work properly. This will be improved by #7072.
  - Huge thanks to MinerX fellows for [helping testing the implementation, reporting the issues so they were fixed by now and providing feedbacks](https://github.com/filecoin-project/lotus/discussions/6861) to user docs in the past three weeks!
- Config for collateral from miner available balance ([filecoin-project/lotus#6629](https://github.com/filecoin-project/lotus/pull/6629))
  - Better control your sector collateral payment by setting `CollateralFromMinerBalance`, `AvailableBalanceBuffer` and `DisableCollateralFallback`.
    - `CollateralFromMinerBalance`:  whether to use available miner balance for sector collateral instead of sending it with each message, default is `false`.
    - `AvailableBalanceBuffer`: minimum available balance to keep in the miner actor before sending it with messages, default is 0FIL.
    - `DisableCollateralFallback`: whether to send collateral with messages even if there is no available balance in the miner actor, default is `false`.
- Config for deal publishing control addresses ([filecoin-project/lotus#6697](https://github.com/filecoin-project/lotus/pull/6697))
  - Set `DealPublishControl` to set the wallet used for sending `PublishStorageDeals` messages, instructions [here](https://lotus.filecoin.io/storage-providers/operate/addresses/#control-addresses).
- Config UX improvements ([filecoin-project/lotus#6848](https://github.com/filecoin-project/lotus/pull/6848))
  - You can now preview the default and updated node config by running `lotus/lotus-miner config default/updated`

## New Features
- ⭐️⭐️⭐️ Support standalone miner-market process ([filecoin-project/lotus#6356](https://github.com/filecoin-project/lotus/pull/6356))
- **⭐️⭐️ Experimental** [Splitstore]((https://github.com/filecoin-project/lotus/blob/master/blockstore/splitstore/README.md)) (more details coming in v1.11.2! Stay tuned! Join the discussion [here](https://github.com/filecoin-project/lotus/discussions/5788) if you have questions!) :
  - Improve splitstore warmup ([filecoin-project/lotus#6867](https://github.com/filecoin-project/lotus/pull/6867))
  - Moving GC for badger ([filecoin-project/lotus#6854](https://github.com/filecoin-project/lotus/pull/6854))
  - splitstore shed utils ([filecoin-project/lotus#6811](https://github.com/filecoin-project/lotus/pull/6811))
  - fix warmup by decoupling state from message receipt walk ([filecoin-project/lotus#6841](https://github.com/filecoin-project/lotus/pull/6841))
  - Splitstore: support on-disk marksets using badger ([filecoin-project/lotus#6833](https://github.com/filecoin-project/lotus/pull/6833))
  - cache loaded block messages ([filecoin-project/lotus#6760](https://github.com/filecoin-project/lotus/pull/6760))
  - Splitstore: add retention policy option for keeping messages in the hotstore ([filecoin-project/lotus#6775](https://github.com/filecoin-project/lotus/pull/6775))
  - Introduce the LOTUS_CHAIN_BADGERSTORE_DISABLE_FSYNC envvar ([filecoin-project/lotus#6817](https://github.com/filecoin-project/lotus/pull/6817))
  - Splitstore: add support for protecting out of chain references in the blockstore ([filecoin-project/lotus#6777](https://github.com/filecoin-project/lotus/pull/6777))
  - Implement exposed splitstore ([filecoin-project/lotus#6762](https://github.com/filecoin-project/lotus/pull/6762))
  - Splitstore code reorg ([filecoin-project/lotus#6756](https://github.com/filecoin-project/lotus/pull/6756))
  - Splitstore: Some small fixes ([filecoin-project/lotus#6754](https://github.com/filecoin-project/lotus/pull/6754))
  - Splitstore Enhanchements ([filecoin-project/lotus#6474](https://github.com/filecoin-project/lotus/pull/6474))
- lotus-shed: initial export cmd for markets related metadata ([filecoin-project/lotus#6840](https://github.com/filecoin-project/lotus/pull/6840))
- add a very verbose -vv flag to lotus and lotus-miner. ([filecoin-project/lotus#6888](https://github.com/filecoin-project/lotus/pull/6888))
- Add allocated sectorid vis ([filecoin-project/lotus#4638](https://github.com/filecoin-project/lotus/pull/4638))
- add a command for compacting sector numbers bitfield ([filecoin-project/lotus#4640](https://github.com/filecoin-project/lotus/pull/4640))
  - Run `lotus-miner actor compact-allocated` to compact sector number allocations to reduce the size of the allocated sector number bitfield.
- Add ChainGetMessagesInTipset API ([filecoin-project/lotus#6642](https://github.com/filecoin-project/lotus/pull/6642))
- Handle the --color flag via proper global state ([filecoin-project/lotus#6743](https://github.com/filecoin-project/lotus/pull/6743))
  - Enable color by default only if os.Stdout is a TTY ([filecoin-project/lotus#6696](https://github.com/filecoin-project/lotus/pull/6696))
  - Stop outputing ANSI color on non-TTY ([filecoin-project/lotus#6694](https://github.com/filecoin-project/lotus/pull/6694))
- Envvar to disable slash filter ([filecoin-project/lotus#6620](https://github.com/filecoin-project/lotus/pull/6620))
- commit batch: AggregateAboveBaseFee config ([filecoin-project/lotus#6650](https://github.com/filecoin-project/lotus/pull/6650))
- shed tool to estimate aggregate network fees ([filecoin-project/lotus#6631](https://github.com/filecoin-project/lotus/pull/6631))

## Bug Fixes
- Fix padding of deals, which only partially shipped in #5988 ([filecoin-project/lotus#6683](https://github.com/filecoin-project/lotus/pull/6683))
- fix deal concurrency test failures by upgrading graphsync and others ([filecoin-project/lotus#6724](https://github.com/filecoin-project/lotus/pull/6724))
- fix: on randomness change, use new rand ([filecoin-project/lotus#6805](https://github.com/filecoin-project/lotus/pull/6805))  - fix: always check if StateSearchMessage returns nil ([filecoin-project/lotus#6802](https://github.com/filecoin-project/lotus/pull/6802))
- test: fix flaky window post tests ([filecoin-project/lotus#6804](https://github.com/filecoin-project/lotus/pull/6804))
-  wrap close(wait) with sync.Once to avoid panic ([filecoin-project/lotus#6800](https://github.com/filecoin-project/lotus/pull/6800))
- fixes #6786 segfault ([filecoin-project/lotus#6787](https://github.com/filecoin-project/lotus/pull/6787))
-  ClientRetrieve stops on cancel([filecoin-project/lotus#6739](https://github.com/filecoin-project/lotus/pull/6739))
- Fix bugs in sectors extend --v1-sectors ([filecoin-project/lotus#6066](https://github.com/filecoin-project/lotus/pull/6066))
- fix "lotus-seed genesis car" error "merkledag: not found" ([filecoin-project/lotus#6688](https://github.com/filecoin-project/lotus/pull/6688))
- Get retrieval pricing input should not error out on a deal state fetch ([filecoin-project/lotus#6679](https://github.com/filecoin-project/lotus/pull/6679))
- Fix more CID double-encoding as hex ([filecoin-project/lotus#6680](https://github.com/filecoin-project/lotus/pull/6680))
- storage: Fix FinalizeSector with sectors in stoage paths ([filecoin-project/lotus#6653](https://github.com/filecoin-project/lotus/pull/6653))
- Fix tiny error in check-client-datacap ([filecoin-project/lotus#6664](https://github.com/filecoin-project/lotus/pull/6664))
- Fix: precommit_batch method used the wrong cfg.CommitBatchWait ([filecoin-project/lotus#6658](https://github.com/filecoin-project/lotus/pull/6658))
- fix ticket expiration check ([filecoin-project/lotus#6635](https://github.com/filecoin-project/lotus/pull/6635))
- remove precommit check in handleCommitFailed ([filecoin-project/lotus#6634](https://github.com/filecoin-project/lotus/pull/6634))
- fix prove commit aggregate send token amount ([filecoin-project/lotus#6625](https://github.com/filecoin-project/lotus/pull/6625))

## Improvements
- Eliminate inefficiency in markets logging ([filecoin-project/lotus#6895](https://github.com/filecoin-project/lotus/pull/6895))
- rename `cmd/lotus{-storage=>}-miner` to match binary. ([filecoin-project/lotus#6886](https://github.com/filecoin-project/lotus/pull/6886))
- fix racy TestSimultanenousTransferLimit. ([filecoin-project/lotus#6862](https://github.com/filecoin-project/lotus/pull/6862))
- ValidateBlock: Assert that block header height's are greater than parents ([filecoin-project/lotus#6872](https://github.com/filecoin-project/lotus/pull/6872))
- feat: Don't panic when api impl is nil ([filecoin-project/lotus#6857](https://github.com/filecoin-project/lotus/pull/6857))
- add docker-compose file ([filecoin-project/lotus#6544](https://github.com/filecoin-project/lotus/pull/6544))
- easy way to make install app ([filecoin-project/lotus#5183](https://github.com/filecoin-project/lotus/pull/5183))
- api: Separate the Net interface from Common ([filecoin-project/lotus#6627](https://github.com/filecoin-project/lotus/pull/6627))  - add StateReadState to gateway api ([filecoin-project/lotus#6818](https://github.com/filecoin-project/lotus/pull/6818))
- add SealProof in SectorBuilder ([filecoin-project/lotus#6815](https://github.com/filecoin-project/lotus/pull/6815))
- sealing: Handle preCommitParams errors more correctly ([filecoin-project/lotus#6763](https://github.com/filecoin-project/lotus/pull/6763))
- ClientFindData: always fetch peer id from chain ([filecoin-project/lotus#6807](https://github.com/filecoin-project/lotus/pull/6807))
- test: handle null blocks in TestForkRefuseCall ([filecoin-project/lotus#6758](https://github.com/filecoin-project/lotus/pull/6758))
- Add more deal details to lotus-miner info ([filecoin-project/lotus#6708](https://github.com/filecoin-project/lotus/pull/6708))
- add election backtest ([filecoin-project/lotus#5950](https://github.com/filecoin-project/lotus/pull/5950))
- add dollar sign ([filecoin-project/lotus#6690](https://github.com/filecoin-project/lotus/pull/6690))
- get-actor cli spelling fix ([filecoin-project/lotus#6681](https://github.com/filecoin-project/lotus/pull/6681))
- polish(statetree): accept a context in statetree diff for timeouts ([filecoin-project/lotus#6639](https://github.com/filecoin-project/lotus/pull/6639))
- Add helptext to lotus chain export ([filecoin-project/lotus#6672](https://github.com/filecoin-project/lotus/pull/6672))
- add an incremental nonce itest. ([filecoin-project/lotus#6663](https://github.com/filecoin-project/lotus/pull/6663))
- commit batch: Initialize the FailedSectors map ([filecoin-project/lotus#6647](https://github.com/filecoin-project/lotus/pull/6647))
- Fast-path retry submitting commit aggregate if commit is still valid  ([filecoin-project/lotus#6638](https://github.com/filecoin-project/lotus/pull/6638))
- Reuse timers in sealing batch logic ([filecoin-project/lotus#6636](https://github.com/filecoin-project/lotus/pull/6636))

## Dependency Updates
- Update to proof v8.0.3 ([filecoin-project/lotus#6890](https://github.com/filecoin-project/lotus/pull/6890))
- update to go-fil-market v1.6.0 ([filecoin-project/lotus#6885](https://github.com/filecoin-project/lotus/pull/6885))
- Bump go-multihash, adjust test for supported version ([filecoin-project/lotus#6674](https://github.com/filecoin-project/lotus/pull/6674))
- github.com/filecoin-project/go-data-transfer (v1.6.0 -> v1.7.2):
- github.com/filecoin-project/go-fil-markets (v1.5.0 -> v1.6.2):
- github.com/filecoin-project/go-padreader (v0.0.0-20200903213702-ed5fae088b20 -> v0.0.0-20210723183308-812a16dc01b1)
- github.com/filecoin-project/go-state-types (v0.1.1-0.20210506134452-99b279731c48 -> v0.1.1-0.20210810190654-139e0e79e69e)
- github.com/filecoin-project/go-statemachine (v0.0.0-20200925024713-05bd7c71fbfe -> v1.0.1)
- update go-libp2p-pubsub to v0.5.0 ([filecoin-project/lotus#6764](https://github.com/filecoin-project/lotus/pull/6764))

## Others
- Master->v1.11.1 ([filecoin-project/lotus#7051](https://github.com/filecoin-project/lotus/pull/7051))
- v1.11.1-rc2 ([filecoin-project/lotus#6966](https://github.com/filecoin-project/lotus/pull/6966))
- Backport master -> v1.11.1 ([filecoin-project/lotus#6965](https://github.com/filecoin-project/lotus/pull/6965))
- Fixes in master -> release ([filecoin-project/lotus#6933](https://github.com/filecoin-project/lotus/pull/6933))
- Add changelog  for v1.11.1-rc1 and bump the version ([filecoin-project/lotus#6900](https://github.com/filecoin-project/lotus/pull/6900))
- Fix  merge release -> v1.11.1 ([filecoin-project/lotus#6897](https://github.com/filecoin-project/lotus/pull/6897))
- Update RELEASE_ISSUE_TEMPLATE.md ([filecoin-project/lotus#6880](https://github.com/filecoin-project/lotus/pull/6880))
- Add github actions for staled pr ([filecoin-project/lotus#6879](https://github.com/filecoin-project/lotus/pull/6879))
- Update issue templates and add templates for M1 ([filecoin-project/lotus#6856](https://github.com/filecoin-project/lotus/pull/6856))
- Fix links in issue templates
- Update issue templates to forms ([filecoin-project/lotus#6798](https://github.com/filecoin-project/lotus/pull/6798)
- Nerpa v13 upgrade ([filecoin-project/lotus#6837](https://github.com/filecoin-project/lotus/pull/6837))
- add docker-compose file ([filecoin-project/lotus#6544](https://github.com/filecoin-project/lotus/pull/6544))
- release -> master ([filecoin-project/lotus#6828](https://github.com/filecoin-project/lotus/pull/6828))
- Resurrect CODEOWNERS, but for maintainers group ([filecoin-project/lotus#6773](https://github.com/filecoin-project/lotus/pull/6773))
- Master disclaimer  ([filecoin-project/lotus#6757](https://github.com/filecoin-project/lotus/pull/6757))
- Create stale.yml ([filecoin-project/lotus#6747](https://github.com/filecoin-project/lotus/pull/6747))
- Release template: Update all testnet infra at once ([filecoin-project/lotus#6710](https://github.com/filecoin-project/lotus/pull/6710))
- Release Template: remove binary validation step ([filecoin-project/lotus#6709](https://github.com/filecoin-project/lotus/pull/6709))
- Reset of the interop network ([filecoin-project/lotus#6689](https://github.com/filecoin-project/lotus/pull/6689))
- Update version.go to 1.11.1 ([filecoin-project/lotus#6621](https://github.com/filecoin-project/lotus/pull/6621))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @vyzo | 313 | +8928/-6010 | 415 |
| @nonsense | 103 | +6041/-4041 | 304 |
| @magik6k | 37 | +3851/-1611 | 146 |
| @ZenGround0 | 24 | +1693/-1394 | 95 |
| @placer14 | 1 | +2310/-578 | 8 |
| @dirkmc | 7 | +1154/-726 | 29 |
| @raulk | 44 | +969/-616 | 141 |
| @jennijuju | 15 | +682/-354 | 47 |
| @ribasushi | 18 | +469/-273 | 64 |
| @coryschwartz | 5 | +576/-135 | 14 |
| @hunjixin | 7 | +404/-82 | 19 |
| @dirkmc | 17 | +348/-47 | 17 |
| @tchardin | 2 | +262/-34 | 5 |
| @aarshkshah1992 | 9 | +233/-63 | 44 |
| @Kubuxu | 4 | +254/-16 | 4 |
| @hannahhoward | 6 | +163/-75 | 8 |
| @whyrusleeping | 4 | +157/-16 | 9 |
| @Whyrusleeping | 2 | +87/-66 | 10 |
| @arajasek | 10 | +81/-53 | 13 |
| @zgfzgf | 2 | +104/-4 | 2 |
| @aarshkshah1992 | 6 | +85/-19 | 10 |
| @llifezou | 4 | +59/-20 | 4 |
| @Stebalien | 7 | +47/-17 | 9 |
| @johnli-helloworld | 3 | +46/-15 | 5 |
| @frrist | 1 | +28/-23 | 2 |
| @hannahhoward | 4 | +46/-5 | 11 |
| @Jennifer | 4 | +31/-2 | 4 |
| @wangchao | 1 | +1/-27 | 1 |
| @jennijuju | 2 | +7/-21 | 2 |
| @chadwick2143 | 1 | +15/-1 | 1 |
| @Jerry | 2 | +9/-4 | 2 |
| Steve Loeppky | 2 | +12/-0 | 2 |
| David Dias | 1 | +9/-0 | 1 |
| dependabot[bot] | 1 | +3/-3 | 1 |
| zhoutian527 | 1 | +2/-2 | 1 |
| xloem | 1 | +4/-0 | 1 |
| | 2 | +2/-2 | 3 |
| Liviu Damian | 2 | +2/-2 | 2 |
| @jimpick | 2 | +2/-2 | 2 |
| Frank | 1 | +3/-0 | 1 |
| turuslan | 1 | +1/-1 | 1 |
| Kirk Baird | 1 | +0/-0 | 1 |

# 1.11.0 / 2021-07-22

This is a **highly recommended** release of Lotus that have many bug fixes, improvements and new features.

## Highlights
- Miner SimultaneousTransfers config ([filecoin-project/lotus#6612](https://github.com/filecoin-project/lotus/pull/6612))
- Miner SimultaneousTransfers config ([filecoin-project/lotus#6612](https://github.com/filecoin-project/lotus/pull/6612))
  - Set `SimultaneousTransfers` in lotus miner config to configure the maximum number of parallel online data transfers, including both storage and retrieval deals.
- Dynamic Retrieval pricing ([filecoin-project/lotus#6175](https://github.com/filecoin-project/lotus/pull/6175))
  - Customize your retrieval ask price, see a quick tutorial [here](https://github.com/filecoin-project/lotus/discussions/6780).
- Robust message management  ([filecoin-project/lotus#5822](https://github.com/filecoin-project/lotus/pull/5822))
  - run `lotus mpool manage and follow the instructions!
  - Demo available at https://www.youtube.com/watch?v=QDocpLQjZgQ.
- Add utils to use multisigs as miner owners ([filecoin-project/lotus#6490](https://github.com/filecoin-project/lotus/pull/6490))

## More New Features
-  feat: implement lotus-sim ([filecoin-project/lotus#6406](https://github.com/filecoin-project/lotus/pull/6406))
- implement a command to export a car ([filecoin-project/lotus#6405](https://github.com/filecoin-project/lotus/pull/6405))
- Add a command to get the fees of a deal ([filecoin-project/lotus#5307](https://github.com/filecoin-project/lotus/pull/5307))
  - run `lotus-shed market get-deal-fees`
- Add a command to list retrievals ([filecoin-project/lotus#6337](https://github.com/filecoin-project/lotus/pull/6337))
  - run `lotus client list-retrievals`
- lotus-gateway: add check command ([filecoin-project/lotus#6373](https://github.com/filecoin-project/lotus/pull/6373))
- lotus-wallet: JWT Support ([filecoin-project/lotus#6360](https://github.com/filecoin-project/lotus/pull/6360))
- Allow starting networks from arbitrary actor versions ([filecoin-project/lotus#6305](https://github.com/filecoin-project/lotus/pull/6305))
- oh, snap! ([filecoin-project/lotus#6202](https://github.com/filecoin-project/lotus/pull/6202))
- Add a shed util to count 64 GiB miner stats ([filecoin-project/lotus#6290](https://github.com/filecoin-project/lotus/pull/6290))
- Introduce stateless offline dealflow, bypassing the FSM/deallists ([filecoin-project/lotus#5961](https://github.com/filecoin-project/lotus/pull/5961))
- Transplant some useful commands to lotus-shed actor ([filecoin-project/lotus#5913](https://github.com/filecoin-project/lotus/pull/5913))
  - run `lotus-shed actor`
- actor wrapper codegen ([filecoin-project/lotus#6108](https://github.com/filecoin-project/lotus/pull/6108))
- Add a shed util to count miners by post type ([filecoin-project/lotus#6169](https://github.com/filecoin-project/lotus/pull/6169))
- shed: command to list duplicate messages in tipsets (steb) ([filecoin-project/lotus#5847](https://github.com/filecoin-project/lotus/pull/5847))
- feat: allow checkpointing to forks ([filecoin-project/lotus#6107](https://github.com/filecoin-project/lotus/pull/6107))
- Add a CLI tool for miner proving deadline ([filecoin-project/lotus#6132](https://github.com/filecoin-project/lotus/pull/6132))
  - run `lotus state miner-proving-deadline`


## Bug Fixes
- Fix wallet error messages ([filecoin-project/lotus#6594](https://github.com/filecoin-project/lotus/pull/6594))
- Fix CircleCI gen ([filecoin-project/lotus#6589](https://github.com/filecoin-project/lotus/pull/6589))
- Make query-ask CLI more graceful ([filecoin-project/lotus#6590](https://github.com/filecoin-project/lotus/pull/6590))
- scale up sector expiration to avoid sector expire in batch-pre-commit waitting ([filecoin-project/lotus#6566](https://github.com/filecoin-project/lotus/pull/6566))
-  Fix an error in msigLockCancel ([filecoin-project/lotus#6582](https://github.com/filecoin-project/lotus/pull/6582)
- fix circleci being out of sync. ([filecoin-project/lotus#6573](https://github.com/filecoin-project/lotus/pull/6573))
- Fix helptext for ask price([filecoin-project/lotus#6560](https://github.com/filecoin-project/lotus/pull/6560))
- fix commit finalize failed ([filecoin-project/lotus#6521](https://github.com/filecoin-project/lotus/pull/6521))
- Fix soup ([filecoin-project/lotus#6501](https://github.com/filecoin-project/lotus/pull/6501))
- fix: pick the correct partitions-per-post limit ([filecoin-project/lotus#6502](https://github.com/filecoin-project/lotus/pull/6502))
- sealing: Fix restartSectors race ([filecoin-project/lotus#6495](https://github.com/filecoin-project/lotus/pull/6495))
- Fix: correct the change of message size limit ([filecoin-project/lotus#6430](https://github.com/filecoin-project/lotus/pull/6430))
- Fix logging of stringified CIDs double-encoded in hex ([filecoin-project/lotus#6413](https://github.com/filecoin-project/lotus/pull/6413))
- Fix success handling in Retreival ([filecoin-project/lotus#5921](https://github.com/filecoin-project/lotus/pull/5921))
- storagefsm: Fix batch deal packing behavior ([filecoin-project/lotus#6041](https://github.com/filecoin-project/lotus/pull/6041))
- events: Fix handling of multiple matched events per epoch ([filecoin-project/lotus#6355](https://github.com/filecoin-project/lotus/pull/6355))
- Fix logging around mineOne ([filecoin-project/lotus#6310](https://github.com/filecoin-project/lotus/pull/6310))
- Fix shell completions ([filecoin-project/lotus#6316](https://github.com/filecoin-project/lotus/pull/6316))
- Allow 8MB sectors in devnet ([filecoin-project/lotus#6312](https://github.com/filecoin-project/lotus/pull/6312))
- fix ticket expired ([filecoin-project/lotus#6304](https://github.com/filecoin-project/lotus/pull/6304))
- Revert "chore: update go-libp2p" ([filecoin-project/lotus#6306](https://github.com/filecoin-project/lotus/pull/6306))
- fix: wait-api should use GetAPI to acquire binary specific API ([filecoin-project/lotus#6246](https://github.com/filecoin-project/lotus/pull/6246))
- fix(ci): Updates to lotus CI build process ([filecoin-project/lotus#6256](https://github.com/filecoin-project/lotus/pull/6256))
- fix: use a consistent tipset in commands ([filecoin-project/lotus#6142](https://github.com/filecoin-project/lotus/pull/6142))
- go mod tidy for lotus-soup testplans ([filecoin-project/lotus#6124](https://github.com/filecoin-project/lotus/pull/6124))
- fix testground payment channel tests: use 1 miner ([filecoin-project/lotus#6126](https://github.com/filecoin-project/lotus/pull/6126))
- fix: use the parent state when listing actors ([filecoin-project/lotus#6143](https://github.com/filecoin-project/lotus/pull/6143))
- Speed up StateListMessages in some cases ([filecoin-project/lotus#6007](https://github.com/filecoin-project/lotus/pull/6007))
- fix(splitstore): fix a panic on revert-only head changes ([filecoin-project/lotus#6133](https://github.com/filecoin-project/lotus/pull/6133))
- drand: fix beacon cache ([filecoin-project/lotus#6164](https://github.com/filecoin-project/lotus/pull/6164))

## Improvements
- gateway: Add support for Version method ([filecoin-project/lotus#6618](https://github.com/filecoin-project/lotus/pull/6618))
- revamped integration test kit (aka. Operation Sparks Joy) ([filecoin-project/lotus#6329](https://github.com/filecoin-project/lotus/pull/6329))
- move with changed name ([filecoin-project/lotus#6587](https://github.com/filecoin-project/lotus/pull/6587))
- dynamic circleci config for streamlining test execution ([filecoin-project/lotus#6561](https://github.com/filecoin-project/lotus/pull/6561))
- extern/storage: add ability to ignore worker resources when scheduling. ([filecoin-project/lotus#6542](https://github.com/filecoin-project/lotus/pull/6542))
- Adjust various CLI display ratios to arbitrary precision ([filecoin-project/lotus#6309](https://github.com/filecoin-project/lotus/pull/6309))
- Test multicore SDR support ([filecoin-project/lotus#6479](https://github.com/filecoin-project/lotus/pull/6479))
- Unit tests for sector batchers ([filecoin-project/lotus#6432](https://github.com/filecoin-project/lotus/pull/6432))
- Update chain list with correct help instructions ([filecoin-project/lotus#6465](https://github.com/filecoin-project/lotus/pull/6465))
- clean failed sectors in batch commit ([filecoin-project/lotus#6451](https://github.com/filecoin-project/lotus/pull/6451))
- itests/kit: add guard to ensure imports from tests only. ([filecoin-project/lotus#6445](https://github.com/filecoin-project/lotus/pull/6445))
- consolidate integration tests into `itests` package; create test kit; cleanup ([filecoin-project/lotus#6311](https://github.com/filecoin-project/lotus/pull/6311))
- Fee config for sector batching ([filecoin-project/lotus#6420](https://github.com/filecoin-project/lotus/pull/6420))
- UX: lotus state power CLI should fail if called with a not-miner ([filecoin-project/lotus#6425](https://github.com/filecoin-project/lotus/pull/6425))
- Increase message size limit ([filecoin-project/lotus#6419](https://github.com/filecoin-project/lotus/pull/6419))
- polish(stmgr): define ExecMonitor for message application callback ([filecoin-project/lotus#6389](https://github.com/filecoin-project/lotus/pull/6389))
- upgrade testground action version ([filecoin-project/lotus#6403](https://github.com/filecoin-project/lotus/pull/6403))
- Bypass task scheduler for reading unsealed pieces ([filecoin-project/lotus#6280](https://github.com/filecoin-project/lotus/pull/6280))
- testplans: lotus-soup: use default WPoStChallengeWindow ([filecoin-project/lotus#6400](https://github.com/filecoin-project/lotus/pull/6400))
- Integration tests for offline deals ([filecoin-project/lotus#6081](https://github.com/filecoin-project/lotus/pull/6081))
- Fix some flaky tests ([filecoin-project/lotus#6397](https://github.com/filecoin-project/lotus/pull/6397))
- build appimage in CI ([filecoin-project/lotus#6384](https://github.com/filecoin-project/lotus/pull/6384))
- Generate AppImage ([filecoin-project/lotus#6208](https://github.com/filecoin-project/lotus/pull/6208))
- Add test for AddVerifiedClient ([filecoin-project/lotus#6317](https://github.com/filecoin-project/lotus/pull/6317))
- Typo fix in error message: "pubusb" -> "pubsub" ([filecoin-project/lotus#6365](https://github.com/filecoin-project/lotus/pull/6365))
- Improve the cli state call command  ([filecoin-project/lotus#6226](https://github.com/filecoin-project/lotus/pull/6226))
- Upscale mineOne message to a WARN on unexpected ineligibility ([filecoin-project/lotus#6358](https://github.com/filecoin-project/lotus/pull/6358))
- Remove few useless variable assignments ([filecoin-project/lotus#6359](https://github.com/filecoin-project/lotus/pull/6359))
- Reduce noise from 'peer has different genesis' messages ([filecoin-project/lotus#6357](https://github.com/filecoin-project/lotus/pull/6357))
- Get current seal proof when necessary ([filecoin-project/lotus#6339](https://github.com/filecoin-project/lotus/pull/6339))
- Remove log line when tracing is not configured ([filecoin-project/lotus#6334](https://github.com/filecoin-project/lotus/pull/6334))
- separate tracing environment variables ([filecoin-project/lotus#6323](https://github.com/filecoin-project/lotus/pull/6323))
- feat: log dispute rate ([filecoin-project/lotus#6322](https://github.com/filecoin-project/lotus/pull/6322))
- Move verifreg shed utils to CLI ([filecoin-project/lotus#6135](https://github.com/filecoin-project/lotus/pull/6135))
- consider storiface.PathStorage when calculating storage requirements ([filecoin-project/lotus#6233](https://github.com/filecoin-project/lotus/pull/6233))
- `storage` module: add go docs and minor code quality refactors ([filecoin-project/lotus#6259](https://github.com/filecoin-project/lotus/pull/6259))
- Increase data transfer timeouts ([filecoin-project/lotus#6300](https://github.com/filecoin-project/lotus/pull/6300))
- gateway: spin off from cmd to package ([filecoin-project/lotus#6294](https://github.com/filecoin-project/lotus/pull/6294))
- Return total power when GetPowerRaw doesn't find miner claim ([filecoin-project/lotus#4938](https://github.com/filecoin-project/lotus/pull/4938))
- add flags to control gateway lookback parameters ([filecoin-project/lotus#6247](https://github.com/filecoin-project/lotus/pull/6247))
- chore(ci): Enable build on RC tags ([filecoin-project/lotus#6238](https://github.com/filecoin-project/lotus/pull/6238))
- cron-wc ([filecoin-project/lotus#6178](https://github.com/filecoin-project/lotus/pull/6178))
- Allow creation of state tree v3s ([filecoin-project/lotus#6167](https://github.com/filecoin-project/lotus/pull/6167))
- mpool: Cleanup pre-nv12 selection logic ([filecoin-project/lotus#6148](https://github.com/filecoin-project/lotus/pull/6148))
- attempt to do better padding on pieces being written into sectors ([filecoin-project/lotus#5988](https://github.com/filecoin-project/lotus/pull/5988))
- remove duplicate ask and calculate ping before lock ([filecoin-project/lotus#5968](https://github.com/filecoin-project/lotus/pull/5968))
- flaky tests improvement: separate TestBatchDealInput from TestAPIDealFlow ([filecoin-project/lotus#6141](https://github.com/filecoin-project/lotus/pull/6141))
- Testground checks on push ([filecoin-project/lotus#5887](https://github.com/filecoin-project/lotus/pull/5887))
- Use EmptyTSK where appropriate ([filecoin-project/lotus#6134](https://github.com/filecoin-project/lotus/pull/6134))
- upgrade `lotus-soup` testplans and reduce deals concurrency to a single miner ([filecoin-project/lotus#6122](https://github.com/filecoin-project/lotus/pull/6122)

## Dependency Updates
- downgrade libp2p/go-libp2p-yamux to v0.5.1. ([filecoin-project/lotus#6605](https://github.com/filecoin-project/lotus/pull/6605))
- Update libp2p to 0.14.2 ([filecoin-project/lotus#6404](https://github.com/filecoin-project/lotus/pull/6404))
- update to markets-v1.4.0 ([filecoin-project/lotus#6369](https://github.com/filecoin-project/lotus/pull/6369))
- Use new actor tags ([filecoin-project/lotus#6291](https://github.com/filecoin-project/lotus/pull/6291))
- chore: update go-libp2p ([filecoin-project/lotus#6231](https://github.com/filecoin-project/lotus/pull/6231))
- Update ffi to proofs v7 ([filecoin-project/lotus#6150](https://github.com/filecoin-project/lotus/pull/6150))

## Others
- Initial draft: basic build instructions on Readme ([filecoin-project/lotus#6498](https://github.com/filecoin-project/lotus/pull/6498))
- Remove rc changelog, compile the new changelog for final release only ([filecoin-project/lotus#6444](https://github.com/filecoin-project/lotus/pull/6444))
- updated configuration comments for docs ([filecoin-project/lotus#6440](https://github.com/filecoin-project/lotus/pull/6440))
- Set ntwk v13 HyperDrive Calibration upgrade epoch ([filecoin-project/lotus#6441](https://github.com/filecoin-project/lotus/pull/6441))
- build snapcraft ([filecoin-project/lotus#6388](https://github.com/filecoin-project/lotus/pull/6388))
- Fix the doc errors of the sealing config funcs ([filecoin-project/lotus#6399](https://github.com/filecoin-project/lotus/pull/6399))
- Add doc on gas balancing ([filecoin-project/lotus#6392](https://github.com/filecoin-project/lotus/pull/6392))
- Add interop network ([filecoin-project/lotus#6387](https://github.com/filecoin-project/lotus/pull/6387))
- Network version 13 (v1.11) ([filecoin-project/lotus#6342](https://github.com/filecoin-project/lotus/pull/6342))
- Add a warning to the release issue template ([filecoin-project/lotus#6374](https://github.com/filecoin-project/lotus/pull/6374))
- Update RELEASE_ISSUE_TEMPLATE.md ([filecoin-project/lotus#6236](https://github.com/filecoin-project/lotus/pull/6236))
- Delete CODEOWNERS ([filecoin-project/lotus#6289](https://github.com/filecoin-project/lotus/pull/6289))
- Feat/nerpa v4 ([filecoin-project/lotus#6248](https://github.com/filecoin-project/lotus/pull/6248))
- Introduce a release issue template ([filecoin-project/lotus#5826](https://github.com/filecoin-project/lotus/pull/5826))
- This is a 1:1 forward-port of PR#6183 from 1.9.x to master ([filecoin-project/lotus#6196](https://github.com/filecoin-project/lotus/pull/6196))
- Update cli gen ([filecoin-project/lotus#6155](https://github.com/filecoin-project/lotus/pull/6155))
- Generate CLI docs ([filecoin-project/lotus#6145](https://github.com/filecoin-project/lotus/pull/6145))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @raulk | 118 | +11972/-10860 | 472 |
| @magik6k | 65 | +10824/-4158 | 353 |
| @aarshkshah1992 | 59 | +8057/-3355 | 224 |
| @arajasek | 41 | +8786/-1691 | 331 |
| @Stebalien | 106 | +7653/-2718 | 273 |
| dirkmc | 11 | +2580/-1371 | 77 |
| @dirkmc | 39 | +1865/-1194 | 79 |
|  | 19 | +1973/-485 | 81 |
| @vyzo | 4 | +1748/-330 | 50 |
| @aarshkshah1992 | 5 | +1462/-213 | 27 |
| @coryschwartz | 35 | +568/-206 | 59 |
| @chadwick2143 | 3 | +739/-1 | 4 |
| @ribasushi | 21 | +487/-164 | 36 |
| @hannahhoward | 5 | +544/-5 | 19 |
| @jennijuju | 9 | +241/-174 | 19 |
| @frrist | 1 | +137/-88 | 7 |
| @travisperson | 3 | +175/-6 | 7 |
| @wadeAlexC | 1 | +48/-129 | 1 |
| @whyrusleeping | 8 | +161/-13 | 11 |
| lotus | 1 | +114/-46 | 1 |
| @nonsense | 8 | +107/-53 | 20 |
| @rjan90 | 4 | +115/-33 | 4 |
| @ZenGround0 | 3 | +114/-1 | 4 |
| @Aloxaf | 1 | +43/-61 | 7 |
| @yaohcn | 4 | +89/-9 | 5 |
| @mitchellsoo | 1 | +51/-0 | 1 |
| @placer14 | 3 | +28/-18 | 4 |
| @jennijuju | 6 | +9/-14 | 6 |
| @Frank | 2 | +11/-10 | 2 |
| @wangchao | 3 | +5/-4 | 4 |
| @Steve Loeppky | 1 | +7/-1 | 1 |
| @Lion | 1 | +4/-2 | 1 |
| @Mimir | 1 | +2/-2 | 1 |
| @raulk | 1 | +1/-1 | 1 |
| @Jack Yao | 1 | +1/-1 | 1 |
| @IPFSUnion | 1 | +1/-1 | 1 |

# 1.10.1 / 2021-07-05

This is an optional but **highly recommended** release of Lotus for lotus miners that has many bug fixes and improvements based on the feedback we got from the community since HyperDrive.

## New Features
- commit batch: AggregateAboveBaseFee config #6650
  - `AggregateAboveBaseFee` is added to miner sealing configuration for setting the network base fee to start aggregating proofs. When the network base fee is lower than this value, the prove commits will be submitted individually via `ProveCommitSector`. According to the [Batch Incentive Alignment](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0013.md#batch-incentive-alignment) introduced in FIP-0013, we recommend miners to set this value to 0.15 nanoFIL(which is the default value) to avoid unexpected aggregation fee in burn and enjoy the most benefits of aggregation!

## Bug Fixes
- storage: Fix FinalizeSector with sectors in storage paths #6652
- Fix tiny error in check-client-datacap #6664
- Fix: precommit_batch method used the wrong cfg.PreCommitBatchWait #6658
- to optimize the batchwait #6636
- fix getTicket: sector precommitted but expired case #6635
- handleSubmitCommitAggregate() exception handling #6595
- remove precommit check in handleCommitFailed #6634
- ensure agg fee is adequate
- fix: miner balance is not enough, so that ProveCommitAggregate msg exec failed #6623
- commit batch: Initialize the FailedSectors map #6647

Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @magik6k| 7 | +151/-56 | 21 |
| @llifezou | 4 | +59/-20 | 4 |
| @johnli-helloworld | 2 | +45/-14 | 4 |
| @wangchao | 1 | +1/-27 | 1 |
| Jerry | 2 | +9/-4 | 2 |
| @zhoutian527 | 1 | +2/-2 | 1 |
| @ribasushi| 1 | +1/-1 | 1 |

# 1.10.1 / 2021-07-05

This is an optional but **highly recommended** release of Lotus for lotus miners that has many bug fixes and improvements based on the feedback we got from the community since HyperDrive.

## New Features
- commit batch: AggregateAboveBaseFee config #6650
  - `AggregateAboveBaseFee` is added to miner sealing configuration for setting the network base fee to start aggregating proofs. When the network base fee is lower than this value, the prove commits will be submitted individually via `ProveCommitSector`. According to the [Batch Incentive Alignment](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0013.md#batch-incentive-alignment) introduced in FIP-0013, we recommend miners to set this value to 0.15 nanoFIL(which is the default value) to avoid unexpected aggregation fee in burn and enjoy the most benefits of aggregation!

## Bug Fixes
- storage: Fix FinalizeSector with sectors in storage paths #6652
- Fix tiny error in check-client-datacap #6664
- Fix: precommit_batch method used the wrong cfg.PreCommitBatchWait #6658
- to optimize the batchwait #6636
- fix getTicket: sector precommitted but expired case #6635
- handleSubmitCommitAggregate() exception handling #6595
- remove precommit check in handleCommitFailed #6634
- ensure agg fee is adequate
- fix: miner balance is not enough, so that ProveCommitAggregate msg exec failed #6623
- commit batch: Initialize the FailedSectors map #6647

Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| @magik6k| 7 | +151/-56 | 21 |
| @llifezou | 4 | +59/-20 | 4 |
| @johnli-helloworld | 2 | +45/-14 | 4 |
| @wangchao | 1 | +1/-27 | 1 |
| Jerry | 2 | +9/-4 | 2 |
| @zhoutian527 | 1 | +2/-2 | 1 |
| @ribasushi| 1 | +1/-1 | 1 |

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

**Check out the documentation [here]((https://lotus.filecoin.io/storage-providers/advanced-configurations/sealing/#precommitsectorsbatch) for details on the new Lotus miner sealing config options, [here](https://lotus.filecoin.io/storage-providers/setup/configuration/#fees-section) for fee config options, and explanations of the new features.**

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
- We'd expect the [state-tree](https://spec.filecoin.io/#section-systems.filecoin_vm.state_tree) (and therefore [snapshot](https://lotus.filecoin.io/lotus/manage/chain-management/#lightweight-snapshot)) size to grow by 1.16GiB per day.
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
  - [Configuration docs and cli examples](https://lotus.filecoin.io/storage-providers/advanced-configurations/sealing/#precommitsectorsbatch)
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

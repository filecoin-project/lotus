# Lotus changelog v1.20.0 - v1.29.2

* [Node v1.29.2 / 2024-10-03](#node-v1292--2024-10-03)
* [Node v1.29.1 / 2024-09-16](#node-v1291--2024-09-16)
* [1.28.3 / 2024-09-16](#1283--2024-09-16)
* [Node v1.29.2 / 2024-10-03](#node-v1292--2024-10-03-1)
* [Node v1.29.0 / 2024-09-02](#node-v1290--2024-09-02)
* [1.28.2 / 2024-08-15](#1282--2024-08-15)
* [v1.28.1 / 2024-07-24](#v1281--2024-07-24)
* [v1.28.0 / 2024-07-23](#v1280--2024-07-23)
* [v1.27.2 / 2024-07-17](#v1272--2024-07-17)
* [v1.27.1 / 2024-06-24](#v1271--2024-06-24)
* [v1.27.0 / 2024-05-27](#v1270--2024-05-27)
* [v1.26.3 / 2024-04-22](#v1263--2024-04-22)
* [v1.26.2 / 2024-04-08](#v1262--2024-04-08)
* [v1.26.1 / 2024-03-27](#v1261--2024-03-27)
* [v1.26.0 / 2024-03-21](#v1260--2024-03-21)
* [v1.25.2 / 2024-01-11](#v1252--2024-01-11)
* [v1.25.1 / 2023-12-09](#v1251--2023-12-09)
* [v 1.25.0 / 2023-11-22](#v-1250--2023-11-22)
* [1.24.0 / 2023-11-22](#1240--2023-11-22)
* [v1.23.3 / 2023-08-01](#v1233--2023-08-01)
* [v1.23.2 / 2023-06-28](#v1232--2023-06-28)
* [v1.23.1 / 2023-06-20](#v1231--2023-06-20)
* [v1.23.0 / 2023-04-21](#v1230--2023-04-21)
* [v1.22.1 / 2023-04-23](#v1221--2023-04-23)
* [v1.22.0 / 2023-04-21](#v1220--2023-04-21)
* [v1.20.4 / 2023-03-17](#v1204--2023-03-17)
* [v1.20.3 / 2023-03-09](#v1203--2023-03-09)
* [v1.20.2 / 2023-03-09](#v1202--2023-03-09)
* [v1.20.1 / 2023-03-06](#v1201--2023-03-06)
* [1.20.0 / 2023-02-28](#1200--2023-02-28)


# Node v1.29.2 / 2024-10-03

This is the stable release for Lotus node v1.29.2. Key updates in this release include:

- **New API Support:** Added support for `EthGetBlockReceipts` RPC method to retrieve transaction receipts for a specified block. This method allows users to obtain Ethereum format receipts of all transactions included in a given tipset as specified by its Ethereum block equivalent. ([filecoin-project/lotus#12478](https://github.com/filecoin-project/lotus/pull/12478))
- **Dependency Update:** Upgraded go-libp2p to version v0.35.5 ([filecoin-project/lotus#12511](https://github.com/filecoin-project/lotus/pull/12511)), and go-multiaddr-dns to v0.4.0 ([filecoin-project/lotus#12540](https://github.com/filecoin-project/lotus/pull/12540)).
- **Bug Fix:** Legacy/historical Drand lookups via `StateGetBeaconEntry` now work again for all historical epochs. `StateGetBeaconEntry` now uses the on-chain beacon entries and follows the same rules for historical Drand round matching as `StateGetRandomnessFromBeacon` and the `get_beacon_randomness` FVM syscall. Be aware that there will be some variance in matching Filecoin epochs to Drand rounds where null Filecoin rounds are involved prior to network version 14. ([filecoin-project/lotus#12428](https://github.com/filecoin-project/lotus/pull/12428)).

## ☢️ Upgrade Warnings ☢️

- Minimum go-version been updated to v1.22.7 ([filecoin-project/lotus#12459](https://github.com/filecoin-project/lotus/pull/12459))

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| aarshkshah1992 | 2 | +1753/-662 | 12 |
| Viraj Bhartiya | 1 | +770/-38 | 18 |
| Rod Vagg | 1 | +480/-83 | 14 |
| Phi-rjan | 2 | +20/-13 | 9 |
| Phi | 2 | +6/-25 | 7 |

# Node v1.29.1 / 2024-09-16

This is a Lotus Node patch release that addresses a critical sync issue affecting users of the v1.29.0 release. The primary fix in this release is:

- Downgrade of a dependency that was causing invalid BLS signatures, leading to sync failures for many Lotus nodes. See [#12467](https://github.com/filecoin-project/lotus/issues/12467) for more information about the bug.

We strongly recommend that all users currently running Lotus v1.29.0 upgrade to this version to resolve the syncing problems.

## Bug Fixes
- Update BLS dependency to fix [filecoin-project/lotus#12467](https://github.com/filecoin-project/lotus/pull/12467).

# 1.28.3 / 2024-09-16

This is a Lotus Node patch release that addresses a critical sync issue affecting users of the v1.28.2 release. The primary fix in this patch release is downgrading of a dependency that was causing invalid BLS signatures, leading to sync failures for many Lotus nodes. See #12467 for more information about the issue/bug.

## Bug Fixes
- Update BLS dependency to fix [filecoin-project/lotus#12467](https://github.com/filecoin-project/lotus/pull/12467).

# Node v1.29.2 / 2024-10-03

This is the stable release for Lotus node v1.29.2. Key updates in this release include:

- **New API Support:** Added support for `EthGetBlockReceipts` RPC method to retrieve transaction receipts for a specified block. This method allows users to obtain Ethereum format receipts of all transactions included in a given tipset as specified by its Ethereum block equivalent. ([filecoin-project/lotus#12478](https://github.com/filecoin-project/lotus/pull/12478))
- **Dependency Update:** Upgraded go-libp2p to version v0.35.5 ([filecoin-project/lotus#12511](https://github.com/filecoin-project/lotus/pull/12511)), and go-multiaddr-dns to v0.4.0 ([filecoin-project/lotus#12540](https://github.com/filecoin-project/lotus/pull/12540)).
- **Bug Fix:** Legacy/historical Drand lookups via `StateGetBeaconEntry` now work again for all historical epochs. `StateGetBeaconEntry` now uses the on-chain beacon entries and follows the same rules for historical Drand round matching as `StateGetRandomnessFromBeacon` and the `get_beacon_randomness` FVM syscall. Be aware that there will be some variance in matching Filecoin epochs to Drand rounds where null Filecoin rounds are involved prior to network version 14. ([filecoin-project/lotus#12428](https://github.com/filecoin-project/lotus/pull/12428)).

## ☢️ Upgrade Warnings ☢️
- This release requires a minimum Go version of v1.22.7 or higher ([filecoin-project/lotus#12459](https://github.com/filecoin-project/lotus/pull/12459))

## 📝 Changelog

See https://github.com/filecoin-project/lotus/compare/v1.29.1...release/v1.29.2 for the set of changes since the last release.

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| aarshkshah1992 | 2 | +1753/-662 | 12 |
| Viraj Bhartiya | 1 | +770/-38 | 18 |
| Rod Vagg | 1 | +480/-83 | 14 |
| Phi-rjan | 2 | +20/-13 | 9 |
| Phi | 2 | +6/-25 | 7 |

# Node v1.29.0 / 2024-09-02

This is a Lotus Node only release, which includes a variety of new features, improvements, and fixes, particularly focused on enhancing ETH RPC functionality. Key highlights of this release include:

- **New Features:**
  - **Trace Filter API:** Added support for the [`trace_filter`](https://openethereum.github.io/JSONRPC-trace-module#trace_filter) RPC method, allowing users to configure `EthTraceFilterMaxResults` to limit the number of results returned in any individual `trace_filter` RPC API call. ([filecoin-project/lotus#12123](https://github.com/filecoin-project/lotus/pull/12123))
  - **Filecoin to ETH Address Conversion:** The `FilecoinAddressToEthAddress` RPC can now return ETH addresses for all Filecoin address types ("f0"/"f1"/"f2"/"f3") based on the client's re-org tolerance. Note that this is a breaking change if you are using the API via the go-jsonrpc library or by using Lotus as a library, but it is non-breaking when using the API via any other RPC method as it adds an optional second argument. ([filecoin-project/lotus#12324](https://github.com/filecoin-project/lotus/pull/12324))
  - **Sending to ETH addresses** The `lotus send` command now supports sending to ETH address recipients. ([filecoin-project/lotus#12319](https://github.com/filecoin-project/lotus/pull/12319))

- **Improvements:**
  - **Gateway Enhancements:** Significant improvements to the `lotus-gateway` including better rate limiting and stateful handling. The `--per-conn-rate-limit` now works as advertised, and a new `--eth-max-filters-per-conn` option allows setting the maximum number of filters and subscriptions per connection, defaulting to 16. Stateful Ethereum APIs are now disabled for plain HTTP connections and require websockets. Additionally, the default value for the `Events.FilterTTL` config option has been reduced from 24h to 1h. ([filecoin-project/lotus#12315](https://github.com/filecoin-project/lotus/pull/12315))
  - **Performance Improvements:** Addressed SQLite index selection performance regressions, significantly improving query times for event-related data. ([filecoin-project/lotus#12261](https://github.com/filecoin-project/lotus/pull/12261)).

## ☢️ Upgrade Warnings ☢️

- `lotus-gateway` behaviour, CLI-arguments and APIs have received minor changes. See the improvements section below for more information.
- The `FilecoinAddressToEthAddress` RPC introduces a breaking change for users of the go-jsonrpc library or Lotus as a library.
- We are aware that legacy/historical Drand lookups via `StateGetBeaconEntry` are currently broken. If you rely on `StateGetBeaconEntry` for looking up historic beacons, we recommend waiting for the Lotus v1.29.1 release. You can follow the progress on this issue in [#12414](https://github.com/filecoin-project/lotus/issues/12414).

## 📝 Changelog

See https://github.com/filecoin-project/lotus/compare/v1.28.2...release/v1.29.0 for the set of changes since the last release.  

<details><summary>Organized Changelog</summary>

### New features

- feat: Add trace filter API supporting RPC method `trace_filter` ([filecoin-project/lotus#12123](https://github.com/filecoin-project/lotus/pull/12123)). Configuring `EthTraceFilterMaxResults` sets a limit on how many results are returned in any individual `trace_filter` RPC API call.
- feat: `FilecoinAddressToEthAddress` RPC can now return ETH addresses for all Filecoin address types ("f0"/"f1"/"f2"/"f3") based on client's re-org tolerance. This is a breaking change if you are using the API via the go-jsonrpc library or by using Lotus as a library, but is a non-breaking change when using the API via any other RPC method as it adds an optional second argument.
([filecoin-project/lotus#12324](https://github.com/filecoin-project/lotus/pull/12324)).
- feat: Added `lotus-shed indexes inspect-events` health-check command ([filecoin-project/lotus#12346](https://github.com/filecoin-project/lotus/pull/12346)).
- feat(libp2p): expose libp2p bandwidth metrics (#12402) ([filecoin-project/lotus#12402](https://github.com/filecoin-project/lotus/pull/12402))
- feat: Lotus Send CLI: Lotus send should work with ETH address receipients (#12319)

### Improvements

- feat!: gateway: fix rate limiting, better stateful handling ([filecoin-project/lotus#12327](https://github.com/filecoin-project/lotus/pull/12327)).
  - CLI usage documentation has been improved for `lotus-gateway`
  - `--per-conn-rate-limit` now works as advertised.
  - `--eth-max-filters-per-conn` is new and allows you to set the maximum number of filters and subscription per connection, it defaults to 16.
    - Previously, this limit was set to `16` and applied separately to filters and subscriptions. This limit is now unified and applies to both filters and subscriptions.
  - Stateful Ethereum APIs (those involving filters and subscriptions) are now disabled for plain HTTP connections. A client must be using websockets to access these APIs.
    - These APIs are also now automatically removed from the node by the gateway when a client disconnects.
  - Some APIs have changed which may impact users consuming Lotus Gateway code as a library.
  - The default value for the `Events.FilterTTL` config option has been reduced from 24h to 1h. This means that filters will expire on a Lotus node after 1 hour of not being accessed by the client.
- feat: f3: override F3BootstrapEpoch on 2k devnet with environment variable (#12354) ([filecoin-project/lotus#12354](https://github.com/filecoin-project/lotus/pull/12354))
- feat: p2p: allow overriding bootstrap nodes with environmemnt variable (#12292) ([filecoin-project/lotus#12292](https://github.com/filecoin-project/lotus/pull/12292))
- feat(f3): F3 has been updated with many performance improvements and additional metrics.
- fix: Eth Event Receipt Logs: use event index for logs (#12269) ([filecoin-project/lotus#12269](https://github.com/filecoin-project/lotus/pull/12269))
- fix: add datacap balance to circ supply internal accounting as unCirc (#12348) ([filecoin-project/lotus#12348](https://github.com/filecoin-project/lotus/pull/12348))
- feat: Use a block cache to speed up the `EthGetBlockByHash` API (#12359) ([filecoin-project/lotus#12359](https://github.com/filecoin-project/lotus/pull/12359))
- fix: lotus-shed: store processed tipset after backfilling events (#12335) ([filecoin-project/lotus#12335](https://github.com/filecoin-project/lotus/pull/12335))
- fix: Eth Tx Events Bloom Filter: fix slice modification bug and flaky test (#12203) ([filecoin-project/lotus#12203](https://github.com/filecoin-project/lotus/pull/12203))
- fix(ETH RPC): receipts: use correct txtype in receipts (#12332) ([filecoin-project/lotus#12332](https://github.com/filecoin-project/lotus/pull/12332)
- fix(cli): only change method for 0x recipients (#12328) ([filecoin-project/lotus#12328](https://github.com/filecoin-project/lotus/pull/12328))
- feat: api: clean API for Miners (#12112) ([filecoin-project/lotus#12112](https://github.com/filecoin-project/lotus/pull/12112))
- feat: p2p: environment variables for disabling DHT query filter and routing table filter (#12289) ([filecoin-project/lotus#12289](https://github.com/filecoin-project/lotus/pull/12289))
- fix: cli: Add delegated to cli docs (#12229) ([filecoin-project/lotus#12229](https://github.com/filecoin-project/lotus/pull/12229))
- feat: sqlite: extract common init and migration utilities (#12098) ([filecoin-project/lotus#12098](https://github.com/filecoin-project/lotus/pull/12098))
- feat: niporep: multi-sector onboarding through UnmanagedMiner (#12180) ([filecoin-project/lotus#12180](https://github.com/filecoin-project/lotus/pull/12180))

### Dependencies
From https://github.com/filecoin-project/lotus/compare/v1.28.2...release/v1.29.0#diff-33ef32bf6c23acb95f5902d7097b7a1d5128ca061167ec0716715b0b9eeaa5f6
- github.com/filecoin-project/go-commp-utils (v0.1.3 -> v0.1.4):
- github.com/filecoin-project/go-commp-utils/nonffi (v0.0.0-20220905160352-62059082a837 -> v0.0.0-20240802040721-2a04ffc8ffe8):
- github.com/filecoin-project/go-hamt-ipld/v3 (v3.1.0 -> v3.4.0):
- github.com/filecoin-project/go-jsonrpc (v0.3.2 -> v0.6.0):
- github.com/filecoin-project/jackc/pgx (v5.4.1 -> v5.6.0)
- feat(f3): update from v0.0.7 to v0.1.0 (#12382) ([filecoin-project/lotus#12382](https://github.com/filecoin-project/lotus/pull/12382))
- feat: f3: update go-f3 to 0.2.0 (#12390) ([filecoin-project/lotus#12390](https://github.com/filecoin-project/lotus/pull/12390))
- feat: update cheggaaa's pb to v3 ([filecoin-project/lotus#12518](https://github.com/filecoin-project/lotus/pull/12518))

### Chores

- feat(eth): fix EthGetTransactionCount for pending block parameter ([filecoin-project/lotus#12520](https://github.com/filecoin-project/lotus/pull/11581))
- chore: ffi: copy verifier iface, mock & ffi out of storage (#11581) ([filecoin-project/lotus#11581](https://github.com/filecoin-project/lotus/pull/11581))
- docs: update LOTUS_RELEASE_FLOW.MD document (#12322) ([filecoin-project/lotus#12322](https://github.com/filecoin-project/lotus/pull/12322))
- docs: update references to releases branch (#12396) ([filecoin-project/lotus#12396](https://github.com/filecoin-project/lotus/pull/12396))
- docs: pr title check fix to link to right doc (#12371) ([filecoin-project/lotus#12371](https://github.com/filecoin-project/lotus/pull/12371))
- docs: updating CONTRIBUTING.md with table of contents and TOC guidance (#12372) ([filecoin-project/lotus#12372](https://github.com/filecoin-project/lotus/pull/12372))
- docs: release template: update based on 1.28 learnings (#12356) ([filecoin-project/lotus#12356](https://github.com/filecoin-project/lotus/pull/12356))
- refactor: adjust PR Title Check workflow to reduce noise (#12373) ([filecoin-project/lotus#12373](https://github.com/filecoin-project/lotus/pull/12373))
- feat(ci): add PR title check workflow (#12340) ([filecoin-project/lotus#12340](https://github.com/filecoin-project/lotus/pull/12340))
- chore(docs): fix some function names (#12368) ([filecoin-project/lotus#12368](https://github.com/filecoin-project/lotus/pull/12368))
- feat: ci: automate the new release process (#12096) ([filecoin-project/lotus#12096](https://github.com/filecoin-project/lotus/pull/12096))
- feat: ci: upload junit xml reports to buildpulse (#12225) ([filecoin-project/lotus#12225](https://github.com/filecoin-project/lotus/pull/12225))
- github: improve stalebot behavior/language (#12370) ([filecoin-project/lotus#12370](https://github.com/filecoin-project/lotus/pull/12370))
- chore: docs: fix some misspellings (#12333) ([filecoin-project/lotus#12333](https://github.com/filecoin-project/lotus/pull/12333))
- docs: expand `CONTRIBUTING.md` and update `README.md` (#12366) ([filecoin-project/lotus#12366](https://github.com/filecoin-project/lotus/pull/12366))
- chore: docs: Update step in skeleton guide (#12349) ([filecoin-project/lotus#12349](https://github.com/filecoin-project/lotus/pull/12349))
- chore: docs: Add initial `Update_Dependencies_Lotus.md` (#12107) ([filecoin-project/lotus#12107](https://github.com/filecoin-project/lotus/pull/12107))
- chore: proxy single Put/Delete to the Many variants (#12313) ([filecoin-project/lotus#12313](https://github.com/filecoin-project/lotus/pull/12313))
- chore: logging: switch stdout print to use the logger (#12311) ([filecoin-project/lotus#12311](https://github.com/filecoin-project/lotus/pull/12311))
- fix(test): flaky eth_legacy_tx_test: wait for async msg indexing (#12200) ([filecoin-project/lotus#12200](https://github.com/filecoin-project/lotus/pull/12200))
- chore: ci: prefix ref passed to build pulse with refs/heads/ (Piotr Galar) (#12338) ([filecoin-project/lotus#12338](https://github.com/filecoin-project/lotus/pull/12338))
- chore: types: remove all references to FFI/rust implementations of CommP/sha2-256-254 (#12345) ([filecoin-project/lotus#12345](https://github.com/filecoin-project/lotus/pull/12345))
- ci: provide additional input for the buildpulse-action (#12284) ([filecoin-project/lotus#12284](https://github.com/filecoin-project/lotus/pull/12284))
- fix: ci: fix configuration variables retrieval in buildpulse workflow (#12282) ([filecoin-project/lotus#12282](https://github.com/filecoin-project/lotus/pull/12282))
- fix: eth: make tx hash immediately available in lookup db after submission (#12219) ([filecoin-project/lotus#12219](https://github.com/filecoin-project/lotus/pull/12219))
- chore: blockstore_gc: minor cleanups ([filecoin-project/lotus#12312](https://github.com/filecoin-project/lotus/pull/12312))
- fix: flaky wdPost dispute itest (#12243) ([filecoin-project/lotus#12243](https://github.com/filecoin-project/lotus/pull/12243))
- ci: add changelog check workflow (#12191) ([filecoin-project/lotus#12191](https://github.com/filecoin-project/lotus/pull/12191))
- fix: build break for Docker test environments (#12229) ([filecoin-project/lotus#12229](https://github.com/filecoin-project/lotus/pull/12229))
- misc: add waffle to match w/ forest (#12247) ([filecoin-project/lotus#12247](https://github.com/filecoin-project/lotus/pull/12247))
- docs: Update Building_a_network_skeleton.md ()
- chore: cli: use `embed` pkg to split long template content to file (#12193) ([filecoin-project/lotus#12193](https://github.com/filecoin-project/lotus/pull/12193))
- ci: provide additional input for the buildpulse-action (#12329) ([filecoin-project/lotus#12329](https://github.com/filecoin-project/lotus/pull/12329))
- chore: docs: Update label to skip/changelog in the PR-template (#12201) ([filecoin-project/lotus#12201](https://github.com/filecoin-project/lotus/pull/12201))
chore: all: fix comment (#12256) ([filecoin-project/lotus#12256](https://github.com/filecoin-project/lotus/pull/12256))
- chore: ui: add a terminal check (#12256) ([filecoin-project/lotus#12256](https://github.com/filecoin-project/lotus/pull/12256))
- chore: types: buildconstants split post-cleanup (#12246) ([filecoin-project/lotus#12246](https://github.com/filecoin-project/lotus/pull/12246))
- chore: lint: enable method, function & type godoc linting (#12258) ([filecoin-project/lotus#12258](https://github.com/filecoin-project/lotus/pull/12258))
- chore: types: buildconstants split post-cleanup (Peter Rabbitson) (#12245) ([filecoin-project/lotus#12245](https://github.com/filecoin-project/lotus/pull/12245))
- fix (ci): support checking PRs from forks in PR-title check (#12367) ([filecoin-project/lotus#12367](https://github.com/filecoin-project/lotus/pull/12367))
- chore: post release steps for Lotus Miner & Node v1.28.2 Release  (#12379) ([filecoin-project/lotus#12379](https://github.com/filecoin-project/lotus/pull/12379))
- chore(release): bump MinerBuildVersion in master (#12380) ([filecoin-project/lotus#12380](https://github.com/filecoin-project/lotus/pull/12380))
- fix(ci): don't PR or changelog check for draft PRs (#12405) ([filecoin-project/lotus#12405](https://github.com/filecoin-project/lotus/pull/12405))
- build: release Lotus node v1.29.0-rc1 (#12410) ([filecoin-project/lotus#12410](https://github.com/filecoin-project/lotus/pull/12410))
- fix: docs: address typo (#12321) ([filecoin-project/lotus#12321](https://github.com/filecoin-project/lotus/pull/12321))
- chore: drand: cleanup legacy and unused configs (#12272) ([filecoin-project/lotus#12272](https://github.com/filecoin-project/lotus/pull/12272))
- chore: merge release/v1.28.0 / v1.28.0-rc4 back to master (#12190) ([filecoin-project/lotus#12190](https://github.com/filecoin-project/lotus/pull/12190))
- fix: Eth Tx Events Bloom Filter: fix slice modification bug and flaky test (#12203) ([filecoin-project/lotus#12203](https://github.com/filecoin-project/lotus/pull/12203))
- Merge releases into master (#12302) ([filecoin-project/lotus#12302](https://github.com/filecoin-project/lotus/pull/12302))
- Merge 1.28.2 to master (#12317) ([filecoin-project/lotus#12317](https://github.com/filecoin-project/lotus/pull/12317))
- fix: panic in api-test because of Commit Batche (#12244)  ([filecoin-project/lotus#12244](https://github.com/filecoin-project/lotus/pull/12244))
- fix: events: remove filter if we fail to add it to the FilterStore (#12318) ([filecoin-project/lotus#12318](https://github.com/filecoin-project/lotus/pull/12318))
- fix: events: don't initialise events helpers when not needed (#12314) ([filecoin-project/lotus#12314](https://github.com/filecoin-project/lotus/pull/12314))
- fix: ethhashlookup: clean-up query management and lifecycle (#12235) ([filecoin-project/lotus#12235](https://github.com/filecoin-project/lotus/pull/12235))
- fix: test: use parents instead of height-1 to calculate sector activation epoch (#12220) ([filecoin-project/lotus#12235](https://github.com/filecoin-project/lotus/pull/12220))
</details>

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Rod Vagg | 45 | +4839/-2634 | 217 |
| Peter Rabbitson | 18 | +2503/-2195 | 209 |
| Jakub Sztandera | 5 | +2695/-1074 | 61 |
| Mikers | 1 | +1274/-455 | 23 |
| Masih H. Derkani | 7 | +873/-682 | 37 |
| Andrew Jackson (Ajax) | 3 | +732/-504 | 75 |
| LexLuthr | 3 | +167/-996 | 8 |
| Piotr Galar | 6 | +622/-372 | 15 |
| Aarsh Shah | 11 | +791/-177 | 44 |
| Phi-rjan | 15 | +476/-178 | 50 |
| web3-bot | 32 | +330/-319 | 39 |
| Steven Allen | 8 | +367/-165 | 41 |
| aarshkshah1992 | 17 | +379/-87 | 32 |
| Frrist | 1 | +403/-22 | 5 |
| Łukasz Magiera | 4 | +266/-27 | 13 |
| winniehere | 1 | +146/-144 | 3 |
| Steve Loeppky | 5 | +162/-53 | 7 |
| Aryan Tikarya | 2 | +183/-8 | 7 |
| adlrocha | 2 | +123/-38 | 21 |
| Jiaying Wang | 7 | +52/-68 | 9 |
| Ian Davis | 2 | +60/-38 | 5 |
| Aayush Rajasekaran | 1 | +80/-2 | 2 |
| hanabi1224 | 4 | +46/-4 | 5 |
| Laurent Senta | 1 | +44/-1 | 2 |
| jennijuju | 6 | +21/-20 | 17 |
| Brendan O'Brien | 1 | +25/-10 | 2 |
| Jennifer Wang | 4 | +24/-8 | 6 |
| dependabot[bot] | 4 | +15/-15 | 8 |
| riskrose | 1 | +8/-8 | 7 |
| Phi | 1 | +6/-6 | 6 |
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
| zl | 1 | +1/-1 | 1 |
| qwdsds | 1 | +1/-1 | 1 |
| Elias Rad | 1 | +1/-1 | 1 |

# 1.28.2 / 2024-08-15

This is a Lotus patch release v1.28.2 for Node operators and Storage Providers.

For node operators, this patch release is HIGHLY RECOMMENDED as it fixes an issue where excessive bandwidth usage (issue #12381) was caused by a routing loop in pubsub, where small "manifest" messages were cycling repeatedly around the network due to an ineffective routing loop prevention mechanism. The new f3 release also has a couple performance improvements around CPU usage. (If you are curious about the progress of F3 testing, follow the updates [here](https://github.com/filecoin-project/lotus/discussions/12287#discussioncomment-10343447)).

For storage providers, this patch release fixes pledge issues users have been encountering. This update addresses existing issues, including the too-small pledge in snap and the lack of DDO-awareness in PoRep Commit.

## ☢️ Upgrade Warnings ☢️
- The `releases` branch has been deprecated with the 202408 split of 'Lotus Node' and 'Lotus Miner'. See https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#why-is-the-releases-branch-deprecated-and-what-are-alternatives for more info and alternatives for getting the latest release for both the 'Lotus Node' and 'Lotus Miner' based on the [Branch and Tag Strategy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#branch-and-tag-strategy).
   - To get the latest Lotus Node tag: `git tag -l 'v*' | sort -V -r | head -n 1`  
   - To get the latest Lotus Miner tag: `git tag -l 'miner/v*' | sort -V -r | head -n 1`
- Breaking change in Miner public APIs `storage/pipeline.NewPreCommitBatcher` and `storage/pipeline.New`. They now have an additional error return to deal with errors arising from fetching the sealing config.

- https://github.com/filecoin-project/lotus/pull/12390: Update go-f3 to 0.2.0
- https://github.com/filecoin-project/lotus/pull/12341: fix: miner: Fix DDO pledge math

# v1.28.1 / 2024-07-24

This is the MANDATORY Lotus v1.28.1 release, which will deliver the Filecoin network version 23, codenamed Waffle 🧇. v1.28.1 is also the minimal version that supports nv23.
**This release sets the Mainnet to upgrade at epoch 4154640, corresponding to 2024-08-06T12:00:00Z.**

## ☢️ Upgrade Warnings ☢️
- If you are running the `v1.26.x` version of Lotus, please go through the `Upgrade Warnings` section for the `v1.27.*` releases, before upgrading to this RC.
- Note that v1.28.0 needed a bug fix and a feature enhancement to ensure a smooth support for nv23 and it was retracted. Please update your node to v1.28.1 or above before the nv23 upgrade!
- This upgrade includes an additional migration to the events database. Node operators running Lotus with events turned on (off by default) may experience some delay in initial start-up of Lotus as a minor database migration takes place. See [filecoin-project/lotus#12080](https://github.com/filecoin-project/lotus/pull/12080) for full details.
- All **Storage Providers MUST finish onboarding all sectors that have deal IDs in the `PreCommitSectors` `OnChainSectorInfo`s** before upgrading the lotus miner OR THEY WILL BE WASTED. Please see more details in the next section.

## The Filecoin network version 23 delivers the following FIPs:

- [FIP-0065: Ignore built-in market locked balance in circulating supply calculation](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0065.md)
- [FIP-0079: Add BLS Aggregate Signatures to FVM](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0079.md)
- [FIP-0084: Remove Storage Miner Actor Method ProveCommitSectors](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0084.md)
  - :warning: Please note that onboarding via `ProveCommitSectors` is deprecated in favor of `ProveCommitSectors3`, which was introduced in FIP-0076 and activated in the last network upgrade (NV22). `ProveCommitSectors3` will reject the activation of sectors that were precommitted with deal IDs.
    Storage Providers should ensure that their pipeline is updated to adopt the `ProveCommitSector3` flow as soon as possible. Otherwise, they risk losing deal collateral, PCD, and sealing work for sectors that were not fully committed on-chain before the upgrade yet have deal IDs in the precommitted sector on-chain info. This release removes the deprecated `ProveCommitSectors` pipeline, and the new pipeline is fully supported. Prior to this release, the onboarding pipeline would still prefer to use `ProveCommitSectors` for sectors containing deals. If you have any questions, please don't hesitate to reach out in #fil-curio-dev.
- [FIP-0085: Convert f090 Mining Reserve Actor to Keyless Account Actor](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0085.md)
- [FIP-0091: Add support for legacy Ethereum transactions](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0091.md)
- [FIP-0092: NI-PoRep](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0092.md)
- [FIP-0086: Fast Finality Soft Launch](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0086.md)

Note that we are only doing a "soft launch"/"passive testing" for F3 (Fast Finality) i.e. FIP-0086 in NV23. Please see [this doc](https://docs.google.com/document/d/14hMFN95_AsByBh7iMc4r_czUgg8tfjHQ1gTsmmHZ8jI/edit#heading=h.dhzqs3lisv24) for more details.

## v14 Builtin Actor Bundle

[Builtin actor v14.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v14.0.0) is used for supporting this upgrade. Make sure that your lotus actor bundle matches the v14 actors manifest by running the following cli after upgrading:

```
lotus state actor-cids --network-version=23
Network Version: 23
Actor Version: 14
Manifest CID: bafy2bzacecbueuzsropvqawsri27owo7isa5gp2qtluhrfsto2qg7wpgxnkba

Actor             CID  
account           bafk2bzacebr7ik7lng7vysm754mu5x7sakphwm4soqi6zwbox4ukpd6ndwvqy
cron              bafk2bzacecwn6eiwa7ysimmk6i57i5whj4cqzwijx3xdlxwb5canmweaez6xc
datacap           bafk2bzacecidw7ajvtjhmygqs2yxhmuybyvtwp25dxpblvdxxo7u4gqfzirjg
eam               bafk2bzaced2cxnfwngpcubg63h7zk4y5hjwwuhfjxrh43xozax2u6u2woweju
ethaccount        bafk2bzacechu4u7asol5mpcsr6fo6jeaeltvayj5bllupyiux7tcynsxby7ko
evm               bafk2bzacedupohbgwrcw5ztbbsvrpqyybnokr4ylegmk7hrbt3ueeykua6zxw
init              bafk2bzacecbbcshenkb6z2v4irsudv7tyklfgphhizhghix6ke5gpl4r5f2b6
multisig          bafk2bzaceajcmsngu3f2chk2y7nanlen5xlftzatytzm6hxwiiw5i5nz36bfc
paymentchannel    bafk2bzaceavslp27u3f4zwjq45rlg6assj6cqod7r5f6wfwkptlpi6j4qkmne
placeholder       bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro
reward            bafk2bzacedvfnjittwrkhoar6n5xrykowg2e6rpur4poh2m572f7m7evyx4lc
storagemarket     bafk2bzaceaju5wobednmornvdqcyi6khkvdttkru4dqduqicrdmohlwfddwhg
storageminer      bafk2bzacea3f43rxzemmakjpktq2ukayngean3oo2de5cdxlg2wsyn53wmepc
storagepower      bafk2bzacedo6scxizooytn53wjwg2ooiawnj4fsoylcadnp7mhgzluuckjl42
system            bafk2bzacecak4ow7tmauku42s3u2yydonk4hx6ov6ov542hy7lcbji3nhrrhs
verifiedregistry  bafk2bzacebvyzjzmvmjvpypphqsumpy6rzxuugnehgum7grc6sv3yqxzrshb4
```

## Migration

All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 120 epochs before the network upgrade. The migration for the NV23 upgrade is expected to be light with no heavy pre-migrations, here are some expected timings and resource consumption numbers:

- Pre-migration is expected to take less than 1 minute.
- The migration is expected to take less than 30 seconds on a node with an NVMe drive and a newer CPU. For nodes running on slower disks/CPU, it is still expected to take less than 1 minute.
- Max memory usage during benchmarking the migration in "offline mode" (i.e., node not syncing) was 23GiB.
- Max memory usage when benchmarking the migration in "online mode" (i.e., while the node is syncing) was 30GiB. Numbers here might vary depending on the load your node is under.
  More details on the migration benchmarking can be found in https://github.com/filecoin-project/lotus/issues/12128

We recommend node operators (who haven't enabled splitstore discard mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

For certain node operators, such as full archival nodes or systems that need to keep large amounts of state (RPC providers), we recommend skipping the pre-migration and running the non-cached migration (i.e., just running the migration at the network upgrade epoch), and scheduling some additional downtime. Operators of such nodes can read the [How to disable premigration in network upgrade tutorial.](https://lotus.filecoin.io/kb/disable-premigration/)

## Fast Finality for Filecoin (f3) soft launch

We are one step closer to reduce Filecoin's finality from 7.5 hours to a minute or so, you can checkout the [FIP](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0086.md) for more details.  Changing the consensus protocol is not trivial, and the f3 implementation team has designed a [passive testing plan to verify the protocol](https://github.com/filecoin-project/go-f3/issues/213) and give time for client implementation teams to integrate and test F3 before it is fully activated in the network consensus. That said, the lotus team has implemented f3 & the manifest for passive testing in this release, and we would like to ask node operators, especially storage providers, to participate in the testing by participating in F3 on the mainnet (which is enabled by default in this release)! We will keep updating [this discussion](https://github.com/filecoin-project/lotus/discussions/12287) to capture "what can you expect" & testing status. If you notice any unexpected behaviour caused by f3, please do not hesitate to reach out to us in #fil-fast-finality.

F3 (Fast Finality) is experimental in this release. All the new F3 APIs  are unstable and subject to until nv24 release (assuming f3 will be fully activated in this upgrade).

Exchanges and RPC providers are recommended to opt-out of F3 functionality for now. You can disable F3 by adding the `LOTUS_DISABLE_F3 = 1` environment variable, which will output a log saying that F3 has been disabled.

## Dependencies

- github.com/filecoin-project/go-state-types (`v0.14.0-dev` -> `v0.14.0`)
- github.com/filecoin-project/filecoin-ffi (`v1.27.0-rc2` -> `v1.28.0`)
- github.com/filecoin-project/go-libp2p2 (`v0.35.3` -> `v0.35.4`)
- `ref-fvm4` (as part of `filecoin-ffi`) (`4.2.0` -> `4.3.1`)
- A new `github.com/filecoin-project/go-f3` dependency for F3 soft launch (`v0.0.5`)

## Others

- Soft launch of F3 (https://github.com/filecoin-project/lotus/pull/12119)
- NI-PoRep changes (https://github.com/filecoin-project/lotus/pull/12130)
- Fixes for the ETH events API (https://github.com/filecoin-project/lotus/pull/12080)
- Support for legacy Ethereum transactions (https://github.com/filecoin-project/lotus/pull/11969)
- Ignore market balance after nv23 (https://github.com/filecoin-project/lotus/pull/11976)
- Add finality-related params for `eth_getBlockByNumber` (https://github.com/filecoin-project/lotus/pull/12110)
- rename `Actor.Address` to `Actor.DelegatedAddress` and only use it for f4 addresses (https://github.com/filecoin-project/lotus/pull/12155)
- feat:ec: integrate F3 dynamic manifest #12185
- fix: f3: Fix F3 build parameters for testground target (#12189) ([filecoin-project/lotus#12189](https://github.com/filecoin-project/lotus/pull/12189))
- fix: eth_getLogs: https://github.com/filecoin-project/lotus/pull/12212
- chore: lotus-shed: Add support for nv23 in migrate-state cmd #12172
- feat: F3: Update go-f3, change the style of participation call #12196
- chore: f3: Upgrade go mod F3 dependency to v0.0.3 tagged release #12216
- fix: Eth Trace Block: nil access panic #12221
- chore!: markets: remove stray unixfs constants, features and references #12217
- chore: metrics: Upgrade to OpenTelemetry v1.28.0 #12223
- fix: bug: Reduce log level in F3 message sending to Debug #12224
- [skip changelog] chore: config: yet more lp2p removal from miner #12252
- fix(store): correctly break weight ties based on smaller ticket #12253
- fix: exchange bug #12275
- chore: deps: Update GST, Filecoin-FFI and Actors to final versions NV23 #12276
- metrics: f3: Set up otel metrics reporting to prometheus #12285
- dep: f3: Update go-f3 to 0.0.7, enable it on mainnet
- fix: lotus-miner: remove provecommit1 method #12251

# v1.28.0 / 2024-07-23

Update on 2027-07-24
This release is retracted, please refer to v1.28.1 for more details

# v1.27.2 / 2024-07-17

This is the stable release of Lotus v1.27.2. This will be an OPTIONAL Lotus release. It contains some improvements that are relevant for node operators that are using or serving `eth_*` RPC methods. It also contains an upgraded libp2p to v0.35.3 which is included in this release for additional testing of some fixes that may solve some connectivity problems experienced by some users (See [libp2p/go-libp2p#2858](https://github.com/libp2p/go-libp2p/issues/2858) for more information).

## ☢️ Upgrade Warnings ☢️

- This Lotus release includes some correctness improvements to the events subsystem, impacting RPC APIs including `GetActorEventsRaw`, `SubscribeActorEventsRaw`, `eth_getLogs` and the `eth` filter APIs. Part of these improvements involve an events database migration that may take some time to complete on nodes with extensive event databases. See [filecoin-project/lotus#12080](https://github.com/filecoin-project/lotus/pull/12080) for details.

## Improvements

- fix: events index: record processed epochs and tipsets for events and eth_get_log blocks till requested tipset has been indexed (#12080) ([filecoin-project/lotus#12080](https://github.com/filecoin-project/lotus/pull/12080))
- feat: eth: support "safe" and "finalized" for eth_getBlockByNumber (#12110) ([filecoin-project/lotus#12110](https://github.com/filecoin-project/lotus/pull/12110))
- feat: api: sanity check the "to" address of outgoing messages (#12135) ([filecoin-project/lotus#12135](https://github.com/filecoin-project/lotus/pull/12135))
- chore: ci: remove non-existent market tests from CI workflow (#12099) ([filecoin-project/lotus#12099](https://github.com/filecoin-project/lotus/pull/12099))
- fix: bootstrap: remove unmaintained bootstrap node (#12133) ([filecoin-project/lotus#12133](https://github.com/filecoin-project/lotus/pull/12133))
- Update bootstrap list to support both IPv4 and IPv6 (#12103) ([filecoin-project/lotus#12103](https://github.com/filecoin-project/lotus/pull/12103))

## Dependencies

- chore: deps: upgrade to libp2p@v0.35.3 from v0.34.1 ([filecoin-project/lotus#12249](https://github.com/filecoin-project/lotus/pull/12249))

Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Aarsh Shah | 2 | +424/-28 | 4 |
| Steven Allen | 1 | +137/-0 | 3 |
| Mikers | 1 | +63/-0 | 4 |
| Phi-rjan | 1 | +10/-10 | 2 |
| Peter Rabbitson | 1 | +4/-8 | 1 |
| Hubert | 1 | +0/-1 | 1 |

# v1.27.1 / 2024-06-24

This release, v1.27.1, is an OPTIONAL lotus release. It is HIGHLY RECOMMENDED for node operators that are building Filecoin index off lotus!

## ☢️ Upgrade Warnings ☢️

- This Lotus release completely removes the Legacy Lotus/Lotus-Miner Markets sub-system from the codebase, which was announced to reach EOL on January 31, 2023.
- The **Curio Storage** software, designed to simplify the setup and operation of storage providers, has moved to their own Github-repository: https://github.com/filecoin-project/curio.
- The events subsystem includes some minor correctness fixes and performance improvements. Nodes operators running Lotus with events turned on (off by default) may experience some delay in initial start-up as a minor database migration takes place and the write-ahead log is compacted. See [filecoin-project/lotus#11952](https://github.com/filecoin-project/lotus/pull/11952) and [filecoin-project/lotus#12090](https://github.com/filecoin-project/lotus/pull/12090) for full details.

### JSON-RPC 2.0 Specification Conformance

The JSON-RPC 2.0 specification requires that a `"result"` property be present in the case of no error from an API call. This release ensures that all API calls that return a result have a `"result"` property in the response. This is a behaviour change over Lotus v1.26 and will impact any API call that only has a single error return value, where no error has occurred.

For example, a successful `WalletSetDefault` in v1.26 would return:

```json
{
  "jsonrpc": "2.0",
  "id": 1
}
```

As of this change, in conformance with the JSON-RPC 2.0 specification it will return:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": null
}
```

There is no change in the behaviour when a call returns an error, as the error object will still be present in the response.

## New features

- feat: Add trace transaction API supporting RPC method `trace_transaction` ([filecoin-project/lotus#12068](https://github.com/filecoin-project/lotus/pull/12068))
- feat: Skeleton for nv23 (#11964) ([filecoin-project/lotus#11964](https://github.com/filecoin-project/lotus/pull/11964))
- feat: state: Ignore market balance after nv23 (#11976) ([filecoin-project/lotus#11976](https://github.com/filecoin-project/lotus/pull/11976))
- feat: ETH compatibility in Filecoin : Support Homestead and EIP-155 Ethereum transactions("legacy" transactions) in Filecoin after NV23 (#11969) ([filecoin-project/lotus#11969](https://github.com/filecoin-project/lotus/pull/11969))
- fix: hello: avoid dialing when fetching hello tipset (#12032) ([filecoin-project/lotus#12032](https://github.com/filecoin-project/lotus/pull/12032))
- feat: cli,events: speed up backfill with temporary index (#11953) ([filecoin-project/lotus#11953](https://github.com/filecoin-project/lotus/pull/11953))

## Improvements
- Event index should be unique for tipsets (#11952) ([filecoin-project/lotus#11952](https://github.com/filecoin-project/lotus/pull/11952))
- cleanup: Lotus client: Remove markets and deal-making from Lotus Client (#11999) ([filecoin-project/lotus#11999](https://github.com/filecoin-project/lotus/pull/11999))
- fix: ci: use filecoin-ffi hash to cache make deps outputs (#11961) ([filecoin-project/lotus#11961](https://github.com/filecoin-project/lotus/pull/11961))
- add ETH addrs API to Gateway (#11979) ([filecoin-project/lotus#11979](https://github.com/filecoin-project/lotus/pull/11979))
- chore: remove unmaintained bootstrappers (#11983) ([filecoin-project/lotus#11983](https://github.com/filecoin-project/lotus/pull/11983))
- feat: api: add SectorNumber to MarketDealState (nv22)
- fix: copy Flags field from SectorOnChainInfo
- fix: ETH RPC API: ETH Call should use the parent state root of the subsequent tipset ([filecoin-project/lotus#11905](https://github.com/filecoin-project/lotus/pull/11905))
- fix: events: sqlite db improvements ([filecoin-project/lotus#12090](https://github.com/filecoin-project/lotus/pull/12090))

## Dependencies

- chore: libp2p: update to v0.34.1 (#12027) ([filecoin-project/lotus#12027](https://github.com/filecoin-project/lotus/pull/12027))
- chore: update drand (#12021) ([filecoin-project/lotus#12021](https://github.com/filecoin-project/lotus/pull/12021))
- Bump pubsub-dep (#11966) ([filecoin-project/lotus#11966](https://github.com/filecoin-project/lotus/pull/11966))
- fix: update go-jsonrpc to v0.3.2
- Bump go-jsonrpc to v0.4.0 (#12034) ([filecoin-project/lotus#12034](https://github.com/filecoin-project/lotus/pull/12034))
- docs: rpc: document go-jsonrpc behaviour change
- chore: update go-data-transfer and go-graphsync
- github.com/filecoin-project/go-jsonrpc (v0.3.1 -> v0.3.2)
- github.com/filecoin-project/go-state-types (v0.13.3 -> v0.14.0-dev)

## Lotus-Miner / Curio related changes

- fix logs (#12036) ([filecoin-project/lotus#12036](https://github.com/filecoin-project/lotus/pull/12036))
- feat: curioweb: Improve task_history indexes (#11911) ([filecoin-project/lotus#11911](https://github.com/filecoin-project/lotus/pull/11911))
- fix: curio taskstorage: Don't try to free reservations by nulled TaskID (#12018) ([filecoin-project/lotus#12018](https://github.com/filecoin-project/lotus/pull/12018))
- fix actor string (#12019) ([filecoin-project/lotus#12019](https://github.com/filecoin-project/lotus/pull/12019))
- fix: curio: Update pgx imports, fix db_storage alloc
- feat: curioweb: Show piece info on the sector page (#11955) ([filecoin-project/lotus#11955](https://github.com/filecoin-project/lotus/pull/11955))
- curio: feat: break trees task into TreeD(prefetch) and TreeRC (#11895) ([filecoin-project/lotus#11895](https://github.com/filecoin-project/lotus/pull/11895))
- fix: curio: node UI & darwin gpu count (#11950) ([filecoin-project/lotus#11950](https://github.com/filecoin-project/lotus/pull/11950))
- feat: curio: Keep more sector metadata in the DB long-term (#11933) ([filecoin-project/lotus#11933](https://github.com/filecoin-project/lotus/pull/11933))
- fix: curio/lmrpc: Check ParkPiece success before creating sectors (#11975) ([filecoin-project/lotus#11975](https://github.com/filecoin-project/lotus/pull/11975))
- feat: curio: docker devnet (#11954) ([filecoin-project/lotus#11954](https://github.com/filecoin-project/lotus/pull/11954))
- feat: curio: alertManager (#11926) ([filecoin-project/lotus#11926](https://github.com/filecoin-project/lotus/pull/11926))
- curio cfg edit: ux cleanups (#11985) ([filecoin-project/lotus#11985](https://github.com/filecoin-project/lotus/pull/11985))
- fix: curio: Drop FKs from pipeline to fix retry loops (#11973) ([filecoin-project/lotus#11973](https://github.com/filecoin-project/lotus/pull/11973))
- Produce DEB files for amd64 for openCL and cuda (#11885) ([filecoin-project/lotus#11885](https://github.com/filecoin-project/lotus/pull/11885))
- gui-listen fix (#12013) ([filecoin-project/lotus#12013](https://github.com/filecoin-project/lotus/pull/12013))
- feat: curio: allow multiple pieces per sector (#11935) ([filecoin-project/lotus#11935](https://github.com/filecoin-project/lotus/pull/11935))
- chore: update yugabyte deps (#12022) ([filecoin-project/lotus#12022](https://github.com/filecoin-project/lotus/pull/12022))
- fix: harmonydb: Use timestampz instead of timestamp across the schema (#12030) ([filecoin-project/lotus#12030](https://github.com/filecoin-project/lotus/pull/12030))
- cleanup: miner: remove markets and deal-making from Lotus Miner (#12005) ([filecoin-project/lotus#12005](https://github.com/filecoin-project/lotus/pull/12005))
- fix non existing sector (#12012) ([filecoin-project/lotus#12012](https://github.com/filecoin-project/lotus/pull/12012))
- feat: curio ffiselect: Isolate gpu calls in a subprocess (#11994) ([filecoin-project/lotus#11994](https://github.com/filecoin-project/lotus/pull/11994))
- feat: curio: jsonrpc in webui (#11904) ([filecoin-project/lotus#11904](https://github.com/filecoin-project/lotus/pull/11904))
- fix: itests: Fix flaky curio itest (#12037) ([filecoin-project/lotus#12037](https://github.com/filecoin-project/lotus/pull/12037))
- feat: curio: wdPost and wnPost alerts (#12029) ([filecoin-project/lotus#12029](https://github.com/filecoin-project/lotus/pull/12029))
- fix: storage: Fix a race in GenerateWindowPoStAdv (#12064) ([filecoin-project/lotus#12064](https://github.com/filecoin-project/lotus/pull/12064))
- Remove "provider" relics  (#11992) ([filecoin-project/lotus#11992](https://github.com/filecoin-project/lotus/pull/11992))
- fix sector UI (#12016) ([filecoin-project/lotus#12016](https://github.com/filecoin-project/lotus/pull/12016))

## Others
- ci: deprecate circle ci in favour of github actions (#11786) ([filecoin-project/lotus#11786](https://github.com/filecoin-project/lotus/pull/11786))
- src: chain: remove C dependency from builtin types (#12015) ([filecoin-project/lotus#12015](https://github.com/filecoin-project/lotus/pull/12015))
- chore: fix function names (#12043) ([filecoin-project/lotus#12043](https://github.com/filecoin-project/lotus/pull/12043))
- chore: bump build version in master (#11946) ([filecoin-project/lotus#11946](https://github.com/filecoin-project/lotus/pull/11946))
- fix: test: no snap deals in immutable deadlines (#12071) ([filecoin-project/lotus#12071](https://github.com/filecoin-project/lotus/pull/12071))
- test: actors: manual CC onboarding and proving integration test (#12017) ([filecoin-project/lotus#12017](https://github.com/filecoin-project/lotus/pull/12017))
- fix: ci: keep lotus checkout clean in the release workflow (#12028) ([filecoin-project/lotus#12028](https://github.com/filecoin-project/lotus/pull/12028))
- feat!: build: separate miner and node version strings
- chore: lint: address feedback from reviews
- chore: lint: fix lint errors with new linting config
- chore: lint: update golangci lint config
- ci: fix when sorted pr checks workflow is executed
- doc: eth: restore comment lost in linter cleanup
- fix: ci: publish correct docker tags on workflow dispatch (#12060) ([filecoin-project/lotus#12060](https://github.com/filecoin-project/lotus/pull/12060))
- feat: libp2p: Lotus stream cleanup (#11993) ([filecoin-project/lotus#11993](https://github.com/filecoin-project/lotus/pull/11993))
- Update SupportedProofTypes (#11988) ([filecoin-project/lotus#11988](https://github.com/filecoin-project/lotus/pull/11988))
- Revert "Update SupportedProofTypes (#11988)" (#11990) ([filecoin-project/lotus#11990](https://github.com/filecoin-project/lotus/pull/11990))
- chore: docs: Update skeleton guide (#11960) ([filecoin-project/lotus#11960](https://github.com/filecoin-project/lotus/pull/11960))
- chore: ci: request contents read permissions explicitly in gha (#12055) ([filecoin-project/lotus#12055](https://github.com/filecoin-project/lotus/pull/12055))
- fix: ci: use custom GITHUB_TOKEN for GoReleaser (#12059) ([filecoin-project/lotus#12059](https://github.com/filecoin-project/lotus/pull/12059))
- chore: pin golanglint-ci to v1.58.2 (#12054) ([filecoin-project/lotus#12054](https://github.com/filecoin-project/lotus/pull/12054))
- chore: fix some function names (#12031) ([filecoin-project/lotus#12031](https://github.com/filecoin-project/lotus/pull/12031))
- src: lint: bump golangci-lint to 1.59, address unchecked fmt.Fprint*
- fix: ci: do not use deprecated --debug goreleaser flag ([filecoin-project/lotus#12086](https://github.com/filecoin-project/lotus/pull/12086))
- chore: Remove forgotten graphsync references ([filecoin-project/lotus#12084](https://github.com/filecoin-project/lotus/pull/12084))
- chore: types: remove more items forgotten after markets ([filecoin-project/lotus#12095](https://github.com/filecoin-project/lotus/pull/12095))
- chore: api: the Net API/CLI now remains only on daemon ([filecoin-project/lotus#12100](https://github.com/filecoin-project/lotus/pull/12100))
- fix: release: update goreleaser config filei #12120

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Aarsh Shah | 9 | +5710/-35899 | 201 |
| Łukasz Magiera | 21 | +1891/-33776 | 335 |
| LexLuthr | 9 | +4916/-1637 | 107 |
| Phi-rjan | 9 | +3544/-187 | 92 |
| Rod Vagg | 15 | +2183/-479 | 164 |
| Piotr Galar | 6 | +130/-2386 | 30 |
| Andrew Jackson (Ajax) | 6 | +1072/-533 | 63 |
| ZenGround0 | 1 | +235/-13 | 3 |
| Hubert Bugaj | 3 | +57/-37 | 5 |
| Steven Allen | 3 | +25/-15 | 6 |
| Peter Rabbitson | 1 | +16/-8 | 4 |
| tomfees | 1 | +6/-6 | 5 |
| imxyb | 1 | +6/-0 | 1 |
| yumeiyin | 1 | +2/-2 | 2 |
| galargh | 1 | +2/-2 | 1 |

# v1.27.0 / 2024-05-27

This is an optional feature release of Lotus. Lotus v1.27.0 includes numerous improvements, bugfixes and enhancements for node operators, RPC- and ETH RPC-providers. This feature release also introduces Curio in a Beta release. Check out the Curio Beta release section for how you can get started with Curio.

## ☢️ Upgrade Warnings ☢️

- This feature release drops the Raft cluster code experiment from the codebase. This Raft cluster never graduated beyond an experiment, had poor UX (e.g. no way to manage a running cluster, so it didn't provide High Availability), and pulled in a lot of heavy dependencies. We keep the multi-node RPC feature, it is not perfect, but it is useful.
- Event Database: Two sequential migrations will adjust indexes without altering data or columns, ensuring minimal invasiveness when upgrading to this release. However, these migrations may be time-consuming for nodes with extensive event databases.

## Indexers, RPC- and ETH RPC-providers improvements

This release includes a lot of improvements and fixes for indexers, RPC- and ETH RPC-providers. Specifically these PRs:

- [Significant performance improvements of eth_getLog](https://github.com/filecoin-project/lotus/pull/11477)
- [Return the correct block gas limit in the EthAP](https://github.com/filecoin-project/lotus/pull/11747)
- [Accept input data in call arguments under field 'input'](https://github.com/filecoin-project/lotus/pull/11505)
- [Length check the array sent to eth_feeHistory RPC](https://github.com/filecoin-project/lotus/pull/11696)
- [ETH subscribe tipsets API should only return tipsets that have been executed](https://github.com/filecoin-project/lotus/pull/11858)
- [Adjust indexes in event index db to match query patterns](https://github.com/filecoin-project/lotus/pull/111934)

## ⭐️ Curio Beta Release  ⭐️

**Curio**, the next generation of Lotus-Miner, also referred to as MinerV2! This release officially transitions Curio into beta and introduces a suite of powerful features designed to enhance your storage operations.

### Highlights

- **Curio as MinerV2**: Embrace the revolutionary upgrade from Lotus-Miner to Curio. This transition is not just a rebranding—it's an upgrade to a more robust, scalable, and user-friendly version.
- **High Availability**: Curio is designed for high availability. You can run multiple instances of Curio nodes to handle similar type of tasks. The distributed scheduler and greedy worker design will ensure that tasks are completed on time despite most partial outages. You can safely update one of your Curio machines without disrupting the operation of the others.
- **Node Heartbeat**: Each Curio node in a cluster must post a heartbeat message every 10 minutes in HarmonyDB updating its status. If a heartbeat is missed, the node is considered lost and all tasks can now be scheduled on remaining nodes.
- **Task Retry**: Each task in Curio has a limit on how many times it should be tried before being declared lost. This ensures that Curio does not keep retrying bad tasks indefinitely. This safeguards against lost computation time and storage.
- **Polling**: Curio avoids overloading nodes with a polling system. Nodes check for tasks they can handle, prioritizing idle nodes for even workload distribution.
- **Simple Configuration Management**: The configuration is stored in the database in the forms of layers. These layers can be stacked on top of each other to create a final configuration. Users can reuse these layers to control the behavior of multiple machines without needing to maintain the configuration of each node. Start the binary with the appropriate flags to connect with YugabyteDB and specify which configuration layers to use to get desired behaviour.

### Getting Started with Curio

```bash
cd lotus
git pull
make clean deps all
sudo make install
```

On your local-dev-net or calibrationnet lotus-miner machine, initiate:

`curio guided-setup`

### Need More Info?

For detailed documentation and additional information on Curio:

Curio Overview <- insert link
Visit the Curio Official Website insert link

❗Curio is in Beta state, and we recommend our users to run Curio in a testing environment or on the Calibration network for the time being.

## New features

- feat: exchange: change GetBlocks to always fetch the requested number of tipsets ([filecoin-project/lotus#11565](https://github.com/filecoin-project/lotus/pull/11565))
- feat: syncer: optimize syncFork for one-epoch forks ([filecoin-project/lotus#11533](https://github.com/filecoin-project/lotus/pull/11533))
- feat: api: improve the correctness of Eth's trace_block (#11609) ([filecoin-project/lotus#11609](https://github.com/filecoin-project/lotus/pull/11609))
- perf: api: add indexes to event topics and emitter addr (#11477) ([filecoin-project/lotus#11477](https://github.com/filecoin-project/lotus/pull/11477))
- feat: drand: refactor round verification ([filecoin-project/lotus#11598](https://github.com/filecoin-project/lotus/pull/11598))
- feat: add Forest bootstrap nodes (#11636) ([filecoin-project/lotus#11636](https://github.com/filecoin-project/lotus/pull/11636))
- feat: curio: add miner init (#11775) ([filecoin-project/lotus#11775](https://github.com/filecoin-project/lotus/pull/11775))
- feat: curio: sectors UI (#11869) ([filecoin-project/lotus#11869](https://github.com/filecoin-project/lotus/pull/11869))
- feat: curio: storage index gc task (#11884) ([filecoin-project/lotus#11884](https://github.com/filecoin-project/lotus/pull/11884))
- feat: curio: web based config edit (#11822) ([filecoin-project/lotus#11822](https://github.com/filecoin-project/lotus/pull/11822))
- feat: spcli: sectors extend improvements (#11798) ([filecoin-project/lotus#11798](https://github.com/filecoin-project/lotus/pull/11798))
- feat: curio: Add schemas for DDO deal support (#11805) ([filecoin-project/lotus#11805](https://github.com/filecoin-project/lotus/pull/11805))
- feat: curioweb: add favicon (#11804) ([filecoin-project/lotus#11804](https://github.com/filecoin-project/lotus/pull/11804))
- feat: lotus-provider: Fetch params on startup when needed ([filecoin-project/lotus#11650](https://github.com/filecoin-project/lotus/pull/11650))
- feat: mpool: Cache actors in lite mode (#11668) ([filecoin-project/lotus#11668](https://github.com/filecoin-project/lotus/pull/11668))
- feat: curio: simpler reservation release logic (#11900) ([filecoin-project/lotus#11900](https://github.com/filecoin-project/lotus/pull/11900))
- feat: curio: add StorageInit api (#11918) ([filecoin-project/lotus#11918](https://github.com/filecoin-project/lotus/pull/11918))
- feat: lotus-provider: SDR Sealing pipeline ([filecoin-project/lotus#11534](https://github.com/filecoin-project/lotus/pull/11534))
- feat: curioweb: Sector info page (#11846) ([filecoin-project/lotus#11846](https://github.com/filecoin-project/lotus/pull/11846))
- feat: curio web: node info page (#11745) ([filecoin-project/lotus#11745](https://github.com/filecoin-project/lotus/pull/11745))
- feat: fvm: optimize FVM lanes a bit (#11875) ([filecoin-project/lotus#11875](https://github.com/filecoin-project/lotus/pull/11875))
- feat: Gateway API: Add ETH -> FIL and FIL -> ETH address conversion APIs to the Gateway (#11979) ([filecoin-project/lotus#11979](https://github.com/filecoin-project/lotus/pull/11979))

## Improvements

- fix: api: return the correct block gas limit in the EthAPI (#11747) ([filecoin-project/lotus#11747](https://github.com/filecoin-project/lotus/pull/11747))
- fix: exchange: explicitly cast the block message limit const (#11511) ([filecoin-project/lotus#11511](https://github.com/filecoin-project/lotus/pull/11511))
- fix: Eth API: accept input data in call arguments under field 'input' (#11505) ([filecoin-project/lotus#11505](https://github.com/filecoin-project/lotus/pull/11505))
- fix: api: Length check the array sent to eth_feeHistory RPC (#11696) ([filecoin-project/lotus#11696](https://github.com/filecoin-project/lotus/pull/11696))
- fix: api: fix EthSubscribe tipsets off by one (#11858) ([filecoin-project/lotus#11858](https://github.com/filecoin-project/lotus/pull/11858))
- fix: lotus-provider: Fix log output format in wdPostTaskCmd ([filecoin-project/lotus#11504](https://github.com/filecoin-project/lotus/pull/11504))
- fix: lmcli: make 'sectors list' DDO-aware (#11839) ([filecoin-project/lotus#11839](https://github.com/filecoin-project/lotus/pull/11839))
- fix: lpwinning: Fix MiningBase.afterPropDelay ([filecoin-project/lotus#11654](https://github.com/filecoin-project/lotus/pull/11654))
- fix: exchange: allow up to 10k messages per block ([filecoin-project/lotus#11506](https://github.com/filecoin-project/lotus/pull/11506))
- fix: harmony: Fix task reclaim on restart ([filecoin-project/lotus#11498](https://github.com/filecoin-project/lotus/pull/11498))
- fix: lotus-provider: Wait for the correct taskID ([filecoin-project/lotus#11493](https://github.com/filecoin-project/lotus/pull/11493))
- fix: lotus-provider: show addresses in log ([filecoin-project/lotus#11490](https://github.com/filecoin-project/lotus/pull/11490))
- fix: sql Scan cannot write to an object ([filecoin-project/lotus#11485](https://github.com/filecoin-project/lotus/pull/11485))
- fix: lotus-provider: Fix winning PoSt ([filecoin-project/lotus#11482](https://github.com/filecoin-project/lotus/pull/11482))
- fix: lotus-provider: lotus-provider msg sending ([filecoin-project/lotus#11480](https://github.com/filecoin-project/lotus/pull/11480))
- fix: chain: use latest go-state-types types for miner UI ([filecoin-project/lotus#11566](https://github.com/filecoin-project/lotus/pull/11566))
- fix: Dockerfile non-interactive snapshot import (#11579) ([filecoin-project/lotus#11579](https://github.com/filecoin-project/lotus/pull/11579))
- fix: daemon: avoid prompting to remove chain when noninteractive (#11582) ([filecoin-project/lotus#11582](https://github.com/filecoin-project/lotus/pull/11582))
- fix: (events): check for sync-in-progress (#11932) ([filecoin-project/lotus#11932](https://github.com/filecoin-project/lotus/pull/11932))
- fix: curio: common commands (#11879) ([filecoin-project/lotus#11879](https://github.com/filecoin-project/lotus/pull/11879))
- fix: curio: fix incorrect null check for varchar column (#11881) ([filecoin-project/lotus#11881](https://github.com/filecoin-project/lotus/pull/11881))
- fix: local storage reservations fixes (#11866) ([filecoin-project/lotus#11866](https://github.com/filecoin-project/lotus/pull/11866))
- fix: curio: Check deal start epoch passed in PrecommitSubmit (#11873) ([filecoin-project/lotus#11873](https://github.com/filecoin-project/lotus/pull/11873))
- fix: curio: base config by default (#11676) ([filecoin-project/lotus#11676](https://github.com/filecoin-project/lotus/pull/11676))
- fix: curio: Start BoostAdapters before blocking rpc serve (#11871) ([filecoin-project/lotus#11871](https://github.com/filecoin-project/lotus/pull/11871))
- fix: cli: json flag (#11868) ([filecoin-project/lotus#11868](https://github.com/filecoin-project/lotus/pull/11868))
- feat: curio/lmrpc: Ingest backpressure (#11865) ([filecoin-project/lotus#11865](https://github.com/filecoin-project/lotus/pull/11865))
- feat: curio: Cleanup data copies after seal ops (#11847) ([filecoin-project/lotus#11847](https://github.com/filecoin-project/lotus/pull/11847))
- fix: spcli: add reference to the terminate command (#11851) ([filecoin-project/lotus#11851](https://github.com/filecoin-project/lotus/pull/11851))
- fix: sealing: improve gasEstimate logging (#11840) ([filecoin-project/lotus#11840](https://github.com/filecoin-project/lotus/pull/11840))
- fix: harmony: Try other tasks when storage claim fails
- fix: test: TestForkPreMigration hanging when env-var is set (#11838) ([filecoin-project/lotus#11838](https://github.com/filecoin-project/lotus/pull/11838))
- fix: piece: Don't return StartEport in PieceDealInfo.EndEpoch (#11832) ([filecoin-project/lotus#11832](https://github.com/filecoin-project/lotus/pull/11832))
- fix: paths/local: Fix on-disk storage accounting in new reservations (#11825) ([filecoin-project/lotus#11825](https://github.com/filecoin-project/lotus/pull/11825))
- fix: sealing pipeline: Fix panic on padding pieces in WaitDeals (#11708) ([filecoin-project/lotus#11708](https://github.com/filecoin-project/lotus/pull/11708))
- feat: ipfs: remove IPFS client backend (#11661) ([filecoin-project/lotus#11661](https://github.com/filecoin-project/lotus/pull/11661))
- fix: docs: Modify generate-lotus-cli.py to ignoring aliases. ([filecoin-project/lotus#11535](https://github.com/filecoin-project/lotus/pull/11535))
- fix: eth: decode as actor creation iff "to" is the EAM (#11520) ([filecoin-project/lotus#11520](https://github.com/filecoin-project/lotus/pull/11520))
- fix(events): properly decorate events db errors (#11856) ([filecoin-project/lotus#11856](https://github.com/filecoin-project/lotus/pull/11856))
- fix: CLI: adjust TermMax for extend-claim used by a different client (#11764) ([filecoin-project/lotus#11764](https://github.com/filecoin-project/lotus/pull/111764))
- fix: copy Flags field from SectorOnChainInfo (#11963) ([filecoin-project/lotus#11963](https://github.com/filecoin-project/lotus/pull/11963))
- feat: libp2p: Lotus stream cleanup (#11993) ([filecoin-project/lotus#11993](https://github.com/filecoin-project/lotus/pull/11993))

## Dependencies

- chore: update deps (#11819) ([filecoin-project/lotus#11819](https://github.com/filecoin-project/lotus/pull/11819))
- chore: mod: use upstream poseidon ([filecoin-project/lotus#11557](https://github.com/filecoin-project/lotus/pull/11557))
- deps: multiaddress ([filecoin-project/lotus#11558](https://github.com/filecoin-project/lotus/pull/11558))
- chore:libp2p: update libp2p deps in master ([filecoin-project/lotus#11522](https://github.com/filecoin-project/lotus/pull/11522))
- dep: go-multi-address ([filecoin-project/lotus#11563](https://github.com/filecoin-project/lotus/pull/11563))
- chore: update go-data-transfer and go-graphsync (#12000) ([filecoin-project/lotus#12000](https://github.com/filecoin-project/lotus/pull/2000))
- chore: update drand (#12021) ([filecoin-project/lotus#12021](https://github.com/filecoin-project/lotus/pull/12021))
- chore: libp2p: update to v0.34.1 (12027) ([filecoin-project/lotus#12027](https://github.com/filecoin-project/lotus/pull/12027))
- github.com/filecoin-project/go-amt-ipld/ (v4.2.0 -> v4.3.0)
- github.com/filecoin-project/go-state-types (v0.13.1 -> v0.13.3)
- github.com/libp2p/go-libp2p-pubsub (v0.10.0 -> v0.10.1)
- github.com/libp2p/go-libp2p (v0.33.2 -> v0.34.1)

## Others

- ci: ci: create gh workflow that runs go checks (#11761) ([filecoin-project/lotus#11761](https://github.com/filecoin-project/lotus/pull/11761))
- ci: ci: create gh workflow that runs go build (#11760) ([filecoin-project/lotus#11760](https://github.com/filecoin-project/lotus/pull/11760))
- ci: cancel in progress runs on pull requests only (#11842) ([filecoin-project/lotus#11842](https://github.com/filecoin-project/lotus/pull/11842))
- ci: ci: list processes before calling apt-get to enable debugging (#11815) ([filecoin-project/lotus#11815](https://github.com/filecoin-project/lotus/pull/11815))
- ci: ci: allow master main sync to write to the repository (#11784) ([filecoin-project/lotus#11784](https://github.com/filecoin-project/lotus/pull/11784))
- ci: ci: create gh workflow that runs go tests (#11762) ([filecoin-project/lotus#11762](https://github.com/filecoin-project/lotus/pull/11762))
- ci: ci: deprecate circle ci in favour of github actions (#11786) ([filecoin-project/lotus#11786](https://github.com/filecoin-project/lotus/pull/1786))
- misc: Drop the raft-cluster experiment ([filecoin-project/lotus#11468](https://github.com/filecoin-project/lotus/pull/11468))
- chore: fix some typos in comments (#11892) ([filecoin-project/lotus#11892](https://github.com/filecoin-project/lotus/pull/11892))
- chore: fix typos (#11848) ([filecoin-project/lotus#11848](https://github.com/filecoin-project/lotus/pull/11848))
- chore: fix typo (#11697) ([filecoin-project/lotus#11697](https://github.com/filecoin-project/lotus/pull/11697))
- chore: fix 2 typo's (#11542) ([filecoin-project/lotus#11542](https://github.com/filecoin-project/lotus/pull/11542))
- chore: calibnet: Update bootstrap peer list ([filecoin-project/lotus#11672](https://github.com/filecoin-project/lotus/pull/11672))
- chore: build: Bump version in master ([filecoin-project/lotus#11475](https://github.com/filecoin-project/lotus/pull/11475))
- chore: releases: merge releases branch to master ([filecoin-project/lotus#11578](https://github.com/filecoin-project/lotus/pull/11578))
- chore: Add systemd memory note on install and in config (#11641) ([filecoin-project/lotus#11641](https://github.com/filecoin-project/lotus/pull/11641))
- chore: switch back to upstream ledger library (#11651) ([filecoin-project/lotus#11651](https://github.com/filecoin-project/lotus/pull/11651))
- chore: build: update minimum go version to 1.21.7 (#11652) ([filecoin-project/lotus#11652](https://github.com/filecoin-project/lotus/pull/11652))
- chore: docs: nv-skeleton documentation (#11065) ([filecoin-project/lotus#11065](https://github.com/filecoin-project/lotus/pull/11065))
- chore: Add v13 support to invariants-checker (#11931) ([filecoin-project/lotus#11931](https://github.com/filecoin-project/lotus/pull/11931))
- chore: remove unmaintained bootstrappers (#11983) ([filecoin-project/lotus#11983](https://github.com/filecoin-project/lotus/pull/11983))
- chore: go mod: revert go version change as it breaks Docker build (#12050) ([filecoin-project/lotus#12050](https://github.com/filecoin-project/lotus/pull/12050))
- chore: pin golanglint-ci to v1.58.2 ([filecoin-project/lotus#12054](https://github.com/filecoin-project/lotus/pull/12054))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Rod Vagg | 20 | +55315/-204 | 58 |
| Łukasz Magiera | 201 | +16244/-6541 | 647 |
| Andrew Jackson (Ajax) | 53 | +15293/-6764 | 394 |
| Phi-rjan | 6 | +12669/-4521 | 221 |
| LexLuthr | 20 | +5972/-2815 | 120 |
| Steven Allen | 22 | +1626/-1264 | 77 |
| Piotr Galar | 9 | +790/-412 | 33 |
| Aayush Rajasekaran | 4 | +642/-509 | 12 |
| Lee | 1 | +601/-533 | 9 |
| qwdsds | 3 | +617/-510 | 11 |
| Phi | 11 | +551/-83 | 32 |
| Jiaying Wang | 5 | +433/-20 | 13 |
| Masih H. Derkani | 4 | +350/-101 | 18 |
| Aayush | 4 | +143/-76 | 17 |
| Aarsh Shah | 3 | +63/-11 | 5 |
| jennijuju | 3 | +22/-22 | 12 |
| hunjixin | 1 | +21/-14 | 4 |
| beck | 2 | +17/-17 | 2 |
| tom123222 | 2 | +28/-4 | 2 |
| Ian Norden | 1 | +21/-1 | 1 |
| ZenGround0 | 1 | +3/-15 | 1 |
| shuangcui | 1 | +7/-7 | 6 |
| Vid Bregar | 1 | +7/-4 | 2 |
| writegr | 1 | +5/-5 | 5 |
| Nagaprasad V R | 1 | +9/-0 | 1 |
| forcedebug | 1 | +4/-4 | 4 |
| parthshah1 | 2 | +6/-1 | 2 |
| fuyangpengqi | 1 | +3/-3 | 3 |
| Samuel Arogbonlo | 1 | +6/-0 | 2 |
| GlacierWalrus | 1 | +0/-6 | 1 |
| Aloxaf | 1 | +6/-0 | 2 |
| Rob Quist | 2 | +2/-3 | 3 |
| wersfeds | 1 | +2/-2 | 1 |
| Jon | 1 | +2/-0 | 1 |
| 0x5459 | 1 | +1/-0 | 1 |

# v1.26.3 / 2024-04-22

**This is a patch release that addresses high memory load concerns for the Lotus daemon in the coming network migration for network version 22, scheduled on epoch `3855360 - 2024-04-24 - 14:00:00Z`.**

If your Lotus daemon is running on a machine with less memory and swap than 160GB, you should upgrade to this patch release to ensure you do not encounter any Out-Of-Memory issues during the pre-migration.

# v1.26.2 / 2024-04-08

**This is a mandatory patch release for the Filecoin network version 22 mainnet upgrade, for all node operators.**

There is an update in the upgrade epoch for nv22, you can read the [full discussion in Slack here.](https://filecoinproject.slack.com/archives/C05P37R9KQD/p1712548103521969)

The new upgrade epoch is scheduled to be on **epoch `3855360 - 2024-04-24 - 14:00:00Z`**. That means:

- **All mainnet node operators that have upgraded to v1.26.x, must upgrade to this patch release before 2024-04-11T14:00:00Z.**
- **All mainnet node operators that are on a version lower the v1.26.x, must upgrade to this patch release before 2024-04-24T14:00:00Z.**

This patch also includes fixes for node operators who want to index builtin-actor events after the nv22 upgrade. Specifically, it ensures the builtin actor event entries are ordered by insertion order when selected ([#11834](https://github.com/filecoin-project/lotus/pull/11834)). It also includes a couple Lotus-Miner patch fixes, ensuring that SnapDeals works properly and are using the new ProveReplicaUpdate3 message after the network version 22 upgrade, ensuring that DDO-sectors has the correct sector expirations, as well as DDO-sector visibility in the `lotus-miner sectors list` cmd.

## Upgrade Warnings

For users currently on a version of Lotus lower than v1.26.0, please note that **this release requires a minimum Go version of v1.21.7 or higher to successfully build Lotus.**

## v1.26.x Inclusions

See the [v1.26.0](#v1260--2024-03-21) release notes below for inclusions and notes on the v1.26.x series.

* [v13 Builtin Actor Bundle](#v13-builtin-actor-bundle)
* [Migration](#migration)
* [New features](#new-features-1)
  * [Tracing API](#tracing-api)
  * [Ethereum Tracing API (`trace_block` and `trace_replayBlockTransactions`)](#ethereum-tracing-api-trace_block-and-trace_replayblocktransactions)
  * [GetActorEventsRaw and SubscribeActorEventsRaw](#getactoreventsraw-and-subscribeactoreventsraw)
  * [Events Configuration Changes](#events-configuration-changes)
  * [GetAllClaims and GetAllAlocations](#getallclaims-and-getallalocations)
  * [Lotus CLI](#lotus-cli)

#v1260--2024-03-21

# v1.26.1 / 2024-03-27

***RETRACTED: Due to a change in network version 22 upgrade epoch, Lotus v1.26.1 should not be used prior to the new upgrade epoch. See v1.26.2 release notes above.***

**This is a patch release for the Calibration network user.** The Calibration network is scheduled for an upgrade to include the two additional built-in actor events to ease the transition and observability of DDO for the ecosystem ([#964](https://github.com/filecoin-project/FIPs/pull/964) and [#968](https://github.com/filecoin-project/FIPs/pull/968)).

The agreed-upon epoch between the Filecoin implementer team for the update is `1493854`, corresponding to `2024-04-03T11:00:00Z`. All Calibration network users need to upgrade to this patch release before that.

 **Lotus Mainnet Users**: For users on the Mainnet, the [Lotus v1.26.0](https://github.com/filecoin-project/lotus/releases/tag/v1.26.0) release already includes the aforementioned events in preparation for the Mainnet nv22 upgrade. Therefore, both v1.26.0 and v1.26.1 versions are suitable for use on the Mainnet for the coming network version 22 upgrade.

# v1.26.0 / 2024-03-21

***RETRACTED: Due to a change in network version 22 upgrade epoch, Lotus v1.26.0 should not be used prior to the new upgrade epoch. See v1.26.2 release notes above.***

This is the stable release for the upcoming MANDATORY Filecoin network upgrade v22, codenamed Dragon 🐉, at `epoch 3817920 - 2024-04-11 - 14:00:00Z`

The Filecoin network version 22 delivers the following FIPs:

- [FIP-0063: Switching to new Drand mainnet network](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0063.md)
- [FIP-0074: Remove cron-based automatic deal settlement](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0074.md)
- [FIP-0076: Direct data onboarding](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0076.md)
- [FIP-0083: Add built-in Actor events in the Verified Registry, Miner and Market Actors](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0083.md)

## ☢️ Upgrade Warnings ☢️

- This release requires a minimum Go version of v1.21.7 or higher to successfully build Lotus.

## v13 Builtin Actor Bundle

[Builtin actor v13.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v13.0.0) is used for supporting this upgrade. Make sure that your lotus actor bundle matches the v13 actors manifest by running the following cli after upgrading:

```
lotus state actor-cids --network-version=22
Network Version: 22
Actor Version: 13

Manifest CID: bafy2bzacecdhvfmtirtojwhw2tyciu4jkbpsbk5g53oe24br27oy62sn4dc4e

Actor             CID  
account           bafk2bzacedxnbtlsqdk76fsfmnhyvsblwyfducerwwtp3mqtx2wbrvs5idl52
cron              bafk2bzacebbopddyn5csb3fsuhh2an4ttd23x6qnwixgohlirj5ahtcudphyc
datacap           bafk2bzaceah42tfnhd7xnztawgf46gbvc3m2gudoxshlba2ucmmo2vy67t7ci
eam               bafk2bzaceb23bhvvcjsth7cn7vp3gbaphrutsaz7v6hkls3ogotzs4bnhm4mk
ethaccount        bafk2bzaceautge6zhuy6jbj3uldwoxwhpywuon6z3xfvmdbzpbdribc6zzmei
evm               bafk2bzacedq6v2lyuhgywhlllwmudfj2zufzcauxcsvvd34m2ek5xr55mvh2q
init              bafk2bzacedr4xacm3fts4vilyeiacjr2hpmwzclyzulbdo24lrfxbtau2wbai
multisig          bafk2bzacecr5zqarfqak42xqcfeulsxlavcltawsx2fvc7zsjtby6ti4b3wqc
paymentchannel    bafk2bzacebntdhfmyc24e7tm52ggx5tnw4i3hrr3jmllsepv3mibez4hywsa2
placeholder       bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro
reward            bafk2bzacedq4q2kwkruu4xm7rkyygumlbw2yt4nimna2ivea4qarvtkohnuwu
storagemarket     bafk2bzacebjtoltdviyznpj34hh5qp6u257jnnbjole5rhqfixm7ug3epvrfu
storageminer      bafk2bzacebf4rrqyk7gcfggggul6nfpzay7f2ordnkwm7z2wcf4mq6r7i77t2
storagepower      bafk2bzacecjy4dkulvxppg3ocbmeixe2wgg6yxoyjxrm4ko2fm3uhpvfvam6e
system            bafk2bzacecyf523quuq2kdjfdvyty446z2ounmamtgtgeqnr3ynlu5cqrlt6e
verifiedregistry  bafk2bzacedkxehp7y7iyukbcje3wbpqcvufisos6exatkanyrbotoecdkrbta
```

## Migration

We are expecting a bit heavier than normal state migration for this upgrade due to the amount of state changes introduced with Direct Data Onboarding.

All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 120 epochs before the upgrade. It will take around 10-20 minutes for the pre-migration and less than 30 seconds for the final migration, depending on the amount of historical state in the node blockstore and the hardware specs the node is running on. During this time, expect slower block validation times, increased CPU and memory usage, and longer delays for API queries

We recommend node operators (who haven't enabled splitstore discard mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

You can test out the migration by running the [`benchmarking a network migration` tutorial.](https://lotus.filecoin.io/kb/test-migration/)

For certain node operators, such as full archival nodes or systems that need to keep large amounts of state (RPC providers), completing the pre-migration in time before the network upgrade might not be achievable. For those node operators, it is recommended to skip the pre-migration and run the non-cached migration (i.e., just running the migration at the exact upgrade epoch), and schedule for some downtime during the upgrade epoch. Operators of such nodes can read the [`How to disable premigration in network upgrade` tutorial.](https://lotus.filecoin.io/kb/disable-premigration/)

## New features
- feat: api: new verified registry methods to get all allocations and claims (#11631) ([filecoin-project/lotus#11631](https://github.com/filecoin-project/lotus/pull/11631))
- feat: sealing: Support nv22 DDO features in the sealing pipeline (#11226) ([filecoin-project/lotus#11226](https://github.com/filecoin-project/lotus/pull/11226))
- feat: implement FIP-0063 ([filecoin-project/lotus#11572](https://github.com/filecoin-project/lotus/pull/11572))
- feat: events: Add Lotus APIs to consume smart contract and built-in actor events ([filecoin-project/lotus#11618](https://github.com/filecoin-project/lotus/pull/11618))

### Tracing API

Replace the `CodeCid` field in the message trace (added in 1.23.4) with an `InvokedActor` field.

**Before:**

```javascript
{
    "Msg": {
        "From": ...,
        "To": ...,
        ...
        "CodeCid": ... // The actor's code CID.
    }
    "MsgRct": ...,
    "GasCharges": [],
    "Subcalls": [],
}
```

**After:**

```javascript
{
    "Msg": {
        "From": ...,
        "To": ...
    }
    "InvokedActor": {         // The invoked actor (ommitted if the actor wasn't invoked).
        "Id": 1234,           // The ID of the actor.
        "State": {            // The actor's state object (may change between network versions).
           "Code": ...,       // The actor's code CID.
           "Head": ...,       // The actor's state-root (when invoked).
           "CallSeqNum": ..., // The actor's nonce.
           "Balance": ...,    // The actor's balance (when invoked).
           "Address": ...,    // Delegated address (FEVM only).
        }
    }
    "MsgRct": ...,
    "GasCharges": [],
    "Subcalls": [],
}
```

This means the trace now contains an accurate "snapshot" of the actor at the time of the call, information that may not be present in the final state-tree (e.g., due to reverts). This will hopefully improve the performance and accuracy of indexing services.

### Ethereum Tracing API (`trace_block` and `trace_replayBlockTransactions`)

For those with the Ethereum JSON-RPC API enabled, the experimental Ethereum Tracing API has been improved significantly and should be considered "functional". However, it's still new and should be tested extensively before relying on it. This API translates FVM traces to Ethereum-style traces, implementing the OpenEthereum `trace_block` and `trace_replayBlockTransactions` APIs.

This release fixes numerous bugs with this API and now ABI-encodes non-EVM inputs/outputs as if they were explicit EVM calls to [`handle_filecoin_method`][handlefilecoinmethod] for better block explorer compatibility.

However, there are some _significant_ limitations:

1. The Geth APIs are not implemented, only the OpenEthereum (Erigon, etc.) APIs.
2. Block rewards are not (yet) included in the trace.
3. Selfdestruct operations are not included in the trace.
4. EVM smart contract "create" events always specify `0xfe` as the "code" for newly created EVM smart contracts.

Additionally, Filecoin is not Ethereum no matter how much we try to provide API/tooling compatibility. This API attempts to translate Filecoin semantics into Ethereum semantics as accurately as possible, but it's hardly the best source of data unless you _need_ Filecoin to look like an Ethereum compatible chain. If you're trying to build a new integration with Filecoin, please use the native `StateCompute` method instead.

[handlefilecoinmethod]: https://fips.filecoin.io/FIPS/fip-0054.html#handlefilecoinmethod-general-handler-for-method-numbers--1024

### GetActorEventsRaw and SubscribeActorEventsRaw

[FIP-0049](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0049.md) introduced _Actor Events_ that can be emitted by user programmed actors. [FIP-0083](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0083.md) introduces new events emitted by the builtin Verified Registry, Miner and Market Actors. These new events for builtin actors are being activated with network version 22 to coincide with _Direct Data Onboarding_ as defined in [FIP-0076](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0076.md) which introduces additional flexibility for data onboarding. Sector, Deal and DataCap lifecycles can be tracked with these events, providing visibility and options for programmatic responses to changes in state.

Actor events are available on message receipts, but can now be retrieved from a node using the new `GetActorEventsRaw` and `SubscribeActorEventsRaw` methods. These methods allow for querying and subscribing to actor events, respectively. They depend on the Lotus node both collecting events (with `Fevm.Events.RealTimeFilterAPI` and `Fevm.Events.HistoricFilterAPI`) and being enabled with the new configuration option `Events.EnableActorEventsAPI`. Note that a Lotus node can only respond to requests for historic events that it retains in its event store.

Both `GetActorEventsRaw` and `SubscribeActorEventsRaw` take a filter parameter which can optionally filter events on:

* `Addresses` of the actor(s) emitting the event
* Specific `Fields` within the event
* `FromHeight` and `ToHeight` to filter events by block height
* `TipSetKey` to restrict events contained within a specific tipset

`GetActorEventsRaw` provides a one-time query for actor events, while `SubscribeActorEventsRaw` provides a long-lived connection (via websockets) to the Lotus node, allowing for real-time updates on actor events. The subscription can be cancelled by the client at any time.

A future Lotus release may include `GetActorEvents` and `SubscribeActorEvents` methods which will provide a more user-friendly interface to actor events, including deserialization of event data.

### Events Configuration Changes

All configuration options previously under `Fevm.Events` are now in the top-level `Events` section along with the new `Events.EnableActorEventsAPI` option mentioned above. If you have non-default options in `[Events]` under `[Fevm]` in your configuration file, please move them to the top-level `[Events]`.

While `Fevm.Events.*` options are deprecated and replaced by `Events.*`, any existing custom values will be respected if their new form isn't set, but a warning will be printed to standard error upon startup. Support for these deprecated options will be removed in a future Lotus release, so please migrate your configuration promptly.

### GetAllClaims and GetAllAlocations

Additionally the methods `GetAllAllocations` and `GetAllClaims` has been added to the Lotus API. These methods lists all the available allocations and claims available in the actor state.

### Lotus CLI

The `filplus` commands used for listing allocations and claims have been updated. If no argument is provided to the either command, they will list out all the allocations and claims in the verified registry actor.
The output list columns have been modified to `AllocationID` and `ClaimID` instead of ID. 

```shell
lotus filplus list-allocations --help
NAME:
   lotus filplus list-allocations - List allocations available in verified registry actor or made by a client if specified

USAGE:
   lotus filplus list-allocations [command options] clientAddress

OPTIONS:
   --expired   list only expired allocations (default: false)
   --json      output results in json format (default: false)
   --help, -h  show help


lotus filplus list-claims --help     
NAME:
   lotus filplus list-claims - List claims available in verified registry actor or made by provider if specified

USAGE:
   lotus filplus list-claims [command options] providerAddress

OPTIONS:
   --expired   list only expired claims (default: false)
   --help, -h  show help
```

## Dependencies
- github.com/filecoin-project/go-state-types (v0.12.8 -> v0.13.1)
- chore: deps: update to go-state-types v13.0.0-rc.1 ([filecoin-project/lotus#11662](https://github.com/filecoin-project/lotus/pull/11662))
- chore: deps: update to go-state-types v13.0.0-rc.2 ([filecoin-project/lotus#11675](https://github.com/filecoin-project/lotus/pull/11675))
- chore: deps: update to go-multiaddr v0.12.2 (#11602) ([filecoin-project/lotus#11602](https://github.com/filecoin-project/lotus/pull/11602))
- feat: fvm: update the FVM/FFI to v4.1 (#11608) (#11612) ([filecoin-project/lotus#11612](https://github.com/filecoin-project/lotus/pull/11612))
- chore: deps: update builtin-actors, GST, verified claims tests ([filecoin-project/lotus#11768](https://github.com/filecoin-project/lotus/pull/11768))

## Others
- Remove PL operated bootstrap nodes from mainnet.pi ([filecoin-project/lotus#11491](https://github.com/filecoin-project/lotus/pull/11491))
- Update epoch heights (#11637) ([filecoin-project/lotus#11637](https://github.com/filecoin-project/lotus/pull/11637))
- chore: Set upgrade heights and change codename ([filecoin-project/lotus#11599](https://github.com/filecoin-project/lotus/pull/11599))
- chore:: backport #11609 to the feat/nv22 branch (#11644) ([filecoin-project/lotus#11644](https://github.com/filecoin-project/lotus/pull/11644))
- fix: add UpgradePhoenixHeight to StateGetNetworkParams (#11648) ([filecoin-project/lotus#11648](https://github.com/filecoin-project/lotus/pull/11648))
- feat: drand quicknet: allow scheduling drand quicknet upgrade before nv22 on 2k devnet ([filecoin-project/lotus#11667]https://github.com/filecoin-project/lotus/pull/11667)
- chore: backport #11632 to release/v1.26.0 ([filecoin-project/lotus#11667](https://github.com/filecoin-project/lotus/pull/11667))
- release: bump to v1.26.0-rc2 ([filecoin-project/lotus#11691](https://github.com/filecoin-project/lotus/pull/11691))
- Docs: Drand: document the meaning of "IsChained ([filecoin-project/lotus#11692](https://github.com/filecoin-project/lotus/pull/11692))
- chore: remove old calibnet bootstrappers ([filecoin-project/lotus#11702](https://github.com/filecoin-project/lotus/pull/11702))
- chore: Add lotus-provider to build to match install ([filecoin-project/lotus#11616](https://github.com/filecoin-project/lotus/pull/11616))
- new: add forest bootstrap nodes (#11636) ([filecoin-project/lotus#11636](https://github.com/filecoin-project/lotus/pull/11636))

# v1.25.2 / 2024-01-11 

This is an optional but **highly recommended feature release** of Lotus, as it includes fixes for synchronizations issues that users have experienced. The feature release also introduces `Lotus-Provider` in its alpha testing phase, as well as the ability to call external PC2-binaries during the sealing process.

## ☢️ Upgrade Warnings ☢️

There are no upgrade warnings for this feature release.

## ⭐️ Highlights ⭐️

### Lotus-Provider
The feature release ships the alpha release of the new Lotus-Provider binary, together with its initial features - High Availability of WindowPoSt and WinningPoSt.

So what is so exciting about Lotus-Provider:

**High Availability**
- You can run as many `Lotus-Provider` instances as you want for both WindowPoSt and WinningPOSt.
- You can connect them to as many clustered Yugabyte instances as you want to. This allows for an NxN configuration where all instances can communicate with all others.
- You have the option to connect different instances to different chain daemons.

**Simplicity**
- Once the configuration is in the database, setting up a new machine with Lotus-Provider is straightforward. Simply start the binary with the correct flags to find YugabyteDB and specify which configuration layers it should use.

**Durability**
- `Lotus-Provider` is designed with robustness in mind. Updates to the system are handled seamlessly, ensuring that performance and stability are maintained when taking down machines in your cluster for updates.

Read more about [`Lotus-Provider` in the documentation here](https://lotus.filecoin.io/storage-providers/lotus-provider/overview/). And check out the how you can migrate from [Lotus-Miner to Lotus-Provider here](https://lotus.filecoin.io/storage-providers/lotus-provider/setup/). **(Only recommended in testnets while its in Alpha)**

### External PC2-binaries 

In this feature release, storage providers can call external PC2-binaries during the sealing process. This allows storage providers to leverage the SupraSeal PC2 binary, which has been shown to improve sealing speed in the PC2-phase. For instance, our current benchmarks show that an NVIDIA RTX A5000 card was able to complete PC2 in approximately 2.5 minutes.

We have verified that SupraSeal PC2 functions properly with Committed Capacity (CC) sectors, both SyntheticPoReps and non-Synthetic PoReps. However calling SupraSeal PC2 with deal sectors is not supported in this feature release.

For more information on how to use SupraSeal PC2 with your `lotus-worker`, as well as how to use feature, please [refer to the documentation](https://lotus.filecoin.io/tutorials/lotus-miner/supra-seal-pc2/).

## New features
- feat: sturdypost work branch ([filecoin-project/lotus#11405](https://github.com/filecoin-project/lotus/pull/11405))
   - Adds the `Lotus-Provider` binary, and the HarmonyDB framework.
- feat: worker: Support delegating precommit2 to external binary ([filecoin-project/lotus#11185](https://github.com/filecoin-project/lotus/pull/11185))
   - Allows for delegating PreCommit2 to an exteranl binary.
- feat: build: Add SupraSeal-PC2 binary script ([filecoin-project/lotus#11430](https://github.com/filecoin-project/lotus/pull/11430))
   - Adds a script for building the SupraSeal-PC2 binary easily.
- Feat: daemon: Auto remove existing chain if importing chain file or snapshot ([filecoin-project/lotus#11277](https://github.com/filecoin-project/lotus/pull/11277))
   - Auto removes the existing chain when importing a snapshot. 
- feat: Add ETA to lotus sync wait (#11211) ([filecoin-project/lotus#11211](https://github.com/filecoin-project/lotus/pull/11211))
   - Adds a ETA indicator to `lotus sync wait`, so you can get an estimate for how long until sync is completed.
- feat: mpool/wdpost: Maximize feecap config ([filecoin-project/lotus#9746](https://github.com/filecoin-project/lotus/pull/9746))
   - Adds a Maximixe FeeCap Config
- feat: Add lotus-bench cli option to stress test any binary ([filecoin-project/lotus#11270](https://github.com/filecoin-project/lotus/pull/11270))
   - Enables the `Lotus-Bench` to run any binary and analyze their latency and histogram distribution, track most common errors, perform stress testing under different concurrency levels and see how it works under different QPS.
- feat: chain import: don't walk to genesis - 2-3x faster snapshot import (#11446) ([filecoin-project/lotus#11446](https://github.com/filecoin-project/lotus/pull/11446))
   - Improves Snapshot import speed, by not walking back to genesis on import.
- feat: metric: export Mpool message count ([filecoin-project/lotus#11361](https://github.com/filecoin-project/lotus/pull/11361))
   - Adds the mpool count as a prometheus metric.
- feat: bench: flag to output GenerateWinningPoStWithVanilla params ([filecoin-project/lotus#11460](https://github.com/filecoin-project/lotus/pull/11460))

## Improvements
- feat: bootstrap: add glif bootstrap node on calibration ([filecoin-project/lotus#11175](https://github.com/filecoin-project/lotus/pull/11175))
- fix: bench: Set ticket and seed to a non-all zero value ([filecoin-project/lotus#11429](https://github.com/filecoin-project/lotus/pull/11429))
- fix: alert: Check UDPbuffer-size ([filecoin-project/lotus#11360](https://github.com/filecoin-project/lotus/pull/11360))
- feat: cli: sort actor CIDs alphabetically before printing (#11345) ([filecoin-project/lotus#11345](https://github.com/filecoin-project/lotus/pull/11345))
- fix: worker: Connect when --listen is not set ([filecoin-project/lotus#11294](https://github.com/filecoin-project/lotus/pull/11294))
- fix: miner info: Show correct sector state counts ([filecoin-project/lotus#11456](https://github.com/filecoin-project/lotus/pull/11456))
- feat: miner: defensive check for equivocation ([filecoin-project/lotus#11321](https://github.com/filecoin-project/lotus/pull/11321))
- feat: Instructions for setting up Grafana/Prometheus for monitoring local lotus node ([filecoin-project/lotus#11276](https://github.com/filecoin-project/lotus/pull/11276))
- fix: cli: Wrap error in wallet sign ([filecoin-project/lotus#11273](https://github.com/filecoin-project/lotus/pull/11273))
- fix: Add time slicing to splitstore purging to reduce lock congestion ([filecoin-project/lotus#11269](https://github.com/filecoin-project/lotus/pull/11269))
- feat: sealing: load SectorsSummary from sealing SectorStats instead of calling API each time ([filecoin-project/lotus#11353](https://github.com/filecoin-project/lotus/pull/11353))
- fix: shed: additional metrics in `mpool miner-select-messages` ([filecoin-project/lotus#11253](https://github.com/filecoin-project/lotus/pull/11253))
- storage: Return soft err when sector alloc fails in acquire ([filecoin-project/lotus#11338](https://github.com/filecoin-project/lotus/pull/11338))
- feat: miner: log detailed timing breakdown when mining takes longer than the block's timestamp ([filecoin-project/lotus#11228](https://github.com/filecoin-project/lotus/pull/11228))
- fix: shed: make invariants checker work with splitstore ([filecoin-project/lotus#11391](https://github.com/filecoin-project/lotus/pull/11391))
- feat: eth: encode eth tx input as solidity ABI (#11402) ([filecoin-project/lotus#11402](https://github.com/filecoin-project/lotus/pull/11402))
- fix: eth: use the correct state-tree when resolving addresses (#11387) ([filecoin-project/lotus#11387](https://github.com/filecoin-project/lotus/pull/11387))
- fix: eth: remove trace sanity check (#11385) ([filecoin-project/lotus#11385](https://github.com/filecoin-project/lotus/pull/11385))
- fix: chain: make failure to load the chain state fatal (#11426) ([filecoin-project/lotus#11426](https://github.com/filecoin-project/lotus/pull/11426))
- fix: build: an epoch is near an upgrade iff the upgrade is enabled (#11401) ([filecoin-project/lotus#11401](https://github.com/filecoin-project/lotus/pull/11401))
- fix: eth: handle unresolvable addresses (#11433) ([filecoin-project/lotus#11433](https://github.com/filecoin-project/lotus/pull/11433))
- fix: eth: correctly encode and simplify native input/output encoding (#11382) ([filecoin-project/lotus#11382](https://github.com/filecoin-project/lotus/pull/11382))
- fix: worker: listen for interrupt signals in GetStorageMinerAPI loop (#11309) ([filecoin-project/lotus#11309](https://github.com/filecoin-project/lotus/pull/11309))
- fix: sync: iterate over returned messages directly (#11373) ([filecoin-project/lotus#11373](https://github.com/filecoin-project/lotus/pull/11373))
- fix: miner: correct duration logs in mineOne ([filecoin-project/lotus#11241](https://github.com/filecoin-project/lotus/pull/11241))
- fix: cli: Add print to unseal cmd ([filecoin-project/lotus#11271](https://github.com/filecoin-project/lotus/pull/11271))
- fix: networking: avoid dialing when trying to handshake peers ([filecoin-project/lotus#11262](https://github.com/filecoin-project/lotus/pull/11262))
- metric milliseconds computation with golang original method (#11403) ([filecoin-project/lotus#11403](https://github.com/filecoin-project/lotus/pull/11403))
- feat: shed: fix blockstore prune (#11197) ([filecoin-project/lotus#11197](https://github.com/filecoin-project/lotus/pull/11197))
- refactor:ffi: replace ClearLayerData with ClearCache (#11352) ([filecoin-project/lotus#11352](https://github.com/filecoin-project/lotus/pull/11352))
- fix: api: compute gasUsedRatio based on max gas in the tipset (#11354) ([filecoin-project/lotus#11354](https://github.com/filecoin-project/lotus/pull/11354))
- fix: api: compute the effective gas cost with the correct base-fee (#11357) ([filecoin-project/lotus#11357](https://github.com/filecoin-project/lotus/pull/11357))
- fix: api: return errors on failure to lookup an eth txn receipt (#11329) ([filecoin-project/lotus#11329](https://github.com/filecoin-project/lotus/pull/11329))
- fix: api: exclude reverted events in `eth_getLogs` results (#11318) ([filecoin-project/lotus#11318](https://github.com/filecoin-project/lotus/pull/11318))
- api: Add block param to eth_estimateGas ([filecoin-project/lotus#11462](https://github.com/filecoin-project/lotus/pull/11462))
- opt: fix duplicate check exitcode ([filecoin-project/lotus#11171](https://github.com/filecoin-project/lotus/pull/11171))
- fix: lotus-provider: show addresses in log ([filecoin-project/lotus#11490](https://github.com/filecoin-project/lotus/pull/11490))
- fix: lotus-provider: Wait for the correct taskID ([filecoin-project/lotus#11493](https://github.com/filecoin-project/lotus/pull/11493))
- harmony: Fix task reclaim on restart ([filecoin-project/lotus#11498](https://github.com/filecoin-project/lotus/pull/11498))
- fix: lotus-provider: Fix log output format in wdPostTaskCmd ([filecoin-project/lotus#11504](https://github.com/filecoin-project/lotus/pull/11504))
- fix: lp docsgen ([filecoin-project/lotus#11488](https://github.com/filecoin-project/lotus/pull/11488))
- fix: lotus-provider do not suggest default layer ([filecoin-project/lotus#11486](https://github.com/filecoin-project/lotus/pull/11486))
- feat: syncer: optimize syncFork for one-epoch forks ([filecoin-project/lotus#11533](https://github.com/filecoin-project/lotus/pull/11533))
- fix: sync: do not include incoming in return of syncFork ([filecoin-project/lotus#11541](https://github.com/filecoin-project/lotus/pull/11541))
- fix: wdpost: fix vanilla proof indexes ([filecoin-project/lotus#11550](https://github.com/filecoin-project/lotus/pull/11550))
- feat: exchange: change GetBlocks to always fetch the requested number of tipsets ([filecoin-project/lotus#11565](https://github.com/filecoin-project/lotus/pull/11565))

## Dependencies
- update go-libp2p to v0.31.0 ([filecoin-project/lotus#11225](https://github.com/filecoin-project/lotus/pull/11225))
- deps: gostatetype (#11437) ([filecoin-project/lotus#11437](https://github.com/filecoin-project/lotus/pull/11437))
- fix: deps: stop using go-libp2p deprecated peer.ID.Pretty ([filecoin-project/lotus#11263](https://github.com/filecoin-project/lotus/pull/11263))
- chore:libp2p:update libp2p deps in release-v1.25.2 to v0.31.1 ([filecoin-project/lotus#11524](https://github.com/filecoin-project/lotus/pull/11524))
- deps: update go-multiaddr to v0.12.0 ([filecoin-project/lotus#11524](https://github.com/filecoin-project/lotus/pull/11558))
- dep: go-multi-address to v0.12.1 ([filecoin-project/lotus#11564](https://github.com/filecoin-project/lotus/pull/11564))

## Others
- chore: update FFI (#11431) ([filecoin-project/lotus#11431](https://github.com/filecoin-project/lotus/pull/11431))
- chore: build: bump master to v1.25.1-dev ([filecoin-project/lotus#11450](https://github.com/filecoin-project/lotus/pull/11450))
- chore: releases :merge releases into master  ([filecoin-project/lotus#11448](https://github.com/filecoin-project/lotus/pull/11448))
- chore: actors: update v12 to the final release ([filecoin-project/lotus#11440](https://github.com/filecoin-project/lotus/pull/11440))
- chore: Remove ipfs main bootstrap nodes (#11200) ([filecoin-project/lotus#11200](https://github.com/filecoin-project/lotus/pull/11200))
- Remove PL's european bootstrap nodes from mainnet.pi ([filecoin-project/lotus#11315](https://github.com/filecoin-project/lotus/pull/11315))
- chore: deps: update to go-state-types v0.12.7 ([filecoin-project/lotus#11428](https://github.com/filecoin-project/lotus/pull/11428))
- fix: Add .vscode to gitignore ([filecoin-project/lotus#11275](https://github.com/filecoin-project/lotus/pull/11275))
- fix: test: temporarily exempt SynthPorep constants from test ([filecoin-project/lotus#11259](https://github.com/filecoin-project/lotus/pull/11259))
- feat: skip TestSealAndVerify3 until it's fixed ([filecoin-project/lotus#11230](https://github.com/filecoin-project/lotus/pull/11230))
- Update RELEASE_ISSUE_TEMPLATE.md ([filecoin-project/lotus#11250](https://github.com/filecoin-project/lotus/pull/11250))
- fix: config: Update ColdStoreType comments ([filecoin-project/lotus#11274](https://github.com/filecoin-project/lotus/pull/11274))
- readme: bump up golang version (#11347) ([filecoin-project/lotus#11347](https://github.com/filecoin-project/lotus/pull/11347))
- chore: watermelon: upgrade epoch ([filecoin-project/lotus#11374](https://github.com/filecoin-project/lotus/pull/11374))
- add support for v12 check invariants and also a default case to reduce future confusion (#11371) ([filecoin-project/lotus#11371](https://github.com/filecoin-project/lotus/pull/11371))
- test: drand: switch tests to drand testnet (from devnet) (#11359) ([filecoin-project/lotus#11359](https://github.com/filecoin-project/lotus/pull/11359))
- feat: chain: light-weight patch to fix calibrationnet again by removing move_partitions from built-in actors (#11409) ([filecoin-project/lotus#11409](https://github.com/filecoin-project/lotus/pull/11409))
- chore: cli: Revert move-partitions cmd ([filecoin-project/lotus#11408](https://github.com/filecoin-project/lotus/pull/11408))
- chore: forward-port calibnet hotfix to master ([filecoin-project/lotus#11407](https://github.com/filecoin-project/lotus/pull/11407))
- fix: migration: set premigration to 90 minutes ([filecoin-project/lotus#11395](https://github.com/filecoin-project/lotus/pull/11395))
- feat: chain: light-weight patch to fix calibrationnet (#11363) ([filecoin-project/lotus#11363](https://github.com/filecoin-project/lotus/pull/11363))
- chore: merge feat/nv21 into master ([filecoin-project/lotus#11336](https://github.com/filecoin-project/lotus/pull/11336))
- docs: Link the release section in the release flow doc ([filecoin-project/lotus#11299](https://github.com/filecoin-project/lotus/pull/11299))
- fix: ci: fetch params for the storage unit tests ([filecoin-project/lotus#11441](https://github.com/filecoin-project/lotus/pull/11441))
- Update mainnet.pi ([filecoin-project/lotus#11288](https://github.com/filecoin-project/lotus/pull/11288))
- add go linter - "unused" (#11235) ([filecoin-project/lotus#11235](https://github.com/filecoin-project/lotus/pull/11235))
- Fix/texts (#11298) ([filecoin-project/lotus#11298](https://github.com/filecoin-project/lotus/pull/11298))
- fix typo in rate-limit flag description (#11316) ([filecoin-project/lotus#11316](https://github.com/filecoin-project/lotus/pull/11316))
- eth_filter flake debug ([filecoin-project/lotus#11261](https://github.com/filecoin-project/lotus/pull/11261))
- fix: sealing: typo in FinalizeReplicaUpdate ([filecoin-project/lotus#11255](https://github.com/filecoin-project/lotus/pull/11255))
- chore: slice loop replace (#11349) ([filecoin-project/lotus#11349](https://github.com/filecoin-project/lotus/pull/11349))
- backport: docker build fix for v1.25.2 ([filecoin-project/lotus#11560](https://github.com/filecoin-project/lotus/pull/11560))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Andrew Jackson (Ajax) | 161 | +24328/-12464 | 4148 |
| Łukasz Magiera | 99 | +5238/-2690 | 260 |
| Shrenuj Bansal | 27 | +3402/-1265 | 76 |
| Fridrik Asmundsson | 15 | +1148/-307 | 58 |
| Steven Allen | 15 | +674/-337 | 35 |
| Ian Norden | 1 | +625/-3 | 4 |
| Aarsh Shah | 4 | +227/-167 | 14 |
| Phi | 19 | +190/-183 | 32 |
| Aayush Rajasekaran | 3 | +291/-56 | 16 |
| Mikers | 2 | +76/-262 | 19 |
| Aayush | 14 | +111/-59 | 21 |
| Friðrik Ásmundsson | 1 | +101/-1 | 2 |
| Alejandro Criado-Pérez | 1 | +36/-36 | 27 |
| Jie Hou | 5 | +36/-10 | 5 |
| Florian RUEN | 2 | +24/-19 | 5 |
| Phi-rjan | 3 | +20/-8 | 3 |
| Icarus9913 | 1 | +11/-11 | 6 |
| Jiaying Wang | 3 | +8/-7 | 5 |
| guangwu | 1 | +3/-10 | 2 |
| Marten Seemann | 1 | +6/-6 | 2 |
| simlecode | 1 | +0/-6 | 2 |
| GlacierWalrus | 2 | +0/-5 | 2 |
| Anton Evangelatov | 1 | +2/-2 | 1 |
| Ales Dumikau | 3 | +2/-2 | 3 |
| renran | 1 | +2/-1 | 1 |
| Volker Mische | 1 | +1/-1 | 1 |
| Icarus Wu | 1 | +1/-1 | 1 |
| Hubert | 1 | +1/-1 | 1 |
| Aloxaf | 1 | +1/-1 | 1 |
| Alejandro | 1 | +1/-1 | 1 |
| lazavikmaria | 1 | +1/-0 | 1 |

# v1.25.1 / 2023-12-09

This is a **highly recommended PATCH RELEASE.**  The patch release fixes the issue were node operators trying to catch up sync were unable to sync large message blocks/epochs due to an increased number of messages on the network.

This patch release allows for up to 10k messages per block. Additionally, it introduces a limit on the amount of data that can be read at once, ensuring the system can handle worst-case scenarios.

## Improvements
- fix: exchange: allow up to 10k messages per block ([filecoin-project/lotus#11506](https://github.com/filecoin-project/lotus/pull/11506))

# v 1.25.0 / 2023-11-22

This is a highly recommended feature release of Lotus. This optional release supports the Filecoin network version 21 upgrade, codenamed Watermelon 🍉, in addition to the numerous improvements and enhancements for node operators, ETH RPC-providers and storage providers.

**The Filecoin network upgrade v21, codenamed Watermelon 🍉, is at epoch 3469380 - 2023-12-12T13:30:00Z**

The full list of [protocol improvements delivered in the network upgrade can be found here](https://github.com/filecoin-project/core-devs/blob/master/Network%20Upgrades/v21.md).

## ☢️ Upgrade Warnings ☢️

- Read through the [changelog of the mandatory v1.24.0 release](https://github.com/filecoin-project/lotus/releases/tag/v1.24.0). Especially the `Migration` and `v12 Builtin Actor Bundle` sections.
- Please remove and clone a new Lotus repo (`git clone https://github.com/filecoin-project/lotus.git`) when upgrading to this release.
- This feature release requires a minimum Go version of v1.20.7 or higher to successfully build Lotus. Go version 1.21.x is not supported yet.
- EthRPC providers, please check out the [new tracing API to Lotus RPC](https://github.com/filecoin-project/lotus/pull/11100)

## ⭐️ Highlights ⭐️

**Unsealing bugfixes and enhancements**

This feature release introduces significant improvements and bugfixes with regards to unsealing, and ensures that unsealing operates as one would expect. Consequently, unsealing of all sector types (deal sectors, snap-sectors without sector keys, and snap-sectors with sector keys) now all function seamlessly.

Some additional unsealing improvements are:
- Unsealing on workers with only sealing paths works. :tada:
- Transferring unsealed files to long-term storage upon successful unsealing. :arrow_right:
- Ensuring no residual files in sealing paths post a successful unsealing operation. :broom:

**SupraSeal C2**

Lotus-workers can now be built to leverage the SupraSeal C2 sealing optimizations in your sealing pipeline. The code optimizations are currently behind the `FFI_USE_CUDA_SUPRASEAL` feature flag. We advice users to test this feature on a test-network, before trying to use it on the mainnet. Users can test out the feature by building their lotus-workers by exporting the `FFI_USE_CUDA_SUPRASEAL=1` enviroment variable, and building from source. For questions about the SupraSeal C2 sealing optimizations, reach out in the #fil-proofs or the #dsa-sealing slack channel.

## New features
- feat: add Eip155ChainID to StateGetNetworkParams ([filecoin-project/lotus#10987](https://github.com/filecoin-project/lotus/pull/10987))
- feat: profiling: state summary and visualization ([filecoin-project/lotus#11012](https://github.com/filecoin-project/lotus/pull/11012))
- feat: snapshot: remove existing chain ([filecoin-project/lotus#11032](https://github.com/filecoin-project/lotus/pull/11032))
- feat: Add a metric to display pruning of the node's peer ([filecoin-project/lotus#11058](https://github.com/filecoin-project/lotus/pull/11058))
- feat:shed:gather partition metadata ([filecoin-project/lotus#11078](https://github.com/filecoin-project/lotus/pull/11078))
- feat: vm: allow raw "cbor" in state and use the new go-multicodec ([filecoin-project/lotus#11081](https://github.com/filecoin-project/lotus/pull/11081))
- Add new lotus-shed command for backfillling actor events ([filecoin-project/lotus#11088](https://github.com/filecoin-project/lotus/pull/11088))
- feat: Add new tracing API ([filecoin-project/lotus#11100](https://github.com/filecoin-project/lotus/pull/11100))
- feat: FVM: do not error on unsuccessful implicit messages ([filecoin-project/lotus#11120](https://github.com/filecoin-project/lotus/pull/11120))
- feat: chain node: Move consensus slasher to internal service ([filecoin-project/lotus#11126](https://github.com/filecoin-project/lotus/pull/11126))
- feat: miner: implement FRC-0051 ([filecoin-project/lotus#11157](https://github.com/filecoin-project/lotus/pull/11157))
  - feat: chainstore: FRC-0051: Remove all equivocated blocks from tipsets ([filecoin-project/lotus#11104](https://github.com/filecoin-project/lotus/pull/11104))
  - feat: miner: 2 minor refactors ([filecoin-project/lotus#11158](https://github.com/filecoin-project/lotus/pull/11158))
- feat: refactor: return randomness base to FVM without hashing ([filecoin-project/lotus#11167](https://github.com/filecoin-project/lotus/pull/11167))
- feat: Lotus Gateway: add allocation and claim related GET APIs to gateway ([filecoin-project/lotus#11183](https://github.com/filecoin-project/lotus/pull/11183))
- feat: shed: Add exec traces to `lotus-shed msg` ([filecoin-project/lotus#11188](https://github.com/filecoin-project/lotus/pull/11188))
- feat: miner: defensive check for equivocation ([filecoin-project/lotus#11328](https://github.com/filecoin-project/lotus/pull/11328))

## Improvements
- feat: daemon: improvemens to the consensus slasher ([filecoin-project/lotus#10979](https://github.com/filecoin-project/lotus/pull/10979))
- fix: Snapdeals unsealing fixes ([filecoin-project/lotus#11011](https://github.com/filecoin-project/lotus/pull/11011))
- refactor: Make all validation error actions explicit ([filecoin-project/lotus#11016](https://github.com/filecoin-project/lotus/pull/11016))
- feat: shed: command for decoding block headers ([filecoin-project/lotus#11031](https://github.com/filecoin-project/lotus/pull/11031))
- fix: stores: Tune down StorageDeclareSector` log-lvl ([filecoin-project/lotus#11045](https://github.com/filecoin-project/lotus/pull/11045))
- feat: types: apply a max length when decoding events ([filecoin-project/lotus#11054](https://github.com/filecoin-project/lotus/pull/11054))
- feat: slasher: improve UX ([filecoin-project/lotus#11060](https://github.com/filecoin-project/lotus/pull/11060))
- feat: daemon: improvemens to the consensus slasher ([filecoin-project/lotus#11063](https://github.com/filecoin-project/lotus/pull/11063))
- fix: events: Improve performance of event migration from V1 to V2 ([filecoin-project/lotus#11064](https://github.com/filecoin-project/lotus/pull/11064))
- feat:lotus-bench:AMT benchmarking ([filecoin-project/lotus#11075](https://github.com/filecoin-project/lotus/pull/11075))
- fix: DecodeRLP can panic ([filecoin-project/lotus#11079](https://github.com/filecoin-project/lotus/pull/11079))
- fix: daemon: set real beacon schedule when importing chain ([filecoin-project/lotus#11080](https://github.com/filecoin-project/lotus/pull/11080))
- fix: ethtypes: handle length overflow case ([filecoin-project/lotus#11082](https://github.com/filecoin-project/lotus/pull/11082))
- chore: stmgr: migrations: do not log noisily on cache misses ([filecoin-project/lotus#11083](https://github.com/filecoin-project/lotus/pull/11083))
- feat: daemon: import: only setup stmgr if validating chain (#11084) ([filecoin-project/lotus#11084](https://github.com/filecoin-project/lotus/pull/11084))
- fix: sealing pipeline: Fix PC1 retry loop ([filecoin-project/lotus#11087](https://github.com/filecoin-project/lotus/pull/11087))
- chore: legacy syscalls: Cleanup ComputeUnsealedSectorCID ([filecoin-project/lotus#11119](https://github.com/filecoin-project/lotus/pull/11119))
- sector import: fix evaluating randomness when importing a sector ([filecoin-project/lotus#11123](https://github.com/filecoin-project/lotus/pull/11123))
- fix: cli: Only display `warning` if behind sync ([filecoin-project/lotus#11140](https://github.com/filecoin-project/lotus/pull/11140))
- fix: worker: Support IPv6 formatted API-keys ([filecoin-project/lotus#11141](https://github.com/filecoin-project/lotus/pull/11141))
- fix: sealing: Switch to calling PreCommitSectorBatch2 ([filecoin-project/lotus#11142](https://github.com/filecoin-project/lotus/pull/11142))
- fix: downgrade harmless warning to debug ([filecoin-project/lotus#11145](https://github.com/filecoin-project/lotus/pull/11145))
- fix: sealing: Fix RetryCommitWait loop when sector cron activation fails ([filecoin-project/lotus#11046](https://github.com/filecoin-project/lotus/pull/11046))
- fix: gateway: return an error when an Eth filter is not found ([filecoin-project/lotus#11152](https://github.com/filecoin-project/lotus/pull/11152))
- fix: chainstore: do not get stuck in unhappy equivocation cases ([filecoin-project/lotus#11159](https://github.com/filecoin-project/lotus/pull/11159))
- fix: sealing: Run unsealing in the background for better ux ([filecoin-project/lotus#11177](https://github.com/filecoin-project/lotus/pull/11177))
- fix: build: Allow lotus-wallet to be built independently ([filecoin-project/lotus#11187](https://github.com/filecoin-project/lotus/pull/11187))
- fix: wallet: Make import handle SIGINT/SIGTERM ([filecoin-project/lotus#11190](https://github.com/filecoin-project/lotus/pull/11190))
- fix: markets/dagstore: remove trace goroutine for dagstore wrapper ([filecoin-project/lotus#11191](https://github.com/filecoin-project/lotus/pull/11191))
- fix: chain: Do not update message info cache until after message validation ([filecoin-project/lotus#11202](https://github.com/filecoin-project/lotus/pull/11202))
- fix: chain: cancel long operations upon ctx cancelation ([filecoin-project/lotus#11206](https://github.com/filecoin-project/lotus/pull/11206))
- fix(client): single-root error message ([filecoin-project/lotus#11214](https://github.com/filecoin-project/lotus/pull/11214))
- fix: worker: Convert `DC_[SectorSize]_[ResourceRestriction]` if set ([filecoin-project/lotus#11224](https://github.com/filecoin-project/lotus/pull/11224))
- chore: backport #11338 onto release/v1.25.0 ([filecoin-project/lotus#11350](https://github.com/filecoin-project/lotus/pull/11350))
- fix: lotus-provider: lotus-provider msg sending ([filecoin-project/lotus#11480](https://github.com/filecoin-project/lotus/pull/11480))
- fix: lotus-provider: Fix winning PoSt ([filecoin-project/lotus#11483](https://github.com/filecoin-project/lotus/pull/11483))
- chore: fix: sql Scan cannot write to an object ([filecoin-project/lotus#11487](https://github.com/filecoin-project/lotus/pull/11487))

## Dependencies
- deps: update go-libp2p to v0.28.1 ([filecoin-project/lotus#10998](https://github.com/filecoin-project/lotus/pull/10998))
- deps: update go-libp2p to v0.29.2 ([filecoin-project/lotus#11164](https://github.com/filecoin-project/lotus/pull/11164))
- deps: update go-libp2p to v0.30.0 ([filecoin-project/lotus#11189](https://github.com/filecoin-project/lotus/pull/11189))
- fix: build: use tagged releases ([filecoin-project/lotus#11194](https://github.com/filecoin-project/lotus/pull/11194))
- chore: test-vectors: update ([filecoin-project/lotus#11196](https://github.com/filecoin-project/lotus/pull/11196))
- chore: backport #11365 to release/v1.25.0 ([filecoin-project/lotus#11369](https://github.com/filecoin-project/lotus/pull/11369))
- chore: deps: update to go-state-types v0.12.8 ([filecoin-project/lotus#11339](https://github.com/filecoin-project/lotus/pull/11437))
- chore: deps: update to final actors ([filecoin-project/lotus#11330](https://github.com/filecoin-project/lotus/pull/11440))
- github.com/filecoin-project/go-amt-ipld/v4 (v4.0.0 -> v4.2.0)
- github.com/filecoin-project/test-vectors/schema (v0.0.5 -> v0.0.7)

## Others
- chore: Extract stable release and post release portion outside of RC testing in template ([filecoin-project/lotus#11000](https://github.com/filecoin-project/lotus/pull/11000))
- fix: docs: include FFI steps in lotus RELEASE_ISSUE_TEMPLATE ([filecoin-project/lotus#11047](https://github.com/filecoin-project/lotus/pull/11047))
- chore: build: update to v1.23.4-dev ([filecoin-project/lotus#11049](https://github.com/filecoin-project/lotus/pull/11049))
- fix: deflake: Use MockProofs ([filecoin-project/lotus#11059](https://github.com/filecoin-project/lotus/pull/11059))
- fix: failing test: Tweak TestWindowPostV1P1NV20 test condition ([filecoin-project/lotus#11121](https://github.com/filecoin-project/lotus/pull/11121))
- fix: CI: make test-unit-rest actually be the rest of the tests ([filecoin-project/lotus#11147](https://github.com/filecoin-project/lotus/pull/11147))
- chore: merge releases into master ([filecoin-project/lotus#11154](https://github.com/filecoin-project/lotus/pull/11154))
- tests: deflake: TestGetBlockByNumber ([filecoin-project/lotus#11155](https://github.com/filecoin-project/lotus/pull/11155))
- tests: mac seal test ([filecoin-project/lotus#11180](https://github.com/filecoin-project/lotus/pull/11180))
- tests: Take Download out of Sealer time ([filecoin-project/lotus#11182](https://github.com/filecoin-project/lotus/pull/11182))
- feat: test: Test that verified clients can directly transfer datacap, creating allocations ([filecoin-project/lotus#11169](https://github.com/filecoin-project/lotus/pull/11169))
- chore: merge feat/nv21 into master ([filecoin-project/lotus#11201](https://github.com/filecoin-project/lotus/pull/11201))
- ci: Use larger executor for cli tests ([filecoin-project/lotus#11212](https://github.com/filecoin-project/lotus/pull/11212))
- fix: dockerfile: Bump to Go 1.20.7 image ([filecoin-project/lotus#11221](https://github.com/filecoin-project/lotus/pull/11221))
- docs: Update PR template to callout remembering to update CHANGELOG ([filecoin-project/lotus#11232](https://github.com/filecoin-project/lotus/pull/11232))
- chore: release: 1.23.4rc1 prep ([filecoin-project/lotus#11248](https://github.com/filecoin-project/lotus/pull/11248))
- chore: backport #11262 (#11265) ([filecoin-project/lotus#11265](https://github.com/filecoin-project/lotus/pull/11265))
- chore: backport #11294 into `release/v1.23.4` ([filecoin-project/lotus#11295](https://github.com/filecoin-project/lotus/pull/11295))
- chore: release: V1.25 rebase ([filecoin-project/lotus#11342](https://github.com/filecoin-project/lotus/pull/11342))
- backport: tests: add SynthPorep layers to cachefiles ([filecoin-project/lotus#11344](https://github.com/filecoin-project/lotus/pull/11344))
- chore: backport #11408 to release/v1.25.0 ([filecoin-project/lotus#11414](https://github.com/filecoin-project/lotus/pull/11414))
- chore: backport calibnet lightweight patch ([filecoin-project/lotus#11422](https://github.com/filecoin-project/lotus/pull/11422))
- chore: update bootstrap nodes ([filecoin-project/lotus#11288](https://github.com/filecoin-project/lotus/pull/11288))
- chore: add bootstrap node on calibration ([filecoin-project/lotus#11175](https://github.com/filecoin-project/lotus/pull/11175))

# 1.24.0 / 2023-11-22

This is the stable release for the upcoming **MANDATORY**  Filecoin network upgrade v21, codenamed Watermelon 🍉, at **epoch 3469380 - 2023-12-12T13:30:00Z**.


The Filecoin network version 21 delivers the following FIPs:

- [FIP0052: Increase Max Sector Commitment to 3.5 years](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)
- [FIP0059: Synthetic PoRep](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0059.md)
- [FIP0071: Deterministic State Access (IPLD Reachability)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0071.md)
- [FIP0072: Improved event syscall API](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0072.md) 
- [FIP0073: Remove beneficiary from the self_destruct syscall](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0073.md)
- [FIP0075: Improvements to FVM randomness syscalls](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0075.md)


Full list of the other protocol improvements we are delivering can be found [here](https://github.com/filecoin-project/core-devs/blob/master/Network%20Upgrades/v21.md).

## ☢️ Upgrade Warnings ☢️

This feature release requires a minimum Go version of v1.20.7 or higher to successfully build Lotus. Go version 1.21.x is not supported yet.

## v12 Builtin Actor Bundles

[Builtin actor v12.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v12.0.0) is used for supporting this upgrade.
Make sure that your lotus actor bundle matches the v12 actors manifest by running the following cli after upgrading:

```
./lotus state actor-cids --network-version 21
Network Version: 21
Actor Version: 12
Manifest CID: bafy2bzaceapkgfggvxyllnmuogtwasmsv5qi2qzhc2aybockd6kag2g5lzaio

Actor             CID  
datacap           bafk2bzacebpiwb2ml4qbnnaayxumtk43ryhc63exdgnhivy3hwgmzemawsmpq
paymentchannel    bafk2bzacectv4cm47bnhga5febf3lo3fq47g72kmmp2xd5s6tcxz7hiqdywa4
storagemarket     bafk2bzacedylkg5am446lcuih4voyzdn4yjeqfsxfzh5b6mcuhx4mok5ph5c4
storagepower      bafk2bzacecsij5tpfzjpfuckxvccv2p3bdqjklkrfyyoei6lx5dyj5j4fvjm6
cron              bafk2bzacechxjkfe2cehx4s7skj3wzfpzf7zolds64khrrrs66bhazsemktls
eam               bafk2bzaceb3elj4hfbbjp7g5bptc7su7mptszl4nlqfedilxvstjo5ungm6oe
ethaccount        bafk2bzaceb4gkau2vgsijcxpfuq33bd7w3efr2rrhxrwiacjmns2ntdiamswq
reward            bafk2bzacealqnxn5lwzwexd6reav4dppypquklx2ujlnvaxiqk2tzstyvkp5u
verifiedregistry  bafk2bzacedudgflxc75c77c6zkmfyq4u2xuk7k6xw6dfdccarjrvxx453b77q
evm               bafk2bzacecmnyfiwb52tkbwmm2dsd7ysi3nvuxl3lmspy7pl26wxj4zj7w4wi
placeholder       bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro
storageminer      bafk2bzacedo75pabe4i2l3hvhtsjmijrcytd2y76xwe573uku25fi7sugqld6
system            bafk2bzacebfqrja2hip7esf4eafxjmu6xcogoqu5xxtgdg7xa5szgvvdguchu
account           bafk2bzaceboftg75mdiba7xbo2i3uvgtca4brhnr3u5ptihonixgpnrvhpxoa
init              bafk2bzacebllyegx5r6lggf6ymyetbp7amacwpuxakhtjvjtvoy2bfkzk3vms
```

## Migration

We are expecting a heavier than normal state migration for this upgrade due to the amount of state changes introduced for miner sector info. (This is a similar migration as the Shark upgrade, however, we have introduced a couple of migration performance optimizations since then for a smoother upgrade experience.)

All node operators, including storage providers, should be aware that ONE pre-migration is being scheduled 180 epochs before the upgrade, around 2023-12-12T12:00:00Z. It will take around 20-30 minutes for the pre-migration and less than 30 seconds for the final migration, depending on the amount of historical state in the node blockstore and the hardware specs the node is running on. During this time, expect slower block validation times, increased CPU and memory usage, and longer delays for API queries (during our testing, it topped out about 205 RAM(htop) on a 1TiB box).

We recommend node operators (who haven't enabled splitstore `discard` mode) that do not care about historical chain states, to prune the chain blockstore by syncing from a snapshot 1-2 days before the upgrade.

Note to full archival node operators: you may expect it takes some time  for the node to complete the final migration, during this period your node will fall out of sync and your chain service may have some disruption. However, you can expect the node to catch up soon after the migration completes. You can test out the migration by running the following on your node in offline mode:

1. `lotus chain head | head -n1`
2. Stop Lotus daemon
3. `./lotus-shed migrate-state --repo=[path-to-your-lotus-repo] 21 [output-of-step-1]`

You can check out the [tutorial for benchmarking the network migration here.](https://lotus.filecoin.io/kb/test-migration/)


## BREAKING CHANGE

There is a new protocol limit on how many partition could be submited in one PoSt - if you have any customized tooling for batching PoSts, please update accordingly.
- feat: limit PoSted partitions to 3 ([filecoin-project/lotus#11327](https://github.com/filecoin-project/lotus/pull/11327))

## New features
- Implement and support [FIP0052: Increase Max Sector Commitment to 3.5 years](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)
   - fix: docs: Update SectorLifetime to be in line with FIP-0052 ([filecoin-project/lotus#11314](https://github.com/filecoin-project/lotus/pull/11314))
- Implement and support [FIP0059: Synthetic PoRep](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0059.md) - Check out the [Lotus documentation for Synthetic PoRep](https://lotus.filecoin.io/storage-providers/advanced-configurations/sealing/#synthetic-porep).
   - feat: implement Synthetic PoRep ([filecoin-project/lotus#11258](https://github.com/filecoin-project/lotus/pull/11258))
   - chore: config: Update todo in UseSyntheticPoRep ([filecoin-project/lotus#11297](https://github.com/filecoin-project/lotus/pull/11297))
   
## Improvements
- Backport: feat: sealing: Switch to calling PreCommitSectorBatch2 ([filecoin-project/lotus#11215](https://github.com/filecoin-project/lotus/pull/11215))
- updated the boostrap nodes

## Dependencies
- github.com/filecoin-project/go-amt-ipld/v4 (v4.0.0 -> v4.2.0)
- chore: deps: update FFI, FVM, and actors ([filecoin-project/lotus#11310](https://github.com/filecoin-project/lotus/pull/11310))
- chore: deps: update to final actors ([filecoin-project/lotus#11330](https://github.com/filecoin-project/lotus/pull/11440))
- chore: deps: update to go-state-types v0.12.8 ([filecoin-project/lotus#11339](https://github.com/filecoin-project/lotus/pull/11437))
- chore: deps: update libp2p to v0.30.0 #11434


## Snapshots

The [Forest team](https://filecoinproject.slack.com/archives/C029LPZ5N73) at Chainsafe has launched a brand new lightweight snapshot service that is backed up by forest nodes! This is a great alternative service along with the fil-infra one, and it is compatible with lotus! We recommend lotus users to check it out [here](https://docs.filecoin.io/networks/mainnet#resources)!



# v1.23.3 / 2023-08-01

This feature release of Lotus includes numerous improvements and enhancements for node operators, ETH RPC-providers and storage providers.

This feature release requires a **minimum Go version of v1.19.12 or higher to successfully build Lotus**. Go version 1.20 is also supported, but 1.21 is NOT.

## Highlights

- [Lotus now includes a Slasher tool](https://github.com/filecoin-project/lotus/pull/10928) to monitor the network for Consensus Faults, and report them as appropriate
  - The Slasher investigates all incoming blocks, and assesses whether they trigger any of the three Consensus Faults defined in the Filecoin protocol
  - If any faults are detected, the Slasher sends a `ReportConsensusFault` message to the faulty miner
  - For more information on the Slasher, including how to run it, please find the documentation [here](https://lotus.filecoin.io/lotus/manage/slasher-and-disputer/)
- The Ethereum-like RPC exposed by Lotus is now compatible with EIP-1898: https://github.com/filecoin-project/lotus/pull/10815
- The lotus-miner PieceReader now supports parallel reads: https://github.com/filecoin-project/lotus/pull/10913
- Added new environment variable `LOTUS_EXEC_TRACE_CACHE_SIZE` to configure execution trace cache size ([filecoin-project/lotus#10585](https://github.com/filecoin-project/lotus/pull/10585))
  - If unset, we default to caching 16 most recent execution traces. Storage Providers may want to set this to 0, while exchanges may want to crank it up.

## New features
  - feat: miner cli: sectors list upgrade-bounds tool ([filecoin-project/lotus#10923](https://github.com/filecoin-project/lotus/pull/10923))
  - Add new RPC stress testing tool (lotus-bench rpc) with rich reporting ([filecoin-project/lotus#10761](https://github.com/filecoin-project/lotus/pull/10761))
  - feat: alert: Add FVM_CONCURRENCY alert ([filecoin-project/lotus#10933](https://github.com/filecoin-project/lotus/pull/10933))
  - feat: Add eth_syncing RPC method ([filecoin-project/lotus#10719](https://github.com/filecoin-project/lotus/pull/10719))
  - feat: sealing: flag to run data_cid untied from addpiece ([filecoin-project/lotus#10797](https://github.com/filecoin-project/lotus/pull/10797))
  - feat: Lotus Gateway: add MpoolPending, ChainGetBlock and MinerGetBaseInfo ([filecoin-project/lotus#10929](https://github.com/filecoin-project/lotus/pull/10929))

## Improvements && Bug Fixes
  - chore: update ffi & fvm ([filecoin-project/lotus#11040](https://github.com/filecoin-project/lotus/pull/11040))
  - feat: Make sure we don't store duplidate actor events caused to reorgs in events.db ([filecoin-project/lotus#11015](https://github.com/filecoin-project/lotus/pull/11015))
  - sealing: Use only non-assigned deals when selecting snap sectors ([filecoin-project/lotus#11002](https://github.com/filecoin-project/lotus/pull/11002))
  - chore: not display privatekey ([filecoin-project/lotus#11006](https://github.com/filecoin-project/lotus/pull/11006))
  - chore: shed: update actor version ([filecoin-project/lotus#11020](https://github.com/filecoin-project/lotus/pull/11020))
  - chore: migrate to boxo ([filecoin-project/lotus#10921](https://github.com/filecoin-project/lotus/pull/10921))
  - feat: deflake TestDealsWithFinalizeEarly ([filecoin-project/lotus#10978](https://github.com/filecoin-project/lotus/pull/10978))
  - fix: pubsub: do not treat ErrExistingNonce as Reject ([filecoin-project/lotus#10973](https://github.com/filecoin-project/lotus/pull/10973))
  - feat: deflake TestDMLevelPartialRetrieval (#10972) ([filecoin-project/lotus#10972](https://github.com/filecoin-project/lotus/pull/10972))
  - fix: eth: ensure that the event topics are non-nil ([filecoin-project/lotus#10971](https://github.com/filecoin-project/lotus/pull/10971))
  - Add comment stating msgIndex is an experimental feature ([filecoin-project/lotus#10968](https://github.com/filecoin-project/lotus/pull/10968))
  - feat: cli(compute-state) default to the tipset at the given epoch ([filecoin-project/lotus#10965](https://github.com/filecoin-project/lotus/pull/10965))
  - Upgrade urfave dependency which now supports DisableSliceFlagSeparato… ([filecoin-project/lotus#10950](https://github.com/filecoin-project/lotus/pull/10950))
  - Add new lotus-shed command for computing eth hash for a given message cid (#10961) ([filecoin-project/lotus#10961](https://github.com/filecoin-project/lotus/pull/10961))
  - Prefill GetTipsetByHeight skiplist cache on lotus startup ([filecoin-project/lotus#10955](https://github.com/filecoin-project/lotus/pull/10955))
  - Add lotus-shed command for backfilling txhash.db ([filecoin-project/lotus#10932](https://github.com/filecoin-project/lotus/pull/10932))
  - chore: deps: update to go-libp2p 0.27.5 ([filecoin-project/lotus#10948](https://github.com/filecoin-project/lotus/pull/10948))
  - Small improvement to make gen output ([filecoin-project/lotus#10951](https://github.com/filecoin-project/lotus/pull/10951))
  - fix: improve perf of msgindex backfill ([filecoin-project/lotus#10941](https://github.com/filecoin-project/lotus/pull/10941))
  - deps: update libp2p ([filecoin-project/lotus#10936](https://github.com/filecoin-project/lotus/pull/10936))
  - sealing: Improve upgrade sector selection ([filecoin-project/lotus#10915](https://github.com/filecoin-project/lotus/pull/10915))
  - Add timing test for mpool select with a large mpool dump ([filecoin-project/lotus#10650](https://github.com/filecoin-project/lotus/pull/10650))
  - feat: slashfilter: drop outdated near-upgrade check ([filecoin-project/lotus#10925](https://github.com/filecoin-project/lotus/pull/10925))
  - opt: MinerInfo adds the PendingOwnerAddress field ([filecoin-project/lotus#10927](https://github.com/filecoin-project/lotus/pull/10927))
  - feat: itest: force PoSt more aggressively around deadline closure ([filecoin-project/lotus#10926](https://github.com/filecoin-project/lotus/pull/10926))
  - test: messagepool: gas rewards are negative if GasFeeCap too low ([filecoin-project/lotus#10649](https://github.com/filecoin-project/lotus/pull/10649))
  - fix: types: error out on decoding BlockMsg with extraneous data ([filecoin-project/lotus#10863](https://github.com/filecoin-project/lotus/pull/10863))
  - update interop upgrade schedule  ([filecoin-project/lotus#10879](https://github.com/filecoin-project/lotus/pull/10879))
  - itests: Test PoSt V1_1 on workers ([filecoin-project/lotus#10732](https://github.com/filecoin-project/lotus/pull/10732))
  - Update gas_balancing.md ([filecoin-project/lotus#10924](https://github.com/filecoin-project/lotus/pull/10924))
  - feat: cli: Make compact partitions cmd better ([filecoin-project/lotus#9070](https://github.com/filecoin-project/lotus/pull/9070))
  - fix: include extra messages in ComputeState InvocResult output ([filecoin-project/lotus#10628](https://github.com/filecoin-project/lotus/pull/10628))
  - feat: pubsub: treat ErrGasFeeCapTooLow as ignore, not reject ([filecoin-project/lotus#10652](https://github.com/filecoin-project/lotus/pull/10652))
  - feat: run lotus-shed commands in context that is cancelled on sigterm ([filecoin-project/lotus#10877](https://github.com/filecoin-project/lotus/pull/10877))
  - fix:lotus-fountain:set default data-cap same as MinVerifiedDealSize ([filecoin-project/lotus#10920](https://github.com/filecoin-project/lotus/pull/10920))
  - pass the right g-recaptcha data
  - fix: not call RUnlock ([filecoin-project/lotus#10912](https://github.com/filecoin-project/lotus/pull/10912))
  - opt: cli: If present, print Events Root ([filecoin-project/lotus#10893](https://github.com/filecoin-project/lotus/pull/10893))
  - Calibration faucet UI improvements ([filecoin-project/lotus#10905](https://github.com/filecoin-project/lotus/pull/10905))
  - chore: chain: replace storetheindex with go-libipni ([filecoin-project/lotus#10841](https://github.com/filecoin-project/lotus/pull/10841))
  - Add alerts to `Lotus info` cmd ([filecoin-project/lotus#10894](https://github.com/filecoin-project/lotus/pull/10894))
  - fix: cli: make redeclare cmd work properly ([filecoin-project/lotus#10860](https://github.com/filecoin-project/lotus/pull/10860))
  - fix: shed remove datacap not working with ledger ([filecoin-project/lotus#10880](https://github.com/filecoin-project/lotus/pull/10880))
  - Check if epoch is negative in GetTipsetByHeight ([filecoin-project/lotus#10878](https://github.com/filecoin-project/lotus/pull/10878))
  - chore: update go-fil-markets ([filecoin-project/lotus#10867](https://github.com/filecoin-project/lotus/pull/10867))
  - feat: alerts: Add lotus-miner legacy-markets alert ([filecoin-project/lotus#10868](https://github.com/filecoin-project/lotus/pull/10868))
  - feat:fountain:add grant-datacap support ([filecoin-project/lotus#10856](https://github.com/filecoin-project/lotus/pull/10856))
  - feat: itests: add logs to blockminer.go failure case ([filecoin-project/lotus#10861](https://github.com/filecoin-project/lotus/pull/10861))
  - feat: eth: Add support for blockHash param in eth_getLogs ([filecoin-project/lotus#10782](https://github.com/filecoin-project/lotus/pull/10782))
  - lotus-fountain: make compatible with 0x addresses #10560 ([filecoin-project/lotus#10784](https://github.com/filecoin-project/lotus/pull/10784))
  - feat: deflake sector_import_simple ([filecoin-project/lotus#10858](https://github.com/filecoin-project/lotus/pull/10858))
  - fix: splitstore: remove deadlock around waiting for sync ([filecoin-project/lotus#10857](https://github.com/filecoin-project/lotus/pull/10857))
  - fix: sched: Address GET_32G_MAX_CONCURRENT regression (#10850) ([filecoin-project/lotus#10850](https://github.com/filecoin-project/lotus/pull/10850))
  - feat: fix deadlock in splitstore-mpool interaction ([filecoin-project/lotus#10840](https://github.com/filecoin-project/lotus/pull/10840))
  - chore: update go-libp2p to v0.27.3 ([filecoin-project/lotus#10671](https://github.com/filecoin-project/lotus/pull/10671))
  - libp2p: add QUIC and WebTransport to default listen addresses ([filecoin-project/lotus#10848](https://github.com/filecoin-project/lotus/pull/10848))
  - fix: ci: Debugging m1 build ([filecoin-project/lotus#10749](https://github.com/filecoin-project/lotus/pull/10749))
  - Validate that FromBlock/ToBlock epoch is indeed a hex value (#10780) ([filecoin-project/lotus#10780](https://github.com/filecoin-project/lotus/pull/10780))
  - fix: remove invalid field UpgradePriceListOopsHeight ([filecoin-project/lotus#10772](https://github.com/filecoin-project/lotus/pull/10772))
  - feat: deflake eth_balance_test ([filecoin-project/lotus#10847](https://github.com/filecoin-project/lotus/pull/10847))
  - fix: tests: Use mutex-wrapped datastore in storage tests ([filecoin-project/lotus#10846](https://github.com/filecoin-project/lotus/pull/10846))
  - Make lotus-fountain UI slightly friendlier ([filecoin-project/lotus#10785](https://github.com/filecoin-project/lotus/pull/10785))
  - Make (un)subscribe and filter RPC methods require only read perm ([filecoin-project/lotus#10825](https://github.com/filecoin-project/lotus/pull/10825))
  - deps: Update go-jsonrpc to v0.3.1 ([filecoin-project/lotus#10845](https://github.com/filecoin-project/lotus/pull/10845))
  - feat: deflake paych_api_test ([filecoin-project/lotus#10843](https://github.com/filecoin-project/lotus/pull/10843))
  - fix: Eth RPC: do not occlude block param errors. ([filecoin-project/lotus#10534](https://github.com/filecoin-project/lotus/pull/10534))
  - feat: cli: More ux-friendly batching cmds ([filecoin-project/lotus#10837](https://github.com/filecoin-project/lotus/pull/10837))
  - fix: cli: Hide legacy markets cmds ([filecoin-project/lotus#10842](https://github.com/filecoin-project/lotus/pull/10842))
  - feat: chainstore: exit early in MaybeTakeHeavierTipset ([filecoin-project/lotus#10839](https://github.com/filecoin-project/lotus/pull/10839))
  - fix: itest: fix eth deploy test flake ([filecoin-project/lotus#10829](https://github.com/filecoin-project/lotus/pull/10829))
  - style: mempool: chain errors using xerrors.Errorf ([filecoin-project/lotus#10836](https://github.com/filecoin-project/lotus/pull/10836))
  - feat: deflake msgindex_test.go ([filecoin-project/lotus#10826](https://github.com/filecoin-project/lotus/pull/10826))
  - feat: deflake TestEthFeeHistory ([filecoin-project/lotus#10816](https://github.com/filecoin-project/lotus/pull/10816))
  - feat: make RunClientTest louder when deals fail ([filecoin-project/lotus#10817](https://github.com/filecoin-project/lotus/pull/10817))
  - fix: cli: Change arg wording in change-beneficiary cmd ([filecoin-project/lotus#10823](https://github.com/filecoin-project/lotus/pull/10823))
  - refactor: streamline error handling in CheckPendingMessages (#10818) ([filecoin-project/lotus#10818](https://github.com/filecoin-project/lotus/pull/10818))
  - feat: Add tmp indices to events table while performing migration to V2
  - fix: sync: iterate over returned messages directly #11373
  - fix: api: compute the effective gas cost with the correct base-fee #11357
  - fix: check invariants: v12 check #11371
  - fix: api: compute gasUsedRatio based on max gas in the tipset #11354

# v1.23.2 / 2023-06-28

This is a patch release on top of 1.23.1 containing the fix for https://github.com/filecoin-project/lotus/issues/10906
This fixes the syncing issue seen by many node operators/SPs, usually when performing actions which would result in msgs staying in their mpool for longer periods of time (ex. PSD) resulting in these msgs being republished multiple times and possibly lowering your peer scores. Please refer to the above issue for more details.
We'd recommend everyone to accept this fix to better overall network health

## Improvements
- fix: pubsub: do not treat ErrExistingNonce as Reject


# v1.23.1 / 2023-06-20

This is an optional feature release of Lotus. This feature release includes numerous improvements and enhancements for node operators, ETH RPC-providers and storage providers.

**☢️ Upgrade Warnings ☢️**

If you are upgrading to this release candidate from Lotus v1.22.1, please make sure to read the upgrade warnings section in the [v1.23.0 release first.](https://github.com/filecoin-project/lotus/releases/tag/v1.23.0)

- *Storage providers:* The Lotus-Miner legacy-markets has been disbled by default in this feature release and will be removed in the near term future. Users are adviced to migrate to [Boost](https://boost.filecoin.io) or other SP markets systems.

## Highlights

**🛣 Execution Lanes 🛣**
This feature release introduces VM Execution Lanes! Execution lanes efficiently divide the workload between system processes  (chainsync) and RPC requests. This way syncing the chain will not be at the mercy of responding to users' requests and RPC providers nodes should have less problems catching up.

To take advantage of VM Execution Lanes, you need to set up two environment variables:
- `LOTUS_FVM_CONCURRENCY` - read more about how this value should be set to [here](https://lotus.filecoin.io/lotus/configure/ethereum-rpc/#environment-variables)
- `LOTUS_FVM_CONCURRENCY_RESERVED = 4` 

**🧱 Aggregation / Batching fixes 🔨**

Numerous aggregation and batching fixes has been included in the feature release. Large `ProveCommitAggregate` and `PreCommitBatching` messages that exceeds the block limit will now automatically be split into smaller messages when sent to the chain.

Additionally we have added a new feature that staggers the amount of ProveCommit messages sent simulatanously to the chain if a storage provider has been aggregating many sectors in ProveCommitAggregate message, but at the time of publishing the BaseFee is below the aggregation threshold. This stagger feature prevents issues where some of the ProveCommit messages fail with the SysErrorOutOfGas message. You can tweak how many messages will be staggered per epoch by changing `MaxSectorProveCommitsSubmittedPerEpoch` in the [sealing section of the config.toml file.](https://lotus.filecoin.io/storage-providers/advanced-configurations/sealing/#sealing-section)

*NB:* While these fixes are great for the reliability of aggregation and batching on the Lotus side, it has been uncovered that aggregating ProveCommit messages for sectors containing verified deals are currently more expensive then single messages due to an issue on the actors side. We therefore do not reccomend our users to aggregate ProveCommit messages when doing verified deals until that issue has been resolved. You can follow the discussion on resolving the issue on the [actors side here.](https://github.com/filecoin-project/FIPs/discussions/689)

**Unsealing CLI/API**

This feature release adds a dedicated `lotus-miner sectors unseal` command and API, allowing you to unseal specific sealed sectors easily.

## New features
- feat: VM Execution Lanes ([filecoin-project/lotus#10551](https://github.com/filecoin-project/lotus/pull/10551))
   - Adds VM exections lanes, efficiently dividing the workload between system processes and RPC-requests.
- Add API and CLI to unseal sector (#10626) ([filecoin-project/lotus#10626](https://github.com/filecoin-project/lotus/pull/10626))
   - Adds `lotus-miner sectors unseal` cmd, and a API-method to unseal a sector.
- feat: sealing: Split PCA/PCB batches if gas used exceeds block limit ([filecoin-project/lotus#10647](https://github.com/filecoin-project/lotus/pull/10647))
   - Splits ProveCommitAggregate and PreCommitBatch messages into multiple messages if the message exceeds the block limit.
- Add feature to stagger sector prove commit submission (#10543) ([filecoin-project/lotus#10543](https://github.com/filecoin-project/lotus/pull/10543))
   - Staggers the amount of ProveCommit messages sent simultanously if a storage provider has been aggregating many message, but at the moment of publishing the BaseFee is below the threshold for aggregation to prevent unwanted SysErrorOutOfGas issues.
- Set default for MaxSectorProveCommitsSubmittedPerEpoch ([filecoin-project/lotus#10728](https://github.com/filecoin-project/lotus/pull/10728))
   - Sets the default amount of ProveCommits submitted per epoch to 20.
- feat: worker: Ensure tempdir exists (#10433) ([filecoin-project/lotus#10433](https://github.com/filecoin-project/lotus/pull/10433))
   - Ensures that a temporary directory exists on start of a lotus-worker with a custom TMPDIR set.
- feat: sync: harden chain sync (#10756) ([filecoin-project/lotus#10756](https://github.com/filecoin-project/lotus/pull/10756))
- feat: populate the index on snapshot import ([filecoin-project/lotus#10556](https://github.com/filecoin-project/lotus/pull/10556))
- feat:chain: Message Index (**HIGHLY EXPERIMENTAL**) ([filecoin-project/lotus#10452](https://github.com/filecoin-project/lotus/pull/10452))
   - MVP of a message index that allows us to accelrate StateSearchMessage and related functionality, and eventually accelerate critical chain calls (follow up).
- feat: Add small cache to execution traces ([filecoin-project/lotus#10517](https://github.com/filecoin-project/lotus/pull/10517))
- feat: shed: incoming block-sub chainwatch tool ([filecoin-project/lotus#10513](https://github.com/filecoin-project/lotus/pull/10513))

## Improvements
- feat: daemon: Auto-resume interrupted snapshot imports ([filecoin-project/lotus#10636](https://github.com/filecoin-project/lotus/pull/10636))
   - Auto-resumes interrupted snapshot imports when using an URL.
- fix: storage: Remove temp fetching files after failed fetch ([filecoin-project/lotus#10661](https://github.com/filecoin-project/lotus/pull/10661))
   - Clean up partially fetched failed after a failed fetch on a lotus-worker.
- feat: chainstore: batch writes of tipsets ([filecoin-project/lotus#10800](https://github.com/filecoin-project/lotus/pull/10800))
   - Reduces the time to persist all headers from 4-5 minutes, to < 15 seconds. 
- Check if epoch is negative in GetTipsetByHeight
- fix: sched: Address GET_32G_MAX_CONCURRENT regression
- fix: cli: Hide legacy markets cmds
   - Hides the lotus-miner legacy markets commands from the lotus-miner CLI.
- fix: ci: Debugging m1 build
- Disable lotus markets by default (#10809) ([filecoin-project/lotus#10809](https://github.com/filecoin-project/lotus/pull/10809))
   - Disables lotus-miner legacy markets [EOL] by default.
- perf: mempool: lower priority optimizations (#10693) ([filecoin-project/lotus#10693](https://github.com/filecoin-project/lotus/pull/10693))
- perf: message pool: change locks to RWMutexes for performance ([filecoin-project/lotus#10561](https://github.com/filecoin-project/lotus/pull/10561))
- perf: eth: gas estimate set applyTsMessages false (#10546) ([filecoin-project/lotus#10546](https://github.com/filecoin-project/lotus/pull/10546))
- Change args check ([filecoin-project/lotus#10812](https://github.com/filecoin-project/lotus/pull/10812))
- fix: sealing: Make lotus-worker report GPU usage to miner during ReplicaUpdate task (#10806) ([filecoin-project/lotus#10806](https://github.com/filecoin-project/lotus/pull/10806))
- fix:splitstore:Don't block when potentially holding txnLk as writer ([filecoin-project/lotus#10811](https://github.com/filecoin-project/lotus/pull/10811))
- fix: prover: Propagate skipped sectors in local PoSt
- fix: unseal: check if sealed/update sector exists ([filecoin-project/lotus#10639](https://github.com/filecoin-project/lotus/pull/10639))
- fix: sealing pipeline: Allow nil message in TerminateWait ([filecoin-project/lotus#10696](https://github.com/filecoin-project/lotus/pull/10696))
- fix: cli: Check if the sectorID exists before removing ([filecoin-project/lotus#10611](https://github.com/filecoin-project/lotus/pull/10611))
- feat:splitstore:limit moving gc threads ([filecoin-project/lotus#10621](https://github.com/filecoin-project/lotus/pull/10621))
- fix: cli: Make `net connect` to miner address work ([filecoin-project/lotus#10599](https://github.com/filecoin-project/lotus/pull/10599))
- fix: log: Stop logging `file does not exists` ([filecoin-project/lotus#10588](https://github.com/filecoin-project/lotus/pull/10588))
- Update config default value (#10605) ([filecoin-project/lotus#10605](https://github.com/filecoin-project/lotus/pull/10605))
- fix: cap the message gas limit at the block gas limit (#10637) ([filecoin-project/lotus#10637](https://github.com/filecoin-project/lotus/pull/10637))
- fix: miner: correctly count sector extensions ([filecoin-project/lotus#10544](https://github.com/filecoin-project/lotus/pull/10544))
- fix:mpool: prune excess messages before selection ([filecoin-project/lotus#10648](https://github.com/filecoin-project/lotus/pull/10648))
- fix: proving: Initialize slice with with same length as partition ([filecoin-project/lotus#10569](https://github.com/filecoin-project/lotus/pull/10569))
- perf: Address performance of EthGetTransactionCount  ([filecoin-project/lotus#10700](https://github.com/filecoin-project/lotus/pull/10700))
- fix: sync: reduce log from error to info ([filecoin-project/lotus#10759](https://github.com/filecoin-project/lotus/pull/10759))
- fix: state: lotus-miner info should show deals info without admin permission ([filecoin-project/lotus#10323](https://github.com/filecoin-project/lotus/pull/10323))
- fix: tvx: make extract-multiple support the FVM ([filecoin-project/lotus#10714](https://github.com/filecoin-project/lotus/pull/10714))
- feat: badger: add a has check before writing to reduce duplicates ([filecoin-project/lotus#10680](https://github.com/filecoin-project/lotus/pull/10680))
- fix: chain: record heaviest tipset before notifying (#10694) ([filecoin-project/lotus#10694](https://github.com/filecoin-project/lotus/pull/10694))
- fix: Eth JSON-RPC api: handle messages with gasFeeCap less than baseFee (#10614) ([filecoin-project/lotus#10614](https://github.com/filecoin-project/lotus/pull/10614))
- feat: chainstore: optimize BlockMsgsForTipset ([filecoin-project/lotus#10552](https://github.com/filecoin-project/lotus/pull/10552))
- refactor: stop using deprecated io/ioutil ([filecoin-project/lotus#10596](https://github.com/filecoin-project/lotus/pull/10596))
- feat: shed: refactor market cron-state command ([filecoin-project/lotus#10746](https://github.com/filecoin-project/lotus/pull/10746))
- fix: events: don't set GC confidence to 1 ([filecoin-project/lotus#10713](https://github.com/filecoin-project/lotus/pull/10713))
- feat: sync: validate (early) that blocks fall within range (#10691) ([filecoin-project/lotus#10691](https://github.com/filecoin-project/lotus/pull/10691))
- chainstore: Fix raw blocks getting scanned for links during snapshots (#10684) ([filecoin-project/lotus#10684](https://github.com/filecoin-project/lotus/pull/10684))
- fix: remove pointless panic ([filecoin-project/lotus#10690](https://github.com/filecoin-project/lotus/pull/10690))
- fix: check for nil bcastDict (#10646) ([filecoin-project/lotus#10646](https://github.com/filecoin-project/lotus/pull/10646))
- fix: make state compute --html work with unknown methods ([filecoin-project/lotus#10619](https://github.com/filecoin-project/lotus/pull/10619))
- shed: get balances of evm accounts  ([filecoin-project/lotus#10489](https://github.com/filecoin-project/lotus/pull/10489))
- feat: Use MessageIndex in WaitForMessage ([filecoin-project/lotus#10587](https://github.com/filecoin-project/lotus/pull/10587))
- fix: searchForIndexedMsg always returns an error ([filecoin-project/lotus#10586](https://github.com/filecoin-project/lotus/pull/10586))
- Fix: export-range: Ignore ipld Blocks not found in Receipts. ([filecoin-project/lotus#10535](https://github.com/filecoin-project/lotus/pull/10535))
- feat: stmgr: speed up calculation of genesis circ supply ([filecoin-project/lotus#10553](https://github.com/filecoin-project/lotus/pull/10553))
- fix: gas estimation: don't special case paych collects ([filecoin-project/lotus#10549](https://github.com/filecoin-project/lotus/pull/10549))
- fix: tracer: emit raw peer ids for compatibility with libp2p tracer ([filecoin-project/lotus#10271](https://github.com/filecoin-project/lotus/pull/10271))
- Merge branch 'feat/new-gw-methods'

## Dependencies
- chore: deps: update to go-libp2p 0.27.5
- devs: update libp2p #10937
- chore: deps: update to FVM 3.3.1 ([filecoin-project/lotus#10786](https://github.com/filecoin-project/lotus/pull/10786))
- chore: boxo: migrate from go-libipfs to boxo ([filecoin-project/lotus#10562](https://github.com/filecoin-project/lotus/pull/10562))
- chore: deps: update to go-state-types v0.11.0-alpha-3 ([filecoin-project/lotus#10606](https://github.com/filecoin-project/lotus/pull/10606))
- chore: bump go-libipfs ([filecoin-project/lotus#10531](https://github.com/filecoin-project/lotus/pull/10531))

## Others
- feat:networking:  (Synchronous) Consistent Broadcast for Filecoin EC ([filecoin-project/lotus#9858](https://github.com/filecoin-project/lotus/pull/9858))
- Revert #9858 (consistent broadcast changes) ([filecoin-project/lotus#10777](https://github.com/filecoin-project/lotus/pull/10777))
- Update build version for release/v1.23.1
- chore: drop flaky TestBatchDealInput subcase ([filecoin-project/lotus#10810](https://github.com/filecoin-project/lotus/pull/10810))
- chore: changelog clean up ([filecoin-project/lotus#10744](https://github.com/filecoin-project/lotus/pull/10744))
- chore: refactor: drop unused IsTicketWinner (#10801) ([filecoin-project/lotus#10801](https://github.com/filecoin-project/lotus/pull/10801))
- chore: build: bump matser version to v1.23.1-dev ([filecoin-project/lotus#10709](https://github.com/filecoin-project/lotus/pull/10709))
- fix: deflake: use 2 miners for flaky tests ([filecoin-project/lotus#10764](https://github.com/filecoin-project/lotus/pull/10764))
- test: eth: deflake multiblock lookup test (#10769) ([filecoin-project/lotus#10769](https://github.com/filecoin-project/lotus/pull/10769))
- shed: migrations: add reminder about continuity testing tool ([filecoin-project/lotus#10762](https://github.com/filecoin-project/lotus/pull/10762))
- chore: merge releases into master ([filecoin-project/lotus#10742](https://github.com/filecoin-project/lotus/pull/10742))
- test: events: fix race when recording tipsets (#10665) ([filecoin-project/lotus#10665](https://github.com/filecoin-project/lotus/pull/10665))
- fix: build: add CBDeliveryDelay to testground ([filecoin-project/lotus#10613](https://github.com/filecoin-project/lotus/pull/10613))
- fix: build: Fixed incorrect words that could not be compiled ([filecoin-project/lotus#10610](https://github.com/filecoin-project/lotus/pull/10610))
- build: docker: Update GO-version ([filecoin-project/lotus#10581](https://github.com/filecoin-project/lotus/pull/10581))
- fix: itests: Don't call t.Error in MineBlocks goroutine ([filecoin-project/lotus#10572](https://github.com/filecoin-project/lotus/pull/10572))
- docs: api: clarify MpoolClear params ([filecoin-project/lotus#10550](https://github.com/filecoin-project/lotus/pull/10550))

Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| vyzo | 70 | +1990/-429 | 135 |
| Alfonso de la Rocha | 25 | +814/-299 | 56 |
| Steven Allen | 14 | +125/-539 | 28 |
| Shrenuj Bansal | 13 | +482/-138 | 52 |
| Aayush | 17 | +317/-301 | 90 |
| Łukasz Magiera | 13 | +564/-26 | 16 |
| Jennifer Wang | 7 | +401/-140 | 10 |
| Fridrik Asmundsson | 14 | +315/-84 | 20 |
| Jorropo | 2 | +139/-137 | 74 |
| Mikers | 6 | +114/-43 | 14 |
| Hector Sanjuan | 5 | +92/-44 | 5 |
| Ales Dumikau | 1 | +117/-0 | 10 |
| Mike Seiler | 4 | +51/-51 | 6 |
| zenground0 | 6 | +33/-25 | 8 |
| Phi | 8 | +32/-10 | 10 |
| Aayush Rajasekaran | 1 | +1/-32 | 2 |
| Ian Davis | 2 | +7/-10 | 3 |
| Marcel Telka | 1 | +5/-7 | 1 |
| ychiao | 1 | +8/-3 | 2 |
| jennijuju | 1 | +4/-4 | 8 |
| adlrocha | 2 | +2/-2 | 2 |
| Jiaying Wang | 1 | +0/-4 | 1 |
| ZenGround0 | 1 | +2/-1 | 2 |
| Zeng Li | 1 | +1/-1 | 1 |

# v1.23.0 / 2023-04-21

This is the stable feature release for the upcoming MANDATORY network upgrade at `2023-04-27T13:00:00Z`, epoch `2809800`. This feature release delivers the nv19 Lighting and nv20 Thunder network upgrade for mainnet, and includes numerous improvements and enhancements for node operators, ETH RPC-providers and storage providers.

## ☢️ Upgrade Warnings ☢️

Please read carefully through the **upgrade warnings** section if you are upgrading from a v1.20.X release, or the v1.22.0 release. If you are upgrading from a v1.21.0-rcX these warnings should be familiar to you.

- Starting from this release, the SplitStore feature is automatically activated on new nodes. However, for existing Lotus users, you need to explicitly configure SplitStore by uncommenting the `EnableSplitstore` option in your `config.toml` file. To enable SplitStore, set `EnableSplitstore=true`, and to disable it, set `EnableSplitstore=false`. **It's important to note that your Lotus node will not start unless this configuration is properly set. Set it to false if you are running a full archival node!**
- This feature release requires a **minimum Go version of v1.19.7 or higher to successfully build Lotus**. Additionally, Go version v1.20 and higher is now also supported.
- **Storage Providers:** The proofs libraries now have CUDA enabled by default, which requires you to install [CUDA](https://lotus.filecoin.io/tutorials/lotus-miner/cuda/) if you haven't already done so. If you prefer to use OpenCL on your GPUs instead, you can use the `FFI_USE_OPENCL=1` flag when building from source. On the other hand, if you want to disable GPUs altogether, you can use the `FFI_NO_GPU=1` environment variable when building from source.
- **Storage Providers:** The `lotus-miner sectors extend` command has been refactored to the functionality of `lotus-miner sectors renew`.
- **Exchanges/Node operators/RPC-providers::** Execution traces (returned from `lotus state exec-trace`, `lotus state replay`, etc.), has changed to account for changes introduced by the FVM. **Please make sure to read the `Execution trace format change` section carefully, as these are interface breaking changes**
- **Syncing issues:** If you have been struggling with syncing issues in normal operations you can try to adjust the amount of threads used for more concurrent FMV execution through via the `LOTUS_FVM_CONCURRENCY` enviroment variable. It is set to 4 threads by default. Recommended formula for concurrency == YOUR_RAM/4 , but max during a network upgrade is 24. If you are a Storage Provider and are pushing many messages within a short period of time, exporting `LOTUS_SKIP_APPLY_TS_MESSAGE_CALL_WITH_GAS=1` will also help with keeping in sync.
- **Catching up from a Snapshot:** Users have noticed that catching up sync from a snapshot is taking a lot longer these day. This is largely related to the built-in market actor consuming a lot of computational demand for block validation. A FIP for a short-term mitigation for this is currently in Last Call and will be included network version 19 upgrade if accepted. You [can read the FIP here.](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0060.md)

## Highlights

### Execution Trace Format Changes

Execution traces (returned from `lotus state exec-trace`, `lotus state replay`, etc.), has changed to account for changes introduced by the FVM. Specifically:

- The `Msg` field no longer matches the Filecoin message format as many of the message fields didn't make sense in on-chain sub-calls. Instead, it now has the fields `To`, `From`, `Value`, `Method`, `Params`, and `ParamsCodec` where `ParamsCodec` is a new field indicating the IPLD codec of the parameters.
    - Importantly, the `Msg.CID` field has been removed. This field is still present in top-level invocation results, just not inside the execution trace itself.
- The `MsgRct` field no longer includes a `GasUsed` field and now has a `ReturnCodec` field to indicating the IPLD codec of the return value.
- The `Error` and `Duration` fields have been removed as these are not set by the FVM. The top-level message "invocation result" retains the `Error` and `Duration` fields, they've only been removed from the trace itself.
- Gas Charges no longer include "virtual" gas fields (those starting with `v...`) or source location information (`loc`) as neither field is set by the FVM.

A note on "codecs": FVM parameters and return values are IPLD blocks where the "codec" specifies the data encoding. The codec will generally be one of:

- `0x51`, `0x71` - CBOR or DagCBOR. You should generally treat these as equivalent.
- `0x55` - Raw bytes.
- `0x00` - Nothing. If the codec is `0x00`, the parameter and/or return value should be empty and should be treated as "void" (not specified).

<details>
<summary>
Old <code>ExecutionTrace</code>:
</summary>

```json
{
  "Msg": {
    "Version": 0,
    "To": "f01234",
    "From": "f04321",
    "Nonce": 1,
    "Value": "0",
    "GasLimit": 0,
    "GasFeeCap": "1234",
    "GasPremium": "1234",
    "Method": 42,
    "Params": "<base64-data-or-null>",
    "CID": {
        "/": "bafyxyz....."
    },
  },
  "MsgRct": {
    "ExitCode": 0,
    "Return": "<base64-data-or-null>",
    "GasUsed": 12345,
  },
  "Error": "",
  "Duration": 568191845,
  "GasCharges": [
    {
      "Name": "OnMethodInvocation",
      "loc": null,
      "tg": 23856,
      "cg": 23856,
      "sg": 0,
      "vtg": 0,
      "vcg": 0,
      "vsg": 0,
      "tt": 0
    },
    {
      "Name": "wasm_exec",
      "loc": null,
      "tg": 1764,
      "cg": 1764,
      "sg": 0,
      "vtg": 0,
      "vcg": 0,
      "vsg": 0,
      "tt": 0
    },
    {
      "Name": "OnSyscall",
      "loc": null,
      "tg": 14000,
      "cg": 14000,
      "sg": 0,
      "vtg": 0,
      "vcg": 0,
      "vsg": 0,
      "tt": 0
    },
  ],
  "Subcalls": [
    {
      "Msg": { },
      "MsgRct": { },
      "Error": "",
      "Duration": 1235,
      "GasCharges": [],
      "Subcalls": [],
    },
  ]
}
```
</details>

<details>
<summary>
New <code>ExecutionTrace</code>:
</summary>

```json
{
  "Msg": {
    "To": "f01234",
    "From": "f04321",
    "Value": "0",
    "Method": 42,
    "Params": "<base64-data-or-null>",
    "ParamsCodec": 81
  },
  "MsgRct": {
    "ExitCode": 0,
    "Return": "<base64-data-or-null>",
    "ReturnCodec": 81
  },
  "GasCharges": [
    {
      "Name": "OnMethodInvocation",
      "loc": null,
      "tg": 23856,
      "cg": 23856,
      "tt": 0
    },
    {
      "Name": "wasm_exec",
      "loc": null,
      "tg": 1764,
      "cg": 1764,
      "sg": 0,
      "tt": 0
    },
    {
      "Name": "OnSyscall",
      "loc": null,
      "tg": 14000,
      "cg": 14000,
      "sg": 0,
      "tt": 0
    },
  ],
  "Subcalls": [
    {
      "Msg": { },
      "MsgRct": { },
      "GasCharges": [],
      "Subcalls": [],
    },
  ]
}
```

</details>

**SplitStore**

This feature release introduces numerous improvements and fixes to tackle SplitStore related issues that has been reported. With this feature release SplitStore is automatically activated by default on new nodes. However, for existing Lotus users, you need to explicitly configure SplitStore by uncommenting the `EnableSplitstore` option in your `config.toml` file. To enable SplitStore, set `EnableSplitstore=true`, and to disable it, set `EnableSplitstore=false`. **It's important to note that your Lotus node will not start unless this configuration is properly set. Set it to false if you are running a full archival node!**

SplitStore also has some new configuration settings that you can set in your config.toml file:
- `HotstoreMaxSpaceTarget` suggests the max allowed space (in bytes) the hotstore can take.
- `HotstoreMaxSpaceThreshold` a moving GC will be triggered when total moving size exceeds this threshold (in bytes).
- `HotstoreMaxSpaceSafetyBuffer` a safety buffer to prevent moving GC from an overflowing disk.

The SplitStore also has two new commands:

- `lotus chain prune hot` is a much less resource-intensive GC and is best suited for situations where you don't have the spare disk space for a full GC.
- `lotus chain prune hot-moving` will run a full moving garbage collection of the hotstore. This commands create a new hotstore before deleting the old one so you need working room in the hotstore directory. The current size of a fully GC'd hotstore is around 295 GiB so you need to make sure you have at least that available.

You can read more about the new SplitStore commands in [the documentation](https://lotus.filecoin.io/lotus/configure/splitstore/#manual-chain-store-garbage-collection).

**RPC API improvements**

This feature release includes all the RPC API improvements made in the Lotus v1.20.x patch releases. It includes an updated FFI that sets the FVM parallelism to 4 by default.

Node operators with higher memory specs can experiment with setting LOTUS_FVM_CONCURRENCY to higher values, up to 48, to allow for more concurrent FVM execution.

**Experimental scheduler assigners**

In this release there are four new expirmental scheduler assigners:

- The `experiment-spread-qcount` - similar to the spread assigner but also takes into account task counts which are in running/preparing/queued states.
- The `experiment-spread-tasks` - similar to the spread assigner, but counts running tasks on a per-task-type basis
- The `experiment-spread-tasks-qcount` -  similar to the spread assigner, but also takes into account task counts which are in running/preparing/queued states, as well as counting running tasks on a per-task-type basis. Check the results for this assigner on ([storage-only lotus-workers here](https://github.com/filecoin-project/lotus/issues/8566#issuecomment-1446978856)).
- The `experiment-random` - In each schedule loop the assinger figures a set of all workers which can handle the task and then picks a random one. Check the results for this assigner on ([storage-only lotus-workers here](https://github.com/filecoin-project/lotus/issues/8566#issuecomment-1447064218)).

**Graceful shutdown of lotus-workers**
We have cleaned up some commands in the `lotus-worker` to make it less confusing how to gracefully shutting down a `lotus-worker` while there are incoming sealing tasks in the pipeline. To shut down a `lotus-worker` gracefully:

1. `lotus-worker tasks disable --all` and wait for the worker to finish processing its current tasks.
2. `lotus-worker stop` to detach it and do maintenance/upgrades.

**CLI speedups**

The `lotus-miner sector list` is now running in parallel - which should speed up the process from anywhere between 2x-10x+. You can tune it additionally with the `check-parallelism` option in the command. The `Lotus-Miner info` command also has a large speed improvement, as calls to the lotus legacy market has been removed.

## New features
- feat: splitstore: Pause compaction when out of sync ([filecoin-project/lotus/#10641](https://github.com/filecoin-project/lotus/pull/10641))
   - Pause the SplitStore compaction if the node is out of sync. Resumes the compation when its back in sync.
- feat: splitstore: limit moving gc threads (#10621) ([filecoin-project/lotus/#10621](https://github.com/filecoin-project/lotus/pull/10621))
   - Makes moving gc less likely to cause node falling out of sync.
- feat: splitstore: Update config default value (#10605) ([filecoin-project/lotus/#10605](https://github.com/filecoin-project/lotus/pull/10605))
   - Sets Splitstore HotStoreMaxSpaceTarget config to 650GB as default
- feat: splitstore: Splitstore enabled by default (#10429) ([filecoin-project/lotus#10429](https://github.com/filecoin-project/lotus/pull/10429))
   - Enables SplitStore by default on new Lotus nodes. Existing Lotus users need to explicitly configure 
- feat: splitstore: Configure max space used by hotstore and GC makes best effort to respect ([filecoin-project/lotus#10391](https://github.com/filecoin-project/lotus/pull/10391))
   - Adds three new configs for setting the maximum allowed space the hotstore can take.
- feat: splitstore: Badger GC of hotstore command ([filecoin-project/lotus#10387](https://github.com/filecoin-project/lotus/pull/10387))
   - Adds a `lotus chain prune hot` command, to run the garbage collection of the hotstore in a user driven way.
- feat: sched: Assigner experiments ([filecoin-project/lotus#10356](https://github.com/filecoin-project/lotus/pull/10356))
   - Introduces experimental scheduler assigners that works better for setups that uses storage-only lotus-workers. 
- fix: wdpost: disabled post worker handling ([filecoin-project/lotus#10394](https://github.com/filecoin-project/lotus/pull/10394))
   - Improved scheduling logic for Proof-of-SpaceTime workers.
- feat: cli: list claims and remove expired claims ([filecoin-project/lotus#9875](https://github.com/filecoin-project/lotus/pull/9875))
   - Adds a command to list claims made by a provider `lotus filplus list-claims`. And `lotus filplus remove-expired-claims` to remove expired claims.
- feat: cli: make sectors list much faster ([filecoin-project/lotus#10202](https://github.com/filecoin-project/lotus/pull/10202))
   - Makes `lotus-miner sector list` checks run in parallel.
- feat: cli: Add an EVM command to fetch a contract's bytecode ([filecoin-project/lotus#10443](https://github.com/filecoin-project/lotus/pull/10443))
   - Adds an `lotus evm bytecode` command to fetch a contract's bytecode.
- feat: mempool: Reduce minimum replace fee from 1.25x to 1.1x (#10416) ([filecoin-project/lotus#10416](https://github.com/filecoin-project/lotus/pull/10416))
   - Reduces replacement message fee logic to help include update message replacements from developers using Ethereum tools like MetaMask.
- feat: update renew-sectors with FIP-0045 logic ([filecoin-project/lotus#10328](https://github.com/filecoin-project/lotus/pull/10328))
   - Updates the `lotus-miner sectors extend` with FIP-0045 logic to include the ability to drop claims and set the maximum number of messages contained in a message.
- feat: IPC: Abstract common consensus functions and consensus interface ([filecoin-project/lotus#9481](https://github.com/filecoin-project/lotus/pull/9481))
   - Add eudico's consensus interface to Lotus and implement EC behind that interface. This abstraction is the stepping-stone for Mir's integration.
- fix: worker: add all tasks flag ([filecoin-project/lotus#10232](https://github.com/filecoin-project/lotus/pull/10232))
   - Adds an `all` flag for the `lotus-worker tasks enable/disable` cmds.
- feat:shed:add cid to cbor serialization command ([filecoin-project/lotus#10032](https://github.com/filecoin-project/lotus/pull/10032))
  - Adds two `lotus-shed` commands, `lotus-shed cid bytes` and `lotus-shed cid cbor` to serialize cid to cbor and cid to bytes.
- feat: add toolshed commands to inspect statetree size ([filecoin-project/lotus#9982](https://github.com/filecoin-project/lotus/pull/9982))
  - Adds two commands, `lotus-shed stat-actor` and `lotus-shed stat-obj` that work with an offline lotus repo to report dag size stats.
- feat: shed: encode address to bytes ([filecoin-project/lotus#10105](https://github.com/filecoin-project/lotus/pull/10105))
   - Adds a `lotus-shed address encode` for encoding a filecoin address to hex bytes.
- feat: chain: export-range ([filecoin-project/lotus#10145](https://github.com/filecoin-project/lotus/pull/10145))
   - Adds a `lotus chain export-range` command that can create archival-grade ranged exports of the chain as quickly as possible.
- feat: stmgr: cache migrated stateroots ([filecoin-project/lotus#10282](https://github.com/filecoin-project/lotus/pull/10282))
   - Cache network migration results to avoid running migrations twice.
- feat: shed: Add a tool to read data from sectors ([filecoin-project/lotus#10169](https://github.com/filecoin-project/lotus/pull/10169))
   - Adds a lotus-shed sectors read command that extract data from sectors from a running lotus-miner deployment.
- feat: cli: Refactor renew and remove extend ([filecoin-project/lotus#9920](https://github.com/filecoin-project/lotus/pull/9920))
   - Refactors the `lotus-miner sectors extend` command to have the functionality of `lotus-miner sectors renew`. The `lotus-miner sectors renew` command has been deprecated.
- feat: shed: Add beneficiary commands ([filecoin-project/lotus#10037](https://github.com/filecoin-project/lotus/pull/10037))
   - Adds the beneficiary address command to `lotus-shed`. You can now use `lotus-shed actor propose-change-beneficiary` and `lotus-shed actor confirm-change-beneficiary` to change beneficiary addresses.

## Improvements

- backport: fix: miner: correctly count sector extensions (10555) ([filecoin-project/lotus#10555](https://github.com/filecoin-project/lotus/pull/10555))
   - Fixes the issue with sector extensions.
- fix: proving: Initialize slice with with same length as partition (#10574) ([filecoin-project/lotus#10574](https://github.com/filecoin-project/lotus/pull/10574))
   - Fixes an issue where `lotus-miner proving compute window-post` paniced when trying to make skipped sectors human readable.
- feat: stmgr: speed up calculation of genesis circ supply (#10553) ([filecoin-project/lotus#10553](https://github.com/filecoin-project/lotus/pull/10553))
- perf: eth: gas estimate set applyTsMessages false (#10546) ([filecoin-project/lotus#10456](https://github.com/filecoin-project/lotus/pull/10546))
- feat: config: Force existing users to opt into new defaults (#10488) ([filecoin-project/lotus#10488](https://github.com/filecoin-project/lotus/pull/10488))
   - Force existing users to opt into the new SplitStore defaults.
- fix: splitstore: Demote now common logs (#10516) ([filecoin-project/lotus#10516](https://github.com/filecoin-project/lotus/pull/10516))
- fix: splitstore: Don't enforce walking receipt tree during compaction ([filecoin-project/lotus#10502](https://github.com/filecoin-project/lotus/pull/10502))
- fix: splitstore: Fix the overzealous fix (#10366) ([filecoin-project/lotus#10366](https://github.com/filecoin-project/lotus/pull/10366))
- fix: splitstore: Two fixes, better logging and comments (#10332) ([filecoin-project/lotus#10332](https://github.com/filecoin-project/lotus/pull/10332))
- fix: fsm: shutdown removed sectors FSMs ([filecoin-project/lotus#10363](https://github.com/filecoin-project/lotus/pull/10363))
   - Fixes an issue where removed sectors still got state machine events.
- fix: rpcenc: Don't hang when source dies ([filecoin-project/lotus#10116](https://github.com/filecoin-project/lotus/pull/10116))
   - Fixes an issue where AddPiece tasks could get stuck if the Boost process was abruptly lost.
- fix: make debugging windowPoSt-failures human readable ([filecoin-project/lotus#10390](https://github.com/filecoin-project/lotus/pull/10390))
   - Makes the skipped sector list in `lotus-miner proving compute window-post` human readable.
- fix: cli: Hide `lotus-worker set` command ([filecoin-project/lotus#10384](https://github.com/filecoin-project/lotus/pull/10384))
   - Hides the `lotus-worker set` command. This command will be deprecated later.
- fix: worker: Hide `wait-quiet` cmd ([filecoin-project/lotus#10331](https://github.com/filecoin-project/lotus/pull/10331))
   - Hides the `lotus-worker wait-quiet` command. This command will be deprecated later.
- fix: post: Tune down default post-parallel-reads ([filecoin-project/lotus#10365](https://github.com/filecoin-project/lotus/pull/10365))
   - Tuning down the default post-parallel-reads to a more conservative number to prevent sectors from being skipped due to network timeouts.
- fix: cli: error if backup file already exists ([filecoin-project/lotus#10209](https://github.com/filecoin-project/lotus/pull/10209))
   - Error out if a backup file with the same name already exists when using the `lotus-miner backup` or `lotus backup` command
- fix: cli: option to set-seal-delay in seconds ([filecoin-project/lotus#10208](https://github.com/filecoin-project/lotus/pull/10208))
   - Adds the option to specify `lotus-miner sectors set-seal-delay` in seconds
- fix: cli: extend cmd to get the right sector number ([filecoin-project/lotus#10182](https://github.com/filecoin-project/lotus/pull/10182))
   - Making sure the `lotus-miner sectors extend` command gets the correct sector number.
- feat: wdpost: Emit more detailed errors ([filecoin-project/lotus#10121](https://github.com/filecoin-project/lotus/pull/10121))
   - Emits more detailed windowPoSt error messages, making it easier to debug PoSt issues.
- fix: Lotus Gateway: Add missing methods - master ([filecoin-project/lotus#10420](https://github.com/filecoin-project/lotus/pull/10420))
   - Adds `StateNetworkName`, `MpoolGetNonce`, `StateCall` and `StateDecodeParams` methods to Lotus Gateway.
- fix: stmgr: don't attempt to lookup genesis state (#10472) ([filecoin-project/lotus#10472](https://github.com/filecoin-project/lotus/pull/10472))
- feat: gateway: export StateVerifierStatus ([filecoin-project/lotus#10477](https://github.com/filecoin-project/lotus/pull/10477))
- fix: gateway: correctly apply the fee history lookback max ([filecoin-project/lotus#10464](https://github.com/filecoin-project/lotus/pull/10464))
- fix: gateway: drop overzealous guard on MsigGetVested ([filecoin-project/lotus#10451](https://github.com/filecoin-project/lotus/pull/10451))
- feat: apply gateway lookback limit to eth API lookback ([filecoin-project/lotus#10467](https://github.com/filecoin-project/lotus/pull/10467))
- fix: revert "Eth API: drop support for 'pending' block parameter." ([filecoin-project/lotus#10474](https://github.com/filecoin-project/lotus/pull/10474))
- fix: Eth API: make net_version return the chain ID ([filecoin-project/lotus#10456](https://github.com/filecoin-project/lotus/pull/10456))
- fix: eth: handle a potential divide by zero in receipt handling ([filecoin-project/lotus#10495](https://github.com/filecoin-project/lotus/pull/10495))
- fix: ethrpc: Don't lock up when eth subscriber goes away ([filecoin-project/lotus#10485](https://github.com/filecoin-project/lotus/pull/10485))
- feat: eth: Avoid StateCompute in EthTxnReceipt lookup (#10460) ([filecoin-project/lotus#10460](https://github.com/filecoin-project/lotus/pull/10460))
- feat: eth: optimize eth block loading + eth_feeHistory ([filecoin-project/lotus#10446](https://github.com/filecoin-project/lotus/pull/10446))
- feat: state: skip tipset execution when possible ([filecoin-project/lotus#10445](https://github.com/filecoin-project/lotus/pull/10445))
- feat: eth API: reject masked ID addresses embedded in f410f payloads ([filecoin-project/lotus#10440](https://github.com/filecoin-project/lotus/pull/10440))
- fix: Eth API: make block parameter parsing sounder. ([filecoin-project/lotus#10427](https://github.com/filecoin-project/lotus/pull/10427))
- fix: eth API: return correct txIdx around null blocks (#10419) ([filecoin-project/lotus#10419](https://github.com/filecoin-project/lotus/pull/10419))
- fix: EthAPI: use StateCompute for feeHistory; apply minimum gas premium (#10413) ([filecoin-project/lotus#10413](https://github.com/filecoin-project/lotus/pull/10413))
- refactor: EthAPI: Drop unnecessary param from newEthTxReceipt ([filecoin-project/lotus#10411](https://github.com/filecoin-project/lotus/pull/10411))
- fix: eth API: correct gateway restrictions, drop unimplemented methods ([filecoin-project/lotus#10409](https://github.com/filecoin-project/lotus/pull/10409))
- fix: EthAPI: Correctly get parent hash ([filecoin-project/lotus#10389](https://github.com/filecoin-project/lotus/pull/10389))
- fix: EthAPI: Make newEthBlockFromFilecoinTipSet faster and correct ([filecoin-project/lotus#10380](https://github.com/filecoin-project/lotus/pull/10380))
- fix: eth: incorrect struct tags (#10309) ([filecoin-project/lotus#10309](https://github.com/filecoin-project/lotus/pull/10309))
- refactor: update cache to the new generic version (#10463) ([filecoin-project/lotus#10463](https://github.com/filecoin-project/lotus/pull/10463))
- feat: consensus: log ApplyBlock timing/gas stats ([filecoin-project/lotus#10470](https://github.com/filecoin-project/lotus/pull/10470))
- feat: chain: make chain tipset fetching 1000x faster ([filecoin-project/lotus#10423](https://github.com/filecoin-project/lotus/pull/10423))
- chain: explicitly check that gasLimit is above zero ([filecoin-project/lotus#10198](https://github.com/filecoin-project/lotus/pull/10198))
- feat: blockstore: Envvar can adjust badger compaction worker poolsize ([filecoin-project/lotus#9973](https://github.com/filecoin-project/lotus/pull/9973))
- feat: stmgr: add env to disable premigrations ([filecoin-project/lotus#10283](https://github.com/filecoin-project/lotus/pull/10283))
- chore: Remove legacy market info from lotus-miner info ([filecoin-project/lotus#10364](https://github.com/filecoin-project/lotus/pull/10364))
   - Removes the legacy market info in the `Lotus-Miner info`. Speeds up the command significantly.
- chore: blockstore: Plumb through a proper Flush() method on all blockstores ([filecoin-project/lotus#10465](https://github.com/filecoin-project/lotus/pull/10465))
- fix: extend LOTUS_CHAIN_BADGERSTORE_DISABLE_FSYNC to the markset ([filecoin-project/lotus#10172](https://github.com/filecoin-project/lotus/pull/10172))
- feat: vm: switch to the new exec trace format (#10372) ([filecoin-project/lotus#10372](https://github.com/filecoin-project/lotus/pull/10372))
- fix: Remove workaround that is no longer needed ([filecoin-project/lotus#9995](https://github.com/filecoin-project/lotus/pull/9995))
- feat: Check for allocation expiry when waiting to seal sectors ([filecoin-project/lotus#9878](https://github.com/filecoin-project/lotus/pull/9878))
- feat: Allow libp2p user agent to be overriden ([filecoin-project/lotus#10149](https://github.com/filecoin-project/lotus/pull/10149))
- feat: cli: Add global color flag ([filecoin-project/lotus#10022](https://github.com/filecoin-project/lotus/pull/10022))
- fix: should not serve non v0 api in v0 ([filecoin-project/lotus#10066](https://github.com/filecoin-project/lotus/pull/10066))
- fix: build: drop drand incentinet servers ([filecoin-project/lotus#10476](https://github.com/filecoin-project/lotus/pull/10476))
- fix: sealing: stub out the FileSize function on Windows ([filecoin-project/lotus#10035](https://github.com/filecoin-project/lotus/pull/10035))

## Dependencies
- github.com/filecoin-project/go-dagaggregator-unixfs (v0.2.0 -> v0.3.0):
- github.com/filecoin-project/go-fil-markets (v1.25.2 -> v1.27.0-rc1):
- github.com/filecoin-project/go-jsonrpc (v0.2.1 -> v0.2.3):
- github.com/filecoin-project/go-statemachine (v1.0.2 -> v1.0.3):
- github.com/filecoin-project/go-state-types (v0.10.0 -> v0.11.0-alpha-3)
- github.com/ipfs/go-cid (v0.3.2 -> v0.4.0):
- github.com/ipfs/go-libipfs (v0.5.0 -> v0.7.0):
- github.com/ipfs/go-path (v0.3.0 -> v0.3.1):
- chore: deps: update to go-state-types v0.11.0-alpha-3 (([filecoin-project/lotus#10606](https://github.com/filecoin-project/lotus/pull/10606))
- deps: update go-libp2p-pubsub to v0.9.3 ([filecoin-project/lotus#10483](https://github.com/filecoin-project/lotus/pull/10483))
- deps: Update go-jsonrpc to v0.2.2 ([filecoin-project/lotus#10395](https://github.com/filecoin-project/lotus/pull/10395))
- Update to go-data-transfer v2 and libp2p, still wip ([filecoin-project/lotus#10382](https://github.com/filecoin-project/lotus/pull/10382))
- dep: ipld: update ipld prime to v0.20.0 ([filecoin-project/lotus#10247](https://github.com/filecoin-project/lotus/pull/10247))
- chore: node: migrate go-bitswap to go-libipfs/bitswap ([filecoin-project/lotus#10138](https://github.com/filecoin-project/lotus/pull/10138))
- chore: all: bump go-libipfs to replace go-block-format ([filecoin-project/lotus#10126](https://github.com/filecoin-project/lotus/pull/10126))
- chore: market: Upgrade to index-provider 0.10.0 ([filecoin-project/lotus#9981](https://github.com/filecoin-project/lotus/pull/9981))
- chore: all: bump go-libipfs ([filecoin-project/lotus#10563](https://github.com/filecoin-project/lotus/pull/10563))

## Others
- Update service_developer_bug_report.yml ([filecoin-project/lotus#10321](https://github.com/filecoin-project/lotus/pull/10321))
- Update service_developer_bug_report.yml ([filecoin-project/lotus#10321](https://github.com/filecoin-project/lotus/pull/10321))
- chore: github: Service-provider/dev bug template ([filecoin-project/lotus#10321](https://github.com/filecoin-project/lotus/pull/10321))
- chore: github: update enhancement and feature templates ([filecoin-project/lotus#10291](https://github.com/filecoin-project/lotus/pull/10291))
- chore: github: Update bug_report template ([filecoin-project/lotus#10289](https://github.com/filecoin-project/lotus/pull/10289))
- fix: itest: avoid failing the test when we race the miner ([filecoin-project/lotus#10461](https://github.com/filecoin-project/lotus/pull/10461))
- fix: github: Discussion and FIP links in `New Issue` ([filecoin-project/lotus#10268](https://github.com/filecoin-project/lotus/pull/10268))
- fix: state: short-circuit genesis state computation ([filecoin-project/lotus#10397](https://github.com/filecoin-project/lotus/pull/10397))
- fix: rpcenc: deflake TestReaderRedirectDrop ([filecoin-project/lotus#10406](https://github.com/filecoin-project/lotus/pull/10406))
- fix: tests: Fix TestMinerAllInfo test ([filecoin-project/lotus#10319](https://github.com/filecoin-project/lotus/pull/10319))
- fix: tests: Make TestWorkerKeyChange not flaky ([filecoin-project/lotus#10320](https://github.com/filecoin-project/lotus/pull/10320))
- test: eth: make sure we can deploy a new placeholder on transfer (#10281) ([filecoin-project/lotus#10281](https://github.com/filecoin-project/lotus/pull/10281))
- fix: itests: Fix flaky paych test ([filecoin-project/lotus#10100](https://github.com/filecoin-project/lotus/pull/10100))
- fix: cli: add ArgsUsage ([filecoin-project/lotus#10147](https://github.com/filecoin-project/lotus/pull/10147))
- chore: cli: cleanup cli  ([filecoin-project/lotus#10114](https://github.com/filecoin-project/lotus/pull/10114))
- chore: cli: Remove unneeded individual color flags ([filecoin-project/lotus#10028](https://github.com/filecoin-project/lotus/pull/10028))
- fix: cli: remove requirements in helptext ([filecoin-project/lotus#9969](https://github.com/filecoin-project/lotus/pull/9969))
- chore: build: release v1.21.0-rc1 prep ([filecoin-project/lotus#10524](https://github.com/filecoin-project/lotus/pull/10524))
- chore: merge release/v1.20.0 into master ([filecoin-project/lotus#10308](https://github.com/filecoin-project/lotus/pull/10308))
- chore: merge release branch into master ([filecoin-project/lotus#10272](https://github.com/filecoin-project/lotus/pull/10272))
- chore: merge release/v1.20.0 into master ([filecoin-project/lotus#10238](https://github.com/filecoin-project/lotus/pull/10238))
- chore: releases to master  ([filecoin-project/lotus#10490](https://github.com/filecoin-project/lotus/pull/10490))
- chore: merge releases into master ([filecoin-project/lotus#10377](https://github.com/filecoin-project/lotus/pull/10377))
- chore: merge release/v1.20.0 into master ([filecoin-project/lotus#10030](https://github.com/filecoin-project/lotus/pull/10030))
- chore: update ffi to increase execution parallelism (#10480) ([filecoin-project/lotus#10480](https://github.com/filecoin-project/lotus/pull/10480))
- chore: update the FFI for release (#10435) ([filecoin-project/lotus#10444](https://github.com/filecoin-project/lotus/pull/10444))
- build: bump version to v1.21.0-dev ([filecoin-project/lotus#10249](https://github.com/filecoin-project/lotus/pull/10249))
- build: docker: Update GO-version (#10591) ([filecoin-project/lotus#10249](https://github.com/filecoin-project/lotus/pull/10591))
- chore: merge release/v1.20.0 into master ([filecoin-project/lotus#10184](https://github.com/filecoin-project/lotus/pull/10184))
- docs: API Gateway: patch documentation note about make gen command ([filecoin-project/lotus#10422](https://github.com/filecoin-project/lotus/pull/10422))
- chore: docs: fix docs typos ([filecoin-project/lotus#10155](https://github.com/filecoin-project/lotus/pull/10155))
- chore: docker: Add back <<network>> parameter for docker push ([filecoin-project/lotus#10096](https://github.com/filecoin-project/lotus/pull/10096))
- chore: docker: Properly balance <<?>> in circleci docker config ([filecoin-project/lotus#10088](https://github.com/filecoin-project/lotus/pull/10088))
- chore: ci: Fix dirty git state when building docker images ([filecoin-project/lotus#10125](https://github.com/filecoin-project/lotus/pull/10125))
- chore: build: Remove AppImage and Snapcraft build automation ([filecoin-project/lotus#10003](https://github.com/filecoin-project/lotus/pull/10003))
- chore: ci: Update codeql to v2 ([filecoin-project/lotus#10120](https://github.com/filecoin-project/lotus/pull/10120))
- feat: ci: make ci more efficient ([filecoin-project/lotus#9910](https://github.com/filecoin-project/lotus/pull/9910))
- feat: scripts: go.mod dep diff script ([filecoin-project/lotus#9711](https://github.com/filecoin-project/lotus/pull/9711))

## Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Hannah Howard | 2 | +2909/-6026 | 84 |
| Łukasz Magiera | 42 | +2967/-1848 | 95 |
| Steven Allen | 20 | +1703/-1345 | 88 |
| Alfonso de la Rocha | 17 | +823/-1808 | 86 |
| Peter Rabbitson | 9 | +1957/-219 | 34 |
| Geoff Stuart | 12 | +818/-848 | 29 |
| hannahhoward | 5 | +507/-718 | 36 |
| Hector Sanjuan | 6 | +443/-726 | 35 |
| Kevin Li | 1 | +1124/-14 | 22 |
| zenground0 | 30 | +791/-269 | 88 |
| frrist | 1 | +992/-16 | 13 |
| Travis Person | 4 | +837/-53 | 24 |
| Phi | 20 | +622/-254 | 34 |
| Ian Davis | 7 | +35/-729 | 20 |
| Aayush | 10 | +378/-177 | 40 |
| Raúl Kripalani | 15 | +207/-138 | 19 |
| Arsenii Petrovich | 7 | +248/-94 | 30 |
| ZenGround0 | 5 | +238/-39 | 15 |
| Neel Virdy | 1 | +109/-107 | 58 |
| ychiao | 1 | +135/-39 | 3 |
| Jorropo | 2 | +87/-82 | 67 |
| Marten Seemann | 8 | +69/-64 | 17 |
| Rod Vagg | 1 | +55/-16 | 3 |
| Masih H. Derkani | 3 | +39/-27 | 12 |
| raulk | 2 | +30/-29 | 5 |
| dependabot[bot] | 4 | +37/-17 | 8 |
| beck | 2 | +38/-2 | 2 |
| Jennifer Wang | 4 | +20/-19 | 19 |
| Richard Guan | 3 | +28/-8 | 5 |
| omahs | 7 | +14/-14 | 7 |
| dirkmc | 2 | +19/-7 | 6 |
| David Choi | 2 | +16/-5 | 2 |
| Mike Greenberg | 1 | +18/-1 | 1 |
| Adin Schmahmann | 1 | +19/-0 | 2 |
| Phi-rjan | 5 | +12/-4 | 5 |
| Dirk McCormick | 2 | +6/-6 | 3 |
| Aayush Rajasekaran | 2 | +9/-3 | 2 |
| Jiaying Wang | 5 | +6/-4 | 5 |
| Anjor Kanekar | 1 | +5/-5 | 1 |
| vyzo | 1 | +3/-3 | 2 |
| 0x5459 | 1 | +1/-1 | 1 |

# v1.22.1 / 2023-04-23

## Important Notice

This is a MANDATORY hotfix release that fixes a consensus-critical bug that was in v1.22.0 -- the necessary fix is https://github.com/filecoin-project/ref-fvm/pull/1750 and it is integrated into lotus via https://github.com/filecoin-project/lotus/pull/10735.
You can NOT use 1.22.0 for the nv19 upgrade, you MUST be on 1.22.1 or higher.

## About This Release

This is the stable release of Lotus v1.22.1 for the upcoming MANDATORY network upgrade at `2023-04-27T13:00:00Z`, epoch `2809800`. This release delivers the nv19 Lighting and nv20 Thunder network upgrade for mainnet.

Note that you must be on a go version higher than Go 1.18.8, but lower than Go v1.20.0. We would recommend Go 1.19.7.

The Lighting and Thunder upgrade introduces the following Filecoin Improvement Proposals (FIPs), delivered by builtin-actors v11 (see actors [v11.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v11.0.0-rc2)):

- [FIP 0060](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0060.md) - Thirty day market deal maintenance interval
- [FIP 0061](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0061.md) - WindowPoSt grindability fix
- [FIP 0062](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0062.md) - Fallback method handler for multisig actor

## Expedited nv19 Lightning ⚡️ rollout

In light of the recent degraded chain quality on the mainnet [an expedited nv19 upgrade has been proposed and accepted](https://github.com/filecoin-project/core-devs/discussions/123#discussioncomment-5642909) to roll out the market cron mitigation ([FIP0060](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0060.md)) that will improve block validation times, and with that the delay in block production that is causing a decrease in the chain quality currently.

With this expedited roll out we want to inform you of some **key changes and important dates:**

- Accelerate the nv19-upgrade on **mainnet** from May 11th to **April 27th**.
- Derisk nv19 by descoping the sector info migration, activation epoch fixes and drop [[FIP0052 - Extend sector/deal max duration to 3.5 year.](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)
  - By descoping these changes we can greatly derisk the network upgrade itself by removing a heavy migration that could cause instability for storage providers and node operators during the network upgrade.
- Increase the rollover period for [[FIP0061](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md) from 1 week to 3 weeks on mainnet. The rollover period is the duration between nv19 and nv20 which both old proofs (v1) and the new proofs (v1_1) proofs will be accepted by the network.

The Lighting and Thunder upgrade now implements the following Filecoin Improvement Proposals (FIPs), delivered by builtin-actors v11 (see actors [v11.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v11.0.0)):

- [FIP 0060](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0060.md) - Thirty day market deal maintenance interval
- [FIP 0061](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0061.md) - WindowPoSt grindability fix
- [FIP 0062](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0062.md) - Fallback method handler for multisig actor

## v11 Builtin Actor Bundles

Make sure that your lotus actor bundle matches the v11 actors manifest by running after upgrading:

```
lotus state actor-cids --network-version 19
Network Version: 19
Actor Version: 11
Manifest CID: bafy2bzacecnhaiwcrpyjvzl4uv4q3jzoif26okl3m66q3cijp3dfwlcxwztwo

Actor             CID  
datacap           bafk2bzacebslykoyrb2hm7aacjngqgd5n2wmeii2goadrs5zaya3pvdf6pdnq
init              bafk2bzaceckwf3w6n2nw6eh77ktmsxqgsvshonvgnyk5q5syyngtetxvasfxg
reward            bafk2bzacebwjw2vxkobs7r2kwjdqqb42h2kucyuk6flbnyzw4odg5s4mogamo
cron              bafk2bzacebpewdvvgt6tk2o2u4rcovdgym67tadiis5usemlbejg7k3kt567o
ethaccount        bafk2bzaceclkmc4yidxc6lgcjpfypbde2eddnevcveo4j5kmh4ek6inqysz2k
evm               bafk2bzacediwh6etwzwmb5pivtclpdplewdjzphouwqpppce6opisjv2fjqfe
storagemarket     bafk2bzaceazu2j2zu4p24tr22btnqzkhzjvyjltlvsagaj6w3syevikeb5d7m
storagepower      bafk2bzaceaxgloxuzg35vu7l7tohdgaq2frsfp4ejmuo7tkoxjp5zqrze6sf4
system            bafk2bzaced7npe5mt5nh72jxr2igi2sofoa7gedt4w6kueeke7i3xxugqpjfm
account           bafk2bzacealnlr7st6lkwoh6wxpf2hnrlex5sknaopgmkr2tuhg7vmbfy45so
placeholder       bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro
eam               bafk2bzaceaelwt4yfsfvsu3pa3miwalsvy3cfkcjvmt4sqoeopsppnrmj2mf2
multisig          bafk2bzaceafajceqwg5ybiz7xw6rxammuirkgtuv625gzaehsqfprm4bazjmk
paymentchannel    bafk2bzaceb4e6cnsnviegmqvsmoxzncruvhra54piq7bwiqfqevle6oob2gvo
storageminer      bafk2bzacec24okjqrp7c7rj3hbrs5ez5apvwah2ruka6haesgfngf37mhk6us
verifiedregistry  bafk2bzacedej3dnr62g2je2abmyjg3xqv4otvh6e26du5fcrhvw7zgcaaez3a
```

## Changelog

- feat: build: set Lightning and Thunder upgrade epochs [filecoin-project/lotus#10716](https://github.com/filecoin-project/lotus/pull/10707)
- fix: PoSt worker: use go-state-types for proof policies [filecoin-project/lotus#10716](https://github.com/filecoin-project/lotus/pull/10716)
- chore: deps: update to actors v11.0.0 [filecoin-project/lotus#10718](https://github.com/filecoin-project/lotus/pull/10718)
- chore: deps: update to go-state-types v0.11.1 [filecoin-project/lotus#10720](https://github.com/filecoin-project/lotus/pull/10720)
- feat: upgrade: expedite nv19 [filecoin-project/lotus#10681](https://github.com/filecoin-project/lotus/pull/10681)
  - Update changelog build version (commit: [67d419e](https://github.com/filecoin-project/lotus/commit/67d419e1623e6b9f5b871d6157a3096378477c3b))
  - Update actors v11 (commit: [5df4f75](https://github.com/filecoin-project/lotus/commit/5df4f75dc22318fd304313714d5c4f4cfeed22c9))
  - Correct epoch to match specified date (commit: [a28fcea](https://github.com/filecoin-project/lotus/commit/a28fceaa559b6c7e1b5df09383af56a5c2f51caa))
  - Fast butterfly migration to validate migration (commit: [37a0dca](https://github.com/filecoin-project/lotus/commit/37a0dca11ebfadebad3920a337b4f1b2fba08a7b))
  - Make docsgen (commit: [daba4ff](https://github.com/filecoin-project/lotus/commit/daba4ff5f0e97ab6ed444a34f61499a64b92a220))
  - Update go-state-types (commit: [244ca0b](https://github.com/filecoin-project/lotus/commit/244ca0b5f32a2af684f3f9586b92861a06bb8833))
  - Revert FIP0052 (commit: [68ed494](https://github.com/filecoin-project/lotus/commit/68ed494a6e497ac556eb93b28b2536c881dc9a4c))
  - Modify upgrade schedule and params (commit: [fa0dfdf](https://github.com/filecoin-project/lotus/commit/fa0dfdfd9f89fab8491f3e613909782ab9bb7cee))
  - Update go-state-types (commit: [19ae05f](https://github.com/filecoin-project/lotus/commit/19ae05f3b3a589e28efe4690c5816dfc1c7866a6))

### Dependencies
github.com/filecoin-project/go-state-types (v0.11.0-rc1 -> v0.11.1):

# v1.22.0 / 2023-04-21

EDIT: Do NOT use this release for nv19, you MUST use v1.22.1 or higher.

This is the stable release of Lotus v1.22.0 for the upcoming MANDATORY network upgrade at `2023-04-27T13:00:00Z`, epoch `2809800`. This release delivers the nv19 Lighting and nv20 Thunder network upgrade for mainnet.

Note that you must be on a go version higher then Go 1.18.8, but lower then Go v1.20.0. We would recommend Go 1.19.7.

The Lighting and Thunder upgrade introduces the following Filecoin Improvement Proposals (FIPs), delivered by builtin-actors v11 (see actors [v11.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v11.0.0-rc2)):

- [FIP 0060](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0060.md) - Thirty day market deal maintenance interval
- [FIP 0061](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0061.md) - WindowPoSt grindability fix
- [FIP 0062](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0062.md) - Fallback method handler for multisig actor

## Expedited nv19 Lightning ⚡️ rollout

In light of the recent degraded chain quality on the mainnet [an expedited nv19 upgrade has been proposed and accepted](https://github.com/filecoin-project/core-devs/discussions/123#discussioncomment-5642909) to roll out the market cron mitigation ([FIP0060](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0060.md)) that will improve block validation times, and with that the delay in block production that is causing a decrease in the chain quality currently.

With this expedited roll out we want to inform you of some **key changes and important dates:**

- Accelerate the nv19-upgrade on **mainnet** from May 11th to **April 27th**.
- Derisk nv19 by descoping the sector info migration, activation epoch fixes and drop [[FIP0052 - Extend sector/deal max duration to 3.5 year.](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)
    - By descoping these changes we can greatly derisk the network upgrade itself by removing a heavy migration that could cause instability for storage providers and node operators during the network upgrade.
- Increase the rollover period for [[FIP0061](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0052.md) from 1 week to 3 weeks on mainnet. The rollover period is the duration between nv19 and nv20 which both old proofs (v1) and the new proofs (v1_1) proofs will be accepted by the network.

The Lighting and Thunder upgrade now implements the following Filecoin Improvement Proposals (FIPs), delivered by builtin-actors v11 (see actors [v11.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v11.0.0)):

- [FIP 0060](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0060.md) - Thirty day market deal maintenance interval
- [FIP 0061](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0061.md) - WindowPoSt grindability fix
- [FIP 0062](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0062.md) - Fallback method handler for multisig actor

## v11 Builtin Actor Bundles

Make sure that your lotus actor bundle matches the v11 actors manifest by running after upgrading:

```
lotus state actor-cids --network-version 19
Network Version: 19
Actor Version: 11
Manifest CID: bafy2bzacecnhaiwcrpyjvzl4uv4q3jzoif26okl3m66q3cijp3dfwlcxwztwo

Actor             CID  
datacap           bafk2bzacebslykoyrb2hm7aacjngqgd5n2wmeii2goadrs5zaya3pvdf6pdnq
init              bafk2bzaceckwf3w6n2nw6eh77ktmsxqgsvshonvgnyk5q5syyngtetxvasfxg
reward            bafk2bzacebwjw2vxkobs7r2kwjdqqb42h2kucyuk6flbnyzw4odg5s4mogamo
cron              bafk2bzacebpewdvvgt6tk2o2u4rcovdgym67tadiis5usemlbejg7k3kt567o
ethaccount        bafk2bzaceclkmc4yidxc6lgcjpfypbde2eddnevcveo4j5kmh4ek6inqysz2k
evm               bafk2bzacediwh6etwzwmb5pivtclpdplewdjzphouwqpppce6opisjv2fjqfe
storagemarket     bafk2bzaceazu2j2zu4p24tr22btnqzkhzjvyjltlvsagaj6w3syevikeb5d7m
storagepower      bafk2bzaceaxgloxuzg35vu7l7tohdgaq2frsfp4ejmuo7tkoxjp5zqrze6sf4
system            bafk2bzaced7npe5mt5nh72jxr2igi2sofoa7gedt4w6kueeke7i3xxugqpjfm
account           bafk2bzacealnlr7st6lkwoh6wxpf2hnrlex5sknaopgmkr2tuhg7vmbfy45so
placeholder       bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro
eam               bafk2bzaceaelwt4yfsfvsu3pa3miwalsvy3cfkcjvmt4sqoeopsppnrmj2mf2
multisig          bafk2bzaceafajceqwg5ybiz7xw6rxammuirkgtuv625gzaehsqfprm4bazjmk
paymentchannel    bafk2bzaceb4e6cnsnviegmqvsmoxzncruvhra54piq7bwiqfqevle6oob2gvo
storageminer      bafk2bzacec24okjqrp7c7rj3hbrs5ez5apvwah2ruka6haesgfngf37mhk6us
verifiedregistry  bafk2bzacedej3dnr62g2je2abmyjg3xqv4otvh6e26du5fcrhvw7zgcaaez3a
```

## Changelog


- feat: build: set Lightning and Thunder upgrade epochs [filecoin-project/lotus#10716](https://github.com/filecoin-project/lotus/pull/10707)
- fix: PoSt worker: use go-state-types for proof policies [filecoin-project/lotus#10716](https://github.com/filecoin-project/lotus/pull/10716)
- chore: deps: update to actors v11.0.0 [filecoin-project/lotus#10718](https://github.com/filecoin-project/lotus/pull/10718)
- chore: deps: update to go-state-types v0.11.1 [filecoin-project/lotus#10720](https://github.com/filecoin-project/lotus/pull/10720)
- feat: upgrade: expedite nv19 [filecoin-project/lotus#10681](https://github.com/filecoin-project/lotus/pull/10681)
   - Update changelog build version (commit: [67d419e](https://github.com/filecoin-project/lotus/commit/67d419e1623e6b9f5b871d6157a3096378477c3b))
   - Update actors v11 (commit: [5df4f75](https://github.com/filecoin-project/lotus/commit/5df4f75dc22318fd304313714d5c4f4cfeed22c9))
   - Correct epoch to match specified date (commit: [a28fcea](https://github.com/filecoin-project/lotus/commit/a28fceaa559b6c7e1b5df09383af56a5c2f51caa))
   - Fast butterfly migration to validate migration (commit: [37a0dca](https://github.com/filecoin-project/lotus/commit/37a0dca11ebfadebad3920a337b4f1b2fba08a7b))
   - Make docsgen (commit: [daba4ff](https://github.com/filecoin-project/lotus/commit/daba4ff5f0e97ab6ed444a34f61499a64b92a220))
   - Update go-state-types (commit: [244ca0b](https://github.com/filecoin-project/lotus/commit/244ca0b5f32a2af684f3f9586b92861a06bb8833))
   - Revert FIP0052 (commit: [68ed494](https://github.com/filecoin-project/lotus/commit/68ed494a6e497ac556eb93b28b2536c881dc9a4c))
   - Modify upgrade schedule and params (commit: [fa0dfdf](https://github.com/filecoin-project/lotus/commit/fa0dfdfd9f89fab8491f3e613909782ab9bb7cee))
   - Update go-state-types (commit: [19ae05f](https://github.com/filecoin-project/lotus/commit/19ae05f3b3a589e28efe4690c5816dfc1c7866a6))

### Dependencies
github.com/filecoin-project/go-state-types (v0.11.0-rc1 -> v0.11.1):

# v1.20.4 / 2023-03-17

This is a patch release intended to alleviate performance issues reported by some users since the nv18 upgrade. 
The primary change is to update the FFI to allow for FVM parallelism of 4 by default, and make this user-configurable.
through the `LOTUS_FVM_CONCURRENCY` env var. 

Users with higher memory specs can experiment with setting `LOTUS_FVM_CONCURRENCY` to higher values, up to 48, to allow for more concurrent FVM execution.

## Bug fixes

- Splitstore: Don't enforce walking receipt tree during compaction #10505
- fix: build: drop drand incentinet servers #10506 

## Improvement

- chore: update ffi to increase execution parallelism #10503 

# v1.20.3 / 2023-03-09

A 🐈 stepped on the ⌨️ and made a mistake while resolving conflicts 😨. This releases only includes #10439 to fix that mistake. v1.20.2 is retracted - Please skip v1.20.2 and **only** update to v1.20.3!!!

## Changelog
> compare to v1.20.1

This is a HIGHLY RECOMMENDED patch release for node operators/API service providers that run ETH RPC service and an optional release for Storage Providers.

## Bug fixes
- fix: EthAPI: use StateCompute for feeHistory; apply minimum gas premium #10413
- fix: eth API: return correct txIdx around null blocks #10419
- fix: Eth API: make block parameter parsing sounder. #10427

## Improvement
- feat: Lotus Gateway: Add missing methods - master #10420
- feat: mempool: Reduce minimum replace fee from 1.25x to 1.1x #10416
- We recommend storage providers to update your nodes to this patch, that will help improve developers who use Ethereum tooling's experience.

# v1.20.2 / 2023-03-09

DO NOT USE: Use 1.20.3 instead!

This is a HIGHLY RECOMMENDED patch release for node operators/API service providers that run ETH RPC service and an optional release for Storage Providers.

## Bug fixes
- fix: EthAPI: use StateCompute for feeHistory; apply minimum gas premium #10413
- fix: eth API: return correct txIdx around null blocks #10419
- fix: Eth API: make block parameter parsing sounder. #10427

## Improvement
- feat: Lotus Gateway: Add missing methods - master #10420
- feat: mempool: Reduce minimum replace fee from 1.25x to 1.1x #10416
 - We recommend storage providers to update your nodes to this patch, that will help improve developers who use Ethereum tooling's experience.

# v1.20.1 / 2023-03-06

This an optional patch releases for node operators/API service providers that run ETH RPC service.

## Bug fixes
- fix: EthAPI: Correctly get parent hash #10389
- fix: EthAPI: Make newEthBlockFromFilecoinTipSet faster and correct #10380
- fix: state: short-circuit genesis state computation

# 1.20.0 / 2023-02-28

This is a MANDATORY release of Lotus that delivers the [Hygge network upgrade](https://github.com/filecoin-project/community/discussions/74?sort=top#discussioncomment-4313888), introducing Filecoin network version 18. The centerpiece of the upgrade is the introduction of the [Filecoin Virtual Machine (FVM)’s Milestone 2.1](https://fvm.filecoin.io/), which will allow for EVM-compatible contracts to be deployed on the Filecoin network. This upgrade delivers user-programmablity to the Filecoin network for the first time!

The Filecoin mainnet is scheduled to upgrade to nv18 at epoch 2683348, on March 14th at 2023-03-14T15:14:00Z. All node operators, including storage providers, must upgrade to this release before that time. Storage providers must update their daemons, miners, market and worker(s)/boost.
At the upgrade, a short migration will run that converts code actors v9 code CIDs to v10 CIDs, and installs the new Ethereum Address Manager singleton (see below). This is expected to be a lightweight migration that causes no service disruption.

The Hygge upgrade introduces the following Filecoin Improvement Proposals (FIPs), delivered in FVM3 (see FVM [v3.0.0](https://github.com/filecoin-project/ref-fvm/pull/1683)) and builtin-actors v10 (see actors [v10.0.0](https://github.com/filecoin-project/builtin-actors/releases/tag/v10.0.0)):

- [FIP-0048](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0048.md): f4 Address Class
- [FIP-0049](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0049.md): Actor events
- [FIP-0050](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0050.md): API between user-programmed actors and built-in actors
- [FIP-0054](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0054.md): Filecoin EVM runtime (FEVM)
- [FIP-0055](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0055.md): Supporting Ethereum Accounts, Addresses, and Transactions
- [FIP-0057](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0057.md): Update gas charging schedule and system limits for FEVM

## Filecoin Ethereum Virtual Machine (FEVM)

The Filecoin Ethereum Virtual Machine (FEVM) is built on top of the WASM-based execution environment introduced in the Skyr v16 upgrade. The chief feature introduced is the ability for anyone participating in the Filecoin network to deploy their own EVM-compatible contracts onto the blockchain, and invoke them as appropriate.

## New Built-in Actors

The FEVM is principally delivered through the introduction of **the new [EVM actor](https://github.com/filecoin-project/builtin-actors/tree/master/actors/evm)**. This actor “represents” smart contracts on the Filecoin network, and includes an interpreter that implements all EVM opcodes as their Filecoin equivalents, and translates state I/O operations to be compatible with Filecoin’s IPLD-based data model. For more on the EVM actors, please see [FIP-0054](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0054.md).

The creation of EVM actors is managed by **the new** [Ethereum Address Manager actor (EAM)](https://github.com/filecoin-project/builtin-actors/tree/master/actors/eam), a singleton that is invoked in order to deploy EVM actors. In order to make usage of the FEVM as seamless as possible for users familiar with the Ethereum ecosystem, this upgrades also introduces **a dedicated actor to serve as “[Ethereum Accounts](https://github.com/filecoin-project/builtin-actors/tree/master/actors/ethaccount)”**. This actor exists to allow for secp keys to be used in the Ethereum addressing scheme. **The last new built-in actor introduced is [the Placeholder actor](https://github.com/filecoin-project/builtin-actors/tree/master/actors/placeholder)**, a thin “shell” of an actor that can transform into either EVM or EthAccount actors. For more on the EAM, EthAccount, and Placeholder actors, please see [FIP-0055](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0055.md).

### v10 Built-in actor bundles

Bundles for all networks (mainnet, calibnet, etc.) are included in the lotus source tree (`build/actors/`) and embedded on build, for v10 actors you can find it [here](https://github.com/filecoin-project/lotus/blob/master/build/actors/v10.tar.zst). 
Reminder: Lotus verifies that the bundle CIDs are the right ones upon build & upgrade against the values in `build/builtin_actors_gen.go`, according to the network you are building. You may also check the bundle manifest CID matches the bundle gen-ed values by running `lotus state actor-cids --network-version 18`.

The manifest CID & full list of actor code CIDs for nv18 using [actor v10](https://github.com/filecoin-project/builtin-actors/releases/tag/v10.0.0) is:

    "_manifest":        "bafy2bzacecsuyf7mmvrhkx2evng5gnz5canlnz2fdlzu2lvcgptiq2pzuovos"
    "account":          "bafk2bzaceampw4romta75hyz5p4cqriypmpbgnkxncgxgqn6zptv5lsp2w2bo"
    "cron":             "bafk2bzacedcbtsifegiu432m5tysjzkxkmoczxscb6hqpmrr6img7xzdbbs2g"
    "datacap":          "bafk2bzacealj5uk7wixhvk7l5tnredtelralwnceafqq34nb2lbylhtuyo64u"
    "eam":             "bafk2bzacedrpm5gbleh4xkyo2jvs7p5g6f34soa6dpv7ashcdgy676snsum6g"
    "ethaccount":   "bafk2bzaceaqoc5zakbhjxn3jljc4lxnthllzunhdor7sxhwgmskvc6drqc3fa"
    "evm":                "bafk2bzaceahmzdxhqsm7cu2mexusjp6frm7r4kdesvti3etv5evfqboos2j4g"
    "init":                "bafk2bzaced2f5rhir3hbpqbz5ght7ohv2kgj42g5ykxrypuo2opxsup3ykwl6"
    "multisig":         "bafk2bzaceduf3hayh63jnl4z2knxv7cnrdenoubni22fxersc4octlwpxpmy4"
    "paymentchannel":   "bafk2bzaceartlg4mrbwgzcwric6mtvyawpbgx2xclo2vj27nna57nxynf3pgc"
    "placeholder": "bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro"
    "reward":           "bafk2bzacebnhtaejfjtzymyfmbdrfmo7vgj3zsof6zlucbmkhrvcuotw5dxpq"
    "storagemarket":    "bafk2bzaceclejwjtpu2dhw3qbx6ow7b4pmhwa7ocrbbiqwp36sq5yeg6jz2bc"
    "storageminer":     "bafk2bzaced4h7noksockro7glnssz2jnmo2rpzd7dvnmfs4p24zx3h6gtx47s"
    "storagepower":     "bafk2bzacec4ay4crzo73ypmh7o3fjendhbqrxake46bprabw67fvwjz5q6ixq"
    "system":           "bafk2bzacedakk5nofebyup4m7nvx6djksfwhnxzrfuq4oyemhpl4lllaikr64"
    "verifiedregistry": "bafk2bzacedfel6edzqpe5oujno7fog4i526go4dtcs6vwrdtbpy2xq6htvcg6"

## Node Operators

FVM has been running in lotus since v1.16.0 and up, and the new FEVM does not increase any node hardware spec requirement.

With FEVM on Filecoin, we aim to provide full compatibility with the existing EVM ecosystem and its tooling out of the box. 
Consequently, lotus now provides a full set of [Ethereum-styled APIs](https://github.com/filecoin-project/lotus/blob/release/v1.20.0/node/impl/full/eth.go) for developers and token holders to interact with the Filecoin network as well.
For full documentation on this new tooling, please see the [Lotus docs website](https://lotus.filecoin.io/lotus/configure/ethereum-rpc/).

**Enabling Ethereum JSON RPC API**

Note that Ethereum APIs are only supported in the lotus v1 API, meaning that any node operator who wants to enable Eth API services must be using the v1 API, instead of the v0 API. To enable Eth RPC, simply set `EnableEthRPC` to `true` in your node config.toml file; or set env var `LOTUS_FEVM_ENABLEETHRPC` to `1` before starting your lotus node.

**Eth tx hash and Filecoin message CID**

Most of the Eth APIs take Eth accounts and tx has as an input, and they start with `0x` , and that is what Ethereum tooling support. However, in Filecoin, we have Filecoin account formats where things start with `f` (`f410` specifically for eth accounts on Filecoin) and the messages are in the format of CIDs. To enable a smooth developer experience, Lotus internally converts between Ethereum address and Filecoin account address as needed. In addition, lotus also keeps a Eth tx hash <> Filecoin message CID map and stores them in a SQLite database as node sees a FEVM messages. The database is initiated and the maps are populated  automatically in `~/<lotus_repo>/sqlite/txhash.db` for any node that as Eth RPC enabled. Node operators can configure how many historical mappings they wanna store by configuring `EthTxHashMappingLifetimeDays` .

**Events**

[FIP-0049 introduces actor events](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0049.md) that can be emitted and externally observable during message execution. An `events.db` is created automatically under `~/<lotus_repo>/sqlite` to store these events if the node has Eth RPC enabled. Node operators can configure the events support base on their needs by configuration `Events` configurations.

Note: All three features are new, and we welcome user feedback, please create an issue if you have any enhancements that you’d like to see!

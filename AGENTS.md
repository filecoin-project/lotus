# Lotus agent guide

Lotus is the Go implementation of a Filecoin node, miner and worker. It is consensus-critical software used by long-lived production systems.

Current code, CI and the linked repository documentation are authoritative.

## Where Lotus sits in Filecoin

Lotus is one of two node implementations; [Forest](https://github.com/ChainSafe/forest) (Rust) is the other. Both must compute identical state from the same chain, but reach the shared Rust code by different routes, so a mismatch between them is a consensus fork, whichever side is wrong.

- [builtin-actors](https://github.com/filecoin-project/builtin-actors) (Rust, compiled to WASM) defines actor behaviour. Lotus embeds its release bundles in `build/actors/` and maps them in `build/builtin_actors_gen.go`; Forest loads the same bundles. Changing compiled actor output changes bundle CIDs and gas, so it ships only in a network upgrade.
- [go-state-types](https://github.com/filecoin-project/go-state-types) is the Go mirror of builtin-actors' state, parameter and return types, and holds the Lotus-side state migrations (`builtin/vN/migration/`), which Lotus wires in `chain/consensus/filcns/upgrades.go`. Forest writes its own migrations in Rust; both must produce the same state root.
- [filecoin-ffi](https://github.com/filecoin-project/filecoin-ffi) is the cgo bridge to [ref-fvm](https://github.com/filecoin-project/ref-fvm) and [rust-fil-proofs](https://github.com/filecoin-project/rust-fil-proofs). It builds from the `extern/filecoin-ffi` submodule: `go.mod` replaces the module with that path, so `go get` doesn't change what Lotus builds. Modules that import Lotus, such as Curio, ignore that `replace` and build the `require` version, so bump the submodule pointer and the `require` together. ref-fvm and proofs changes reach Lotus only through an ffi release; Forest takes ref-fvm crates directly.
- From network version 16 messages execute in the FVM through ffi (`chain/vm/vmi.go`). `chain/vm/vm.go` and `invoker.go` are the legacy Go VM for earlier epochs, which also keeps the `specs-actors` v0 to v8 dependencies alive. Gas charged during FVM execution is priced in ref-fvm, so changing it spans ref-fvm, ffi and a network version; `vm.PricelistByEpoch` (`chain/vm/gas.go`) still prices Lotus-side checks such as block validation and mpool minimum gas.
- [go-hamt-ipld](https://github.com/filecoin-project/go-hamt-ipld), [go-amt-ipld](https://github.com/filecoin-project/go-amt-ipld) and [go-kamt-ipld](https://github.com/filecoin-project/go-kamt-ipld) must match ref-fvm's Rust `ipld/` crates bit for bit. [cbor-gen](https://github.com/whyrusleeping/cbor-gen) and [go-jsonrpc](https://github.com/filecoin-project/go-jsonrpc) generate Lotus's CBOR codecs and serve its API.
- [Curio](https://github.com/filecoin-project/curio) imports Lotus as a Go module and calls its APIs. Exported packages (`api/`, `chain/types/`, `lib/`) and API behaviour, including error text, are a library surface for Curio and other tools.

When a change crosses one of these boundaries, check the other side. If a sibling checkout is available, read its `AGENTS.md` before changing a shared contract.

## Orientation

- Nodes are assembled with Fx dependency injection; follow providers from `node/builder.go` and `node/builder_chain.go`.
- Tipset execution is `TipSetExecutor.ApplyBlocks` in `chain/consensus/compute_state.go`.
- `storage/` contains sealing and proving. Storage and retrieval deal-making have moved to projects such as Boost and Curio; don't recreate them here.
- Protocol changes require a Filecoin Improvement Proposal before implementation.

### Before changing or reviewing these, read

- `chain/index/`: `chain/index/AGENTS.md`
- `chain/store/` height lookups: `chain/store/AGENTS.md`
- `chain/messagepool/`: `chain/messagepool/AGENTS.md`
- `gateway/` or eth RPC range, lookback and rate limits: `gateway/AGENTS.md`
- JSON-RPC error codes, messages or `data`: `api/AGENTS.md`
- Node assembly, startup hooks or lite mode: `node/AGENTS.md`
- Integration tests, CI runners or `conformance/`: `itests/AGENTS.md`
- Integrating builtin-actors changes or bundles: `documentation/misc/Builtin-actors_Development.md`
- Bumping ffi, go-state-types or a bundle: `documentation/misc/Update_Dependencies_Lotus.md`
- Adding a network version, actors version or upgrade height: `documentation/misc/Building_a_network_skeleton.md`
- Finality, `safe` or `finalized` tag resolution: `documentation/en/finality.md`
- Releases and backports: `documentation/misc/RELEASE_ISSUE_TEMPLATE.md` and `cmd/release/README.md`

## Correctness constraints

- Consensus and state-transition code must be deterministic across nodes. Never let map iteration order, local wall-clock time, host state or nondeterministic I/O affect consensus output.
- Filecoin has null rounds and chain reorganisations. Do not assume every epoch has a tipset or that an observed head is permanent. Reorg-aware components must handle both apply and revert paths.
- Tipset execution is deferred: messages in a tipset are executed while its child is produced, and that child commits the resulting state and receipts. Check `chain/stmgr` and existing API semantics before changing state or receipt lookups.
- Gate consensus behaviour at the correct network version or upgrade epoch, and test immediately before and after the boundary. Upgrade heights must be strictly increasing (`chain/stmgr/forks.go`), so each mainnet `LOTUS_DISABLE_<UPGRADE>` switch in `build/buildconstants/params_mainnet.go` must also push every later upgrade height out, in order; extend the earlier switches when adding an upgrade.
- Preserve on-disk and wire compatibility unless the change includes a migration or version transition. Types written with cbor-gen tuple encoders (`gen/main.go`) encode fields by position, so reordering, adding or removing a field changes the format.
- Operators run nodes you can't see. Archival nodes, RPC providers, exchanges and indexers keep databases built by older releases, often holding far more history than a default node. Ask what a database written by the previous few releases looks like under the new code, and whether its repair path is atomic, restart-safe, reorg-safe, proportional to an archival node (hours or days, not months) and able to run online.
- Restarts empty in-memory caches. Operators restart to upgrade, to run offline tools and after crashes; a request path that is fast on a warm process can time out cold. A new cache dependency in a request path needs a cold-start story.
- Actor events begin at FEVM (mainnet 2683348, calibnet 322354), and many archival nodes hold state only from there. Default event and index backfill bounds to FEVM and to the lowest epoch with state, not to genesis.
- Bound work and memory derived from RPC or network input. Validate CIDs, lengths and counts before caching or persisting data. Compare request cost with downstream CPU, memory, I/O and chain traversal, including concurrent amplification: a syntactically small request must not trigger attacker-controlled replay, state walks, scans or unbounded results.
- `gateway/` is a security and resource-control boundary commonly exposed to the internet, not a transparent pass-through. Its rate-limit tokens are coarse admission control; bound the work inside each request too.
- Respect blockstore roles and keep the typed blockstores in `node/modules/dtypes` distinct. `ExposedBlockstore` is deliberately isolated from internal caches; bypassing its cache-invalidation rules can serve stale or inconsistent data.
- `api/api_full.go` is the stable v1 full-node API, `api/v0api/` is legacy and `api/v2api/full.go` is experimental. Preserve v0/v1 compatibility: put new or changed APIs in v2, and edit v0/v1 only for a required compatibility change or bug fix. An API change usually spans the interface, permissioned surface, implementation, version wrappers, gateway proxy, mocks and generated OpenRPC documentation; trace the complete call path.
- Every goroutine needs an owner and a cancellation path that runs at shutdown. Avoid blocking channel sends while holding a lock.
- Dependencies can add on-disk state. Lotus hands its metadata datastore to libp2p components, so a release that adds persistence starts writing to the node's repo even though Lotus code didn't change. Read release notes of libp2p and other datastore-holding dependencies for new persistent state and defaults.

## Build and test

Use the Go version declared by `go.mod` and `GO_VERSION_MIN`; do not copy a version number into new documentation or automation.

`make lotus` builds the node, fetching or building ffi first; prerequisites are in `README.md`. Iterate with the smallest relevant test (`go test ./path/to/package -run TestName`) and report the checks you didn't run.

Build tags select network constants. When changing network-specific behaviour, test every affected configuration, for example `-tags=calibnet` or `-tags=2k`, in addition to default mainnet.

### Integration tests

- CI runs each itest file as its own package (`go test ./itests/foo_test.go`), so a helper in one `_test.go` file is undefined in another. `go test ./itests/...` compiles them together and hides this. Shared helpers go in `itests/kit/`; run an itest the way CI does.
- Add new tests to the existing `itests/*_test.go` file whose class of behaviour they exercise. Each file is its own CI job, so a new file adds a job and its setup cost; check job times and runners in `itests/AGENTS.md` first.
- Tests marked expensive are skipped locally unless `LOTUS_RUN_EXPENSIVE_TESTS=1` is set. Null rounds can make height assumptions flaky; discover actual tipsets or use the established slower block-time pattern. Read the node logs above a generic harness assertion before diagnosing a failure.

## Generation and validation

Do not hand-edit generated code; change its source and regenerate. Not all of it is marked:

- `make actors-gen` writes `chain/actors/builtin/**` (`<actor>.go`, `vN.go`, `messageN.go`), `chain/actors/policy/policy.go`, `builtin.go` and `registry.go` from `*.go.template` files and `gen/inlinegen-data.json`. These files aren't marked "Code generated".
- `gen/inline-gen` rewrites the regions between `/* inline-gen start */` and `/* inline-gen end */` inside hand-written files such as `chain/actors/version.go`, `build/buildconstants/params.go` and `itests/kit/ensemble_opts_nv.go`. Both `make actors-gen` and `make gen` run it.
- API interfaces, CBOR types, actor metadata or configuration: `make gen`
- CLI commands or configuration docs: `make docsgen-cli`
- New API types may also need example values in `api/docgen/docgen.go` for OpenRPC generation.

Before handing off a change:

```sh
go fmt ./path/to/changed/package/...
make fiximports
make lint
make unittests            # when the change warrants the full unit suite
```

`make lint` runs `go mod tidy`, `go vet ./...` and the configured `golangci-lint` suite. CI reruns generation, formatting, module tidiness and linting and fails if they produce a diff.

## Change hygiene

Add tests that would fail for a plausible regression. Report suspected vulnerabilities privately as `SECURITY.md` describes, never in a public issue or PR.

### Comments, docs and commit messages

- Write code comments and reference docs year-zero: describe the code as it is, as if it had always been this way. Migration guides and release notes may compare old and new behaviour. History (old behaviour, the bug, the fix, the issue) goes in the commit message and PR description. In comments, words like "now", "previously", "used to", "no longer", "instead of" and "fixed" signal change narration; rewrite or delete. A link that explains a constraint that still applies stays ("remove once go#12345 ships").
- A comment, in code or tests, earns its place by stating what a maintainer can't read from the code: intent, an invariant, a compatibility constraint, a surprising cost. Don't restate the code, the test name or the assertion; prefer one precise sentence. API docs state the contract: inputs, outputs, errors, bounds.
- Commit messages and PR descriptions explain what changed and why, for a reviewer. Skip summaries of the diff, boilerplate sections and claims you haven't verified.

### Commits and pull requests

Use Conventional Commits form, `<type>(<scope>): <description>`, for commit subjects and PR titles. CI enforces the PR title. Allowed types are `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style` and `test`; the scope is optional. Use a commit body when the motivation, constraints or operational consequences do not fit in the subject.

Prefer released dependency versions. Exceptions require the `dependency-check-ignore` rationale documented beside the version in `go.mod`.

### Release notes

Don't edit `CHANGELOG.md` outside release PRs. The PR title is the changelog line: name the affected API, command or component and what changes for users, and mark breaking changes with `!`. Labels and `## Release note` blocks: [Release Notes](CONTRIBUTING.md#release-notes).

Node and miner release separately (`NodeBuildVersion` and `MinerBuildVersion` in `build/version.go`, tags `vX` and `miner/vX`, branches `release/vX` and `release/miner/vX`). Once a release branch has diverged from master, a fix reaches it only through the `release/backport` label and a cherry-pick.

## Reviewing changes

Read the relevant callers, interfaces and tests before commenting, and verify assumptions against current code; Lotus has intentional patterns that look unusual in isolation.

Priorities, in order: consensus and state-transition correctness; untrusted-input bounds at RPC, gateway and chain-exchange boundaries; data loss or corruption; API compatibility and completeness; concurrency and goroutine lifecycle; tests, generated output and PR titles. The constraints above define each.

Comment only on a concrete, plausible failure the change introduces: the triggering input or state, the wrong result, its consequence. Skip style preferences, speculative hardening, pre-existing problems and refactors without behavioural benefit. One exception: this repository treats comments that narrate the change or restate the code as defects, so report them (see Comments, docs and commit messages). For generated files, name the source or generator that must change.

## Maintaining agent context

This file and the `AGENTS.md` files it lists are maintained with the code. When a change makes one of them wrong, fix it in the same PR. When a change alters a contract with a neighbouring repository, name the follow-up that repository needs, including its `AGENTS.md`, in the PR description.

Each `AGENTS.md` has a sibling `CLAUDE.md` containing only `@AGENTS.md`, because by default Claude Code skips `AGENTS.md` files whenever a `CLAUDE.md` exists at or above its working directory. Add the pair together; put content in `AGENTS.md`, never in the `CLAUDE.md`.

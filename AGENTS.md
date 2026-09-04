# Lotus agent guide

Lotus is the Go implementation of a Filecoin node, miner and worker. It is consensus-critical software used by long-lived production systems. Prefer narrow, unsurprising changes whose behaviour is explicit and testable.

Current code, CI and the linked repository documentation are authoritative. If this file disagrees with them, follow the source and update this file.

## Scope and architecture

- `cmd/` contains binary entry points. User-facing command implementations mostly live in `cli/`.
- `api/api_full.go` defines the stable v1 full-node API; `api/v0api/` is legacy and `api/v2api/` is experimental. Preserve v0/v1 compatibility and normally put new or changed APIs in v2. Edit v0/v1 only when the task specifically requires a compatibility change or bug fix.
- `node/` assembles nodes with Fx dependency injection. Follow providers from `node/builder.go` and `node/builder_chain.go`; implementations live under `node/impl/`.
- `chain/` contains chain sync and validation, consensus, state management, VM execution, the message pool, F3 integration and core chain types.
- `blockstore/` and `chain/store/` contain persistent chain data and snapshot logic. Preserve the distinction between typed blockstores in `node/modules/dtypes`.
- `storage/` contains sealing and proving. Storage and retrieval deal-making implementations have moved to projects such as Boost and Curio; do not recreate them here.
- `gateway/` implements the curated v1 and v2 reverse proxies commonly deployed in front of a full node's RPC. It is a security and resource-control boundary, not a transparent pass-through.
- `build/` contains network parameters, upgrade configuration and embedded actor bundles. Network-specific constants are selected with build tags.
- `itests/` contains resource-intensive integration tests and `itests/kit/` contains the test-node harness.
- `extern/` contains required git submodules, including `filecoin-ffi` and test vectors.

Protocol changes require a Filecoin Improvement Proposal before implementation. Builtin actor behaviour is developed in `filecoin-project/builtin-actors`; read `documentation/misc/Builtin-actors_Development.md` when integrating actor changes.

## Correctness constraints

- Consensus and state-transition code must be deterministic across nodes. Never let map iteration order, local wall-clock time, host state or nondeterministic I/O affect consensus output.
- Filecoin has null rounds and chain reorganisations. Do not assume every epoch has a tipset or that an observed head is permanent. Reorg-aware components must handle both apply and revert paths.
- Tipset execution is deferred: messages in a tipset are executed while its child is produced, and that child commits the resulting state and receipts. Check `chain/stmgr` and existing API semantics before changing state or receipt lookups.
- Gate consensus behaviour at the correct network version or upgrade epoch, and test immediately before and after the boundary.
- Preserve on-disk and wire compatibility unless the change explicitly includes a migration or protocol/version transition. Database changes must be atomic, restart-safe and reorg-safe where applicable.
- Bound work and memory derived from RPC or network input. Validate CIDs, lengths and counts before caching or persisting data. Compare request cost with downstream CPU, memory, I/O and chain traversal: a syntactically small request must not trigger attacker-controlled replay, state walks, scans or unbounded results.
- Respect blockstore roles. `ExposedBlockstore` is deliberately isolated from internal caches; bypassing its cache-invalidation rules can serve stale or inconsistent data.
- API changes usually span the interface, permissioned surface, implementation, version wrappers, gateway proxy, mocks and generated OpenRPC documentation. Trace the complete call path. For v2, start at `api/v2api/full.go`; for existing v1 behaviour, start at `api/api_full.go`.
- Gateway methods must preserve `gateway/`'s layered controls: the curated proxy surface, weighted basic/wallet/chain/state rate-limit tokens, global and per-connection limiting, and method-specific lookback, confidence, filter, subscription and trace-range bounds. Rate-limit tokens are coarse admission control, not a substitute for bounding the work inside one request.
- Every goroutine needs an owner, cancellation path and defined shutdown behaviour. Avoid blocking sends while holding locks, leaked subscriptions and unbounded queues.

## Build and test

Use the Go version declared by `go.mod` and `GO_VERSION_MIN`; do not copy a version number into new documentation or automation.

In a fresh checkout, initialise submodules and the proofs dependency first:

```sh
make deps
make lotus       # node only
make all         # lotus, lotus-miner and lotus-worker
```

Prefer the smallest meaningful test while iterating:

```sh
go test ./path/to/package
go test -run TestName ./path/to/package
go test -v -count=1 -run TestName ./itests/example_test.go
```

Integration tests are intentionally run as individual files. Tests marked expensive are skipped locally unless `LOTUS_RUN_EXPENSIVE_TESTS=1` is set. Null rounds can make height assumptions flaky; discover actual tipsets or use the established slower block-time pattern. Read the node logs above a generic harness assertion before diagnosing a failure.

Build tags select network constants. When changing network-specific behaviour, test every affected configuration, for example `-tags=calibnet` or `-tags=2k`, in addition to default mainnet.

## Generation and validation

Do not hand-edit files marked as generated. Regenerate from the source definition:

- API interfaces, CBOR types, actor metadata or configuration: `make gen`
- CLI commands or configuration docs: `make docsgen-cli`
- New API types may also need example values in `api/docgen/docgen.go` for OpenRPC generation.
- Adding or removing an integration-test file requires updating the explicit runner assignments and tests in `cmd/ci/`.

Before handing off a change:

```sh
go fmt ./path/to/changed/package/...
make fiximports
make lint
make unittests            # when the change warrants the full unit suite
```

`make lint` runs `go mod tidy`, `go vet ./...` and the configured `golangci-lint` suite. CI reruns generation, formatting, module tidiness and linting and requires no resulting diff. Run targeted tests for the changed behaviour even when the full suite is impractical.

## Change hygiene

Follow `CONTRIBUTING.md` and existing local patterns. Avoid unrelated cleanup and subjective rewrites. Add tests that would fail for a plausible regression.

### Commits and pull requests

Use Conventional Commits form, `<type>(<scope>): <description>`, for commit subjects and PR titles. CI enforces the PR title. Allowed types are `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style` and `test`; the scope is optional. Use a commit body when the motivation, constraints or operational consequences do not fit in the subject.

Prefer released dependency versions. Exceptions require the `dependency-check-ignore` rationale documented beside the version in `go.mod`.

### Changelog

Add a `CHANGELOG.md` entry when operators, users or downstream developers need to know about the change: new or changed behaviour, bug fixes, compatibility changes, API or CLI changes, configuration changes, security or performance consequences, or changed build and runtime requirements.

Do not add changelog noise for tests, documentation, CI, internal refactors, generated churn or maintenance with no observable effect. For these changes, a maintainer or administrator can apply the `skip/changelog` label; the documented `[skip changelog]` PR-body marker is also accepted by CI.

Put entries under the appropriate `# UNRELEASED` subsection: Upgrade Warnings for required operator action or serious compatibility concerns, otherwise New Features, Bug Fixes or Improvements. Describe the observable effect and any action users must take.

End each entry with the eventual PR link in this form:

```md
([filecoin-project/lotus#12345](https://github.com/filecoin-project/lotus/pull/12345))
```

Prediction hint: issues and PRs share one sequence; fetch `GET /repos/filecoin-project/lotus/issues?state=all&sort=created&direction=desc&per_page=1` and add one to `.[0].number`.

Before the PR exists, either use a carefully predicted next number or add the link after opening it. Verify every predicted number before merge; do not leave placeholders or links to another PR.

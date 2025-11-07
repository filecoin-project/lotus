# EIP-7702 EthAccount + ref-fvm Migration Plan

This document defines the current work priority for EIP-7702 across builtin-actors, ref-fvm, and Lotus. It moves delegation state to the EthAccount actor and implements delegation execution semantics in ref-fvm, keeping the EVM interpreter intact.

## Context

- Working development branch; no backward-compatibility constraints.
- Delegation mapping is currently local to the EVM actor; cross-contract visibility test fails.
- EVM interpreter implements delegated execution and EXTCODE* pointer semantics today.

## Goals

- Persist delegation state per EOA in EthAccount so it is globally visible.
- Implement delegated execution and pointer code behavior in the VM (ref-fvm).
- Preserve end-to-end behavior: atomic ApplyAndCall, depth=1, EXTCODE* projects 23-byte pointer code, Delegated(address) event with authority, revert-data propagation, value-transfer behavior, and behavioral gas model in Lotus.

## Design

### State in EthAccount

- Add to EthAccount state:
  - `delegate_to: Option<EthAddress>` — current delegate pointer for the EOA.
  - `auth_nonce: u64` — per-authority nonce for authorization tuples.
  - `evm_storage_root: Cid` — persistent storage root used when executing under authority context.
- These fields replace per-authority maps previously held in the EVM actor.

### VM (ref-fvm) Delegation Semantics

- Pointer code projection (EXTCODE*):
  - On EXTCODESIZE/HASH/COPY for an EOA with `delegate_to != None`, return a virtual 23-byte code image: `0xEF 0x01 0x00 || <delegate(20)>`.
  - EXTCODEHASH returns keccak(pointer_code), size is 23, copy returns the exact 23 bytes.
- Delegated dispatch for CALL to EOAs:
  - If target is an EOA with `delegate_to`, transfer value to the authority; fail outer call if transfer fails.
  - Execute the delegate contract’s EVM bytecode under an authority context with depth limit=1.
  - Mount the authority’s `evm_storage_root` during execution and persist the updated root back to EthAccount on exit.
  - Propagate returndata on success; on revert, set status=0 and surface revert bytes.
  - Emit `Delegated(address)` with the authority address (best-effort; may drop under gas tightness).
- Authority context rules:
  - Do not re-follow delegation (depth limit enforced by a context flag).
  - SELFDESTRUCT is a no-op in authority context (no balance move or tombstone effect).

### ApplyAndCall in EthAccount

- Method `ApplyAndCall` validates tuples and updates state, then invokes a VM syscall to execute the outer call with all gas forwarded.
- Tuple validation (consensus-critical): domain separator 0x05; `yParity in {0,1}`; non-zero R/S; ≤32-byte R/S accepted with left-padding; low-s enforced; nonce equality; tuple cap (64 placeholder).
- Pre-existence check: reject if the authority resolves to an EVM contract actor.
- Returns embedded status and return data; mapping and nonce updates persist even if outer call reverts.

### Interfaces (proposed)

- VM syscall invoked by EthAccount.ApplyAndCall (pseudo-signature):
  - `fn evm_apply_and_call(authority: EthAddress, to: EthAddress, value: TokenAmount, input: Bytes) -> (status: u8, returndata: Bytes)`
  - Semantics: forwards all remaining gas to the outer call; performs delegated dispatch/pointer semantics per rules above; returns status and returndata without undoing prior state updates.
- Event topic constant: `keccak("Delegated(address)")`.
- Pointer code constants: `MAGIC=0xEF01`, `VERSION=0x00` → 23-byte image: `0xEF 0x01 0x00 || delegate(20)`.
- EthAccount.ApplyAndCall params (CBOR wrapper, canonical):
  - `[ [ tuples... ], [ to(20), value(u256), input(bytes) ] ]`.
  - Tuple fields validated with domain `0x05` hash preimage for signature checks.

## ref-fvm Changes (new branch)

- Branch: `eip7702`.
- Implement VM features:
  - EXTCODE* pointer projection for EOAs with `delegate_to`.
  - Delegated dispatch for CALL to EOAs with authority context + storage overlay + depth limit.
  - Event emission for `Delegated(address)` with authority address.
  - Syscall invoked by EthAccount.ApplyAndCall to perform the outer call with all gas forwarded and return status/returndata.
  - Context plumbing to prevent re-following delegation and to manage storage root mount/restore.
- Tests in ref-fvm:
  - Pointer projection (size/hash/copy), delegated dispatch success/revert, depth limit, SELFDESTRUCT no-op, revert-data propagation, value-transfer short-circuit.

### Repo/Path Impact (ref-fvm)

- Execution layer:
  - EVM call entry path: intercept CALL→EOA; implement authority-context dispatch.
  - Code query path: intercept EXTCODE{SIZE,HASH,COPY} for EOAs; provide virtual pointer bytes/hash/size.
- Context plumbing:
  - Execution context flag for authority mode; prevent delegation re-follow.
  - Storage overlay: mount EthAccount.evm_storage_root and persist updated root on exit.
- Events:
  - Emit `Delegated(address)` after delegated outer call (best-effort).

## builtin-actors Changes

- EthAccount:
  - Extend state with `delegate_to`, `auth_nonce`, `evm_storage_root`.
  - Implement `ApplyAndCall`:
    - Validate tuples and nonces.
    - Update state (delegate_to + auth_nonce; initialize storage root if needed).
    - Invoke ref-fvm syscall for the outer call; return embedded status + return data.
- EVM actor:
  - Remove internal delegation/nonces/storage-root structures and InvokeAsEoa.
  - Keep the interpreter intact; do not alter EXTCODE* — the VM supplies pointer behavior.
- Tests:
  - Global mapping test passes (eoa_pointer_mapping_global.rs).
  - Existing suites for pointer semantics, revert-data, depth limit, pre-existence, R/S padding, tuple cap keep passing through EthAccount + VM path.

### Repo/Path Impact (builtin-actors)

- EthAccount actor:
  - State struct: add `delegate_to`, `auth_nonce`, `evm_storage_root`.
  - Methods: add `ApplyAndCall` entry; tuple validation and state updates.
- EVM actor:
  - Remove/stop using: internal delegation/nonces/storage-roots; InvokeAsEoa; any EXTCODE* delegation lookups.
  - Interpreter remains; storage and bytecode handling unchanged.

## Lotus Changes

- Route type-0x04 transactions to `EthAccount.ApplyAndCall` (canonical CBOR wrapper parameters unchanged).
- Receipts/logs:
  - Continue deriving `delegatedTo` from `authorizationList` or the synthetic `Delegated(address)` event.
  - Receipt `status` mirrors embedded status returned by `ApplyAndCall`.
- Gas estimation:
  - Keep behavioral model: tuple overhead applied when `authorizationList` is non-empty and grows with tuple count; do not pin numeric constants.

### Repo/Path Impact (Lotus)

- node/impl/eth:
  - Send path: point 0x04 to EthAccount.ApplyAndCall method number.
  - Receipts: keep adjuster for `Delegated(address)` topic and `authorizationList` attribution; status from embedded status.
- chain/types/ethtypes:
  - No shape changes; keep canonical wrapper encode/decode and `AuthorizationKeccak` tests.

## Sequencing

1. Authoritative plan: land this document and mark as current priority in AGENTS.md.
2. Create ref-fvm branch and scaffold pointer projection, authority context flags, and ApplyAndCall syscall.
3. Implement EthAccount state and `ApplyAndCall`; remove delegation logic from EVM actor.
4. Implement ref-fvm delegated dispatch, storage overlay, event emission, and EXTCODE* projection.
5. Switch Lotus routing for 0x04 to EthAccount.ApplyAndCall; keep receipts/gas behavior.
6. Validate across repos and stabilize tests; then clean dead code paths.

## Testing Migration Matrix (Coverage)

This matrix maps existing builtin-actors EVM tests to expected coverage post-migration and adds ref-fvm unit tests where behavior moved to the VM.

- Pointer semantics
  - actors/evm/tests/eoa_call_pointer_semantics.rs → remains; backed by VM EXTCODE* projection.
  - ref-fvm: add unit tests for EXTCODESIZE=23, EXTCODECOPY bytes match, EXTCODEHASH(pointer_code).
- Global mapping visibility
  - actors/evm/tests/eoa_pointer_mapping_global.rs → remains; now passes using EthAccount state.
  - ref-fvm: add test to verify cross-contract EXTCODE* sees pointer code for any EOA with delegate.
- Delegated CALL behavior
  - actors/evm/tests/delegated_call_revert_data.rs → remains; VM must propagate revert data to return buffer + memory.
  - actors/evm/tests/apply_and_call_value_transfer.rs → remains; VM short-circuits on failed transfer.
- Atomic ApplyAndCall
  - actors/evm/tests/apply_and_call_atomicity_* → port to EthAccount.ApplyAndCall; assert persistence on revert.
  - ref-fvm: syscall tests ensure status/returndata returned per VM dispatch.
- Depth limit
  - actors/evm/tests/apply_and_call_depth_limit.rs → remains; VM prevents delegation re-follow under authority context.
- SELFDESTRUCT no-op under authority
  - actors/evm/tests/apply_and_call_selfdestruct.rs → remains; VM enforces no-op and state/balance invariants.
- Storage isolation/persistence
  - actors/evm/tests/delegated_storage_isolation.rs, delegated_storage_persistence.rs → remain; VM must mount and persist `evm_storage_root` for authority.
- Validation vectors
  - actors/evm/tests/apply_and_call_invalids.rs (domain, yParity, zero/over-32 R/S, high-S, nonce mismatch, duplicates, tuple cap) → rewire to EthAccount.ApplyAndCall; behavior unchanged.
  - actors/evm/tests/apply_and_call_tuple_roundtrip.rs, apply_and_call_tuple_cap.rs → remain.
- Pre-existence policy
  - actors/evm/tests/apply_and_call_delegate_no_code.rs → remains; EthAccount rejects authorities resolving to EVM contracts.
- NV gating (none)
  - actors/evm/tests/eoa_invoke_delegation_nv.rs → remains; ensure no runtime gating.

Lotus tests (no loss of coverage):

- chain/types/ethtypes: parsing/encoding, RLP per-type limit, canonical wrapper, AuthorizationKeccak vectors.
- node/impl/eth: receipts attribution from tuples and `Delegated(address)`; gas estimation behavioral tests; status decode from embedded.
- itests (post-bundle): E2E mirrors geth TestEIP7702; applies two delegations, CALL→EOA executes delegate, storage updates under authority, receipt.status reflects outcome, attribution present.

ref-fvm tests (new):

- Pointer projection (EXTCODE*); delegated dispatch success and revert with returndata; depth limit; SELFDESTRUCT no-op; failed transfer short-circuit; authority storage overlay mount/persist.

## Validation

- builtin-actors:
  - `cargo test -p fil_actor_evm` including pointer semantics, delegated revert data, depth limit, storage isolation/persistence, pre-existence, tuple cap, R/S padding.
  - Add EthAccount ApplyAndCall tests to cover nonce initialization, duplicates, invalid domain/yParity/high-s/zero R/S, wrong chain id.
- ref-fvm:
  - Unit tests for EXTCODE* pointer projection, delegated dispatch, depth limit, SELFDESTRUCT no-op, revert-data propagation, and value-transfer handling.
- Lotus:
  - `go test` suites for 7702 parsing/encoding, receipts attribution, behavioral gas estimation.
  - E2E once bundles include ref-fvm changes: apply delegations, CALL→EOA executes delegate, storage persists under authority, event emitted, receipts correct.

## Execution Checklist

- ref-fvm (branch `eip7702`)
  - [ ] EXTCODE* pointer projection for EOAs with delegate
  - [ ] CALL→EOA delegated dispatch with authority context and depth limit
  - [ ] Authority storage overlay mount/persist
  - [ ] `Delegated(address)` event emission
  - [ ] ApplyAndCall syscall with all-gas-forwarded semantics
  - [ ] Unit tests for features above
- builtin-actors
  - [ ] EthAccount: add `delegate_to`, `auth_nonce`, `evm_storage_root`
  - [ ] EthAccount: implement `ApplyAndCall` validation + state updates + syscall
  - [ ] Remove delegation maps and InvokeAsEoa from EVM actor
  - [ ] Adapt tests to EthAccount.ApplyAndCall; keep coverage green
- Lotus
  - [ ] Route 0x04 to EthAccount.ApplyAndCall
  - [ ] Receipts attribution and status decode intact
  - [ ] Behavioral gas estimation unchanged

## Risks & Mitigations

- Risk: Pointer semantics regress under VM projection.
  - Mitigation: Add ref-fvm EXTCODE* tests and keep existing actor tests.
- Risk: Storage overlay bugs cause state loss/corruption.
  - Mitigation: Add persistence/isolation tests; verify root before/after.
- Risk: Revert-data propagation differences between VM and actor path.
  - Mitigation: Dedicated tests for RETURNDATASIZE/RETURNDATACOPY semantics.
- Risk: Event emission dropped under gas tightness.
  - Mitigation: Document best-effort; ensure outer call status unaffected; test both with/without sufficient gas.

## Out of Scope

- Numeric gas constants/refunds finalization (Lotus remains behavioral; actor refunds staged separately).
- Mempool policy changes (none required).

## Acceptance Criteria

- Delegation state and nonces are per-EOA in EthAccount and persist across transactions.
- EXTCODESIZE/HASH/COPY on delegated EOAs exposes a 23-byte pointer code globally.
- CALL to delegated EOAs executes the delegate’s code in authority context with a depth limit of 1 and authority storage overlay; SELFDESTRUCT no-op.
- `ApplyAndCall` persists mapping + nonces pre-call and returns embedded status + return data; receipts/logs attribute `delegatedTo` as specified.
- Behavioral gas tests in Lotus remain green; no special mempool policies.

## Ownership & Areas

- ref-fvm: VM pointer projection, delegated dispatch, authority-context storage overlay, events, ApplyAndCall syscall.
- builtin-actors: EthAccount state + `ApplyAndCall` validation and state updates; remove EVM-actor delegation code.
- Lotus: 0x04 routing to EthAccount.ApplyAndCall; receipts attribution and behavioral gas estimation.

## Branch Strategy

- ref-fvm: `eip7702`.
- builtin-actors and Lotus: continue on current `eip7702` branches; no backward-compatibility constraints.

## Quick Commands

- builtin-actors: `cargo test -p fil_actor_evm`
- Lotus:
  - `go build ./chain/types/ethtypes`
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./node/impl/eth -run 7702 -count=1`

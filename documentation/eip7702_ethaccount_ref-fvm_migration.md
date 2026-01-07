# EIP-7702 EthAccount + ref-fvm Migration Plan

This document defines the current work priority for EIP-7702 across builtin-actors, ref-fvm, and Lotus. It moves delegation state to the EthAccount actor and implements delegation execution semantics in ref-fvm, keeping the EVM interpreter intact.

## Context

- Working development branch; no backward-compatibility constraints.
- Delegation mapping and nonces now live per-EOA in EthAccount state; the deprecated EVM-local delegation map has been removed.
- Delegated execution and EXTCODE* pointer semantics are implemented in ref-fvm via a VM intercept; the EVM interpreter no longer follows delegation internally.

## Goals

- Keep delegation state per EOA in EthAccount so it is globally visible.
- Maintain delegated execution and pointer code behavior in the VM (ref-fvm) as the single source of truth.
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
- Delegated dispatch for CALL to EOAs (implemented):
  - Intercept EVM `InvokeEVM` → EthAccount target when the EthAccount has `delegate_to` set.
  - Value transfer: the call manager transfers value to the authority prior to interception; on transfer failure the EVM call reports success=0 via `EVM_CONTRACT_REVERTED`.
  - Execution: ref‑fvm invokes the caller EVM actor via a private trampoline `InvokeAsEoaWithRoot` with parameters `(code_cid, input, caller, receiver, value, initial_storage_root)`:
    - The EVM actor mounts `initial_storage_root`, sets authority context (depth=1), executes delegate code, and returns `(output_data, new_storage_root)` or reverts with the revert payload.
  - Persistence: the call manager persists `new_storage_root` back into the EthAccount state (`evm_storage_root`). Persistence occurs only on success; revert and value‑transfer short‑circuit paths do not update the authority overlay.
  - Return mapping: on success, returns `ExitCode::OK` with raw `output_data` for the EVM interpreter; on revert, returns `EVM_CONTRACT_REVERTED` with the revert payload.
  - Event: emits `Delegated(address)` (topic keccak("Delegated(address)")) with the authority encoded as a 32‑byte ABI word. Emission is best‑effort.
- Authority context rules:
  - Do not re-follow delegation (depth limit enforced by a context flag).
  - SELFDESTRUCT is a no-op in authority context (no balance move or tombstone effect).
  - Success/revert mapping: delegated subcalls map to standard EVM CALL semantics. On success, return ExitCode::OK and
    returndata. On revert, return EVM_CONTRACT_REVERTED with revert payload as data; the EVM interpreter will set
    success=0 and propagate returndata via RETURNDATASIZE/RETURNDATACOPY.

### ApplyAndCall in EthAccount

- Method `ApplyAndCall` validates tuples and updates state, then invokes a VM syscall to execute the outer call with all gas forwarded.
- Tuple validation (consensus-critical): domain separator 0x05; `yParity in {0,1}`; non-zero R/S; ≤32-byte R/S accepted with left-padding; low-s enforced; nonce equality; tuple cap (64 placeholder).
- Pre-existence check: reject if the authority resolves to an EVM contract actor.
- Receiver-only constraint (current): all tuples must target the receiver authority (authority==receiver). Multi-authority updates will be realized via VM intercept semantics.
- Returns embedded status and return data; mapping and nonce updates persist even if outer call reverts.

### Interfaces (proposed)

- VM syscall invoked by EthAccount.ApplyAndCall (pseudo-signature):
  - `fn evm_apply_and_call(authority: EthAddress, to: EthAddress, value: TokenAmount, input: Bytes) -> (status: u8, returndata: Bytes)`
  - Semantics: forwards all remaining gas to the outer call; performs delegated dispatch/pointer semantics per rules above; returns status and returndata without undoing prior state updates.
- Runtime/Kernel helper for EXTCODE* projection (used by EVM actor):
  - `fn get_eth_delegate_to(actor: ActorID) -> Option<[u8; 20]>` (implemented; ref-fvm + SDK)
  - Read-only; returns the delegate address if the target is an EthAccount with `delegate_to` set.
  - EVM interpreter (ext.rs) consults this helper on Account/EOA targets to decide if pointer code is exposed.
- Helper EOA detection and resolution order:
  - Resolve target to ID, load code CID, verify it is the EthAccount builtin actor; only then decode state and read
    `delegate_to`. Never return a delegate for EVM actors or non-EOA code.
- Event topic constant: `keccak("Delegated(address)")`; data encoded as a 32-byte ABI word (last 20 bytes form the address).
- Pointer code constants: `MAGIC=0xEF01`, `VERSION=0x00` → 23-byte image: `0xEF 0x01 0x00 || delegate(20)`.
- EthAccount.ApplyAndCall params (CBOR wrapper, canonical):
  - `[ [ tuples... ], [ to(20), value(u256), input(bytes) ] ]`.
  - Tuple fields validated with domain `0x05` hash preimage for signature checks.

Implementation notes
- Trampoline: `Evm.InvokeAsEoaWithRoot` (FRC‑42 selector) is added for VM use only. It mounts the provided `initial_storage_root`, runs in authority context, and returns `(output_data, new_storage_root)`.
- EXTCODE* projection: the interpreter consults `get_eth_delegate_to(ActorID)`; pointer code is exposed as `0xEF 0x01 0x00 || delegate(20)`, `EXTCODESIZE=23`, `EXTCODEHASH=keccak(pointer_code)`, and `EXTCODECOPY` enforces windowing/zero‑fill.

Tests (current)
- Added ref‑fvm EthAccount state (de)serialization roundtrip test to guard state layout.
- Added skeleton tests (ignored) for intercept semantics: EXTCODE* projection, delegated CALL success/revert mapping with revert bytes, depth limit enforcement, value‑transfer short‑circuit, and SELFDESTRUCT no‑op. These will be enabled with a DefaultCallManager harness in a follow‑up.

### EXTCODE* Semantics Over Pointer Code

- EXTCODESIZE(authority) returns 23 when `delegate_to` is set, else the authority’s actual code size (typically 0).
- EXTCODEHASH(authority) returns keccak(pointer_code) when delegated, else code hash per normal rules.
- EXTCODECOPY(authority, destOffset, offset, size) copies up to 23 bytes of the virtual pointer code starting at
  `offset`, truncates when out of range, and zero-fills the remainder per EVM rules. When `offset >= 23` and `size > 0`,
  the copy yields all zeros.

## ref-fvm Changes (status)

- Branch: `eip7702`.
- VM features (implemented):
  - EXTCODE* pointer projection for EOAs with `delegate_to`.
  - Delegated dispatch for CALL to EOAs with authority context + storage overlay + depth limit.
  - Event emission for `Delegated(address)` with authority address.
  - Syscall invoked by EthAccount.ApplyAndCall to perform the outer call with all gas forwarded and return status/returndata.
  - Context plumbing to prevent re-following delegation and to manage storage root mount/restore.
- Tests in ref-fvm (landed):
  - Pointer projection (size/hash/copy), delegated dispatch success/revert, depth limit, SELFDESTRUCT no-op, revert-data propagation, value-transfer short-circuit, and `Delegated(address)` event topic/data coverage.

### Docker Bundle/Test Flow (macOS friendly)

- Build builtin-actors bundle in Docker:
  - From `../builtin-actors`: `make bundle-testing-repro`
- Run ref-fvm tests with a prebuilt bundle:
  - From `../ref-fvm`: `scripts/run_eip7702_tests.sh`
- This avoids macOS toolchain issues (e.g., `__stack_chk_fail` Wasm import) when invoking EVM in tests.

### EXTCODECOPY Windowing Examples

- For an EOA A delegated to B, pointer code = `0xEF 0x01 0x00 || B(20)`.
- EXTCODECOPY(A, dest, 0, 23) → full 23 bytes.
- EXTCODECOPY(A, dest, 1, 22) → pointer_code[1..23].
- EXTCODECOPY(A, dest, 23, 1) → single zero.
- EXTCODECOPY(A, dest, 100, 10) → 10 zeros.

### Repo/Path Impact (ref-fvm)

- Execution layer:
  - EVM call entry path: intercept CALL→EOA; implement authority-context dispatch.
  - Code query path: intercept EXTCODE{SIZE,HASH,COPY} for EOAs; provide virtual pointer bytes/hash/size.
- Context plumbing:
  - Execution context flag for authority mode; prevent delegation re-follow.
  - Storage overlay: mount EthAccount.evm_storage_root and persist updated root on exit.
  - EthAccount state mutation policy: ref-fvm is responsible for persisting `evm_storage_root` back to the authority
    EthAccount after delegated execution. This is a privileged VM operation; user code cannot mutate this field.
    Optionally, implement as an internal EthAccount hook callable only by the VM.
  - Address resolution policy: the VM intercept must resolve Ethereum addresses to ActorIDs, then use code CID to
    distinguish EthAccount vs EVM actors. Delegated dispatch applies only to EthAccounts (EOAs).
- Events:
  - Emit `Delegated(address)` after delegated outer call (best-effort).

## builtin-actors Changes

- EthAccount (implemented):
  - State extended with `delegate_to`, `auth_nonce`, `evm_storage_root`.
  - `ApplyAndCall`:
    - Validates tuples and nonces.
    - Updates state (delegate_to + auth_nonce; initializes storage root if needed).
    - Invokes ref-fvm bridge for the outer call; returns embedded status + return data.
- EVM actor (cleaned up):
  - Removes internal delegation/nonces/storage-root structures and the legacy InvokeAsEoa entrypoint.
  - Keeps the interpreter intact; does not alter EXTCODE* — the VM supplies pointer behavior.
  - Updates ext.rs to consult the runtime helper `get_eth_delegate_to` for Account/EOA targets to project pointer code.
- Tests:
  - Global mapping test passes (eoa_pointer_mapping_global.rs).
  - Suites for pointer semantics, revert-data, depth limit, pre-existence, R/S padding, tuple cap keep passing through EthAccount + VM path.

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

## Sequencing (high level, completed)

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
- EXTCODECOPY windowing
  - Add ref-fvm tests that exercise partial and out-of-bounds windows over the 23-byte image, asserting correct truncation
    and zero-fill semantics.
- Global mapping visibility
  - actors/evm/tests/eoa_pointer_mapping_global.rs → remains; now passes using EthAccount state.
  - ref-fvm: add test to verify cross-contract EXTCODE* sees pointer code for any EOA with delegate.
- Delegated CALL behavior
  - actors/evm/tests/delegated_call_revert_data.rs → remains; VM must propagate revert data to return buffer + memory.
  - actors/evm/tests/apply_and_call_value_transfer.rs → remains; VM short-circuits on failed transfer.
- Event emission under gas tightness
  - Add ref-fvm tests that enforce extremely tight gas conditions to verify Delegated(address) emission is best-effort
    and absence of the event does not affect outer call status/returndata.
- Atomic ApplyAndCall
  - actors/evm/tests/apply_and_call_atomicity_* → port to EthAccount.ApplyAndCall; assert persistence on revert.
  - ref-fvm: syscall tests ensure status/returndata returned per VM dispatch.
- Depth limit
  - actors/evm/tests/apply_and_call_depth_limit.rs → remains; VM prevents delegation re-follow under authority context.
  - Add ref-fvm test for A->B and B->C configured; CALL→A delegates to B only (no follow to C).
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
- Nonce initialization
  - actors/evm/tests/apply_and_call_nonces.rs → ensure absent authority defaults to nonce=0; applying nonce=0 succeeds;
    subsequent nonce=0 fails with mismatch.
  - Add a ref-fvm unit test covering EthAccount state read/write round-trips to prevent struct drift.

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
  - Add a receipt test variant that tolerates missing Delegated(address) when gas is extremely tight (best-effort event).
  - Add a decode robustness test for revert data in EVM paths to ensure no silent fallback on decode failures (return
    illegal_state in invariants instead of discarding data).

## Execution Checklist (status)

- ref-fvm (branch `eip7702`)
  - [x] EXTCODE* pointer projection for EOAs with delegate
  - [x] CALL→EOA delegated dispatch with authority context and depth limit
  - [x] Authority storage overlay mount/persist
  - [x] `Delegated(address)` event emission
  - [x] ApplyAndCall syscall with all-gas-forwarded semantics
  - [x] Unit tests for features above
- builtin-actors
  - [x] EthAccount: add `delegate_to`, `auth_nonce`, `evm_storage_root`
  - [x] EthAccount: implement `ApplyAndCall` validation + state updates + syscall
  - [x] Remove delegation maps and InvokeAsEoa from EVM actor
  - [x] Adapt tests to EthAccount.ApplyAndCall; keep coverage green
- Lotus
  - [x] Route 0x04 to EthAccount.ApplyAndCall
  - [x] Receipts attribution and status decode intact
  - [x] Behavioral gas estimation unchanged

## Risks & Mitigations

- Risk: Pointer semantics regress under VM projection.
  - Mitigation: Add ref-fvm EXTCODE* tests and keep existing actor tests.
- Risk: Storage overlay bugs cause state loss/corruption.
  - Mitigation: Add persistence/isolation tests; verify root before/after.
- Risk: Revert-data propagation differences between VM and actor path.
  - Mitigation: Dedicated tests for RETURNDATASIZE/RETURNDATACOPY semantics.
- Risk: Event emission dropped under gas tightness.
  - Mitigation: Document best-effort; ensure outer call status unaffected; test both with/without sufficient gas.
- Risk: Helper drift and EOA misclassification.
  - Mitigation: Centralize EthAccount state struct and add ref-fvm round-trip tests; strictly verify code CID for
    EthAccount in helper and intercept paths.

## Out of Scope

- Numeric gas constants/refunds finalization (Lotus remains behavioral; actor refunds staged separately).
- Mempool policy changes (none required).

## Acceptance Criteria

- Delegation state and nonces are per-EOA in EthAccount and persist across transactions.
- EXTCODESIZE/HASH/COPY on delegated EOAs exposes a 23-byte pointer code globally.
- CALL to delegated EOAs executes the delegate’s code in authority context with a depth limit of 1 and authority storage overlay; SELFDESTRUCT no-op.
- `ApplyAndCall` persists mapping + nonces pre-call and returns embedded status + return data; receipts/logs attribute `delegatedTo` as specified; Lotus routes 0x04 to EthAccount.ApplyAndCall.
- Behavioral gas tests in Lotus remain green; no special mempool policies.

## Ownership & Areas

- ref-fvm: VM pointer projection, delegated dispatch, authority-context storage overlay, events, ApplyAndCall syscall.
- builtin-actors: EthAccount state + `ApplyAndCall` validation and state updates; remove EVM-actor delegation code.
- Lotus: 0x04 routing to EthAccount.ApplyAndCall; receipts attribution and behavioral gas estimation.

## Branch Strategy

- ref-fvm: `eip7702`.
- builtin-actors and Lotus: continue on current `eip7702` branches; no backward-compatibility constraints.

## Quick Commands

- builtin-actors:
  - `cargo test -p fil_actor_evm`
  - `cargo test -p fil_actor_ethaccount`
- ref-fvm:
  - `cargo test -p fvm --test eth_delegate_to`
- Lotus:
  - `go build ./chain/types/ethtypes`
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./node/impl/eth -run 7702 -count=1`

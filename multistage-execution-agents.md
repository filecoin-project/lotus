# Multi‑Stage Execution (Gas Reservation) — Feature Sprint Notebook

This notebook specifies a new feature sprint to add multi‑stage execution to Lotus (and paired repos) to fix miner‑penalty risk under deferred execution. It provides motivation, requirements, a detailed implementation plan, testing and rollout strategy. Treat this as an agents.md for this sprint: keep diffs small, follow the plan, and track progress in this file.

**Paired Repos**
- `./lotus` (this repo)
- `../builtin-actors` (runtime hooks and enforcement)

**Branching**
- Create a new branch off `master`: `multistage-execution`.
- Keep this sprint isolated from the EIP‑7702 branch; no cross‑branch coupling.

---

## Motivation

Problem (today): with deferred execution and intra‑tipset reordering effects, a malicious sender can:
- Include two messages with nonces X and X+1 in the same tipset.
- Message X sends away (spends) the account’s funds via contract execution.
- Message X+1 uses a large gas limit and passes block‑packing admission because the miner accounts for declared value and rough gas affordability at pack time.
- At execution time, funds are gone; charging the sender fails. The protocol charges the miner (block provider) for the gas discrepancy (see Background: Miner Penalty Semantics).

Packing heuristics partially mitigate this by tracking declared message `Value` when building blocks, but they can’t anticipate runtime value transfers performed by earlier messages.

Proposed fix: introduce multi‑stage execution over the entire tipset with up‑front reservation of gas funds per message, preventing later messages’ gas from being “stolen” by earlier message execution.

---

## Background: Miner Penalty Semantics

How miner penalty is applied today when a message can’t pay for gas at execution time:

- Per‑message upfront “gas holder” deposit
  - At message apply, the VM withdraws `GasLimit * GasFeeCap` from the sender into an internal gas holder before execution. If the sender’s current balance is insufficient, the message is rejected with a miner penalty computed below.
    - Reference: `chain/vm/vm.go:540`

- Miner penalty triggers and amount
  - If the gas holder prepay fails (insufficient funds), the VM returns a failure with `GasCosts.MinerPenalty = baseFee * GasLimit`.
    - References: `chain/vm/vm.go:473`, `chain/vm/vm.go:527`
  - Similar penalties are set for other precondition failures (e.g., nonce/state invalid, actor not found, gas limit below on‑chain costs).
    - References: `chain/vm/vm.go:458`, `chain/vm/vm.go:479`, `chain/vm/vm.go:510`
  - During normal execution, additional miner penalty can accrue from fee cap shortfall and over‑estimation burn (charged at basefee gap and burned gas respectively).
    - References: `chain/vm/burn.go:70`, `chain/vm/burn.go:98`

- Block‑level aggregation and charging
  - Consensus aggregates `GasCosts.MinerPenalty` across all messages in a block and passes it to the reward actor as a penalty when awarding the block reward; this effectively charges the miner.
    - Reference: `chain/consensus/compute_state.go:235`

Because the gas holder deposit is assessed at the start of each message, a prior message in the same tipset can drain funds, causing later messages from the same account to fail the prepay and shift cost to the miner. Multi‑stage reservation fixes this by reserving funds for all messages up‑front at the tipset level.

## Goals and Non‑Goals

Goals
- Prevent miners from being charged when a sender drains their balance ahead of later messages in the same tipset.
- Make gas funding deterministic at tipset granularity by reserving each message’s maximum gas cost before any message executes.
- Keep execution order and semantics otherwise unchanged; reserve → execute → refund occurs within the same tipset application.
- Provide a clear consensus story (i.e., blocks that don’t reserve sufficient funds for all included messages are invalid under the new rule).
- Move us closer to full Account Abstraction by separating affordability (funding) from execution effects.

Non‑Goals (for this sprint)
- Changing gas pricing formulas or on‑chain fee markets.
- Reserving for dynamic, runtime value transfers (beyond the message’s declared `Value`). The focus here is gas, not runtime sends.
- Persisting reservations across epochs/tipsets.

---

## High‑Level Design

Multi‑stage execution across the full tipset (all messages in all blocks of the tipset, in canonical execution order):

1) Stage 1 — Reserve (Preflight)
- Validate all messages as today (syntactic checks, nonce continuity, etc.).
- For each message, compute a per‑message gas reservation from the sender:
  - EffectiveGasPrice = min(FeeCap, BaseFee + GasPremium) at the parent basefee (known at pack/validate time).
  - GasReserve = GasLimit * EffectiveGasPrice.
  - Optionally reserve DeclaredValueReserve = message `Value` (keeps packer invariants simple; see “Value Reservation” below).
  - TotalReserve = GasReserve (+ DeclaredValueReserve if enabled in this sprint; default: reserve gas only).
- Maintain a per‑account ledger of “reserved” balances across the entire tipset. A reservation succeeds if `Available(sender) - AlreadyReserved(sender) >= TotalReserve`.
- If any reservation fails, the block(s)/tipset is invalid under the new rule (consensus check), and miners must not build such blocks once activated.

2) Stage 2 — Execute
- Execute messages in order as today.
- All value transfers (including SELFDESTRUCT, explicit FEVM sends, etc.) must treat the sender’s reserved gas balance as locked (unspendable) for the duration of tipset execution. I.e., `transfer(sender→X, amount)` must ensure `Balance(sender) - ReservedGas(sender) - amount >= 0`.
- Gas charges during execution draw down from the reservation: on gas burn/reward, deduct from the sender’s reserved gas and the account’s actual balance simultaneously.

3) Stage 3 — Refund
- For each message, after execution, compute actual gas paid using existing rules. Refund (unreserve) `GasReserve - GasPaid` back to the sender, immediately after the message completes.
- After the last message from an account completes (or at end of tipset), any unused reservation must be fully released.

Consensus semantics
- Stage 1 is consensus‑validating: a block set that cannot be fully reserved (for gas) is invalid.
- Stages 2 and 3 happen atomically during tipset execution; no inter‑tipset reservation persistence.

Value reservation (optional)
- We keep the primary goal focused on gas safety. Declared `Value` reservation is optional but recommended for packer simplicity and to mirror today’s running‑total heuristic with a formal guarantee.
- If enabled, we reserve `Value` in Stage 1 as well and release it immediately when the message starts executing (or convert into an actual transfer at call entry), keeping semantics unchanged.

Observability
- Execution traces and receipts are unchanged. No new fields are required on receipts for this sprint.
- Balance reads (e.g., EVM `BALANCE`) return the account’s on‑chain balance (including reserved funds). The lock only affects spendability, not visibility, to avoid breaking existing contract assumptions. Enforcement happens at transfer/charge sites.

---

## Accounting Details

EffectiveGasPrice
- `effective_price = min(FeeCap, BaseFee + GasPremium)` at parent basefee.

Gas reservation per message
- `GasReserve = GasLimit * effective_price`.
- Refunded after execution: `GasReserve - ActualGasPaid`.

Charge application
- Existing burn/reward rules remain: basefee burn, miner tip, over‑estimation burn (if any) are charged out of the reserved portion first.
- If Stage 1 succeeded, Stage 2 gas charges must not underflow due to earlier runtime transfers.

Transfers vs. reserved gas
- During Stage 2, any `transfer` from an account must ensure `amount <= Balance(sender) - ReservedGas(sender)`. If not, return `USR_INSUFFICIENT_FUNDS` (or equivalent). This blocks “stealing” reserved gas.

---

## Implementation Plan

We split the work across repos. The critical path is consensus/runtime. Lotus changes augment pack, estimation, and validation flows; runtime changes enforce locks.

Phases
1) Prototype (feature branch)
2) Miner/packer alignment (pack-time reservation simulation)
3) Enablement and rollout (see Rollout Strategy)

### A. Builtin‑Actors Runtime (consensus‑critical)

Primary approach for builtin‑actors‑only changes: GasReservoirActor (escrow)

Rationale
- Implementing an ephemeral, host‑enforced ledger requires changes outside builtin‑actors. To keep this sprint self‑contained in builtin‑actors + Lotus, we lead with an escrow actor that holds reserved funds and settles burns/tips/refunds during the tipset. This makes reservation and enforcement explicit with on‑chain value movements and avoids altering transfer internals.

New builtin actor: GasReservoirActor
- Responsibilities
  - Hold per‑sender reservations for the current tipset application window.
  - Disburse basefee burns, miner tips, and over‑estimation burns for each message as it completes.
  - Refund unused reservation immediately after each message.
- State
  - `session_epoch: ChainEpoch` — guards a single open reservation session.
  - `base_fee: TokenAmount` — effective base fee used for the session.
  - `locked: Map<ActorID, TokenAmount>` — current reserved gas funds per sender.
  - `msg_seen: Set<Cid>` — idempotency guard for settlement calls (avoid double‑settle).
- Exports (method names illustrative)
  - `StartSession{ epoch, base_fee, message_root } -> ()`
    - Resets state for the epoch. Fails if a session is already open for a different epoch.
  - `Reserve{ sender: ActorID, amount: TokenAmount } -> ()`
    - Requires caller authorization (see Access Control). Increases `locked[sender]` by `amount` and receives `amount` FIL via an explicit send alongside this call.
  - `Settle{ msg_cid: Cid, sender: ActorID, gas_limit: int64, fee_cap: TokenAmount, premium: TokenAmount, gas_used: int64, burn_enabled: bool } -> SettlementResult`
    - Idempotent per `msg_cid`. Computes `GasOutputs` equivalent (basefee burn, miner tip, over‑estimation burn, refund) using `base_fee` from session and supplied execution results. Transfers:
      - BaseFeeBurn to `BurntFundsActor`.
      - MinerTip to `RewardActor` (caller supplies miner beneficiary for the current block as a parameter or this call is invoked per‑block with context; see Lotus orchestration below).
      - Refund back to `sender`.
    - Decrements `locked[sender]` accordingly; errors if insufficient locked funds (violates Stage 1 invariants).
  - `EndSession{}` -> ()
    - Refunds any residual locked balances and closes the session. Idempotent.
- Access control
  - `StartSession/Reserve/Settle/EndSession` callable only by the System actor (i.e., consensus driver invoked entrypoints). All external user‑initiated calls are forbidden.
- Error semantics
  - `Reserve` must be accompanied by value equal to `amount`; mismatch reverts.
  - `Settle` validates `gas_used <= gas_limit` and enforces math invariants consistent with `chain/vm/burn.go`.

Implications
- Account `BALANCE` reflects funds moved into the reservoir during the tipset. This is observable and acceptable for the sprint; contracts will see reduced balance while their messages are pending execution within the same tipset.
- No changes are required to individual actor transfer paths; enforcement arises from holding funds in escrow.

Runtime interfaces likely touched (builtin‑actors tree)
- Add a new crate under `actors/gas_reservoir/` with the ABI and state types.
- Extend `actors/system` to whitelist the reservoir actor and authorize calls from the consensus driver.
- Update `actors/reward` to accept per‑message miner tips via reservoir settlement (or use an intermediate send to `RewardActor` from the reservoir actor during `Settle`).
- Export shared fee math utilities alongside `actors/runtime` equivalents to keep settlement math consistent with Lotus `chain/vm/burn.go`.

### B. Lotus (client)

Block building / miner packer
- Add a pre‑pack simulation of Stage 1 across candidate messages to pre‑screen blocks. This guarantees miners don’t build invalid blocks once the consensus rule is active.
- If value reservation is enabled, include `Value` into the simulated reservation.

Detailed Lotus changes (files/functions)
- `chain/consensus/compute_state.go`
  - Stage 1 (per‑tipset, before execution):
    - Walk canonical execution order for all messages in the tipset; compute `effective_price = min(feeCap, baseFee + premium)` and `reserve = gasLimit * effective_price` per message.
    - Build `reserved[sender] += reserve`; if `balance(sender) < reserved[sender]`, mark tipset invalid.
    - Call `GasReservoirActor.StartSession{ epoch, base_fee, message_root }` once.
    - For each unique `sender`, call `GasReservoirActor.Reserve{ sender, amount }` with an accompanying value transfer equal to `amount`.
  - Stage 2/3 (during execution loop):
    - After each `ApplyMessage`, call `GasReservoirActor.Settle{ msg_cid, sender, gas_limit, fee_cap, premium, gas_used, burn_enabled }` to burn basefee, pay tip, burn overestimation, and refund remainder to sender.
  - After all messages (per block or tipset), call `GasReservoirActor.EndSession{}` to close and refund any residual.
- `chain/vm/vm.go`
  - Bypass the per‑message gas holder path to avoid double withdrawals/transfers when multi‑stage execution is enabled in this branch:
    - Skip `transferToGasHolder` and all subsequent `transferFromGasHolder` burns/tips/refunds.
    - Still compute and return `GasOutputs` for receipts and for block‑reward accounting.
    - Consensus layer performs settlement via `GasReservoirActor.Settle` after `ApplyMessage` returns.
- `miner/*`
  - Integrate Stage‑1 simulation into the message selection loop to avoid assembling blocks that would be invalid under Stage‑1 reservation (same reservation math as above).
- `itests/*`
  - Add end‑to‑end tests covering the exploit scenario and verifying reservoir settlement flows and balances.

Chain validation
- On tipset application: when calling into the runtime, pass the full ordered message list and expect Stage 1 to run inside the runtime. The runtime enforces consensus rules.
- Node validation code should be ready to surface reservation failures as block invalid.

Gas estimation and mempool
- `eth_estimateGas` and `mpool` policies remain functionally unchanged.
- Optional: expose a “would reserve” debug endpoint to aid operators when diagnosing inclusion failures.

Feature toggles
- Activation and rollout are managed outside this document. Miners should run Stage‑1 simulation in the packer to avoid assembling blocks that fail reservation.

---

## Code Map (Touch Points)

Lotus
- Block application / VM entry: `chain/vm/*` — ensure we pass full tipset to runtime and handle reservation failure codes.
- Miner packer: `miner/*` — pre‑pack Stage‑1 simulation, maintaining per‑account reservations when selecting messages.
- Validation plumbing and errors: `chain/consensus/*` (surface runtime reservation failures as invalid blocks).
- Tests: `itests/*`, unit tests in `chain/vm`, miner packer tests.

Builtin‑actors runtime
- Runtime transfer path and gas charging: enforce `ReservedGas` locks; apply refunds post‑message.
- Tipset execution orchestrator: implement Stage 1 → Stage 2 → Stage 3 flow.
- Tests under `runtime/tests` and `actors/*` as applicable.

New builtin actor
- Add `actors/gas_reservoir/` with:
  - `types.rs` (state structs: session_epoch, base_fee, locked map, msg_seen)
  - `lib.rs` (actor methods: StartSession, Reserve, Settle, EndSession; access control)
  - `tests/*` covering reservations, settlement math, idempotency, and refunds

ABI / API Sketch (CBOR)
- Method numbers use FRC‑42 (keccak4 of method names). Suggested names:
  - `StartSession`, `Reserve`, `Settle`, `EndSession`.
- Param encoding uses canonical CBOR arrays (fixed field order).
  - StartSessionParams: `[ epoch:int64, base_fee:TokenAmount, message_root:Cid ]`
  - ReserveParams: `[ sender:Address(ID), amount:TokenAmount ]` (value attached to call must equal `amount`)
  - SettleParams: `[ msg_cid:Cid, sender:Address(ID), gas_limit:int64, fee_cap:TokenAmount, premium:TokenAmount, gas_used:int64, burn_enabled:bool, miner_beneficiary:Address(ID) ]`
  - EndSessionParams: `[]`
- Return values
  - `Settle` returns `[ basefee_burn:TokenAmount, miner_tip:TokenAmount, overestimation_burn:TokenAmount, refund:TokenAmount ]` for observability; others return `[]`.

Activation and wiring checklist (non‑prescriptive)
- System actor whitelists calls from consensus driver to GasReservoirActor methods.
- Integration tests cover reservation failures (block invalid) and success paths.

---

## Testing Plan

Unit tests (runtime)
- Reservation math: EffectiveGasPrice, GasReserve, cumulative per‑account reservations across multiple messages.
- Transfer enforcement: attempts to transfer reserved gas fail with insufficient funds; transfers within free balance succeed.
- Gas charges: reserved pool decrements with gas usage; refunds are returned immediately post‑execution.
- Failure cases: reservation failure on Stage 1 yields invalid block.

Lotus integration tests
- Tipset with two messages from the same account:
  - M1 drains balance via contract; M2 has large gas limit. With multi‑stage enabled, M2 executes and pays gas; M1 cannot steal reserved funds.
  - With feature disabled, reproduce miner‑charged behavior (baseline).
- Miner packer simulation: verify pre‑pack Stage 1 excludes over‑committing message sets.
- Value reservation (if enabled): reserved `Value` is released/converted at message entry; semantics are preserved.

Property/fuzz tests
- Generate random multi‑message tipsets with varying balances/fees to assert invariants: no execution path can consume reserved gas except gas charging; refunds never exceed reservations; no negative balances.

Performance testing
- Measure Stage 1 overhead for large tipsets; ensure reservation pass is O(n) and does not regress block validation latency unacceptably.

---

## Rollout Strategy

Phase 0 — Branch work
- Add miner pre‑pack simulation (opt‑in config) for early testing.

Phase 1 — Testnets
- Enable multi‑stage on a devnet; monitor latency and failure modes; add telemetry around reservation failures and refund sizes.

Phase 2 — Mainnet candidate
- Coordinate migration; require miner upgrade (packer simulation) to avoid building invalid blocks.

Backwards compatibility
- Prior to activation, blocks are validated under existing rules; miner pre‑pack simulation is advisory.
- At activation, blocks must pass Stage 1 reservation or are invalid.

---

## Risks and Mitigations

- Semantics of balance reads: contracts may observe a balance that includes reserved gas. This is intentional; enforcement occurs at transfer sites to avoid breaking assumptions.
- Implementing transfer enforcement incompletely: must ensure all value‑moving paths (including SELFDESTRUCT and implicit value transfers) are enforced through the same check.
- Performance: Stage 1 must be efficient; caching effective price and grouping by sender helps.
- Tipset ordering: ensure Stage 1 operates over the exact message order used for execution.

---

## User Impact

High‑level behavior
- Funds for gas are reserved up‑front across the entire tipset. Users can’t “steal” their own reserved gas with earlier messages in the same tipset.
- During the tipset, the reserved portion is held by GasReservoirActor; the account’s visible balance is reduced accordingly. After each message, unused gas is refunded immediately; after the tipset ends, any residual is released.

Attack scenario (drain funds, then big‑gas follow‑up)
- Stage‑1 reserves the maximum gas cost for both messages (M1 nonce X, M2 nonce X+1). The reservoir receives two deposits from the sender, reducing the sender’s free balance.
  - When M1 executes and attempts to transfer “all funds”, only the unreserved portion is available. If M1 tries to transfer more than `free_balance`, that transfer fails (insufficient funds) and M1 reverts; if it transfers ≤ `free_balance`, it succeeds but cannot consume reserved gas.
  - M2 still has its gas fully reserved and executes normally; any unused gas is refunded afterward. In all cases, the miner is not charged for gas shortfalls because reservations covered the cost up‑front.

Notes
- This sprint reserves gas only (by default). Declared value reservation is optional and can be enabled later to make packer accounting symmetrical for value as well.

---

## Normal (Good) Path — What Users Experience

Single message with sufficient funds
- Stage 1 (reservation): the miner/node reserves `GasLimit * min(FeeCap, BaseFee + Premium)` for the message by depositing that amount into the GasReservoirActor. The sender’s free balance is reduced temporarily.
- Stage 2 (execution): the message executes normally. Value transfers inside the transaction spend from the sender’s free (unreserved) balance.
- Stage 3 (refund/settlement): immediately after execution, the node calls `Settle`, which:
  - Burns basefee for `gasUsed`.
  - Pays the miner tip corresponding to `gasLimit` and the effective tip.
  - Burns any over‑estimation per current rules.
  - Refunds the remainder back to the sender.
- Result: receipts look the same (gasUsed, status, etc.). The reserved funds that were not spent return to the sender. The miner is paid as usual; nothing changes for dapps besides the brief intra‑tipset balance reservation.

Multiple messages from the same account with sufficient funds
- Stage 1 reserves gas for all messages up‑front. The sender’s free balance decreases by the sum of all reservations.
- Messages execute in order. Each message settles immediately after execution, refunding unused gas and preserving the next messages’ reservations intact.
- If any message performs value transfers, they succeed as long as the transfer amount ≤ current free balance (excluding amounts held in the reservoir). Because all gas is reserved, later messages won’t be starved of gas by earlier transfers.

---

## Work Breakdown

Runtime (consensus)
1. Add tipset application context and Stage 1 reservation pass (ephemeral ledger)
2. Enforce reserved‑aware transfers on all value paths
3. Charge gas against reserved pool; implement per‑message refund at completion
4. Return clear errors on reservation failure
5. Runtime unit tests for reservation, enforcement, refunds

Lotus (client)
1. Miner packer: add pre‑pack Stage 1 simulation (config‑gated), integrate into selection loop
2. VM entry: pass message list to runtime; propagate reservation failures
3. Integration tests covering the exploit scenario and miner safety
4. Optional debug endpoint for reservation previews

Docs and Ops
1. Document operator requirements and observability
2. Add operator guidance to detect and fix blocks that would fail Stage 1

---

## Acceptance Criteria

- Under multi‑stage execution, every included message has its gas reservation secured in Stage 1; no execution path can cause a miner‑charged underfunded message.
- Transfer enforcement guarantees that reserved gas cannot be spent by other runtime operations.
- Refunds are correct and immediate after each message; cumulative reserved balance returns to zero by tipset end.
- Miner packers avoid building invalid blocks when pre‑pack simulation is enabled.
- Tests: exploit scenario passes (miner not charged), reservation math tests pass, no regressions in unrelated execution paths.

---

## Editing and Commit Guidance

- Keep diffs minimal and focused; avoid mixing formatting with logic.
- Split PRs by repo and by concern: runtime core, lotus packer, tests.
- Pre‑commit in Lotus: `make gen`, `go fmt ./...`. In builtin‑actors: `cargo fmt --all`, `make check`.
- Add targeted tests with each change; don’t pin numeric gas constants beyond EffectiveGasPrice logic.

---

## Decisions (Resolved)

Value Reservation (Default)
- Decision: Reserve gas only by default (no declared `Value` reservation in Stage 1).
- Rationale:
  - Minimal semantic change; fixes the miner‑penalty risk without altering value‑transfer behavior.
  - Keeps intra‑tipset reservations limited to gas, reducing surprises for contract logic that relies on free balance.
  - Simpler to ship and validate; we can add an optional packer‑time mode for declared Value reservation later if operators demand stricter packing.
- User impact:
  - Users see a temporary (intra‑tipset) reduction in their free balance equal to the reserved gas; value transfers behave as they do today based on free balance.
  - The “drain then big gas” attack no longer works because gas for all included messages is reserved up‑front.
- Implementation notes:
  - Stage 1 computes `reserve = gasLimit * min(feeCap, baseFee + premium)` per message and deposits into GasReservoirActor.
  - Packer simulation uses the same formula; blocks that cannot reserve for all included messages are invalid.
  - Optionally add a packer configuration to simulate/attempt declared `Value` reservation; not enabled by default.

Integration Approach (Builtin‑Actors)
- Decision: Implement a GasReservoirActor (escrow) in builtin‑actors to hold and settle reservations.
- Rationale:
  - Keeps the sprint self‑contained within Lotus + builtin‑actors, with explicit on‑chain accounting and simple enforcement.
  - Avoids invasive changes to transfer internals; uses explicit sends to escrow for reservation and settlement, which are easy to test and reason about.
  - Aligns settlement math with existing `chain/vm/burn.go` fee logic for correctness and parity in receipts and rewards.
- Trade‑offs vs. ephemeral ledger:
  - Ephemeral ledger would make reservations invisible to balance reads but requires host/runtime changes outside builtin‑actors scope.
  - GasReservoirActor causes a visible intra‑tipset reduction in balance (acceptable) and is simpler to implement now.
- User impact:
  - During a tipset, users’ visible balance is reduced by the reserved gas; unused gas is refunded immediately after each message completes; miners are paid as usual.
- Implementation notes:
  - Add `actors/gas_reservoir/` with StartSession, Reserve (value‑bearing), Settle (burn/tip/overburn/refund), and EndSession.
  - In Lotus: pre‑pack simulation; call StartSession once per tipset, Reserve per sender, Settle after each ApplyMessage, EndSession at the end.
  - In `chain/vm/vm.go`: when multi‑stage is enabled in this branch, bypass the per‑message gas holder path to avoid double withdrawals; still compute GasOutputs for receipts and reward accounting.

Applicability (All Actors)
- Decision: Apply multi‑stage reservation and settlement to all messages regardless of target actor (not just EVM).
- Rationale:
  - The underlying risk (draining funds between messages from the same sender) is actor‑agnostic; any actor code path can move balance.
  - Stage‑1 reservation is keyed by sender and gas parameters, and settlement is driven by VM‑computed GasOutputs; both are independent of the target actor.
  - A single consistent rule avoids edge‑cases where a non‑EVM message could still shift costs to the miner.
- User impact:
  - Uniform experience across the system: gas is always reserved up‑front; unused gas refunds immediately after each message.
  - No contract‑level semantic changes beyond the temporary intra‑tipset balance reservation; value transfers continue to behave as today.

// Bundle-dependent FEVM gas canaries.
//
// # What these tests are
//
// These tests pin the boundary recursion counts at which specific FEVM
// contracts exhaust their gas budget. They are *canaries*: they are expected
// to fail when the built-in actors bundle is rebuilt, because a new Rust
// toolchain or any actor source change produces slightly different WASM,
// which in turn produces slightly different per-instruction gas totals after
// `fvm_wasm_instrument` injects the pre-block gas counter. Typical
// per-iteration drift is on the order of ±15k gas in `wasm_exec` (a
// fraction of a percent of the ~3M per-iteration total), and is often zero
// for a given release. Boundary retunes are rare. See the historical drift
// table further down.
//
// # When a canary fails
//
// First, decide if this is *normal bundle drift* or something to investigate.
// Run TestFEVMCanaryBundleCompare (or .Historical for the full table); it
// prints a per-bucket delta between the previous and current bundle. If:
//
//   - the delta is entirely in `wasm_exec`, all other buckets are bit-
//     identical before and after,
//   - the per-iteration delta is on the order of ±20k gas or smaller, and
//   - the boundary has shifted by at most a couple of iterations,
//
// then this is normal bundle-rebuild drift (different compiled WASM =
// different metered-block split = different wasm_exec total). Re-tune the
// boundary constants in this file to the new passing values and update the
// corresponding `PerIterGasApprox` constant to the new per-iteration total.
//
// If drift also shows up in non-wasm_exec buckets, the FVM pricelist or an
// IPLD schema changed at this network-version boundary, this is not pure bundle
// drift. That's only expected at NV boundaries that explicitly ship a new
// pricelist (e.g. v11->v12 was the Watermelon pricelist; v15->v16 was the
// Watermelon -> Teep transition). Investigate before assuming a simple
// retune is safe.
//
// If the per-iteration delta is large (>~5% of per-iter total), or the
// failing exit code has changed rather than just the boundary, escalate.
//
// # Seeing what changed across a bundle upgrade
//
// TestFEVMCanaryBundleCompare below uses `kit.UpgradeSchedule` to run the
// canary message *twice* in one test: once under the previous actors bundle
// (current NV - 1), then once under the current bundle after an in-test
// network upgrade. It prints per-bucket gas for both and a delta. Run it
// with:
//
//	go test -v -run TestFEVMCanaryBundleCompare ./itests/fevm_bundle_canary_test.go
//
// This is diagnostic, never asserts on gas values, and is the quickest way
// to confirm "is this just bundle drift".
//
// # Tuning workflow
//
//  1. Canary fails in CI on a bundle bump.
//  2. Run `TestFEVMCanaryBundleCompare` to see the before/after delta.
//  3. If drift looks normal, update the boundary constants below. Also
//     update the corresponding `PerIterGasApprox` constant to the new
//     observed per-iteration cost — the failure reporter uses it to flag
//     anomalous drift next time.
//  4. If drift looks anomalous, escalate.
//
// Add a row to the "Historical drift" table below when retuning.
//
// # Historical drift
//
// Per-iteration gas delta for the RecCall actor-call canary
// (stackDepth=0, 100 recursive calls), measured with
// TestFEVMCanaryBundleCompareHistorical. Most transitions are 100%
// wasm_exec drift (pure bundle rebuild, compiler-induced); NV-level FVM
// or pricelist changes that touch other buckets are noted.
//
//	Transition          | per-iter delta |    total / 100 | notes
//	--------------------+----------------+----------------+------------------------------------
//	v11 -> v12 (nv21)   |         -3,490 |       -349,084 | Watermelon pricelist intro;
//	                    |                |                |   wasm_exec -906,896;
//	                    |                |                |   new IPLD buckets replace
//	                    |                |                |   OnBlockOpenPerByte (+557,812
//	                    |                |                |   across non-wasm_exec)
//	v12 -> v13 (nv22)   |              0 |              0 | no change
//	v13 -> v14 (nv23)   |        -91,769 |     -9,176,966 | wasm_exec only; v14 much cheaper.
//	                    |                |                |   Triggered PR #12144 (2024-06):
//	                    |                |                |   delegatecall 226 -> 228,
//	                    |                |                |   actor-call pass 253 -> 255
//	v14 -> v15 (nv24)   |              0 |              0 | no change
//	v15 -> v16 (nv25)   |        +15,957 |     +1,595,701 | Watermelon -> Teep pricelist;
//	                    |                |                |   wasm_exec +1,586,656;
//	                    |                |                |   OnScanIpldLinks +7,035,
//	                    |                |                |   OnBlockOpen +2,010
//	v16 -> v17 (nv27)   |         -8,045 |       -804,557 | wasm_exec only
//	v17 -> v18 (nv28)   |        +14,402 |     +1,440,205 | wasm_exec only.
//	                    |                |                |   Triggered PR #13585 (2026-04):
//	                    |                |                |   actor-call pass 255 -> 254
//
// When retuning, regenerate this table by running:
//
//	LOTUS_TEST_FEVM_CANARY_HISTORICAL=1 go test -v \
//	  -run TestFEVMCanaryBundleCompareHistorical \
//	  ./itests/fevm_bundle_canary_test.go
package itests

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/gasstats"
)

// Canary constants. Retune alongside any bundle-driven failure. Keep
// `PerIterGasApprox` in the same ballpark as the latest observed per-
// iteration cost so the failure reporter can flag anomalous drift.
const (
	// TestFEVMRecursiveActorCallCanary: recursive FEVM actor-call boundaries.
	// Pairs are (stackDepth, recursionLimit). Highest passing recursionLimit
	// for each stackDepth, plus the first failing value.
	// Last tuned: 2026-04-21 against actors v18.0.0-rc1 (nv28 rollout).
	actorCallBoundaryD0Pass   = 254
	actorCallBoundaryD0Fail   = 261
	actorCallBoundaryD251Pass = 164
	actorCallBoundaryD251Fail = 173
	// Approximate gas per recursive call observed when these constants were
	// last tuned. Used to sanity-check the magnitude of drift on failure.
	actorCallPerIterGasApprox int64 = 3_010_000

	// TestFEVMRecursiveDelegatecallCanary: max delegatecall depth.
	// `highestSuccess` is the largest N where the contract returns N
	// successfully; `iterationsBeforeFailing` is the N reached inside the
	// contract before running out of gas on a too-large request.
	delegatecallHighestSuccess             = 228
	delegatecallIterationsBeforeFail       = 222
	delegatecallPerIterGasApprox     int64 = 100_000

	// Sub-boundary count used by the bundle-compare diagnostic. Must pass
	// comfortably under both the current and the previous actors bundle.
	bundleCompareRecursionCount = 100
)

// TestFEVMRecursiveActorCallCanary exercises the recursive-actor-call
// boundaries against the current actors bundle. See the file header for how
// to interpret a failure.
func TestFEVMRecursiveActorCallCanary(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/RecCall.hex")

	const exitTransactionReverted = exitcode.ExitCode(33)

	cases := []struct {
		stackDepth     int
		recursionLimit int
		expect         exitcode.ExitCode
		label          string
	}{
		{0, actorCallBoundaryD0Pass, exitcode.Ok, "d0-pass"},
		{251, actorCallBoundaryD251Pass, exitcode.Ok, "d251-pass"},
		{0, actorCallBoundaryD0Fail, exitTransactionReverted, "d0-fail"},
		{251, actorCallBoundaryD251Fail, exitTransactionReverted, "d251-fail"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s(depth=%d,limit=%d)", tc.label, tc.stackDepth, tc.recursionLimit), func(t *testing.T) {
			trace, actualExit := runRecursiveActorCall(ctx, t, client, fromAddr, actorAddr, tc.stackDepth, tc.recursionLimit)
			if actualExit == tc.expect {
				return
			}
			reportCanaryFailure(t, canaryFailure{
				testName:       "TestFEVMRecursiveActorCallCanary",
				caseLabel:      tc.label,
				recursionCount: tc.recursionLimit,
				expectedExit:   tc.expect,
				actualExit:     actualExit,
				trace:          trace,
				perIterApprox:  actorCallPerIterGasApprox,
			})
		})
	}
}

// TestFEVMRecursiveDelegatecallCanary exercises the maximum delegatecall
// depth against the current actors bundle. See the file header for how to
// interpret a failure.
func TestFEVMRecursiveDelegatecallCanary(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	cases := []struct {
		recursionCount uint64
		expectSuccess  bool
	}{
		// successes up to the tuned boundary
		{1, true},
		{2, true},
		{10, true},
		{100, true},
		{delegatecallHighestSuccess, true},
		// failures past the boundary
		{delegatecallHighestSuccess + 1, false},
		{1000, false},
		{10000000, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("count=%d,success=%t", tc.recursionCount, tc.expectSuccess), func(t *testing.T) {
			fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/RecursiveDelegeatecall.hex")
			trace, exit, observedCount, err := runRecursiveDelegatecall(ctx, t, client, fromAddr, actorAddr, tc.recursionCount)
			require.NoError(t, err)

			// The outer recursiveCall always exits Ok: the contract catches
			// failing sub-delegatecalls and returns the iteration count
			// reached before the failure. A non-Ok outer exit signals a
			// behavioural change beyond bundle drift, not a retune.
			if exit != exitcode.Ok {
				reportCanaryFailure(t, canaryFailure{
					testName:       "TestFEVMRecursiveDelegatecallCanary",
					caseLabel:      fmt.Sprintf("count=%d", tc.recursionCount),
					recursionCount: int(tc.recursionCount),
					expectedExit:   exitcode.Ok,
					actualExit:     exit,
					trace:          trace,
					perIterApprox:  delegatecallPerIterGasApprox,
					extraReason:    "outer recursiveCall returned non-Ok; the contract is expected to catch inner failures and return Ok",
				})
				return
			}

			var problem string
			switch {
			case tc.expectSuccess && int(observedCount) != int(tc.recursionCount):
				problem = fmt.Sprintf("expected to reach %d, reached %d", tc.recursionCount, observedCount)
			case !tc.expectSuccess && int(observedCount) == int(tc.recursionCount):
				problem = fmt.Sprintf("expected to run out of gas before %d, reached %d anyway", tc.recursionCount, tc.recursionCount)
			case !tc.expectSuccess && int(observedCount) != int(delegatecallIterationsBeforeFail):
				problem = fmt.Sprintf("expected to fail at iteration %d, failed at %d", delegatecallIterationsBeforeFail, observedCount)
			}
			if problem == "" {
				return
			}
			reportCanaryFailure(t, canaryFailure{
				testName:       "TestFEVMRecursiveDelegatecallCanary",
				caseLabel:      fmt.Sprintf("count=%d", tc.recursionCount),
				recursionCount: int(tc.recursionCount),
				expectedExit:   exitcode.Ok,
				actualExit:     exit,
				trace:          trace,
				perIterApprox:  delegatecallPerIterGasApprox,
				extraReason:    problem,
			})
		})
	}
}

// bundleTransition describes a network-version boundary at which the actors
// bundle changed. Discovered dynamically from filcns.DefaultUpgradeSchedule
// and actorstypes.VersionForNetwork — no manual list to maintain.
type bundleTransition struct {
	// label is a short identifier like "v17->v18 (nv28)".
	label string
	// genesisNV is the network version the test genesis starts at (the last
	// NV where the pre-transition actors version was active).
	genesisNV network.Version
	// upgradeNV is the target version to migrate to.
	upgradeNV network.Version
	// migration is the state-migration function invoked at upgradeNV.
	migration stmgr.MigrationFunc
}

// bundleTransitionsFromSchedule walks the default lotus upgrade schedule and
// returns the subset of upgrades that advance the actors version. Filtered
// to transitions where the pre-upgrade actors version is >= minActorsVersion,
// since the canary message exercises FEVM (EAM/EVM/EthAccount actors) which
// stabilised at v11. Ordered chronologically.
func bundleTransitionsFromSchedule() []bundleTransition {
	const minActorsVersion = actorstypes.Version11

	schedule := filcns.DefaultUpgradeSchedule()
	sort.Slice(schedule, func(i, j int) bool { return schedule[i].Height < schedule[j].Height })

	var out []bundleTransition
	for _, up := range schedule {
		if up.Migration == nil {
			continue
		}
		newAV, err := actorstypes.VersionForNetwork(up.Network)
		if err != nil || up.Network == 0 {
			continue
		}
		prevAV, err := actorstypes.VersionForNetwork(up.Network - 1)
		if err != nil || newAV <= prevAV || prevAV < minActorsVersion {
			continue
		}
		out = append(out, bundleTransition{
			label:     fmt.Sprintf("v%d->v%d (nv%d)", prevAV, newAV, up.Network),
			genesisNV: up.Network - 1,
			upgradeNV: up.Network,
			migration: up.Migration,
		})
	}
	return out
}

// TestFEVMCanaryBundleCompare is a diagnostic, not an assertion test. It
// runs the canary message under the previous actors bundle, triggers an
// in-chain migration to the current bundle, and runs it again, printing a
// per-bucket gas delta. Use this to judge whether a canary failure is
// normal bundle drift or an anomaly.
func TestFEVMCanaryBundleCompare(t *testing.T) {
	transitions := bundleTransitionsFromSchedule()
	require.NotEmpty(t, transitions, "no bundle transitions discovered from upgrade schedule")
	measureBundleTransition(t, transitions[len(transitions)-1])
}

// TestFEVMCanaryBundleCompareHistorical iterates through every bundle
// transition from v11 onward and prints the per-iteration gas delta for each.
// Use when regenerating the "Historical drift" table in this file's header.
func TestFEVMCanaryBundleCompareHistorical(t *testing.T) {
	if os.Getenv("LOTUS_TEST_FEVM_CANARY_HISTORICAL") == "" {
		t.Skip("set LOTUS_TEST_FEVM_CANARY_HISTORICAL=1 to run")
	}
	for _, tr := range bundleTransitionsFromSchedule() {
		t.Run(tr.label, func(t *testing.T) {
			measureBundleTransition(t, tr)
		})
	}
}

// measureBundleTransition runs the canary message under the pre-migration
// bundle and again under the post-migration bundle, logging a per-bucket
// delta table. Never asserts on gas values.
func measureBundleTransition(t *testing.T, tr bundleTransition) {
	kit.QuietMiningLogs()

	const upgradeHeight = abi.ChainEpoch(40)

	client, _, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Height:  -1,
			Network: tr.genesisNV,
		}, stmgr.Upgrade{
			Height:    upgradeHeight,
			Network:   tr.upgradeNV,
			Migration: tr.migration,
		}),
	)
	ens.InterconnectAll().BeginMining(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/RecCall.hex")

	t.Logf("running canary under previous actors bundle (nv%d)", tr.genesisNV)
	beforeTrace, exit := runRecursiveActorCall(ctx, t, client, fromAddr, actorAddr, 0, bundleCompareRecursionCount)
	require.Equal(t, exitcode.Ok, exit, "sub-boundary canary run should pass under previous bundle")

	t.Logf("waiting for network upgrade to nv%d at height %d", tr.upgradeNV, upgradeHeight)
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeHeight+5))

	t.Logf("running canary under post-upgrade actors bundle (nv%d)", tr.upgradeNV)
	afterTrace, exit := runRecursiveActorCall(ctx, t, client, fromAddr, actorAddr, 0, bundleCompareRecursionCount)
	require.Equal(t, exitcode.Ok, exit, "sub-boundary canary run should pass under current bundle")

	t.Logf("\n=== %s: stackDepth=0, recursionLimit=%d ===", tr.label, bundleCompareRecursionCount)
	logBundleCompare(t, beforeTrace, afterTrace, bundleCompareRecursionCount)
}

// --- helpers ---

// runRecursiveActorCall invokes exec1(uint256,uint256,uint256) on a RecCall
// contract, waits for the message, and returns its execution trace plus the
// receipt exit code. Unlike the kit's ExpectExit helper, this returns both
// the exit code and the trace so the caller can decide what to do on a
// boundary mismatch.
func runRecursiveActorCall(ctx context.Context, t *testing.T, client *kit.TestFullNode, from, contract address.Address, stackDepth, recursionLimit int) (types.ExecutionTrace, exitcode.ExitCode) {
	t.Helper()
	input := make([]byte, 32*3)
	binary.BigEndian.PutUint64(input[24:], uint64(stackDepth))
	binary.BigEndian.PutUint64(input[32+24:], uint64(stackDepth))
	binary.BigEndian.PutUint64(input[32+32+24:], uint64(recursionLimit))

	entry := kit.CalcFuncSignature("exec1(uint256,uint256,uint256)")
	wait, err := client.EVM().InvokeSolidity(ctx, from, contract, entry, input)
	require.NoError(t, err)
	replay, err := client.StateReplay(ctx, types.EmptyTSK, wait.Message)
	require.NoError(t, err)
	return replay.ExecutionTrace, wait.Receipt.ExitCode
}

// runRecursiveDelegatecall invokes recursiveCall(uint256) on a
// RecursiveDelegatecall contract and then queries totalCalls() to find out
// how many iterations actually ran. Returns the recursiveCall trace, the
// outer-call receipt exit code, and the observed iteration count.
func runRecursiveDelegatecall(ctx context.Context, t *testing.T, client *kit.TestFullNode, from, contract address.Address, n uint64) (types.ExecutionTrace, exitcode.ExitCode, uint64, error) {
	t.Helper()
	input := kit.EvmWordUint64(n)
	entry := kit.CalcFuncSignature("recursiveCall(uint256)")
	wait, err := client.EVM().InvokeSolidity(ctx, from, contract, entry, input)
	if err != nil {
		return types.ExecutionTrace{}, 0, 0, err
	}
	replay, err := client.StateReplay(ctx, types.EmptyTSK, wait.Message)
	if err != nil {
		return types.ExecutionTrace{}, 0, 0, err
	}
	result, _, err := client.EVM().InvokeContractByFuncName(ctx, from, contract, "totalCalls()", []byte{})
	if err != nil {
		return replay.ExecutionTrace, wait.Receipt.ExitCode, 0, err
	}
	total, err := kit.EvmDecodeUint64(result)
	if err != nil {
		return replay.ExecutionTrace, wait.Receipt.ExitCode, 0, err
	}
	return replay.ExecutionTrace, wait.Receipt.ExitCode, total, nil
}

// canaryFailure bundles the context needed to produce a useful failure
// report.
type canaryFailure struct {
	testName       string
	caseLabel      string
	recursionCount int
	expectedExit   exitcode.ExitCode
	actualExit     exitcode.ExitCode
	trace          types.ExecutionTrace
	perIterApprox  int64
	extraReason    string
}

// reportCanaryFailure prints a per-bucket gas breakdown, computes the per-
// iteration cost and compares against the stored approximation, and gives a
// one-line hint on whether this looks like normal drift or an anomaly.
// After reporting, it fails the test.
func reportCanaryFailure(t *testing.T, f canaryFailure) {
	t.Helper()

	tallies, total := gasstats.Aggregate(f.trace, true)
	top := gasstats.TopN(tallies, 8)

	var perIter int64
	if f.recursionCount > 0 {
		perIter = total.Total() / int64(f.recursionCount)
	}

	t.Logf("\n=== CANARY FAILURE: %s / %s ===", f.testName, f.caseLabel)
	if f.extraReason != "" {
		t.Logf("reason: %s", f.extraReason)
	} else {
		t.Logf("expected exit %s, got %s", f.expectedExit, f.actualExit)
	}
	t.Logf("total gas: %d (compute %d, storage %d, charges %d)", total.Total(), total.ComputeGas, total.StorageGas, total.Count)
	t.Logf("per-iteration gas (total / %d iterations): ~%d", f.recursionCount, perIter)
	if f.perIterApprox > 0 && perIter > 0 {
		deltaPct := float64(perIter-f.perIterApprox) / float64(f.perIterApprox) * 100
		t.Logf("stored approx: ~%d; drift: %+.1f%%", f.perIterApprox, deltaPct)
	}
	t.Logf("top gas buckets:")
	for _, tally := range top {
		pct := float64(tally.Total()) / float64(total.Total()) * 100
		t.Logf("  %-28s count=%-6d total=%-12d (%5.1f%%)", tally.Name, tally.Count, tally.Total(), pct)
	}

	t.Logf("\ninterpretation:")
	t.Logf("  Run TestFEVMCanaryBundleCompare to see the per-bucket delta")
	t.Logf("  vs the previous bundle. Normal bundle drift is 100%% in wasm_exec")
	t.Logf("  Refer to this file's header for more information and investigate")
	t.Logf("  before a possible mechanical retune of the boundary constants.")

	t.Fatalf("canary boundary tripped: see diagnostic above")
}

// logBundleCompare prints side-by-side per-bucket gas for two traces of the
// same message under different actors bundles, plus deltas. Sorted by
// absolute delta descending so the drifting buckets rise to the top.
func logBundleCompare(t *testing.T, before, after types.ExecutionTrace, iterCount int) {
	t.Helper()

	beforeTallies, totalBefore := gasstats.Aggregate(before, true)
	afterTallies, totalAfter := gasstats.Aggregate(after, true)

	byName := map[string]struct{ before, after int64 }{}
	for _, t := range beforeTallies {
		e := byName[t.Name]
		e.before = t.Total()
		byName[t.Name] = e
	}
	for _, t := range afterTallies {
		e := byName[t.Name]
		e.after = t.Total()
		byName[t.Name] = e
	}

	type row struct {
		name          string
		before, after int64
		delta         int64
	}
	rows := make([]row, 0, len(byName))
	for name, v := range byName {
		rows = append(rows, row{name, v.before, v.after, v.after - v.before})
	}
	sort.Slice(rows, func(i, j int) bool { return abs(rows[i].delta) > abs(rows[j].delta) })

	t.Logf("%-28s %14s %14s %14s", "Bucket", "Before", "After", "Delta")
	for _, r := range rows {
		t.Logf("%-28s %14d %14d %+14d", r.name, r.before, r.after, r.delta)
	}
	t.Logf("%-28s %14d %14d %+14d", "TOTAL", totalBefore.Total(), totalAfter.Total(), totalAfter.Total()-totalBefore.Total())
	if iterCount > 0 {
		t.Logf("per-iteration delta (%d iterations): %+d", iterCount, (totalAfter.Total()-totalBefore.Total())/int64(iterCount))
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

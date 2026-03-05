package gasutils

import (
	"context"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	lbuiltin "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("node/gasutils")

const MinGasPremium = 100e3
const MaxSpendOnFeeDenom = 100

type StateManagerAPI interface {
	ParentState(ts *types.TipSet) (*state.StateTree, error)
	CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, applyTsMessages bool) (*api.InvocResult, error)
	ResolveToDeterministicAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)
}

type ChainStoreAPI interface {
	GetHeaviestTipSet() *types.TipSet
	GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
}

type MessagePoolAPI interface {
	PendingFor(ctx context.Context, a address.Address) ([]*types.SignedMessage, *types.TipSet)
	Pending(ctx context.Context) ([]*types.SignedMessage, *types.TipSet)
}

type GasMeta struct {
	Price big.Int
	Limit int64
}

// GasEstimateCallWithGas invokes a message "msgIn" on the earliest available tipset with pending
// messages in the message pool. The function returns the result of the message invocation, the
// pending messages, the tipset used for the invocation, and an error if occurred.
// The returned information can be used to make subsequent calls to CallWithGas with the same parameters.
func GasEstimateCallWithGas(
	ctx context.Context,
	cstore ChainStoreAPI,
	smgr StateManagerAPI,
	mpool MessagePoolAPI,
	msgIn *types.Message,
	currTs *types.TipSet,
) (*api.InvocResult, []types.ChainMsg, *types.TipSet, error) {
	msg := *msgIn
	fromA, err := smgr.ResolveToDeterministicAddress(ctx, msgIn.From, currTs)
	if err != nil {
		return nil, []types.ChainMsg{}, nil, xerrors.Errorf("getting key address: %w", err)
	}

	pending, ts := mpool.PendingFor(ctx, fromA)
	priorMsgs := make([]types.ChainMsg, 0, len(pending))
	for _, m := range pending {
		if m.Message.Nonce == msg.Nonce {
			break
		}
		priorMsgs = append(priorMsgs, m)
	}

	applyTsMessages := true
	if os.Getenv("LOTUS_SKIP_APPLY_TS_MESSAGE_CALL_WITH_GAS") == "1" {
		applyTsMessages = false
	}

	// Try calling until we find a height with no migration.
	var res *api.InvocResult
	for {
		res, err = smgr.CallWithGas(ctx, &msg, priorMsgs, ts, applyTsMessages)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = cstore.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, []types.ChainMsg{}, nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}
	if err != nil {
		return nil, []types.ChainMsg{}, nil, xerrors.Errorf("CallWithGas failed: %w", err)
	}

	return res, priorMsgs, ts, nil
}

func GasEstimateGasLimit(
	ctx context.Context,
	cstore ChainStoreAPI,
	smgr StateManagerAPI,
	mpool *messagepool.MessagePool,
	msgIn *types.Message,
	currTs *types.TipSet,
) (int64, error) {
	msg := *msgIn
	msg.GasLimit = buildconstants.BlockGasLimit
	msg.GasFeeCap = big.Zero()
	msg.GasPremium = big.Zero()

	res, _, ts, err := GasEstimateCallWithGas(ctx, cstore, smgr, mpool, &msg, currTs)
	if err != nil {
		return -1, xerrors.Errorf("gas estimation failed: %w", err)
	}

	if res.MsgRct.ExitCode == exitcode.SysErrOutOfGas {
		return -1, &api.ErrOutOfGas{}
	}

	if res.MsgRct.ExitCode != exitcode.Ok {
		return -1, api.NewErrExecutionRevertedFromResult(res)
	}

	ret := res.MsgRct.GasUsed

	log.Debugw("GasEstimateGasLimit CallWithGas Result", "GasUsed", ret, "ExitCode", res.MsgRct.ExitCode)

	transitionalMulti := 1.0
	// Overestimate gas around the upgrade
	if ts.Height() <= buildconstants.UpgradeHyggeHeight && (buildconstants.UpgradeHyggeHeight-ts.Height() <= 20) {
		func() {

			// Bare transfers get about 3x more expensive: https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0057.md#product-considerations
			if msgIn.Method == builtin.MethodSend {
				transitionalMulti = 3.0
				return
			}

			st, err := smgr.ParentState(ts)
			if err != nil {
				return
			}
			act, err := st.GetActor(msg.To)
			if err != nil {
				return
			}

			if lbuiltin.IsStorageMinerActor(act.Code) {
				switch msgIn.Method {
				case 3:
					transitionalMulti = 1.92
				case 4:
					transitionalMulti = 1.72
				case 6:
					transitionalMulti = 1.06
				case 7:
					transitionalMulti = 1.2
				case 16:
					transitionalMulti = 1.19
				case 18:
					transitionalMulti = 1.73
				case 23:
					transitionalMulti = 1.73
				case 26:
					transitionalMulti = 1.15
				case 27:
					transitionalMulti = 1.18
				default:
				}
			}
		}()
	}
	ret = (ret * int64(transitionalMulti*1024)) >> 10

	return ret, nil
}

func GasEstimateFeeCap(ctx context.Context, cstore ChainStoreAPI, msg *types.Message, maxqueueblks int64, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := cstore.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return types.BigInt{}, xerrors.Errorf("getting tipset from key: %w", err)
	}

	parentBaseFee := ts.Blocks()[0].ParentBaseFee
	increaseFactor := math.Pow(1.+1./float64(buildconstants.BaseFeeMaxChangeDenom), float64(maxqueueblks))

	feeInFuture := types.BigMul(parentBaseFee, types.NewInt(uint64(increaseFactor*(1<<8))))
	out := types.BigDiv(feeInFuture, types.NewInt(1<<8))

	if msg.GasPremium != types.EmptyInt {
		out = types.BigAdd(out, msg.GasPremium)
	}

	return out, nil
}

// GasEstimateGasPremiumFromMempool implements the gas premium estimation algorithm from
// FIP-0115. It analyzes the current mempool state and models future transaction arrivals to
// recommend the minimum premium needed for inclusion within nblocksincl blocks.
//
// Algorithm (per FIP-0115):
//  1. k = gaslimit / BlockGasLimit
//  2. Initialize result to safe maximum
//  3. Build distribution from mempool once (dedup by sender+nonce)
//  4. Scale above-baseFee portion by fraction of current epoch remaining
//  5. Premium_k = k-th percentile of top BlockGasLimit gas in scaled distribution
//  6. result = min(result, Premium_k)
//  7. If nblocksincl < 2, return result
//  8. Else: simulate block, remove top txs, recalculate baseFee, repeat from 4
func GasEstimateGasPremiumFromMempool(
	ctx context.Context,
	cstore ChainStoreAPI,
	mpool MessagePoolAPI,
	nblocksincl uint64,
	gaslimit int64,
	tsKey types.TipSetKey,
) (types.BigInt, error) {
	if nblocksincl == 0 {
		nblocksincl = 1
	} else if nblocksincl > 10 {
		nblocksincl = 10
	}
	if gaslimit <= 0 {
		gaslimit = buildconstants.BlockGasLimit / 5
	}
	if gaslimit > buildconstants.BlockGasLimit {
		gaslimit = buildconstants.BlockGasLimit
	}

	// Step 1: k = GasLimit_i / BlockGasLimit
	k := float64(gaslimit) / float64(buildconstants.BlockGasLimit)

	ts, err := cstore.GetTipSetFromKey(ctx, tsKey)
	if err != nil {
		return types.BigInt{}, xerrors.Errorf("getting tipset from key: %w", err)
	}
	baseFee := ts.Blocks()[0].ParentBaseFee
	initialBaseFee := baseFee

	// Step 2: Initialize result to safe maximum (will be reduced by min in the loop)
	result := abi.NewTokenAmount(math.MaxInt64)

	// Step 3: Build the distribution once from the mempool, deduplicating by sender+nonce.
	// We keep GasFeeCap and GasPremium so we can recompute effective premiums as baseFee
	// changes during multi-block simulation. originalGasLimit is preserved so that step 4
	// can add a fixed amount of modeled gas per epoch, ensuring linear rather than
	// exponential growth in modeled competition over multiple simulated blocks.
	type rawMsg struct {
		gasLimit         int64
		originalGasLimit int64
		gasFeeCap        abi.TokenAmount
		gasPremium       abi.TokenAmount
	}
	pendingMsgs, _ := mpool.Pending(ctx)
	seen := make(map[store.SenderNonce]struct{}, len(pendingMsgs))
	distribution := make([]rawMsg, 0, len(pendingMsgs))
	for _, sm := range pendingMsgs {
		m := &sm.Message
		sn := store.SenderNonce{Sender: m.From, Nonce: m.Nonce}
		if _, ok := seen[sn]; !ok {
			seen[sn] = struct{}{}
			distribution = append(distribution, rawMsg{
				gasLimit:         m.GasLimit,
				originalGasLimit: m.GasLimit,
				gasFeeCap:        m.GasFeeCap,
				gasPremium:       m.GasPremium,
			})
		}
	}

	// Fraction of the current epoch remaining, used to model incoming transactions.
	// For subsequent simulated blocks (future epochs) we use 1.0, since a full
	// epoch of additional transactions is expected.
	fractionRemaining := epochFractionRemaining(ts)

	for {
		// Step 4 (mempool portion): compute effective premiums, then apply the incoming-gas
		// budget to the top of the distribution. Concentrating new arrivals at competitive
		// premium levels (rather than scaling all messages proportionally) better models
		// the actual arrival pattern: new transactions bid at or above market rate.
		premiums := make([]abi.TokenAmount, 0, len(distribution)+1)
		limits := make([]int64, 0, len(distribution)+1)

		effP := make([]abi.TokenAmount, len(distribution))
		for i := range distribution {
			effP[i] = mempoolEffectivePremium(distribution[i].gasFeeCap, distribution[i].gasPremium, baseFee)
		}

		// Apply a fixed incoming-gas budget to the highest-premium non-zombie messages.
		// Budget = fractionRemaining * BlockGasLimit: one block-worth of arriving gas per
		// epoch, scaled by how much of the epoch remains.
		if fractionRemaining > 0 {
			idxs := make([]int, 0, len(distribution))
			for i := range distribution {
				if effP[i].Sign() > 0 {
					idxs = append(idxs, i)
				}
			}
			sort.Slice(idxs, func(a, b int) bool {
				return effP[idxs[a]].GreaterThan(effP[idxs[b]])
			})
			budget := int64(fractionRemaining * float64(buildconstants.BlockGasLimit))
			for _, idx := range idxs {
				if budget <= 0 {
					break
				}
				add := distribution[idx].originalGasLimit
				if add > budget {
					add = budget
				}
				distribution[idx].gasLimit += add
				budget -= add
			}
		}

		for i := range distribution {
			premiums = append(premiums, effP[i])
			limits = append(limits, distribution[i].gasLimit)
		}

		// Step 4 (market-rate portion): append a synthetic entry at baseFee*R to the
		// model distribution, representing half a remaining epoch's worth of new
		// transactions at the equilibrium rate. It participates in step 5 and step 8
		// equally with real messages and may be selected into the simulated block.
		if fractionRemaining > 0 {
			incomingGas := int64(fractionRemaining / 2 * float64(buildconstants.BlockGasLimit))
			if incomingGas > 0 {
				incomingPremium := big.Div(baseFee, big.NewInt(buildconstants.BaseFeeMaxChangeDenom))
				distribution = append(distribution, rawMsg{
					gasLimit:         incomingGas,
					originalGasLimit: incomingGas,
					gasFeeCap:        big.Add(baseFee, incomingPremium),
					gasPremium:       incomingPremium,
				})
				premiums = append(premiums, incomingPremium)
				limits = append(limits, incomingGas)
			}
		}

		// Step 5: Premium_k = premium at position (1-k)*BlockGasLimit from the top.
		// A transaction of size k*BlockGasLimit needs to outbid the (1-k)*BlockGasLimit
		// gas worth of higher-premium transactions that would fill the block ahead of it.
		// WeightedQuickSelect finds this without requiring a sorted distribution.
		position := int64((1 - k) * float64(buildconstants.BlockGasLimit))
		premiumK := store.WeightedQuickSelect(premiums, limits, position)

		// Step 6: result = min(result, Premium_k)
		if big.Cmp(premiumK, result) < 0 {
			result = premiumK
		}

		// Step 7: return if we only need one more block
		if nblocksincl < 2 {
			break
		}
		nblocksincl--
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Step 8: Simulate block selection using the already-computed premiums and limits.
		// Sort indices by effective premium descending to select the top messages.
		indices := make([]int, len(distribution))
		for i := range indices {
			indices[i] = i
		}
		sort.Slice(indices, func(a, b int) bool {
			return premiums[indices[a]].GreaterThan(premiums[indices[b]])
		})

		blockPremiums := make([]abi.TokenAmount, 0, len(distribution))
		blockLimits := make([]int64, 0, len(distribution))
		var blockGasUsed int64
		inBlock := make([]bool, len(distribution))
		for _, idx := range indices {
			if blockGasUsed+limits[idx] <= buildconstants.BlockGasLimit {
				blockPremiums = append(blockPremiums, premiums[idx])
				blockLimits = append(blockLimits, limits[idx])
				blockGasUsed += limits[idx]
				inBlock[idx] = true
			}
		}

		// Compact distribution: remove messages selected into the simulated block.
		j := 0
		for i := range distribution {
			if !inBlock[i] {
				distribution[j] = distribution[i]
				j++
			}
		}
		distribution = distribution[:j]

		// Recalculate baseFee from the simulated block's premium distribution.
		percentilePremium := store.WeightedQuickSelect(blockPremiums, blockLimits, buildconstants.BlockGasTargetIndex)
		baseFee = store.NextBaseFeeFromPremium(baseFee, percentilePremium)

		// For future epochs, a full epoch of new transactions is expected.
		fractionRemaining = 1.0
	}

	// Suggest Pk+1 so the submitter outbids all existing messages at this premium level.
	result = big.Add(result, big.NewInt(1))

	// Floor: baseFee * R / 2, where R = 1/BaseFeeMaxChangeDenom.
	// A zero premium will not be proposed; this ensures a minimum viable miner incentive.
	minPremium := big.Div(initialBaseFee, big.NewInt(2*buildconstants.BaseFeeMaxChangeDenom))
	result = big.Max(result, minPremium)

	// Add small noise to distribute message selection across nodes and avoid ties.
	// Ceiling division ensures noise always rounds up, so it is never truncated to zero
	// for small premium values.
	const precision = 32
	divisor := types.NewInt(1 << precision)
	// mean 1, stddev 0.005 => 95% within +-1%
	noise := 1 + rand.NormFloat64()*0.005
	result = types.BigMul(result, types.NewInt(uint64(noise*(1<<precision))+1))
	result = types.BigDiv(types.BigAdd(result, types.BigSub(divisor, types.NewInt(1))), divisor)

	return result, nil
}

// mempoolEffectivePremium returns the effective gas premium for a mempool message given the
// current baseFee, clamped to max(0, feeCap - baseFee).
func mempoolEffectivePremium(feeCap, premium, baseFee abi.TokenAmount) abi.TokenAmount {
	available := big.Sub(feeCap, baseFee)
	if available.LessThan(big.Zero()) {
		return big.Zero()
	}
	if big.Cmp(premium, available) <= 0 {
		return premium
	}
	return available
}

// epochFractionRemaining returns the fraction [0,1] of the current epoch's block delay remaining.
// This is used to scale the above-baseFee mempool distribution to model incoming transactions.
func epochFractionRemaining(ts *types.TipSet) float64 {
	epochEnd := int64(ts.MinTimestamp()) + int64(buildconstants.BlockDelaySecs)
	remaining := epochEnd - time.Now().Unix()
	if remaining <= 0 {
		return 0
	}
	f := float64(remaining) / float64(buildconstants.BlockDelaySecs)
	if f > 1 {
		return 1
	}
	return f
}

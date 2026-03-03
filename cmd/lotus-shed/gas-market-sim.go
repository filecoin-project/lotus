package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/gasutils"
)

var gasMktSimCmd = &cli.Command{
	Name:  "gas-market-sim",
	Usage: "simulate FIP-0115 gas market dynamics over many epochs",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "epochs",
			Usage: "number of epochs to simulate",
			Value: 3600,
		},
		&cli.Int64Flag{
			Name:  "seed",
			Usage: "random seed for reproducibility",
			Value: 42,
		},
		&cli.BoolFlag{
			Name:  "epoch-remaining-zero",
			Usage: "fix epochFractionRemaining to 0 for all transactions (disables time-based scaling)",
		},
	},
	Action: runGasMktSim,
}

// simChainStore implements gasutils.ChainStoreAPI backed by a single in-memory TipSet.
type simChainStore struct{ ts *types.TipSet }

func (s *simChainStore) GetHeaviestTipSet() *types.TipSet { return s.ts }
func (s *simChainStore) GetTipSetFromKey(_ context.Context, _ types.TipSetKey) (*types.TipSet, error) {
	return s.ts, nil
}
func (s *simChainStore) LoadTipSet(_ context.Context, _ types.TipSetKey) (*types.TipSet, error) {
	return s.ts, nil
}

// simMpool implements gasutils.MessagePoolAPI backed by an in-memory message slice.
type simMpool struct {
	msgs []*types.SignedMessage
	ts   *types.TipSet
}

func (m *simMpool) PendingFor(_ context.Context, _ address.Address) ([]*types.SignedMessage, *types.TipSet) {
	return nil, m.ts
}
func (m *simMpool) Pending(_ context.Context) ([]*types.SignedMessage, *types.TipSet) {
	return m.msgs, m.ts
}

// buildSimTipSet constructs a single-block TipSet with the given baseFee and timestamp.
func buildSimTipSet(baseFee abi.TokenAmount, timestamp uint64) *types.TipSet {
	miner, err := address.NewIDAddress(1)
	if err != nil {
		panic(err)
	}
	dummyCid, err := cid.Decode("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")
	if err != nil {
		panic(err)
	}
	blk := &types.BlockHeader{
		Miner:                 miner,
		Ticket:                &types.Ticket{VRFProof: []byte{0}},
		ElectionProof:         &types.ElectionProof{VRFProof: []byte{0}},
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte{0}},
		ParentStateRoot:       dummyCid,
		ParentMessageReceipts: dummyCid,
		Messages:              dummyCid,
		ParentBaseFee:         baseFee,
		Timestamp:             timestamp,
	}
	ts, err := types.NewTipSet([]*types.BlockHeader{blk})
	if err != nil {
		panic(err)
	}
	return ts
}

// simEffectivePremium returns the effective gas premium clamped to max(0, feeCap-baseFee).
func simEffectivePremium(feeCap, premium, baseFee abi.TokenAmount) abi.TokenAmount {
	available := big.Sub(feeCap, baseFee)
	if available.LessThan(big.Zero()) {
		return big.Zero()
	}
	if big.Cmp(premium, available) <= 0 {
		return premium
	}
	return available
}

// simSampleGasLimit returns a gas limit drawn from a shifted-exponential distribution
// with approximate mean bgl/10, clamped to [bgl/120, 3*bgl/4].
// This gives an average of ~3 full blocks of gas per 30-transaction epoch.
func simSampleGasLimit(rng *rand.Rand, bgl int64) int64 {
	min := bgl / 120
	max := bgl * 3 / 4
	// E[min + Exp(meanExp)] = min + meanExp = bgl/10 → meanExp = 11*bgl/120
	meanExp := float64(bgl) * 11.0 / 120.0
	for {
		sample := min + int64(rng.ExpFloat64()*meanExp)
		if sample <= max {
			return sample
		}
	}
}

// simSelectMessages fills one block for a proposer with the given ticket quality,
// selecting independently from the full mempool without knowledge of other proposers.
func simSelectMessages(msgs []*types.SignedMessage, effPremiums []abi.TokenAmount, tq float64, bgl int64) []bool {
	n := len(msgs)
	chosen := make([]bool, n)
	if n == 0 {
		return chosen
	}

	gasPerfs := make([]float64, n)
	for i := range msgs {
		gasPerfs[i] = float64(effPremiums[i].Int64()) * float64(bgl)
	}

	idxs := make([]int, n)
	for i := range idxs {
		idxs[i] = i
	}
	sort.Slice(idxs, func(a, b int) bool {
		return gasPerfs[idxs[a]] > gasPerfs[idxs[b]]
	})

	if tq > 0.84 {
		var blockGas int64
		for _, idx := range idxs {
			if gasPerfs[idx] < 0 {
				break
			}
			if blockGas+msgs[idx].Message.GasLimit <= bgl {
				chosen[idx] = true
				blockGas += msgs[idx].Message.GasLimit
			}
		}
		return chosen
	}

	blockProb := messagepool.BlockProbabilities(tq)

	partitions := make([]int, n)
	for i := range partitions {
		partitions[i] = len(blockProb)
	}
	part := 0
	var partGas int64
	for _, idx := range idxs {
		if gasPerfs[idx] < 0 {
			break
		}
		if part >= len(blockProb) {
			break
		}
		if partGas+msgs[idx].Message.GasLimit > bgl {
			part++
			partGas = 0
			if part >= len(blockProb) {
				break
			}
		}
		partitions[idx] = part
		partGas += msgs[idx].Message.GasLimit
	}

	effPerfs := make([]float64, n)
	for i := range effPerfs {
		if partitions[i] < len(blockProb) {
			effPerfs[i] = gasPerfs[i] * blockProb[partitions[i]]
		} else {
			effPerfs[i] = -1
		}
	}

	sort.Slice(idxs, func(a, b int) bool {
		return effPerfs[idxs[a]] > effPerfs[idxs[b]]
	})
	var blockGas int64
	for _, idx := range idxs {
		if effPerfs[idx] < 0 {
			break
		}
		if blockGas+msgs[idx].Message.GasLimit <= bgl {
			chosen[idx] = true
			blockGas += msgs[idx].Message.GasLimit
		}
	}
	return chosen
}

// runGasMktSim runs the gas market simulation.
// Every epoch:
//   - 30 transactions are inserted with premiums estimated via GasEstimateGasPremiumFromMempool
//     (nblocksincl bimodal: p(1)=0.1, p(2)=0.2, p(10)=0.7), gas limits drawn from a
//     shifted-exponential averaging bgl/10 (≈3 full blocks of gas per epoch)
//   - Each tx has a deadline = submission epoch + nblocksincl; late txs are counted
//   - 5 independent proposers each select messages with a random tq, unaware of each other;
//     the tipset is the union of their selections
//   - BaseFee is updated per FIP-0115 via NextBaseFeeFromPremium
func runGasMktSim(cctx *cli.Context) error {
	epochs := cctx.Int("epochs")
	seed := cctx.Int64("seed")
	epochRemainingZero := cctx.Bool("epoch-remaining-zero")

	bgl := buildconstants.BlockGasLimit
	rng := rand.New(rand.NewSource(seed))
	ctx := context.Background()

	baseFee := big.NewInt(buildconstants.MinimumBaseFee)
	mempool := make([]*types.SignedMessage, 0, 5000)
	deadlines := make([]int, 0, 5000)       // deadline[i] = epoch by which mempool[i] must be included
	nblocksincls := make([]uint64, 0, 5000) // nblocksincls[i] = nblocksincl used when submitting mempool[i]
	var nextSender uint64 = 2
	var totalGasUsed int64

	for epoch := 0; epoch < epochs; epoch++ {
		ts := buildSimTipSet(baseFee, 0)
		now := time.Now().Unix()
		for i := range 30 {
			var csTs *types.TipSet
			if epochRemainingZero {
				csTs = buildSimTipSet(baseFee, 0)
			} else {
				// Descending timestamps so epochFractionRemaining returns (30-i)/30:
				// the first tx sees a near-full epoch remaining, the last sees 1/30.
				csTs = buildSimTipSet(baseFee, uint64(now-int64(i)))
			}
			cs := &simChainStore{ts: csTs}
			mp := &simMpool{msgs: mempool, ts: ts}

			gl := simSampleGasLimit(rng, bgl)
			var nblocksincl uint64
			switch r := rng.Float64(); {
			case r < 0.1:
				nblocksincl = 1
			case r < 0.3:
				nblocksincl = 2
			default:
				nblocksincl = 10
			}
			premium, err := gasutils.GasEstimateGasPremiumFromMempool(ctx, cs, mp, nblocksincl, gl, types.EmptyTSK)
			if err != nil {
				return fmt.Errorf("epoch %d tx %d: %w", epoch, i, err)
			}

			addr, _ := address.NewIDAddress(nextSender)
			mempool = append(mempool, &types.SignedMessage{
				Message: types.Message{
					From:       addr,
					To:         addr,
					Nonce:      0,
					GasLimit:   gl,
					GasPremium: premium,
					GasFeeCap:  big.Add(baseFee, premium),
				},
			})
			deadlines = append(deadlines, epoch+int(nblocksincl))
			nblocksincls = append(nblocksincls, nblocksincl)
			nextSender++
		}

		effPremiums := make([]abi.TokenAmount, len(mempool))
		for i, m := range mempool {
			effPremiums[i] = simEffectivePremium(m.Message.GasFeeCap, m.Message.GasPremium, baseFee)
		}

		selectedByAny := make([]bool, len(mempool))
		tipPremiums := make([]abi.TokenAmount, 0, len(mempool))
		tipLimits := make([]int64, 0, len(mempool))
		for range 5 {
			tq := rng.Float64()
			chosen := simSelectMessages(mempool, effPremiums, tq, bgl)
			for i, s := range chosen {
				if s && !selectedByAny[i] {
					selectedByAny[i] = true
					tipPremiums = append(tipPremiums, effPremiums[i])
					tipLimits = append(tipLimits, mempool[i].Message.GasLimit)
				}
			}
		}

		percentilePremium := store.WeightedQuickSelect(tipPremiums, tipLimits, buildconstants.BlockGasTargetIndex)
		baseFee = store.NextBaseFeeFromPremium(baseFee, percentilePremium)

		j := 0
		for i, m := range mempool {
			if !selectedByAny[i] {
				mempool[j] = m
				deadlines[j] = deadlines[i]
				nblocksincls[j] = nblocksincls[i]
				j++
			}
		}
		mempool = mempool[:j]
		deadlines = deadlines[:j]
		nblocksincls = nblocksincls[:j]

		var tipGasUsed int64
		for _, l := range tipLimits {
			tipGasUsed += l
		}
		totalGasUsed += tipGasUsed
		tipGasLimit := int64(5) * bgl

		var late1, late2, late10 int
		for i, d := range deadlines {
			if epoch >= d {
				switch nblocksincls[i] {
				case 1:
					late1++
				case 2:
					late2++
				default:
					late10++
				}
			}
		}
		lateCount := late1 + late2 + late10

		fmt.Printf("epoch %4d: baseFee=%15s  mempoolSize=%5d  late=%4d (nb1=%d nb2=%d nb10=%d)  tipGas=%d/%d (%.1f%%)\n",
			epoch+1, baseFee, len(mempool), lateCount, late1, late2, late10,
			tipGasUsed, tipGasLimit, float64(tipGasUsed)/float64(tipGasLimit)*100)
	}

	totalGasLimit := int64(epochs) * int64(5) * bgl
	fmt.Printf("=== SUMMARY: total gas used %d / %d (%.1f%%)\n",
		totalGasUsed, totalGasLimit,
		float64(totalGasUsed)/float64(totalGasLimit)*100)

	return nil
}

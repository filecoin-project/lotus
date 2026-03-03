package gasutils

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

// mockChainStore implements ChainStoreAPI for tests.
type mockChainStore struct{ ts *types.TipSet }

func (m *mockChainStore) GetHeaviestTipSet() *types.TipSet { return m.ts }
func (m *mockChainStore) GetTipSetFromKey(_ context.Context, _ types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStore) LoadTipSet(_ context.Context, _ types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}

// mockMpool implements MessagePoolAPI for tests.
type mockMpool struct {
	msgs []*types.SignedMessage
	ts   *types.TipSet
}

func (m *mockMpool) PendingFor(_ context.Context, _ address.Address) ([]*types.SignedMessage, *types.TipSet) {
	return nil, m.ts
}
func (m *mockMpool) Pending(_ context.Context) ([]*types.SignedMessage, *types.TipSet) {
	return m.msgs, m.ts
}

// makeTipSet builds a single-block tipset with the given ParentBaseFee and timestamp.
// The timestamp controls epochFractionRemaining: set to time.Now().Unix()-i so that
// a tx submitted as the i-th of 30 within an epoch sees fractionRemaining=(30-i)/30.
func makeTipSet(t *testing.T, baseFee abi.TokenAmount, timestamp uint64) *types.TipSet {
	t.Helper()
	miner, err := address.NewIDAddress(1)
	require.NoError(t, err)
	dummyCid, err := cid.Decode("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")
	require.NoError(t, err)
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
	require.NoError(t, err)
	return ts
}

// makeMsg creates a signed message stub with known gas parameters.
func makeMsg(sender uint64, nonce uint64, gasLimit int64, premium abi.TokenAmount, baseFee abi.TokenAmount) *types.SignedMessage {
	addr, err := address.NewIDAddress(sender)
	if err != nil {
		panic(err)
	}
	return &types.SignedMessage{
		Message: types.Message{
			From:       addr,
			To:         addr,
			Nonce:      nonce,
			GasLimit:   gasLimit,
			GasPremium: premium,
			GasFeeCap:  big.Add(baseFee, premium),
		},
	}
}

// TestMempoolEffectivePremium verifies the effective premium clamping logic.
func TestMempoolEffectivePremium(t *testing.T) {
	tests := []struct {
		name     string
		feeCap   int64
		premium  int64
		baseFee  int64
		expected int64
	}{
		{"premium below available", 100, 10, 80, 10},
		{"premium equals available", 100, 20, 80, 20},
		{"premium exceeds available, clamp to feeCap-baseFee", 100, 50, 80, 20},
		{"feeCap below baseFee, zero", 50, 10, 80, 0},
		{"feeCap equals baseFee, zero", 80, 10, 80, 0},
		{"zero baseFee, full premium", 100, 30, 0, 30},
		{"zero premium", 100, 0, 80, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mempoolEffectivePremium(big.NewInt(tc.feeCap), big.NewInt(tc.premium), big.NewInt(tc.baseFee))
			assert.Equal(t, big.NewInt(tc.expected).String(), got.String())
		})
	}
}

// TestPremiumAtGasPosition verifies finding the premium at a given cumulative gas position
// from the top of a descending distribution, capped at BlockGasLimit.
func TestPremiumAtGasPosition(t *testing.T) {
	bgl := buildconstants.BlockGasLimit // 10_000_000_000

	tests := []struct {
		name         string
		distribution []GasMeta
		position     int64
		expected     int64
	}{
		{
			"empty distribution returns zero",
			nil, 1, 0,
		},
		{
			"single entry covers position",
			[]GasMeta{{big.NewInt(100), bgl / 2}},
			bgl / 4, 100,
		},
		{
			"position at boundary of first entry",
			[]GasMeta{{big.NewInt(100), bgl / 4}, {big.NewInt(50), bgl / 2}},
			bgl / 4, 100,
		},
		{
			"position falls in second entry",
			[]GasMeta{{big.NewInt(100), bgl / 4}, {big.NewInt(50), bgl / 2}},
			bgl/4 + 1, 50,
		},
		{
			"position beyond total gas returns zero",
			[]GasMeta{{big.NewInt(100), bgl / 4}},
			bgl / 2, 0,
		},
		{
			"distribution exceeds BlockGasLimit, cap applied",
			// first entry fills 60% of block, second fills 60% → total would be 120% but capped
			// position at 70% should fall in second entry
			[]GasMeta{{big.NewInt(100), bgl * 6 / 10}, {big.NewInt(50), bgl * 6 / 10}},
			bgl * 7 / 10, 50,
		},
		{
			"distribution exceeds BlockGasLimit, position past cap returns zero",
			[]GasMeta{{big.NewInt(100), bgl * 6 / 10}, {big.NewInt(50), bgl * 6 / 10}},
			bgl + 1, 0,
		},
		{
			"position zero returns first entry price",
			[]GasMeta{{big.NewInt(200), 1}, {big.NewInt(100), bgl}},
			0, 200,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := premiumAtGasPosition(tc.distribution, tc.position)
			assert.Equal(t, big.NewInt(tc.expected).String(), got.String())
		})
	}
}

// TestGasEstimateGasPremiumFromMempoolGasLimitEffect verifies that a larger gaslimit produces
// a higher recommended premium, since the transaction competes for more block space.
//
// The mempool has 10 transactions each consuming 10% of the block, with premiums
// 1000, 900, ..., 100. With timestamp=0 the epoch fraction remaining is 0 so no
// future-arrival scaling occurs, making outcomes deterministic up to the ±1% noise term.
//
// Expected premiums (before noise):
//
//	k=0.1 → position=(1-0.1)*BGL=9B → accumulate tx1..tx9 → premium of tx9 = 200
//	k=0.5 → position=(1-0.5)*BGL=5B → accumulate tx1..tx5 → premium of tx5 = 600
//	k=0.9 → position=(1-0.9)*BGL=1B → accumulate tx1       → premium of tx1 = 1000
func TestGasEstimateGasPremiumFromMempoolGasLimitEffect(t *testing.T) {
	bgl := buildconstants.BlockGasLimit
	baseFee := big.NewInt(buildconstants.MinimumBaseFee)
	ts := makeTipSet(t, baseFee, 0)

	// 10 transactions, premiums 1000 down to 100, each using 10% of the block.
	msgs := make([]*types.SignedMessage, 10)
	for i := 0; i < 10; i++ {
		premium := big.NewInt(int64(1000 - i*100))
		msgs[i] = makeMsg(uint64(i+1), 0, bgl/10, premium, baseFee)
	}

	cs := &mockChainStore{ts: ts}
	mp := &mockMpool{msgs: msgs, ts: ts}
	ctx := context.Background()

	small, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, 1, bgl/10, types.EmptyTSK)
	require.NoError(t, err)

	medium, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, 1, bgl/2, types.EmptyTSK)
	require.NoError(t, err)

	large, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, 1, bgl*9/10, types.EmptyTSK)
	require.NoError(t, err)

	assert.True(t, small.LessThan(medium),
		"small tx (%s) should need lower premium than medium tx (%s)", small, medium)
	assert.True(t, medium.LessThan(large),
		"medium tx (%s) should need lower premium than large tx (%s)", medium, large)
}

// BenchmarkGasEstimateGasPremiumFromMempool measures estimation latency under a
// realistic full mempool with varied premiums across several inclusion targets.
//
// Run with: go test -bench=BenchmarkGasEstimateGasPremiumFromMempool -benchmem
func BenchmarkGasEstimateGasPremiumFromMempool(b *testing.B) {
	bgl := buildconstants.BlockGasLimit
	baseFee := big.NewInt(100e9) // 100 nanoFIL, a realistic mid-range base fee

	// Build a tipset with timestamp 0 so fractionRemaining=0 and results are deterministic.
	miner, _ := address.NewIDAddress(1)
	dummyCid, _ := cid.Decode("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")
	blk := &types.BlockHeader{
		Miner:                 miner,
		Ticket:                &types.Ticket{VRFProof: []byte{0}},
		ElectionProof:         &types.ElectionProof{VRFProof: []byte{0}},
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte{0}},
		ParentStateRoot:       dummyCid,
		ParentMessageReceipts: dummyCid,
		Messages:              dummyCid,
		ParentBaseFee:         baseFee,
		Timestamp:             0,
	}
	ts, _ := types.NewTipSet([]*types.BlockHeader{blk})

	// 5000 unique senders, each with one message of ~200k gas (50x block capacity),
	// premiums distributed across a wide range to stress the selection logic.
	const numMsgs = 5000
	msgs := make([]*types.SignedMessage, numMsgs)
	for i := 0; i < numMsgs; i++ {
		premium := big.NewInt(int64(100e3 + i*1000)) // 100k to 5.1M attoFIL
		msgs[i] = makeMsg(uint64(i+2), 0, bgl/50, premium, baseFee)
	}

	cs := &mockChainStore{ts: ts}
	mp := &mockMpool{msgs: msgs, ts: ts}
	ctx := context.Background()

	for _, nblocksincl := range []uint64{1, 3, 10} {
		b.Run(fmt.Sprintf("nblocksincl=%d", nblocksincl), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, nblocksincl, bgl/5, types.EmptyTSK)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestGasEstimateGasPremiumFromMempoolConvergence shows how the estimated premium
// decreases as nblocksincl increases, revealing when results converge to the floor
// (baseFee / (2 * BaseFeeMaxChangeDenom)). Run with -v to see the progression.
func TestGasEstimateGasPremiumFromMempoolConvergence(t *testing.T) {
	bgl := buildconstants.BlockGasLimit
	baseFee := big.NewInt(100_000_000_000) // 100 nanoFIL: floor ≈ 6.25e9, marketRate ≈ 12.5e9
	ts := makeTipSet(t, baseFee, 0)

	// 5000 senders, premiums linearly from 1e9 (below floor) to ~1e12 (10× baseFee),
	// spanning both uncompetitive and highly competitive territory.
	const numMsgs = 5000
	msgs := make([]*types.SignedMessage, numMsgs)
	for i := 0; i < numMsgs; i++ {
		premium := big.NewInt(1_000_000_000 + int64(i)*200_000_000)
		msgs[i] = makeMsg(uint64(i+2), 0, bgl/50, premium, baseFee)
	}

	cs := &mockChainStore{ts: ts}
	mp := &mockMpool{msgs: msgs, ts: ts}
	ctx := context.Background()

	floor := big.Div(baseFee, big.NewInt(2*buildconstants.BaseFeeMaxChangeDenom))
	marketRate := big.Div(baseFee, big.NewInt(buildconstants.BaseFeeMaxChangeDenom))
	t.Logf("floor      = %s attoFIL  (baseFee / %d)", floor, 2*buildconstants.BaseFeeMaxChangeDenom)
	t.Logf("marketRate = %s attoFIL  (baseFee / %d, synthetic entry premium)", marketRate, buildconstants.BaseFeeMaxChangeDenom)

	convergedAt := uint64(0)
	for n := uint64(1); n <= 30; n++ {
		p, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, n, bgl/5, types.EmptyTSK)
		require.NoError(t, err)

		// express as integer percentage of floor (100 = exactly floor, 200 = 2× floor)
		pct := big.Div(big.Mul(p, big.NewInt(100)), floor)

		marker := ""
		switch {
		case big.Cmp(p, floor) <= 0:
			marker = " <- at floor"
			if convergedAt == 0 {
				convergedAt = n
			}
		case big.Cmp(p, marketRate) <= 0:
			marker = " <- at/below market rate"
		}

		t.Logf("nblocksincl=%2d: %15s attoFIL  (%s%% of floor)%s", n, p, pct, marker)
	}

	if convergedAt > 0 {
		t.Logf("converged to floor at nblocksincl=%d", convergedAt)
	} else {
		t.Log("did not converge to floor within 30 blocks")
	}
}

// TestGasEstimateGasPremiumFromMempoolEmptyMempool verifies that an empty mempool
// returns a non-negative premium without panicking.
func TestGasEstimateGasPremiumFromMempoolEmptyMempool(t *testing.T) {
	baseFee := big.NewInt(buildconstants.MinimumBaseFee)
	ts := makeTipSet(t, baseFee, 0)
	cs := &mockChainStore{ts: ts}
	mp := &mockMpool{msgs: nil, ts: ts}

	premium, err := GasEstimateGasPremiumFromMempool(context.Background(), cs, mp, 1, 0, types.EmptyTSK)
	require.NoError(t, err)
	assert.False(t, premium.LessThan(big.Zero()), "premium should be non-negative for empty mempool")
}

// sampleSimGasLimit returns a gas limit drawn from a shifted-exponential distribution
// with approximate mean bgl/10, clamped to [bgl/120, 3*bgl/4].
// This gives an average of ~3 full blocks of gas per 30-transaction epoch.
func sampleSimGasLimit(rng *rand.Rand, bgl int64) int64 {
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

// simBlockProbabilities computes the probability of landing at each block position
// for a miner with ticket quality tq. Mirrors chain/messagepool.blockProbabilities.
func simBlockProbabilities(tq float64) []float64 {
	const maxBlocks = 15
	const mu = 5.0

	// noWinners[i] = P(i+1 other winners | ≥1 other winner), using Poisson(mu) conditioned on ≥1.
	cond := math.Log(-1 + math.Exp(mu))
	noWinners := make([]float64, maxBlocks)
	for i := 0; i < maxBlocks; i++ {
		k := float64(i + 1)
		lg, _ := math.Lgamma(k + 1)
		noWinners[i] = math.Exp(math.Log(mu)*k - lg - cond)
	}

	p := 1 - tq
	binoPdf := func(x, n int) float64 {
		if x > n {
			return 0
		}
		if p <= 0 {
			if x == 0 {
				return 1
			}
			return 0
		}
		if p >= 1 {
			if x == n {
				return 1
			}
			return 0
		}
		coef := 1.0
		for d := 1; d <= x; d++ {
			coef = coef * float64(n-x+d) / float64(d)
		}
		return coef * math.Pow(p, float64(x)) * math.Pow(1-p, float64(n-x))
	}

	out := make([]float64, maxBlocks)
	for place := 0; place < maxBlocks; place++ {
		for k, pCase := range noWinners {
			out[place] += pCase * binoPdf(place, k)
		}
	}
	return out
}

// simSelectMessages fills one block for a proposer with the given ticket quality,
// selecting independently from the full mempool without knowledge of other proposers.
//
// Mirrors the tq-dispatch in chain/messagepool.SelectMessages:
//   - tq > 0.84: greedy by gas performance (selectMessagesGreedy)
//   - tq ≤ 0.84: partition-based optimal selection (selectMessagesOptimal)
//
// Gas performance = effectivePremium * BlockGasLimit (equivalent to the real
// getGasPerf, since gasReward = effectivePremium * gasLimit cancels gasLimit).
func simSelectMessages(msgs []*types.SignedMessage, effPremiums []abi.TokenAmount, tq float64, bgl int64) []bool {
	n := len(msgs)
	chosen := make([]bool, n)
	if n == 0 {
		return chosen
	}

	// gasPerf ∝ effectivePremium (the gasLimit factor cancels in gasReward/gasLimit*BGL).
	gasPerfs := make([]float64, n)
	for i := range msgs {
		gasPerfs[i] = float64(effPremiums[i].Int64()) * float64(bgl)
	}

	// Sort indices by gasPerf descending (same initial sort in both greedy and optimal).
	idxs := make([]int, n)
	for i := range idxs {
		idxs[i] = i
	}
	sort.Slice(idxs, func(a, b int) bool {
		return gasPerfs[idxs[a]] > gasPerfs[idxs[b]]
	})

	if tq > 0.84 {
		// Greedy: fill by gasPerf order, no partitioning.
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

	// Optimal: assign each message to the partition (simulated block position) it would
	// fall into given the full sorted order, then weight by blockProbability[partition].
	// A miner with low tq is likely at a later position, so they prefer messages from
	// later partitions that high-tq miners (at earlier positions) will have already taken.
	blockProb := simBlockProbabilities(tq)

	partitions := make([]int, n)
	for i := range partitions {
		partitions[i] = len(blockProb) // sentinel: beyond any partition
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

	// effPerf = gasPerf * blockProb[partition]; messages beyond all partitions get -1.
	effPerfs := make([]float64, n)
	for i := range effPerfs {
		if partitions[i] < len(blockProb) {
			effPerfs[i] = gasPerfs[i] * blockProb[partitions[i]]
		} else {
			effPerfs[i] = -1
		}
	}

	// Re-sort by effective performance and fill greedily.
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

// TestGasEstimateGasPremiumSimulation runs a 3600-epoch gas market simulation.
// Every epoch:
//   - 30 transactions are inserted, their premium estimated via GasEstimateGasPremiumFromMempool
//     (nblocksincl bimodal: p(1)=0.1, p(2)=0.2, p(10)=0.7), gas limits drawn from a
//     shifted-exponential averaging bgl/10 (≈3 full blocks of gas per epoch)
//   - Each tx has a deadline = submission epoch + nblocksincl; late txs are counted
//   - 5 independent proposers each select messages with a random tq, unaware of each other;
//     the tipset is the union of their selections
//   - BaseFee is updated per FIP-0115 via NextBaseFeeFromPremium
//
// Run with -v to observe the baseFee, mempool-size, and late-tx progression.
func TestGasEstimateGasPremiumSimulation(t *testing.T) {
	const epochs = 3600
	bgl := buildconstants.BlockGasLimit
	seed := int64(42)
	if s := os.Getenv("SIM_SEED"); s != "" {
		fmt.Sscan(s, &seed)
	}
	rng := rand.New(rand.NewSource(seed))
	ctx := context.Background()

	baseFee := big.NewInt(buildconstants.MinimumBaseFee)
	mempool := make([]*types.SignedMessage, 0, 5000)
	deadlines := make([]int, 0, 5000)      // deadline[i] = epoch by which mempool[i] must be included
	nblocksincls := make([]uint64, 0, 5000) // nblocksincls[i] = nblocksincl used when submitting mempool[i]
	var nextSender uint64 = 2
	var totalGasUsed int64

	for epoch := 0; epoch < epochs; epoch++ {
		// Insert 30 transactions. Each estimates its own premium from the mempool as it
		// exists at submission time, so earlier submissions within the epoch are visible.
		// The mpool tipset uses timestamp=0 (only baseFee matters for message selection).
		// Each tx gets its own chainstore tipset with a descending timestamp so that
		// epochFractionRemaining returns (30-i)/30: the first tx sees a full epoch
		// remaining, the last sees 1/30.
		ts := makeTipSet(t, baseFee, 0)
		epochRemainingZero := os.Getenv("SIM_EPOCH_REMAINING_ZERO") == "1"
		now := time.Now().Unix()
		for i := range 30 {
			var csTs *types.TipSet
			if epochRemainingZero {
				csTs = makeTipSet(t, baseFee, 0)
			} else {
				csTs = makeTipSet(t, baseFee, uint64(now-int64(i)))
			}
			cs := &mockChainStore{ts: csTs}
			mp := &mockMpool{msgs: mempool, ts: ts}

			gl := sampleSimGasLimit(rng, bgl)
			var nblocksincl uint64
			switch r := rng.Float64(); {
			case r < 0.1:
				nblocksincl = 1
			case r < 0.3:
				nblocksincl = 2
			default:
				nblocksincl = 10
			}
			premium, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, nblocksincl, gl, types.EmptyTSK)
			require.NoError(t, err)

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

		// Compute effective premiums at the current baseFee.
		effPremiums := make([]abi.TokenAmount, len(mempool))
		for i, m := range mempool {
			effPremiums[i] = mempoolEffectivePremium(m.Message.GasFeeCap, m.Message.GasPremium, baseFee)
		}

		// Five proposers independently select from the full mempool, each with their own
		// random tq and unaware of what the others will pick. The tipset is the union.
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

		// Update baseFee per FIP-0115 using the tipset's premium distribution.
		percentilePremium := store.WeightedQuickSelect(tipPremiums, tipLimits, buildconstants.BlockGasTargetIndex)
		baseFee = store.NextBaseFeeFromPremium(baseFee, percentilePremium)

		// Remove selected messages from the mempool, keeping parallel slices in sync.
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

		// Count messages whose deadline has already passed, broken down by nblocksincl.
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

		t.Logf("epoch %4d: baseFee=%15s  mempoolSize=%5d  late=%4d (nb1=%d nb2=%d nb10=%d)  tipGas=%d/%d (%.1f%%)",
			epoch+1, baseFee, len(mempool), lateCount, late1, late2, late10,
			tipGasUsed, tipGasLimit, float64(tipGasUsed)/float64(tipGasLimit)*100)
	}

	totalGasLimit := int64(epochs) * int64(5) * bgl
	t.Logf("=== SUMMARY: total gas used %d / %d (%.1f%%)",
		totalGasUsed, totalGasLimit,
		float64(totalGasUsed)/float64(totalGasLimit)*100)

	assert.False(t, baseFee.LessThan(big.NewInt(buildconstants.MinimumBaseFee)),
		"baseFee should remain at or above minimum")
}

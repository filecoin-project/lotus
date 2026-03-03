package gasutils

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
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

// makeTipSet builds a minimal single-block TipSet with the given ParentBaseFee.
func makeTipSet(t testing.TB, baseFee abi.TokenAmount) *types.TipSet {
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
		Timestamp:             0,
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
	ts := makeTipSet(t, baseFee)

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
	ts := makeTipSet(b, baseFee)

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

// TestGasEstimateGasPremiumFromMempoolConvergence verifies that the estimated premium
// decreases as nblocksincl increases: a submitter willing to wait longer needs to
// outbid less competition, since high-premium messages will have cleared by then.
func TestGasEstimateGasPremiumFromMempoolConvergence(t *testing.T) {
	bgl := buildconstants.BlockGasLimit
	baseFee := big.NewInt(100_000_000_000) // 100 nanoFIL
	ts := makeTipSet(t, baseFee)

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

	p1, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, 1, bgl/5, types.EmptyTSK)
	require.NoError(t, err)
	p30, err := GasEstimateGasPremiumFromMempool(ctx, cs, mp, 30, bgl/5, types.EmptyTSK)
	require.NoError(t, err)

	assert.True(t, p1.GreaterThan(p30),
		"premium should decrease as nblocksincl increases: nb1=%s nb30=%s", p1, p30)
}

// TestGasEstimateGasPremiumFromMempoolEmptyMempool verifies that an empty mempool
// returns a non-negative premium without panicking.
func TestGasEstimateGasPremiumFromMempoolEmptyMempool(t *testing.T) {
	baseFee := big.NewInt(buildconstants.MinimumBaseFee)
	ts := makeTipSet(t, baseFee)
	cs := &mockChainStore{ts: ts}
	mp := &mockMpool{msgs: nil, ts: ts}

	premium, err := GasEstimateGasPremiumFromMempool(context.Background(), cs, mp, 1, 0, types.EmptyTSK)
	require.NoError(t, err)
	assert.False(t, premium.LessThan(big.Zero()), "premium should be non-negative for empty mempool")
}

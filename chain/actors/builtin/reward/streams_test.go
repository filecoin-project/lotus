package reward

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

// The evaluation cases the reward actor's own weight tests pin, at the scale they use: the
// clamps hold outside the segment and the line is exact within it.
func TestComputeWeight(t *testing.T) {
	rising := WeightRecord{VStart: 10, Slope: 2, TStart: 5, Floor: 4, Cap: 20}
	require.EqualValues(t, 4, ComputeWeight(rising, 0))
	require.EqualValues(t, 10, ComputeWeight(rising, 5))
	require.EqualValues(t, 20, ComputeWeight(rising, 10))

	falling := WeightRecord{VStart: 10, Slope: -2, TStart: 5, Floor: 4, Cap: 20}
	require.EqualValues(t, 20, ComputeWeight(falling, 0))
	require.EqualValues(t, 10, ComputeWeight(falling, 5))
	require.EqualValues(t, 4, ComputeWeight(falling, 10))

	flat := WeightRecord{VStart: 20, Slope: 0, TStart: 0, Floor: 4, Cap: 20}
	require.EqualValues(t, 20, ComputeWeight(flat, math.MinInt64))
	require.EqualValues(t, 20, ComputeWeight(flat, math.MaxInt64))

	// A record anchored at the end of the epoch domain, evaluated at its start.
	extreme := WeightRecord{VStart: Denom, Slope: math.MinInt64, TStart: math.MaxInt64, Floor: 0, Cap: Denom}
	require.EqualValues(t, Denom, ComputeWeight(extreme, math.MinInt64))

	// A floor above the cap clamps to the floor, as the actor's min-then-max does.
	reversed := WeightRecord{VStart: 10, Slope: 0, TStart: 0, Floor: 20, Cap: 4}
	require.EqualValues(t, 20, ComputeWeight(reversed, 0))

	// A canonical delayed ramp: flat at its floor until the anchor, then rising to the cap.
	delayed := WeightRecord{VStart: 10, Slope: 2, TStart: 5, Floor: 10, Cap: 20}
	for epoch, expected := range map[abi.ChainEpoch]uint64{0: 10, 4: 10, 5: 10, 6: 12, 10: 20, 100: 20} {
		require.EqualValues(t, expected, ComputeWeight(delayed, epoch), "epoch %d", epoch)
	}
}

func TestStreamLedger(t *testing.T) {
	const (
		epoch    = abi.ChainEpoch(2_000)
		timelock = abi.ChainEpoch(20_160)
		pct      = Denom / 100
	)
	req := require.New(t)
	store := memStore(t)

	swa := mustIDAddress(t, 777)
	writer := mustIDAddress(t, 100)
	recipients := []address.Address{mustIDAddress(t, 101), mustIDAddress(t, 102), mustIDAddress(t, 103)}

	// A consensus stream ramping down to a 50% floor, and a service stream holding 30% of which
	// its map allocates 90%, the remainder burning alongside the schedule's own residual.
	consensus := WeightRecord{VStart: 70 * pct, Slope: -int64(pct) / 100, TStart: 1_000, Floor: 50 * pct, Cap: 70 * pct}
	service := WeightRecord{VStart: 30 * pct, TStart: 1_000, Floor: 30 * pct, Cap: 30 * pct}
	distribution := &reward19.ExplicitDistribution{
		Writer: writer,
		Shares: []RecipientShare{
			{Recipient: recipients[0], Share: 40 * pct},
			{Recipient: recipients[1], Share: 50 * pct},
		},
		Payable:       []RecipientAmount{{Recipient: recipients[0], Amount: abi.NewTokenAmount(5)}},
		ClaimedPeriod: []RecipientAmount{{Recipient: recipients[1], Amount: abi.NewTokenAmount(7)}},
	}
	streams := &reward19.StreamsState{
		Streams: []reward19.Stream{
			{ID: 1, Weight: consensus},
			{ID: 2, Weight: service, Distribution: distribution},
		},
		Tombstones: []reward19.Tombstone{{
			ID:      3,
			Payable: []RecipientAmount{{Recipient: recipients[2], Amount: abi.NewTokenAmount(11)}},
		}},
		PendingWritesQueue: []reward19.PendingWrite{
			{
				Op: reward19.PendingWriteOpSetWeightRecords,
				Payload: mustPayload(t, &reward19.SetWeightRecordsParams{Updates: []WeightRecordUpdate{
					{ID: 1, Weight: consensus},
					{ID: 2, Weight: service},
				}}),
				EffectiveEpoch: epoch + timelock,
			},
			{
				ID: streamID(4),
				Op: reward19.PendingWriteOpRegisterStream,
				Payload: mustPayload(t, &RegisterStreamPayload{
					Weight: service,
					Distribution: &DistributionInit{
						Writer: writer,
						Shares: []RecipientShare{{Recipient: recipients[2], Share: Denom}},
					},
				}),
				EffectiveEpoch: epoch + timelock + 1,
			},
			{
				ID:             streamID(2),
				Op:             reward19.PendingWriteOpSetDistribution,
				Payload:        mustPayload(t, &reward19.SetDistributionPayload{Writer: recipients[0]}),
				EffectiveEpoch: epoch + timelock + 2,
			},
			{
				ID:             streamID(2),
				Op:             reward19.PendingWriteOpRemoveStream,
				Payload:        []byte{0x80},
				EffectiveEpoch: epoch + timelock + 3,
			},
		},
	}

	st, err := reward19.ConstructState(store, big.Zero())
	req.NoError(err)
	st.SWAActor = swa
	st.SWATimelockEpochs = timelock
	st.TotalMintedReward = abi.NewTokenAmount(1_000)
	st.TotalBurnMinted = abi.NewTokenAmount(200)
	st.TotalExplicitMinted = abi.NewTokenAmount(300)
	st.Accrued = []reward19.StreamAccrual{{ID: 2, Amount: abi.NewTokenAmount(100)}}
	st.StreamsRoot = mustPut(t, store, streams)

	state, err := load19(store, mustPut(t, store, st))
	req.NoError(err)
	ledger, err := state.StreamLedger(epoch)
	req.NoError(err)

	req.Equal(epoch, ledger.Epoch)
	req.Equal(swa, ledger.SWAActor)
	req.Equal(timelock, ledger.SWATimelock)
	req.Equal(abi.NewTokenAmount(1_000), ledger.TotalMinted)
	req.Equal(abi.NewTokenAmount(200), ledger.TotalBurnMinted)
	req.Equal(abi.NewTokenAmount(300), ledger.TotalExplicitMinted)
	req.Equal(abi.NewTokenAmount(500), ledger.MinerMinted())

	req.Len(ledger.Streams, 2)
	implicit := ledger.Streams[0]
	req.True(implicit.Implicit)
	req.Equal(StreamID(1), implicit.ID)
	req.Equal(consensus, implicit.Weight)
	// A thousand epochs of ramp take the consensus stream ten points below its 70% start.
	req.EqualValues(60*pct, implicit.EvaluatedWeight)
	req.Equal(big.Zero(), implicit.Accrued)
	req.Empty(implicit.Shares)
	req.True(implicit.Writer.Empty())

	explicit := ledger.Streams[1]
	req.False(explicit.Implicit)
	req.Equal(StreamID(2), explicit.ID)
	req.EqualValues(30*pct, explicit.EvaluatedWeight)
	req.Equal(writer, explicit.Writer)
	req.Equal(abi.NewTokenAmount(100), explicit.Accrued)
	req.Equal(distribution.Shares, explicit.Shares)
	req.Equal(distribution.Payable, explicit.Payable)
	req.Equal(distribution.ClaimedPeriod, explicit.ClaimedPeriod)
	req.EqualValues(90*pct, explicit.ShareTotal().Uint64())
	req.EqualValues(10*pct, explicit.ShareBurn().Uint64())
	// The period's accrual less what its recipients drew, plus the balance carried into it.
	req.Equal(abi.NewTokenAmount(98), explicit.Liability())

	req.EqualValues(90*pct, ledger.WeightTotal().Uint64())
	req.EqualValues(10*pct, ledger.BurnWeight().Uint64())

	req.Len(ledger.Tombstones, 1)
	req.Equal(StreamID(3), ledger.Tombstones[0].ID)
	req.Equal(abi.NewTokenAmount(11), ledger.Tombstones[0].Liability())
	req.Equal(abi.NewTokenAmount(109), ledger.Liability())

	req.Len(ledger.PendingWrites, 4)
	schedule := ledger.PendingWrites[0]
	req.Nil(schedule.ID)
	req.Equal(OpSetWeightRecords, schedule.Op)
	req.Equal("SetWeightRecords", schedule.Op.String())
	req.Equal(epoch+timelock, schedule.EffectiveEpoch)
	req.Equal([]WeightRecordUpdate{{ID: 1, Weight: consensus}, {ID: 2, Weight: service}}, schedule.Updates)

	registration := ledger.PendingWrites[1]
	req.NotNil(registration.ID)
	req.Equal(StreamID(4), *registration.ID)
	req.Equal(OpRegisterStream, registration.Op)
	req.NotNil(registration.Register)
	req.Equal(service, registration.Register.Weight)
	req.Equal(writer, registration.Register.Distribution.Writer)
	req.Equal([]RecipientShare{{Recipient: recipients[2], Share: Denom}}, registration.Register.Distribution.Shares)

	rewrite := ledger.PendingWrites[2]
	req.Equal(OpSetDistribution, rewrite.Op)
	req.Equal(recipients[0], rewrite.Writer)

	removal := ledger.PendingWrites[3]
	req.Equal(OpRemoveStream, removal.Op)
	req.Equal(StreamID(2), *removal.ID)
	req.Nil(removal.Register)
}

// An accrual row belongs to exactly one live explicit stream. The ledger refuses state where
// that pairing is broken rather than reporting a stream's earnings as zero.
func TestStreamLedgerAccrualPairing(t *testing.T) {
	explicit := reward19.Stream{
		ID:           2,
		Weight:       WeightRecord{VStart: Denom, Floor: Denom, Cap: Denom},
		Distribution: &reward19.ExplicitDistribution{Writer: mustIDAddress(t, 100)},
	}

	for name, accrued := range map[string][]reward19.StreamAccrual{
		"no row for the explicit stream": nil,
		"a row for no live stream": {
			{ID: 2, Amount: abi.NewTokenAmount(1)},
			{ID: 3, Amount: abi.NewTokenAmount(1)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := require.New(t)
			store := memStore(t)
			st, err := reward19.ConstructState(store, big.Zero())
			req.NoError(err)
			st.Accrued = accrued
			st.StreamsRoot = mustPut(t, store, &reward19.StreamsState{Streams: []reward19.Stream{explicit}})

			state, err := load19(store, mustPut(t, store, st))
			req.NoError(err)
			_, err = state.StreamLedger(0)
			req.Error(err)
		})
	}
}

// The ledger arrives with the reward actor's v19 state, and the versions before it say so.
func TestStreamLedgerUnsupported(t *testing.T) {
	req := require.New(t)
	store := memStore(t)

	for _, version := range []actorstypes.Version{actorstypes.Version0, actorstypes.Version8, actorstypes.Version18} {
		state, err := MakeState(store, version, big.Zero())
		req.NoError(err)
		_, err = state.StreamLedger(0)
		req.ErrorContains(err, "unsupported")
	}
}

func memStore(t *testing.T) adt.Store {
	t.Helper()
	return adt.WrapStore(context.Background(), cbor.NewCborStore(blockstore.NewMemory()))
}

func mustIDAddress(t *testing.T, id uint64) address.Address {
	t.Helper()
	addr, err := address.NewIDAddress(id)
	require.NoError(t, err)
	return addr
}

func streamID(id StreamID) *StreamID {
	return &id
}

func mustPayload(t *testing.T, params cbg.CBORMarshaler) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, params.MarshalCBOR(&buf))
	return buf.Bytes()
}

func mustPut(t *testing.T, store adt.Store, value cbg.CBORMarshaler) cid.Cid {
	t.Helper()
	root, err := store.Put(store.Context(), value)
	require.NoError(t, err)
	return root
}

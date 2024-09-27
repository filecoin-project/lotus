package index

import (
	"context"
	"database/sql"
	"errors"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestGetEventsForFilterNoEvents(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))

	headHeight := abi.ChainEpoch(60)
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	defer si.Close()

	// Create a fake tipset at height 1
	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)

	// Set the dummy chainstore to return this tipset for height 1
	cs.SetTipsetByHeight(1, fakeTipSet1) // empty DB

	// tipset is not indexed
	f := &EventFilter{
		MinHeight: 1,
		MaxHeight: 1,
	}
	ces, err := si.GetEventsForFilter(ctx, f, false)
	require.True(t, errors.Is(err, ErrNotFound))
	require.Equal(t, 0, len(ces))

	tsCid, err := fakeTipSet1.Key().Cid()
	require.NoError(t, err)
	f = &EventFilter{
		TipsetCid: tsCid,
	}

	ces, err = si.GetEventsForFilter(ctx, f, false)
	require.True(t, errors.Is(err, ErrNotFound))
	require.Equal(t, 0, len(ces))

	// tipset is indexed but has no events
	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		si.indexTipset(ctx, tx, fakeTipSet1)
		return nil
	})
	require.NoError(t, err)

	ces, err = si.GetEventsForFilter(ctx, f, false)
	require.NoError(t, err)
	require.Equal(t, 0, len(ces))

	f = &EventFilter{
		TipsetCid: tsCid,
	}
	ces, err = si.GetEventsForFilter(ctx, f, false)
	require.NoError(t, err)
	require.Equal(t, 0, len(ces))

	// search for a range that is absent
	f = &EventFilter{
		MinHeight: 100,
		MaxHeight: 200,
	}
	ces, err = si.GetEventsForFilter(ctx, f, false)
	require.NoError(t, err)
	require.Equal(t, 0, len(ces))
}

func TestGetEventsForFilterWithEvents(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))
	headHeight := abi.ChainEpoch(60)
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	defer si.Close()

	ev1 := fakeEvent(
		abi.ActorID(1),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	ev2 := fakeEvent(
		abi.ActorID(2),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr2")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	events := []types.Event{*ev1, *ev2}

	fm := fakeMessage(address.TestAddress, address.TestAddress)
	em1 := executedMessage{
		msg: fm,
		evs: events,
	}

	si.SetIdToRobustAddrFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}

		return idAddr, true
	})

	si.SetEventLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1}, nil
	})

	// Create a fake tipset at height 1
	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)

	// Set the dummy chainstore to return this tipset for height 1
	cs.SetTipsetByHeight(1, fakeTipSet1) // empty DB
	cs.SetTipsetByHeight(2, fakeTipSet2) // empty DB

	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})

	// index tipset and events
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	// fetch it based on height -> works
	f := &EventFilter{
		MinHeight: 1,
		MaxHeight: 1,
	}
	ces, err := si.GetEventsForFilter(ctx, f, false)
	require.NoError(t, err)
	require.Equal(t, 2, len(ces))

	// fetch it based on cid -> works
	tsCid1, err := fakeTipSet1.Key().Cid()
	require.NoError(t, err)

	tsCid2, err := fakeTipSet2.Key().Cid()
	require.NoError(t, err)

	f = &EventFilter{
		TipsetCid: tsCid1,
	}
	ces, err = si.GetEventsForFilter(ctx, f, false)
	require.NoError(t, err)
	require.Equal(t, 2, len(ces))

	require.Equal(t, ev1.Entries, ces[0].Entries)
	require.Equal(t, ev2.Entries, ces[1].Entries)

	// mark fakeTipSet2 as reverted so events for fakeTipSet1 are reverted
	require.NoError(t, si.Revert(ctx, fakeTipSet2, fakeTipSet1))

	var reverted bool
	err = si.db.QueryRow("SELECT reverted FROM tipset_message WHERE tipset_key_cid = ?", tsCid2.Bytes()).Scan(&reverted)
	require.NoError(t, err)
	require.True(t, reverted)

	var reverted2 bool
	err = si.db.QueryRow("SELECT reverted FROM tipset_message WHERE tipset_key_cid = ?", tsCid1.Bytes()).Scan(&reverted2)
	require.NoError(t, err)
	require.False(t, reverted2)

	// fetching events fails if excludeReverted is true
	f = &EventFilter{
		TipsetCid: tsCid1,
	}
	ces, err = si.GetEventsForFilter(ctx, f, true)
	require.NoError(t, err)
	require.Equal(t, 0, len(ces))

	// works if excludeReverted is false
	ces, err = si.GetEventsForFilter(ctx, f, false)
	require.NoError(t, err)
	require.Equal(t, 2, len(ces))
}

func fakeMessage(to, from address.Address) *types.Message {
	return &types.Message{
		To:         to,
		From:       from,
		Nonce:      197,
		Method:     1,
		Params:     []byte("some random bytes"),
		GasLimit:   126723,
		GasPremium: types.NewInt(4),
		GasFeeCap:  types.NewInt(120),
	}
}

func fakeEvent(emitter abi.ActorID, indexed []kv, unindexed []kv) *types.Event {
	ev := &types.Event{
		Emitter: emitter,
	}

	for _, in := range indexed {
		ev.Entries = append(ev.Entries, types.EventEntry{
			Flags: 0x01,
			Key:   in.k,
			Codec: cid.Raw,
			Value: in.v,
		})
	}

	for _, in := range unindexed {
		ev.Entries = append(ev.Entries, types.EventEntry{
			Flags: 0x00,
			Key:   in.k,
			Codec: cid.Raw,
			Value: in.v,
		})
	}

	return ev
}

type kv struct {
	k string
	v []byte
}

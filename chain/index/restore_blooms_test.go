package index

import (
	"bytes"
	"context"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// fevmHeight is this build's FEVM activation height, typed: networks declare
// it as a typed or untyped constant or a variable.
func fevmHeight() abi.ChainEpoch { return buildconstants.UpgradeHyggeHeight }

// afterFEVM is a test height where tipsets can have events.
func afterFEVM() abi.ChainEpoch { return max(fevmHeight(), 0) + 50 }

func TestRestoreTipsetBlooms(t *testing.T) {
	epoch := afterFEVM()
	removeBloom := func(t *testing.T, f *bloomsFixture) { f.removeBloom(t) }

	for _, tc := range []struct {
		name   string
		tipset tipsetContent
		// beforeFEVM places the tipset below FEVM activation
		beforeFEVM bool
		// damage breaks the bloom, and possibly the index, before restoring
		damage func(t *testing.T, f *bloomsFixture)
		want   RestoreTipsetBloomsCounts
	}{{
		name:   "missing with events",
		tipset: withTopicEvent,
		damage: removeBloom,
		want:   RestoreTipsetBloomsCounts{WithEvents: 1},
	}, {
		name:   "missing with hollow event",
		tipset: withHollowEvent,
		damage: removeBloom,
		want:   RestoreTipsetBloomsCounts{WithEvents: 1},
	}, {
		name:   "missing without messages",
		tipset: withoutMessages,
		damage: removeBloom,
		want:   RestoreTipsetBloomsCounts{NoMessages: 1},
	}, {
		name:       "missing without events, before FEVM",
		tipset:     withoutEvents,
		beforeFEVM: true,
		damage:     removeBloom,
		want:       RestoreTipsetBloomsCounts{BeforeEvents: 1},
	}, {
		name:   "missing without events",
		tipset: withoutEvents,
		damage: removeBloom,
		want:   RestoreTipsetBloomsCounts{Unverified: 1},
	}, {
		name:   "placeholder",
		tipset: withTopicEvent,
		damage: func(t *testing.T, f *bloomsFixture) { f.placeholderBloom(t) },
		want:   RestoreTipsetBloomsCounts{WithEvents: 1},
	}, {
		name:   "present",
		tipset: withTopicEvent,
		damage: func(*testing.T, *bloomsFixture) {},
	}, {
		name:   "reverted events",
		tipset: withTopicEvent,
		damage: func(t *testing.T, f *bloomsFixture) {
			f.removeBloom(t)
			_, err := f.si.stmts.updateEventsToRevertedStmt.ExecContext(context.Background(), f.tsKeyCid.Bytes())
			require.NoError(t, err)
		},
		want: RestoreTipsetBloomsCounts{RevertedEvents: []abi.ChainEpoch{afterFEVM()}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			epoch := epoch
			if tc.beforeFEVM {
				if fevmHeight() < 1 {
					t.Skip("FEVM is active from genesis on this network")
				}
				epoch = fevmHeight() - 1
			}

			ctx := context.Background()
			f := setupBlooms(t, epoch, tc.tipset)
			wantBloom := f.bloom(t)
			hasEvents := tc.tipset == withTopicEvent || tc.tipset == withHollowEvent
			require.Equal(t, hasEvents, !bytes.Equal(wantBloom, ethtypes.NewEmptyEthBloom()))
			tc.damage(t, f)
			damagedBloom := f.bloom(t)

			// Only the tipset under test; the fixture's executing tipset has
			// no bloom of its own
			opts := RestoreTipsetBloomsOptions{From: epoch, To: epoch, Batch: 1, QuietStretch: 100, DryRun: true}
			counts, err := f.si.RestoreTipsetBlooms(ctx, opts)
			require.NoError(t, err)
			require.Equal(t, tc.want, *counts)
			require.Equal(t, damagedBloom, f.bloom(t))

			opts.DryRun = false
			counts, err = f.si.RestoreTipsetBlooms(ctx, opts)
			require.NoError(t, err)
			require.Equal(t, tc.want, *counts)
			if tc.want.Restored() > 0 {
				require.Equal(t, wantBloom, f.bloom(t))
			} else {
				require.Equal(t, damagedBloom, f.bloom(t))
			}

			// Restored tipsets are done; reverted ones are reported again
			counts, err = f.si.RestoreTipsetBlooms(ctx, opts)
			require.NoError(t, err)
			require.Zero(t, counts.Restored())
			require.Equal(t, tc.want.RevertedEvents, counts.RevertedEvents)
		})
	}
}

func TestRestoreTipsetBloomsQuietStretches(t *testing.T) {
	type tipset struct {
		h     abi.ChainEpoch
		class bloomClass
	}
	tipsets := []tipset{
		{1000, bloomUnverified},
		{995, bloomNoMessages},
		{990, bloomUnverified},
	}
	// a gap of tipsets that already have blooms ends the stretch above
	for h := abi.ChainEpoch(900); h >= 800; h -= 10 {
		tipsets = append(tipsets, tipset{h, bloomUnverified})
	}
	tipsets = append(tipsets,
		tipset{799, bloomWithEvents},
		tipset{700, bloomUnverified},
		tipset{699, bloomUnverified},
	)

	c := &RestoreTipsetBloomsCounts{}
	for _, ts := range tipsets {
		c.add(ts.h, ts.class, 50)
	}
	c.closeQuiet(50)
	require.Equal(t, []HeightRange{{From: 900, To: 800}}, c.QuietStretches)
	require.Equal(t, 15, c.Unverified)
}

func TestRestoreTipsetBloomsQuietStretchAcrossBatches(t *testing.T) {
	ctx := context.Background()
	// Eventless tipsets every 3 epochs from base+18 down to base+3, each
	// executed by a tipset without messages, then one with events at base
	// ending the stretch
	base := afterFEVM()
	f := setupBlooms(t, base+18, withoutEvents)
	for h := base + 15; h >= base+3; h -= 3 {
		f.index(h, withoutEvents)
	}
	f.index(base, withTopicEvent)
	_, err := f.si.db.ExecContext(ctx, "DELETE FROM tipset_bloom")
	require.NoError(t, err)

	counts, err := f.si.RestoreTipsetBlooms(ctx, RestoreTipsetBloomsOptions{From: base + 19, To: base, Batch: 4, QuietStretch: 10})
	require.NoError(t, err)
	require.Equal(t, 6, counts.Unverified)
	require.Equal(t, 7, counts.NoMessages)
	require.Equal(t, 1, counts.WithEvents)
	require.Equal(t, []HeightRange{{From: base + 18, To: base + 3}}, counts.QuietStretches)
}

func TestRestoreTipsetBloomsRejectsBadOptions(t *testing.T) {
	f := setupBlooms(t, 50, withoutMessages)
	for _, opts := range []RestoreTipsetBloomsOptions{
		{From: 1, To: 2, Batch: 1, QuietStretch: 1},
		{From: 2, To: 1, Batch: 0, QuietStretch: 1},
		{From: 2, To: 1, Batch: 1, QuietStretch: 0},
	} {
		_, err := f.si.RestoreTipsetBlooms(context.Background(), opts)
		require.Error(t, err)
	}
}

type tipsetContent int

const (
	withoutMessages tipsetContent = iota
	withoutEvents
	withTopicEvent
	// EVM LOG0 with no data emits an event with no entries
	withHollowEvent
)

type bloomsFixture struct {
	si    *SqliteIndexer
	epoch abi.ChainEpoch
	// index indexes another tipset at h, executed by a tipset at h+1
	index             func(h abi.ChainEpoch, content tipsetContent) (ts, executionTs *types.TipSet)
	tsKeyCid          cid.Cid
	executionTsKeyCid cid.Cid
}

func (f *bloomsFixture) removeBloom(t *testing.T) {
	_, err := f.si.stmts.removeTipsetBloomStmt.ExecContext(context.Background(), f.tsKeyCid.Bytes())
	require.NoError(t, err)
}

func (f *bloomsFixture) placeholderBloom(t *testing.T) {
	_, err := f.si.stmts.insertTipsetBloomStmt.ExecContext(context.Background(), f.tsKeyCid.Bytes(), f.epoch, ethtypes.NewFullEthBloom())
	require.NoError(t, err)
}

// bloom returns the tipset's stored bloom, or nil if there is none.
func (f *bloomsFixture) bloom(t *testing.T) []byte {
	bloom, _, err := f.si.GetTipsetBloom(context.Background(), f.tsKeyCid)
	require.NoError(t, err)
	return bloom
}

// setupBlooms indexes a tipset at epoch with the given content through the
// live indexer, along with a tipset executing it at epoch+1.
func setupBlooms(t *testing.T, epoch abi.ChainEpoch, content tipsetContent) *bloomsFixture {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))

	si, _, cs := setupWithHeadIndexed(t, 300, rng)
	t.Cleanup(func() { _ = si.Close() })
	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	executedByHeight := map[abi.ChainEpoch][]executedMessage{}
	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return executedByHeight[msgTs.Height()], nil
	})

	f := &bloomsFixture{si: si, epoch: epoch}
	f.index = func(h abi.ChainEpoch, content tipsetContent) (ts, executionTs *types.TipSet) {
		ts = fakeTipSet(t, rng, h, []cid.Cid{})
		executionTs = fakeTipSet(t, rng, h+1, ts.Key().Cids())
		if content != withoutMessages {
			msg := fakeMessage(randomIDAddr(t, rng), randomIDAddr(t, rng))
			em := executedMessage{msg: msg}
			switch content {
			case withTopicEvent:
				topic := make([]byte, 32)
				rng.Read(topic)
				em.evs = []types.Event{*fakeEvent(1, []kv{{k: "t1", v: topic}}, nil)}
			case withHollowEvent:
				topic := make([]byte, 32)
				rng.Read(topic)
				em.evs = []types.Event{{Emitter: 2}, *fakeEvent(1, []kv{{k: "t1", v: topic}}, nil)}
			}
			executedByHeight[h] = []executedMessage{em}
			cs.SetMessagesForTipset(ts, []types.ChainMsg{msg})
		}
		require.NoError(t, si.Apply(ctx, ts, executionTs))
		return ts, executionTs
	}

	ts, executionTs := f.index(epoch, content)

	var err error
	f.tsKeyCid, err = ts.Key().Cid()
	require.NoError(t, err)
	f.executionTsKeyCid, err = executionTs.Key().Cid()
	require.NoError(t, err)
	return f
}

package full

import (
	"context"
	"fmt"
	pseudo "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/raulk/clock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/types"
)

var testCid = cid.MustParse("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")

func TestParseHeightRange(t *testing.T) {
	epochPtr := func(i int) *abi.ChainEpoch {
		e := abi.ChainEpoch(i)
		return &e
	}

	testCases := []struct {
		name     string
		heaviest abi.ChainEpoch
		from     *abi.ChainEpoch
		to       *abi.ChainEpoch
		maxRange abi.ChainEpoch
		minOut   abi.ChainEpoch
		maxOut   abi.ChainEpoch
		errStr   string
	}{
		{
			name:     "fails when both are specified and range is greater than max allowed range",
			heaviest: 100,
			from:     epochPtr(256),
			to:       epochPtr(512),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "too large",
		},
		{
			name:     "fails when min is specified and range is greater than max allowed range",
			heaviest: 500,
			from:     epochPtr(16),
			to:       nil,
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "'from' height is too far in the past",
		},
		{
			name:     "fails when max is specified and range is greater than max allowed range",
			heaviest: 500,
			from:     nil,
			to:       epochPtr(65536),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "'to' height is too far in the future",
		},
		{
			name:     "fails when from is greater than to",
			heaviest: 100,
			from:     epochPtr(512),
			to:       epochPtr(256),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "must be after",
		},
		{
			name:     "works when range is valid (nil from)",
			heaviest: 500,
			from:     nil,
			to:       epochPtr(48),
			maxRange: 1000,
			minOut:   -1,
			maxOut:   48,
		},
		{
			name:     "works when range is valid (nil to)",
			heaviest: 500,
			from:     epochPtr(0),
			to:       nil,
			maxRange: 1000,
			minOut:   0,
			maxOut:   -1,
		},
		{
			name:     "works when range is valid (nil from and to)",
			heaviest: 500,
			from:     nil,
			to:       nil,
			maxRange: 1000,
			minOut:   -1,
			maxOut:   -1,
		},
		{
			name:     "works when range is valid and specified",
			heaviest: 500,
			from:     epochPtr(16),
			to:       epochPtr(48),
			maxRange: 1000,
			minOut:   16,
			maxOut:   48,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)
			min, max, err := parseHeightRange(tc.heaviest, tc.from, tc.to, tc.maxRange)
			req.Equal(tc.minOut, min)
			req.Equal(tc.maxOut, max)
			if tc.errStr != "" {
				t.Log(err)
				req.Error(err)
				req.Contains(err.Error(), tc.errStr)
			} else {
				req.NoError(err)
			}
		})
	}
}

func TestGetActorEvents(t *testing.T) {
	ctx := context.Background()
	req := require.New(t)

	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	const maxFilterHeightRange = 100

	minerAddr, err := address.NewIDAddress(uint64(rng.Int63()))
	req.NoError(err)

	testCases := []struct {
		name                   string
		filter                 *types.ActorEventFilter
		currentHeight          int64
		installMinHeight       int64
		installMaxHeight       int64
		installTipSetKey       cid.Cid
		installAddresses       []address.Address
		installKeysWithCodec   map[string][]types.ActorEventBlock
		installExcludeReverted bool
		expectErr              string
	}{
		{
			name:             "nil filter",
			filter:           nil,
			installMinHeight: -1,
			installMaxHeight: -1,
		},
		{
			name:             "empty filter",
			filter:           &types.ActorEventFilter{},
			installMinHeight: -1,
			installMaxHeight: -1,
		},
		{
			name: "basic height range filter",
			filter: &types.ActorEventFilter{
				FromHeight: epochPtr(0),
				ToHeight:   epochPtr(maxFilterHeightRange),
			},
			installMinHeight: 0,
			installMaxHeight: maxFilterHeightRange,
		},
		{
			name: "from, no to height",
			filter: &types.ActorEventFilter{
				FromHeight: epochPtr(0),
			},
			currentHeight:    maxFilterHeightRange - 1,
			installMinHeight: 0,
			installMaxHeight: -1,
		},
		{
			name: "to, no from height",
			filter: &types.ActorEventFilter{
				ToHeight: epochPtr(maxFilterHeightRange - 1),
			},
			installMinHeight: -1,
			installMaxHeight: maxFilterHeightRange - 1,
		},
		{
			name: "from, no to height, too far",
			filter: &types.ActorEventFilter{
				FromHeight: epochPtr(0),
			},
			currentHeight: maxFilterHeightRange + 1,
			expectErr:     "invalid epoch range: 'from' height is too far in the past",
		},
		{
			name: "to, no from height, too far",
			filter: &types.ActorEventFilter{
				ToHeight: epochPtr(maxFilterHeightRange + 1),
			},
			currentHeight: 0,
			expectErr:     "invalid epoch range: 'to' height is too far in the future",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			efm := newMockEventFilterManager(t)
			collectedEvents := makeCollectedEvents(t, rng, 0, 1, 10)
			filter := newMockFilter(ctx, t, rng, collectedEvents)

			if tc.expectErr == "" {
				efm.expectInstall(abi.ChainEpoch(tc.installMinHeight), abi.ChainEpoch(tc.installMaxHeight), tc.installTipSetKey, tc.installAddresses, tc.installKeysWithCodec, tc.installExcludeReverted, filter)
			}

			ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, tc.currentHeight)})
			req.NoError(err)
			chain := newMockChainAccessor(t, ts)

			handler := NewActorEventHandler(chain, efm, 50*time.Millisecond, maxFilterHeightRange)

			gotEvents, err := handler.GetActorEvents(ctx, tc.filter)
			if tc.expectErr != "" {
				req.Error(err)
				req.Contains(err.Error(), tc.expectErr)
			} else {
				req.NoError(err)
				expectedEvents := collectedToActorEvents(collectedEvents)
				req.Equal(expectedEvents, gotEvents)
				efm.assertRemoved(filter.ID())
			}
		})
	}
}

func TestSubscribeActorEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	mockClock := clock.NewMock()

	const maxFilterHeightRange = 100
	const blockDelay = 30 * time.Second
	const filterStartHeight = 0
	const currentHeight = 10
	const finishHeight = 20
	const eventsPerEpoch = 2

	minerAddr, err := address.NewIDAddress(uint64(rng.Int63()))
	require.NoError(t, err)

	for _, tc := range []struct {
		name           string
		receiveSpeed   time.Duration // how fast will we receive all events _per epoch_
		expectComplete bool          // do we expect this to succeed?
		endEpoch       int           // -1 for no end
	}{
		{"fast", 0, true, -1},
		{"fast with end", 0, true, finishHeight},
		{"half block speed", blockDelay / 2, true, -1},
		{"half block speed with end", blockDelay / 2, true, finishHeight},
		// testing exactly blockDelay is a border case and will be flaky
		{"1.5 block speed", blockDelay * 3 / 2, false, -1},
		{"twice block speed", blockDelay * 2, false, -1},
	} {

		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			mockClock.Set(time.Now())
			mockFilterManager := newMockEventFilterManager(t)
			allEvents := makeCollectedEvents(t, rng, filterStartHeight, eventsPerEpoch, finishHeight)
			historicalEvents := allEvents[0 : (currentHeight-filterStartHeight)*eventsPerEpoch]
			mockFilter := newMockFilter(ctx, t, rng, historicalEvents)
			mockFilterManager.expectInstall(abi.ChainEpoch(0), abi.ChainEpoch(tc.endEpoch), cid.Undef, nil, nil, false, mockFilter)

			ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, currentHeight)})
			req.NoError(err)
			mockChain := newMockChainAccessor(t, ts)

			handler := NewActorEventHandlerWithClock(mockChain, mockFilterManager, blockDelay, maxFilterHeightRange, mockClock)

			aef := &types.ActorEventFilter{FromHeight: epochPtr(0)}
			if tc.endEpoch >= 0 {
				aef.ToHeight = epochPtr(tc.endEpoch)
			}
			eventChan, err := handler.SubscribeActorEvents(ctx, aef)
			req.NoError(err)

			gotEvents := make([]*types.ActorEvent, 0)

			// assume we can cleanly pick up all historical events in one go
			for len(gotEvents) < len(historicalEvents) && ctx.Err() == nil {
				select {
				case e, ok := <-eventChan:
					req.True(ok)
					gotEvents = append(gotEvents, e)
				case <-ctx.Done():
					t.Fatalf("timed out waiting for event")
				}
			}
			req.Equal(collectedToActorEvents(historicalEvents), gotEvents)

			mockClock.Add(blockDelay)
			nextReceiveTime := mockClock.Now()

			// Ticker to simulate both time and the chain advancing, including emitting events at
			// the right time directly to the filter.

			go func() {
				for thisHeight := int64(currentHeight); ctx.Err() == nil; thisHeight++ {
					ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, thisHeight)})
					req.NoError(err)
					mockChain.setHeaviestTipSet(ts)

					var eventsThisEpoch []*filter.CollectedEvent
					if thisHeight <= finishHeight {
						eventsThisEpoch = allEvents[(thisHeight-filterStartHeight)*eventsPerEpoch : (thisHeight-filterStartHeight+1)*eventsPerEpoch]
					}
					for i := 0; i < eventsPerEpoch; i++ {
						if len(eventsThisEpoch) > 0 {
							mockFilter.sendEventToChannel(eventsThisEpoch[0])
							eventsThisEpoch = eventsThisEpoch[1:]
						}
						select {
						case <-time.After(2 * time.Millisecond): // allow everyone to catch a breath
							mockClock.Add(blockDelay / eventsPerEpoch)
						case <-ctx.Done():
							return
						}
					}

					if thisHeight == finishHeight+1 && tc.expectComplete && tc.endEpoch < 0 && ctx.Err() == nil {
						// at finish+1, for the case where we expect clean completion and there is no ToEpoch
						// set on the filter, if we send one more event at the next height so we end up with
						// something uncollected in the buffer, causing a disconnect
						evt := makeCollectedEvents(t, rng, finishHeight+1, 1, finishHeight+1)[0]
						mockFilter.sendEventToChannel(evt)
					} // else if endEpoch is set, we expect the chain advance to force closure
				}
			}()

			// Client collecting events off the channel

			var prematureEnd bool
			for thisHeight := int64(currentHeight); thisHeight <= finishHeight && !prematureEnd && ctx.Err() == nil; thisHeight++ {
				// delay to simulate latency
				select {
				case <-mockClock.After(nextReceiveTime.Sub(mockClock.Now())):
				case <-ctx.Done():
					t.Fatalf("timed out simulating receive delay")
				}

				// collect eventsPerEpoch more events
				newEvents := make([]*types.ActorEvent, 0)
				for len(newEvents) < eventsPerEpoch && !prematureEnd && ctx.Err() == nil {
					select {
					case e, ok := <-eventChan: // receive the events from the subscription
						if ok {
							newEvents = append(newEvents, e)
						} else {
							prematureEnd = true
						}
					case <-ctx.Done():
						t.Fatalf("timed out waiting for event")
					}
					nextReceiveTime = nextReceiveTime.Add(tc.receiveSpeed)
				}

				if tc.expectComplete || !prematureEnd {
					// sanity check that we got what we expected this epoch
					req.Len(newEvents, eventsPerEpoch)
					epochEvents := allEvents[(thisHeight)*eventsPerEpoch : (thisHeight+1)*eventsPerEpoch]
					req.Equal(collectedToActorEvents(epochEvents), newEvents)
					gotEvents = append(gotEvents, newEvents...)
				}
			}

			req.Equal(tc.expectComplete, !prematureEnd, "expected to complete")
			if tc.expectComplete {
				req.Len(gotEvents, len(allEvents))
				req.Equal(collectedToActorEvents(allEvents), gotEvents)
			} else {
				req.NotEqual(len(gotEvents), len(allEvents))
			}

			// cleanup
			mockFilter.waitAssertClearSubChannelCalled(500 * time.Millisecond)
			mockFilterManager.waitAssertRemoved(mockFilter.ID(), 500*time.Millisecond)
		})
	}
}

func TestSubscribeActorEvents_OnlyHistorical(t *testing.T) {
	// Similar to TestSubscribeActorEvents but we set an explicit end that caps out at the current height
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	mockClock := clock.NewMock()

	const maxFilterHeightRange = 100
	const blockDelay = 30 * time.Second
	const filterStartHeight = 0
	const currentHeight = 10
	const eventsPerEpoch = 2

	minerAddr, err := address.NewIDAddress(uint64(rng.Int63()))
	require.NoError(t, err)

	for _, tc := range []struct {
		name                string
		blockTimeToComplete float64 // fraction of a block time that it takes to receive all events
		expectComplete      bool    // do we expect this to succeed?
	}{
		{"fast", 0, true},
		{"half block speed", 0.5, true},
		{"1.5 block speed", 1.5, false},
		{"twice block speed", 2, false},
	} {

		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)

			mockClock.Set(time.Now())
			mockFilterManager := newMockEventFilterManager(t)
			allEvents := makeCollectedEvents(t, rng, filterStartHeight, eventsPerEpoch, currentHeight-1)
			mockFilter := newMockFilter(ctx, t, rng, allEvents)
			mockFilterManager.expectInstall(abi.ChainEpoch(0), abi.ChainEpoch(currentHeight), cid.Undef, nil, nil, false, mockFilter)

			ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, currentHeight)})
			req.NoError(err)
			mockChain := newMockChainAccessor(t, ts)

			handler := NewActorEventHandlerWithClock(mockChain, mockFilterManager, blockDelay, maxFilterHeightRange, mockClock)

			aef := &types.ActorEventFilter{FromHeight: epochPtr(0), ToHeight: epochPtr(currentHeight)}
			eventChan, err := handler.SubscribeActorEvents(ctx, aef)
			req.NoError(err)

			gotEvents := make([]*types.ActorEvent, 0)

			// assume we can cleanly pick up all historical events in one go
		receiveLoop:
			for len(gotEvents) < len(allEvents) && ctx.Err() == nil {
				select {
				case e, ok := <-eventChan:
					if tc.expectComplete || ok {
						req.True(ok)
						gotEvents = append(gotEvents, e)
						mockClock.Add(time.Duration(float64(blockDelay) * tc.blockTimeToComplete / float64(len(allEvents))))
						// no need to advance the chain, we're also testing that's not necessary
						time.Sleep(2 * time.Millisecond) // catch a breath
					} else {
						break receiveLoop
					}
				case <-ctx.Done():
					t.Fatalf("timed out waiting for event, got %d/%d events", len(gotEvents), len(allEvents))
				}
			}
			if tc.expectComplete {
				req.Equal(collectedToActorEvents(allEvents), gotEvents)
			} else {
				req.NotEqual(len(gotEvents), len(allEvents))
			}
			// advance the chain and observe cleanup
			ts, err = types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, currentHeight+1)})
			req.NoError(err)
			mockChain.setHeaviestTipSet(ts)
			mockClock.Add(blockDelay)
			mockFilterManager.waitAssertRemoved(mockFilter.ID(), 1*time.Second)
		})
	}
}

var (
	_ ChainAccessor      = (*mockChainAccessor)(nil)
	_ filter.EventFilter = (*mockFilter)(nil)
	_ EventFilterManager = (*mockEventFilterManager)(nil)
)

type mockChainAccessor struct {
	t  *testing.T
	ts *types.TipSet
	lk sync.Mutex
}

func newMockChainAccessor(t *testing.T, ts *types.TipSet) *mockChainAccessor {
	return &mockChainAccessor{t: t, ts: ts}
}

func (m *mockChainAccessor) setHeaviestTipSet(ts *types.TipSet) {
	m.lk.Lock()
	defer m.lk.Unlock()
	m.ts = ts
}

func (m *mockChainAccessor) GetHeaviestTipSet() *types.TipSet {
	m.lk.Lock()
	defer m.lk.Unlock()
	return m.ts
}

type mockFilter struct {
	t                    *testing.T
	ctx                  context.Context
	id                   types.FilterID
	lastTaken            time.Time
	ch                   chan<- interface{}
	historicalEvents     []*filter.CollectedEvent
	subChannelCalls      int
	clearSubChannelCalls int
	lk                   sync.Mutex
}

func newMockFilter(ctx context.Context, t *testing.T, rng *pseudo.Rand, historicalEvents []*filter.CollectedEvent) *mockFilter {
	t.Helper()
	byt := make([]byte, 32)
	_, err := rng.Read(byt)
	require.NoError(t, err)
	return &mockFilter{
		t:                t,
		ctx:              ctx,
		id:               types.FilterID(byt),
		historicalEvents: historicalEvents,
	}
}

func (m *mockFilter) sendEventToChannel(e *filter.CollectedEvent) {
	m.lk.Lock()
	defer m.lk.Unlock()
	if m.ch != nil {
		select {
		case m.ch <- e:
		case <-m.ctx.Done():
		}
	}
}

func (m *mockFilter) waitAssertClearSubChannelCalled(timeout time.Duration) {
	m.t.Helper()
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(10 * time.Millisecond) {
		m.lk.Lock()
		c := m.clearSubChannelCalls
		m.lk.Unlock()
		switch c {
		case 0:
			continue
		case 1:
			return
		default:
			m.t.Fatalf("ClearSubChannel called more than once")
		}
	}
	m.t.Fatalf("ClearSubChannel not called")
}

func (m *mockFilter) ID() types.FilterID {
	return m.id
}

func (m *mockFilter) LastTaken() time.Time {
	return m.lastTaken
}

func (m *mockFilter) SetSubChannel(ch chan<- interface{}) {
	m.t.Helper()
	m.lk.Lock()
	defer m.lk.Unlock()
	m.subChannelCalls++
	m.ch = ch
}

func (m *mockFilter) ClearSubChannel() {
	m.t.Helper()
	m.lk.Lock()
	defer m.lk.Unlock()
	m.clearSubChannelCalls++
	m.ch = nil
}

func (m *mockFilter) TakeCollectedEvents(ctx context.Context) []*filter.CollectedEvent {
	e := m.historicalEvents
	m.historicalEvents = nil
	m.lastTaken = time.Now()
	return e
}

func (m *mockFilter) CollectEvents(ctx context.Context, tse *filter.TipSetEvents, reorg bool, ar filter.AddressResolver) error {
	m.t.Fatalf("unexpected call to CollectEvents")
	return nil
}

type filterManagerExpectation struct {
	minHeight, maxHeight abi.ChainEpoch
	tipsetCid            cid.Cid
	addresses            []address.Address
	keysWithCodec        map[string][]types.ActorEventBlock
	excludeReverted      bool
	returnFilter         filter.EventFilter
}

type mockEventFilterManager struct {
	t            *testing.T
	expectations []filterManagerExpectation
	removed      []types.FilterID
	lk           sync.Mutex
}

func newMockEventFilterManager(t *testing.T) *mockEventFilterManager {
	return &mockEventFilterManager{t: t}
}

func (m *mockEventFilterManager) expectInstall(
	minHeight, maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
	excludeReverted bool,
	returnFilter filter.EventFilter) {

	m.t.Helper()
	m.expectations = append(m.expectations, filterManagerExpectation{
		minHeight:       minHeight,
		maxHeight:       maxHeight,
		tipsetCid:       tipsetCid,
		addresses:       addresses,
		keysWithCodec:   keysWithCodec,
		excludeReverted: excludeReverted,
		returnFilter:    returnFilter,
	})
}

func (m *mockEventFilterManager) assertRemoved(id types.FilterID) {
	m.t.Helper()
	m.lk.Lock()
	defer m.lk.Unlock()
	require.Contains(m.t, m.removed, id)
}

func (m *mockEventFilterManager) waitAssertRemoved(id types.FilterID, timeout time.Duration) {
	m.t.Helper()
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(10 * time.Millisecond) {
		m.lk.Lock()
		if len(m.removed) == 0 {
			m.lk.Unlock()
			continue
		}
		defer m.lk.Unlock()
		require.Contains(m.t, m.removed, id)
		return
	}
	m.t.Fatalf("filter %x not removed", id)
}

func (m *mockEventFilterManager) Install(
	_ context.Context,
	minHeight, maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
	excludeReverted bool,
) (filter.EventFilter, error) {

	require.True(m.t, len(m.expectations) > 0, "unexpected call to Install")
	exp := m.expectations[0]
	m.expectations = m.expectations[1:]
	// check the expectation matches the call then return the attached filter
	require.Equal(m.t, exp.minHeight, minHeight)
	require.Equal(m.t, exp.maxHeight, maxHeight)
	require.Equal(m.t, exp.tipsetCid, tipsetCid)
	require.Equal(m.t, exp.addresses, addresses)
	require.Equal(m.t, exp.keysWithCodec, keysWithCodec)
	require.Equal(m.t, exp.excludeReverted, excludeReverted)
	return exp.returnFilter, nil
}

func (m *mockEventFilterManager) Remove(_ context.Context, id types.FilterID) error {
	m.lk.Lock()
	defer m.lk.Unlock()
	m.removed = append(m.removed, id)
	return nil
}

func newBlockHeader(minerAddr address.Address, height int64) *types.BlockHeader {
	return &types.BlockHeader{
		Miner: minerAddr,
		Ticket: &types.Ticket{
			VRFProof: []byte("vrf proof0000000vrf proof0000000"),
		},
		ElectionProof: &types.ElectionProof{
			VRFProof: []byte("vrf proof0000000vrf proof0000000"),
		},
		Parents:               []cid.Cid{testCid, testCid},
		ParentMessageReceipts: testCid,
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte("sign me up")},
		ParentWeight:          types.NewInt(123125126212),
		Messages:              testCid,
		Height:                abi.ChainEpoch(height),
		ParentStateRoot:       testCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte("sign me up")},
		ParentBaseFee:         types.NewInt(3432432843291),
	}
}

func epochPtr(i int) *abi.ChainEpoch {
	e := abi.ChainEpoch(i)
	return &e
}

func collectedToActorEvents(collected []*filter.CollectedEvent) []*types.ActorEvent {
	var out []*types.ActorEvent
	for _, c := range collected {
		out = append(out, &types.ActorEvent{
			Entries:   c.Entries,
			Emitter:   c.EmitterAddr,
			Reverted:  c.Reverted,
			Height:    c.Height,
			TipSetKey: c.TipSetKey,
			MsgCid:    c.MsgCid,
		})
	}
	return out
}

func makeCollectedEvents(t *testing.T, rng *pseudo.Rand, eventStartHeight, eventsPerHeight, eventEndHeight int64) []*filter.CollectedEvent {
	var out []*filter.CollectedEvent
	for h := eventStartHeight; h <= eventEndHeight; h++ {
		for i := int64(0); i < eventsPerHeight; i++ {
			out = append(out, makeCollectedEvent(t, rng, types.NewTipSetKey(mkCid(t, fmt.Sprintf("h=%d", h))), abi.ChainEpoch(h)))
		}
	}
	return out
}

func makeCollectedEvent(t *testing.T, rng *pseudo.Rand, tsKey types.TipSetKey, height abi.ChainEpoch) *filter.CollectedEvent {
	addr, err := address.NewIDAddress(uint64(rng.Int63()))
	require.NoError(t, err)

	return &filter.CollectedEvent{
		Entries: []types.EventEntry{
			{Flags: 0x01, Key: "k1", Codec: cid.Raw, Value: []byte("v1")},
			{Flags: 0x01, Key: "k2", Codec: cid.Raw, Value: []byte("v2")},
		},
		EmitterAddr: addr,
		EventIdx:    0,
		Reverted:    false,
		Height:      height,
		TipSetKey:   tsKey,
		MsgIdx:      0,
		MsgCid:      testCid,
	}
}

func mkCid(t *testing.T, s string) cid.Cid {
	h, err := multihash.Sum([]byte(s), multihash.SHA2_256, -1)
	require.NoError(t, err)
	return cid.NewCidV1(cid.Raw, h)
}

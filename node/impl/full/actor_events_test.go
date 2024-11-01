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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
)

var testCid = cid.MustParse("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")

func TestParseHeightRange(t *testing.T) {
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

func TestGetActorEventsRaw(t *testing.T) {
	ctx := context.Background()
	req := require.New(t)

	const (
		seed                 = 984651320
		maxFilterHeightRange = 100
	)

	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))

	minerAddr, err := address.NewIDAddress(uint64(rng.Int63()))
	req.NoError(err)

	c := mkCid(t, "c")
	tskey := types.NewTipSetKey(c)
	tsKeyCid, err := tskey.Cid()
	req.NoError(err)

	testCases := []struct {
		name                 string
		filter               *types.ActorEventFilter
		currentHeight        int64
		installMinHeight     int64
		installMaxHeight     int64
		installTipSetKey     cid.Cid
		installAddresses     []address.Address
		installKeysWithCodec map[string][]types.ActorEventBlock
		expectErr            string
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
			name: "query for tipset key",
			filter: &types.ActorEventFilter{
				TipSetKey: &tskey,
			},
			installTipSetKey: tsKeyCid,
			installMinHeight: 0,
			installMaxHeight: 0,
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
				efm.expectFill(abi.ChainEpoch(tc.installMinHeight), abi.ChainEpoch(tc.installMaxHeight), tc.installTipSetKey, tc.installAddresses, tc.installKeysWithCodec, filter)
			}

			ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, tc.currentHeight)})
			req.NoError(err)
			chain := newMockChainAccessor(t, ts)

			handler := NewActorEventHandler(chain, efm, 50*time.Millisecond, maxFilterHeightRange)

			gotEvents, err := handler.GetActorEventsRaw(ctx, tc.filter)
			if tc.expectErr != "" {
				req.Error(err)
				req.Contains(err.Error(), tc.expectErr)
			} else {
				req.NoError(err)
				expectedEvents := collectedToActorEvents(collectedEvents)
				req.Equal(expectedEvents, gotEvents)
			}
		})
	}
}

func TestSubscribeActorEventsRaw(t *testing.T) {
	const (
		seed                 = 984651320
		maxFilterHeightRange = 100
		blockDelay           = 30 * time.Second
		filterStartHeight    = 0
		currentHeight        = 10
		finishHeight         = 20
		eventsPerEpoch       = 2
	)
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	mockClock := clock.NewMock()

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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)

			mockClock.Set(time.Now())
			mockFilterManager := newMockEventFilterManager(t)
			allEvents := makeCollectedEvents(t, rng, filterStartHeight, eventsPerEpoch, finishHeight)
			historicalEvents := allEvents[0 : (currentHeight-filterStartHeight)*eventsPerEpoch]
			mockFilter := newMockFilter(ctx, t, rng, historicalEvents)
			mockFilterManager.expectInstall(abi.ChainEpoch(0), abi.ChainEpoch(tc.endEpoch), cid.Undef, nil, nil, mockFilter)

			ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, currentHeight)})
			req.NoError(err)
			mockChain := newMockChainAccessor(t, ts)

			handler := NewActorEventHandlerWithClock(mockChain, mockFilterManager, blockDelay, maxFilterHeightRange, mockClock)

			aef := &types.ActorEventFilter{FromHeight: epochPtr(0)}
			if tc.endEpoch >= 0 {
				aef.ToHeight = epochPtr(tc.endEpoch)
			}
			eventChan, err := handler.SubscribeActorEventsRaw(ctx, aef)
			req.NoError(err)

			// assume we can cleanly pick up all historical events in one go
			var gotEvents []*types.ActorEvent
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

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for thisHeight := int64(currentHeight); ctx.Err() == nil; thisHeight++ {
					ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, thisHeight)})
					req.NoError(err)
					mockChain.setHeaviestTipSet(ts)

					var eventsThisEpoch []*index.CollectedEvent
					if thisHeight <= finishHeight {
						eventsThisEpoch = allEvents[(thisHeight-filterStartHeight)*eventsPerEpoch : (thisHeight-filterStartHeight+2)*eventsPerEpoch]
					}
					for i := 0; i < eventsPerEpoch && ctx.Err() == nil; i++ {
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
				var newEvents []*types.ActorEvent
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
			mockFilter.requireClearSubChannelCalledEventually(500 * time.Millisecond)
			mockFilterManager.requireRemovedEventually(mockFilter.ID(), 500*time.Millisecond)
			cancel()
			wg.Wait() // wait for the chain to stop advancing
		})
	}
}

func TestSubscribeActorEventsRaw_OnlyHistorical(t *testing.T) {
	// Similar to TestSubscribeActorEventsRaw but we set an explicit end that caps out at the current height
	const (
		seed                 = 984651320
		maxFilterHeightRange = 100
		blockDelay           = 30 * time.Second
		filterStartHeight    = 0
		currentHeight        = 10
		eventsPerEpoch       = 2
	)
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	mockClock := clock.NewMock()

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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)

			mockClock.Set(time.Now())
			mockFilterManager := newMockEventFilterManager(t)
			allEvents := makeCollectedEvents(t, rng, filterStartHeight, eventsPerEpoch, currentHeight)
			mockFilter := newMockFilter(ctx, t, rng, allEvents)
			mockFilterManager.expectInstall(abi.ChainEpoch(0), abi.ChainEpoch(currentHeight), cid.Undef, nil, nil, mockFilter)

			ts, err := types.NewTipSet([]*types.BlockHeader{newBlockHeader(minerAddr, currentHeight)})
			req.NoError(err)
			mockChain := newMockChainAccessor(t, ts)

			handler := NewActorEventHandlerWithClock(mockChain, mockFilterManager, blockDelay, maxFilterHeightRange, mockClock)

			aef := &types.ActorEventFilter{FromHeight: epochPtr(0), ToHeight: epochPtr(currentHeight)}
			eventChan, err := handler.SubscribeActorEventsRaw(ctx, aef)
			req.NoError(err)

			var gotEvents []*types.ActorEvent

			// assume we can cleanly pick up all historical events in one go
		receiveLoop:
			for ctx.Err() == nil {
				select {
				case e, ok := <-eventChan:
					if ok {
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
			mockFilterManager.requireRemovedEventually(mockFilter.ID(), 1*time.Second)
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
	historicalEvents     []*index.CollectedEvent
	subChannelCalls      int
	clearSubChannelCalls int
	lk                   sync.Mutex
}

func newMockFilter(ctx context.Context, t *testing.T, rng *pseudo.Rand, historicalEvents []*index.CollectedEvent) *mockFilter {
	t.Helper()
	var id [32]byte
	_, err := rng.Read(id[:])
	require.NoError(t, err)
	return &mockFilter{
		t:                t,
		ctx:              ctx,
		id:               id,
		historicalEvents: historicalEvents,
	}
}

func (m *mockFilter) sendEventToChannel(e *index.CollectedEvent) {
	m.lk.Lock()
	defer m.lk.Unlock()
	if m.ch != nil {
		select {
		case m.ch <- e:
		case <-m.ctx.Done():
		}
	}
}

func (m *mockFilter) requireClearSubChannelCalledEventually(timeout time.Duration) {
	m.t.Helper()
	require.Eventually(m.t,
		func() bool {
			m.lk.Lock()
			c := m.clearSubChannelCalls
			m.lk.Unlock()
			switch c {
			case 0:
				return false
			case 1:
				return true
			default:
				m.t.Fatalf("ClearSubChannel called more than once: %d", c)
				return false
			}
		}, timeout, 10*time.Millisecond, "ClearSubChannel is not called exactly once")
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

func (m *mockFilter) TakeCollectedEvents(context.Context) []*index.CollectedEvent {
	e := m.historicalEvents
	m.historicalEvents = nil
	m.lastTaken = time.Now()
	return e
}

func (m *mockFilter) CollectEvents(context.Context, *filter.TipSetEvents, bool, filter.AddressResolver) error {
	m.t.Fatalf("unexpected call to CollectEvents")
	return nil
}

type filterManagerExpectation struct {
	minHeight, maxHeight abi.ChainEpoch
	tipsetCid            cid.Cid
	addresses            []address.Address
	keysWithCodec        map[string][]types.ActorEventBlock
	returnFilter         filter.EventFilter
}

type mockEventFilterManager struct {
	t                   *testing.T
	installExpectations []filterManagerExpectation
	fillExpectations    []filterManagerExpectation
	removed             []types.FilterID
	lk                  sync.Mutex
}

func newMockEventFilterManager(t *testing.T) *mockEventFilterManager {
	return &mockEventFilterManager{t: t}
}

func (m *mockEventFilterManager) expectFill(
	minHeight, maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
	returnFilter filter.EventFilter) {

	m.t.Helper()
	m.fillExpectations = append(m.fillExpectations, filterManagerExpectation{
		minHeight:     minHeight,
		maxHeight:     maxHeight,
		tipsetCid:     tipsetCid,
		addresses:     addresses,
		keysWithCodec: keysWithCodec,
		returnFilter:  returnFilter,
	})
}

func (m *mockEventFilterManager) expectInstall(
	minHeight, maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
	returnFilter filter.EventFilter) {

	m.t.Helper()
	m.installExpectations = append(m.installExpectations, filterManagerExpectation{
		minHeight:     minHeight,
		maxHeight:     maxHeight,
		tipsetCid:     tipsetCid,
		addresses:     addresses,
		keysWithCodec: keysWithCodec,
		returnFilter:  returnFilter,
	})
}

func (m *mockEventFilterManager) requireRemovedEventually(id types.FilterID, timeout time.Duration) {
	m.t.Helper()
	require.Eventuallyf(m.t, func() bool {
		m.lk.Lock()
		defer m.lk.Unlock()
		if len(m.removed) == 0 {
			return false
		}
		assert.Contains(m.t, m.removed, id)
		return true
	}, timeout, 10*time.Millisecond, "filter %x not removed", id)
}

func (m *mockEventFilterManager) Fill(
	_ context.Context,
	minHeight, maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
) (filter.EventFilter, error) {

	require.True(m.t, len(m.fillExpectations) > 0, "unexpected call to Fill")
	exp := m.fillExpectations[0]
	m.fillExpectations = m.fillExpectations[1:]
	// check the expectation matches the call then return the attached filter
	require.Equal(m.t, exp.minHeight, minHeight)
	require.Equal(m.t, exp.maxHeight, maxHeight)
	require.Equal(m.t, exp.tipsetCid, tipsetCid)
	require.Equal(m.t, exp.addresses, addresses)
	require.Equal(m.t, exp.keysWithCodec, keysWithCodec)
	return exp.returnFilter, nil
}

func (m *mockEventFilterManager) Install(
	_ context.Context,
	minHeight, maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
) (filter.EventFilter, error) {

	require.True(m.t, len(m.installExpectations) > 0, "unexpected call to Install")
	exp := m.installExpectations[0]
	m.installExpectations = m.installExpectations[1:]
	// check the expectation matches the call then return the attached filter
	require.Equal(m.t, exp.minHeight, minHeight)
	require.Equal(m.t, exp.maxHeight, maxHeight)
	require.Equal(m.t, exp.tipsetCid, tipsetCid)
	require.Equal(m.t, exp.addresses, addresses)
	require.Equal(m.t, exp.keysWithCodec, keysWithCodec)
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

func collectedToActorEvents(collected []*index.CollectedEvent) []*types.ActorEvent {
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

func makeCollectedEvents(t *testing.T, rng *pseudo.Rand, eventStartHeight, eventsPerHeight, eventEndHeight int64) []*index.CollectedEvent {
	var out []*index.CollectedEvent
	for h := eventStartHeight; h <= eventEndHeight; h++ {
		for i := int64(0); i < eventsPerHeight; i++ {
			out = append(out, makeCollectedEvent(t, rng, types.NewTipSetKey(mkCid(t, fmt.Sprintf("h=%d", h))), abi.ChainEpoch(h)))
		}
	}
	return out
}

func makeCollectedEvent(t *testing.T, rng *pseudo.Rand, tsKey types.TipSetKey, height abi.ChainEpoch) *index.CollectedEvent {
	addr, err := address.NewIDAddress(uint64(rng.Int63()))
	require.NoError(t, err)

	return &index.CollectedEvent{
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

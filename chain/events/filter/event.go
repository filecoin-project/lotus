package filter

import (
	"bytes"
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/index"
	cstore "github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

func isIndexedValue(b uint8) bool {
	// currently we mark the full entry as indexed if either the key
	// or the value are indexed; in the future we will need finer-grained
	// management of indices
	return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
}

// AddressResolver is a function that resolves an actor ID to an address. If the
// actor ID cannot be resolved to an address, the function must return
// address.Undef.
type AddressResolver func(context.Context, abi.ActorID, *types.TipSet) address.Address

type EventFilter interface {
	Filter

	TakeCollectedEvents(context.Context) []*index.CollectedEvent
	CollectEvents(context.Context, *TipSetEvents, bool, AddressResolver) error
}

type eventFilter struct {
	id        types.FilterID
	minHeight abi.ChainEpoch // minimum epoch to apply filter or -1 if no minimum
	maxHeight abi.ChainEpoch // maximum epoch to apply filter or -1 if no maximum
	tipsetCid cid.Cid
	addresses []address.Address // list of actor addresses that are extpected to emit the event

	keysWithCodec map[string][]types.ActorEventBlock // map of key names to a list of alternate values that may match
	maxResults    int                                // maximum number of results to collect, 0 is unlimited

	mu        sync.Mutex
	collected []*index.CollectedEvent
	lastTaken time.Time
	ch        chan<- interface{}
}

var _ Filter = (*eventFilter)(nil)

func (f *eventFilter) ID() types.FilterID {
	return f.id
}

func (f *eventFilter) SetSubChannel(ch chan<- interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = ch
	f.collected = nil
}

func (f *eventFilter) ClearSubChannel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = nil
}

func (f *eventFilter) CollectEvents(ctx context.Context, te *TipSetEvents, revert bool, resolver AddressResolver) error {
	if !f.matchTipset(te) {
		return nil
	}

	ems, err := te.messages(ctx)
	if err != nil {
		return xerrors.Errorf("load executed messages: %w", err)
	}

	eventCount := 0

	for msgIdx, em := range ems {
		for _, ev := range em.Events() {
			addr := resolver(ctx, ev.Emitter, te.rctTs)
			if addr == address.Undef {
				// not an address we will be able to match against
				continue
			}

			if !f.matchAddress(addr) {
				continue
			}
			if !f.matchKeys(ev.Entries) {
				continue
			}

			// event matches filter, so record it
			cev := &index.CollectedEvent{
				Entries:     ev.Entries,
				EmitterAddr: addr,
				EventIdx:    eventCount,
				Reverted:    revert,
				Height:      te.msgTs.Height(),
				TipSetKey:   te.msgTs.Key(),
				MsgCid:      em.Message().Cid(),
				MsgIdx:      msgIdx,
			}

			f.mu.Lock()
			// if we have a subscription channel then push event to it
			if f.ch != nil {
				f.ch <- cev
				f.mu.Unlock()
				continue
			}

			if f.maxResults > 0 && len(f.collected) == f.maxResults {
				copy(f.collected, f.collected[1:])
				f.collected = f.collected[:len(f.collected)-1]
			}
			f.collected = append(f.collected, cev)
			f.mu.Unlock()
			eventCount++
		}
	}

	return nil
}

func (f *eventFilter) setCollectedEvents(ces []*index.CollectedEvent) {
	f.mu.Lock()
	f.collected = ces
	f.mu.Unlock()
}

func (f *eventFilter) TakeCollectedEvents(ctx context.Context) []*index.CollectedEvent {
	f.mu.Lock()
	collected := f.collected
	f.collected = nil
	f.lastTaken = time.Now().UTC()
	f.mu.Unlock()

	return collected
}

func (f *eventFilter) LastTaken() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastTaken
}

// matchTipset reports whether this filter matches the given tipset
func (f *eventFilter) matchTipset(te *TipSetEvents) bool {
	if f.tipsetCid != cid.Undef {
		tsCid, err := te.Cid()
		if err != nil {
			return false
		}
		return f.tipsetCid.Equals(tsCid)
	}

	if f.minHeight >= 0 && f.minHeight > te.Height() {
		return false
	}
	if f.maxHeight >= 0 && f.maxHeight < te.Height() {
		return false
	}
	return true
}

func (f *eventFilter) matchAddress(o address.Address) bool {
	if len(f.addresses) == 0 {
		return true
	}

	// Assume short lists of addresses
	// TODO: binary search for longer lists or restrict list length
	return slices.Contains(f.addresses, o)
}

func (f *eventFilter) matchKeys(ees []types.EventEntry) bool {
	if len(f.keysWithCodec) == 0 {
		return true
	}
	// TODO: optimize this naive algorithm
	// tracked in https://github.com/filecoin-project/lotus/issues/9987

	// Note keys names may be repeated so we may have multiple opportunities to match

	matched := map[string]bool{}
	for _, ee := range ees {
		// Skip an entry that is not indexable
		if !isIndexedValue(ee.Flags) {
			continue
		}

		keyname := ee.Key

		// skip if we have already matched this key
		if matched[keyname] {
			continue
		}

		wantlist, ok := f.keysWithCodec[keyname]
		if !ok || len(wantlist) == 0 {
			continue
		}

		for _, w := range wantlist {
			if bytes.Equal(w.Value, ee.Value) && w.Codec == ee.Codec {
				matched[keyname] = true
				break
			}
		}

		if len(matched) == len(f.keysWithCodec) {
			// all keys have been matched
			return true
		}

	}

	return false
}

type TipSetEvents struct {
	rctTs *types.TipSet // rctTs is the tipset containing the receipts of executed messages
	msgTs *types.TipSet // msgTs is the tipset containing the messages that have been executed

	load func(ctx context.Context, msgTs, rctTs *types.TipSet) ([]executedMessage, error)

	once sync.Once // for lazy population of ems
	ems  []executedMessage
	err  error
}

func (te *TipSetEvents) Height() abi.ChainEpoch {
	return te.msgTs.Height()
}

func (te *TipSetEvents) Cid() (cid.Cid, error) {
	return te.msgTs.Key().Cid()
}

func (te *TipSetEvents) messages(ctx context.Context) ([]executedMessage, error) {
	te.once.Do(func() {
		// populate executed message list
		ems, err := te.load(ctx, te.msgTs, te.rctTs)
		if err != nil {
			te.err = err
			return
		}
		te.ems = ems
	})
	return te.ems, te.err
}

type executedMessage struct {
	msg types.ChainMsg
	rct *types.MessageReceipt
	// events extracted from receipt
	evs []*types.Event
}

func (e *executedMessage) Message() types.ChainMsg {
	return e.msg
}

func (e *executedMessage) Receipt() *types.MessageReceipt {
	return e.rct
}

func (e *executedMessage) Events() []*types.Event {
	return e.evs
}

type EventFilterManager struct {
	ChainStore       *cstore.ChainStore
	AddressResolver  AddressResolver
	MaxFilterResults int
	ChainIndexer     index.Indexer

	mu            sync.Mutex // guards mutations to filters
	filters       map[types.FilterID]EventFilter
	currentHeight abi.ChainEpoch
}

func (m *EventFilterManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentHeight = to.Height()

	if len(m.filters) == 0 {
		return nil
	}

	tse := &TipSetEvents{
		msgTs: from,
		rctTs: to,
		load:  m.loadExecutedMessages,
	}

	tsAddressResolver := tipSetCachedAddressResolver(m.AddressResolver)

	g, ctx := errgroup.WithContext(ctx)
	for _, f := range m.filters {
		g.Go(func() error {
			return f.CollectEvents(ctx, tse, false, tsAddressResolver)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func (m *EventFilterManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentHeight = to.Height()

	if len(m.filters) == 0 {
		return nil
	}

	tse := &TipSetEvents{
		msgTs: to,
		rctTs: from,
		load:  m.loadExecutedMessages,
	}

	tsAddressResolver := tipSetCachedAddressResolver(m.AddressResolver)

	g, ctx := errgroup.WithContext(ctx)
	for _, f := range m.filters {
		g.Go(func() error {
			return f.CollectEvents(ctx, tse, true, tsAddressResolver)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func (m *EventFilterManager) Fill(
	ctx context.Context,
	minHeight,
	maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
) (EventFilter, error) {
	m.mu.Lock()
	if m.currentHeight == 0 {
		// sync in progress, we haven't had an Apply
		m.currentHeight = m.ChainStore.GetHeaviestTipSet().Height()
	}
	currentHeight := m.currentHeight
	m.mu.Unlock()

	if m.ChainIndexer == nil && minHeight != -1 && minHeight < currentHeight {
		return nil, xerrors.Errorf("historic event index disabled")
	}

	id, err := newFilterID()
	if err != nil {
		return nil, xerrors.Errorf("new filter id: %w", err)
	}

	f := &eventFilter{
		id:            id,
		minHeight:     minHeight,
		maxHeight:     maxHeight,
		tipsetCid:     tipsetCid,
		addresses:     addresses,
		keysWithCodec: keysWithCodec,
		maxResults:    m.MaxFilterResults,
	}

	if m.ChainIndexer != nil && minHeight != -1 && minHeight < currentHeight {
		ef := &index.EventFilter{
			MinHeight:     minHeight,
			MaxHeight:     maxHeight,
			TipsetCid:     tipsetCid,
			Addresses:     addresses,
			KeysWithCodec: keysWithCodec,
			MaxResults:    m.MaxFilterResults,
		}

		ces, err := m.ChainIndexer.GetEventsForFilter(ctx, ef)
		if err != nil {
			return nil, xerrors.Errorf("get events for filter: %w", err)
		}

		f.setCollectedEvents(ces)
	}

	return f, nil
}

func (m *EventFilterManager) Install(
	ctx context.Context,
	minHeight,
	maxHeight abi.ChainEpoch,
	tipsetCid cid.Cid,
	addresses []address.Address,
	keysWithCodec map[string][]types.ActorEventBlock,
) (EventFilter, error) {
	f, err := m.Fill(ctx, minHeight, maxHeight, tipsetCid, addresses, keysWithCodec)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if m.filters == nil {
		m.filters = make(map[types.FilterID]EventFilter)
	}
	m.filters[f.(*eventFilter).id] = f
	m.mu.Unlock()

	return f, nil
}

func (m *EventFilterManager) Remove(ctx context.Context, id types.FilterID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found := m.filters[id]; !found {
		return ErrFilterNotFound
	}
	delete(m.filters, id)
	return nil
}

func (m *EventFilterManager) loadExecutedMessages(ctx context.Context, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
	msgs, err := m.ChainStore.MessagesForTipset(ctx, msgTs)
	if err != nil {
		return nil, xerrors.Errorf("read messages: %w", err)
	}

	st := m.ChainStore.ActorStore(ctx)

	arr, err := blockadt.AsArray(st, rctTs.Blocks()[0].ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("load receipts amt: %w", err)
	}

	if uint64(len(msgs)) != arr.Length() {
		return nil, xerrors.Errorf("mismatching message and receipt counts (%d msgs, %d rcts)", len(msgs), arr.Length())
	}

	ems := make([]executedMessage, len(msgs))

	for i := 0; i < len(msgs); i++ {
		ems[i].msg = msgs[i]

		var rct types.MessageReceipt
		found, err := arr.Get(uint64(i), &rct)
		if err != nil {
			return nil, xerrors.Errorf("load receipt: %w", err)
		}
		if !found {
			return nil, xerrors.Errorf("receipt %d not found", i)
		}
		ems[i].rct = &rct

		if rct.EventsRoot == nil {
			continue
		}

		evtArr, err := amt4.LoadAMT(ctx, st, *rct.EventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
		if err != nil {
			return nil, xerrors.Errorf("load events amt: %w", err)
		}

		ems[i].evs = make([]*types.Event, evtArr.Len())
		var evt types.Event
		err = evtArr.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
			if u > math.MaxInt {
				return xerrors.Errorf("too many events")
			}
			if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
				return err
			}

			cpy := evt
			ems[i].evs[int(u)] = &cpy //nolint:scopelint
			return nil
		})

		if err != nil {
			return nil, xerrors.Errorf("read events: %w", err)
		}

	}

	return ems, nil
}

// tipSetCachedAddressResolver returns a thread-safe function that resolves actor IDs to addresses
// with a cache that is shared across all calls to the returned function. This should only be used
// for a single TipSet, as the resolution may vary across TipSets and the cache does not account for
// this.
func tipSetCachedAddressResolver(resolver AddressResolver) func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) address.Address {
	addressLookups := make(map[abi.ActorID]address.Address)
	var addressLookupsLk sync.Mutex

	return func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) address.Address {
		addressLookupsLk.Lock()
		defer addressLookupsLk.Unlock()

		addr, ok := addressLookups[emitter]
		if !ok {
			addr = resolver(ctx, emitter, ts)
			addressLookups[emitter] = addr
		}

		return addr
	}
}

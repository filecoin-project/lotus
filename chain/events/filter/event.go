package filter

import (
	"bytes"
	"context"
	"math"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	cstore "github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

func isIndexedValue(b uint8) bool {
	// currently we mark the full entry as indexed if either the key
	// or the value are indexed; in the future we will need finer-grained
	// management of indices
	return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
}

type EventFilter struct {
	id         types.FilterID
	minHeight  abi.ChainEpoch // minimum epoch to apply filter or -1 if no minimum
	maxHeight  abi.ChainEpoch // maximum epoch to apply filter or -1 if no maximum
	tipsetCid  cid.Cid
	addresses  []address.Address   // list of f4 actor addresses that are extpected to emit the event
	keys       map[string][][]byte // map of key names to a list of alternate values that may match
	maxResults int                 // maximum number of results to collect, 0 is unlimited

	mu        sync.Mutex
	collected []*CollectedEvent
	lastTaken time.Time
	ch        chan<- interface{}
}

var _ Filter = (*EventFilter)(nil)

type CollectedEvent struct {
	Entries     []types.EventEntry
	EmitterAddr address.Address // f4 address of emitter
	EventIdx    int             // index of the event within the list of emitted events
	Reverted    bool
	Height      abi.ChainEpoch
	TipSetKey   types.TipSetKey // tipset that contained the message
	MsgIdx      int             // index of the message in the tipset
	MsgCid      cid.Cid         // cid of message that produced event
}

func (f *EventFilter) ID() types.FilterID {
	return f.id
}

func (f *EventFilter) SetSubChannel(ch chan<- interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = ch
	f.collected = nil
}

func (f *EventFilter) ClearSubChannel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = nil
}

func (f *EventFilter) CollectEvents(ctx context.Context, te *TipSetEvents, revert bool, resolver func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)) error {
	if !f.matchTipset(te) {
		return nil
	}

	// cache of lookups between actor id and f4 address
	addressLookups := make(map[abi.ActorID]address.Address)

	ems, err := te.messages(ctx)
	if err != nil {
		return xerrors.Errorf("load executed messages: %w", err)
	}
	for msgIdx, em := range ems {
		for evIdx, ev := range em.Events() {
			// lookup address corresponding to the actor id
			addr, found := addressLookups[ev.Emitter]
			if !found {
				var ok bool
				addr, ok = resolver(ctx, ev.Emitter, te.rctTs)
				if !ok {
					// not an address we will be able to match against
					continue
				}
				addressLookups[ev.Emitter] = addr
			}

			if !f.matchAddress(addr) {
				continue
			}
			if !f.matchKeys(ev.Entries) {
				continue
			}

			// event matches filter, so record it
			cev := &CollectedEvent{
				Entries:     ev.Entries,
				EmitterAddr: addr,
				EventIdx:    evIdx,
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
		}
	}

	return nil
}

func (f *EventFilter) setCollectedEvents(ces []*CollectedEvent) {
	f.mu.Lock()
	f.collected = ces
	f.mu.Unlock()
}

func (f *EventFilter) TakeCollectedEvents(ctx context.Context) []*CollectedEvent {
	f.mu.Lock()
	collected := f.collected
	f.collected = nil
	f.lastTaken = time.Now().UTC()
	f.mu.Unlock()

	return collected
}

func (f *EventFilter) LastTaken() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastTaken
}

// matchTipset reports whether this filter matches the given tipset
func (f *EventFilter) matchTipset(te *TipSetEvents) bool {
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

func (f *EventFilter) matchAddress(o address.Address) bool {
	if len(f.addresses) == 0 {
		return true
	}

	// Assume short lists of addresses
	// TODO: binary search for longer lists or restrict list length
	for _, a := range f.addresses {
		if a == o {
			return true
		}
	}
	return false
}

func (f *EventFilter) matchKeys(ees []types.EventEntry) bool {
	if len(f.keys) == 0 {
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

		wantlist, ok := f.keys[keyname]
		if !ok || len(wantlist) == 0 {
			continue
		}

		for _, w := range wantlist {
			if bytes.Equal(w, ee.Value) {
				matched[keyname] = true
				break
			}
		}

		if len(matched) == len(f.keys) {
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
	AddressResolver  func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)
	MaxFilterResults int
	EventIndex       *EventIndex

	mu            sync.Mutex // guards mutations to filters
	filters       map[types.FilterID]*EventFilter
	currentHeight abi.ChainEpoch
}

func (m *EventFilterManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentHeight = to.Height()

	if len(m.filters) == 0 && m.EventIndex == nil {
		return nil
	}

	tse := &TipSetEvents{
		msgTs: from,
		rctTs: to,
		load:  m.loadExecutedMessages,
	}

	if m.EventIndex != nil {
		if err := m.EventIndex.CollectEvents(ctx, tse, false, m.AddressResolver); err != nil {
			return err
		}
	}

	// TODO: could run this loop in parallel with errgroup if there are many filters
	for _, f := range m.filters {
		if err := f.CollectEvents(ctx, tse, false, m.AddressResolver); err != nil {
			return err
		}
	}

	return nil
}

func (m *EventFilterManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentHeight = to.Height()

	if len(m.filters) == 0 && m.EventIndex == nil {
		return nil
	}

	tse := &TipSetEvents{
		msgTs: to,
		rctTs: from,
		load:  m.loadExecutedMessages,
	}

	if m.EventIndex != nil {
		if err := m.EventIndex.CollectEvents(ctx, tse, true, m.AddressResolver); err != nil {
			return err
		}
	}

	// TODO: could run this loop in parallel with errgroup if there are many filters
	for _, f := range m.filters {
		if err := f.CollectEvents(ctx, tse, true, m.AddressResolver); err != nil {
			return err
		}
	}

	return nil
}

func (m *EventFilterManager) Install(ctx context.Context, minHeight, maxHeight abi.ChainEpoch, tipsetCid cid.Cid, addresses []address.Address, keys map[string][][]byte) (*EventFilter, error) {
	m.mu.Lock()
	currentHeight := m.currentHeight
	m.mu.Unlock()

	if m.EventIndex == nil && minHeight != -1 && minHeight < currentHeight {
		return nil, xerrors.Errorf("historic event index disabled")
	}

	id, err := newFilterID()
	if err != nil {
		return nil, xerrors.Errorf("new filter id: %w", err)
	}

	f := &EventFilter{
		id:         id,
		minHeight:  minHeight,
		maxHeight:  maxHeight,
		tipsetCid:  tipsetCid,
		addresses:  addresses,
		keys:       keys,
		maxResults: m.MaxFilterResults,
	}

	if m.EventIndex != nil && minHeight != -1 && minHeight < currentHeight {
		// Filter needs historic events
		if err := m.EventIndex.PrefillFilter(ctx, f); err != nil {
			return nil, err
		}
	}

	m.mu.Lock()
	if m.filters == nil {
		m.filters = make(map[types.FilterID]*EventFilter)
	}
	m.filters[id] = f
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

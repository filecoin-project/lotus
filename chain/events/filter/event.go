package filter

import (
	"bytes"
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	cstore "github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
)

const indexed uint8 = 0x01

type EventFilter struct {
	id        string
	minHeight abi.ChainEpoch // minimum epoch to apply filter or -1 if no minimum
	maxHeight abi.ChainEpoch // maximum epoch to apply filter or -1 if no maximum
	tipsetCid cid.Cid
	addresses []address.Address   // list of actor ids that originated the event
	keys      map[string][][]byte // map of key names to a list of alternate values that may match

	mu        sync.Mutex
	collected []*CollectedEvent
}

var _ Filter = (*EventFilter)(nil)

type CollectedEvent struct {
	Event *types.Event
}

func (f *EventFilter) ID() string {
	return f.id
}

func (f *EventFilter) CollectEvents(ctx context.Context, te *TipSetEvents, revert bool) error {
	if !f.matchTipset(te) {
		return nil
	}

	ems, err := te.Messages(ctx)
	if err != nil {
		return xerrors.Errorf("load executed messages: %w", err)
	}
	for _, em := range ems {
		for _, ev := range em.Events() {
			if !f.matchAddress(ev.Emitter) {
				continue
			}
			if !f.matchKeys(ev.Entries) {
				continue
			}

			// event matches filter, so record it
			cev := &CollectedEvent{
				Event: ev,
			}
			f.mu.Lock()
			f.collected = append(f.collected, cev)
			f.mu.Unlock()
		}
	}

	return nil
}

func (f *EventFilter) TakeCollectedEvents(ctx context.Context) []*CollectedEvent {
	f.mu.Lock()
	collected := f.collected
	f.collected = nil
	f.mu.Unlock()

	return collected
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
	// TODO: binary search for longer lists
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
	// Note keys names may be repeated so we may have multiple opportunities to match

	matched := map[string]bool{}
	for _, ee := range ees {
		// Skip an entry that is not indexable
		if ee.Flags&indexed != indexed {
			continue
		}

		keyname := string(ee.Key)

		// skip if we have already matched this key
		if matched[keyname] {
			continue
		}

		wantlist, ok := f.keys[keyname]
		if !ok {
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

func (te *TipSetEvents) Messages(ctx context.Context) ([]executedMessage, error) {
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
	msg *types.Message
	rct *types.MessageReceipt
	// events extracted from receipt
	evs []*types.Event
}

func (e *executedMessage) Message() *types.Message {
	return e.msg
}

func (e *executedMessage) Receipt() *types.MessageReceipt {
	return e.rct
}

func (e *executedMessage) Events() []*types.Event {
	return e.evs
}

type EventFilterManager struct {
	ChainStore cstore.ChainStore

	mu      sync.Mutex // guards mutations to filters
	filters map[string]*EventFilter
}

func (m *EventFilterManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filters) == 0 {
		return nil
	}

	tse := &TipSetEvents{
		msgTs: from,
		rctTs: to,
		load:  m.loadExecutedMessages,
	}

	for _, f := range m.filters {
		f.CollectEvents(ctx, tse, false)
	}

	return nil
}

func (m *EventFilterManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filters) == 0 {
		return nil
	}

	tse := &TipSetEvents{
		msgTs: to,
		rctTs: from,
		load:  m.loadExecutedMessages,
	}

	// TODO: could run this loop in parallel with errgroup
	for _, f := range m.filters {
		f.CollectEvents(ctx, tse, true)
	}

	return nil
}

func (m *EventFilterManager) Install(ctx context.Context, minHeight, maxHeight abi.ChainEpoch, tipsetCid cid.Cid, addresses []address.Address, keys map[string][][]byte) (*EventFilter, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}

	f := &EventFilter{
		id:        id.String(),
		minHeight: minHeight,
		maxHeight: maxHeight,
		tipsetCid: tipsetCid,
		addresses: addresses,
		keys:      keys,
	}

	m.mu.Lock()
	m.filters[id.String()] = f
	m.mu.Unlock()

	return f, nil
}

func (m *EventFilterManager) Remove(ctx context.Context, id string) error {
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
		ems[i].msg = msgs[i].VMMessage()

		var rct types.MessageReceipt
		found, err := arr.Get(uint64(i), &rct)
		if err != nil {
			return nil, xerrors.Errorf("load receipt: %w", err)
		}
		if !found {
			return nil, xerrors.Errorf("receipt %d not found", i)
		}
		ems[i].rct = &rct

		evtArr, err := blockadt.AsArray(st, rct.Events)
		if err != nil {
			return nil, xerrors.Errorf("load events amt: %w", err)
		}

		ems[i].evs = make([]*types.Event, evtArr.Length())
		var evt types.Event
		err = arr.ForEach(&evt, func(i int64) error {
			cpy := evt
			ems[i].evs[int(i)] = &cpy
			return nil
		})

	}

	return ems, nil
}

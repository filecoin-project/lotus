package full

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/zyedidia/generic/queue"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

type filterEventCollector interface {
	TakeCollectedEvents(context.Context) []*filter.CollectedEvent
}

type filterMessageCollector interface {
	TakeCollectedMessages(context.Context) []*types.SignedMessage
}

type filterTipSetCollector interface {
	TakeCollectedTipSets(context.Context) []types.TipSetKey
}

func ethLogFromEvent(entries []types.EventEntry) (data []byte, topics []ethtypes.EthHash, ok bool) {
	var (
		topicsFound      [4]bool
		topicsFoundCount int
		dataFound        bool
	)
	// Topics must be non-nil, even if empty. So we might as well pre-allocate for 4 (the max).
	topics = make([]ethtypes.EthHash, 0, 4)
	for _, entry := range entries {
		// Drop events with non-raw topics. Built-in actors emit CBOR, and anything else would be
		// invalid anyway.
		if entry.Codec != cid.Raw {
			return nil, nil, false
		}
		// Check if the key is t1..t4
		if len(entry.Key) == 2 && "t1" <= entry.Key && entry.Key <= "t4" {
			// '1' - '1' == 0, etc.
			idx := int(entry.Key[1] - '1')

			// Drop events with mis-sized topics.
			if len(entry.Value) != 32 {
				log.Warnw("got an EVM event topic with an invalid size", "key", entry.Key, "size", len(entry.Value))
				return nil, nil, false
			}

			// Drop events with duplicate topics.
			if topicsFound[idx] {
				log.Warnw("got a duplicate EVM event topic", "key", entry.Key)
				return nil, nil, false
			}
			topicsFound[idx] = true
			topicsFoundCount++

			// Extend the topics array
			for len(topics) <= idx {
				topics = append(topics, ethtypes.EthHash{})
			}
			copy(topics[idx][:], entry.Value)
		} else if entry.Key == "d" {
			// Drop events with duplicate data fields.
			if dataFound {
				log.Warnw("got duplicate EVM event data")
				return nil, nil, false
			}

			dataFound = true
			data = entry.Value
		} else {
			// Skip entries we don't understand (makes it easier to extend things).
			// But we warn for now because we don't expect them.
			log.Warnw("unexpected event entry", "key", entry.Key)
		}

	}

	// Drop events with skipped topics.
	if len(topics) != topicsFoundCount {
		log.Warnw("EVM event topic length mismatch", "expected", len(topics), "actual", topicsFoundCount)
		return nil, nil, false
	}
	return data, topics, true
}

func ethFilterResultFromEvents(ctx context.Context, evs []*filter.CollectedEvent, sa StateAPI) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}
	for _, ev := range evs {
		log := ethtypes.EthLog{
			Removed:          ev.Reverted,
			LogIndex:         ethtypes.EthUint64(ev.EventIdx),
			TransactionIndex: ethtypes.EthUint64(ev.MsgIdx),
			BlockNumber:      ethtypes.EthUint64(ev.Height),
		}
		var (
			err error
			ok  bool
		)

		log.Data, log.Topics, ok = ethLogFromEvent(ev.Entries)
		if !ok {
			continue
		}

		log.Address, err = ethtypes.EthAddressFromFilecoinAddress(ev.EmitterAddr)
		if err != nil {
			return nil, err
		}

		log.TransactionHash, err = ethTxHashFromMessageCid(ctx, ev.MsgCid, sa)
		if err != nil {
			return nil, err
		}
		c, err := ev.TipSetKey.Cid()
		if err != nil {
			return nil, err
		}
		log.BlockHash, err = ethtypes.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, log)
	}

	return res, nil
}

func ethFilterResultFromTipSets(tsks []types.TipSetKey) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}

	for _, tsk := range tsks {
		c, err := tsk.Cid()
		if err != nil {
			return nil, err
		}
		hash, err := ethtypes.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

func ethFilterResultFromMessages(cs []*types.SignedMessage) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}

	for _, c := range cs {
		hash, err := ethTxHashFromSignedMessage(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

type EthSubscriptionManager struct {
	Chain    *store.ChainStore
	StateAPI StateAPI
	ChainAPI ChainAPI
	mu       sync.Mutex
	subs     map[ethtypes.EthSubscriptionID]*ethSubscription
}

func (e *EthSubscriptionManager) StartSubscription(ctx context.Context, out ethSubscriptionCallback, dropFilter func(context.Context, filter.Filter) error) (*ethSubscription, error) { // nolint
	rawid, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}
	id := ethtypes.EthSubscriptionID{}
	copy(id[:], rawid[:]) // uuid is 16 bytes

	ctx, quit := context.WithCancel(ctx)

	sub := &ethSubscription{
		Chain:           e.Chain,
		StateAPI:        e.StateAPI,
		ChainAPI:        e.ChainAPI,
		uninstallFilter: dropFilter,
		id:              id,
		in:              make(chan interface{}, 200),
		out:             out,
		quit:            quit,

		toSend:   queue.New[[]byte](),
		sendCond: make(chan struct{}, 1),
	}

	e.mu.Lock()
	if e.subs == nil {
		e.subs = make(map[ethtypes.EthSubscriptionID]*ethSubscription)
	}
	e.subs[sub.id] = sub
	e.mu.Unlock()

	go sub.start(ctx)
	go sub.startOut(ctx)

	return sub, nil
}

func (e *EthSubscriptionManager) StopSubscription(ctx context.Context, id ethtypes.EthSubscriptionID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sub, ok := e.subs[id]
	if !ok {
		return xerrors.Errorf("subscription not found")
	}
	sub.stop()
	delete(e.subs, id)

	return nil
}

type ethSubscriptionCallback func(context.Context, jsonrpc.RawParams) error

const maxSendQueue = 20000

type ethSubscription struct {
	Chain           *store.ChainStore
	StateAPI        StateAPI
	ChainAPI        ChainAPI
	uninstallFilter func(context.Context, filter.Filter) error
	id              ethtypes.EthSubscriptionID
	in              chan interface{}
	out             ethSubscriptionCallback

	mu      sync.Mutex
	filters []filter.Filter
	quit    func()

	sendLk       sync.Mutex
	sendQueueLen int
	toSend       *queue.Queue[[]byte]
	sendCond     chan struct{}
}

func (e *ethSubscription) addFilter(ctx context.Context, f filter.Filter) {
	e.mu.Lock()
	defer e.mu.Unlock()

	f.SetSubChannel(e.in)
	e.filters = append(e.filters, f)
}

// sendOut processes the final subscription queue. It's here in case the subscriber
// is slow, and we need to buffer the messages.
func (e *ethSubscription) startOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.sendCond:
			e.sendLk.Lock()

			for !e.toSend.Empty() {
				front := e.toSend.Dequeue()
				e.sendQueueLen--

				e.sendLk.Unlock()

				if err := e.out(ctx, front); err != nil {
					log.Warnw("error sending subscription response, killing subscription", "sub", e.id, "error", err)
					e.stop()
					return
				}

				e.sendLk.Lock()
			}

			e.sendLk.Unlock()
		}
	}
}

func (e *ethSubscription) send(ctx context.Context, v interface{}) {
	resp := ethtypes.EthSubscriptionResponse{
		SubscriptionID: e.id,
		Result:         v,
	}

	outParam, err := json.Marshal(resp)
	if err != nil {
		log.Warnw("marshaling subscription response", "sub", e.id, "error", err)
		return
	}

	e.sendLk.Lock()
	defer e.sendLk.Unlock()

	e.toSend.Enqueue(outParam)

	e.sendQueueLen++
	if e.sendQueueLen > maxSendQueue {
		log.Warnw("subscription send queue full, killing subscription", "sub", e.id)
		e.stop()
		return
	}

	select {
	case e.sendCond <- struct{}{}:
	default: // already signalled, and we're holding the lock so we know that the event will be processed
	}
}

func (e *ethSubscription) start(ctx context.Context) {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case v := <-e.in:
			switch vt := v.(type) {
			case *filter.CollectedEvent:
				evs, err := ethFilterResultFromEvents(ctx, []*filter.CollectedEvent{vt}, e.StateAPI)
				if err != nil {
					continue
				}

				for _, r := range evs.Results {
					e.send(ctx, r)
				}
			case *types.TipSet:
				ev, err := newEthBlockFromFilecoinTipSet(ctx, vt, true, e.Chain, e.StateAPI)
				if err != nil {
					break
				}

				e.send(ctx, ev)
			case *types.SignedMessage: // mpool txid
				evs, err := ethFilterResultFromMessages([]*types.SignedMessage{vt})
				if err != nil {
					continue
				}

				for _, r := range evs.Results {
					e.send(ctx, r)
				}
			default:
				log.Warnf("unexpected subscription value type: %T", vt)
			}
		}
	}
}

func (e *ethSubscription) stop() {
	e.mu.Lock()
	if e.quit == nil {
		e.mu.Unlock()
		return
	}

	if e.quit != nil {
		e.quit()
		e.quit = nil
		e.mu.Unlock()

		for _, f := range e.filters {
			// note: the context in actually unused in uninstallFilter
			if err := e.uninstallFilter(context.TODO(), f); err != nil {
				// this will leave the filter a zombie, collecting events up to the maximum allowed
				log.Warnf("failed to remove filter when unsubscribing: %v", err)
			}
		}
	}
}

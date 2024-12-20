package eth

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/zyedidia/generic/queue"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

const maxSendQueue = 20000

type EthSubscriptionManager struct {
	chainStore   ChainStore
	stateManager StateManager
	mu           sync.Mutex
	subs         map[ethtypes.EthSubscriptionID]*ethSubscription
}

func NewEthSubscriptionManager(chainStore ChainStore, stateManager StateManager) *EthSubscriptionManager {
	return &EthSubscriptionManager{
		chainStore:   chainStore,
		stateManager: stateManager,
	}
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
		chainStore:      e.chainStore,
		stateManager:    e.stateManager,
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
		return xerrors.New("subscription not found")
	}
	sub.stop()
	delete(e.subs, id)

	return nil
}

type ethSubscription struct {
	chainStore      ChainStore
	stateManager    StateManager
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

	lastSentTipset *types.TipSetKey
}

type ethSubscriptionCallback func(context.Context, jsonrpc.RawParams) error

func (e *ethSubscription) addFilter(f filter.Filter) {
	e.mu.Lock()
	defer e.mu.Unlock()

	f.SetSubChannel(e.in)
	e.filters = append(e.filters, f)
}

// startOut processes the final subscription queue. It's here in case the subscriber
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
			case *index.CollectedEvent:
				evs, err := ethFilterResultFromEvents(ctx, []*index.CollectedEvent{vt}, e.chainStore, e.stateManager)
				if err != nil {
					continue
				}

				for _, r := range evs.Results {
					e.send(ctx, r)
				}
			case *types.TipSet:
				// Skip processing for tipset at epoch 0 as it has no parent
				if vt.Height() == 0 {
					continue
				}
				// Check if the parent has already been processed
				parentTipSetKey := vt.Parents()
				if e.lastSentTipset != nil && (*e.lastSentTipset) == parentTipSetKey {
					continue
				}
				parentTipSet, loadErr := e.chainStore.LoadTipSet(ctx, parentTipSetKey)
				if loadErr != nil {
					log.Warnw("failed to load parent tipset", "tipset", parentTipSetKey, "error", loadErr)
					continue
				}
				ethBlock, ethBlockErr := newEthBlockFromFilecoinTipSet(ctx, parentTipSet, true, e.chainStore, e.stateManager)
				if ethBlockErr != nil {
					continue
				}

				e.send(ctx, ethBlock)
				e.lastSentTipset = &parentTipSetKey
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

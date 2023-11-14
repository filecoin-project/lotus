package gateway

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

type EthSubHandler struct {
	queued map[ethtypes.EthSubscriptionID][]ethtypes.EthSubscriptionResponse
	sinks  map[ethtypes.EthSubscriptionID]func(context.Context, *ethtypes.EthSubscriptionResponse) error

	lk sync.Mutex
}

func NewEthSubHandler() *EthSubHandler {
	return &EthSubHandler{
		queued: make(map[ethtypes.EthSubscriptionID][]ethtypes.EthSubscriptionResponse),
		sinks:  make(map[ethtypes.EthSubscriptionID]func(context.Context, *ethtypes.EthSubscriptionResponse) error),
	}
}

func (e *EthSubHandler) AddSub(ctx context.Context, id ethtypes.EthSubscriptionID, sink func(context.Context, *ethtypes.EthSubscriptionResponse) error) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	for _, p := range e.queued[id] {
		p := p // copy
		if err := sink(ctx, &p); err != nil {
			return err
		}
	}
	delete(e.queued, id)
	e.sinks[id] = sink
	return nil
}

func (e *EthSubHandler) RemoveSub(id ethtypes.EthSubscriptionID) {
	e.lk.Lock()
	defer e.lk.Unlock()

	delete(e.sinks, id)
	delete(e.queued, id)
}

func (e *EthSubHandler) EthSubscription(ctx context.Context, r jsonrpc.RawParams) error {
	p, err := jsonrpc.DecodeParams[ethtypes.EthSubscriptionResponse](r)
	if err != nil {
		return err
	}

	e.lk.Lock()

	sink := e.sinks[p.SubscriptionID]

	if sink == nil {
		e.queued[p.SubscriptionID] = append(e.queued[p.SubscriptionID], p)
		e.lk.Unlock()
		return nil
	}

	e.lk.Unlock()

	return sink(ctx, &p) // todo track errors and auto-unsubscribe on rpc conn close?
}

var _ api.EthSubscriber = (*EthSubHandler)(nil)

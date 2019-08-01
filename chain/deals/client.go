package deals

import (
	"context"
	"sync/atomic"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
)

var log = logging.Logger("deals")

type DealStatus int

const (
	DealResolvingMiner = DealStatus(iota)
)

type Deal struct {
	ID     uint64
	Status DealStatus
}

type Client struct {
	cs *store.ChainStore

	next  uint64
	deals map[uint64]Deal

	incoming chan Deal

	stop    chan struct{}
	stopped chan struct{}
}

func NewClient(cs *store.ChainStore) *Client {
	c := &Client{
		cs: cs,

		deals: map[uint64]Deal{},

		incoming: make(chan Deal, 16),

		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	return c
}

func (c *Client) Run() {
	go func() {
		defer close(c.stopped)

		for {
			select {
			case deal := <-c.incoming:
				log.Info("incoming deal")

				c.deals[deal.ID] = deal

			case <-c.stop:
				return
			}
		}
	}()
}

func (c *Client) Start(ctx context.Context, data cid.Cid, miner address.Address, blocksDuration uint64) (uint64, error) {
	// Getting PeerID
	// TODO: Is there a nicer way?

	ts := c.cs.GetHeaviestTipSet()
	state, err := c.cs.TipSetState(ts.Cids())
	if err != nil {
		return 0, err
	}

	vmi, err := vm.NewVM(state, ts.Height(), ts.Blocks()[0].Miner, c.cs)
	if err != nil {
		return 0, xerrors.Errorf("failed to set up vm: %w", err)
	}

	msg := &types.Message{
		To:     miner,
		Method: actors.MAMethods.GetPeerID,

		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(10000000000),
	}

	// TODO: maybe just use the invoker directly?
	r, err := vmi.ApplyMessage(ctx, msg)
	if err != nil {
		return 0, err
	}
	if r.ExitCode != 0 {
		panic("TODO: do we error here?")
	}
	pid, err := peer.IDFromBytes(r.Return)
	if err != nil {
		return 0, err
	}

	log.Warnf("miner pid:%s", pid)

	id := atomic.AddUint64(&c.next, 1)
	deal := Deal{
		ID:     id,
		Status: DealResolvingMiner,
	}

	c.incoming <- deal
	return id, nil
}

func (c *Client) Stop() {
	close(c.stop)
	<-c.stopped
}

package storage

import (
	"context"
	"errors"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/lib/statestore"
)

var log = logging.Logger("storageminer")

const PoStConfidence = 3

type Miner struct {
	api    storageMinerApi
	events *events.Events
	h      host.Host

	maddr  address.Address
	worker address.Address

	// PoSt
	postLk    sync.Mutex
	schedPost uint64

	// Sealing
	sb      *sectorbuilder.SectorBuilder
	sectors *statestore.StateStore
	tktFn   TicketFn

	sectorIncoming chan *SectorInfo
	sectorUpdated  chan sectorUpdate
	stop           chan struct{}
	stopped        chan struct{}
}

type storageMinerApi interface {
	// Call a read only method on actors (no interaction with the chain required)
	StateCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error)
	StateMinerWorker(context.Context, address.Address, *types.TipSet) (address.Address, error)
	StateMinerProvingPeriodEnd(context.Context, address.Address, *types.TipSet) (uint64, error)
	StateMinerSectors(context.Context, address.Address, *types.TipSet) ([]*api.ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, *types.TipSet) ([]*api.ChainSectorInfo, error)
	StateMinerSectorSize(context.Context, address.Address, *types.TipSet) (uint64, error)
	StateWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error) // TODO: removeme eventually
	StateGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, *types.TipSet) (*types.MessageReceipt, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetRandomness(context.Context, types.TipSetKey, []*types.Ticket, int) ([]byte, error)
	ChainGetTipSetByHeight(context.Context, uint64, *types.TipSet) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)

	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

func NewMiner(api storageMinerApi, addr address.Address, h host.Host, ds datastore.Batching, sb *sectorbuilder.SectorBuilder, tktFn TicketFn) (*Miner, error) {
	return &Miner{
		api: api,

		maddr: addr,
		h:     h,
		sb:    sb,
		tktFn: tktFn,

		sectors: statestore.New(namespace.Wrap(ds, datastore.NewKey("/sectors"))),

		sectorIncoming: make(chan *SectorInfo),
		sectorUpdated:  make(chan sectorUpdate),
		stop:           make(chan struct{}),
		stopped:        make(chan struct{}),
	}, nil
}

func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return xerrors.Errorf("miner preflight checks failed: %w", err)
	}

	m.events = events.NewEvents(ctx, m.api)

	go m.beginPosting(ctx)
	go m.sectorStateLoop(ctx)
	return nil
}

func (m *Miner) Stop(ctx context.Context) error {
	close(m.stop)
	select {
	case <-m.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Miner) runPreflightChecks(ctx context.Context) error {
	worker, err := m.api.StateMinerWorker(ctx, m.maddr, nil)
	if err != nil {
		return err
	}

	m.worker = worker

	has, err := m.api.WalletHas(ctx, worker)
	if err != nil {
		return xerrors.Errorf("failed to check wallet for worker key: %w", err)
	}

	if !has {
		return errors.New("key for worker not found in local wallet")
	}

	log.Infof("starting up miner %s, worker addr %s", m.maddr, m.worker)
	return nil
}

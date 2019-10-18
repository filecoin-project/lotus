package storage

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/pkg/errors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/events"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/storage/commitment"
	"github.com/filecoin-project/go-lotus/storage/sector"
)

var log = logging.Logger("storageminer")

const PoStConfidence = 3

type Miner struct {
	api    storageMinerApi
	events *events.Events

	secst *sector.Store
	commt *commitment.Tracker

	maddr address.Address

	worker address.Address

	h host.Host

	ds datastore.Batching

	schedLk   sync.Mutex
	schedPost uint64
}

type storageMinerApi interface {
	// I think I want this... but this is tricky
	//ReadState(ctx context.Context, addr address.Address) (????, error)

	// Call a read only method on actors (no interaction with the chain required)
	StateCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error)
	StateMinerWorker(context.Context, address.Address, *types.TipSet) (address.Address, error)
	StateMinerProvingPeriodEnd(context.Context, address.Address, *types.TipSet) (uint64, error)
	StateMinerProvingSet(context.Context, address.Address, *types.TipSet) ([]*api.SectorInfo, error)
	StateWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetRandomness(context.Context, *types.TipSet, []*types.Ticket, int) ([]byte, error)
	ChainGetTipSetByHeight(context.Context, uint64, *types.TipSet) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

func NewMiner(api storageMinerApi, addr address.Address, h host.Host, ds datastore.Batching, secst *sector.Store, commt *commitment.Tracker) (*Miner, error) {
	return &Miner{
		api: api,

		maddr: addr,
		h:     h,
		ds:    ds,
		secst: secst,
		commt: commt,
	}, nil
}

func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return errors.Wrap(err, "miner preflight checks failed")
	}

	m.events = events.NewEvents(ctx, m.api)

	go m.handlePostingSealedSectors(ctx)
	go m.beginPosting(ctx)
	return nil
}

func (m *Miner) handlePostingSealedSectors(ctx context.Context) {
	incoming := m.secst.Incoming()
	defer m.secst.CloseIncoming(incoming)

	for {
		select {
		case sinfo, ok := <-incoming:
			if !ok {
				// TODO: set some state variable so that this state can be
				// visible via some status command
				log.Warn("sealed sector channel closed, aborting process")
				return
			}

			if err := m.commitSector(ctx, sinfo); err != nil {
				log.Errorf("failed to commit sector: %s", err)
				continue
			}

		case <-ctx.Done():
			log.Warn("exiting seal posting routine")
			return
		}
	}
}

func (m *Miner) commitSector(ctx context.Context, sinfo sectorbuilder.SectorSealingStatus) error {
	log.Info("committing sector")

	ok, err := sectorbuilder.VerifySeal(build.SectorSize, sinfo.CommR[:], sinfo.CommD[:], sinfo.CommRStar[:], m.maddr, sinfo.SectorID, sinfo.Proof)
	if err != nil {
		log.Error("failed to verify seal we just created: ", err)
	}
	if !ok {
		log.Error("seal we just created failed verification")
	}

	params := &actors.CommitSectorParams{
		SectorID:  sinfo.SectorID,
		CommD:     sinfo.CommD[:],
		CommR:     sinfo.CommR[:],
		CommRStar: sinfo.CommRStar[:],
		Proof:     sinfo.Proof,
	}
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return errors.Wrap(aerr, "could not serialize commit sector parameters")
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.worker,
		Method:   actors.MAMethods.CommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return errors.Wrap(err, "pushing message to mpool")
	}

	if err := m.commt.TrackCommitSectorMsg(m.maddr, sinfo.SectorID, smsg.Cid()); err != nil {
		return errors.Wrap(err, "tracking sector commitment")
	}

	go func() {
		_, err := m.api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return
		}

		m.beginPosting(ctx)
	}()

	return nil
}

func (m *Miner) runPreflightChecks(ctx context.Context) error {
	worker, err := m.api.StateMinerWorker(ctx, m.maddr, nil)
	if err != nil {
		return err
	}

	m.worker = worker

	has, err := m.api.WalletHas(ctx, worker)
	if err != nil {
		return errors.Wrap(err, "failed to check wallet for worker key")
	}

	if !has {
		return errors.New("key for worker not found in local wallet")
	}

	log.Infof("starting up miner %s, worker addr %s", m.maddr, m.worker)
	return nil
}

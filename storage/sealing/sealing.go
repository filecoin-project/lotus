package sealing

import (
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/padreader"
	"github.com/filecoin-project/lotus/lib/statemachine"
)

const SectorStorePrefix = "/sectors"

var log = logging.Logger("sectors")

type TicketFn func(context.Context) (*sectorbuilder.SealTicket, error)

type sealingApi interface { // TODO: trim down
	// Call a read only method on actors (no interaction with the chain required)
	StateCall(context.Context, *types.Message, types.TipSetKey) (*api.InvocResult, error)
	StateMinerWorker(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMinerElectionPeriodStart(ctx context.Context, actor address.Address, tsk types.TipSetKey) (uint64, error)
	StateMinerSectors(context.Context, address.Address, types.TipSetKey) ([]*api.ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, types.TipSetKey) ([]*api.ChainSectorInfo, error)
	StateMinerSectorSize(context.Context, address.Address, types.TipSetKey) (uint64, error)
	StateWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error) // TODO: removeme eventually
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)
	StateMarketStorageDeal(context.Context, uint64, types.TipSetKey) (*actors.OnChainDeal, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetRandomness(context.Context, types.TipSetKey, int64) ([]byte, error)
	ChainGetTipSetByHeight(context.Context, uint64, types.TipSetKey) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)

	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

type Sealing struct {
	api    sealingApi
	events *events.Events

	maddr  address.Address
	worker address.Address

	sb      sectorbuilder.Interface
	sectors *statemachine.StateGroup
	tktFn   TicketFn
}

func New(api sealingApi, events *events.Events, maddr address.Address, worker address.Address, ds datastore.Batching, sb sectorbuilder.Interface, tktFn TicketFn) *Sealing {
	s := &Sealing{
		api:    api,
		events: events,

		maddr:  maddr,
		worker: worker,
		sb:     sb,
		tktFn:  tktFn,
	}

	s.sectors = statemachine.New(namespace.Wrap(ds, datastore.NewKey(SectorStorePrefix)), s, SectorInfo{})

	return s
}

func (m *Sealing) Run(ctx context.Context) error {
	if err := m.restartSectors(ctx); err != nil {
		log.Errorf("%+v", err)
		return xerrors.Errorf("failed load sector states: %w", err)
	}

	return nil
}

func (m *Sealing) Stop(ctx context.Context) error {
	return m.sectors.Stop(ctx)
}

func (m *Sealing) AllocatePiece(size uint64) (sectorID uint64, offset uint64, err error) {
	if padreader.PaddedSize(size) != size {
		return 0, 0, xerrors.Errorf("cannot allocate unpadded piece")
	}

	sid, err := m.sb.AcquireSectorId() // TODO: Put more than one thing in a sector
	if err != nil {
		return 0, 0, xerrors.Errorf("acquiring sector ID: %w", err)
	}

	// offset hard-coded to 0 since we only put one thing in a sector for now
	return sid, 0, nil
}

func (m *Sealing) SealPiece(ctx context.Context, size uint64, r io.Reader, sectorID uint64, dealID uint64) error {
	log.Infof("Seal piece for deal %d", dealID)

	ppi, err := m.sb.AddPiece(ctx, size, sectorID, r, []uint64{})
	if err != nil {
		return xerrors.Errorf("adding piece to sector: %w", err)
	}

	return m.newSector(ctx, sectorID, dealID, ppi)
}

func (m *Sealing) newSector(ctx context.Context, sid uint64, dealID uint64, ppi sectorbuilder.PublicPieceInfo) error {
	log.Infof("Start sealing %d", sid)
	return m.sectors.Send(sid, SectorStart{
		id: sid,
		pieces: []Piece{
			{
				DealID: dealID,

				Size:  ppi.Size,
				CommP: ppi.CommP[:],
			},
		},
	})
}

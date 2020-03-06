package sealing

import (
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/statemachine"
	"github.com/filecoin-project/lotus/storage/sealmgr"
)

const SectorStorePrefix = "/sectors"

var log = logging.Logger("sectors")

type TicketFn func(context.Context) (*api.SealTicket, error)

type sealingApi interface { // TODO: trim down
	// Call a read only method on actors (no interaction with the chain required)
	StateCall(context.Context, *types.Message, types.TipSetKey) (*api.MethodCall, error)
	StateMinerWorker(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMinerPostState(ctx context.Context, actor address.Address, ts types.TipSetKey) (*miner.PoStState, error)
	StateMinerSectors(context.Context, address.Address, types.TipSetKey) ([]*api.ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, types.TipSetKey) ([]*api.ChainSectorInfo, error)
	StateMinerSectorSize(context.Context, address.Address, types.TipSetKey) (abi.SectorSize, error)
	StateWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error) // TODO: removeme eventually
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetRandomness(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)

	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

type Sealing struct {
	api    sealingApi
	events *events.Events

	maddr  address.Address
	worker address.Address

	sealer  sealmgr.Manager
	sectors *statemachine.StateGroup
	tktFn   TicketFn
}

func New(api sealingApi, events *events.Events, maddr address.Address, worker address.Address, ds datastore.Batching, sealer sealmgr.Manager, tktFn TicketFn) *Sealing {
	s := &Sealing{
		api:    api,
		events: events,

		maddr:  maddr,
		worker: worker,
		sealer: sealer,
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

func (m *Sealing) AllocatePiece(size abi.UnpaddedPieceSize) (sectorID abi.SectorNumber, offset uint64, err error) {
	if (padreader.PaddedSize(uint64(size))) != size {
		return 0, 0, xerrors.Errorf("cannot allocate unpadded piece")
	}

	sid, err := m.sealer.NewSector() // TODO: Put more than one thing in a sector
	if err != nil {
		return 0, 0, xerrors.Errorf("acquiring sector ID: %w", err)
	}

	// offset hard-coded to 0 since we only put one thing in a sector for now
	return sid, 0, nil
}

func (m *Sealing) SealPiece(ctx context.Context, size abi.UnpaddedPieceSize, r io.Reader, sectorID abi.SectorNumber, dealID abi.DealID) error {
	log.Infof("Seal piece for deal %d", dealID)

	ppi, err := m.sealer.AddPiece(ctx, sectorID, []abi.UnpaddedPieceSize{}, size, r)
	if err != nil {
		return xerrors.Errorf("adding piece to sector: %w", err)
	}

	_, rt, err := api.ProofTypeFromSectorSize(m.sealer.SectorSize())
	if err != nil {
		return xerrors.Errorf("bad sector size: %w", err)
	}

	return m.newSector(sectorID, rt, []Piece{
		{
			DealID: &dealID,

			Size:  ppi.Size.Unpadded(),
			CommP: ppi.PieceCID,
		},
	})
}

func (m *Sealing) newSector(sid abi.SectorNumber, rt abi.RegisteredProof, pieces []Piece) error {
	log.Infof("Start sealing %d", sid)
	return m.sectors.Send(uint64(sid), SectorStart{
		id:         sid,
		pieces:     pieces,
		sectorType: rt,
	})
}

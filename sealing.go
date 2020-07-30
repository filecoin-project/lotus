package sealing

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	padreader "github.com/filecoin-project/go-padreader"
	statemachine "github.com/filecoin-project/go-statemachine"
	sectorstorage "github.com/filecoin-project/sector-storage"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
)

const SectorStorePrefix = "/sectors"

var log = logging.Logger("sectors")

type SectorLocation struct {
	Deadline  uint64
	Partition uint64
}

type SealingAPI interface {
	StateWaitMsg(context.Context, cid.Cid) (MsgLookup, error)
	StateComputeDataCommitment(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tok TipSetToken) (cid.Cid, error)
	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok TipSetToken) (*miner.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok TipSetToken) (*miner.SectorOnChainInfo, error)
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok TipSetToken) (*SectorLocation, error)
	StateMinerSectorSize(context.Context, address.Address, TipSetToken) (abi.SectorSize, error)
	StateMinerWorkerAddress(ctx context.Context, maddr address.Address, tok TipSetToken) (address.Address, error)
	StateMinerDeadlines(ctx context.Context, maddr address.Address, tok TipSetToken) ([]*miner.Deadline, error)
	StateMinerPreCommitDepositForPower(context.Context, address.Address, miner.SectorPreCommitInfo, TipSetToken) (big.Int, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, TipSetToken) (big.Int, error)
	StateMarketStorageDeal(context.Context, abi.DealID, TipSetToken) (market.DealProposal, error)
	SendMsg(ctx context.Context, from, to address.Address, method abi.MethodNum, value, gasPrice big.Int, gasLimit int64, params []byte) (cid.Cid, error)
	ChainHead(ctx context.Context) (TipSetToken, abi.ChainEpoch, error)
	ChainGetRandomness(ctx context.Context, tok TipSetToken, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
}

type Sealing struct {
	api    SealingAPI
	events Events

	maddr address.Address

	sealer  sectorstorage.SectorManager
	sectors *statemachine.StateGroup
	sc      SectorIDCounter
	verif   ffiwrapper.Verifier

	pcp             PreCommitPolicy
	unsealedInfoMap UnsealedSectorMap

	upgradeLk sync.Mutex
	toUpgrade map[abi.SectorNumber]struct{}

	getSealDelay GetSealingDelayFunc
}

type UnsealedSectorMap struct {
	infos map[abi.SectorNumber]UnsealedSectorInfo
	mux   sync.Mutex
}

type UnsealedSectorInfo struct {
	numDeals uint64
	// stored should always equal sum of pieceSizes.Padded()
	stored     abi.PaddedPieceSize
	pieceSizes []abi.UnpaddedPieceSize
}

func New(api SealingAPI, events Events, maddr address.Address, ds datastore.Batching, sealer sectorstorage.SectorManager, sc SectorIDCounter, verif ffiwrapper.Verifier, pcp PreCommitPolicy, gsd GetSealingDelayFunc) *Sealing {
	s := &Sealing{
		api:    api,
		events: events,

		maddr:  maddr,
		sealer: sealer,
		sc:     sc,
		verif:  verif,
		pcp:    pcp,
		unsealedInfoMap: UnsealedSectorMap{
			infos: make(map[abi.SectorNumber]UnsealedSectorInfo),
			mux:   sync.Mutex{},
		},

		toUpgrade:    map[abi.SectorNumber]struct{}{},
		getSealDelay: gsd,
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
func (m *Sealing) AddPieceToAnySector(ctx context.Context, size abi.UnpaddedPieceSize, r io.Reader, d DealInfo) (abi.SectorNumber, abi.PaddedPieceSize, error) {
	log.Infof("Adding piece for deal %d", d.DealID)
	if (padreader.PaddedSize(uint64(size))) != size {
		return 0, 0, xerrors.Errorf("cannot allocate unpadded piece")
	}

	if size > abi.PaddedPieceSize(m.sealer.SectorSize()).Unpadded() {
		return 0, 0, xerrors.Errorf("piece cannot fit into a sector")
	}

	m.unsealedInfoMap.mux.Lock()

	sid, pads, err := m.getSectorAndPadding(size)
	if err != nil {
		m.unsealedInfoMap.mux.Unlock()
		return 0, 0, xerrors.Errorf("getting available sector: %w", err)
	}

	for _, p := range pads {
		err = m.addPiece(ctx, sid, p.Unpadded(), m.pledgeReader(p.Unpadded()), nil)
		if err != nil {
			m.unsealedInfoMap.mux.Unlock()
			return 0, 0, xerrors.Errorf("writing pads: %w", err)
		}
	}

	offset := m.unsealedInfoMap.infos[sid].stored
	err = m.addPiece(ctx, sid, size, r, &d)

	if err != nil {
		m.unsealedInfoMap.mux.Unlock()
		return 0, 0, xerrors.Errorf("adding piece to sector: %w", err)
	}

	m.unsealedInfoMap.mux.Unlock()
	if m.unsealedInfoMap.infos[sid].numDeals == getDealPerSectorLimit(m.sealer.SectorSize()) {
		if err := m.StartPacking(sid); err != nil {
			return 0, 0, xerrors.Errorf("start packing: %w", err)
		}
	}

	return sid, offset, nil
}

// Caller should hold m.unsealedInfoMap.mux
func (m *Sealing) addPiece(ctx context.Context, sectorID abi.SectorNumber, size abi.UnpaddedPieceSize, r io.Reader, di *DealInfo) error {
	log.Infof("Adding piece to sector %d", sectorID)
	ppi, err := m.sealer.AddPiece(sectorstorage.WithPriority(ctx, DealSectorPriority), m.minerSector(sectorID), m.unsealedInfoMap.infos[sectorID].pieceSizes, size, r)
	if err != nil {
		return xerrors.Errorf("writing piece: %w", err)
	}
	piece := Piece{
		Piece:    ppi,
		DealInfo: di,
	}

	err = m.sectors.Send(uint64(sectorID), SectorAddPiece{NewPiece: piece})
	if err != nil {
		return err
	}

	ui := m.unsealedInfoMap.infos[sectorID]
	num := m.unsealedInfoMap.infos[sectorID].numDeals
	if di != nil {
		num = num + 1
	}
	m.unsealedInfoMap.infos[sectorID] = UnsealedSectorInfo{
		numDeals:   num,
		stored:     ui.stored + piece.Piece.Size,
		pieceSizes: append(ui.pieceSizes, piece.Piece.Size.Unpadded()),
	}

	return nil
}

func (m *Sealing) Remove(ctx context.Context, sid abi.SectorNumber) error {
	return m.sectors.Send(uint64(sid), SectorRemove{})
}

// Caller should NOT hold m.unsealedInfoMap.mux
func (m *Sealing) StartPacking(sectorID abi.SectorNumber) error {
	log.Infof("Starting packing sector %d", sectorID)
	err := m.sectors.Send(uint64(sectorID), SectorStartPacking{})
	if err != nil {
		return err
	}

	m.unsealedInfoMap.mux.Lock()
	delete(m.unsealedInfoMap.infos, sectorID)
	m.unsealedInfoMap.mux.Unlock()

	return nil
}

// Caller should hold m.unsealedInfoMap.mux
func (m *Sealing) getSectorAndPadding(size abi.UnpaddedPieceSize) (abi.SectorNumber, []abi.PaddedPieceSize, error) {
	ss := abi.PaddedPieceSize(m.sealer.SectorSize())
	for k, v := range m.unsealedInfoMap.infos {
		pads, padLength := ffiwrapper.GetRequiredPadding(v.stored, size.Padded())
		if v.stored+size.Padded()+padLength <= ss {
			return k, pads, nil
		}
	}

	ns, err := m.newSector()
	if err != nil {
		return 0, nil, err
	}

	m.unsealedInfoMap.infos[ns] = UnsealedSectorInfo{
		numDeals:   0,
		stored:     0,
		pieceSizes: nil,
	}

	return ns, nil, nil
}

// newSector creates a new sector for deal storage
func (m *Sealing) newSector() (abi.SectorNumber, error) {
	sid, err := m.sc.Next()
	if err != nil {
		return 0, xerrors.Errorf("getting sector number: %w", err)
	}

	err = m.sealer.NewSector(context.TODO(), m.minerSector(sid))
	if err != nil {
		return 0, xerrors.Errorf("initializing sector: %w", err)
	}

	rt, err := ffiwrapper.SealProofTypeFromSectorSize(m.sealer.SectorSize())
	if err != nil {
		return 0, xerrors.Errorf("bad sector size: %w", err)
	}

	log.Infof("Creating sector %d", sid)
	err = m.sectors.Send(uint64(sid), SectorStart{
		ID:         sid,
		SectorType: rt,
	})

	if err != nil {
		return 0, xerrors.Errorf("starting the sector fsm: %w", err)
	}

	sd, err := m.getSealDelay()
	if err != nil {
		return 0, xerrors.Errorf("getting the sealing delay: %w", err)
	}

	if sd > 0 {
		timer := time.NewTimer(sd)
		go func() {
			<-timer.C
			m.StartPacking(sid)
		}()
	}

	return sid, nil
}

// newSectorCC accepts a slice of pieces with no deal (junk data)
func (m *Sealing) newSectorCC(sid abi.SectorNumber, pieces []Piece) error {
	rt, err := ffiwrapper.SealProofTypeFromSectorSize(m.sealer.SectorSize())
	if err != nil {
		return xerrors.Errorf("bad sector size: %w", err)
	}

	log.Infof("Creating CC sector %d", sid)
	return m.sectors.Send(uint64(sid), SectorStartCC{
		ID:         sid,
		Pieces:     pieces,
		SectorType: rt,
	})
}

func (m *Sealing) minerSector(num abi.SectorNumber) abi.SectorID {
	mid, err := address.IDFromAddress(m.maddr)
	if err != nil {
		panic(err)
	}

	return abi.SectorID{
		Number: num,
		Miner:  abi.ActorID(mid),
	}
}

func (m *Sealing) Address() address.Address {
	return m.maddr
}

func getDealPerSectorLimit(size abi.SectorSize) uint64 {
	if size < 64<<30 {
		return 256
	} else {
		return 512
	}
}

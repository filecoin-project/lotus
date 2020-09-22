package sealing

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	padreader "github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	statemachine "github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
)

const SectorStorePrefix = "/sectors"

var ErrTooManySectorsSealing = xerrors.New("too many sectors sealing")

var log = logging.Logger("sectors")

type SectorLocation struct {
	Deadline  uint64
	Partition uint64
}

var ErrSectorAllocated = errors.New("sectorNumber is allocated, but PreCommit info wasn't found on chain")

type SealingAPI interface {
	StateWaitMsg(context.Context, cid.Cid) (MsgLookup, error)
	StateSearchMsg(context.Context, cid.Cid) (*MsgLookup, error)
	StateComputeDataCommitment(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tok TipSetToken) (cid.Cid, error)

	// Can return ErrSectorAllocated in case precommit info wasn't found, but the sector number is marked as allocated
	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok TipSetToken) (*miner.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok TipSetToken) (*miner.SectorOnChainInfo, error)
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok TipSetToken) (*SectorLocation, error)
	StateMinerSectorSize(context.Context, address.Address, TipSetToken) (abi.SectorSize, error)
	StateMinerWorkerAddress(ctx context.Context, maddr address.Address, tok TipSetToken) (address.Address, error)
	StateMinerPreCommitDepositForPower(context.Context, address.Address, miner.SectorPreCommitInfo, TipSetToken) (big.Int, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, TipSetToken) (big.Int, error)
	StateMarketStorageDeal(context.Context, abi.DealID, TipSetToken) (market.DealProposal, error)
	StateNetworkVersion(ctx context.Context, tok TipSetToken) (network.Version, error)
	SendMsg(ctx context.Context, from, to address.Address, method abi.MethodNum, value, maxFee abi.TokenAmount, params []byte) (cid.Cid, error)
	ChainHead(ctx context.Context) (TipSetToken, abi.ChainEpoch, error)
	ChainGetRandomnessFromBeacon(ctx context.Context, tok TipSetToken, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainGetRandomnessFromTickets(ctx context.Context, tok TipSetToken, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
}

type SectorStateNotifee func(before, after SectorInfo)

type Sealing struct {
	api    SealingAPI
	feeCfg FeeConfig
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

	notifee SectorStateNotifee

	stats SectorStats

	getConfig GetSealingConfigFunc
}

type FeeConfig struct {
	MaxPreCommitGasFee abi.TokenAmount
	MaxCommitGasFee    abi.TokenAmount
}

type UnsealedSectorMap struct {
	infos map[abi.SectorNumber]UnsealedSectorInfo
	lk    sync.Mutex
}

type UnsealedSectorInfo struct {
	numDeals uint64
	// stored should always equal sum of pieceSizes.Padded()
	stored     abi.PaddedPieceSize
	pieceSizes []abi.UnpaddedPieceSize
}

func New(api SealingAPI, fc FeeConfig, events Events, maddr address.Address, ds datastore.Batching, sealer sectorstorage.SectorManager, sc SectorIDCounter, verif ffiwrapper.Verifier, pcp PreCommitPolicy, gc GetSealingConfigFunc, notifee SectorStateNotifee) *Sealing {
	s := &Sealing{
		api:    api,
		feeCfg: fc,
		events: events,

		maddr:  maddr,
		sealer: sealer,
		sc:     sc,
		verif:  verif,
		pcp:    pcp,
		unsealedInfoMap: UnsealedSectorMap{
			infos: make(map[abi.SectorNumber]UnsealedSectorInfo),
			lk:    sync.Mutex{},
		},

		toUpgrade: map[abi.SectorNumber]struct{}{},

		notifee: notifee,

		getConfig: gc,

		stats: SectorStats{
			bySector: map[abi.SectorID]statSectorState{},
		},
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
	log.Infof("Adding piece for deal %d (publish msg: %s)", d.DealID, d.PublishCid)
	if (padreader.PaddedSize(uint64(size))) != size {
		return 0, 0, xerrors.Errorf("cannot allocate unpadded piece")
	}

	if size > abi.PaddedPieceSize(m.sealer.SectorSize()).Unpadded() {
		return 0, 0, xerrors.Errorf("piece cannot fit into a sector")
	}

	m.unsealedInfoMap.lk.Lock()

	sid, pads, err := m.getSectorAndPadding(size)
	if err != nil {
		m.unsealedInfoMap.lk.Unlock()
		return 0, 0, xerrors.Errorf("getting available sector: %w", err)
	}

	for _, p := range pads {
		err = m.addPiece(ctx, sid, p.Unpadded(), NewNullReader(p.Unpadded()), nil)
		if err != nil {
			m.unsealedInfoMap.lk.Unlock()
			return 0, 0, xerrors.Errorf("writing pads: %w", err)
		}
	}

	offset := m.unsealedInfoMap.infos[sid].stored
	err = m.addPiece(ctx, sid, size, r, &d)

	if err != nil {
		m.unsealedInfoMap.lk.Unlock()
		return 0, 0, xerrors.Errorf("adding piece to sector: %w", err)
	}

	startPacking := m.unsealedInfoMap.infos[sid].numDeals >= getDealPerSectorLimit(m.sealer.SectorSize())

	m.unsealedInfoMap.lk.Unlock()

	if startPacking {
		if err := m.StartPacking(sid); err != nil {
			return 0, 0, xerrors.Errorf("start packing: %w", err)
		}
	}

	return sid, offset, nil
}

// Caller should hold m.unsealedInfoMap.lk
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

// Caller should NOT hold m.unsealedInfoMap.lk
func (m *Sealing) StartPacking(sectorID abi.SectorNumber) error {
	// locking here ensures that when the SectorStartPacking event is sent, the sector won't be picked up anywhere else
	m.unsealedInfoMap.lk.Lock()
	defer m.unsealedInfoMap.lk.Unlock()

	// cannot send SectorStartPacking to sectors that have already been packed, otherwise it will cause the state machine to exit
	if _, ok := m.unsealedInfoMap.infos[sectorID]; !ok {
		log.Warnf("call start packing, but sector %v not in unsealedInfoMap.infos, maybe have called", sectorID)
		return nil
	}
	log.Infof("Starting packing sector %d", sectorID)
	err := m.sectors.Send(uint64(sectorID), SectorStartPacking{})
	if err != nil {
		return err
	}
	log.Infof("send Starting packing event success sector %d", sectorID)

	delete(m.unsealedInfoMap.infos, sectorID)

	return nil
}

// Caller should hold m.unsealedInfoMap.lk
func (m *Sealing) getSectorAndPadding(size abi.UnpaddedPieceSize) (abi.SectorNumber, []abi.PaddedPieceSize, error) {
	ss := abi.PaddedPieceSize(m.sealer.SectorSize())
	for k, v := range m.unsealedInfoMap.infos {
		pads, padLength := ffiwrapper.GetRequiredPadding(v.stored, size.Padded())
		if v.stored+size.Padded()+padLength <= ss {
			return k, pads, nil
		}
	}

	ns, err := m.newDealSector()
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

// newDealSector creates a new sector for deal storage
func (m *Sealing) newDealSector() (abi.SectorNumber, error) {
	// First make sure we don't have too many 'open' sectors

	cfg, err := m.getConfig()
	if err != nil {
		return 0, xerrors.Errorf("getting config: %w", err)
	}

	if cfg.MaxSealingSectorsForDeals > 0 {
		if m.stats.curSealing() > cfg.MaxSealingSectorsForDeals {
			return 0, ErrTooManySectorsSealing
		}
	}

	if cfg.MaxWaitDealsSectors > 0 {
		// run in a loop because we have to drop the map lock here for a bit
		tries := 0

		// we have to run in a loop as we're dropping unsealedInfoMap.lk
		//  to actually call StartPacking. When we do that, another entry can
		//  get added to unsealedInfoMap.
		for uint64(len(m.unsealedInfoMap.infos)) >= cfg.MaxWaitDealsSectors {
			if tries > 10 {
				// whatever...
				break
			}

			if tries > 0 {
				m.unsealedInfoMap.lk.Unlock()
				time.Sleep(time.Second)
				m.unsealedInfoMap.lk.Lock()
			}

			tries++
			var mostStored abi.PaddedPieceSize = math.MaxUint64
			var best abi.SectorNumber = math.MaxUint64

			for sn, info := range m.unsealedInfoMap.infos {
				if info.stored+1 > mostStored+1 { // 18446744073709551615 + 1 = 0
					best = sn
				}
			}

			if best == math.MaxUint64 {
				// probably not possible, but who knows
				break
			}

			m.unsealedInfoMap.lk.Unlock()
			if err := m.StartPacking(best); err != nil {
				log.Error("newDealSector StartPacking error: %+v", err)
				continue // let's pretend this is fine
			}
			m.unsealedInfoMap.lk.Lock()
		}
	}

	// Now actually create a new sector

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

	cf, err := m.getConfig()
	if err != nil {
		return 0, xerrors.Errorf("getting the sealing delay: %w", err)
	}

	if cf.WaitDealsDelay > 0 {
		timer := time.NewTimer(cf.WaitDealsDelay)
		go func() {
			<-timer.C
			if err := m.StartPacking(sid); err != nil {
				log.Errorf("starting sector %d: %+v", sid, err)
			}
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
	}
	return 512
}

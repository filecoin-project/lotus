package sealing

import (
	"context"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/go-storedcounter"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/lib/result"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const SectorStorePrefix = "/sectors"

var ErrTooManySectorsSealing = xerrors.New("too many sectors sealing")

var log = logging.Logger("sectors")

type SealingAPI interface {
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)

	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorLocation, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMinerPreCommitDepositForPower(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (big.Int, error)
	StateMinerInitialPledgeForSector(ctx context.Context, sectorDuration abi.ChainEpoch, sectorSize abi.SectorSize, verifiedSize uint64, tsk types.TipSetKey) (types.BigInt, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (big.Int, error)
	StateMinerSectorAllocated(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (bool, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateMinerDeadlines(context.Context, address.Address, types.TipSetKey) ([]api.Deadline, error)
	StateMinerPartitions(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error)
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	StateMinerAllocated(context.Context, address.Address, types.TipSetKey) (*bitfield.BitField, error)
	StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocationIdForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (verifreg.AllocationId, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)

	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateVMCirculatingSupplyInternal(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error)
	ChainHasObj(ctx context.Context, c cid.Cid) (bool, error)
	ChainPutObj(ctx context.Context, block blocks.Block) error

	// Address selector
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

type SectorStateNotifee func(before, after SectorInfo)

type Events interface {
	ChainAt(ctx context.Context, hnd events.HeightHandler, rev events.RevertHandler, confidence int, h abi.ChainEpoch) error
}

type AddressSelector interface {
	AddressFor(ctx context.Context, a ctladdr.NodeApi, mi api.MinerInfo, use api.AddrUse, goodFunds, minFunds abi.TokenAmount) (address.Address, abi.TokenAmount, error)
}

type Sealing struct {
	Api      SealingAPI
	DealInfo *CurrentDealInfoManager

	ds datastore.Batching

	feeCfg config.MinerFeeConfig
	events Events

	startupWait sync.WaitGroup

	maddr address.Address

	sealer  sealer.SectorManager
	sectors *statemachine.StateGroup
	verif   storiface.Verifier
	pcp     PreCommitPolicy

	inputLk        sync.Mutex
	openSectors    map[abi.SectorID]*openSector
	sectorTimers   map[abi.SectorID]*time.Timer
	pendingPieces  map[piece.PieceKey]*pendingPiece
	assignedPieces map[abi.SectorID][]piece.PieceKey
	nextDealSector *abi.SectorNumber // used to prevent a race where we could create a new sector more than once

	available map[abi.SectorID]struct{}

	journal        journal.Journal
	sealingEvtType journal.EventType
	notifee        SectorStateNotifee
	addrSel        AddressSelector

	stats SectorStats

	terminator  *TerminateBatcher
	precommiter *PreCommitBatcher
	commiter    *CommitBatcher

	sclk     sync.Mutex
	legacySc *storedcounter.StoredCounter

	getConfig dtypes.GetSealingConfigFunc
}

type openSector struct {
	used        abi.UnpaddedPieceSize // change to bitfield/rle when AddPiece gains offset support to better fill sectors
	lastDealEnd abi.ChainEpoch
	number      abi.SectorNumber
	ccUpdate    bool

	maybeAccept func(key piece.PieceKey) error // called with inputLk
}

func (o *openSector) checkDealAssignable(piece *pendingPiece, expF expFn) (bool, error) {
	log := log.With(
		"sector", o.number,

		"piece", piece.deal.String(),
		"dealEnd", result.Wrap(piece.deal.EndEpoch()),
		"dealStart", result.Wrap(piece.deal.StartEpoch()),
		"dealClaimEnd", piece.claimTerms.claimTermEnd,

		"lastAssignedDealEnd", o.lastDealEnd,
		"update", o.ccUpdate,
	)

	// if there are deals assigned, check that no assigned deal expires after termMax
	if o.lastDealEnd > piece.claimTerms.claimTermEnd {
		log.Debugw("deal not assignable to sector", "reason", "term end beyond last assigned deal end")
		return false, nil
	}

	// check that in case of upgrade sectors, sector expiration is at least deal expiration
	if !o.ccUpdate {
		return true, nil
	}
	sectorExpiration, _, err := expF(o.number)
	if err != nil {
		log.Debugw("deal not assignable to sector", "reason", "error getting sector expiranion", "error", err)
		return false, err
	}

	log = log.With(
		"sectorExpiration", sectorExpiration,
	)

	// check that in case of upgrade sector, it's expiration isn't above deals claim TermMax
	if sectorExpiration > piece.claimTerms.claimTermEnd {
		log.Debugw("deal not assignable to sector", "reason", "term end beyond sector expiration")
		return false, nil
	}

	endEpoch, err := piece.deal.EndEpoch()
	if err != nil {
		return false, xerrors.Errorf("failed to get end epoch: %w", err)
	}

	if sectorExpiration < endEpoch {
		log.Debugw("deal not assignable to sector", "reason", "sector expiration less than deal expiration")
		return false, nil
	}

	return true, nil
}

type pieceAcceptResp struct {
	sn     abi.SectorNumber
	offset abi.UnpaddedPieceSize
	err    error
}

type pieceClaimBounds struct {
	// dealStart + termMax
	claimTermEnd abi.ChainEpoch
}

type pendingPiece struct {
	doneCh chan struct{}
	resp   *pieceAcceptResp

	size abi.UnpaddedPieceSize
	deal UniversalPieceInfo

	claimTerms pieceClaimBounds

	data storiface.Data

	assigned bool // assigned to a sector?
	accepted func(abi.SectorNumber, abi.UnpaddedPieceSize, error)
}

func New(mctx context.Context, sapi SealingAPI, fc config.MinerFeeConfig, events Events, maddr address.Address, ds datastore.Batching, sealer sealer.SectorManager, verif storiface.Verifier, prov storiface.Prover, pcp PreCommitPolicy, gc dtypes.GetSealingConfigFunc, journal journal.Journal, addrSel AddressSelector) (*Sealing, error) {
	s := &Sealing{
		Api:      sapi,
		DealInfo: &CurrentDealInfoManager{sapi},

		ds: ds,

		feeCfg: fc,
		events: events,

		maddr:  maddr,
		sealer: sealer,
		verif:  verif,
		pcp:    pcp,

		openSectors:    map[abi.SectorID]*openSector{},
		sectorTimers:   map[abi.SectorID]*time.Timer{},
		pendingPieces:  map[piece.PieceKey]*pendingPiece{},
		assignedPieces: map[abi.SectorID][]piece.PieceKey{},

		available: map[abi.SectorID]struct{}{},

		journal:        journal,
		sealingEvtType: journal.RegisterEventType("storage", "sealing_states"),

		addrSel: addrSel,

		terminator: NewTerminationBatcher(mctx, maddr, sapi, addrSel, fc, gc),
		getConfig:  gc,

		legacySc: storedcounter.New(ds, datastore.NewKey(StorageCounterDSPrefix)),

		stats: SectorStats{
			bySector: map[abi.SectorID]SectorState{},
			byState:  map[SectorState]int64{},
		},
	}
	pc, err := NewPreCommitBatcher(mctx, maddr, sapi, addrSel, fc, gc)
	if err != nil {
		return nil, err
	}
	s.precommiter = pc

	cc, err := NewCommitBatcher(mctx, maddr, sapi, addrSel, fc, gc, prov)
	if err != nil {
		return nil, err
	}
	s.commiter = cc

	s.notifee = func(before, after SectorInfo) {
		s.journal.RecordEvent(s.sealingEvtType, func() interface{} {
			return SealingStateEvt{
				SectorNumber: before.SectorNumber,
				SectorType:   before.SectorType,
				From:         before.State,
				After:        after.State,
				Error:        after.LastErr,
			}
		})
	}

	s.startupWait.Add(1)

	s.sectors = statemachine.New(namespace.Wrap(ds, datastore.NewKey(SectorStorePrefix)), s, SectorInfo{})

	return s, nil
}

func (m *Sealing) Run(ctx context.Context) {
	if err := m.restartSectors(ctx); err != nil {
		log.Errorf("failed load sector states: %+v", err)
	}
}

func (m *Sealing) Stop(ctx context.Context) error {
	if err := m.terminator.Stop(ctx); err != nil {
		return err
	}

	if err := m.sectors.Stop(ctx); err != nil {
		return err
	}
	return nil
}

func (m *Sealing) RemoveSector(ctx context.Context, sid abi.SectorNumber) error {
	m.startupWait.Wait()

	return m.sectors.Send(uint64(sid), SectorRemove{})
}

func (m *Sealing) TerminateSector(ctx context.Context, sid abi.SectorNumber) error {
	m.startupWait.Wait()

	return m.sectors.Send(uint64(sid), SectorTerminate{})
}

func (m *Sealing) SectorsSummary(ctx context.Context) map[api.SectorState]int {
	m.stats.lk.Lock()
	defer m.stats.lk.Unlock()

	out := make(map[api.SectorState]int)

	for st, count := range m.stats.byState {
		state := api.SectorState(st)
		out[state] = int(count)
	}

	return out
}

func (m *Sealing) TerminateFlush(ctx context.Context) (*cid.Cid, error) {
	return m.terminator.Flush(ctx)
}

func (m *Sealing) TerminatePending(ctx context.Context) ([]abi.SectorID, error) {
	return m.terminator.Pending(ctx)
}

func (m *Sealing) SectorPreCommitFlush(ctx context.Context) ([]sealiface.PreCommitBatchRes, error) {
	return m.precommiter.Flush(ctx)
}

func (m *Sealing) SectorPreCommitPending(ctx context.Context) ([]abi.SectorID, error) {
	return m.precommiter.Pending(ctx)
}

func (m *Sealing) CommitFlush(ctx context.Context) ([]sealiface.CommitBatchRes, error) {
	return m.commiter.Flush(ctx)
}

func (m *Sealing) CommitPending(ctx context.Context) ([]abi.SectorID, error) {
	return m.commiter.Pending(ctx)
}

func (m *Sealing) currentSealProof(ctx context.Context) (abi.RegisteredSealProof, error) {
	mi, err := m.Api.StateMinerInfo(ctx, m.maddr, types.EmptyTSK)
	if err != nil {
		return 0, err
	}

	ver, err := m.Api.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return 0, err
	}

	c, err := m.getConfig()
	if err != nil {
		return 0, err
	}

	return miner.PreferredSealProofTypeFromWindowPoStType(ver, mi.WindowPoStProofType, c.UseSyntheticPoRep)
}

func (m *Sealing) minerSector(spt abi.RegisteredSealProof, num abi.SectorNumber) storiface.SectorRef {
	return storiface.SectorRef{
		ID:        m.minerSectorID(num),
		ProofType: spt,
	}
}

func (m *Sealing) minerSectorID(num abi.SectorNumber) abi.SectorID {
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

func getDealPerSectorLimit(size abi.SectorSize) (int, error) {
	if size < 64<<30 {
		return 256, nil
	}
	return 512, nil
}

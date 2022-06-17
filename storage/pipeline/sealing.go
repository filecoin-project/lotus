package sealing

import (
	"context"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/api"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const SectorStorePrefix = "/sectors"

var ErrTooManySectorsSealing = xerrors.New("too many sectors sealing")

var log = logging.Logger("sectors")

//go:generate go run github.com/golang/mock/mockgen -destination=mocks/api.go -package=mocks . SealingAPI

type SealingAPI interface {
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)

	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
	StateComputeDataCID(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tsk types.TipSetKey) (cid.Cid, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*lminer.SectorLocation, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMinerPreCommitDepositForPower(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (big.Int, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (big.Int, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (big.Int, error)
	StateMinerSectorAllocated(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (bool, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateMinerDeadlines(context.Context, address.Address, types.TipSetKey) ([]api.Deadline, error)
	StateMinerPartitions(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error)
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
}

type SectorStateNotifee func(before, after SectorInfo)

type AddrSel func(ctx context.Context, mi api.MinerInfo, use api.AddrUse, goodFunds, minFunds abi.TokenAmount) (address.Address, abi.TokenAmount, error)

type Events interface {
	ChainAt(ctx context.Context, hnd events.HeightHandler, rev events.RevertHandler, confidence int, h abi.ChainEpoch) error
}

type Sealing struct {
	Api      SealingAPI
	DealInfo *CurrentDealInfoManager

	feeCfg config.MinerFeeConfig
	events Events

	startupWait sync.WaitGroup

	maddr address.Address

	sealer  sealer.SectorManager
	sectors *statemachine.StateGroup
	sc      SectorIDCounter
	verif   storiface.Verifier
	pcp     PreCommitPolicy

	inputLk        sync.Mutex
	openSectors    map[abi.SectorID]*openSector
	sectorTimers   map[abi.SectorID]*time.Timer
	pendingPieces  map[cid.Cid]*pendingPiece
	assignedPieces map[abi.SectorID][]cid.Cid
	nextDealSector *abi.SectorNumber // used to prevent a race where we could create a new sector more than once

	available map[abi.SectorID]struct{}

	notifee SectorStateNotifee
	addrSel AddrSel

	stats SectorStats

	terminator  *TerminateBatcher
	precommiter *PreCommitBatcher
	commiter    *CommitBatcher

	getConfig GetSealingConfigFunc
}

type openSector struct {
	used     abi.UnpaddedPieceSize // change to bitfield/rle when AddPiece gains offset support to better fill sectors
	number   abi.SectorNumber
	ccUpdate bool

	maybeAccept func(cid.Cid) error // called with inputLk
}

func (o *openSector) dealFitsInLifetime(dealEnd abi.ChainEpoch, expF expFn) (bool, error) {
	if !o.ccUpdate {
		return true, nil
	}
	expiration, _, err := expF(o.number)
	if err != nil {
		return false, err
	}
	return expiration >= dealEnd, nil
}

type pieceAcceptResp struct {
	sn     abi.SectorNumber
	offset abi.UnpaddedPieceSize
	err    error
}

type pendingPiece struct {
	doneCh chan struct{}
	resp   *pieceAcceptResp

	size abi.UnpaddedPieceSize
	deal api.PieceDealInfo

	data storiface.Data

	assigned bool // assigned to a sector?
	accepted func(abi.SectorNumber, abi.UnpaddedPieceSize, error)
}

func New(mctx context.Context, api SealingAPI, fc config.MinerFeeConfig, events Events, maddr address.Address, ds datastore.Batching, sealer sealer.SectorManager, sc SectorIDCounter, verif storiface.Verifier, prov storiface.Prover, pcp PreCommitPolicy, gc GetSealingConfigFunc, notifee SectorStateNotifee, as AddrSel) *Sealing {
	s := &Sealing{
		Api:      api,
		DealInfo: &CurrentDealInfoManager{api},

		feeCfg: fc,
		events: events,

		maddr:  maddr,
		sealer: sealer,
		sc:     sc,
		verif:  verif,
		pcp:    pcp,

		openSectors:    map[abi.SectorID]*openSector{},
		sectorTimers:   map[abi.SectorID]*time.Timer{},
		pendingPieces:  map[cid.Cid]*pendingPiece{},
		assignedPieces: map[abi.SectorID][]cid.Cid{},

		available: map[abi.SectorID]struct{}{},

		notifee: notifee,
		addrSel: as,

		terminator:  NewTerminationBatcher(mctx, maddr, api, as, fc, gc),
		precommiter: NewPreCommitBatcher(mctx, maddr, api, as, fc, gc),
		commiter:    NewCommitBatcher(mctx, maddr, api, as, fc, gc, prov),

		getConfig: gc,

		stats: SectorStats{
			bySector: map[abi.SectorID]SectorState{},
			byState:  map[SectorState]int64{},
		},
	}
	s.startupWait.Add(1)

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
	if err := m.terminator.Stop(ctx); err != nil {
		return err
	}

	if err := m.sectors.Stop(ctx); err != nil {
		return err
	}
	return nil
}

func (m *Sealing) Remove(ctx context.Context, sid abi.SectorNumber) error {
	m.startupWait.Wait()

	return m.sectors.Send(uint64(sid), SectorRemove{})
}

func (m *Sealing) Terminate(ctx context.Context, sid abi.SectorNumber) error {
	m.startupWait.Wait()

	return m.sectors.Send(uint64(sid), SectorTerminate{})
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

	return lminer.PreferredSealProofTypeFromWindowPoStType(ver, mi.WindowPoStProofType)
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

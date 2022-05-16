package storage

import (
	"context"
	"errors"
	"time"

	"github.com/filecoin-project/go-bitfield"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/specs-storage/storage"

	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("storageminer")

// Miner is the central miner entrypoint object inside Lotus. It is
// instantiated in the node builder, along with the WindowPoStScheduler.
//
// This object is the owner of the sealing pipeline. Most of the actual logic
// lives in the storage-sealing module (sealing.Sealing), and the Miner object
// exposes it to the rest of the system by proxying calls.
//
// Miner#Run starts the sealing FSM.
type Miner struct {
	api     fullNodeFilteredAPI
	feeCfg  config.MinerFeeConfig
	sealer  sectorstorage.SectorManager
	ds      datastore.Batching
	sc      sealing.SectorIDCounter
	verif   ffiwrapper.Verifier
	prover  ffiwrapper.Prover
	addrSel *AddressSelector

	maddr address.Address

	getSealConfig dtypes.GetSealingConfigFunc
	sealing       *sealing.Sealing

	sealingEvtType journal.EventType

	journal journal.Journal
}

// SealingStateEvt is a journal event that records a sector state transition.
type SealingStateEvt struct {
	SectorNumber abi.SectorNumber
	SectorType   abi.RegisteredSealProof
	From         sealing.SectorState
	After        sealing.SectorState
	Error        string
}

// fullNodeFilteredAPI is the subset of the full node API the Miner needs from
// a Lotus full node.
type fullNodeFilteredAPI interface {
	// Call a read only method on actors (no interaction with the chain required)
	StateCall(context.Context, *types.Message, types.TipSetKey) (*api.InvocResult, error)
	StateMinerSectors(context.Context, address.Address, *bitfield.BitField, types.TipSetKey) ([]*miner.SectorOnChainInfo, error)
	StateSectorPreCommitInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok types.TipSetKey) (*miner.SectorLocation, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (miner.MinerInfo, error)
	StateMinerAvailableBalance(ctx context.Context, maddr address.Address, tok types.TipSetKey) (types.BigInt, error)
	StateMinerActiveSectors(context.Context, address.Address, types.TipSetKey) ([]*miner.SectorOnChainInfo, error)
	StateMinerDeadlines(context.Context, address.Address, types.TipSetKey) ([]api.Deadline, error)
	StateMinerPartitions(context.Context, address.Address, uint64, types.TipSetKey) ([]api.Partition, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateMinerPreCommitDepositForPower(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (types.BigInt, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (types.BigInt, error)
	StateMinerSectorAllocated(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (bool, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)
	StateMinerRecoveries(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (network.Version, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)

	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)

	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
	GasEstimateFeeCap(context.Context, *types.Message, int64, types.TipSetKey) (types.BigInt, error)
	GasEstimateGasPremium(_ context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error)

	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

// NewMiner creates a new Miner object.
func NewMiner(api fullNodeFilteredAPI,
	maddr address.Address,
	ds datastore.Batching,
	sealer sectorstorage.SectorManager,
	sc sealing.SectorIDCounter,
	verif ffiwrapper.Verifier,
	prover ffiwrapper.Prover,
	gsd dtypes.GetSealingConfigFunc,
	feeCfg config.MinerFeeConfig,
	journal journal.Journal,
	as *AddressSelector) (*Miner, error) {
	m := &Miner{
		api:     api,
		feeCfg:  feeCfg,
		sealer:  sealer,
		ds:      ds,
		sc:      sc,
		verif:   verif,
		prover:  prover,
		addrSel: as,

		maddr:          maddr,
		getSealConfig:  gsd,
		journal:        journal,
		sealingEvtType: journal.RegisterEventType("storage", "sealing_states"),
	}

	return m, nil
}

// Run starts the sealing FSM in the background, running preliminary checks first.
func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return xerrors.Errorf("miner preflight checks failed: %w", err)
	}

	md, err := m.api.StateMinerProvingDeadline(ctx, m.maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting miner info: %w", err)
	}

	// consumer of chain head changes.
	evts, err := events.NewEvents(ctx, m.api)
	if err != nil {
		return xerrors.Errorf("failed to subscribe to events: %w", err)
	}
	evtsAdapter := NewEventsAdapter(evts)

	// Create a shim to glue the API required by the sealing component
	// with the API that Lotus is capable of providing.
	// The shim translates between "tipset tokens" and tipset keys, and
	// provides extra methods.
	adaptedAPI := NewSealingAPIAdapter(m.api)

	// Instantiate a precommit policy.
	cfg := sealing.GetSealingConfigFunc(m.getSealConfig)
	provingBuffer := md.WPoStProvingPeriod * 2

	pcp := sealing.NewBasicPreCommitPolicy(adaptedAPI, cfg, provingBuffer)

	// address selector.
	as := func(ctx context.Context, mi miner.MinerInfo, use api.AddrUse, goodFunds, minFunds abi.TokenAmount) (address.Address, abi.TokenAmount, error) {
		return m.addrSel.AddressFor(ctx, m.api, mi, use, goodFunds, minFunds)
	}

	// Instantiate the sealing FSM.
	m.sealing = sealing.New(ctx, adaptedAPI, m.feeCfg, evtsAdapter, m.maddr, m.ds, m.sealer, m.sc, m.verif, m.prover, &pcp, cfg, m.handleSealingNotifications, as)

	// Run the sealing FSM.
	go m.sealing.Run(ctx) //nolint:errcheck // logged intside the function

	return nil
}

func (m *Miner) handleSealingNotifications(before, after sealing.SectorInfo) {
	m.journal.RecordEvent(m.sealingEvtType, func() interface{} {
		return SealingStateEvt{
			SectorNumber: before.SectorNumber,
			SectorType:   before.SectorType,
			From:         before.State,
			After:        after.State,
			Error:        after.LastErr,
		}
	})
}

func (m *Miner) Stop(ctx context.Context) error {
	return m.sealing.Stop(ctx)
}

// runPreflightChecks verifies that preconditions to run the miner are satisfied.
func (m *Miner) runPreflightChecks(ctx context.Context) error {
	mi, err := m.api.StateMinerInfo(ctx, m.maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("failed to resolve miner info: %w", err)
	}

	workerKey, err := m.api.StateAccountKey(ctx, mi.Worker, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("failed to resolve worker key: %w", err)
	}

	has, err := m.api.WalletHas(ctx, workerKey)
	if err != nil {
		return xerrors.Errorf("failed to check wallet for worker key: %w", err)
	}

	if !has {
		return errors.New("key for worker not found in local wallet")
	}

	log.Infof("starting up miner %s, worker addr %s", m.maddr, workerKey)
	return nil
}

type StorageWpp struct {
	prover   storage.Prover
	verifier ffiwrapper.Verifier
	miner    abi.ActorID
	winnRpt  abi.RegisteredPoStProof
}

func NewWinningPoStProver(api v1api.FullNode, prover storage.Prover, verifier ffiwrapper.Verifier, miner dtypes.MinerID) (*StorageWpp, error) {
	ma, err := address.NewIDAddress(uint64(miner))
	if err != nil {
		return nil, err
	}

	mi, err := api.StateMinerInfo(context.TODO(), ma, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting sector size: %w", err)
	}

	if build.InsecurePoStValidation {
		log.Warn("*****************************************************************************")
		log.Warn(" Generating fake PoSt proof! You should only see this while running tests! ")
		log.Warn("*****************************************************************************")
	}

	return &StorageWpp{prover, verifier, abi.ActorID(miner), mi.WindowPoStProofType}, nil
}

var _ gen.WinningPoStProver = (*StorageWpp)(nil)

func (wpp *StorageWpp) GenerateCandidates(ctx context.Context, randomness abi.PoStRandomness, eligibleSectorCount uint64) ([]uint64, error) {
	start := build.Clock.Now()

	cds, err := wpp.verifier.GenerateWinningPoStSectorChallenge(ctx, wpp.winnRpt, wpp.miner, randomness, eligibleSectorCount)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate candidates: %w", err)
	}
	log.Infof("Generate candidates took %s (C: %+v)", time.Since(start), cds)
	return cds, nil
}

func (wpp *StorageWpp) ComputeProof(ctx context.Context, ssi []builtin.ExtendedSectorInfo, rand abi.PoStRandomness, currEpoch abi.ChainEpoch, nv network.Version) ([]builtin.PoStProof, error) {
	if build.InsecurePoStValidation {
		return []builtin.PoStProof{{ProofBytes: []byte("valid proof")}}, nil
	}

	log.Infof("Computing WinningPoSt ;%+v; %v", ssi, rand)

	start := build.Clock.Now()
	proof, err := wpp.prover.GenerateWinningPoSt(ctx, wpp.miner, ssi, rand)
	if err != nil {
		return nil, err
	}
	log.Infof("GenerateWinningPoSt took %s", time.Since(start))
	return proof, nil
}

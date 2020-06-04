package storage

import (
	"context"
	"errors"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	sectorstorage "github.com/filecoin-project/sector-storage"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	sealing "github.com/filecoin-project/storage-fsm"
)

var log = logging.Logger("storageminer")

type Miner struct {
	api    storageMinerApi
	h      host.Host
	sealer sectorstorage.SectorManager
	ds     datastore.Batching
	sc     sealing.SectorIDCounter
	verif  ffiwrapper.Verifier

	maddr  address.Address
	worker address.Address

	sealing *sealing.Sealing
}

type storageMinerApi interface {
	// Call a read only method on actors (no interaction with the chain required)
	StateCall(context.Context, *types.Message, types.TipSetKey) (*api.InvocResult, error)
	StateMinerDeadlines(ctx context.Context, maddr address.Address, tok types.TipSetKey) (*miner.Deadlines, error)
	StateMinerSectors(context.Context, address.Address, *abi.BitField, bool, types.TipSetKey) ([]*api.ChainSectorInfo, error)
	StateSectorPreCommitInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (miner.MinerInfo, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*miner.DeadlineInfo, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (types.BigInt, error)
	StateWaitMsg(context.Context, cid.Cid) (*api.MsgLookup, error) // TODO: removeme eventually
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) (*abi.BitField, error)
	StateMinerRecoveries(context.Context, address.Address, types.TipSetKey) (*abi.BitField, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	ChainGetRandomness(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error)

	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

func NewMiner(api storageMinerApi, maddr, worker address.Address, h host.Host, ds datastore.Batching, sealer sectorstorage.SectorManager, sc sealing.SectorIDCounter, verif ffiwrapper.Verifier) (*Miner, error) {
	m := &Miner{
		api:    api,
		h:      h,
		sealer: sealer,
		ds:     ds,
		sc:     sc,
		verif:  verif,

		maddr:  maddr,
		worker: worker,
	}

	return m, nil
}

func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return xerrors.Errorf("miner preflight checks failed: %w", err)
	}

	md, err := m.api.StateMinerProvingDeadline(ctx, m.maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting miner info: %w", err)
	}

	evts := events.NewEvents(ctx, m.api)
	adaptedAPI := NewSealingAPIAdapter(m.api)
	pcp := sealing.NewBasicPreCommitPolicy(adaptedAPI, 10000000, md.PeriodStart%miner.WPoStProvingPeriod)
	m.sealing = sealing.New(adaptedAPI, NewEventsAdapter(evts), m.maddr, m.ds, m.sealer, m.sc, m.verif, &pcp)

	go m.sealing.Run(ctx)

	return nil
}

func (m *Miner) Stop(ctx context.Context) error {
	defer m.sealing.Stop(ctx)
	return nil
}

func (m *Miner) runPreflightChecks(ctx context.Context) error {
	has, err := m.api.WalletHas(ctx, m.worker)
	if err != nil {
		return xerrors.Errorf("failed to check wallet for worker key: %w", err)
	}

	if !has {
		return errors.New("key for worker not found in local wallet")
	}

	log.Infof("starting up miner %s, worker addr %s", m.maddr, m.worker)
	return nil
}

type StorageWpp struct {
	prover   storage.Prover
	verifier ffiwrapper.Verifier
	miner    abi.ActorID
	winnRpt  abi.RegisteredProof
}

func NewWinningPoStProver(api api.FullNode, prover storage.Prover, verifier ffiwrapper.Verifier, miner dtypes.MinerID) (*StorageWpp, error) {
	ma, err := address.NewIDAddress(uint64(miner))
	if err != nil {
		return nil, err
	}

	mi, err := api.StateMinerInfo(context.TODO(), ma, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting sector size: %w", err)
	}

	spt, err := ffiwrapper.SealProofTypeFromSectorSize(mi.SectorSize)
	if err != nil {
		return nil, err
	}

	wpt, err := spt.RegisteredWinningPoStProof()
	if err != nil {
		return nil, err
	}

	return &StorageWpp{prover, verifier, abi.ActorID(miner), wpt}, nil
}

var _ gen.WinningPoStProver = (*StorageWpp)(nil)

func (wpp *StorageWpp) GenerateCandidates(ctx context.Context, randomness abi.PoStRandomness, eligibleSectorCount uint64) ([]uint64, error) {
	start := time.Now()

	cds, err := wpp.verifier.GenerateWinningPoStSectorChallenge(ctx, wpp.winnRpt, wpp.miner, randomness, eligibleSectorCount)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate candidates: %w", err)
	}
	log.Infof("Generate candidates took %s (C: %+v)", time.Since(start), cds)
	return cds, nil
}

func (wpp *StorageWpp) ComputeProof(ctx context.Context, ssi []abi.SectorInfo, rand abi.PoStRandomness) ([]abi.PoStProof, error) {
	if build.InsecurePoStValidation {
		log.Warn("Generating fake EPost proof! You should only see this while running tests!")
		return []abi.PoStProof{{ProofBytes: []byte("valid proof")}}, nil
	}

	log.Infof("Computing WinningPoSt ;%+v; %v", ssi, rand)

	start := time.Now()
	proof, err := wpp.prover.GenerateWinningPoSt(ctx, wpp.miner, ssi, rand)
	if err != nil {
		return nil, err
	}
	log.Infof("GenerateWinningPoSt took %s", time.Since(start))
	return proof, nil
}

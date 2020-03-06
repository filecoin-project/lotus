package storage

import (
	"context"
	"errors"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sealing"
)

var log = logging.Logger("storageminer")

type Miner struct {
	api   storageMinerApi
	h     host.Host
	sb    sectorbuilder.Interface
	ds    datastore.Batching
	tktFn sealing.TicketFn

	maddr  address.Address
	worker address.Address

	sealing *sealing.Sealing
}

type storageMinerApi interface {
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
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) ([]uint64, error)

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

func NewMiner(api storageMinerApi, maddr, worker address.Address, h host.Host, ds datastore.Batching, sb sectorbuilder.Interface, tktFn sealing.TicketFn) (*Miner, error) {
	m := &Miner{
		api:   api,
		h:     h,
		sb:    sb,
		ds:    ds,
		tktFn: tktFn,

		maddr:  maddr,
		worker: worker,
	}

	return m, nil
}

func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return xerrors.Errorf("miner preflight checks failed: %w", err)
	}

	evts := events.NewEvents(ctx, m.api)
	m.sealing = sealing.New(m.api, evts, m.maddr, m.worker, m.ds, m.sb, m.tktFn)

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

type SectorBuilderEpp struct {
	sb sectorbuilder.Interface
}

func NewElectionPoStProver(sb sectorbuilder.Interface) *SectorBuilderEpp {
	return &SectorBuilderEpp{sb}
}

var _ gen.ElectionPoStProver = (*SectorBuilderEpp)(nil)

func (epp *SectorBuilderEpp) GenerateCandidates(ctx context.Context, ssi sectorbuilder.SortedPublicSectorInfo, rand []byte) ([]sectorbuilder.EPostCandidate, error) {
	start := time.Now()
	var faults []uint64 // TODO

	var randbuf [32]byte
	copy(randbuf[:], rand)
	cds, err := epp.sb.GenerateEPostCandidates(ssi, randbuf, faults)
	if err != nil {
		return nil, err
	}
	log.Infof("Generate candidates took %s", time.Since(start))
	return cds, nil
}

func (epp *SectorBuilderEpp) ComputeProof(ctx context.Context, ssi sectorbuilder.SortedPublicSectorInfo, rand []byte, winners []sectorbuilder.EPostCandidate) ([]byte, error) {
	if build.InsecurePoStValidation {
		log.Warn("Generating fake EPost proof! You should only see this while running tests!")
		return []byte("valid proof"), nil
	}
	start := time.Now()
	proof, err := epp.sb.ComputeElectionPoSt(ssi, rand, winners)
	if err != nil {
		return nil, err
	}
	log.Infof("ComputeElectionPost took %s", time.Since(start))
	return proof, nil
}

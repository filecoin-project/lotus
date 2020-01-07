package storage

import (
	"context"
	"errors"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("storageminer")

const SectorStorePrefix = "/sectors"

type Miner struct {
	api    storageMinerApi
	events *events.Events
	h      host.Host

	maddr  address.Address
	worker address.Address

	// Sealing
	sb      *sectorbuilder.SectorBuilder
	sectors *statestore.StateStore
	tktFn   TicketFn

	sectorIncoming chan *SectorInfo
	sectorUpdated  chan sectorUpdate
	stop           chan struct{}
	stopped        chan struct{}
}

type storageMinerApi interface {
	// Call a read only method on actors (no interaction with the chain required)
	StateCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error)
	StateMinerWorker(context.Context, address.Address, *types.TipSet) (address.Address, error)
	StateMinerElectionPeriodStart(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error)
	StateMinerSectors(context.Context, address.Address, *types.TipSet) ([]*api.ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, *types.TipSet) ([]*api.ChainSectorInfo, error)
	StateMinerSectorSize(context.Context, address.Address, *types.TipSet) (uint64, error)
	StateWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error) // TODO: removeme eventually
	StateGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, *types.TipSet) (*types.MessageReceipt, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetRandomness(context.Context, types.TipSetKey, int64) ([]byte, error)
	ChainGetTipSetByHeight(context.Context, uint64, *types.TipSet) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)

	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

func NewMiner(api storageMinerApi, addr address.Address, h host.Host, ds datastore.Batching, sb *sectorbuilder.SectorBuilder, tktFn TicketFn) (*Miner, error) {
	return &Miner{
		api: api,

		maddr: addr,
		h:     h,
		sb:    sb,
		tktFn: tktFn,

		sectors: statestore.New(namespace.Wrap(ds, datastore.NewKey(SectorStorePrefix))),

		sectorIncoming: make(chan *SectorInfo),
		sectorUpdated:  make(chan sectorUpdate),
		stop:           make(chan struct{}),
		stopped:        make(chan struct{}),
	}, nil
}

func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return xerrors.Errorf("miner preflight checks failed: %w", err)
	}

	m.events = events.NewEvents(ctx, m.api)

	fps := &fpostScheduler{
		api:    m.api,
		sb:     m.sb,
		actor:  m.maddr,
		worker: m.worker,
	}

	go fps.run(ctx)
	if err := m.sectorStateLoop(ctx); err != nil {
		log.Errorf("%+v", err)
		return xerrors.Errorf("failed to startup sector state loop: %w", err)
	}

	return nil
}

func (m *Miner) Stop(ctx context.Context) error {
	close(m.stop)
	select {
	case <-m.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Miner) runPreflightChecks(ctx context.Context) error {
	worker, err := m.api.StateMinerWorker(ctx, m.maddr, nil)
	if err != nil {
		return err
	}

	m.worker = worker

	has, err := m.api.WalletHas(ctx, worker)
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
	sb *sectorbuilder.SectorBuilder
}

func NewElectionPoStProver(sb *sectorbuilder.SectorBuilder) *SectorBuilderEpp {
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

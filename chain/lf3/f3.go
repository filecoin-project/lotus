package lf3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/autobatch"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/blssig"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

var _ F3Backend = (*F3)(nil)

type F3Backend interface {
	GetOrRenewParticipationTicket(_ context.Context, minerID uint64, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error)
	Participate(_ context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error)
	ListParticipants() []api.F3Participant
	GetManifest(ctx context.Context) (*manifest.Manifest, error)
	GetCert(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error)
	GetLatestCert(ctx context.Context) (*certs.FinalityCertificate, error)
	GetPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error)
	GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error)
	GetPowerTableByInstance(ctx context.Context, instance uint64) (gpbft.PowerEntries, error)
	IsRunning() bool
	Progress() gpbft.InstanceProgress
}

type F3 struct {
	inner *f3.F3
	ec    *ecWrapper

	signer gpbft.Signer
	leaser *leaser
}

type F3Params struct {
	fx.In

	PubSub        *pubsub.PubSub
	Host          host.Host
	ChainStore    *store.ChainStore
	Syncer        *chain.Syncer
	StateManager  *stmgr.StateManager
	MetaDatastore dtypes.MetadataDS
	F3Datastore   dtypes.F3DS
	Wallet        api.Wallet
	Config        *Config
	LockedRepo    repo.LockedRepo

	Net api.Net
}

var log = logging.Logger("f3")

var migrationKey = datastore.NewKey("/f3-migration/1")

func checkMigrationComplete(ctx context.Context, source datastore.Batching, target datastore.Batching) (bool, error) {
	valSource, err := source.Get(ctx, migrationKey)
	if errors.Is(err, datastore.ErrNotFound) {
		log.Debug("migration not complete, no migration flag in source datastore")
		return false, nil
	}
	if err != nil {
		return false, err
	}
	valTarget, err := target.Get(ctx, migrationKey)
	if errors.Is(err, datastore.ErrNotFound) {
		log.Debug("migration not complete, no migration flag in target datastore")
		return false, nil
	}
	if err != nil {
		return false, err
	}
	log.Debugw("migration flags", "source", string(valSource), "target", string(valTarget))

	// if the values are equal, the migration is complete
	return bytes.Equal(valSource, valTarget), nil
}

// migrateDatastore can be removed once at least one network upgrade passes
func migrateDatastore(ctx context.Context, source datastore.Batching, target datastore.Batching) error {
	if complete, err := checkMigrationComplete(ctx, source, target); err != nil {
		return xerrors.Errorf("checking if migration complete: %w", err)
	} else if complete {
		return nil
	}

	startDate := time.Now()
	migrationVal := startDate.Format(time.RFC3339Nano)

	if err := target.Put(ctx, migrationKey, []byte(migrationVal)); err != nil {
		return xerrors.Errorf("putting migration flag in target datastore: %w", err)
	}
	// make sure the migration flag is not set in the source datastore
	if err := source.Delete(ctx, migrationKey); err != nil && !errors.Is(err, datastore.ErrNotFound) {
		return xerrors.Errorf("deleting migration flag in source datastore: %w", err)
	}

	log.Infow("starting migration of f3 datastore", "tag", migrationVal)
	qr, err := source.Query(ctx, query.Query{})
	if err != nil {
		return xerrors.Errorf("starting source wildcard query: %w", err)
	}

	// batch size of 2000, at the time of writing, F3 datastore has 150,000 keys taking ~170MiB
	// meaning that a batch of 2000 keys would be ~2MiB of memory
	batch := autobatch.NewAutoBatching(target, 2000)
	var numMigrated int
	for ctx.Err() == nil {
		res, ok := qr.NextSync()
		if !ok {
			break
		}
		if err := batch.Put(ctx, datastore.NewKey(res.Key), res.Value); err != nil {
			_ = qr.Close()
			return xerrors.Errorf("putting key %s in target datastore: %w", res.Key, err)
		}
		numMigrated++
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := batch.Flush(ctx); err != nil {
		return xerrors.Errorf("flushing batch: %w", err)
	}
	if err := qr.Close(); err != nil {
		return xerrors.Errorf("closing query: %w", err)
	}

	// set migration flag in the source datastore to signify that the migration is complete
	if err := source.Put(ctx, migrationKey, []byte(migrationVal)); err != nil {
		return xerrors.Errorf("putting migration flag in source datastore: %w", err)
	}
	took := time.Since(startDate)
	log.Infow("completed migration of f3 datastore", "tag", migrationVal, "tookSeconds", took.Seconds(), "numMigrated", numMigrated)

	return nil
}

func New(mctx helpers.MetricsCtx, lc fx.Lifecycle, params F3Params) (*F3, error) {

	if params.Config.StaticManifest == nil {
		return nil, fmt.Errorf("configuration invalid, nil StaticManifest in the Config")
	}
	metaDs := namespace.Wrap(params.MetaDatastore, datastore.NewKey("/f3"))
	err := migrateDatastore(mctx, metaDs, params.F3Datastore)
	if err != nil {
		return nil, xerrors.Errorf("migrating datastore: %w", err)
	}
	ec := newEcWrapper(params.ChainStore, params.Syncer, params.StateManager)
	verif := blssig.VerifierWithKeyOnG1()

	f3FsPath := filepath.Join(params.LockedRepo.Path(), "f3")
	module, err := f3.New(mctx, *params.Config.StaticManifest, params.F3Datastore,
		params.Host, params.PubSub, verif, ec, f3FsPath)
	if err != nil {
		return nil, xerrors.Errorf("creating F3: %w", err)
	}
	nodeId, err := params.Net.ID(mctx)
	if err != nil {
		return nil, xerrors.Errorf("getting node ID: %w", err)
	}

	// maxLeasableInstances is the maximum number of leased F3 instances this node
	// would give out.
	const maxLeasableInstances = 5
	status := func() (manifest.Manifest, gpbft.InstanceProgress) {
		return module.Manifest(), module.Progress()
	}
	fff := &F3{
		inner:  module,
		ec:     ec,
		signer: &signer{params.Wallet},
		leaser: newParticipationLeaser(nodeId, status, maxLeasableInstances),
	}

	// Start F3
	lc.Append(fx.Hook{
		OnStart: fff.inner.Start,
		OnStop:  fff.inner.Stop,
	})

	// Start signing F3 messages.
	lCtx, cancel := context.WithCancel(mctx)
	doneCh := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(doneCh)
				fff.runSigningLoop(lCtx)
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			<-doneCh
			return nil
		},
	})

	return fff, nil
}

func (fff *F3) runSigningLoop(ctx context.Context) {
	participateOnce := func(ctx context.Context, mb *gpbft.MessageBuilder, minerID uint64) error {
		signatureBuilder, err := mb.PrepareSigningInputs(gpbft.ActorID(minerID))
		if errors.Is(err, gpbft.ErrNoPower) {
			// we don't have any power in F3, continue
			log.Debugf("no power to participate in F3: %+v", err)
			return nil
		}
		if err != nil {
			return xerrors.Errorf("preparing signing inputs: %+v", err)
		}
		// if worker keys were stored not in the node, the signatureBuilder can be send there
		// the sign can be called where the keys are stored and then
		// {signatureBuilder, payloadSig, vrfSig} can be sent back to lotus for broadcast
		payloadSig, vrfSig, err := signatureBuilder.Sign(ctx, fff.signer)
		if err != nil {
			return xerrors.Errorf("signing message: %+v", err)
		}
		log.Debugf("miner with id %d is sending message in F3", minerID)
		fff.inner.Broadcast(ctx, signatureBuilder, payloadSig, vrfSig)
		return nil
	}

	msgCh := fff.inner.MessagesToSign()

	var mb *gpbft.MessageBuilder
	alreadyParticipated := make(map[uint64]struct{})
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-fff.leaser.notifyParticipation:
			if mb == nil {
				continue
			}
		case mb = <-msgCh: // never closed
			clear(alreadyParticipated)
		}

		participants := fff.leaser.getParticipantsByInstance(mb.NetworkName, mb.Payload.Instance)
		for _, id := range participants {
			if _, ok := alreadyParticipated[id]; ok {
				continue
			} else if err := participateOnce(ctx, mb, id); err != nil {
				log.Errorf("while participating for miner f0%d: %+v", id, err)
			} else {
				alreadyParticipated[id] = struct{}{}
			}
		}
	}
}

func (fff *F3) GetOrRenewParticipationTicket(_ context.Context, minerID uint64, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error) {
	return fff.leaser.getOrRenewParticipationTicket(minerID, previous, instances)
}

func (fff *F3) Participate(_ context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	return fff.leaser.participate(ticket)
}

func (fff *F3) GetCert(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	return fff.inner.GetCert(ctx, instance)
}

func (fff *F3) GetLatestCert(ctx context.Context) (*certs.FinalityCertificate, error) {
	return fff.inner.GetLatestCert(ctx)
}

func (fff *F3) GetManifest(ctx context.Context) (*manifest.Manifest, error) {
	m := fff.inner.Manifest()
	if m.InitialPowerTable.Defined() {
		return &m, nil
	}
	cert0, err := fff.inner.GetCert(ctx, 0)
	if err != nil {
		return &m, nil // return manifest without power table
	}

	m.InitialPowerTable = cert0.ECChain.Base().PowerTable
	return &m, nil
}

func (fff *F3) GetPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.ec.getPowerTableLotusTSK(ctx, tsk)
}

func (fff *F3) GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.inner.GetPowerTable(ctx, tsk.Bytes())
}

// GetPowerTableByInstance returns the power table (committee) used to validate the specified instance.
func (fff *F3) GetPowerTableByInstance(ctx context.Context, instance uint64) (gpbft.PowerEntries, error) {
	return fff.inner.GetPowerTableByInstance(ctx, instance)
}

func (fff *F3) IsRunning() bool {
	return fff.inner.IsRunning()
}

func (fff *F3) Progress() gpbft.InstanceProgress {
	return fff.inner.Progress()
}

func (fff *F3) ListParticipants() []api.F3Participant {
	leases := fff.leaser.getValidLeases()
	participants := make([]api.F3Participant, len(leases))
	for i, lease := range leases {
		participants[i] = api.F3Participant{
			MinerID:      lease.MinerID,
			FromInstance: lease.FromInstance,
			ValidityTerm: lease.ValidityTerm,
		}
	}
	return participants
}

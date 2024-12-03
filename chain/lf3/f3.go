package lf3

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
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
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

type F3 struct {
	inner *f3.F3
	ec    *ecWrapper

	signer gpbft.Signer
	leaser *leaser
}

type F3Params struct {
	fx.In

	ManifestProvider manifest.ManifestProvider
	PubSub           *pubsub.PubSub
	Host             host.Host
	ChainStore       *store.ChainStore
	Syncer           *chain.Syncer
	StateManager     *stmgr.StateManager
	Datastore        dtypes.MetadataDS
	Wallet           api.Wallet
	Config           *Config
	LockedRepo       repo.LockedRepo

	Net api.Net
}

var log = logging.Logger("f3")

func New(mctx helpers.MetricsCtx, lc fx.Lifecycle, params F3Params) (*F3, error) {
	ds := namespace.Wrap(params.Datastore, datastore.NewKey("/f3"))
	ec := &ecWrapper{
		ChainStore:   params.ChainStore,
		StateManager: params.StateManager,
		Syncer:       params.Syncer,
	}
	verif := blssig.VerifierWithKeyOnG1()

	f3FsPath := filepath.Join(params.LockedRepo.Path(), "f3")
	module, err := f3.New(mctx, params.ManifestProvider, ds,
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
	status := func() (*manifest.Manifest, gpbft.Instant) {
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
			log.Debug("no power to participate in F3: %+v", err)
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

func (fff *F3) GetManifest(ctx context.Context) *manifest.Manifest {
	m := fff.inner.Manifest()
	if m.InitialPowerTable.Defined() {
		return m
	}
	cert0, err := fff.inner.GetCert(ctx, 0)
	if err != nil {
		return m
	}

	var mCopy = *m
	m = &mCopy
	m.InitialPowerTable = cert0.ECChain.Base().PowerTable
	return m
}

func (fff *F3) GetPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.ec.getPowerTableLotusTSK(ctx, tsk)
}

func (fff *F3) GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.inner.GetPowerTable(ctx, tsk.Bytes())
}

func (fff *F3) IsRunning() bool {
	return fff.inner.IsRunning()
}

func (fff *F3) Progress() gpbft.Instant {
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

// ResolveTipSetKeySelector resolves a TipSetSelector into a concrete TipSetKey.
//
// For TipSetKey selectors, returns the key directly.
// For epoch descriptors:
//   - "latest" returns the current heaviest tipset
//   - "finalized" returns the most recently finalized tipset, using either F3
//     or EC finality depending on whether F3 is enabled and running.
//
// Returns:
//   - TipSetKey: The resolved tipset key (this may be EmptyTSK if that was
//     the selector)
//   - error: On invalid selectors or resolution failures
func ResolveTipSetKeySelector(ctx context.Context, fff *F3, chain *store.ChainStore, tss types.TipSetSelector) (types.TipSetKey, error) {
	if tss.TipSetKey != nil {
		return *tss.TipSetKey, nil
	}
	if tss.EpochDescriptor == nil {
		return types.EmptyTSK, errors.New("invalid tipset selector")
	}
	switch *tss.EpochDescriptor {
	case types.EpochLatest:
		return chain.GetHeaviestTipSet().Key(), nil
	case types.EpochFinalized:
		if fff != nil { // F3 is enabled
			cert, err := fff.inner.GetLatestCert(ctx)
			if err == nil { // F3 is running
				tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
				if err != nil {
					return types.EmptyTSK, xerrors.Errorf("decoding tipset key reported by F3: %w", err)
				}
				return tsk, nil
			}
		}
		// either F3 isn't enabled, or isn't running
		head := chain.GetHeaviestTipSet()
		finalizedHeight := head.Height() - policy.ChainFinality
		ts, err := chain.GetTipsetByHeight(ctx, finalizedHeight, head, true)
		if err != nil {
			return types.EmptyTSK, xerrors.Errorf("getting tipset by height: %w", err)
		}
		return ts.Key(), nil
	}
	return types.EmptyTSK, fmt.Errorf("invalid epoch descriptor: %s", *tss.EpochDescriptor)
}

// ResolveTipSetSelector resolves a TipSetSelector into a concrete TipSet.
//
// For TipSetKey selectors, loads the tipset directly.
// For epoch descriptors:
//   - "latest" returns the current heaviest tipset
//   - "finalized" returns the most recently finalized tipset, using either F3
//     or EC finality depending on whether F3 is enabled and running.
//
// Returns:
//   - TipSet: The resolved tipset
//   - error: On invalid selectors or resolution failures
func ResolveTipSetSelector(ctx context.Context, fff *F3, chain *store.ChainStore, tss types.TipSetSelector) (*types.TipSet, error) {
	if tss.TipSetKey != nil {
		ts, err := chain.GetTipSetFromKey(ctx, *tss.TipSetKey)
		if err != nil {
			return nil, xerrors.Errorf("loading tipset %s: %w", *tss.TipSetKey, err)
		}
		return ts, nil
	}
	if tss.EpochDescriptor == nil {
		return nil, errors.New("invalid tipset selector")
	}

	return ResolveEpochDescriptorTipSet(ctx, fff, chain, *tss.EpochDescriptor)
}

// ResolveEpochSelector resolves an EpochSelector into a concrete ChainEpoch.
//
// For ChainEpoch selectors, returns the epoch directly.
// For epoch descriptors:
//   - "latest" returns the current epoch
//   - "finalized" returns the most recently finalized epoch, using either F3
//     or EC finality depending on whether F3 is enabled and running.
//
// Returns:
//   - ChainEpoch: The resolved epoch
//   - error: On invalid selectors or resolution failures
func ResolveEpochSelector(ctx context.Context, fff *F3, chain *store.ChainStore, es types.EpochSelector) (abi.ChainEpoch, error) {
	if es.ChainEpoch != nil {
		return *es.ChainEpoch, nil
	} else if es.EpochDescriptor == nil {
		return 0, errors.New("invalid epoch selector")
	}

	if ts, err := ResolveEpochDescriptorTipSet(ctx, fff, chain, *es.EpochDescriptor); err != nil {
		return 0, xerrors.Errorf("resolving epoch descriptor: %w", err)
	} else {
		return ts.Height(), nil
	}
}

// ResolveEpochDescriptorTipSet resolves an EpochDescriptor into a concrete TipSet.
//
// For "latest", returns the current heaviest tipset.
// For "finalized", returns the most recently finalized tipset, using either F3
// or EC finality depending on whether F3 is enabled and running.
//
// Returns:
//   - TipSet: The resolved tipset
//   - error: On invalid descriptors or resolution failures
func ResolveEpochDescriptorTipSet(ctx context.Context, fff *F3, chain *store.ChainStore, ed types.EpochDescriptor) (*types.TipSet, error) {
	switch ed {
	case types.EpochLatest:
		return chain.GetHeaviestTipSet(), nil

	case types.EpochFinalized:
		if fff != nil { // F3 is enabled
			cert, err := fff.inner.GetLatestCert(ctx)
			if err == nil { // F3 is running
				tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
				if err != nil {
					return nil, xerrors.Errorf("decoding tipset key reported by F3: %w", err)
				}
				if ts, err := chain.LoadTipSet(ctx, tsk); err != nil {
					return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
				} else {
					return ts, nil
				}
			}
		}

		// either F3 isn't enabled, or isn't running
		head := chain.GetHeaviestTipSet()
		finalizedHeight := head.Height() - policy.ChainFinality
		if ts, err := chain.GetTipsetByHeight(ctx, finalizedHeight, head, true); err != nil {
			return nil, xerrors.Errorf("getting tipset by height: %w", err)
		} else {
			return ts, nil
		}

	default:
		return nil, fmt.Errorf("invalid epoch descriptor: %s", ed)
	}
}

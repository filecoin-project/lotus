package modules

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/jpillora/backoff"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/metrics"
	lotusminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/paths"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

// F3LeaseTerm The number of instances the miner will attempt to lease from nodes.
const F3LeaseTerm = 5

type UuidWrapper struct {
	v1api.FullNode
}

func (a *UuidWrapper) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	if spec == nil {
		spec = new(api.MessageSendSpec)
	}
	spec.MsgUuid = uuid.New()
	return a.FullNode.MpoolPushMessage(ctx, msg, spec)
}

func MakeUuidWrapper(a v1api.RawFullNodeAPI) v1api.FullNode {
	return &UuidWrapper{a}
}

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(context.TODO(), datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func GetParams(prover bool) func(spt abi.RegisteredSealProof) error {
	return func(spt abi.RegisteredSealProof) error {
		ssize, err := spt.SectorSize()
		if err != nil {
			return err
		}

		// If built-in assets are disabled, we expect the user to have placed the right
		// parameters in the right location on the filesystem (/var/tmp/filecoin-proof-parameters).
		if build.DisableBuiltinAssets {
			return nil
		}

		var provingSize uint64
		if prover {
			provingSize = uint64(ssize)
		}

		// TODO: We should fetch the params for the actual proof type, not just based on the size.
		if err := paramfetch.GetParams(context.TODO(), build.ParametersJSON(), build.SrsJSON(), provingSize); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	}
}

func MinerAddress(ds dtypes.MetadataDS) (dtypes.MinerAddress, error) {
	ma, err := minerAddrFromDS(ds)
	return dtypes.MinerAddress(ma), err
}

func MinerID(ma dtypes.MinerAddress) (dtypes.MinerID, error) {
	id, err := address.IDFromAddress(address.Address(ma))
	return dtypes.MinerID(id), err
}

func StorageNetworkName(ctx helpers.MetricsCtx, a v1api.FullNode) (dtypes.NetworkName, error) {
	if !buildconstants.Devnet {
		return "testnetnet", nil
	}
	return a.StateNetworkName(ctx)
}

func SealProofType(maddr dtypes.MinerAddress, fnapi v1api.FullNode) (abi.RegisteredSealProof, error) {
	mi, err := fnapi.StateMinerInfo(context.TODO(), address.Address(maddr), types.EmptyTSK)
	if err != nil {
		return 0, err
	}
	networkVersion, err := fnapi.StateNetworkVersion(context.TODO(), types.EmptyTSK)
	if err != nil {
		return 0, err
	}

	// node seal proof type does not decide whether or not we use synthetic porep
	return miner.PreferredSealProofTypeFromWindowPoStType(networkVersion, mi.WindowPoStProofType, false)
}

func AddressSelector(addrConf *config.MinerAddressConfig) func() (*ctladdr.AddressSelector, error) {
	return func() (*ctladdr.AddressSelector, error) {
		as := &ctladdr.AddressSelector{}
		if addrConf == nil {
			return as, nil
		}

		as.DisableOwnerFallback = addrConf.DisableOwnerFallback
		as.DisableWorkerFallback = addrConf.DisableWorkerFallback

		for _, s := range addrConf.PreCommitControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing precommit control address: %w", err)
			}

			as.PreCommitControl = append(as.PreCommitControl, addr)
		}

		for _, s := range addrConf.CommitControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing commit control address: %w", err)
			}

			as.CommitControl = append(as.CommitControl, addr)
		}

		for _, s := range addrConf.TerminateControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing terminate control address: %w", err)
			}

			as.TerminateControl = append(as.TerminateControl, addr)
		}

		for _, s := range addrConf.DealPublishControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing deal publishing control address: %w", err)
			}

			as.DealPublishControl = append(as.DealPublishControl, addr)
		}

		return as, nil
	}
}

func PreflightChecks(mctx helpers.MetricsCtx, lc fx.Lifecycle, api v1api.FullNode, maddr dtypes.MinerAddress) error {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{OnStart: func(context.Context) error {
		mi, err := api.StateMinerInfo(ctx, address.Address(maddr), types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to resolve miner info: %w", err)
		}

		workerKey, err := api.StateAccountKey(ctx, mi.Worker, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to resolve worker key: %w", err)
		}

		has, err := api.WalletHas(ctx, workerKey)
		if err != nil {
			return xerrors.Errorf("failed to check wallet for worker key: %w", err)
		}

		if !has {
			return xerrors.New("key for worker not found in local wallet")
		}

		log.Infof("starting up miner %s, worker addr %s", address.Address(maddr), workerKey)
		return nil
	}})

	return nil
}

type SealingPipelineParams struct {
	fx.In

	Lifecycle          fx.Lifecycle
	MetricsCtx         helpers.MetricsCtx
	API                v1api.FullNode
	MetadataDS         dtypes.MetadataDS
	Sealer             sealer.SectorManager
	Verifier           storiface.Verifier
	Prover             storiface.Prover
	GetSealingConfigFn dtypes.GetSealingConfigFunc
	Journal            journal.Journal
	AddrSel            *ctladdr.AddressSelector
	Maddr              dtypes.MinerAddress
}

func SealingPipeline(fc config.MinerFeeConfig) func(params SealingPipelineParams) (*sealing.Sealing, error) {
	return func(params SealingPipelineParams) (*sealing.Sealing, error) {
		var (
			ds     = params.MetadataDS
			mctx   = params.MetricsCtx
			lc     = params.Lifecycle
			api    = params.API
			sealer = params.Sealer
			verif  = params.Verifier
			prover = params.Prover
			gsd    = params.GetSealingConfigFn
			j      = params.Journal
			as     = params.AddrSel
			maddr  = address.Address(params.Maddr)
		)

		ctx := helpers.LifecycleCtx(mctx, lc)

		evts, err := events.NewEvents(ctx, api)
		if err != nil {
			return nil, xerrors.Errorf("failed to subscribe to events: %w", err)
		}

		md, err := api.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("getting miner info: %w", err)
		}
		provingBuffer := md.WPoStProvingPeriod * 2
		pcp := sealing.NewBasicPreCommitPolicy(api, gsd, provingBuffer)

		pipeline, err := sealing.New(ctx, api, fc, evts, maddr, ds, sealer, verif, prover, &pcp, gsd, j, as)
		if err != nil {
			return nil, xerrors.Errorf("creating sealing pipeline: %w", err)
		}

		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go pipeline.Run(ctx)
				return nil
			},
			OnStop: pipeline.Stop,
		})

		return pipeline, nil
	}
}

func WindowPostScheduler(fc config.MinerFeeConfig, pc config.ProvingConfig) func(params SealingPipelineParams) (*wdpost.WindowPoStScheduler, error) {
	return func(params SealingPipelineParams) (*wdpost.WindowPoStScheduler, error) {
		var (
			mctx   = params.MetricsCtx
			lc     = params.Lifecycle
			api    = params.API
			sealer = params.Sealer
			verif  = params.Verifier
			j      = params.Journal
			as     = params.AddrSel
		)

		ctx := helpers.LifecycleCtx(mctx, lc)
		ctx = metrics.AddNetworkTag(ctx)
		fps, err := wdpost.NewWindowedPoStScheduler(api, fc, pc, as, sealer, verif, sealer, j, []dtypes.MinerAddress{params.Maddr})

		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go fps.Run(ctx)
				return nil
			},
		})

		return fps, nil
	}
}

func SetupBlockProducer(lc fx.Lifecycle, ds dtypes.MetadataDS, api v1api.FullNode, epp gen.WinningPoStProver, sf *slashfilter.SlashFilter, j journal.Journal) (*lotusminer.Miner, error) {
	minerAddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	m := lotusminer.NewMiner(api, epp, minerAddr, sf, j)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := m.Start(ctx); err != nil {
				return err
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return m.Stop(ctx)
		},
	})

	return m, nil
}

var WorkerCallsPrefix = datastore.NewKey("/worker/calls")
var ManagerWorkPrefix = datastore.NewKey("/stmgr/calls")

func LocalStorage(mctx helpers.MetricsCtx, lc fx.Lifecycle, ls paths.LocalStorage, si paths.SectorIndex, urls paths.URLs) (*paths.Local, error) {
	ctx := helpers.LifecycleCtx(mctx, lc)
	return paths.NewLocal(ctx, ls, si, urls)
}

func RemoteStorage(lstor *paths.Local, si paths.SectorIndex, sa sealer.StorageAuth, sc config.SealerConfig) *paths.Remote {
	return paths.NewRemote(lstor, si, http.Header(sa), sc.ParallelFetchLimit, &paths.DefaultPartialFileHandler{})
}

func SectorStorage(mctx helpers.MetricsCtx, lc fx.Lifecycle, lstor *paths.Local, stor paths.Store, ls paths.LocalStorage, si paths.SectorIndex, sc config.SealerConfig, pc config.ProvingConfig, ds dtypes.MetadataDS) (*sealer.Manager, error) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	wsts := statestore.New(namespace.Wrap(ds, WorkerCallsPrefix))
	smsts := statestore.New(namespace.Wrap(ds, ManagerWorkPrefix))

	sst, err := sealer.New(ctx, lstor, stor, ls, si, sc, pc, wsts, smsts)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: sst.Close,
	})

	return sst, nil
}

func F3Participation(mctx helpers.MetricsCtx, lc fx.Lifecycle, node v1api.FullNode, participant dtypes.MinerAddress) error {
	const (
		// maxCheckProgressAttempts defines the maximum number of failed attempts
		// before we abandon the current lease and restart the participation process.
		//
		// The default backoff takes 12 attempts to reach a maximum delay of 1 minute.
		// Allowing for 13 failures results in approximately 2 minutes of backoff since
		// the lease was granted. Given a lease validity of up to 5 instances, this means
		// we would give up on checking the lease during its mid-validity period;
		// typically when we would try to renew the participation ticket. Hence, the value
		// to 13.
		checkProgressMaxAttempts = 13
	)

	p := lf3.NewParticipant(
		mctx,
		node,
		participant,
		&backoff.Backoff{
			Min:    1 * time.Second,
			Max:    1 * time.Minute,
			Factor: 1.5,
		},
		checkProgressMaxAttempts,
		F3LeaseTerm,
	)

	lc.Append(fx.Hook{
		OnStart: p.Start,
		OnStop:  p.Stop,
	})
	return nil
}

func StorageAuth(ctx helpers.MetricsCtx, ca v0api.Common) (sealer.StorageAuth, error) {
	token, err := ca.AuthNew(ctx, []auth.Permission{"admin"})
	if err != nil {
		return nil, xerrors.Errorf("creating storage auth header: %w", err)
	}

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))
	return sealer.StorageAuth(headers), nil
}

func StorageAuthWithURL(apiInfo string) interface{} {
	if strings.HasPrefix(apiInfo, "harmony:") {
		return func(ctx helpers.MetricsCtx, ca MinerStorageService) (sealer.StorageAuth, error) {
			return StorageAuth(ctx, ca)
		}
	}

	return func(ctx helpers.MetricsCtx, ca v0api.Common) (sealer.StorageAuth, error) {
		s := strings.Split(apiInfo, ":")
		if len(s) != 2 {
			return nil, errors.New("unexpected format of `apiInfo`")
		}
		headers := http.Header{}
		headers.Add("Authorization", "Bearer "+s[0])
		return sealer.StorageAuth(headers), nil
	}
}

func NewSetSealConfigFunc(r repo.LockedRepo) (dtypes.SetSealingConfigFunc, error) {
	return func(cfg sealiface.Config) (err error) {
		err = mutateSealingCfg(r, func(c config.SealingConfiger) {
			newCfg := config.SealingConfig{
				MaxWaitDealsSectors:             cfg.MaxWaitDealsSectors,
				MaxSealingSectors:               cfg.MaxSealingSectors,
				MaxSealingSectorsForDeals:       cfg.MaxSealingSectorsForDeals,
				PreferNewSectorsForDeals:        cfg.PreferNewSectorsForDeals,
				MaxUpgradingSectors:             cfg.MaxUpgradingSectors,
				CommittedCapacitySectorLifetime: config.Duration(cfg.CommittedCapacitySectorLifetime),
				WaitDealsDelay:                  config.Duration(cfg.WaitDealsDelay),
				MakeNewSectorForDeals:           cfg.MakeNewSectorForDeals,
				MinUpgradeSectorExpiration:      cfg.MinUpgradeSectorExpiration,
				MakeCCSectorsAvailable:          cfg.MakeCCSectorsAvailable,
				AlwaysKeepUnsealedCopy:          cfg.AlwaysKeepUnsealedCopy,
				FinalizeEarly:                   cfg.FinalizeEarly,

				CollateralFromMinerBalance: cfg.CollateralFromMinerBalance,
				AvailableBalanceBuffer:     types.FIL(cfg.AvailableBalanceBuffer),
				DisableCollateralFallback:  cfg.DisableCollateralFallback,

				MaxPreCommitBatch:   cfg.MaxPreCommitBatch,
				PreCommitBatchWait:  config.Duration(cfg.PreCommitBatchWait),
				PreCommitBatchSlack: config.Duration(cfg.PreCommitBatchSlack),

				AggregateCommits: cfg.AggregateCommits,
				MinCommitBatch:   cfg.MinCommitBatch,
				MaxCommitBatch:   cfg.MaxCommitBatch,
				CommitBatchWait:  config.Duration(cfg.CommitBatchWait),
				CommitBatchSlack: config.Duration(cfg.CommitBatchSlack),

				TerminateBatchMax:                      cfg.TerminateBatchMax,
				TerminateBatchMin:                      cfg.TerminateBatchMin,
				TerminateBatchWait:                     config.Duration(cfg.TerminateBatchWait),
				MaxSectorProveCommitsSubmittedPerEpoch: cfg.MaxSectorProveCommitsSubmittedPerEpoch,
				UseSyntheticPoRep:                      cfg.UseSyntheticPoRep,

				RequireActivationSuccess:         cfg.RequireActivationSuccess,
				RequireActivationSuccessUpdate:   cfg.RequireActivationSuccessUpdate,
				RequireNotificationSuccess:       cfg.RequireNotificationSuccess,
				RequireNotificationSuccessUpdate: cfg.RequireNotificationSuccessUpdate,
			}
			c.SetSealingConfig(newCfg)
		})
		return
	}, nil
}

func ToSealingConfig(dealmakingCfg config.DealmakingConfig, sealingCfg config.SealingConfig) sealiface.Config {
	return sealiface.Config{
		MaxWaitDealsSectors:        sealingCfg.MaxWaitDealsSectors,
		MaxSealingSectors:          sealingCfg.MaxSealingSectors,
		MaxSealingSectorsForDeals:  sealingCfg.MaxSealingSectorsForDeals,
		PreferNewSectorsForDeals:   sealingCfg.PreferNewSectorsForDeals,
		MinUpgradeSectorExpiration: sealingCfg.MinUpgradeSectorExpiration,
		MaxUpgradingSectors:        sealingCfg.MaxUpgradingSectors,

		StartEpochSealingBuffer:         abi.ChainEpoch(dealmakingCfg.StartEpochSealingBuffer),
		MakeNewSectorForDeals:           sealingCfg.MakeNewSectorForDeals,
		CommittedCapacitySectorLifetime: time.Duration(sealingCfg.CommittedCapacitySectorLifetime),
		WaitDealsDelay:                  time.Duration(sealingCfg.WaitDealsDelay),
		MakeCCSectorsAvailable:          sealingCfg.MakeCCSectorsAvailable,
		AlwaysKeepUnsealedCopy:          sealingCfg.AlwaysKeepUnsealedCopy,
		FinalizeEarly:                   sealingCfg.FinalizeEarly,

		CollateralFromMinerBalance: sealingCfg.CollateralFromMinerBalance,
		AvailableBalanceBuffer:     types.BigInt(sealingCfg.AvailableBalanceBuffer),
		DisableCollateralFallback:  sealingCfg.DisableCollateralFallback,

		MaxPreCommitBatch:   sealingCfg.MaxPreCommitBatch,
		PreCommitBatchWait:  time.Duration(sealingCfg.PreCommitBatchWait),
		PreCommitBatchSlack: time.Duration(sealingCfg.PreCommitBatchSlack),

		AggregateCommits:                       sealingCfg.AggregateCommits,
		MinCommitBatch:                         sealingCfg.MinCommitBatch,
		MaxCommitBatch:                         sealingCfg.MaxCommitBatch,
		CommitBatchWait:                        time.Duration(sealingCfg.CommitBatchWait),
		CommitBatchSlack:                       time.Duration(sealingCfg.CommitBatchSlack),
		MaxSectorProveCommitsSubmittedPerEpoch: sealingCfg.MaxSectorProveCommitsSubmittedPerEpoch,

		TerminateBatchMax:  sealingCfg.TerminateBatchMax,
		TerminateBatchMin:  sealingCfg.TerminateBatchMin,
		TerminateBatchWait: time.Duration(sealingCfg.TerminateBatchWait),
		UseSyntheticPoRep:  sealingCfg.UseSyntheticPoRep,

		RequireActivationSuccess:         sealingCfg.RequireActivationSuccess,
		RequireActivationSuccessUpdate:   sealingCfg.RequireActivationSuccessUpdate,
		RequireNotificationSuccess:       sealingCfg.RequireNotificationSuccess,
		RequireNotificationSuccessUpdate: sealingCfg.RequireNotificationSuccessUpdate,
	}
}

func NewGetSealConfigFunc(r repo.LockedRepo) (dtypes.GetSealingConfigFunc, error) {
	return func() (out sealiface.Config, err error) {
		err = readSealingCfg(r, func(dc config.DealmakingConfiger, sc config.SealingConfiger) {
			scfg := sc.GetSealingConfig()
			dcfg := dc.GetDealmakingConfig()
			out = ToSealingConfig(dcfg, scfg)
		})
		return
	}, nil
}

func readSealingCfg(r repo.LockedRepo, accessor func(config.DealmakingConfiger, config.SealingConfiger)) error {
	raw, err := r.Config()
	if err != nil {
		return err
	}

	scfg, ok := raw.(config.SealingConfiger)
	if !ok {
		return xerrors.New("expected config with sealing config trait")
	}

	dcfg, ok := raw.(config.DealmakingConfiger)
	if !ok {
		return xerrors.New("expected config with dealmaking config trait")
	}

	accessor(dcfg, scfg)

	return nil
}

func mutateSealingCfg(r repo.LockedRepo, mutator func(config.SealingConfiger)) error {
	var typeErr error

	setConfigErr := r.SetConfig(func(raw interface{}) {
		cfg, ok := raw.(config.SealingConfiger)
		if !ok {
			typeErr = errors.New("expected config with sealing config trait")
			return
		}

		mutator(cfg)
	})

	return multierr.Combine(typeErr, setConfigErr)
}

func ExtractEnabledMinerSubsystems(cfg config.MinerSubsystemConfig) (res api.MinerSubsystems) {
	if cfg.EnableMining {
		res = append(res, api.SubsystemMining)
	}
	if cfg.EnableSealing {
		res = append(res, api.SubsystemSealing)
	}
	if cfg.EnableSectorStorage {
		res = append(res, api.SubsystemSectorStorage)
	}

	return res
}

package node

import (
	"errors"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/paths"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	sectorstorage "github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

var MinerNode = Options(
	Override(new(sectorstorage.StorageAuth), modules.StorageAuth),

	// Actor config
	Override(new(dtypes.MinerAddress), modules.MinerAddress),
	Override(new(dtypes.MinerID), modules.MinerID),
	Override(new(abi.RegisteredSealProof), modules.SealProofType),
	Override(new(dtypes.NetworkName), modules.StorageNetworkName),

	// Mining / proving
	Override(new(*ctladdr.AddressSelector), modules.AddressSelector(nil)),
)

func ConfigStorageMiner(c interface{}) Option {
	cfg, ok := c.(*config.StorageMiner)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
	}

	return Options(

		Override(new(v1api.FullNode), modules.MakeUuidWrapper),
		ConfigCommon(&cfg.Common, build.NodeUserVersion()),

		Override(CheckFDLimit, modules.CheckFdLimit(buildconstants.MinerFDLimit)), // recommend at least 100k FD limit to miners

		Override(new(api.MinerSubsystems), modules.ExtractEnabledMinerSubsystems(cfg.Subsystems)),
		Override(new(paths.LocalStorage), From(new(repo.LockedRepo))),
		Override(new(*paths.Local), modules.LocalStorage),
		Override(new(*paths.Remote), modules.RemoteStorage),
		Override(new(paths.Store), From(new(*paths.Remote))),

		If(cfg.Subsystems.EnableMining || cfg.Subsystems.EnableSealing,
			Override(GetParamsKey, modules.GetParams(!cfg.Proving.DisableBuiltinWindowPoSt || !cfg.Proving.DisableBuiltinWinningPoSt || cfg.Storage.AllowCommit || cfg.Storage.AllowProveReplicaUpdate2)),
		),

		If(!cfg.Subsystems.EnableMining,
			If(cfg.Subsystems.EnableSealing, Error(xerrors.Errorf("sealing can only be enabled on a mining node"))),
			If(cfg.Subsystems.EnableSectorStorage, Error(xerrors.Errorf("sealing can only be enabled on a mining node"))),
		),

		If(cfg.Subsystems.EnableMining,
			If(!cfg.Subsystems.EnableSealing, Error(xerrors.Errorf("sealing can't be disabled on a mining node yet"))),
			If(!cfg.Subsystems.EnableSectorStorage, Error(xerrors.Errorf("sealing can't be disabled on a mining node yet"))),

			// Sector storage: Proofs
			Override(new(storiface.Verifier), ffiwrapper.ProofVerifier),
			Override(new(storiface.Prover), ffiwrapper.ProofProver),
			Override(new(storiface.ProverPoSt), From(new(sectorstorage.SectorManager))),

			Override(new(dtypes.SetSealingConfigFunc), modules.NewSetSealConfigFunc),
			Override(new(dtypes.GetSealingConfigFunc), modules.NewGetSealConfigFunc),

			// Mining / proving
			Override(new(*slashfilter.SlashFilter), modules.NewSlashFilter),

			If(!cfg.Subsystems.DisableWinningPoSt,
				Override(new(*miner.Miner), modules.SetupBlockProducer),
				Override(new(gen.WinningPoStProver), storage.NewWinningPoStProver),
			),

			Override(PreflightChecksKey, modules.PreflightChecks),
			Override(new(*sealing.Sealing), modules.SealingPipeline(cfg.Fees)),

			If(!cfg.Subsystems.DisableWindowPoSt, Override(new(*wdpost.WindowPoStScheduler), modules.WindowPostScheduler(cfg.Fees, cfg.Proving))),
			Override(new(sectorblocks.SectorBuilder), From(new(*sealing.Sealing))),
		),

		If(cfg.Subsystems.EnableSectorStorage,
			// Sector storage
			If(cfg.Subsystems.EnableSectorIndexDB,
				Override(new(*paths.DBIndex), paths.NewDBIndex),
				Override(new(paths.SectorIndex), From(new(*paths.DBIndex))),

				// sector index db is the only thing on lotus-miner that will use harmonydb
				Override(new(*harmonydb.DB), func(cfg config.HarmonyDB) (*harmonydb.DB, error) {
					return harmonydb.NewFromConfig(cfg)
				}),
			),
			If(!cfg.Subsystems.EnableSectorIndexDB,
				Override(new(*paths.MemIndex), paths.NewMemIndex),
				Override(new(paths.SectorIndex), From(new(*paths.MemIndex))),
			),

			Override(new(*sectorstorage.Manager), modules.SectorStorage),
			Override(new(sectorstorage.Unsealer), From(new(*sectorstorage.Manager))),
			Override(new(sectorstorage.SectorManager), From(new(*sectorstorage.Manager))),
			Override(new(storiface.WorkerReturn), From(new(sectorstorage.SectorManager))),
		),

		If(!cfg.Subsystems.EnableSectorStorage,
			Override(new(sectorstorage.StorageAuth), modules.StorageAuthWithURL(cfg.Subsystems.SectorIndexApiInfo)),
			Override(new(modules.MinerStorageService), modules.ConnectStorageService(cfg.Subsystems.SectorIndexApiInfo)),
			Override(new(sectorstorage.Unsealer), From(new(modules.MinerStorageService))),
			Override(new(sectorblocks.SectorBuilder), From(new(modules.MinerStorageService))),
		),
		If(!cfg.Subsystems.EnableSealing,
			Override(new(modules.MinerSealingService), modules.ConnectSealingService(cfg.Subsystems.SealerApiInfo)),
			Override(new(paths.SectorIndex), From(new(modules.MinerSealingService))),
		),

		Override(new(config.SealerConfig), cfg.Storage),
		Override(new(config.ProvingConfig), cfg.Proving),
		Override(new(config.HarmonyDB), cfg.HarmonyDB),
		Override(new(*ctladdr.AddressSelector), modules.AddressSelector(&cfg.Addresses)),
		If(build.IsF3Enabled(), Override(F3Participation, modules.F3Participation)),
	)
}

func StorageMiner(out *api.StorageMiner) Option {
	return Options(
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the StorageMiner option must be set before Config option")),
		),

		func(s *Settings) error {
			s.nodeType = repo.StorageMiner
			return nil
		},

		func(s *Settings) error {
			resAPI := &impl.StorageMinerAPI{}
			s.invokes[ExtractApiKey] = fx.Populate(resAPI)
			*out = resAPI
			return nil
		},
	)
}

package miner

import (
	"context"
	"encoding/json"
	"os"

	"github.com/cheggaaa/pb/v3"
	"github.com/docker/go-units"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/backupds"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var restoreCmd = &cli.Command{
	Name:  "restore",
	Usage: "Initialize a lotus miner repo from a backup",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.StringFlag{
			Name:  "config",
			Usage: "config file (config.toml)",
		},
		&cli.StringFlag{
			Name:  "storage-config",
			Usage: "storage paths config (storage.json)",
		},
	},
	ArgsUsage: "[backupFile]",
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		log.Info("Initializing lotus miner using a backup")

		var storageCfg *storiface.StorageConfig
		if cctx.IsSet("storage-config") {
			cf, err := homedir.Expand(cctx.String("storage-config"))
			if err != nil {
				return xerrors.Errorf("expanding storage config path: %w", err)
			}

			cfb, err := os.ReadFile(cf)
			if err != nil {
				return xerrors.Errorf("reading storage config: %w", err)
			}

			storageCfg = &storiface.StorageConfig{}
			err = json.Unmarshal(cfb, storageCfg)
			if err != nil {
				return xerrors.Errorf("cannot unmarshal json for storage config: %w", err)
			}
		}

		repoPath := cctx.String(FlagMinerRepo)

		if err := restore(ctx, cctx, repoPath, storageCfg, nil, func(api lapi.FullNode, maddr address.Address, peerid peer.ID, mi api.MinerInfo) error {
			log.Info("Checking proof parameters")

			if err := paramfetch.GetParams(ctx, build.ParametersJSON(), build.SrsJSON(), uint64(mi.SectorSize)); err != nil {
				return xerrors.Errorf("fetching proof parameters: %w", err)
			}

			log.Info("Configuring miner actor")
			if err := configureStorageMiner(ctx, api, maddr, peerid, big.Zero(), cctx.Uint64("confidence")); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	},
}

func restore(ctx context.Context, cctx *cli.Context, targetPath string, strConfig *storiface.StorageConfig, manageConfig func(*config.StorageMiner) error, after func(api lapi.FullNode, addr address.Address, peerid peer.ID, mi api.MinerInfo) error) error {
	if cctx.NArg() != 1 {
		return lcli.IncorrectNumArgs(cctx)
	}

	log.Info("Trying to connect to full node RPC")

	api, closer, err := lcli.GetFullNodeAPIV1(cctx) // TODO: consider storing full node address in config
	if err != nil {
		return err
	}
	defer closer()

	log.Info("Checking full node version")

	v, err := api.Version(ctx)
	if err != nil {
		return err
	}

	if !v.APIVersion.EqMajorMinor(lapi.FullAPIVersion1) {
		return xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", lapi.FullAPIVersion1, v.APIVersion)
	}

	if !cctx.Bool("nosync") {
		if err := lcli.SyncWait(ctx, &v0api.WrapperV1Full{FullNode: api}, false); err != nil {
			return xerrors.Errorf("sync wait: %w", err)
		}
	}

	bf, err := homedir.Expand(cctx.Args().First())
	if err != nil {
		return xerrors.Errorf("expand backup file path: %w", err)
	}

	st, err := os.Stat(bf)
	if err != nil {
		return xerrors.Errorf("stat backup file (%s): %w", bf, err)
	}

	f, err := os.Open(bf)
	if err != nil {
		return xerrors.Errorf("opening backup file: %w", err)
	}
	defer f.Close() // nolint:errcheck

	log.Info("Checking if repo exists")

	r, err := repo.NewFS(targetPath)
	if err != nil {
		return err
	}

	ok, err := r.Exists()
	if err != nil {
		return err
	}
	if ok {
		return xerrors.Errorf("repo at '%s' is already initialized", cctx.String(FlagMinerRepo))
	}

	log.Info("Initializing repo")

	if err := r.Init(repo.StorageMiner); err != nil {
		return err
	}

	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		return err
	}
	defer lr.Close() //nolint:errcheck

	if cctx.IsSet("config") {
		log.Info("Restoring config")

		cf, err := homedir.Expand(cctx.String("config"))
		if err != nil {
			return xerrors.Errorf("expanding config path: %w", err)
		}

		_, err = os.Stat(cf)
		if err != nil {
			return xerrors.Errorf("stat config file (%s): %w", cf, err)
		}

		var cerr error
		err = lr.SetConfig(func(raw interface{}) {
			rcfg, ok := raw.(*config.StorageMiner)
			if !ok {
				cerr = xerrors.New("expected miner config")
				return
			}

			ff, err := config.FromFile(cf, config.SetDefault(func() (interface{}, error) { return rcfg, nil }))
			if err != nil {
				cerr = xerrors.Errorf("loading config: %w", err)
				return
			}

			*rcfg = *ff.(*config.StorageMiner)
			if manageConfig != nil {
				cerr = manageConfig(rcfg)
			}
		})
		if cerr != nil {
			return cerr
		}
		if err != nil {
			return xerrors.Errorf("setting config: %w", err)
		}

	} else {
		log.Warn("--config NOT SET, WILL USE DEFAULT VALUES")
	}

	if strConfig != nil {
		log.Info("Restoring storage path config")

		err = lr.SetStorage(func(scfg *storiface.StorageConfig) {
			*scfg = *strConfig
		})
		if err != nil {
			return xerrors.Errorf("setting storage config: %w", err)
		}
	} else {
		log.Warn("--storage-config NOT SET. NO SECTOR PATHS WILL BE CONFIGURED")
		// setting empty config to allow miner to be started
		if err := lr.SetStorage(func(sc *storiface.StorageConfig) {
			sc.StoragePaths = append(sc.StoragePaths, storiface.LocalPath{})
		}); err != nil {
			return xerrors.Errorf("set storage config: %w", err)
		}
	}

	log.Info("Restoring metadata backup")

	mds, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		return err
	}

	bar := pb.Full.Start64(st.Size())
	br := bar.NewProxyReader(f)

	err = backupds.RestoreInto(br, mds)
	bar.Finish()

	if err != nil {
		return xerrors.Errorf("restoring metadata: %w", err)
	}

	log.Info("Checking actor metadata")

	abytes, err := mds.Get(ctx, datastore.NewKey("miner-address"))
	if err != nil {
		return xerrors.Errorf("getting actor address from metadata datastore: %w", err)
	}

	maddr, err := address.NewFromBytes(abytes)
	if err != nil {
		return xerrors.Errorf("parsing actor address: %w", err)
	}

	log.Info("ACTOR ADDRESS: ", maddr.String())

	mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting miner info: %w", err)
	}

	log.Info("SECTOR SIZE: ", units.BytesSize(float64(mi.SectorSize)))

	wk, err := api.StateAccountKey(ctx, mi.Worker, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("resolving worker key: %w", err)
	}

	has, err := api.WalletHas(ctx, wk)
	if err != nil {
		return xerrors.Errorf("checking worker address: %w", err)
	}

	if !has {
		return xerrors.Errorf("worker address %s for miner actor %s not present in full node wallet", mi.Worker, maddr)
	}

	log.Info("Initializing libp2p identity")

	p2pSk, err := makeHostKey(lr)
	if err != nil {
		return xerrors.Errorf("make host key: %w", err)
	}

	peerid, err := peer.IDFromPrivateKey(p2pSk)
	if err != nil {
		return xerrors.Errorf("peer ID from private key: %w", err)
	}

	return after(api, maddr, peerid, mi)
}

package main

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/docker/go-units"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"gopkg.in/cheggaaa/pb.v1"

	"github.com/filecoin-project/go-address"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/lib/backupds"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

var initRestoreCmd = &cli.Command{
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
		log.Info("Initializing lotus miner using a backup")
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("expected 1 argument")
		}

		log.Info("Trying to connect to full node RPC")

		api, closer, err := lcli.GetFullNodeAPI(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
		defer closer()

		log.Info("Checking full node version")

		ctx := lcli.ReqContext(cctx)

		v, err := api.Version(ctx)
		if err != nil {
			return err
		}

		if !v.APIVersion.EqMajorMinor(build.FullAPIVersion) {
			return xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", build.FullAPIVersion, v.APIVersion)
		}

		if !cctx.Bool("nosync") {
			if err := lcli.SyncWait(ctx, api); err != nil {
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

		repoPath := cctx.String(FlagMinerRepo)
		r, err := repo.NewFS(repoPath)
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

				ff, err := config.FromFile(cf, rcfg)
				if err != nil {
					cerr = xerrors.Errorf("loading config: %w", err)
					return
				}

				*rcfg = *ff.(*config.StorageMiner)
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

		if cctx.IsSet("storage-config") {
			log.Info("Restoring storage path config")

			cf, err := homedir.Expand(cctx.String("storage-config"))
			if err != nil {
				return xerrors.Errorf("expanding storage config path: %w", err)
			}

			cfb, err := ioutil.ReadFile(cf)
			if err != nil {
				return xerrors.Errorf("reading storage config: %w", err)
			}

			var cerr error
			err = lr.SetStorage(func(scfg *stores.StorageConfig) {
				cerr = json.Unmarshal(cfb, scfg)
			})
			if cerr != nil {
				return xerrors.Errorf("unmarshalling storage config: %w", cerr)
			}
			if err != nil {
				return xerrors.Errorf("setting storage config: %w", err)
			}
		} else {
			log.Warn("--storage-config NOT SET. NO SECTOR PATHS WILL BE CONFIGURED")
		}

		log.Info("Restoring metadata backup")

		mds, err := lr.Datastore("/metadata")
		if err != nil {
			return err
		}

		bar := pb.New64(st.Size())
		br := bar.NewProxyReader(f)
		bar.ShowTimeLeft = true
		bar.ShowPercent = true
		bar.ShowSpeed = true
		bar.Units = pb.U_BYTES

		bar.Start()
		err = backupds.RestoreInto(br, mds)
		bar.Finish()

		if err != nil {
			return xerrors.Errorf("restoring metadata: %w", err)
		}

		log.Info("Checking actor metadata")

		abytes, err := mds.Get(datastore.NewKey("miner-address"))
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

		log.Info("Checking proof parameters")

		if err := paramfetch.GetParams(ctx, build.ParametersJSON(), uint64(mi.SectorSize)); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
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

		log.Info("Configuring miner actor")

		if err := configureStorageMiner(ctx, api, maddr, peerid, big.Zero()); err != nil {
			return err
		}

		return nil
	},
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	levelds "github.com/ipfs/go-ds-leveldb"
	ipldcbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	market14 "github.com/filecoin-project/go-state-types/builtin/v14/market"
	"github.com/filecoin-project/go-state-types/builtin/v14/util/adt"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/backupds"
	"github.com/filecoin-project/lotus/node/repo"
)

var marketCmd = &cli.Command{
	Name:  "market",
	Usage: "Interact with the market actor",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		marketDealFeesCmd,
		marketExportDatastoreCmd,
		marketImportDatastoreCmd,
		marketDealsTotalStorageCmd,
		marketCronStateCmd,
	},
}

var marketCronStateCmd = &cli.Command{
	Name:  "cron-state",
	Usage: "Display summary of all deal operation state scheduled for cron processing",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids)",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		bs := ReadOnlyAPIBlockstore{api}
		adtStore := adt.WrapStore(ctx, ipldcbor.NewCborStore(&bs))

		mAct, err := api.StateGetActor(ctx, builtin.StorageMarketActorAddr, ts.Key())
		if err != nil {
			return err
		}

		var mSt market14.State
		err = adtStore.Get(ctx, mAct.Head, &mSt)
		if err != nil {
			return err
		}

		dealOpsEpochSet, err := adt.AsMap(adtStore, mSt.DealOpsByEpoch, builtin.DefaultHamtBitwidth)
		if err != nil {
			return err
		}
		dealOpsRecord := make(map[uint64][]abi.DealID)
		if err := dealOpsEpochSet.ForEach(&cbg.Deferred{}, func(eKey string) error {
			e, err := abi.ParseUIntKey(eKey)
			if err != nil {
				return err
			}
			dealOpsRecord[e] = make([]abi.DealID, 0)
			return nil
		}); err != nil {
			return err
		}

		dealOpsMultiMap, err := market14.AsSetMultimap(adtStore, mSt.DealOpsByEpoch, builtin.DefaultHamtBitwidth, builtin.DefaultHamtBitwidth)
		if err != nil {
			return err
		}
		for e := range dealOpsRecord {
			e := e
			err := dealOpsMultiMap.ForEach(abi.ChainEpoch(e), func(id abi.DealID) error {
				dealOpsRecord[e] = append(dealOpsRecord[e], id)
				return nil
			})
			if err != nil {
				return err
			}
		}
		jsonStr, err := json.Marshal(dealOpsRecord)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", jsonStr)
		return nil
	},
}

var marketDealFeesCmd = &cli.Command{
	Name:  "get-deal-fees",
	Usage: "View the storage fees associated with a particular deal or storage provider",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "provider",
			Usage: "provider whose outstanding fees you'd like to calculate",
		},
		&cli.IntFlag{
			Name:  "dealId",
			Usage: "deal whose outstanding fees you'd like to calculate",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		ht := ts.Height()

		if cctx.IsSet("provider") {
			p, err := address.NewFromString(cctx.String("provider"))
			if err != nil {
				return fmt.Errorf("failed to parse provider: %w", err)
			}

			deals, err := api.StateMarketDeals(ctx, ts.Key())
			if err != nil {
				return err
			}

			ef := big.Zero()
			pf := big.Zero()
			count := 0

			for _, deal := range deals {
				if deal.Proposal.Provider == p {
					e, p := market.GetDealFees(deal.Proposal, ht)
					ef = big.Add(ef, e)
					pf = big.Add(pf, p)
					count++
				}
			}

			fmt.Println("Total deals: ", count)
			fmt.Println("Total earned fees: ", ef)
			fmt.Println("Total pending fees: ", pf)
			fmt.Println("Total fees: ", big.Add(ef, pf))

			return nil
		}

		if dealid := cctx.Int("dealId"); dealid != 0 {
			deal, err := api.StateMarketStorageDeal(ctx, abi.DealID(dealid), ts.Key())
			if err != nil {
				return err
			}

			ef, pf := market.GetDealFees(deal.Proposal, ht)

			fmt.Println("Earned fees: ", ef)
			fmt.Println("Pending fees: ", pf)
			fmt.Println("Total fees: ", big.Add(ef, pf))

			return nil
		}

		return xerrors.New("must provide either --provider or --dealId flag")
	},
}

const mktsMetadataNamespace = "metadata"

var marketExportDatastoreCmd = &cli.Command{
	Name:        "export-datastore",
	Description: "export markets datastore key/values to a file",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Usage: "path to the repo",
		},
		&cli.StringFlag{
			Name:  "backup-dir",
			Usage: "path to the backup directory",
		},
	},
	Action: func(cctx *cli.Context) error {
		_ = logging.SetLogLevel("badger", "ERROR")

		// If the backup dir is not specified, just use the OS temp dir
		backupDir := cctx.String("backup-dir")
		if backupDir == "" {
			backupDir = os.TempDir()
		}

		// Open the repo at the repo path
		repoPath := cctx.String("repo")
		lr, err := openLockedRepo(repoPath)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		// Open the metadata datastore on the repo
		ds, err := lr.Datastore(cctx.Context, datastore.NewKey(mktsMetadataNamespace).String())
		if err != nil {
			return xerrors.Errorf("opening datastore %s on repo %s: %w", mktsMetadataNamespace, repoPath, err)
		}

		// Create a tmp datastore that we'll add the exported key / values to
		// and then backup
		backupDsDir := path.Join(backupDir, "markets-backup-datastore")
		if err := os.MkdirAll(backupDsDir, 0775); err != nil { //nolint:gosec
			return xerrors.Errorf("creating tmp datastore directory: %w", err)
		}
		defer os.RemoveAll(backupDsDir) //nolint:errcheck

		backupDs, err := levelds.NewDatastore(backupDsDir, &levelds.Options{
			Compression: ldbopts.NoCompression,
			NoSync:      false,
			Strict:      ldbopts.StrictAll,
			ReadOnly:    false,
		})
		if err != nil {
			return xerrors.Errorf("opening backup datastore at %s: %w", backupDir, err)
		}

		// Export the key / values
		prefixes := []string{
			"/deals/provider",
			"/retrievals/provider",
			"/storagemarket",
		}
		for _, prefix := range prefixes {
			err := exportPrefix(prefix, ds, backupDs)
			if err != nil {
				return err
			}
		}

		// Wrap the datastore in a backup datastore
		bds, err := backupds.Wrap(backupDs, "")
		if err != nil {
			return xerrors.Errorf("opening backupds: %w", err)
		}

		// Create a file for the backup
		fpath := path.Join(backupDir, "markets.datastore.backup")
		out, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return xerrors.Errorf("opening backup file %s: %w", fpath, err)
		}

		// Write the backup to the file
		if err := bds.Backup(context.Background(), out); err != nil {
			if cerr := out.Close(); cerr != nil {
				log.Errorw("error closing backup file while handling backup error", "closeErr", cerr, "backupErr", err)
			}
			return xerrors.Errorf("backup error: %w", err)
		}
		if err := out.Close(); err != nil {
			return xerrors.Errorf("closing backup file: %w", err)
		}

		fmt.Println("Wrote backup file to " + fpath)

		return nil
	},
}

func exportPrefix(prefix string, ds datastore.Batching, backupDs datastore.Batching) error {
	q, err := ds.Query(context.Background(), dsq.Query{
		Prefix: prefix,
	})
	if err != nil {
		return xerrors.Errorf("datastore query: %w", err)
	}
	defer q.Close() //nolint:errcheck

	for res := range q.Next() {
		fmt.Println("Exporting key " + res.Key)
		err := backupDs.Put(context.Background(), datastore.NewKey(res.Key), res.Value)
		if err != nil {
			return xerrors.Errorf("putting %s to backup datastore: %w", res.Key, err)
		}
	}

	return nil
}

var marketImportDatastoreCmd = &cli.Command{
	Name:        "import-datastore",
	Description: "import markets datastore key/values from a backup file",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Usage: "path to the repo",
		},
		&cli.StringFlag{
			Name:     "backup-path",
			Usage:    "path to the backup file",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		_ = logging.SetLogLevel("badger", "ERROR")

		backupPath := cctx.String("backup-path")

		// Open the repo at the repo path
		lr, err := openLockedRepo(cctx.String("repo"))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		// Open the metadata datastore on the repo
		repoDs, err := lr.Datastore(cctx.Context, datastore.NewKey(mktsMetadataNamespace).String())
		if err != nil {
			return err
		}

		r, err := os.Open(backupPath)
		if err != nil {
			return xerrors.Errorf("opening backup path %s: %w", backupPath, err)
		}

		fmt.Println("Importing from backup file " + backupPath)
		err = backupds.RestoreInto(r, repoDs)
		if err != nil {
			return xerrors.Errorf("restoring backup from path %s: %w", backupPath, err)
		}

		fmt.Println("Completed importing from backup file " + backupPath)

		return nil
	},
}

var marketDealsTotalStorageCmd = &cli.Command{
	Name:  "get-deals-total-storage",
	Usage: "View the total storage available in all active market deals",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		deals, err := api.StateMarketDeals(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		total := big.Zero()
		count := 0

		for _, deal := range deals {
			if market.IsDealActive(deal.State.Iface()) {
				dealStorage := big.NewIntUnsigned(uint64(deal.Proposal.PieceSize))
				total = big.Add(total, dealStorage)
				count++
			}

		}

		fmt.Println("Total deals: ", count)
		fmt.Println("Total storage: ", total)

		return nil
	},
}

func openLockedRepo(path string) (repo.LockedRepo, error) {
	// Open the repo at the repo path
	rpo, err := repo.NewFS(path)
	if err != nil {
		return nil, xerrors.Errorf("could not open repo %s: %w", path, err)
	}

	// Make sure the repo exists
	exists, err := rpo.Exists()
	if err != nil {
		return nil, xerrors.Errorf("checking repo %s exists: %w", path, err)
	}
	if !exists {
		return nil, xerrors.Errorf("repo does not exist: %s", path)
	}

	// Lock the repo
	lr, err := rpo.Lock(repo.StorageMiner)
	if err != nil {
		return nil, xerrors.Errorf("locking repo %s: %w", path, err)
	}

	return lr, nil
}

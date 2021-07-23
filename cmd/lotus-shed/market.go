package main

import (
	"fmt"
	"os"
	"path"

	levelds "github.com/ipfs/go-ds-leveldb"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/filecoin-project/lotus/lib/backupds"

	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"

	lcli "github.com/filecoin-project/lotus/cli"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var marketCmd = &cli.Command{
	Name:  "market",
	Usage: "Interact with the market actor",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		marketDealFeesCmd,
		marketExportDatastoreCmd,
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
					e, p := deal.Proposal.GetDealFees(ht)
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

			ef, pf := deal.Proposal.GetDealFees(ht)

			fmt.Println("Earned fees: ", ef)
			fmt.Println("Pending fees: ", pf)
			fmt.Println("Total fees: ", big.Add(ef, pf))

			return nil
		}

		return xerrors.New("must provide either --provider or --dealId flag")
	},
}

var marketExportDatastoreCmd = &cli.Command{
	Name:        "export-datastore",
	Description: "export datastore to a file",
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
		logging.SetLogLevel("badger", "ERROR") // nolint:errcheck

		// If the backup dir is not specified, just use the OS temp dir
		backupDir := cctx.String("backup-dir")
		if backupDir == "" {
			backupDir = os.TempDir()
		}

		// Create a new repo at the repo path
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		// Make sure the repo exists
		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		// Lock the repo
		lr, err := r.Lock(repo.StorageMiner)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		// Open the metadata datastore on the repo
		namespace := "metadata"
		ds, err := lr.Datastore(cctx.Context, datastore.NewKey(namespace).String())
		if err != nil {
			return err
		}

		// Create a tmp datastore that we'll add the exported key / values to
		// and then backup
		backupDs, err := levelds.NewDatastore(backupDir, &levelds.Options{
			Compression: ldbopts.NoCompression,
			NoSync:      false,
			Strict:      ldbopts.StrictAll,
			ReadOnly:    false,
		})

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
			return xerrors.Errorf("open %s: %w", fpath, err)
		}

		// Write the backup to the file
		if err := bds.Backup(out); err != nil {
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
	q, err := ds.Query(dsq.Query{
		Prefix: prefix,
	})
	if err != nil {
		return xerrors.Errorf("datastore query: %w", err)
	}
	defer q.Close() //nolint:errcheck

	for res := range q.Next() {
		fmt.Println("Exporting key " + res.Key)
		err := backupDs.Put(datastore.NewKey(res.Key), res.Value)
		if err != nil {
			return xerrors.Errorf("putting %s to backup datastore: %w", res.Key, err)
		}
	}

	return nil
}

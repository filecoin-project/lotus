package main

import (
	"context"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

const (
	MarketsService = "markets"
)

var serviceCmd = &cli.Command{
	Name:  "service",
	Usage: "Initialize a lotus miner sub-service",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "config",
			Usage:    "config file (config.toml)",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.StringSliceFlag{
			Name:  "type",
			Usage: "type of service to be enabled",
		},
		&cli.StringFlag{
			Name:  "api-sealer",
			Usage: "sealer API info (lotus-miner auth api-info --perm=admin)",
		},
		&cli.StringFlag{
			Name:  "api-sector-index",
			Usage: "sector Index API info (lotus-miner auth api-info --perm=admin)",
		},
	},
	ArgsUsage: "[backupFile]",
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		log.Info("Initializing lotus miner service")

		es := EnabledServices(cctx.StringSlice("type"))

		if len(es) == 0 {
			return xerrors.Errorf("at least one module must be enabled")
		}

		// we should remove this as soon as we have more service types and not just `markets`
		if !es.Contains(MarketsService) {
			return xerrors.Errorf("markets module must be enabled")
		}

		if !cctx.IsSet("api-sealer") {
			return xerrors.Errorf("--api-sealer is required without the sealer module enabled")
		}
		if !cctx.IsSet("api-sector-index") {
			return xerrors.Errorf("--api-sector-index is required without the sector storage module enabled")
		}

		repoPath := cctx.String(FlagMarketsRepo)
		if repoPath == "" {
			return xerrors.Errorf("please provide Lotus markets repo path via flag %s", FlagMarketsRepo)
		}

		if err := restore(ctx, cctx, repoPath, &stores.StorageConfig{}, func(cfg *config.StorageMiner) error {
			cfg.Subsystems.EnableMarkets = es.Contains(MarketsService)
			cfg.Subsystems.EnableMining = false
			cfg.Subsystems.EnableSealing = false
			cfg.Subsystems.EnableSectorStorage = false

			if !cfg.Subsystems.EnableSealing {
				ai, err := checkApiInfo(ctx, cctx.String("api-sealer"))
				if err != nil {
					return xerrors.Errorf("checking sealer API: %w", err)
				}
				cfg.Subsystems.SealerApiInfo = ai
			}

			if !cfg.Subsystems.EnableSectorStorage {
				ai, err := checkApiInfo(ctx, cctx.String("api-sector-index"))
				if err != nil {
					return xerrors.Errorf("checking sector index API: %w", err)
				}
				cfg.Subsystems.SectorIndexApiInfo = ai
			}

			return nil
		}, func(api lapi.FullNode, maddr address.Address, peerid peer.ID, mi miner.MinerInfo) error {
			if es.Contains(MarketsService) {
				log.Info("Configuring miner actor")

				if err := configureStorageMiner(ctx, api, maddr, peerid, big.Zero()); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	},
}

type EnabledServices []string

func (es EnabledServices) Contains(name string) bool {
	for _, s := range es {
		if s == name {
			return true
		}
	}
	return false
}

func checkApiInfo(ctx context.Context, ai string) (string, error) {
	ai = strings.TrimPrefix(strings.TrimSpace(ai), "MINER_API_INFO=")
	info := cliutil.ParseApiInfo(ai)
	addr, err := info.DialArgs("v0")
	if err != nil {
		return "", xerrors.Errorf("could not get DialArgs: %w", err)
	}

	log.Infof("Checking api version of %s", addr)

	api, closer, err := client.NewStorageMinerRPCV0(ctx, addr, info.AuthHeader())
	if err != nil {
		return "", err
	}
	defer closer()

	v, err := api.Version(ctx)
	if err != nil {
		return "", xerrors.Errorf("checking version: %w", err)
	}

	if !v.APIVersion.EqMajorMinor(lapi.MinerAPIVersion0) {
		return "", xerrors.Errorf("remote service API version didn't match (expected %s, remote %s)", lapi.MinerAPIVersion0, v.APIVersion)
	}

	return ai, nil
}

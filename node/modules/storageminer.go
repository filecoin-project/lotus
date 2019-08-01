package modules

import (
	"context"
	"path/filepath"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/mitchellh/go-homedir"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/filecoin-project/go-lotus/storage"
)

func SectorBuilderConfig(storagePath string) func() (*sectorbuilder.SectorBuilderConfig, error) {
	return func() (*sectorbuilder.SectorBuilderConfig, error) {
		sp, err := homedir.Expand(storagePath)
		if err != nil {
			return nil, err
		}

		metadata := filepath.Join(sp, "meta")
		sealed := filepath.Join(sp, "sealed")
		staging := filepath.Join(sp, "staging")

		// TODO: get the address of the miner actor
		minerAddr, err := address.NewIDAddress(42)
		if err != nil {
			return nil, err
		}

		sb := &sectorbuilder.SectorBuilderConfig{
			Miner:       minerAddr,
			SectorSize:  1024,
			MetadataDir: metadata,
			SealedDir:   sealed,
			StagedDir:   staging,
		}

		return sb, nil
	}
}

func SectorBuilder(mctx helpers.MetricsCtx, lc fx.Lifecycle, sbc *sectorbuilder.SectorBuilderConfig) (*sectorbuilder.SectorBuilder, error) {
	sb, err := sectorbuilder.New(sbc)
	if err != nil {
		return nil, err
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			sb.Run(ctx)
			return nil
		},
	})

	return sb, nil
}

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api api.FullNode, h host.Host, ds dtypes.MetadataDS, sb *sectorbuilder.SectorBuilder, w *wallet.Wallet) (*storage.Miner, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return nil, err
	}

	maddr, err := address.NewFromBytes(maddrb)
	if err != nil {
		return nil, err
	}

	sm, err := storage.NewMiner(api, maddr, h, ds, sb, w)
	if err != nil {
		return nil, err
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return sm.Run(ctx)
		},
	})

	return sm, nil
}

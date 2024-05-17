package modules

import (
	"bytes"
	"context"

	"github.com/ipfs/go-datastore"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

func HandleMigrateClientFunds(lc fx.Lifecycle, mctx helpers.MetricsCtx, ds dtypes.MetadataDS, wallet full.WalletAPI, fundMgr *market.FundManager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			addr, err := wallet.WalletDefaultAddress(ctx)
			// nothing to be done if there is no default address
			if err != nil {
				return nil
			}
			b, err := ds.Get(helpers.LifecycleCtx(mctx, lc), datastore.NewKey("/marketfunds/client"))
			if err != nil {
				if xerrors.Is(err, datastore.ErrNotFound) {
					return nil
				}
				log.Errorf("client funds migration - getting datastore value: %v", err)
				return nil
			}

			var value abi.TokenAmount
			if err = value.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
				log.Errorf("client funds migration - unmarshalling datastore value: %v", err)
				return nil
			}
			_, err = fundMgr.Reserve(ctx, addr, addr, value)
			if err != nil {
				log.Errorf("client funds migration - reserving funds (wallet %s, addr %s, funds %d): %v",
					addr, addr, value, err)
				return nil
			}

			return ds.Delete(helpers.LifecycleCtx(mctx, lc), datastore.NewKey("/marketfunds/client"))
		},
	})
}

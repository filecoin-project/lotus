package modules

import (
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-filestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/lotus/lib/bufbstore"
	"github.com/filecoin-project/lotus/lib/ipfsbstore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

func IpfsClientBlockstore(mctx helpers.MetricsCtx, lc fx.Lifecycle, fstore dtypes.ClientFilestore) (dtypes.ClientBlockstore, error) {
	ipfsbs, err := ipfsbstore.NewIpfsBstore(helpers.LifecycleCtx(mctx, lc))
	if err != nil {
		return nil, xerrors.Errorf("constructing ipfs blockstore: %w", err)
	}

	return bufbstore.NewTieredBstore(
		ipfsbs,
		blockstore.NewIdStore((*filestore.Filestore)(fstore)),
	), nil
}

func IpfsRemoteClientBlockstore(ipfsMaddr string) func(helpers.MetricsCtx, fx.Lifecycle, dtypes.ClientFilestore) (dtypes.ClientBlockstore, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fstore dtypes.ClientFilestore) (dtypes.ClientBlockstore, error) {
		ma, err := multiaddr.NewMultiaddr(ipfsMaddr)
		if err != nil {
			return nil, xerrors.Errorf("parsing ipfs multiaddr: %w", err)
		}
		ipfsbs, err := ipfsbstore.NewRemoteIpfsBstore(helpers.LifecycleCtx(mctx, lc), ma)
		if err != nil {
			return nil, xerrors.Errorf("constructing ipfs blockstore: %w", err)
		}

		return bufbstore.NewTieredBstore(
			ipfsbs,
			blockstore.NewIdStore((*filestore.Filestore)(fstore)),
		), nil
	}
}

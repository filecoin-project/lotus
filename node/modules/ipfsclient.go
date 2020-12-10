package modules

import (
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/ipfsbstore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

// IpfsClientBlockstore returns a ClientBlockstore implementation backed by an IPFS node.
// If ipfsMaddr is empty, a local IPFS node is assumed considering IPFS_PATH configuration.
// If ipfsMaddr is not empty, it will connect to the remote IPFS node with the provided multiaddress.
// The flag useForRetrieval indicates if the IPFS node will also be used for storing retrieving deals.
func IpfsClientBlockstore(ipfsMaddr string, onlineMode bool) func(helpers.MetricsCtx, fx.Lifecycle, dtypes.ClientImportMgr) (dtypes.ClientBlockstore, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, localStore dtypes.ClientImportMgr) (dtypes.ClientBlockstore, error) {
		var err error
		var ipfsbs blockstore.Blockstore
		if ipfsMaddr != "" {
			var ma multiaddr.Multiaddr
			ma, err = multiaddr.NewMultiaddr(ipfsMaddr)
			if err != nil {
				return nil, xerrors.Errorf("parsing ipfs multiaddr: %w", err)
			}
			ipfsbs, err = ipfsbstore.NewRemoteIpfsBstore(helpers.LifecycleCtx(mctx, lc), ma, onlineMode)
		} else {
			ipfsbs, err = ipfsbstore.NewIpfsBstore(helpers.LifecycleCtx(mctx, lc), onlineMode)
		}
		if err != nil {
			return nil, xerrors.Errorf("constructing ipfs blockstore: %w", err)
		}
		return blockstore.WrapIDStore(ipfsbs), nil
	}
}

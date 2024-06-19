package lf3

import (
	"context"
	"time"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/blssig"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type F3 struct {
	f3 *f3.F3
}

type F3Params struct {
	fx.In

	NetworkName  dtypes.NetworkName
	PubSub       *pubsub.PubSub
	Host         host.Host
	ChainStore   *store.ChainStore
	StateManager *stmgr.StateManager
	Datastore    dtypes.MetadataDS
	Wallet       api.Wallet
}

var log = logging.Logger("f3")

func New(lc fx.Lifecycle, params F3Params) (*F3, error) {
	logging.SetLogLevel("f3", "DEBUG")
	manifest := f3.LocalnetManifest()
	manifest.NetworkName = gpbft.NetworkName(params.NetworkName)
	manifest.ECDelay = time.Duration(build.BlockDelaySecs) * time.Second
	manifest.ECPeriod = manifest.ECDelay
	manifest.BootstrapEpoch = 210
	manifest.ECFinality = 200
	ds := namespace.Wrap(params.Datastore, datastore.NewKey("/f3"))
	ec := &ecWrapper{
		ChainStore:   params.ChainStore,
		StateManager: params.StateManager,
		Manifest:     manifest,
	}
	verif := blssig.VerifierWithKeyOnG1()
	module, err := f3.New(context.TODO(), 1000 /*TODO expose signing*/, manifest, ds,
		params.Host, params.PubSub, &signer{params.Wallet}, verif, ec, log)

	if err != nil {
		return nil, xerrors.Errorf("creating F3: %w", err)
	}

	var liftimeContext context.Context
	var cancel func()
	lc.Append(fx.StartStopHook(
		func(ctx context.Context) {
			liftimeContext, cancel = context.WithCancel(ctx)
			go func() {
				err := module.Run(0, liftimeContext)
				if err != nil {
					log.Errorf("running f3: %+v", err)
				}
			}()
		}, func() {
			cancel()
		}))

	return &F3{
		f3: module,
	}, nil
}

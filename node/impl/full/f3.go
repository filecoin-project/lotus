package full

import (
	"context"
	"errors"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"

	"github.com/filecoin-project/lotus/chain/lf3"
)

type F3API struct {
	fx.In

	F3 *lf3.F3 `optional:"true"`
}

var ErrF3Disabled = errors.New("F3 is disabled")

func (f3api *F3API) F3Participate(ctx context.Context, miner address.Address) (<-chan error, error) {
	// make channel with some buffere to avoid blocking under higher load
	errCh := make(chan error, 4)

	if f3api.F3 == nil {
		log.Infof("F3Participate called for %v, F3 is disabled", miner)
		// we return a channel that will never be closed
		return errCh, nil
	}

	log.Infof("starting F3 participation for %v", miner)

	actorID, err := address.IDFromAddress(miner)
	if err != nil {
		return nil, xerrors.Errorf("miner address in F3Participate not of ID type: %w", err)
	}

	go func() {
		// Participate takes control of closing the channel
		f3api.F3.Participate(ctx, actorID, errCh)
	}()
	return errCh, nil
}

func (f3api *F3API) F3GetCertificate(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	if f3api.F3 == nil {
		return nil, ErrF3Disabled
	}
	return f3api.F3.Inner.GetCert(ctx, instance)
}

func (f3api *F3API) F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error) {
	if f3api.F3 == nil {
		return nil, ErrF3Disabled
	}
	return f3api.F3.Inner.GetLatestCert(ctx)
}

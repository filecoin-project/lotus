package full

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"

	"github.com/filecoin-project/lotus/chain/lf3"
)

type F3API struct {
	fx.In

	F3 *lf3.F3
}

func (f3api *F3API) F3Participate(ctx context.Context, miner address.Address) (<-chan error, error) {
	actorID, err := address.IDFromAddress(miner)
	if err != nil {
		return nil, xerrors.Errorf("miner address in F3Participate not of ID type: %w", err)
	}
	ch := make(chan error, 1)

	go func() {
		ch <- f3api.F3.Participate(ctx, actorID)
		close(ch)
	}()
	return ch, nil
}

func (f3api *F3API) F3GetCertificate(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	return f3api.F3.Inner.GetCert(ctx, instance)
}

func (f3api *F3API) F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error) {
	return f3api.F3.Inner.GetLatestCert(ctx)
}

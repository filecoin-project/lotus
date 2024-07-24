package full

import (
	"context"
	"errors"
	"time"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"

	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
)

type F3API struct {
	fx.In

	F3 *lf3.F3 `optional:"true"`
}

var ErrF3Disabled = errors.New("f3 is disabled")

func (f3api *F3API) F3Participate(ctx context.Context, miner address.Address,
	newLeaseExpiration time.Time, oldLeaseExpiration time.Time) (bool, error) {

	if leaseDuration := time.Until(newLeaseExpiration); leaseDuration > 5*time.Minute {
		return false, xerrors.Errorf("F3 participation lease too long: %v > 5 min", leaseDuration)
	} else if leaseDuration < 0 {
		return false, xerrors.Errorf("F3 participation lease is in the past: %d < 0", leaseDuration)
	}

	if f3api.F3 == nil {
		log.Infof("F3Participate called for %v, F3 is disabled", miner)
		return false, ErrF3Disabled
	}
	minerID, err := address.IDFromAddress(miner)
	if err != nil {
		return false, xerrors.Errorf("miner address is not of ID type: %v: %w", miner, err)
	}

	return f3api.F3.Participate(ctx, minerID, newLeaseExpiration, oldLeaseExpiration), nil
}

func (f3api *F3API) F3GetCertificate(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	if f3api.F3 == nil {
		return nil, ErrF3Disabled
	}
	return f3api.F3.GetCert(ctx, instance)
}

func (f3api *F3API) F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error) {
	if f3api.F3 == nil {
		return nil, ErrF3Disabled
	}
	return f3api.F3.GetLatestCert(ctx)
}
func (f3api *F3API) F3GetECPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	if f3api.F3 == nil {
		return nil, ErrF3Disabled
	}
	return f3api.F3.GetPowerTable(ctx, tsk)
}

func (f3api *F3API) F3GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	if f3api.F3 == nil {
		return nil, ErrF3Disabled
	}
	return f3api.F3.GetF3PowerTable(ctx, tsk)
}

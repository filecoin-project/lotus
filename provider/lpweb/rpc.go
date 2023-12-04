package lpweb

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"golang.org/x/xerrors"
)

type lprpc struct {
	db   *harmonydb.DB
	full api.FullNode
	cfg  *config.LotusProviderConfig
}

func (r *lprpc) SpIDs(ctx context.Context) ([]address.Address, error) {
	ca := r.cfg.Addresses.MinerAddresses
	out := make([]address.Address, len(ca))

	for i, s := range ca {
		a, err := address.NewFromString(s)
		if err != nil {
			return nil, xerrors.Errorf("parsing config address: %w", err)
		}

		out[i] = a
	}

	return out, nil
}

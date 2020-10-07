package modules

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/node/impl/full"

	"github.com/filecoin-project/lotus/chain/messagesigner"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-address"
)

// MpoolNonceAPI substitutes the mpool nonce with an implementation that
// doesn't rely on the mpool - it just gets the nonce from actor state
type MpoolNonceAPI struct {
	fx.In

	StateAPI full.StateAPI
}

// GetNonce gets the nonce from actor state
func (a *MpoolNonceAPI) GetNonce(addr address.Address) (uint64, error) {
	act, err := a.StateAPI.StateGetActor(context.Background(), addr, types.EmptyTSK)
	if err != nil {
		return 0, err
	}
	return act.Nonce, nil
}

var _ messagesigner.MpoolNonceAPI = (*MpoolNonceAPI)(nil)

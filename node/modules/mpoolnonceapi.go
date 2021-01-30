package modules

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

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

// GetNonce gets the nonce from current chain head.
func (a *MpoolNonceAPI) GetNonce(addr address.Address) (uint64, error) {
	ts := a.StateAPI.Chain.GetHeaviestTipSet()

	// make sure we have a key address so we can compare with messages
	keyAddr, err := a.StateAPI.StateManager.ResolveToKeyAddress(context.TODO(), addr, ts)
	if err != nil {
		return 0, err
	}

	// Load the last nonce from the state, if it exists.
	highestNonce := uint64(0)
	if baseActor, err := a.StateAPI.StateManager.LoadActorRaw(context.TODO(), addr, ts.ParentState()); err != nil {
		if !xerrors.Is(err, types.ErrActorNotFound) {
			return 0, err
		}
	} else {
		highestNonce = baseActor.Nonce
	}

	// Otherwise, find the highest nonce in the tipset.
	msgs, err := a.StateAPI.Chain.MessagesForTipset(ts)
	if err != nil {
		return 0, err
	}
	for _, msg := range msgs {
		vmmsg := msg.VMMessage()
		if vmmsg.From != keyAddr {
			continue
		}
		if vmmsg.Nonce >= highestNonce {
			highestNonce = vmmsg.Nonce + 1
		}
	}
	return highestNonce, nil
}

var _ messagesigner.MpoolNonceAPI = (*MpoolNonceAPI)(nil)

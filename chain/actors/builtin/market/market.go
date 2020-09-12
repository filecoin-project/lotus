package market

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/ipfs/go-cid"
)

func Load(store adt.Store, act *types.Actor) (st State, err error) {
	switch act.Code {
	case v0builtin.MarketActorCodeID:
		out := v0State{store: store}
		err := store.Get(store.Context(), act.Head, &out)
		if err != nil {
			return nil, err
		}
		return &out, nil
	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

type State interface {
	cbor.Marshaler
	EscrowTable() (BalanceTable, error)
	LockedTable() (BalanceTable, error)
	TotalLocked() (abi.TokenAmount, error)
}

type BalanceTable interface {
	Get(key address.Address) (abi.TokenAmount, error)
}

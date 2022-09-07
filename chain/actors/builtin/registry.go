package builtin

import (
	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account8 "github.com/filecoin-project/go-state-types/builtin/v8/account"
	account9 "github.com/filecoin-project/go-state-types/builtin/v9/account"
	"github.com/filecoin-project/go-state-types/cbor"
	rtt "github.com/filecoin-project/go-state-types/rt"

	"github.com/filecoin-project/lotus/chain/actors"
)

var _ rtt.VMActor = (*RegistryEntry)(nil)

type RegistryEntry struct {
	state   cbor.Er
	code    cid.Cid
	methods []interface{}
}

func (r RegistryEntry) State() cbor.Er {
	return r.state
}

func (r RegistryEntry) Exports() []interface{} {
	return r.methods
}

func (r RegistryEntry) Code() cid.Cid {
	return r.code
}

func MakeRegistry(av actorstypes.Version) []rtt.VMActor {
	if av < actorstypes.Version8 {
		panic("expected version v8 and up only, use specs-actors for v0-7")
	}
	registry := make([]rtt.VMActor, 0)

	codeIDs, err := actors.GetActorCodeIDs(av)
	if err != nil {
		panic(err)
	}

	switch av {

	case actorstypes.Version8:
		for key, codeID := range codeIDs {
			switch key {
			case actors.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account8.Methods,
					state:   new(account8.State),
				})
			}
		}

	case actorstypes.Version9:
		for key, codeID := range codeIDs {
			switch key {
			case actors.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account9.Methods,
					state:   new(account9.State),
				})
			}
		}

	default:
		panic("expected version v8 and up only, use specs-actors for v0-7")
	}

	return registry
}

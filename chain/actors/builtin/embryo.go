package builtin

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors"
)

func IsEmbryo(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == "embryo"
	}

	return false
}

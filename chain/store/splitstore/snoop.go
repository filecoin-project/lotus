package splitstore

import (
	cid "github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
)

type TrackingStore interface {
	Put(cid.Cid, abi.ChainEpoch) error
	PutBatch([]cid.Cid, abi.ChainEpoch) error
	Get(cid.Cid) (abi.ChainEpoch, error)
	Delete(cid.Cid) error
	Keys() (<-chan cid.Cid, error)
}

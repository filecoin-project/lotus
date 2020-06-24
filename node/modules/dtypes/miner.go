package dtypes

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type MinerAddress address.Address
type MinerID abi.ActorID

// AcceptingStorageDealsConfigFunc is a function which reads from miner config
// to determine if the user has disabled storage deals (or not).
type AcceptingStorageDealsConfigFunc func() (bool, error)

// SetAcceptingStorageDealsConfigFunc is a function which is used to disable or
// enable storage deal acceptance.
type SetAcceptingStorageDealsConfigFunc func(bool) error

// AcceptingRetrievalDealsConfigFunc is a function which reads from miner config
// to determine if the user has disabled retrieval acceptance (or not).
type AcceptingRetrievalDealsConfigFunc func() (bool, error)

// SetAcceptingRetrievalDealsConfigFunc is a function which is used to disable
// or enable retrieval deal acceptance.
type SetAcceptingRetrievalDealsConfigFunc func(bool) error

// StorageDealPieceCidBlocklistConfigFunc is a function which reads from miner config
// to obtain a list of CIDs for which the storage miner will not accept storage
// proposals.
type StorageDealPieceCidBlocklistConfigFunc func() ([]cid.Cid, error)

// SetStorageDealPieceCidBlocklistConfigFunc is a function which is used to set a
// list of CIDs for which the storage miner will reject deal proposals.
type SetStorageDealPieceCidBlocklistConfigFunc func([]cid.Cid) error

package dtypes

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type MinerAddress address.Address
type MinerID abi.ActorID

// AcceptingStorageDealsFunc is a function which reads from miner config to
// determine if the user has disabled storage deals (or not).
type AcceptingStorageDealsConfigFunc func() (bool, error)

// SetAcceptingStorageDealsFunc is a function which is used to disable or enable
// storage deal acceptance.
type SetAcceptingStorageDealsConfigFunc func(bool) error

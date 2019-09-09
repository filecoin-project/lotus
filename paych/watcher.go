package paych

import (
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/events"
	"github.com/filecoin-project/go-lotus/chain/types"
)

type watchedLane struct {
	bestVoucher *types.SignedVoucher
	closed bool
}

type inboundWatcher struct {
	ev *events.Events

	paych *address.Address

	control address.Address

	lanes map[uint64]*watchedLane

	closeAt uint64 // at what H do we plan to call close
	collectAt uint64 // at what H do we plan to call collect

}




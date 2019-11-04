package smm

import "github.com/filecoin-project/lotus/chain/types"

type NodeListener struct {

}
/*

1. Register with node or replace standalone storage miner?
2. Code structure:
    2.1 adapter to make sure the types are not 'bleeding' from one implementation to another
    2.2 listener for new tipset
3. GetChainRandomness requires tipset and ticket chain
    3.1 Ticket chain -> use MinTicket with a linked list and hash for fast lookup
    3.2
 */


func headChanges(revert, apply *[]types.TipSet) {
    // This is supposed to adapt to OnChainStateChanged
    // Adapt/compute the state for each tipset? Or is there a way to get it (without deadlocking)?
    // How do I get an instance of StorageMiningEvents? The notification doesn't allow for state -> singleton
    // See HeadChange in chain/messagepool.go, line 266
}


func Init() {
    // TODO:
    // TODO: Hook headChanges to SubscribeHeadChanges
}

func (listener NodeListener) OnChainStateChanged(epoch Epoch, stateID StateID) {
    // TODO: Use StateGetActor to get the actor state?
    return
}

package delegator

// This file sketches a future HAMT-backed state for the Delegator actor.
// It is intentionally a scaffold; methods return not-implemented errors until
// actor/FVM integration is implemented.

import (
    "context"
    "fmt"

    "github.com/ipfs/go-cid"
    "github.com/filecoin-project/go-address"
    "github.com/filecoin-project/lotus/chain/actors/adt"
)

// HamtState wraps a HAMT mapping of authority -> delegate 20-byte address.
type HamtState struct {
    Root cid.Cid
}

// LoadHamtState would load the mapping from store.
func LoadHamtState(ctx context.Context, store adt.Store, root cid.Cid) (*HamtState, error) {
    // TODO: replace with actual HAMT load
    return &HamtState{Root: root}, nil
}

// GetDelegate returns the 20-byte delegate address for the given authority, if present.
func (s *HamtState) GetDelegate(ctx context.Context, store adt.Store, authority address.Address) ([20]byte, bool, error) {
    // TODO: implement lookup in HAMT
    return [20]byte{}, false, fmt.Errorf("delegator hamt: not implemented")
}

// SetDelegate writes the 20-byte delegate address for the given authority and returns the new root.
func (s *HamtState) SetDelegate(ctx context.Context, store adt.Store, authority address.Address, delegate [20]byte) (cid.Cid, error) {
    // TODO: implement update in HAMT
    return cid.Undef, fmt.Errorf("delegator hamt: not implemented")
}


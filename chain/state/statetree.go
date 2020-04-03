package state

import (
	"context"
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"

	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("statetree")

// Stores actors state by their ID.
type StateTree struct {
	root  *hamt.Node
	Store cbor.IpldStore

	snaps *stateSnaps
}

type stateSnaps struct {
	layers []map[address.Address]streeOp
}

type streeOp struct {
	Act    types.Actor
	Delete bool
}

func newStateSnaps() *stateSnaps {
	return &stateSnaps{
		layers: []map[address.Address]streeOp{make(map[address.Address]streeOp)},
	}
}

func (ss *stateSnaps) addLayer() {
	ss.layers = append(ss.layers, make(map[address.Address]streeOp))
}

func (ss *stateSnaps) dropLayer() {
	ss.layers[len(ss.layers)-1] = nil // allow it to be GCed
	ss.layers = ss.layers[:len(ss.layers)-1]
}

func (ss *stateSnaps) mergeLastLayer() {
	last := ss.layers[len(ss.layers)-1]
	nextLast := ss.layers[len(ss.layers)-2]

	for k, v := range last {
		nextLast[k] = v
	}

	ss.dropLayer()
}

func (ss *stateSnaps) getActor(addr address.Address) (*types.Actor, error) {
	for i := len(ss.layers) - 1; i >= 0; i-- {
		act, ok := ss.layers[i][addr]
		if ok {
			if act.Delete {
				return nil, types.ErrActorNotFound
			}

			return &act.Act, nil
		}
	}
	return nil, nil
}

func (ss *stateSnaps) setActor(addr address.Address, act *types.Actor) {
	ss.layers[len(ss.layers)-1][addr] = streeOp{Act: *act}
}

func (ss *stateSnaps) deleteActor(addr address.Address) {
	ss.layers[len(ss.layers)-1][addr] = streeOp{Delete: true}
}

func NewStateTree(cst cbor.IpldStore) (*StateTree, error) {
	return &StateTree{
		root:  hamt.NewNode(cst, hamt.UseTreeBitWidth(5)),
		Store: cst,
		snaps: newStateSnaps(),
	}, nil
}

func LoadStateTree(cst cbor.IpldStore, c cid.Cid) (*StateTree, error) {
	nd, err := hamt.LoadNode(context.Background(), cst, c, hamt.UseTreeBitWidth(5))
	if err != nil {
		log.Errorf("loading hamt node %s failed: %s", c, err)
		return nil, err
	}

	return &StateTree{
		root:  nd,
		Store: cst,
		snaps: newStateSnaps(),
	}, nil
}

func (st *StateTree) SetActor(addr address.Address, act *types.Actor) error {
	iaddr, err := st.LookupID(addr)
	if err != nil {
		return xerrors.Errorf("ID lookup failed: %w", err)
	}
	addr = iaddr

	st.snaps.setActor(addr, act)
	return nil
}

// `LookupID` gets the ID address of this actor's `addr` stored in the `InitActor`.
func (st *StateTree) LookupID(addr address.Address) (address.Address, error) {
	if addr.Protocol() == address.ID {
		return addr, nil
	}

	act, err := st.GetActor(builtin.InitActorAddr)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting init actor: %w", err)
	}

	var ias init_.State
	if err := st.Store.Get(context.TODO(), act.Head, &ias); err != nil {
		return address.Undef, xerrors.Errorf("loading init actor state: %w", err)
	}

	a, err := ias.ResolveAddress(&AdtStore{st.Store}, addr)
	if err != nil {
		return address.Undef, xerrors.Errorf("resolve address %s: %w", addr, err)
	}
	return a, nil
}

// GetActor returns the actor from any type of `addr` provided.
func (st *StateTree) GetActor(addr address.Address) (*types.Actor, error) {
	if addr == address.Undef {
		return nil, fmt.Errorf("GetActor called on undefined address")
	}

	// Transform `addr` to its ID format.
	iaddr, err := st.LookupID(addr)
	if err != nil {
		if xerrors.Is(err, init_.ErrAddressNotFound) {
			return nil, xerrors.Errorf("resolution lookup failed (%s): %w", addr, err)
		}
		return nil, xerrors.Errorf("address resolution: %w", err)
	}
	addr = iaddr

	snapAct, err := st.snaps.getActor(addr)
	if err != nil {
		return nil, err
	}

	if snapAct != nil {
		return snapAct, nil
	}

	var act types.Actor
	err = st.root.Find(context.TODO(), string(addr.Bytes()), &act)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, types.ErrActorNotFound
		}
		return nil, xerrors.Errorf("hamt find failed: %w", err)
	}

	st.snaps.setActor(addr, &act)

	return &act, nil
}

func (st *StateTree) DeleteActor(addr address.Address) error {
	if addr == address.Undef {
		return xerrors.Errorf("DeleteActor called on undefined address")
	}

	iaddr, err := st.LookupID(addr)
	if err != nil {
		if xerrors.Is(err, init_.ErrAddressNotFound) {
			return xerrors.Errorf("resolution lookup failed (%s): %w", addr, err)
		}
		return xerrors.Errorf("address resolution: %w", err)
	}

	addr = iaddr

	_, err = st.GetActor(addr)
	if err != nil {
		return err
	}

	st.snaps.deleteActor(addr)

	return nil
}

func (st *StateTree) Flush(ctx context.Context) (cid.Cid, error) {
	ctx, span := trace.StartSpan(ctx, "stateTree.Flush")
	defer span.End()
	if len(st.snaps.layers) != 1 {
		return cid.Undef, xerrors.Errorf("tried to flush state tree with snapshots on the stack")
	}

	for addr, sto := range st.snaps.layers[0] {
		if sto.Delete {
			if err := st.root.Delete(ctx, string(addr.Bytes())); err != nil {
				return cid.Undef, err
			}
		} else {
			if err := st.root.Set(ctx, string(addr.Bytes()), &sto.Act); err != nil {
				return cid.Undef, err
			}
		}
	}

	if err := st.root.Flush(ctx); err != nil {
		return cid.Undef, err
	}

	return st.Store.Put(ctx, st.root)
}

func (st *StateTree) Snapshot(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "stateTree.SnapShot")
	defer span.End()

	st.snaps.addLayer()

	return nil
}

func (st *StateTree) ClearSnapshot() {
	st.snaps.mergeLastLayer()
}

func (st *StateTree) RegisterNewAddress(addr address.Address) (address.Address, error) {
	var out address.Address
	err := st.MutateActor(builtin.InitActorAddr, func(initact *types.Actor) error {
		var ias init_.State
		if err := st.Store.Get(context.TODO(), initact.Head, &ias); err != nil {
			return err
		}

		oaddr, err := ias.MapAddressToNewID(&AdtStore{st.Store}, addr)
		if err != nil {
			return err
		}
		out = oaddr

		ncid, err := st.Store.Put(context.TODO(), &ias)
		if err != nil {
			return err
		}

		initact.Head = ncid
		return nil
	})
	if err != nil {
		return address.Undef, err
	}

	return out, nil
}

type AdtStore struct{ cbor.IpldStore }

func (a *AdtStore) Context() context.Context {
	return context.TODO()
}

var _ adt.Store = (*AdtStore)(nil)

func (st *StateTree) Revert() error {
	st.snaps.dropLayer()
	st.snaps.addLayer()

	return nil
}

func (st *StateTree) MutateActor(addr address.Address, f func(*types.Actor) error) error {
	act, err := st.GetActor(addr)
	if err != nil {
		return err
	}

	if err := f(act); err != nil {
		return err
	}

	return st.SetActor(addr, act)
}

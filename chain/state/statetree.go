package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtin_types "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/network"
	states0 "github.com/filecoin-project/specs-actors/actors/states"
	states2 "github.com/filecoin-project/specs-actors/v2/actors/states"
	states3 "github.com/filecoin-project/specs-actors/v3/actors/states"
	states4 "github.com/filecoin-project/specs-actors/v4/actors/states"
	states5 "github.com/filecoin-project/specs-actors/v5/actors/states"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("statetree")

// StateTree stores actors state by their ID.
type StateTree struct {
	root        adt.Map
	version     types.StateTreeVersion
	info        cid.Cid
	Store       cbor.IpldStore
	lookupIDFun func(address.Address) (address.Address, error)

	snaps *stateSnaps
}

type stateSnaps struct {
	layers                        []*stateSnapLayer
	lastMaybeNonEmptyResolveCache int
}

type stateSnapLayer struct {
	actors       map[address.Address]streeOp
	resolveCache map[address.Address]address.Address
}

func newStateSnapLayer() *stateSnapLayer {
	return &stateSnapLayer{
		actors:       make(map[address.Address]streeOp),
		resolveCache: make(map[address.Address]address.Address),
	}
}

type streeOp struct {
	Act    types.Actor
	Delete bool
}

func newStateSnaps() *stateSnaps {
	ss := &stateSnaps{}
	ss.addLayer()
	return ss
}

func (ss *stateSnaps) addLayer() {
	ss.layers = append(ss.layers, newStateSnapLayer())
}

func (ss *stateSnaps) dropLayer() {
	ss.layers[len(ss.layers)-1] = nil // allow it to be GCed

	ss.layers = ss.layers[:len(ss.layers)-1]

	if ss.lastMaybeNonEmptyResolveCache == len(ss.layers) {
		ss.lastMaybeNonEmptyResolveCache = len(ss.layers) - 1
	}
}

func (ss *stateSnaps) mergeLastLayer() {
	last := ss.layers[len(ss.layers)-1]
	nextLast := ss.layers[len(ss.layers)-2]

	for k, v := range last.actors {
		nextLast.actors[k] = v
	}

	for k, v := range last.resolveCache {
		nextLast.resolveCache[k] = v
	}

	ss.dropLayer()
}

func (ss *stateSnaps) resolveAddress(addr address.Address) (address.Address, bool) {
	for i := ss.lastMaybeNonEmptyResolveCache; i >= 0; i-- {
		if len(ss.layers[i].resolveCache) == 0 {
			if ss.lastMaybeNonEmptyResolveCache == i {
				ss.lastMaybeNonEmptyResolveCache = i - 1
			}
			continue
		}
		resa, ok := ss.layers[i].resolveCache[addr]
		if ok {
			return resa, true
		}
	}
	return address.Undef, false
}

func (ss *stateSnaps) cacheResolveAddress(addr, resa address.Address) {
	ss.layers[len(ss.layers)-1].resolveCache[addr] = resa
	ss.lastMaybeNonEmptyResolveCache = len(ss.layers) - 1
}

func (ss *stateSnaps) getActor(addr address.Address) (*types.Actor, error) {
	for i := len(ss.layers) - 1; i >= 0; i-- {
		act, ok := ss.layers[i].actors[addr]
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
	ss.layers[len(ss.layers)-1].actors[addr] = streeOp{Act: *act}
}

func (ss *stateSnaps) deleteActor(addr address.Address) {
	ss.layers[len(ss.layers)-1].actors[addr] = streeOp{Delete: true}
}

// VersionForNetwork returns the state tree version for the given network
// version.
func VersionForNetwork(ver network.Version) (types.StateTreeVersion, error) {
	switch ver {
	case network.Version0, network.Version1, network.Version2, network.Version3:
		return types.StateTreeVersion0, nil
	case network.Version4, network.Version5, network.Version6, network.Version7, network.Version8, network.Version9:
		return types.StateTreeVersion1, nil
	case network.Version10, network.Version11:
		return types.StateTreeVersion2, nil
	case network.Version12:
		return types.StateTreeVersion3, nil

	case network.Version13, network.Version14, network.Version15, network.Version16, network.Version17:
		return types.StateTreeVersion4, nil

	case network.Version18, network.Version19, network.Version20, network.Version21, network.Version22, network.Version23, network.Version24, network.Version25, network.Version26, network.Version27, network.Version28:
		return types.StateTreeVersion5, nil

	default:
		panic(fmt.Sprintf("unsupported network version %d", ver))
	}
}

func NewStateTree(cst cbor.IpldStore, ver types.StateTreeVersion) (*StateTree, error) {
	var info cid.Cid
	switch ver {
	case types.StateTreeVersion0:
		// info is undefined
	case types.StateTreeVersion1, types.StateTreeVersion2, types.StateTreeVersion3, types.StateTreeVersion4, types.StateTreeVersion5:
		var err error
		info, err = cst.Put(context.TODO(), new(types.StateInfo0))
		if err != nil {
			return nil, err
		}
	default:
		return nil, xerrors.Errorf("unsupported state tree version: %d", ver)
	}

	store := adt.WrapStore(context.TODO(), cst)
	var hamt adt.Map
	switch ver {
	case types.StateTreeVersion0:
		tree, err := states0.NewTree(store)
		if err != nil {
			return nil, xerrors.Errorf("failed to create state tree: %w", err)
		}
		hamt = tree.Map
	case types.StateTreeVersion1:
		tree, err := states2.NewTree(store)
		if err != nil {
			return nil, xerrors.Errorf("failed to create state tree: %w", err)
		}
		hamt = tree.Map
	case types.StateTreeVersion2:
		tree, err := states3.NewTree(store)
		if err != nil {
			return nil, xerrors.Errorf("failed to create state tree: %w", err)
		}
		hamt = tree.Map
	case types.StateTreeVersion3:
		tree, err := states4.NewTree(store)
		if err != nil {
			return nil, xerrors.Errorf("failed to create state tree: %w", err)
		}
		hamt = tree.Map
	case types.StateTreeVersion4:
		tree, err := states5.NewTree(store)
		if err != nil {
			return nil, xerrors.Errorf("failed to create state tree: %w", err)
		}
		hamt = tree.Map
	case types.StateTreeVersion5:
		tree, err := builtin_types.NewTree(store)
		if err != nil {
			return nil, xerrors.Errorf("failed to create state tree: %w", err)
		}
		hamt = tree.Map

	default:
		return nil, xerrors.Errorf("unsupported state tree version: %d", ver)
	}

	s := &StateTree{
		root:    hamt,
		info:    info,
		version: ver,
		Store:   cst,
		snaps:   newStateSnaps(),
	}
	s.lookupIDFun = s.lookupInternalIDAddress
	return s, nil
}

func LoadStateTree(cst cbor.IpldStore, c cid.Cid) (*StateTree, error) {
	var root types.StateRoot
	// Try loading as a new-style state-tree (version/actors tuple).
	if err := cst.Get(context.TODO(), c, &root); err != nil {
		// We failed to decode as the new version, must be an old version.
		root.Actors = c
		root.Version = types.StateTreeVersion0
	}

	store := adt.WrapStore(context.TODO(), cst)

	var (
		hamt adt.Map
		err  error
	)
	switch root.Version {
	case types.StateTreeVersion0:
		var tree *states0.Tree
		tree, err = states0.LoadTree(store, root.Actors)
		if tree != nil {
			hamt = tree.Map
		}
	case types.StateTreeVersion1:
		var tree *states2.Tree
		tree, err = states2.LoadTree(store, root.Actors)
		if tree != nil {
			hamt = tree.Map
		}
	case types.StateTreeVersion2:
		var tree *states3.Tree
		tree, err = states3.LoadTree(store, root.Actors)
		if tree != nil {
			hamt = tree.Map
		}
	case types.StateTreeVersion3:
		var tree *states4.Tree
		tree, err = states4.LoadTree(store, root.Actors)
		if tree != nil {
			hamt = tree.Map
		}
	case types.StateTreeVersion4:
		var tree *states5.Tree
		tree, err = states5.LoadTree(store, root.Actors)
		if tree != nil {
			hamt = tree.Map
		}
	case types.StateTreeVersion5:
		var tree *builtin_types.ActorTree
		tree, err = builtin_types.LoadTree(store, root.Actors)
		if tree != nil {
			hamt = tree.Map
		}

	default:
		return nil, xerrors.Errorf("unsupported state tree version: %d", root.Version)
	}
	if err != nil {
		log.Debugf("failed to load state tree: %s", err)
		return nil, xerrors.Errorf("failed to load state tree %s: %w", c, err)
	}

	s := &StateTree{
		root:    hamt,
		info:    root.Info,
		version: root.Version,
		Store:   cst,
		snaps:   newStateSnaps(),
	}
	s.lookupIDFun = s.lookupInternalIDAddress

	return s, nil
}

func (st *StateTree) SetActor(addr address.Address, act *types.Actor) error {
	iaddr, err := st.LookupIDAddress(addr)
	if err != nil {
		return xerrors.Errorf("ID lookup failed: %w", err)
	}
	addr = iaddr

	st.snaps.setActor(addr, act)
	return nil
}

func (st *StateTree) lookupInternalIDAddress(addr address.Address) (address.Address, error) {
	act, err := st.GetActor(init_.Address)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting init actor: %w", err)
	}

	ias, err := init_.Load(&AdtStore{st.Store}, act)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading init actor state: %w", err)
	}

	a, found, err := ias.ResolveAddress(addr)
	if err == nil && !found {
		err = types.ErrActorNotFound
	}
	if err != nil {
		return address.Undef, xerrors.Errorf("resolve address %s: %w", addr, err)
	}
	return a, err
}

// LookupIDAddress gets the ID address of this actor's `addr` stored in the `InitActor`.
func (st *StateTree) LookupIDAddress(addr address.Address) (address.Address, error) {
	if addr.Protocol() == address.ID {
		return addr, nil
	}

	resa, ok := st.snaps.resolveAddress(addr)
	if ok {
		return resa, nil
	}
	a, err := st.lookupIDFun(addr)
	if err != nil {
		return a, err
	}

	st.snaps.cacheResolveAddress(addr, a)

	return a, nil
}

// GetActor returns the actor from any type of `addr` provided.
func (st *StateTree) GetActor(addr address.Address) (*types.Actor, error) {
	if addr == address.Undef {
		return nil, fmt.Errorf("GetActor called on undefined address")
	}

	// Transform `addr` to its ID format.
	iaddr, err := st.LookupIDAddress(addr)
	if err != nil {
		if errors.Is(err, types.ErrActorNotFound) {
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
	var found bool
	if st.version <= types.StateTreeVersion4 {
		var act4 types.ActorV4
		found, err = st.root.Get(abi.AddrKey(addr), &act4)
		if found {
			act = *types.AsActorV5(&act4)
		}
	} else {
		found, err = st.root.Get(abi.AddrKey(addr), &act)
	}
	if err != nil {
		return nil, xerrors.Errorf("hamt find failed: %w", err)
	} else if !found {
		return nil, types.ErrActorNotFound
	}

	st.snaps.setActor(addr, &act)

	return &act, nil
}

func (st *StateTree) DeleteActor(addr address.Address) error {
	if addr == address.Undef {
		return xerrors.Errorf("DeleteActor called on undefined address")
	}

	iaddr, err := st.LookupIDAddress(addr)
	if err != nil {
		if errors.Is(err, types.ErrActorNotFound) {
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
	ctx, span := trace.StartSpan(ctx, "stateTree.Flush") //nolint:staticcheck
	defer span.End()
	if len(st.snaps.layers) != 1 {
		return cid.Undef, xerrors.Errorf("tried to flush state tree with snapshots on the stack")
	}

	for addr, stoTmp := range st.snaps.layers[0].actors {
		sto := stoTmp
		if sto.Delete {
			if err := st.root.Delete(abi.AddrKey(addr)); err != nil {
				return cid.Undef, err
			}
		} else {
			if st.version <= types.StateTreeVersion4 {
				act4 := types.AsActorV4(&sto.Act)
				if err := st.root.Put(abi.AddrKey(addr), act4); err != nil {
					return cid.Undef, err
				}
			} else {
				if err := st.root.Put(abi.AddrKey(addr), &sto.Act); err != nil {
					return cid.Undef, err
				}
			}
		}
	}

	root, err := st.root.Root()
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to flush state-tree hamt: %w", err)
	}
	// If we're version 0, return a raw tree.
	if st.version == types.StateTreeVersion0 {
		return root, nil
	}
	// Otherwise, return a versioned tree.
	return st.Store.Put(ctx, &types.StateRoot{Version: st.version, Actors: root, Info: st.info})
}

func (st *StateTree) Snapshot(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "stateTree.SnapShot") //nolint:staticcheck
	defer span.End()

	st.snaps.addLayer()

	return nil
}

func (st *StateTree) ClearSnapshot() {
	st.snaps.mergeLastLayer()
}

func (st *StateTree) RegisterNewAddress(addr address.Address) (address.Address, error) {
	var out address.Address
	err := st.MutateActor(init_.Address, func(initact *types.Actor) error {
		ias, err := init_.Load(&AdtStore{st.Store}, initact)
		if err != nil {
			return err
		}

		oaddr, err := ias.MapAddressToNewID(addr)
		if err != nil {
			return err
		}
		out = oaddr

		ncid, err := st.Store.Put(context.TODO(), ias)
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

func (st *StateTree) ForEach(f func(address.Address, *types.Actor) error) error {
	// Walk through layers, if any.
	seen := make(map[address.Address]struct{})
	for i := len(st.snaps.layers) - 1; i >= 0; i-- {
		for addr, op := range st.snaps.layers[i].actors {
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			if op.Delete {
				continue
			}
			act := op.Act // copy
			if err := f(addr, &act); err != nil {
				return err
			}
		}

	}

	// Now walk through the saved actors.
	if st.version <= types.StateTreeVersion4 {
		var act types.ActorV4
		return st.root.ForEach(&act, func(k string) error {
			act := act // copy
			addr, err := address.NewFromBytes([]byte(k))
			if err != nil {
				return xerrors.Errorf("invalid address (%x) found in state tree key: %w", []byte(k), err)
			}

			// no need to record anything here, there are no duplicates in the actors HAMT
			// itself.
			if _, ok := seen[addr]; ok {
				return nil
			}

			return f(addr, types.AsActorV5(&act))
		})
	}

	var act types.Actor
	return st.root.ForEach(&act, func(k string) error {
		act := act // copy
		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return xerrors.Errorf("invalid address (%x) found in state tree key: %w", []byte(k), err)
		}

		// no need to record anything here, there are no duplicates in the actors HAMT
		// itself.
		if _, ok := seen[addr]; ok {
			return nil
		}

		return f(addr, &act)
	})
}

// Version returns the version of the StateTree data structure in use.
func (st *StateTree) Version() types.StateTreeVersion {
	return st.version
}

func Diff(ctx context.Context, oldTree, newTree *StateTree) (map[string]types.Actor, error) {
	out := map[string]types.Actor{}

	var (
		ncval, ocval cbg.Deferred
		buf          = bytes.NewReader(nil)
	)
	if err := newTree.root.ForEach(&ncval, func(k string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			addr, err := address.NewFromBytes([]byte(k))
			if err != nil {
				return xerrors.Errorf("address in state tree was not valid: %w", err)
			}

			found, err := oldTree.root.Get(abi.AddrKey(addr), &ocval)
			if err != nil {
				return err
			}

			if found && bytes.Equal(ocval.Raw, ncval.Raw) {
				return nil // not changed
			}

			if newTree.version <= types.StateTreeVersion4 {
				var act types.ActorV4

				buf.Reset(ncval.Raw)
				err = act.UnmarshalCBOR(buf)
				buf.Reset(nil)

				if err != nil {
					return err
				}

				out[addr.String()] = *types.AsActorV5(&act)

			} else {
				var act types.Actor

				buf.Reset(ncval.Raw)
				err = act.UnmarshalCBOR(buf)
				buf.Reset(nil)

				if err != nil {
					return err
				}

				out[addr.String()] = act
			}

			return nil
		}
	}); err != nil {
		return nil, err
	}
	return out, nil
}

package state

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"
	cbg "github.com/whyrusleeping/cbor-gen"
)

// Surgeon is an object used to fetch and manipulate state.
type Surgeon struct {
	ctx    context.Context
	api    api.FullNode
	stores *Stores
}

// NewSurgeon returns a state surgeon, an object used to fetch and manipulate
// state.
func NewSurgeon(ctx context.Context, api api.FullNode, stores *Stores) *Surgeon {
	return &Surgeon{
		ctx:    ctx,
		api:    api,
		stores: stores,
	}
}

// GetMaskedStateTree trims the state tree at the supplied tipset to contain
// only the state of the actors in the retain set. It also "dives" into some
// singleton system actors, like the init actor, to trim the state so as to
// compute a minimal state tree. In the future, thid method will dive into
// other system actors like the power actor and the market actor.
func (sg *Surgeon) GetMaskedStateTree(tsk types.TipSetKey, retain []address.Address) (cid.Cid, error) {
	// TODO: this will need to be parameterized on network version.
	stateTree, err := state.NewStateTree(sg.stores.CBORStore, builtin.Version0)
	if err != nil {
		return cid.Undef, err
	}

	initActor, initState, err := sg.loadInitActor(tsk)
	if err != nil {
		return cid.Undef, err
	}

	resolved, err := sg.resolveAddresses(retain, initState)
	if err != nil {
		return cid.Undef, err
	}

	initState, err = sg.retainInitEntries(initState, retain)
	if err != nil {
		return cid.Undef, err
	}

	err = sg.saveInitActor(initActor, initState, stateTree)
	if err != nil {
		return cid.Undef, err
	}

	err = sg.pluckActorStates(tsk, resolved, stateTree)
	if err != nil {
		return cid.Undef, err
	}

	root, err := stateTree.Flush(sg.ctx)
	if err != nil {
		return cid.Undef, err
	}

	return root, nil
}

// GetAccessedActors identifies the actors that were accessed during the
// execution of a message.
func (sg *Surgeon) GetAccessedActors(ctx context.Context, a api.FullNode, mid cid.Cid) ([]address.Address, error) {
	log.Printf("calculating accessed actors during execution of message: %s", mid)
	msgInfo, err := a.StateSearchMsg(ctx, mid)
	if err != nil {
		return nil, err
	}

	ts, err := a.ChainGetTipSet(ctx, msgInfo.TipSet)
	if err != nil {
		return nil, err
	}

	trace, err := a.StateReplay(ctx, ts.Parents(), mid)
	if err != nil {
		return nil, fmt.Errorf("could not replay msg: %w", err)
	}

	accessed := make(map[address.Address]struct{})
	var recur func(trace *types.ExecutionTrace)
	recur = func(trace *types.ExecutionTrace) {
		accessed[trace.Msg.To] = struct{}{}
		accessed[trace.Msg.From] = struct{}{}
		for _, s := range trace.Subcalls {
			recur(&s)
		}
	}

	recur(&trace.ExecutionTrace)

	ret := make([]address.Address, 0, len(accessed))
	for k := range accessed {
		ret = append(ret, k)
	}

	return ret, nil
}

// WriteCAR recursively writes the tree referenced by the root as a CAR into the
// supplied io.Writer.
func (sg *Surgeon) WriteCAR(w io.Writer, roots ...cid.Cid) error {
	carWalkFn := func(nd format.Node) (out []*format.Link, err error) {
		for _, link := range nd.Links() {
			if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
				continue
			}
			out = append(out, link)
		}
		return out, nil
	}
	return car.WriteCarWithWalker(sg.ctx, sg.stores.DAGService, roots, w, carWalkFn)
}

// pluckActorStates plucks the state from the supplied actors at the given
// tipset, and places it into the supplied state map.
func (sg *Surgeon) pluckActorStates(tsk types.TipSetKey, pluck []address.Address, stateTree *state.StateTree) error {
	for _, a := range pluck {
		actor, err := sg.api.StateGetActor(sg.ctx, a, tsk)
		if err != nil {
			return err
		}

		err = stateTree.SetActor(a, actor)
		if err != nil {
			return err
		}

		// recursive copy of the actor state so we can
		err = vm.Copy(sg.stores.Blockstore, sg.stores.Blockstore, actor.Head)
		if err != nil {
			return err
		}

		actorState, err := sg.api.ChainReadObj(sg.ctx, actor.Head)
		if err != nil {
			return err
		}

		cid, err := sg.stores.CBORStore.Put(sg.ctx, &cbg.Deferred{Raw: actorState})
		if err != nil {
			return err
		}

		if cid != actor.Head {
			panic("mismatched cids")
		}
	}

	return nil
}

// saveInitActor saves the state of the init actor to the provided state map.
func (sg *Surgeon) saveInitActor(initActor *types.Actor, initState init_.State, stateMap *state.StateTree) error {
	log.Printf("saving init actor into state tree")

	// Store the state of the init actor.
	cid, err := sg.stores.CBORStore.Put(sg.ctx, initState)
	if err != nil {
		return err
	}
	actor := *initActor
	actor.Head = cid

	err = stateMap.SetActor(init_.Address, &actor)
	if err != nil {
		return err
	}

	cid, _ = stateMap.Flush(sg.ctx)
	log.Printf("saved init actor into state tree; new root: %s", cid)
	return nil
}

// retainInitEntries takes an old init actor state, and retains only the
// entries in the retain set, returning a new init actor state.
func (sg *Surgeon) retainInitEntries(s init_.State, retain []address.Address) (init_.State, error) {
	log.Printf("retaining init actor entries for addresses: %v", retain)

	// TODO: The simplest approach would be to just add a "remove" method
	// (as below) to the init actor abstraction. However, I'm not sure if we
	// even _want_ this code. From what I can tell, lotus never removes
	// address in the init actor, even if the underlying actor disappears.

	/*
		toRetain := make(map[address.Address]struct{}, len(retain))
		for _, a := range retain {
			if a.Protocol() == address.ID {
				// skip over ID addresses; they don't need a mapping in the init actor.
				continue
			}
			toRetain[a] = struct{}{}
		}

		var toRemove []abi.ActorID
		if err := s.ForEachActor(func(id abi.ActorID, address address.Address) error {
			if _, found := toRetain[address]; found {
				return nil
			}
			toRemove = append(toRemove, id)
			return nil
		}); err != nil {
			return nil, err
		}
		// TODO: this function doesn't exist yet vvv
		if err := s.Remove(toRemove...); err != nil {
			return nil, err
		}
	*/

	log.Printf("new init actor state: %+v", s)
	return s, nil
}

// resolveAddresses resolved the requested addresses from the provided
// InitActor state, returning a slice of length len(orig), where each index
// contains the resolved address.
func (sg *Surgeon) resolveAddresses(orig []address.Address, ist init_.State) (ret []address.Address, err error) {
	log.Printf("resolving addresses: %v", orig)

	ret = make([]address.Address, len(orig))
	for i, addr := range orig {
		resolved, found, err := ist.ResolveAddress(addr)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("address not found: %s", addr)
		}
		ret[i] = resolved
	}

	log.Printf("resolved addresses: %v", ret)
	return ret, nil
}

// loadInitActor loads the init actor state from a given tipset.
func (sg *Surgeon) loadInitActor(tsk types.TipSetKey) (actor *types.Actor, initState init_.State, err error) {
	log.Printf("loading the init actor for tipset: %s", tsk)

	actor, err = sg.api.StateGetActor(sg.ctx, init_.Address, tsk)
	if err != nil {
		return nil, nil, err
	}

	initState, err = init_.Load(sg.stores.ADTStore, actor)
	if err != nil {
		return nil, nil, err
	}

	log.Printf("loaded init actor state: %+v", initState)

	return actor, initState, nil
}

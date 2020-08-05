package state

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"
	cbg "github.com/whyrusleeping/cbor-gen"
)

// Surgeon is an object used to fetch and manipulate state.
type Surgeon struct {
	ctx    context.Context
	api    api.FullNode
	stores *ProxyingStores
}

// NewSurgeon returns a state surgeon, an object used to fetch and manipulate
// state.
func NewSurgeon(ctx context.Context, api api.FullNode, stores *ProxyingStores) *Surgeon {
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
	stateMap := adt.MakeEmptyMap(sg.stores.ADTStore)

	initState, err := sg.loadInitActor(tsk)
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

	err = sg.saveInitActor(initState, stateMap)
	if err != nil {
		return cid.Undef, err
	}

	err = sg.pluckActorStates(tsk, resolved, stateMap)
	if err != nil {
		return cid.Undef, err
	}

	root, err := stateMap.Root()
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
func (sg *Surgeon) WriteCAR(w io.Writer, root cid.Cid) error {
	carWalkFn := func(nd format.Node) (out []*format.Link, err error) {
		for _, link := range nd.Links() {
			if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
				continue
			}
			out = append(out, link)
		}
		return out, nil
	}
	return car.WriteCarWithWalker(sg.ctx, sg.stores.DAGService, []cid.Cid{root}, w, carWalkFn)
}

// pluckActorStates plucks the state from the supplied actors at the given
// tipset, and places it into the supplied state map.
func (sg *Surgeon) pluckActorStates(tsk types.TipSetKey, pluck []address.Address, stateMap *adt.Map) error {
	for _, a := range pluck {
		actor, err := sg.api.StateGetActor(sg.ctx, a, tsk)
		if err != nil {
			return err
		}

		err = stateMap.Put(adt.AddrKey(a), actor)
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
func (sg *Surgeon) saveInitActor(initState *init_.State, stateMap *adt.Map) error {
	log.Printf("saving init actor into state tree")

	// Store the state of the init actor.
	cid, err := sg.stores.CBORStore.Put(sg.ctx, initState)
	if err != nil {
		return err
	}
	actor := &types.Actor{
		Code: builtin.InitActorCodeID,
		Head: cid,
	}

	err = stateMap.Put(adt.AddrKey(builtin.InitActorAddr), actor)
	if err != nil {
		return err
	}

	cid, _ = stateMap.Root()
	log.Printf("saved init actor into state tree; new root: %s", cid)
	return nil
}

// retainInitEntries takes an old init actor state, and retains only the
// entries in the retain set, returning a new init actor state.
func (sg *Surgeon) retainInitEntries(oldState *init_.State, retain []address.Address) (*init_.State, error) {
	log.Printf("retaining init actor entries for addresses: %v", retain)

	oldAddrs, err := adt.AsMap(sg.stores.ADTStore, oldState.AddressMap)
	if err != nil {
		return nil, err
	}

	newAddrs := adt.MakeEmptyMap(sg.stores.ADTStore)
	for _, r := range retain {
		if r.Protocol() == address.ID {
			// skip over ID addresses; they don't need a mapping in the init actor.
			continue
		}

		var d cbg.Deferred
		if _, err := oldAddrs.Get(adt.AddrKey(r), &d); err != nil {
			return nil, err
		}
		if err := newAddrs.Put(adt.AddrKey(r), &d); err != nil {
			return nil, err
		}
	}

	rootCid, err := newAddrs.Root()
	if err != nil {
		return nil, err
	}

	s := &init_.State{
		NetworkName: oldState.NetworkName,
		NextID:      oldState.NextID,
		AddressMap:  rootCid,
	}

	log.Printf("new init actor state: %+v", s)

	return s, nil
}

// resolveAddresses resolved the requested addresses from the provided
// InitActor state, returning a slice of length len(orig), where each index
// contains the resolved address.
func (sg *Surgeon) resolveAddresses(orig []address.Address, ist *init_.State) (ret []address.Address, err error) {
	log.Printf("resolving addresses: %v", orig)

	ret = make([]address.Address, len(orig))
	for i, addr := range orig {
		resolved, err := ist.ResolveAddress(sg.stores.ADTStore, addr)
		if err != nil {
			return nil, err
		}
		ret[i] = resolved
	}

	log.Printf("resolved addresses: %v", ret)
	return ret, nil
}

// loadInitActor loads the init actor state from a given tipset.
func (sg *Surgeon) loadInitActor(tsk types.TipSetKey) (initState *init_.State, err error) {
	log.Printf("loading the init actor for tipset: %s", tsk)

	actor, err := sg.api.StateGetActor(sg.ctx, builtin.InitActorAddr, tsk)
	if err != nil {
		return initState, err
	}

	actorState, err := sg.api.ChainReadObj(sg.ctx, actor.Head)
	if err != nil {
		return initState, err
	}

	initState = new(init_.State)
	err = initState.UnmarshalCBOR(bytes.NewReader(actorState))
	if err != nil {
		return initState, err
	}

	log.Printf("loaded init actor state: %+v", initState)

	return initState, nil
}

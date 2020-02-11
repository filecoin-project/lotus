package genesis

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func SetupInitActor(bs bstore.Blockstore, addrs []address.Address) (*types.Actor, error) {
	var ias actors.InitActorState
	ias.NextID = 100

	cst := cbor.NewCborStore(bs)
	amap := hamt.NewNode(cst)

	for i, a := range addrs {
		if err := amap.Set(context.TODO(), string(a.Bytes()), 100+uint64(i)); err != nil {
			return nil, err
		}
	}

	ias.NextID += uint64(len(addrs))
	if err := amap.Flush(context.TODO()); err != nil {
		return nil, err
	}
	amapcid, err := cst.Put(context.TODO(), amap)
	if err != nil {
		return nil, err
	}

	ias.AddressMap = amapcid

	statecid, err := cst.Put(context.TODO(), &ias)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code: actors.InitCodeCid,
		Head: statecid,
	}

	return act, nil
}

func AdjustInitActorStartID(ctx context.Context, bs blockstore.Blockstore, stateroot cid.Cid, val uint64) (cid.Cid, error) {
	cst := cbor.NewCborStore(bs)

	tree, err := state.LoadStateTree(cst, stateroot)
	if err != nil {
		return cid.Undef, err
	}

	act, err := tree.GetActor(actors.InitAddress)
	if err != nil {
		return cid.Undef, err
	}

	var st actors.InitActorState
	if err := cst.Get(ctx, act.Head, &st); err != nil {
		return cid.Undef, err
	}

	st.NextID = val

	nstate, err := cst.Put(ctx, &st)
	if err != nil {
		return cid.Undef, err
	}

	act.Head = nstate

	if err := tree.SetActor(actors.InitAddress, act); err != nil {
		return cid.Undef, err
	}

	return tree.Flush(ctx)
}

func initActorReassign(vm *vm.VM, cst cbor.IpldStore, from, to address.Address) error {
	ctx := context.TODO()
	initact, err := vm.StateTree().GetActor(actors.InitAddress)
	if err != nil {
		return xerrors.Errorf("couldnt get init actor: %w", err)
	}

	var st actors.InitActorState
	if err := cst.Get(ctx, initact.Head, &st); err != nil {
		return xerrors.Errorf("reassign loading init actor state: %w", err)
	}

	amap, err := hamt.LoadNode(ctx, cst, st.AddressMap)
	if err != nil {
		return xerrors.Errorf("failed to load init actor map: %w", err)
	}

	target, err := address.IDFromAddress(from)
	if err != nil {
		return xerrors.Errorf("failed to extract ID: %w", err)
	}

	var out string
	halt := xerrors.Errorf("halt")
	err = amap.ForEach(ctx, func(k string, v interface{}) error {
		_, val, err := cbg.CborReadHeader(bytes.NewReader(v.(*cbg.Deferred).Raw))
		if err != nil {
			return xerrors.Errorf("parsing int in map failed: %w", err)
		}

		if val == target {
			out = k
			return halt
		}
		return nil
	})

	if err == nil {
		return xerrors.Errorf("could not find from address in init ID map")
	}
	if !xerrors.Is(err, halt) {
		return xerrors.Errorf("finding address in ID map failed: %w", err)
	}

	if err := amap.Delete(ctx, out); err != nil {
		return xerrors.Errorf("deleting 'from' entry in amap: %w", err)
	}

	if err := amap.Set(ctx, out, target); err != nil {
		return xerrors.Errorf("setting 'to' entry in amap: %w", err)
	}

	if err := amap.Flush(ctx); err != nil {
		return xerrors.Errorf("failed to flush amap: %w", err)
	}

	ncid, err := cst.Put(ctx, amap)
	if err != nil {
		return err
	}

	st.AddressMap = ncid

	nacthead, err := cst.Put(ctx, &st)
	if err != nil {
		return err
	}

	initact.Head = nacthead

	return nil
}

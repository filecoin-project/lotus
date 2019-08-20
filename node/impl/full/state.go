package full

import (
	"context"
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"

	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"go.uber.org/fx"
)

type StateAPI struct {
	fx.In

	Chain *store.ChainStore
}

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address) ([]*api.SectorInfo, error) {
	ts := a.Chain.GetHeaviestTipSet()

	stc, err := a.Chain.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(a.Chain.Blockstore())

	st, err := state.LoadStateTree(cst, stc)
	if err != nil {
		return nil, err
	}

	act, err := st.GetActor(addr)
	if err != nil {
		return nil, err
	}

	var minerState actors.StorageMinerActorState
	if err := cst.Get(ctx, act.Head, &minerState); err != nil {
		return nil, err
	}

	nd, err := hamt.LoadNode(ctx, cst, minerState.Sectors)
	if err != nil {
		return nil, err
	}

	var sinfos []*api.SectorInfo
	// Note to self: the hamt isnt a great data structure to use here... need to implement the sector set
	err = nd.ForEach(ctx, func(k string, val interface{}) error {
		sid, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			return err
		}

		bval, ok := val.([]byte)
		if !ok {
			return fmt.Errorf("expected to get bytes in sector set hamt")
		}

		var comms [][]byte
		if err := cbor.DecodeInto(bval, &comms); err != nil {
			return err
		}

		sinfos = append(sinfos, &api.SectorInfo{
			SectorID: sid,
			CommR:    comms[0],
			CommD:    comms[1],
		})
		return nil
	})
	return sinfos, nil
}

func (a *StateAPI) StateMinerProvingSet(ctx context.Context, addr address.Address) ([]*api.SectorInfo, error) {
	ts := a.Chain.GetHeaviestTipSet()

	stc, err := a.Chain.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(a.Chain.Blockstore())

	st, err := state.LoadStateTree(cst, stc)
	if err != nil {
		return nil, err
	}

	act, err := st.GetActor(addr)
	if err != nil {
		return nil, err
	}

	var minerState actors.StorageMinerActorState
	if err := cst.Get(ctx, act.Head, &minerState); err != nil {
		return nil, err
	}

	nd, err := hamt.LoadNode(ctx, cst, minerState.ProvingSet)
	if err != nil {
		return nil, err
	}

	var sinfos []*api.SectorInfo
	// Note to self: the hamt isnt a great data structure to use here... need to implement the sector set
	err = nd.ForEach(ctx, func(k string, val interface{}) error {
		sid, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			return err
		}

		bval, ok := val.([]byte)
		if !ok {
			return fmt.Errorf("expected to get bytes in sector set hamt")
		}

		var comms [][]byte
		if err := cbor.DecodeInto(bval, &comms); err != nil {
			return err
		}

		sinfos = append(sinfos, &api.SectorInfo{
			SectorID: sid,
			CommR:    comms[0],
			CommD:    comms[1],
		})
		return nil
	})
	return sinfos, nil
}

package validation

/*
import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	vstate "github.com/filecoin-project/chain-validation/state"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ vstate.VMWrapper = &StateWrapper{}

type StateWrapper struct {
	// The blockstore underlying the state tree and storage.
	bs blockstore.Blockstore

	ds datastore.Batching
	// HAMT-CBOR store on top of the blockstore.
	cst cbor.IpldStore

	// CID of the root of the state tree.
	stateRoot cid.Cid
}

func NewState() *StateWrapper {
	bs := blockstore.NewBlockstore(datastore.NewMapDatastore())
	cst := cbor.NewCborStore(bs)
	// Put EmptyObjectCid value in the store. When an actor is initially created its Head is set to this value.
	_, err := cst.Put(context.TODO(), map[string]string{})
	if err != nil {
		panic(err)
	}

	treeImpl, err := state.NewStateTree(cst)
	if err != nil {
		panic(err) // Never returns error, the error return should be removed.
	}
	root, err := treeImpl.Flush(context.TODO())
	if err != nil {
		panic(err)
	}
	return &StateWrapper{
		bs:        bs,
		ds:        datastore.NewMapDatastore(),
		cst:       cst,
		stateRoot: root,
	}
}

func (s *StateWrapper) Root() cid.Cid {
	return s.stateRoot
}

func (s *StateWrapper) Store() adt.Store {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		panic(err)
	}
	return &contextStore{tree.Store, context.Background()}
}

func (s *StateWrapper) Actor(addr address.Address) (vstate.Actor, error) {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, err
	}
	fcActor, err := tree.GetActor(addr)
	if err != nil {
		return nil, err
	}
	return &actorWrapper{*fcActor}, nil
}

func (s *StateWrapper) SetActorState(addr address.Address, balance abi.TokenAmount, actorState runtime.CBORMarshaler) (vstate.Actor, error) {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, err
	}
	// actor should exist
	act, err := tree.GetActor(addr)
	if err != nil {
		return nil, err
	}
	// add the state to the store and get a new head cid
	actHead, err := tree.Store.Put(context.Background(), actorState)
	if err != nil {
		return nil, err
	}
	// update the actor object with new head and balance parameter
	actr := &actorWrapper{types.Actor{
		Code:  act.Code,
		Nonce: act.Nonce,
		// updates
		Head:    actHead,
		Balance: balance,
	}}
	if err := tree.SetActor(addr, &actr.Actor); err != nil {
		return nil, err
	}
	return actr, s.flush(tree)
}

func (s *StateWrapper) CreateActor(code cid.Cid, addr address.Address, balance abi.TokenAmount, actorState runtime.CBORMarshaler) (vstate.Actor, address.Address, error) {
	idAddr := addr
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, address.Undef, err
	}
	if addr.Protocol() != address.ID {

		actHead, err := tree.Store.Put(context.Background(), actorState)
		if err != nil {
			return nil, address.Undef, err
		}
		actr := &actorWrapper{types.Actor{
			Code:    code,
			Head:    actHead,
			Balance: balance,
		}}

		idAddr, err = tree.RegisterNewAddress(addr)
		if err != nil {
			return nil, address.Undef, xerrors.Errorf("register new address for actor: %w", err)
		}

		if err := tree.SetActor(addr, &actr.Actor); err != nil {
			return nil, address.Undef, xerrors.Errorf("setting new actor for actor: %w", err)
		}
	}

	// store newState
	head, err := tree.Store.Put(context.Background(), actorState)
	if err != nil {
		return nil, address.Undef, err
	}

	// create and store actor object
	a := types.Actor{
		Code:    code,
		Head:    head,
		Balance: balance,
	}
	if err := tree.SetActor(idAddr, &a); err != nil {
		return nil, address.Undef, err
	}

	return &actorWrapper{a}, idAddr, s.flush(tree)
}

// Flushes a state tree to storage and sets this state's root to that tree's root CID.
func (s *StateWrapper) flush(tree *state.StateTree) (err error) {
	s.stateRoot, err = tree.Flush(context.TODO())
	return
}

//
// Actor Wrapper
//

type actorWrapper struct {
	types.Actor
}

func (a *actorWrapper) Code() cid.Cid {
	return a.Actor.Code
}

func (a *actorWrapper) Head() cid.Cid {
	return a.Actor.Head
}

func (a *actorWrapper) CallSeqNum() uint64 {
	return a.Actor.Nonce
}

func (a *actorWrapper) Balance() big.Int {
	return a.Actor.Balance

}

//
// Storage
//

type contextStore struct {
	cbor.IpldStore
	ctx context.Context
}

func (s *contextStore) Context() context.Context {
	return s.ctx
}
*/

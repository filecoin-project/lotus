package validation

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin"
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

	"github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ vstate.VMWrapper = &StateWrapper{}

type StateWrapper struct {
	// The blockstore underlying the state tree and storage.
	bs blockstore.Blockstore
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
	return &StateWrapper{bs, cst, root}
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
	if addr == builtin.InitActorAddr || addr == builtin.StoragePowerActorAddr || addr == builtin.StorageMarketActorAddr {
		act, err := s.SetupSingletonActor(addr)
		if err != nil {
			return nil, address.Undef, err
		}
		return act, addr, nil
	}
	tree, err := state.LoadStateTree(s.cst, s.Root())
	if err != nil {
		return nil, address.Undef, err
	}
	actHead, err := tree.Store.Put(context.Background(), actorState)
	if err != nil {
		return nil, address.Undef, err
	}
	actr := &actorWrapper{types.Actor{
		Code:    code,
		Head:    actHead,
		Balance: balance,
	}}
	idAddr, err := tree.RegisterNewAddress(addr, &actr.Actor)
	if err != nil {
		return nil, address.Undef, xerrors.Errorf("register new address for actor: %w", err)
	}
	return actr, idAddr, s.flush(tree)
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

func (a *actorWrapper) CallSeqNum() int64 {
	return int64(a.Actor.Nonce)
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

func (s *StateWrapper) SetupSingletonActor(addr address.Address) (vstate.Actor, error) {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, err
	}
	switch addr {
	case builtin.InitActorAddr:
		// FIXME this is going to be a problem if go-filecoin and lotus setup their init actors with different netnames
		// ideally lotus should use the init actor constructor
		initact, err := genesis.SetupInitActor(s.bs, "chain-validation", nil)
		if err != nil {
			return nil, xerrors.Errorf("setup init actor: %w", err)
		}
		if err := tree.SetActor(builtin.InitActorAddr, initact); err != nil {
			return nil, xerrors.Errorf("set init actor: %w", err)
		}

		return &actorWrapper{*initact}, s.flush(tree)
	case builtin.StorageMarketActorAddr:
		smact, err := genesis.SetupStorageMarketActor(s.bs)
		if err != nil {
			return nil, xerrors.Errorf("setup storage marker actor: %w", err)
		}

		if err := tree.SetActor(builtin.StorageMarketActorAddr, smact); err != nil {
			return nil, xerrors.Errorf("set storage marker actor: %w", err)
		}

		return &actorWrapper{*smact}, s.flush(tree)
	case builtin.StoragePowerActorAddr:
		spact, err := genesis.SetupStoragePowerActor(s.bs)
		if err != nil {
			return nil, xerrors.Errorf("setup storage power actor: %w", err)
		}
		if err := tree.SetActor(builtin.StoragePowerActorAddr, spact); err != nil {
			return nil, xerrors.Errorf("set storage power actor: %w", err)
		}
		return &actorWrapper{*spact}, s.flush(tree)
	default:
		return nil, xerrors.Errorf("%v is not a singleton actor address", addr)
	}
}

/*
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
	storageImpl := &directStorage{cst}
	return &StateWrapper{bs, cst, newKeyStore(), root, storageImpl}
}

func (s *StateWrapper) Cid() cid.Cid {
	return s.stateRoot
}

func (s *StateWrapper) Actor(addr address.Address) (vstate.Actor, error) {
	vaddr, err := address.NewFromBytes(addr.Bytes())
	if err != nil {
		return nil, err
	}
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, err
	}
	fcActor, err := tree.GetActor(vaddr)
	if err != nil {
		return nil, err
	}
	return &actorWrapper{*fcActor}, nil
}

func (s *StateWrapper) Storage(addr address.Address) (vstate.Storage, error) {
	return s.storage, nil
}

func (s *StateWrapper) NewAccountAddress() (address.Address, error) {
	return s.keys.NewAddress()
}

func (s *StateWrapper) SetActor(addr address.Address, code vactors.ActorCodeID, balance vtypes.BigInt) (vstate.Actor, vstate.Storage, error) {
	addrInt, err := address.NewFromBytes(addr.Bytes())
	if err != nil {
		return nil, nil, err
	}
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, nil, err
	}
	actr := &actorWrapper{types.Actor{
		Code:    fromActorCode(code),
		Balance: types.BigInt{balance.Int},
		Head:    vm.EmptyObjectCid,
	}}
	// The ID-based address is dropped here, but should be reported back to the caller.
	_, err = tree.RegisterNewAddress(addrInt, &actr.Actor)
	if err != nil {
		return nil, nil, xerrors.Errorf("register new address for actor: %w", err)
	}
	return actr, s.storage, s.flush(tree)
}

func (s *StateWrapper) SetSingletonActor(addr vactors.SingletonActorID, balance vtypes.BigInt) (vstate.Actor, vstate.Storage, error) {
	vaddr := fromSingletonAddress(addr)

	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, nil, err
	}

	lotusAddr, err := address.NewFromBytes(vaddr.Bytes())
	if err != nil {
		return nil, nil, err
	}

	switch lotusAddr {
	case actors.InitAddress:
		initact, err := genesis.SetupInitActor(s.bs, "testing", nil)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.InitAddress, initact); err != nil {
			return nil, nil, xerrors.Errorf("set init actor: %w", err)
		}

		return &actorWrapper{*initact}, s.storage, s.flush(tree)
	case actors.StorageMarketAddress:
		nsroot, err := genesis.SetupStorageMarketActor(s.bs)
		if err != nil {
			return nil, nil, err
		}
		s.stateRoot = nsroot

		tree, err = state.LoadStateTree(s.cst, s.stateRoot)
		if err != nil {
			return nil, nil, err
		}
		smact, err := tree.GetActor(actors.StorageMarketAddress)
		if err != nil {
			return nil, nil, err
		}
		return &actorWrapper{*smact}, s.storage, s.flush(tree)
	case actors.StoragePowerAddress:
		spact, err := genesis.SetupStoragePowerActor(s.bs)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.StoragePowerAddress, spact); err != nil {
			return nil, nil, xerrors.Errorf("set network storage market actor: %w", err)
		}
		return &actorWrapper{*spact}, s.storage, s.flush(tree)
	case actors.NetworkAddress:
		ntwkact := &types.Actor{
			Code:    actors.AccountCodeCid,
			Balance: types.BigInt{balance.Int},
			Head:    vm.EmptyObjectCid,
		}
		if err := tree.SetActor(actors.NetworkAddress, ntwkact); err != nil {
			return nil, nil, xerrors.Errorf("set network actor: %w", err)
		}
		return &actorWrapper{*ntwkact}, s.storage, s.flush(tree)
	case actors.BurntFundsAddress:
		ntwkact := &types.Actor{
			Code:    actors.AccountCodeCid,
			Balance: types.BigInt{balance.Int},
			Head:    vm.EmptyObjectCid,
		}
		if err := tree.SetActor(actors.BurntFundsAddress, ntwkact); err != nil {
			return nil, nil, xerrors.Errorf("set network actor: %w", err)
		}
		return &actorWrapper{*ntwkact}, s.storage, s.flush(tree)
	default:
		return nil, nil, xerrors.Errorf("%v is not a singleton actor address", addr)
	}
}

func (s *StateWrapper) Sign(ctx context.Context, addr address.Address, data []byte) (*vtypes.Signature, error) {
	sig, err := s.keys.Sign(ctx, addr, data)
	if err != nil {
		return nil, err
	}
	return &vtypes.Signature{
		Type: sig.Type,
		Data: sig.Data,
	}, nil
}

func (s *StateWrapper) Signer() *keyStore {
	return s.keys
}

// Flushes a state tree to storage and sets this state's root to that tree's root CID.
func (s *StateWrapper) flush(tree *state.StateTree) (err error) {
	s.stateRoot, err = tree.Flush(context.TODO())
	return
}

//
// Key store
//
type keyStore struct {
	// Private keys by address
	keys map[address.Address]vtypes.KeyInfo
	// Seed for deterministic key generation.
	seed int64
}

func newKeyStore() *keyStore {
	return &keyStore{
		keys: make(map[address.Address]vtypes.KeyInfo),
		seed: 0,
	}
}

func (s *keyStore) NewAddress() (address.Address, error) {
	randSrc := rand.New(rand.NewSource(s.seed))
	prv, err := crypto.GenerateKeyFromSeed(randSrc)
	if err != nil {
		return address.Undef, err
	}

	vki := vtypes.KeyInfo{
		PrivateKey: prv,
		Type:       types.KTSecp256k1,
	}
	key, err := wallet.NewKey(types.KeyInfo{
		Type:       vki.Type,
		PrivateKey: vki.PrivateKey,
	})
	if err != nil {
		return address.Undef, err
	}
	vaddr, err := address.NewFromBytes(key.Address.Bytes())
	if err != nil {
		return address.Undef, err
	}
	s.keys[vaddr] = vki
	s.seed++
	return address.NewFromBytes(key.Address.Bytes())
}

func (s *keyStore) Sign(ctx context.Context, addr address.Address, data []byte) (*types.Signature, error) {
	ki, ok := s.keys[addr]
	if !ok {
		return &types.Signature{}, fmt.Errorf("unknown address %v", addr)
	}
	b2sum := blake2b.Sum256(data)
	digest, err := crypto.Sign(ki.PrivateKey, b2sum[:])
	if err != nil {
		return &types.Signature{}, err
	}
	return &types.Signature{
		Type: types.KTSecp256k1,
		Data: digest,
	}, nil
}


//
// Storage
//

type directStorage struct {
	cst cbor.IpldStore
}

func (d *directStorage) Get(c cid.Cid, out interface{}) error {
	if err := d.cst.Get(context.TODO(), c, out.(cbg.CBORUnmarshaler)); err != nil {
		return err
	}
	return nil
}

*/

package validation

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/pkg/errors"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/vm"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/crypto"

	vstate "github.com/filecoin-project/chain-validation/pkg/state"
)

type StateWrapper struct {
	// The blockstore underlying the state tree and storage.
	bs blockstore.Blockstore
	// HAMT-CBOR store on top of the blockstore.
	cst *hamt.CborIpldStore
	// A store for encryption keys.
	keys *keyStore

	// CID of the root of the state tree.
	stateRoot cid.Cid
	// The root node of the state tree, essentially a cache of LoadStateTree(cst, stateRoot)
	//tree *state.StateTree
	// A look-through storage implementation.
	storage *directStorage
}

var _ vstate.Wrapper = &StateWrapper{}

func NewState() *StateWrapper {
	bs := blockstore.NewBlockstore(datastore.NewMapDatastore())
	cst := hamt.CSTFromBstore(bs)
	// Put EmptyObjectCid value in the store. When an actor is initially created its Head is set to this value.
	// Normally this happens in chain/vm/mkactor.go's init function. Without this flushing the state tree fails as this
	// CID is not found in the store.
	_, err := cst.Put(context.TODO(), map[string]string{})
	if err != nil {
		panic(err)
	}

	treeImpl, err := state.NewStateTree(cst)
	if err != nil {
		panic(err) // Never returns error, the error return should be removed.
	}
	root, err := treeImpl.Flush()
	if err != nil {
		panic(err)
	}
	storageImpl := &directStorage{cst}
	return &StateWrapper{bs, cst, newKeyStore(), root, storageImpl}
}

func (s *StateWrapper) Cid() cid.Cid {
	return s.stateRoot
}

func (s *StateWrapper) Actor(addr vstate.Address) (vstate.Actor, error) {
	vaddr, err := address.NewFromBytes([]byte(addr))
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

func (s *StateWrapper) Storage(addr vstate.Address) (vstate.Storage, error) {
	return s.storage, nil
}

func (s *StateWrapper) NewAccountAddress() (vstate.Address, error) {
	return s.keys.NewAddress()
}

func (s *StateWrapper) SetActor(addr vstate.Address, code cid.Cid, balance vstate.AttoFIL) (vstate.Actor, vstate.Storage, error) {
	addrInt, err := address.NewFromBytes([]byte(addr))
	if err != nil {
		return nil, nil, err
	}
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, nil, err
	}
	// singleton actors get special handling
	switch addrInt {
	case actors.InitActorAddress:
		initact, err := gen.SetupInitActor(s.bs, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.InitActorAddress, initact); err != nil {
			return nil, nil, errors.Wrapf(err, "set init actor")
		}

		return &actorWrapper{*initact}, s.storage, s.flush(tree)
	case actors.StorageMarketAddress:
		smact, err := gen.SetupStorageMarketActor(s.bs)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.StorageMarketAddress, smact); err != nil {
			return nil, nil, errors.Wrapf(err, "set network storage market actor")
		}
		return &actorWrapper{*smact}, s.storage, s.flush(tree)
	case actors.NetworkAddress:
		ntwkact := &types.Actor{
			Code:    code,
			Balance: types.BigInt{balance},
			Head:    vm.EmptyObjectCid,
		}
		if err := tree.SetActor(actors.NetworkAddress, ntwkact); err != nil {
			return nil, nil, errors.Wrapf(err, "set network actor")
		}
		return &actorWrapper{*ntwkact}, s.storage, s.flush(tree)
	default:
		actr := &actorWrapper{types.Actor{
			Code:    code,
			Balance: types.BigInt{balance},
			Head:    vm.EmptyObjectCid,
		}}
		// The ID-based address is dropped here, but should be reported back to the caller.
		_, err = tree.RegisterNewAddress(addrInt, &actr.Actor)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "register new address for actor")
		}
		return actr, s.storage, s.flush(tree)
	}
}

func (s *StateWrapper) Signer() *keyStore {
	return s.keys
}

// Flushes a state tree to storage and sets this state's root to that tree's root CID.
func (s *StateWrapper) flush(tree *state.StateTree) (err error) {
	s.stateRoot, err = tree.Flush()
	return
}

//
// Key store
//
type keyStore struct {
	// Private keys by address
	keys map[address.Address]types.KeyInfo
	// Seed for deterministic key generation.
	seed int64
}

func newKeyStore() *keyStore {
	return &keyStore{
		keys: make(map[address.Address]types.KeyInfo),
		seed: 0,
	}
}

func (s *keyStore) NewAddress() (vstate.Address, error) {
	randSrc := rand.New(rand.NewSource(s.seed))
	prv, err := crypto.GenerateKeyFromSeed(randSrc)
	if err != nil {
		return "", err
	}

	ki := types.KeyInfo{
		PrivateKey: prv,
		Type:       types.KTSecp256k1,
	}
	key, err := wallet.NewKey(ki)
	if err != nil {
		return "", err
	}
	s.keys[key.Address] = ki
	s.seed++
	return vstate.Address(key.Address.Bytes()), nil
}

func (s *keyStore) Sign(ctx context.Context, addr address.Address, data []byte) (*types.Signature, error) {
	ki, ok := s.keys[addr]
	if !ok {
		return &types.Signature{}, fmt.Errorf("unknown address %v", addr)
	}
	digest, err := crypto.Sign(ki.PrivateKey, data)
	if err != nil {
		return &types.Signature{}, err
	}
	return &types.Signature{
		Type: types.KTSecp256k1,
		Data: digest,
	}, nil
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

func (a *actorWrapper) Nonce() uint64 {
	return a.Actor.Nonce
}

func (a *actorWrapper) Balance() vstate.AttoFIL {
	return a.Actor.Balance.Int

}

//
// Storage
//

type directStorage struct {
	cst *hamt.CborIpldStore
}

func (d *directStorage) Get(cid cid.Cid) ([]byte, error) {
	panic("implement me")
}

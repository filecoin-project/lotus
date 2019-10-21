package validation

import (
	"fmt"
	"math/rand"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-lotus/lib/crypto"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/wallet"

	vstate "github.com/filecoin-project/chain-validation/pkg/state"
)
type StateFactory struct {
	keys *keyStore
}

var _ vstate.Factory = &StateFactory{}

func NewStateFactory() *StateFactory {
	return &StateFactory{newKeyStore()}
}

func (s *StateFactory) Signer() *keyStore {
	return s.keys
}

func (s StateFactory) NewAddress() (vstate.Address, error) {
	return s.keys.NewAddress()
}

func (s StateFactory) NewActor(code cid.Cid, balance vstate.AttoFIL) vstate.Actor {
	return &actorWrapper{types.Actor{
		Code:    code,
		Balance: types.BigInt{balance},
	}}
}

func (s StateFactory) NewState(actors []vstate.ActorAndAddress) (vstate.Tree, vstate.StorageMap, error) {
	bs := blockstore.NewBlockstore(datastore.NewMapDatastore())
	cst := hamt.CSTFromBstore(bs)
	// TODO it might be better to use gen.MakeInitialStateTree() here, not sure.
	// more or less following what go-filecoin driver does for the time being.
	treeImpl, err := state.NewStateTree(cst)
	if err != nil {
		return nil, nil, err
	}

	for _, a := range actors {
		actAddr, err := address.NewFromBytes([]byte(a.Address))
		if err != nil {
			return nil, nil, err
		}
		actw := a.Actor.(*actorWrapper)
		if err := treeImpl.SetActor(actAddr, &actw.Actor); err != nil {
			return nil, nil, err
		}
	}

	_, err = treeImpl.Flush()
	if err != nil {
		return nil, nil, err
	}
	return &stateTreeWrapper{treeImpl}, nil, nil
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
		Type:      types.KTSecp256k1,
	}
	key, err := wallet.NewKey(ki)
	if err != nil {
		return "", err
	}
	s.keys[key.Address] = ki
	s.seed++
	return vstate.Address(key.Address.Bytes()), nil
}

func (as *keyStore) Sign(addr address.Address, data []byte) (types.Signature, error) {
	ki, ok := as.keys[addr]
	if !ok {
		return types.Signature{}, fmt.Errorf("unknown address %v", addr)
	}
	digest, err := crypto.Sign(ki.PrivateKey, data)
	if err != nil {
		return types.Signature{}, err
	}
	return types.Signature{
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
// State Tree Wrapper
//

type stateTreeWrapper struct {
	*state.StateTree
}

func (s *stateTreeWrapper) Actor(addr vstate.Address) (vstate.Actor, error) {
	vaddr, err := address.NewFromBytes([]byte(addr))
	if err != nil {
		return nil, err
	}
	fcActor, err := s.GetActor(vaddr)
	if err != nil {
		return nil, err
	}
	return &actorWrapper{*fcActor}, nil
}

func (s *stateTreeWrapper) Cid() cid.Cid {
	panic("implement me")
}

func (s *stateTreeWrapper) ActorStorage(vstate.Address) (vstate.Storage, error) {
	panic("implement me")
}



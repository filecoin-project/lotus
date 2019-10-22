package validation

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/pkg/errors"
	"math/rand"

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
	bs   blockstore.Blockstore
	keys *keyStore

	*state.StateTree
	*directStorage
}

var _ vstate.Wrapper = &StateWrapper{}

func NewState() *StateWrapper {
	bs := blockstore.NewBlockstore(datastore.NewMapDatastore())
	cst := hamt.CSTFromBstore(bs)
	treeImpl, err := state.NewStateTree(cst)
	if err != nil {
		panic(err) // Never returns error, the error return should be removed.
	}
	storageImpl := &directStorage{cst}
	return &StateWrapper{bs, newKeyStore(), treeImpl, storageImpl}
}

func (s *StateWrapper) Cid() cid.Cid {
	// FIXME this isn't how we should be getting the state cid since calling flush here could have side effects.
	c, err := s.Flush()
	if err != nil {
		panic(err)
	}
	return c
}

func (s *StateWrapper) Actor(addr vstate.Address) (vstate.Actor, error) {
	vaddr, err := address.NewFromBytes([]byte(addr))
	if err != nil {
		return nil, err
	}
	fcActor, err := s.StateTree.GetActor(vaddr)
	if err != nil {
		return nil, err
	}
	return &actorWrapper{*fcActor}, nil
}

func (s *StateWrapper) Storage(addr vstate.Address) (vstate.Storage, error) {
	return s.directStorage, nil
}

func (s *StateWrapper) NewAccountAddress() (vstate.Address, error) {
	return s.keys.NewAddress()
}

func (s *StateWrapper) SetActor(addr vstate.Address, code cid.Cid, balance vstate.AttoFIL) (vstate.Actor, vstate.Storage, error) {
	addrInt, err := address.NewFromBytes([]byte(addr))
	if err != nil {
		return nil, nil, err
	}
	// singleton actors get special handling
	switch addrInt {
	case actors.InitActorAddress:
		initact, err := gen.SetupInitActor(s.bs, nil)
		if err != nil{
			return nil, nil, err
		}
		if err := s.StateTree.SetActor(actors.InitActorAddress, initact); err != nil{
			return nil, nil, errors.Errorf("set init actor actor: %w", err)
		}
		return &actorWrapper{*initact}, s.directStorage, nil
	case actors.StorageMarketAddress:
		smact, err := gen.SetupStorageMarketActor(s.bs)
		if err != nil{
			return nil, nil, err
		}
		if err := s.StateTree.SetActor(actors.StorageMarketAddress, smact); err != nil{
			return nil, nil, errors.Errorf("set network storage market actor: %w", err)
		}
		return &actorWrapper{*smact}, s.directStorage, nil
	default:
		actr := &actorWrapper{types.Actor{
			Code:    code,
			Balance: types.BigInt{balance},
			Head:    vm.EmptyObjectCid,
		}}
		// The ID-based address is dropped here, but should be reported back to the caller.
		_, err = s.StateTree.RegisterNewAddress(addrInt, &actr.Actor)
		if err != nil{
			return nil, nil, errors.Errorf("register new address for actor: %w", err)
		}
		return actr, s.directStorage, nil
	}
}

func (s *StateWrapper) Signer() *keyStore {
	return s.keys
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

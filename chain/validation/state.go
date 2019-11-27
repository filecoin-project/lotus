package validation

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/vm"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/crypto"

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

func (s *StateWrapper) SetActor(addr vstate.Address, code vstate.ActorCodeID, balance vstate.AttoFIL) (vstate.Actor, vstate.Storage, error) {
	addrInt, err := address.NewFromBytes([]byte(addr))
	if err != nil {
		return nil, nil, err
	}
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, nil, err
	}
	actr := &actorWrapper{types.Actor{
		Code:    fromActorCode(code),
		Balance: types.BigInt{balance},
		Head:    vm.EmptyObjectCid,
	}}
	// The ID-based address is dropped here, but should be reported back to the caller.
	_, err = tree.RegisterNewAddress(addrInt, &actr.Actor)
	if err != nil {
		return nil, nil, xerrors.Errorf("register new address for actor: %w", err)
	}
	return actr, s.storage, s.flush(tree)
}

func (s *StateWrapper) SetSingletonActor(addr vstate.SingletonActorID, balance vstate.AttoFIL) (vstate.Actor, vstate.Storage, error) {
	vaddr := fromSingletonAddress(addr)
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, nil, err
	}
	lotusAddr, err := address.NewFromBytes([]byte(vaddr))
	if err != nil {
		return nil, nil, err
	}
	switch lotusAddr {
	case actors.InitAddress:
		initact, err := gen.SetupInitActor(s.bs, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.InitAddress, initact); err != nil {
			return nil, nil, xerrors.Errorf("set init actor: %w", err)
		}

		return &actorWrapper{*initact}, s.storage, s.flush(tree)
	case actors.StorageMarketAddress:
		smact, err := gen.SetupStorageMarketActor(s.bs)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.StorageMarketAddress, smact); err != nil {
			return nil, nil, xerrors.Errorf("set network storage market actor: %w", err)
		}
		return &actorWrapper{*smact}, s.storage, s.flush(tree)
	case actors.StoragePowerAddress:
		spact, err := gen.SetupStoragePowerActor(s.bs)
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
			Balance: types.BigInt{balance},
			Head:    vm.EmptyObjectCid,
		}
		if err := tree.SetActor(actors.NetworkAddress, ntwkact); err != nil {
			return nil, nil, xerrors.Errorf("set network actor: %w", err)
		}
		return &actorWrapper{*ntwkact}, s.storage, s.flush(tree)
	case actors.BurntFundsAddress:
		ntwkact := &types.Actor{
			Code:    actors.AccountCodeCid,
			Balance: types.BigInt{balance},
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

func fromActorCode(code vstate.ActorCodeID) cid.Cid {
	switch code {
	case vstate.AccountActorCodeCid:
		return actors.AccountCodeCid
	case vstate.StorageMinerCodeCid:
		return actors.StorageMinerCodeCid
	case vstate.MultisigActorCodeCid:
		return actors.MultisigCodeCid
	case vstate.PaymentChannelActorCodeCid:
		return actors.PaymentChannelCodeCid
	default:
		panic(fmt.Errorf("unknown actor code: %v", code))
	}
}

func fromSingletonAddress(addr vstate.SingletonActorID) vstate.Address {
	switch addr {
	case vstate.InitAddress:
		return vstate.Address(actors.InitAddress.Bytes())
	case vstate.NetworkAddress:
		return vstate.Address(actors.NetworkAddress.Bytes())
	case vstate.StorageMarketAddress:
		return vstate.Address(actors.StorageMarketAddress.Bytes())
	case vstate.BurntFundsAddress:
		return vstate.Address(actors.BurntFundsAddress.Bytes())
	case vstate.StoragePowerAddress:
		return vstate.Address(actors.StoragePowerAddress.Bytes())
	default:
		panic(fmt.Errorf("unknown singleton actor address: %v", addr))
	}
}

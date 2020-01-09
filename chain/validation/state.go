package validation

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/minio/blake2b-simd"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	vstate "github.com/filecoin-project/chain-validation/pkg/state"
	vactors "github.com/filecoin-project/chain-validation/pkg/state/actors"
	vaddress "github.com/filecoin-project/chain-validation/pkg/state/address"
	vtypes "github.com/filecoin-project/chain-validation/pkg/state/types"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-crypto"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
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

func (s *StateWrapper) Actor(addr vaddress.Address) (vstate.Actor, error) {
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

func (s *StateWrapper) Storage(addr vaddress.Address) (vstate.Storage, error) {
	return s.storage, nil
}

func (s *StateWrapper) NewAccountAddress() (vaddress.Address, error) {
	return s.keys.NewAddress()
}

func (s *StateWrapper) SetActor(addr vaddress.Address, code vactors.ActorCodeID, balance vtypes.BigInt) (vstate.Actor, vstate.Storage, error) {
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
		initact, err := gen.SetupInitActor(s.bs, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.InitAddress, initact); err != nil {
			return nil, nil, xerrors.Errorf("set init actor: %w", err)
		}

		return &actorWrapper{*initact}, s.storage, s.flush(tree)
	case actors.StorageMarketAddress:
		nsroot, err := gen.SetupStorageMarketActor(s.bs, s.stateRoot, nil)
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

func (s *StateWrapper) Sign(ctx context.Context, addr vaddress.Address, data []byte) (*vtypes.Signature, error) {
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
	s.stateRoot, err = tree.Flush()
	return
}

//
// Key store
//
type keyStore struct {
	// Private keys by address
	keys map[vaddress.Address]vtypes.KeyInfo
	// Seed for deterministic key generation.
	seed int64
}

func newKeyStore() *keyStore {
	return &keyStore{
		keys: make(map[vaddress.Address]vtypes.KeyInfo),
		seed: 0,
	}
}

func (s *keyStore) NewAddress() (vaddress.Address, error) {
	randSrc := rand.New(rand.NewSource(s.seed))
	prv, err := crypto.GenerateKeyFromSeed(randSrc)
	if err != nil {
		return vaddress.Undef, err
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
		return vaddress.Undef, err
	}
	vaddr, err := vaddress.NewFromBytes(key.Address.Bytes())
	if err != nil {
		return vaddress.Undef, err
	}
	s.keys[vaddr] = vki
	s.seed++
	return vaddress.NewFromBytes(key.Address.Bytes())
}

func (s *keyStore) Sign(ctx context.Context, addr vaddress.Address, data []byte) (*types.Signature, error) {
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

func (a *actorWrapper) Balance() vtypes.BigInt {
	return vtypes.NewInt(a.Actor.Balance.Uint64())

}

//
// Storage
//

type directStorage struct {
	cst *hamt.CborIpldStore
}

func (d *directStorage) Get(c cid.Cid, out interface{}) error {
	if err := d.cst.Get(context.TODO(), c, out.(cbg.CBORUnmarshaler)); err != nil {
		return err
	}
	return nil
}

func fromActorCode(code vactors.ActorCodeID) cid.Cid {
	switch code {
	case vactors.AccountActorCodeCid:
		return actors.AccountCodeCid
	case vactors.StorageMinerCodeCid:
		return actors.StorageMinerCodeCid
	case vactors.MultisigActorCodeCid:
		return actors.MultisigCodeCid
	case vactors.PaymentChannelActorCodeCid:
		return actors.PaymentChannelCodeCid
	default:
		panic(fmt.Errorf("unknown actor code: %v", code))
	}
}

func fromSingletonAddress(addr vactors.SingletonActorID) vaddress.Address {
	switch addr {
	case vactors.InitAddress:
		out, err := vaddress.NewFromBytes(actors.InitAddress.Bytes())
		if err != nil {
			panic(err)
		}
		return out
	case vactors.NetworkAddress:
		out, err := vaddress.NewFromBytes(actors.NetworkAddress.Bytes())
		if err != nil {
			panic(err)
		}
		return out
	case vactors.StorageMarketAddress:
		out, err := vaddress.NewFromBytes(actors.StorageMarketAddress.Bytes())
		if err != nil {
			panic(err)
		}
		return out
	case vactors.BurntFundsAddress:
		out, err := vaddress.NewFromBytes(actors.BurntFundsAddress.Bytes())
		if err != nil {
			panic(err)
		}
		return out
	case vactors.StoragePowerAddress:
		out, err := vaddress.NewFromBytes(actors.StoragePowerAddress.Bytes())
		if err != nil {
			panic(err)
		}
		return out
	default:
		panic(fmt.Errorf("unknown singleton actor address: %v", addr))
	}
}

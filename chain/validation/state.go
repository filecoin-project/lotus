package validation

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/minio/blake2b-simd"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-crypto"

	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	spec_crypto "github.com/filecoin-project/specs-actors/actors/crypto"

	vstate "github.com/filecoin-project/chain-validation/pkg/state"

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
	cst cbor.IpldStore
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

func (s *StateWrapper) SetActor(addr address.Address, code cid.Cid, balance big.Int) (vstate.Actor, vstate.Storage, error) {
	addrInt, err := address.NewFromBytes(addr.Bytes())
	if err != nil {
		return nil, nil, err
	}
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, nil, err
	}
	actr := &actorWrapper{types.Actor{
		Code:    code,
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

func (s *StateWrapper) SetSingletonActor(addr address.Address, balance big.Int) (vstate.Actor, vstate.Storage, error) {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, nil, err
	}

	switch addr {
	case builtin.InitActorAddr:
		initact, err := gen.SetupInitActor(s.bs, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.InitAddress, initact); err != nil {
			return nil, nil, xerrors.Errorf("set init actor: %w", err)
		}
		return &actorWrapper{*initact}, s.storage, s.flush(tree)
	case builtin.StorageMarketActorAddr:
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
	case builtin.StoragePowerActorAddr:
		spact, err := gen.SetupStoragePowerActor(s.bs)
		if err != nil {
			return nil, nil, err
		}
		if err := tree.SetActor(actors.StoragePowerAddress, spact); err != nil {
			return nil, nil, xerrors.Errorf("set network storage market actor: %w", err)
		}
		return &actorWrapper{*spact}, s.storage, s.flush(tree)
	case actors.NetworkAddress: // TODO special case for lotus, should be combined with BurntFundAddress
	panic("see TODO")
		ntwkact := &types.Actor{
			Code:    actors.AccountCodeCid,
			Balance: types.BigInt{balance.Int},
			Head:    vm.EmptyObjectCid,
		}
		if err := tree.SetActor(actors.NetworkAddress, ntwkact); err != nil {
			return nil, nil, xerrors.Errorf("set network actor: %w", err)
		}
		return &actorWrapper{*ntwkact}, s.storage, s.flush(tree)
	case builtin.BurntFundsActorAddr:
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

func (s *StateWrapper) Sign(ctx context.Context, addr address.Address, data []byte) (*spec_crypto.Signature, error) {
	sig, err := s.keys.Sign(ctx, addr, data)
	if err != nil {
		return nil, err
	}

	var specType spec_crypto.SigType
	if sig.Type == types.KTSecp256k1 {
		specType = spec_crypto.SigTypeSecp256k1
	} else if sig.Type == types.KTBLS {
		specType = spec_crypto.SigTypeBLS
	} else {
		panic("invalid address type")
	}

	return &spec_crypto.Signature{
		Type: specType,
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

// TODO support BLS
func (s *keyStore) NewAddress() (address.Address, error) {
	randSrc := rand.New(rand.NewSource(s.seed))
	prv, err := crypto.GenerateKeyFromSeed(randSrc)
	if err != nil {
		return address.Undef, err
	}

	vki := types.KeyInfo{
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
		return nil, fmt.Errorf("unknown address %v", addr)
	}
	b2sum := blake2b.Sum256(data)
	digest, err := crypto.Sign(ki.PrivateKey, b2sum[:])
	if err != nil {
		return nil, err
	}

	// TODO support BLS
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

func (a *actorWrapper) CallSeqNum() int64 {
	return int64(a.Actor.Nonce)
}

func (a *actorWrapper) Balance() big.Int {
	return big.Int{a.Actor.Balance.Int}

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


package builders

import (
	"context"
	"log"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type registeredActor struct {
	handle  AddressHandle
	initial abi.TokenAmount
}

// Actors is an object that manages actors in the test vector.
type Actors struct {
	// registered stores registered actors and their initial balances.
	registered []registeredActor

	b *Builder
}

func newActors(b *Builder) *Actors {
	return &Actors{b: b}
}

// Count returns the number of actors registered during preconditions.
func (a *Actors) Count() int {
	return len(a.registered)
}

// HandleFor gets the canonical handle for a registered address, which can
// appear at either ID or Robust position.
func (a *Actors) HandleFor(addr address.Address) AddressHandle {
	for _, r := range a.registered {
		if r.handle.ID == addr || r.handle.Robust == addr {
			return r.handle
		}
	}
	a.b.Assert.FailNowf("asked for handle of unknown actor", "actor: %s", addr)
	return AddressHandle{} // will never reach here.
}

// InitialBalance returns the initial balance of an actor that was registered
// during preconditions. It matches against both the ID and Robust
// addresses. It records an assertion failure if the actor is unknown.
func (a *Actors) InitialBalance(addr address.Address) abi.TokenAmount {
	for _, r := range a.registered {
		if r.handle.ID == addr || r.handle.Robust == addr {
			return r.initial
		}
	}
	a.b.Assert.FailNowf("asked for initial balance of unknown actor", "actor: %s", addr)
	return big.Zero() // will never reach here.
}

// Handles returns the AddressHandles for all registered actors.
func (a *Actors) Handles() []AddressHandle {
	ret := make([]AddressHandle, 0, len(a.registered))
	for _, r := range a.registered {
		ret = append(ret, r.handle)
	}
	return ret
}

// AccountN creates many account actors of the specified kind, with the
// specified balance, and places their addresses in the supplied AddressHandles.
func (a *Actors) AccountN(typ address.Protocol, balance abi.TokenAmount, handles ...*AddressHandle) {
	for _, handle := range handles {
		h := a.Account(typ, balance)
		*handle = h
	}
}

// Account creates a single account actor of the specified kind, with the
// specified balance, and returns its AddressHandle.
func (a *Actors) Account(typ address.Protocol, balance abi.TokenAmount) AddressHandle {
	a.b.Assert.In(typ, address.SECP256K1, address.BLS)

	var addr address.Address
	switch typ {
	case address.SECP256K1:
		addr = a.b.Wallet.NewSECP256k1Account()
	case address.BLS:
		addr = a.b.Wallet.NewBLSAccount()
	}

	actorState := &account.State{Address: addr}
	handle := a.CreateActor(builtin.AccountActorCodeID, addr, balance, actorState)

	a.registered = append(a.registered, registeredActor{handle, balance})
	return handle
}

type MinerActorCfg struct {
	SealProofType  abi.RegisteredSealProof
	PeriodBoundary abi.ChainEpoch
	OwnerBalance   abi.TokenAmount
}

// Miner creates an owner account, a worker account, and a miner actor managed
// by those initial.
func (a *Actors) Miner(cfg MinerActorCfg) (minerActor, owner, worker AddressHandle) {
	owner = a.Account(address.SECP256K1, cfg.OwnerBalance)
	worker = a.Account(address.BLS, big.Zero())
	// expectedMinerActorIDAddress := chain.MustNewIDAddr(chain.MustIDFromAddress(minerWorkerID) + 1)
	// minerActorAddrs := computeInitActorExecReturn(minerWorkerPk, 0, 1, expectedMinerActorIDAddress)

	ss, err := cfg.SealProofType.SectorSize()
	a.b.Assert.NoError(err, "seal proof sector size")

	ps, err := cfg.SealProofType.WindowPoStPartitionSectors()
	a.b.Assert.NoError(err, "seal proof window PoSt partition sectors")

	mi := &miner.MinerInfo{
		Owner:                      owner.ID,
		Worker:                     worker.ID,
		PendingWorkerKey:           nil,
		PeerId:                     abi.PeerID("chain-validation"),
		Multiaddrs:                 nil,
		SealProofType:              cfg.SealProofType,
		SectorSize:                 ss,
		WindowPoStPartitionSectors: ps,
	}
	infoCid, err := a.b.Stores.CBORStore.Put(context.Background(), mi)
	if err != nil {
		panic(err)
	}

	// create the miner actor s.t. it exists in the init actors map
	minerState, err := miner.ConstructState(infoCid,
		cfg.PeriodBoundary,
		EmptyBitfieldCid,
		EmptyArrayCid,
		EmptyMapCid,
		EmptyDeadlinesCid,
		EmptyVestingFundsCid,
	)
	if err != nil {
		panic(err)
	}

	minerActorAddr := worker.NextActorAddress(0, 0)
	handle := a.CreateActor(builtin.StorageMinerActorCodeID, minerActorAddr, big.Zero(), minerState)

	// assert miner actor has been created, exists in the state tree, and has an entry in the init actor.
	// next update the storage power actor to track the miner

	var spa power.State
	a.ActorState(builtin.StoragePowerActorAddr, &spa)

	// set the miners claim
	hm, err := adt.AsMap(adt.WrapStore(context.Background(), a.b.Stores.CBORStore), spa.Claims)
	if err != nil {
		panic(err)
	}

	// add claim for the miner
	err = hm.Put(adt.AddrKey(handle.ID), &power.Claim{
		RawBytePower:    abi.NewStoragePower(0),
		QualityAdjPower: abi.NewTokenAmount(0),
	})
	if err != nil {
		panic(err)
	}

	// save the claim
	spa.Claims, err = hm.Root()
	if err != nil {
		panic(err)
	}

	// update miner count
	spa.MinerCount += 1

	// update storage power actor's state in the tree
	_, err = a.b.Stores.CBORStore.Put(context.Background(), &spa)
	if err != nil {
		panic(err)
	}

	a.registered = append(a.registered, registeredActor{handle, big.Zero()})
	return handle, owner, worker
}

// CreateActor creates an actor in the state tree, of the specified kind, with
// the specified address and balance, and sets its state to the supplied state.
func (a *Actors) CreateActor(code cid.Cid, addr address.Address, balance abi.TokenAmount, state runtime.CBORMarshaler) AddressHandle {
	var id address.Address
	if addr.Protocol() != address.ID {
		var err error
		id, err = a.b.StateTree.RegisterNewAddress(addr)
		if err != nil {
			log.Panicf("register new address for actor: %v", err)
		}
	}

	// Store the new state.
	head, err := a.b.StateTree.Store.Put(context.Background(), state)
	if err != nil {
		panic(err)
	}

	// Set the actor's head to point to that state.
	actr := &types.Actor{
		Code:    code,
		Head:    head,
		Balance: balance,
	}
	if err := a.b.StateTree.SetActor(addr, actr); err != nil {
		log.Panicf("setting new actor for actor: %v", err)
	}
	return AddressHandle{id, addr}
}

// ActorState retrieves the state of the supplied actor, and sets it in the
// provided object. It also returns the actor's header from the state tree.
func (a *Actors) ActorState(addr address.Address, out cbg.CBORUnmarshaler) *types.Actor {
	actor := a.Header(addr)
	err := a.b.StateTree.Store.Get(context.Background(), actor.Head, out)
	a.b.Assert.NoError(err, "failed to load state for actorr %s; head=%s", addr, actor.Head)
	return actor
}

// Header returns the actor's header from the state tree.
func (a *Actors) Header(addr address.Address) *types.Actor {
	actor, err := a.b.StateTree.GetActor(addr)
	a.b.Assert.NoError(err, "failed to fetch actor %s from state", addr)
	return actor
}

// Balance is a shortcut for Header(addr).Balance.
func (a *Actors) Balance(addr address.Address) abi.TokenAmount {
	return a.Header(addr).Balance
}

// Head is a shortcut for Header(addr).Head.
func (a *Actors) Head(addr address.Address) cid.Cid {
	return a.Header(addr).Head
}

// Nonce is a shortcut for Header(addr).Nonce.
func (a *Actors) Nonce(addr address.Address) uint64 {
	return a.Header(addr).Nonce
}

// Code is a shortcut for Header(addr).Code.
func (a *Actors) Code(addr address.Address) cid.Cid {
	return a.Header(addr).Code
}

package drivers

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/oni/tvx/chain"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	miner_spec "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	power_spec "github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	adt_spec "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	account_spec "github.com/filecoin-project/specs-actors/actors/builtin/account"
)

var (
	SECP = address.SECP256K1
	BLS  = address.BLS
)

// StateDriver mutates and inspects a state.
type StateDriver struct {
	st *StateWrapper
	w  *KeyManager
	rs RandomnessSource

	minerInfo *MinerInfo

	// Mapping for IDAddresses to their pubkey/actor addresses. Used for lookup when signing messages.
	actorIDMap map[address.Address]address.Address
}

// info about the state drivers builtin miner
type MinerInfo struct {
	Owner   address.Address
	OwnerID address.Address

	Worker   address.Address
	WorkerID address.Address
}

// NewStateDriver creates a new state driver for a state.
func NewStateDriver(st *StateWrapper, w *KeyManager) *StateDriver {
	return &StateDriver{st, w, NewRandomnessSource(), nil, make(map[address.Address]address.Address)}
}

// State returns the state.
func (d *StateDriver) State() *StateWrapper {
	return d.st
}

func (d *StateDriver) Wallet() *KeyManager {
	return d.w
}

func (d *StateDriver) Randomness() RandomnessSource {
	return d.rs
}

func (d *StateDriver) GetState(c cid.Cid, out cbg.CBORUnmarshaler) {
	err := d.st.StoreGet(c, out)
	require.NoError(t, err)
}

func (d *StateDriver) PutState(in cbg.CBORMarshaler) cid.Cid {
	c, err := d.st.StorePut(in)
	require.NoError(t, err)
	return c
}

func (d *StateDriver) GetActorState(actorAddr address.Address, out cbg.CBORUnmarshaler) {
	actor, err := d.State().Actor(actorAddr)
	require.NoError(t, err)
	require.NotNil(t, actor)

	d.GetState(actor.Head(), out)
}

// NewAccountActor installs a new account actor, returning the address.
func (d *StateDriver) NewAccountActor(addrType address.Protocol, balanceAttoFil abi_spec.TokenAmount) (pubkey address.Address, id address.Address) {
	var addr address.Address
	switch addrType {
	case address.SECP256K1:
		addr = d.w.NewSECP256k1AccountAddress()
	case address.BLS:
		addr = d.w.NewBLSAccountAddress()
	default:
		require.FailNowf(t, "unsupported address", "protocol for account actor: %v", addrType)
	}

	_, idAddr, err := d.st.CreateActor(builtin_spec.AccountActorCodeID, addr, balanceAttoFil, &account_spec.State{Address: addr})
	require.NoError(t, err)
	d.actorIDMap[idAddr] = addr
	return addr, idAddr
}

func (d *StateDriver) ActorPubKey(idAddress address.Address) address.Address {
	if idAddress.Protocol() != address.ID {
		t.Fatalf("ActorPubKey methods expects ID protocol address. actual: %v", idAddress.Protocol())
	}
	pubkeyAddr, found := d.actorIDMap[idAddress]
	if !found {
		t.Fatalf("Failed to find pubkey address for: %s", idAddress)
	}
	return pubkeyAddr
}

func (d *StateDriver) BuiltinMinerInfo() *MinerInfo {
	return d.minerInfo
}

// create miner without sending a message. modify the init and power actor manually
func (d *StateDriver) newMinerAccountActor(sealProofType abi_spec.RegisteredSealProof, periodBoundary abi_spec.ChainEpoch) address.Address {
	// creat a miner, owner, and its worker
	minerOwnerPk, minerOwnerID := d.NewAccountActor(address.SECP256K1, big_spec.NewInt(1_000_000_000))
	minerWorkerPk, minerWorkerID := d.NewAccountActor(address.BLS, big_spec.Zero())
	expectedMinerActorIDAddress := chain.MustNewIDAddr(chain.MustIdFromAddress(minerWorkerID) + 1)
	minerActorAddrs := computeInitActorExecReturn(minerWorkerPk, 0, 1, expectedMinerActorIDAddress)

	d.minerInfo = &MinerInfo{
		Owner:    minerOwnerPk,
		OwnerID:  minerOwnerID,
		Worker:   minerWorkerPk,
		WorkerID: minerWorkerID,
	}

	ss, err := sealProofType.SectorSize()
	require.NoError(t, err)
	ps, err := sealProofType.WindowPoStPartitionSectors()
	require.NoError(t, err)
	mi := &miner_spec.MinerInfo{
		Owner:                      minerOwnerID,
		Worker:                     minerWorkerID,
		PendingWorkerKey:           nil,
		PeerId:                     abi_spec.PeerID("chain-validation"),
		Multiaddrs:                 nil,
		SealProofType:              sealProofType,
		SectorSize:                 ss,
		WindowPoStPartitionSectors: ps,
	}
	mc, err := d.st.StorePut(mi)
	require.NoError(t, err)

	// create the miner actor s.t. it exists in the init actors map
	minerState, err := miner_spec.ConstructState(mc, periodBoundary, EmptyBitfieldCid, EmptyArrayCid, EmptyMapCid, EmptyDeadlinesCid)

	require.NoError(t, err)
	_, minerActorIDAddr, err := d.State().CreateActor(builtin_spec.StorageMinerActorCodeID, minerActorAddrs.RobustAddress, big_spec.Zero(), minerState)
	require.NoError(t, err)
	require.Equal(t, expectedMinerActorIDAddress, minerActorIDAddr)

	// a miner actor has been created, exists in the state tree, and has an entry in the init actor.
	// next update the storage power actor to track the miner

	var spa power_spec.State
	d.GetActorState(builtin_spec.StoragePowerActorAddr, &spa)

	// set the miners claim
	hm, err := adt_spec.AsMap(AsStore(d.State()), spa.Claims)
	require.NoError(t, err)

	// add claim for the miner
	err = hm.Put(adt_spec.AddrKey(minerActorIDAddr), &power_spec.Claim{
		RawBytePower:    abi_spec.NewStoragePower(0),
		QualityAdjPower: abi_spec.NewTokenAmount(0),
	})
	require.NoError(t, err)

	// save the claim
	spa.Claims, err = hm.Root()
	require.NoError(t, err)

	// update miner count
	spa.MinerCount += 1

	// update storage power actor's state in the tree
	d.PutState(&spa)

	return minerActorIDAddr
}

func AsStore(vmw *StateWrapper) adt_spec.Store {
	return &storeWrapper{vmw: vmw}
}

type storeWrapper struct {
	vmw *StateWrapper
}

func (s storeWrapper) Context() context.Context {
	return context.TODO()
}

func (s storeWrapper) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	return s.vmw.StoreGet(c, out.(runtime.CBORUnmarshaler))
}

func (s storeWrapper) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return s.vmw.StorePut(v.(runtime.CBORMarshaler))
}

package actors

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	mh "github.com/multiformats/go-multihash"
)

var log = logging.Logger("actors")

var EmptyCBOR cid.Cid

const (
	GasCreateActor = 100
)

func init() {

	n, err := cbor.WrapObject(map[string]string{}, mh.SHA2_256, -1)
	if err != nil {
		panic(err) // ok
	}

	EmptyCBOR = n.Cid()
}

type InitActor struct{}

type InitActorState struct {
	AddressMap cid.Cid

	NextID uint64
}

type iAMethods struct {
	Exec uint64
}

var IAMethods = iAMethods{2}

func (ia InitActor) Exports() []interface{} {
	return []interface{}{
		1: nil,
		2: ia.Exec,
	}
}

type ExecParams struct {
	Code   cid.Cid
	Params []byte
}

func CreateExecParams(act cid.Cid, obj cbg.CBORMarshaler) ([]byte, aerrors.ActorError) {
	encparams, err := SerializeParams(obj)
	if err != nil {
		return nil, aerrors.Wrap(err, "creating ExecParams")
	}

	return SerializeParams(&ExecParams{
		Code:   act,
		Params: encparams,
	})
}

func (ia InitActor) Exec(act *types.Actor, vmctx types.VMContext, p *ExecParams) ([]byte, aerrors.ActorError) {
	beginState := vmctx.Storage().GetHead()

	var self InitActorState
	if err := vmctx.Storage().Get(beginState, &self); err != nil {
		return nil, err
	}

	if err := vmctx.ChargeGas(GasCreateActor); err != nil {
		return nil, aerrors.Wrap(err, "run out of gas")
	}

	// Make sure that only the actors defined in the spec can be launched.
	if !IsBuiltinActor(p.Code) {
		return nil, aerrors.New(1,
			"cannot launch actor instance that is not a builtin actor")
	}

	// Ensure that singletons can be only launched once.
	// TODO: do we want to enforce this? If so how should actors be marked as such?
	if IsSingletonActor(p.Code) {
		return nil, aerrors.New(1, "cannot launch another actor of this type")
	}

	// This generates a unique address for this actor that is stable across message
	// reordering
	creator := vmctx.Message().From
	nonce := vmctx.Message().Nonce
	addr, err := ComputeActorAddress(creator, nonce)
	if err != nil {
		return nil, err
	}

	// Set up the actor itself
	actor := types.Actor{
		Code:    p.Code,
		Balance: types.NewInt(0),
		Head:    EmptyCBOR,
		Nonce:   0,
	}

	// The call to the actors constructor will set up the initial state
	// from the given parameters, setting `actor.Head` to a new value when successful.
	// TODO: can constructors fail?
	//actor.Constructor(p.Params)

	// Store the mapping of address to actor ID.
	idAddr, nerr := self.AddActor(vmctx.Ipld(), addr)
	if nerr != nil {
		return nil, aerrors.Escalate(err, "adding new actor mapping")
	}

	// NOTE: This is a privileged call that only the init actor is allowed to make
	// FIXME: Had to comment this  because state is not in interface
	state, err := vmctx.StateTree()
	if err != nil {
		return nil, err
	}

	if err := state.SetActor(idAddr, &actor); err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			return nil, aerrors.Absorb(err, 1, "SetActor, actor not found")
		}
		return nil, aerrors.Escalate(err, "inserting new actor into state tree")
	}

	// '1' is reserved for constructor methods
	_, err = vmctx.Send(idAddr, 1, vmctx.Message().Value, p.Params)
	if err != nil {
		return nil, err
	}

	c, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(beginState, c); err != nil {
		return nil, err
	}

	return idAddr.Bytes(), nil
}

func IsBuiltinActor(code cid.Cid) bool {
	switch code {
	case StorageMarketCodeCid, StoragePowerCodeCid, StorageMinerCodeCid, AccountCodeCid, InitCodeCid, MultisigCodeCid, PaymentChannelCodeCid:
		return true
	default:
		return false
	}
}

func IsSingletonActor(code cid.Cid) bool {
	return code == StoragePowerCodeCid || code == StorageMarketCodeCid || code == InitCodeCid || code == CronCodeCid
}

func (ias *InitActorState) AddActor(cst *hamt.CborIpldStore, addr address.Address) (address.Address, error) {
	nid := ias.NextID

	amap, err := hamt.LoadNode(context.TODO(), cst, ias.AddressMap)
	if err != nil {
		return address.Undef, err
	}

	if err := amap.Set(context.TODO(), string(addr.Bytes()), nid); err != nil {
		return address.Undef, err
	}

	if err := amap.Flush(context.TODO()); err != nil {
		return address.Undef, err
	}

	ncid, err := cst.Put(context.TODO(), amap)
	if err != nil {
		return address.Undef, err
	}
	ias.AddressMap = ncid
	ias.NextID++

	return NewIDAddress(nid)
}

func (ias *InitActorState) Lookup(cst *hamt.CborIpldStore, addr address.Address) (address.Address, error) {
	amap, err := hamt.LoadNode(context.TODO(), cst, ias.AddressMap)
	if err != nil {
		return address.Undef, xerrors.Errorf("ias lookup failed loading hamt node: %w", err)
	}

	var val interface{}
	err = amap.Find(context.TODO(), string(addr.Bytes()), &val)
	if err != nil {
		return address.Undef, xerrors.Errorf("ias lookup failed to do find: %w", err)
	}

	ival, ok := val.(uint64)
	if !ok {
		return address.Undef, fmt.Errorf("invalid value in init actor state, expected uint64, got %T", val)
	}

	return address.NewIDAddress(ival)
}

type AccountActorState struct {
	Address address.Address
}

func ComputeActorAddress(creator address.Address, nonce uint64) (address.Address, ActorError) {
	buf := new(bytes.Buffer)
	_, err := buf.Write(creator.Bytes())
	if err != nil {
		return address.Undef, aerrors.Escalate(err, "could not write address")
	}

	err = binary.Write(buf, binary.BigEndian, nonce)
	if err != nil {
		return address.Undef, aerrors.Escalate(err, "could not write nonce")
	}

	addr, err := address.NewActorAddress(buf.Bytes())
	if err != nil {
		return address.Undef, aerrors.Escalate(err, "could not create address")
	}
	return addr, nil
}

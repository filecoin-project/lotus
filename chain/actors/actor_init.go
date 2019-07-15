package actors

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"
)

var log = logging.Logger("actors")

func init() {
	cbor.RegisterCborType(ExecParams{})
}

type InitActor struct{}

type InitActorState struct {
	// TODO: this needs to be a HAMT, its a dumb map for now
	AddressMap cid.Cid

	NextID uint64
}

func (ia InitActor) Exports() []interface{} {
	return []interface{}{
		nil,
		ia.Exec,
	}
}

type ExecParams struct {
	Code   cid.Cid
	Params []byte
}

func CreateExecParams(act cid.Cid, obj interface{}) ([]byte, error) {
	encparams, err := cbor.DumpObject(obj)
	if err != nil {
		return nil, err
	}

	var ep ExecParams
	ep.Code = act
	ep.Params = encparams

	return cbor.DumpObject(ep)
}

func (ia InitActor) Exec(act *types.Actor, vmctx types.VMContext, p *ExecParams) (types.InvokeRet, error) {
	beginState := vmctx.Storage().GetHead()

	var self InitActorState
	if err := vmctx.Storage().Get(beginState, &self); err != nil {
		return types.InvokeRet{}, err
	}

	// Make sure that only the actors defined in the spec can be launched.
	if !IsBuiltinActor(p.Code) {
		log.Error("cannot launch actor instance that is not a builtin actor")
		return types.InvokeRet{
			ReturnCode: 1,
		}, nil
	}

	// Ensure that singeltons can be only launched once.
	// TODO: do we want to enforce this? If so how should actors be marked as such?
	if IsSingletonActor(p.Code) {
		log.Error("cannot launch another actor of this type")
		return types.InvokeRet{
			ReturnCode: 1,
		}, nil
	}

	// This generates a unique address for this actor that is stable across message
	// reordering
	creator := vmctx.Message().From
	nonce := vmctx.Message().Nonce
	addr, err := ComputeActorAddress(creator, nonce)
	if err != nil {
		return types.InvokeRet{}, err
	}

	// Set up the actor itself
	actor := types.Actor{
		Code:    p.Code,
		Balance: vmctx.Message().Value,
		Head:    cid.Undef,
		Nonce:   0,
	}

	// The call to the actors constructor will set up the initial state
	// from the given parameters, setting `actor.Head` to a new value when successful.
	// TODO: can constructors fail?
	//actor.Constructor(p.Params)

	// Store the mapping of address to actor ID.
	idAddr, err := self.AddActor(vmctx, addr)
	if err != nil {
		return types.InvokeRet{}, errors.Wrap(err, "adding new actor mapping")
	}

	// NOTE: This is a privileged call that only the init actor is allowed to make
	// FIXME: Had to comment this  because state is not in interface
	state, err := vmctx.StateTree()
	if err != nil {
		return types.InvokeRet{}, err
	}

	if err := state.SetActor(idAddr, &actor); err != nil {
		return types.InvokeRet{}, errors.Wrap(err, "inserting new actor into state tree")
	}

	c, err := vmctx.Storage().Put(self)
	if err != nil {
		return types.InvokeRet{}, err
	}

	if err := vmctx.Storage().Commit(beginState, c); err != nil {
		return types.InvokeRet{}, err
	}

	return types.InvokeRet{
		Result: idAddr.Bytes(),
	}, nil
}

func IsBuiltinActor(code cid.Cid) bool {
	switch code {
	case StorageMinerCodeCid, StorageMinerCodeCid, AccountActorCodeCid, InitActorCodeCid, MultisigActorCodeCid:
		return true
	default:
		return false
	}
}

func IsSingletonActor(code cid.Cid) bool {
	return code == StorageMarketActorCodeCid || code == InitActorCodeCid
}

func (ias *InitActorState) AddActor(vmctx types.VMContext, addr address.Address) (address.Address, error) {
	nid := ias.NextID
	ias.NextID++

	amap, err := hamt.LoadNode(context.TODO(), vmctx.Ipld(), ias.AddressMap)
	if err != nil {
		return address.Undef, err
	}

	if err := amap.Set(context.TODO(), string(addr.Bytes()), nid); err != nil {
		return address.Undef, err
	}

	if err := amap.Flush(context.TODO()); err != nil {
		return address.Undef, err
	}

	ncid, err := vmctx.Ipld().Put(context.TODO(), amap)
	if err != nil {
		return address.Undef, err
	}
	ias.AddressMap = ncid

	return address.NewIDAddress(nid)
}

func (ias *InitActorState) Lookup(cst *hamt.CborIpldStore, addr address.Address) (address.Address, error) {
	amap, err := hamt.LoadNode(context.TODO(), cst, ias.AddressMap)
	if err != nil {
		return address.Undef, err
	}

	val, err := amap.Find(context.TODO(), string(addr.Bytes()))
	if err != nil {
		return address.Undef, err
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

func ComputeActorAddress(creator address.Address, nonce uint64) (address.Address, error) {
	buf := new(bytes.Buffer)
	_, err := buf.Write(creator.Bytes())
	if err != nil {
		return address.Address{}, err
	}

	err = binary.Write(buf, binary.BigEndian, nonce)
	if err != nil {
		return address.Address{}, err
	}

	return address.NewActorAddress(buf.Bytes())
}

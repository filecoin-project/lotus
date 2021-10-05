package multisig

import (
	"fmt"

	"github.com/minio/blake2b-simd"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/ipfs/go-cid"

	msig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	msig6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/multisig"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

func init() {

	builtin.RegisterActorState(builtin0.MultisigActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load0(store, root)
	})

	builtin.RegisterActorState(builtin2.MultisigActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load2(store, root)
	})

	builtin.RegisterActorState(builtin3.MultisigActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load3(store, root)
	})

	builtin.RegisterActorState(builtin4.MultisigActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load4(store, root)
	})

	builtin.RegisterActorState(builtin5.MultisigActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load5(store, root)
	})

	builtin.RegisterActorState(builtin6.MultisigActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load6(store, root)
	})
}

func Load(store adt.Store, act *types.Actor) (State, error) {
	switch act.Code {

	case builtin0.MultisigActorCodeID:
		return load0(store, act.Head)

	case builtin2.MultisigActorCodeID:
		return load2(store, act.Head)

	case builtin3.MultisigActorCodeID:
		return load3(store, act.Head)

	case builtin4.MultisigActorCodeID:
		return load4(store, act.Head)

	case builtin5.MultisigActorCodeID:
		return load5(store, act.Head)

	case builtin6.MultisigActorCodeID:
		return load6(store, act.Head)

	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actors.Version, signers []address.Address, threshold uint64, startEpoch abi.ChainEpoch, unlockDuration abi.ChainEpoch, initialBalance abi.TokenAmount) (State, error) {
	switch av {

	case actors.Version0:
		return make0(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actors.Version2:
		return make2(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actors.Version3:
		return make3(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actors.Version4:
		return make4(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actors.Version5:
		return make5(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actors.Version6:
		return make6(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

func GetActorCodeID(av actors.Version) (cid.Cid, error) {
	switch av {

	case actors.Version0:
		return builtin0.MultisigActorCodeID, nil

	case actors.Version2:
		return builtin2.MultisigActorCodeID, nil

	case actors.Version3:
		return builtin3.MultisigActorCodeID, nil

	case actors.Version4:
		return builtin4.MultisigActorCodeID, nil

	case actors.Version5:
		return builtin5.MultisigActorCodeID, nil

	case actors.Version6:
		return builtin6.MultisigActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	LockedBalance(epoch abi.ChainEpoch) (abi.TokenAmount, error)
	StartEpoch() (abi.ChainEpoch, error)
	UnlockDuration() (abi.ChainEpoch, error)
	InitialBalance() (abi.TokenAmount, error)
	Threshold() (uint64, error)
	Signers() ([]address.Address, error)

	ForEachPendingTxn(func(id int64, txn Transaction) error) error
	PendingTxnChanged(State) (bool, error)

	transactions() (adt.Map, error)
	decodeTransaction(val *cbg.Deferred) (Transaction, error)
	GetState() interface{}
}

type Transaction = msig0.Transaction

var Methods = builtin6.MethodsMultisig

func Message(version actors.Version, from address.Address) MessageBuilder {
	switch version {

	case actors.Version0:
		return message0{from}

	case actors.Version2:
		return message2{message0{from}}

	case actors.Version3:
		return message3{message0{from}}

	case actors.Version4:
		return message4{message0{from}}

	case actors.Version5:
		return message5{message0{from}}

	case actors.Version6:
		return message6{message0{from}}
	default:
		panic(fmt.Sprintf("unsupported actors version: %d", version))
	}
}

type MessageBuilder interface {
	// Create a new multisig with the specified parameters.
	Create(signers []address.Address, threshold uint64,
		vestingStart, vestingDuration abi.ChainEpoch,
		initialAmount abi.TokenAmount) (*types.Message, error)

	// Propose a transaction to the given multisig.
	Propose(msig, target address.Address, amt abi.TokenAmount,
		method abi.MethodNum, params []byte) (*types.Message, error)

	// Approve a multisig transaction. The "hash" is optional.
	Approve(msig address.Address, txID uint64, hash *ProposalHashData) (*types.Message, error)

	// Cancel a multisig transaction. The "hash" is optional.
	Cancel(msig address.Address, txID uint64, hash *ProposalHashData) (*types.Message, error)
}

// this type is the same between v0 and v2
type ProposalHashData = msig6.ProposalHashData
type ProposeReturn = msig6.ProposeReturn
type ProposeParams = msig6.ProposeParams
type ApproveReturn = msig6.ApproveReturn

func txnParams(id uint64, data *ProposalHashData) ([]byte, error) {
	params := msig6.TxnIDParams{ID: msig6.TxnID(id)}
	if data != nil {
		if data.Requester.Protocol() != address.ID {
			return nil, xerrors.Errorf("proposer address must be an ID address, was %s", data.Requester)
		}
		if data.Value.Sign() == -1 {
			return nil, xerrors.Errorf("proposal value must be non-negative, was %s", data.Value)
		}
		if data.To == address.Undef {
			return nil, xerrors.Errorf("proposed destination address must be set")
		}
		pser, err := data.Serialize()
		if err != nil {
			return nil, err
		}
		hash := blake2b.Sum256(pser)
		params.ProposalHash = hash[:]
	}

	return actors.SerializeParams(&params)
}

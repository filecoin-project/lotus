package multisig

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/minio/blake2b-simd"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	msig12 "github.com/filecoin-project/go-state-types/builtin/v12/multisig"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.MultisigKey {
			return nil, xerrors.Errorf("actor code is not multisig: %s", name)
		}

		switch av {

		case actorstypes.Version8:
			return load8(store, act.Head)

		case actorstypes.Version9:
			return load9(store, act.Head)

		case actorstypes.Version10:
			return load10(store, act.Head)

		case actorstypes.Version11:
			return load11(store, act.Head)

		case actorstypes.Version12:
			return load12(store, act.Head)

		}
	}

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

	case builtin7.MultisigActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actorstypes.Version, signers []address.Address, threshold uint64, startEpoch abi.ChainEpoch, unlockDuration abi.ChainEpoch, initialBalance abi.TokenAmount) (State, error) {
	switch av {

	case actorstypes.Version0:
		return make0(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version2:
		return make2(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version3:
		return make3(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version4:
		return make4(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version5:
		return make5(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version6:
		return make6(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version7:
		return make7(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version8:
		return make8(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version9:
		return make9(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version10:
		return make10(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version11:
		return make11(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	case actorstypes.Version12:
		return make12(store, signers, threshold, startEpoch, unlockDuration, initialBalance)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	Code() cid.Cid
	ActorKey() string
	ActorVersion() actorstypes.Version

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

type Transaction = msig12.Transaction

var Methods = builtintypes.MethodsMultisig

func Message(version actorstypes.Version, from address.Address) MessageBuilder {
	switch version {

	case actorstypes.Version0:
		return message0{from}

	case actorstypes.Version2:
		return message2{message0{from}}

	case actorstypes.Version3:
		return message3{message0{from}}

	case actorstypes.Version4:
		return message4{message0{from}}

	case actorstypes.Version5:
		return message5{message0{from}}

	case actorstypes.Version6:
		return message6{message0{from}}

	case actorstypes.Version7:
		return message7{message0{from}}

	case actorstypes.Version8:
		return message8{message0{from}}

	case actorstypes.Version9:
		return message9{message0{from}}

	case actorstypes.Version10:
		return message10{message0{from}}

	case actorstypes.Version11:
		return message11{message0{from}}

	case actorstypes.Version12:
		return message12{message0{from}}
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
type ProposalHashData = msig12.ProposalHashData
type ProposeReturn = msig12.ProposeReturn
type ProposeParams = msig12.ProposeParams
type ApproveReturn = msig12.ApproveReturn

func txnParams(id uint64, data *ProposalHashData) ([]byte, error) {
	params := msig12.TxnIDParams{ID: msig12.TxnID(id)}
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

func AllCodes() []cid.Cid {
	return []cid.Cid{
		(&state0{}).Code(),
		(&state2{}).Code(),
		(&state3{}).Code(),
		(&state4{}).Code(),
		(&state5{}).Code(),
		(&state6{}).Code(),
		(&state7{}).Code(),
		(&state8{}).Code(),
		(&state9{}).Code(),
		(&state10{}).Code(),
		(&state11{}).Code(),
		(&state12{}).Code(),
	}
}

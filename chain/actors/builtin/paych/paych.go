package paych

import (
	"encoding/base64"
	"fmt"

	"github.com/ipfs/go-cid"
	ipldcbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	paych0 "github.com/filecoin-project/specs-actors/actors/builtin/paych"
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

// Load returns an abstract copy of payment channel state, regardless of actor version
func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.PaychKey {
			return nil, xerrors.Errorf("actor code is not paych: %s", name)
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

		case actorstypes.Version13:
			return load13(store, act.Head)

		case actorstypes.Version14:
			return load14(store, act.Head)

		case actorstypes.Version15:
			return load15(store, act.Head)

		case actorstypes.Version16:
			return load16(store, act.Head)

		case actorstypes.Version17:
			return load17(store, act.Head)

		case actorstypes.Version18:
			return load18(store, act.Head)

		}
	}

	switch act.Code {

	case builtin0.PaymentChannelActorCodeID:
		return load0(store, act.Head)

	case builtin2.PaymentChannelActorCodeID:
		return load2(store, act.Head)

	case builtin3.PaymentChannelActorCodeID:
		return load3(store, act.Head)

	case builtin4.PaymentChannelActorCodeID:
		return load4(store, act.Head)

	case builtin5.PaymentChannelActorCodeID:
		return load5(store, act.Head)

	case builtin6.PaymentChannelActorCodeID:
		return load6(store, act.Head)

	case builtin7.PaymentChannelActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

// State is an abstract version of payment channel state that works across
// versions
type State interface {
	cbor.Marshaler

	Code() cid.Cid
	ActorKey() string
	ActorVersion() actorstypes.Version

	// Channel owner, who has funded the actor
	From() (address.Address, error)
	// Recipient of payouts from channel
	To() (address.Address, error)

	// Height at which the channel can be `Collected`
	SettlingAt() (abi.ChainEpoch, error)

	// Amount successfully redeemed through the payment channel, paid out on `Collect()`
	ToSend() (abi.TokenAmount, error)

	// Get total number of lanes
	LaneCount() (uint64, error)

	// Iterate lane states
	ForEachLaneState(cb func(idx uint64, dl LaneState) error) error

	GetState() interface{}
}

// LaneState is an abstract copy of the state of a single lane
type LaneState interface {
	Redeemed() (big.Int, error)
	Nonce() (uint64, error)
}

// DecodeSignedVoucher decodes base64 encoded signed voucher.
func DecodeSignedVoucher(s string) (*paychtypes.SignedVoucher, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	var sv paychtypes.SignedVoucher
	if err := ipldcbor.DecodeInto(data, &sv); err != nil {
		return nil, err
	}

	return &sv, nil
}

func Message(version actorstypes.Version, from address.Address) MessageBuilder {
	switch version {

	case actorstypes.Version0:
		return message0{from}

	case actorstypes.Version2:
		return message2{from}

	case actorstypes.Version3:
		return message3{from}

	case actorstypes.Version4:
		return message4{from}

	case actorstypes.Version5:
		return message5{from}

	case actorstypes.Version6:
		return message6{from}

	case actorstypes.Version7:
		return message7{from}

	case actorstypes.Version8:
		return message8{from}

	case actorstypes.Version9:
		return message9{from}

	case actorstypes.Version10:
		return message10{from}

	case actorstypes.Version11:
		return message11{from}

	case actorstypes.Version12:
		return message12{from}

	case actorstypes.Version13:
		return message13{from}

	case actorstypes.Version14:
		return message14{from}

	case actorstypes.Version15:
		return message15{from}

	case actorstypes.Version16:
		return message16{from}

	case actorstypes.Version17:
		return message17{from}

	case actorstypes.Version18:
		return message18{from}

	default:
		panic(fmt.Sprintf("unsupported actors version: %d", version))
	}
}

type MessageBuilder interface {
	Create(to address.Address, initialAmount abi.TokenAmount) (*types.Message, error)
	Update(paych address.Address, voucher *paychtypes.SignedVoucher, secret []byte) (*types.Message, error)
	Settle(paych address.Address) (*types.Message, error)
	Collect(paych address.Address) (*types.Message, error)
}

func toV0SignedVoucher(sv paychtypes.SignedVoucher) paych0.SignedVoucher {
	return paych0.SignedVoucher{
		ChannelAddr:     sv.ChannelAddr,
		TimeLockMin:     sv.TimeLockMin,
		TimeLockMax:     sv.TimeLockMax,
		SecretPreimage:  sv.SecretHash,
		Extra:           (*paych0.ModVerifyParams)(sv.Extra),
		Lane:            sv.Lane,
		Nonce:           sv.Nonce,
		Amount:          sv.Amount,
		MinSettleHeight: sv.MinSettleHeight,
		Merges:          nil,
		Signature:       sv.Signature,
	}
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
		(&state13{}).Code(),
		(&state14{}).Code(),
		(&state15{}).Code(),
		(&state16{}).Code(),
		(&state17{}).Code(),
		(&state18{}).Code(),
	}
}

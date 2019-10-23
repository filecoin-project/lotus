package validation

import(
	"fmt"
	"github.com/pkg/errors"

	"github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/libp2p/go-libp2p-core/peer"
)

//
// Most of the file is based on the abi/ package from go-filecoin.
//
var ErrInvalidType = fmt.Errorf("invalid type")

type Type uint64

const (
	// Invalid is the default value for 'Type' and represents an erroneously set type.
	Invalid = Type(iota)
	Address
	AttoFIL
	PeerID
)

type Value struct {
	Type Type
	Val  interface{}
}


func ToEncodedValues(params ...interface{}) ([]byte, error) {
	vals, err := ToValues(params)
	if err != nil {
		return nil, errors.Wrap(err, "unable to convert params to values")
	}

	bytes, err := EncodeValues(vals)
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode values")
	}

	return bytes, nil
}

func ToValues(i []interface{}) ([]*Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	out := make([]*Value, 0, len(i))
	for _, v := range i {
		switch v := v.(type) {
		case state.Address:
			out = append(out, &Value{Type: Address, Val: v})
		case state.AttoFIL:
			out = append(out, &Value{Type: AttoFIL, Val: v})
		case state.PeerID:
			out = append(out, &Value{Type: PeerID, Val: v})
		}
	}
	return out, nil
}

// EncodeValues encodes a set of values to raw bytes. Zero length arrays of
// values are normalized to nil
func EncodeValues(vals []*Value) ([]byte, error) {
	if len(vals) == 0 {
		return nil, nil
	}

	var arr [][]byte

	for _, val := range vals {
		data, err := val.Serialize()
		if err != nil {
			return nil, err
		}

		arr = append(arr, data)
	}

	return []byte{}, nil
}

type typeError struct {
	exp interface{}
	got interface{}
}

func (ate typeError) Error() string {
	return fmt.Sprintf("expected type %T, got %T", ate.exp, ate.got)
}


func (av *Value) Serialize() ([]byte, error) {
	switch av.Type {
	case Invalid:
		return nil, ErrInvalidType
	case Address:
		addr, ok := av.Val.(address.Address)
		if !ok {
			return nil, &typeError{address.Undef, av.Val}
		}
		return addr.Bytes(), nil
	case AttoFIL:
		ba, ok := av.Val.(types.BigInt)
		if !ok {
			return nil, &typeError{address.Undef, av.Val}
		}
		return ba.Bytes(), nil
	case PeerID:
		pid, ok := av.Val.(peer.ID)
		if !ok {
			return nil, &typeError{peer.ID(""), av.Val}
		}
		return []byte(pid), nil
	default:
		return nil, fmt.Errorf("unrecognized Type: %d", av.Type)
	}
}


package types

import (
	"encoding/json"
	"fmt"
	"math/big"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/polydawn/refmt/obj/atlas"
)

func init() {
	cbor.RegisterCborType(atlas.BuildEntry(BigInt{}).UseTag(2).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(i BigInt) ([]byte, error) {
				if i.Int == nil {
					return []byte{}, nil
				}

				return i.Bytes(), nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(x []byte) (BigInt, error) {
				return BigFromBytes(x), nil
			})).
		Complete())
}

var EmptyInt = BigInt{}

type BigInt struct {
	*big.Int
}

func NewInt(i uint64) BigInt {
	return BigInt{big.NewInt(0).SetUint64(i)}
}

func BigFromBytes(b []byte) BigInt {
	i := big.NewInt(0).SetBytes(b)
	return BigInt{i}
}

func BigFromString(s string) (BigInt, error) {
	v, ok := big.NewInt(0).SetString(s, 10)
	if !ok {
		return BigInt{}, fmt.Errorf("failed to parse string as a big int")
	}

	return BigInt{v}, nil
}

func BigMul(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Mul(a.Int, b.Int)}
}

func BigDiv(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Div(a.Int, b.Int)}
}

func BigAdd(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Add(a.Int, b.Int)}
}

func BigSub(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Sub(a.Int, b.Int)}
}

func BigCmp(a, b BigInt) int {
	return a.Int.Cmp(b.Int)
}

func (bi *BigInt) Nil() bool {
	return bi.Int == nil
}

func (bi *BigInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(bi.String())
}

func (bi *BigInt) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	i, ok := big.NewInt(0).SetString(s, 10)
	if !ok {
		if string(s) == "<nil>" {
			return nil
		}
		return fmt.Errorf("failed to parse bigint string")
	}

	bi.Int = i
	return nil
}

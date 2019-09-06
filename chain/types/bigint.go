package types

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/polydawn/refmt/obj/atlas"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

const BigIntMaxSerializedLen = 128 // is this big enough? or too big?

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
		return xerrors.Errorf("failed to parse bigint string: '%s'", string(b))
	}

	bi.Int = i
	return nil
}

func (bi *BigInt) MarshalCBOR(w io.Writer) error {
	if bi.Int == nil {
		zero := NewInt(0)
		return zero.MarshalCBOR(w)
	}

	if bi.Sign() < 0 {
		// right now we don't support negative integers.
		// In the spec, everything is listed as a Uint.
		return fmt.Errorf("BigInt does not support negative integers")
	}

	header := cbg.CborEncodeMajorType(cbg.MajTag, 2)
	if _, err := w.Write(header); err != nil {
		return err
	}

	b := bi.Bytes()

	header = cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(b)))
	if _, err := w.Write(header); err != nil {
		return err
	}

	if _, err := w.Write(b); err != nil {
		return err
	}

	return nil
}

func (bi *BigInt) UnmarshalCBOR(br io.Reader) error {
	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if maj != cbg.MajTag && extra != 2 {
		return fmt.Errorf("cbor input for big int was not a tagged big int")
	}

	maj, extra, err = cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if maj != cbg.MajByteString {
		return fmt.Errorf("cbor input for big int was not a tagged byte string")
	}

	if extra > BigIntMaxSerializedLen {
		return fmt.Errorf("big integer byte array too long")
	}

	buf := make([]byte, extra)
	if _, err := io.ReadFull(br, buf); err != nil {
		return err
	}

	bi.Int = big.NewInt(0).SetBytes(buf)

	return nil
}

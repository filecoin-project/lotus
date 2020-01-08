package types

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"

	"github.com/filecoin-project/lotus/build"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/polydawn/refmt/obj/atlas"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

const BigIntMaxSerializedLen = 128 // is this big enough? or too big?

var TotalFilecoinInt = FromFil(build.TotalFilecoin)

func init() {
	cbor.RegisterCborType(atlas.BuildEntry(BigInt{}).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(i BigInt) ([]byte, error) {
				return i.cborBytes(), nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(x []byte) (BigInt, error) {
				return fromCborBytes(x)
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

func FromFil(i uint64) BigInt {
	return BigMul(NewInt(i), NewInt(build.FilecoinPrecision))
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

func BigMod(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Mod(a.Int, b.Int)}
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

func (bi BigInt) Nil() bool {
	return bi.Int == nil
}

// LessThan returns true if bi < o
func (bi BigInt) LessThan(o BigInt) bool {
	return BigCmp(bi, o) < 0
}

// GreaterThan returns true if bi > o
func (bi BigInt) GreaterThan(o BigInt) bool {
	return BigCmp(bi, o) > 0
}

// Equals returns true if bi == o
func (bi BigInt) Equals(o BigInt) bool {
	return BigCmp(bi, o) == 0
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

var sizeUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB"}

func (bi BigInt) SizeStr() string {
	r := new(big.Rat).SetInt(bi.Int)
	den := big.NewRat(1, 1024)

	var i int
	for f, _ := r.Float64(); f >= 1024 && i+1 < len(sizeUnits); f, _ = r.Float64() {
		i++
		r = r.Mul(r, den)
	}

	f, _ := r.Float64()
	return fmt.Sprintf("%.3g %s", f, sizeUnits[i])
}

func (bi *BigInt) Scan(value interface{}) error {
	switch value := value.(type) {
	case string:
		i, ok := big.NewInt(0).SetString(value, 10)
		if !ok {
			if value == "<nil>" {
				return nil
			}
			return xerrors.Errorf("failed to parse bigint string: '%s'", value)
		}

		bi.Int = i

		return nil
	case int64:
		bi.Int = big.NewInt(value)
		return nil
	default:
		return xerrors.Errorf("non-string types unsupported: %T", value)
	}
}

func (bi *BigInt) cborBytes() []byte {
	if bi.Int == nil {
		return []byte{}
	}

	switch {
	case bi.Sign() > 0:
		return append([]byte{0}, bi.Bytes()...)
	case bi.Sign() < 0:
		return append([]byte{1}, bi.Bytes()...)
	default: //  bi.Sign() == 0:
		return []byte{}
	}
}

func fromCborBytes(buf []byte) (BigInt, error) {
	if len(buf) == 0 {
		return NewInt(0), nil
	}

	var negative bool
	switch buf[0] {
	case 0:
		negative = false
	case 1:
		negative = true
	default:
		return EmptyInt, fmt.Errorf("big int prefix should be either 0 or 1, got %d", buf[0])
	}

	i := big.NewInt(0).SetBytes(buf[1:])
	if negative {
		i.Neg(i)
	}

	return BigInt{i}, nil
}

func (bi *BigInt) MarshalCBOR(w io.Writer) error {
	if bi.Int == nil {
		zero := NewInt(0)
		return zero.MarshalCBOR(w)
	}

	enc := bi.cborBytes()

	header := cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(enc)))
	if _, err := w.Write(header); err != nil {
		return err
	}

	if _, err := w.Write(enc); err != nil {
		return err
	}

	return nil
}

func (bi *BigInt) UnmarshalCBOR(br io.Reader) error {
	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if maj != cbg.MajByteString {
		return fmt.Errorf("cbor input for fil big int was not a byte string (%x)", maj)
	}

	if extra == 0 {
		bi.Int = big.NewInt(0)
		return nil
	}

	if extra > BigIntMaxSerializedLen {
		return fmt.Errorf("big integer byte array too long")
	}

	buf := make([]byte, extra)
	if _, err := io.ReadFull(br, buf); err != nil {
		return err
	}

	i, err := fromCborBytes(buf)
	if err != nil {
		return err
	}

	*bi = i

	return nil
}

func (bi *BigInt) IsZero() bool {
	return bi.Int.Sign() == 0
}

package types

import (
	"fmt"
	"math/big"

	big2 "github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

const BigIntMaxSerializedLen = 128 // is this big enough? or too big?

var TotalFilecoinInt = FromFil(buildconstants.FilBase)

var EmptyInt = BigInt{}

type BigInt = big2.Int

func NewInt(i uint64) BigInt {
	return BigInt{Int: big.NewInt(0).SetUint64(i)}
}

func FromFil(i uint64) BigInt {
	return BigMul(NewInt(i), NewInt(buildconstants.FilecoinPrecision))
}

func BigFromBytes(b []byte) BigInt {
	i := big.NewInt(0).SetBytes(b)
	return BigInt{Int: i}
}

func BigFromString(s string) (BigInt, error) {
	return big2.FromString(s)
}

func BigMul(a, b BigInt) BigInt {
	return big2.Mul(a, b)
}

func BigDiv(a, b BigInt) BigInt {
	return big2.Div(a, b)
}

func BigDivFloat(num, den BigInt) float64 {
	if den.NilOrZero() {
		panic("divide by zero")
	}
	if num.NilOrZero() {
		return 0
	}
	res, _ := new(big.Rat).SetFrac(num.Int, den.Int).Float64()
	return res
}

func BigMod(a, b BigInt) BigInt {
	return big2.Mod(a, b)
}

func BigAdd(a, b BigInt) BigInt {
	return big2.Add(a, b)
}

func BigSub(a, b BigInt) BigInt {
	return big2.Sub(a, b)
}

func BigCmp(a, b BigInt) int {
	return big2.Cmp(a, b)
}

var byteSizeUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB"}

func SizeStr(bi BigInt) string {
	if bi.NilOrZero() {
		return "0 B"
	}

	r := new(big.Rat).SetInt(bi.Int)
	den := big.NewRat(1, 1024)

	var i int
	for f, _ := r.Float64(); f >= 1024 && i+1 < len(byteSizeUnits); f, _ = r.Float64() {
		i++
		r = r.Mul(r, den)
	}

	f, _ := r.Float64()
	return fmt.Sprintf("%.4g %s", f, byteSizeUnits[i])
}

var deciUnits = []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"}

func DeciStr(bi BigInt) string {
	if bi.NilOrZero() {
		return "0 B"
	}

	r := new(big.Rat).SetInt(bi.Int)
	den := big.NewRat(1, 1024)

	var i int
	for f, _ := r.Float64(); f >= 1024 && i+1 < len(deciUnits); f, _ = r.Float64() {
		i++
		r = r.Mul(r, den)
	}

	f, _ := r.Float64()
	return fmt.Sprintf("%.3g %s", f, deciUnits[i])
}

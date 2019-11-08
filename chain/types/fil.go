package types

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/filecoin-project/lotus/build"
)

// FILUnits prefix reference: https://en.wikipedia.org/wiki/Metric_prefix
var FILUnits = []string{"aFIL", "fFIL", "pFIL", "nFIL", "μFIL", "mFIL", "FIL", "kFIL", "MFIL", "GFIL"}

type FIL BigInt

func (f FIL) String() string {
	r := new(big.Rat).SetFrac(f.Int, big.NewInt(build.FilecoinPrecision))
	if r.Sign() == 0 {
		return "0"
	}
	// TODO: if 100 FIL, it will return 1
	return strings.TrimRight(strings.TrimRight(r.FloatString(18), "0"), ".")
}

func (f FIL) Format(s fmt.State, ch rune) {
	switch ch {
	case 'v': // print FIL with full fractional digits
		fmt.Fprint(s, f.String())
	case 's': // print FIL with prefix units
		i := 0
		prec := big.NewInt(1)
		base := big.NewInt(1000)
		for f.Cmp(base) >= 0 && i < len(FILUnits) - 1 {
			prec.SetInt64(base.Int64())
			base.Mul(base, big.NewInt(1000))
			i++
		}
		r, _ := new(big.Rat).SetFrac(f.Int, prec).Float64()
		fmt.Fprint(s, fmt.Sprintf("%g %s", r, FILUnits[i]))
	default:
		f.Int.Format(s, ch)
	}
}

func ParseFIL(s string) (FIL, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return FIL{}, fmt.Errorf("failed to parse %q as a decimal number", s)
	}

	r = r.Mul(r, big.NewRat(build.FilecoinPrecision, 1))
	if !r.IsInt() {
		return FIL{}, fmt.Errorf("invalid FIL value: %q", s)
	}

	return FIL{r.Num()}, nil
}

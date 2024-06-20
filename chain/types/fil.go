package types

import (
	"encoding"
	"fmt"
	"math/big"
	"strings"

	"github.com/invopop/jsonschema"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

type FIL BigInt

func (f FIL) String() string {
	if f.Int == nil {
		return "0 FIL"
	}
	return f.Unitless() + " FIL"
}

func (f FIL) Unitless() string {
	r := new(big.Rat).SetFrac(f.Int, big.NewInt(int64(buildconstants.FilecoinPrecision)))
	if r.Sign() == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(r.FloatString(18), "0"), ".")
}

var AttoFil = NewInt(1)
var FemtoFil = BigMul(AttoFil, NewInt(1000))
var PicoFil = BigMul(FemtoFil, NewInt(1000))
var NanoFil = BigMul(PicoFil, NewInt(1000))

var unitPrefixes = []string{"a", "f", "p", "n", "μ", "m"}

func (f FIL) Short() string {
	n := BigInt(f).Abs()

	dn := uint64(1)
	var prefix string
	for _, p := range unitPrefixes {
		if n.LessThan(NewInt(dn * 1000)) {
			prefix = p
			break
		}
		dn *= 1000
	}

	r := new(big.Rat).SetFrac(f.Int, big.NewInt(int64(dn)))
	if r.Sign() == 0 {
		return "0"
	}

	return strings.TrimRight(strings.TrimRight(r.FloatString(3), "0"), ".") + " " + prefix + "FIL"
}

func (f FIL) Nano() string {
	r := new(big.Rat).SetFrac(f.Int, big.NewInt(int64(1e9)))
	if r.Sign() == 0 {
		return "0"
	}

	return strings.TrimRight(strings.TrimRight(r.FloatString(9), "0"), ".") + " nFIL"
}

func (f FIL) Format(s fmt.State, ch rune) {
	switch ch {
	case 's', 'v':
		_, _ = fmt.Fprint(s, f.String())
	default:
		f.Int.Format(s, ch)
	}
}

func (f FIL) MarshalText() (text []byte, err error) {
	return []byte(f.String()), nil
}

func (f FIL) UnmarshalText(text []byte) error {
	if f.Int == nil {
		return fmt.Errorf("cannot unmarshal into nil BigInt (text:%s)", string(text))
	}

	p, err := ParseFIL(string(text))
	if err != nil {
		return err
	}

	f.Int.Set(p.Int)
	return nil
}

func ParseFIL(s string) (FIL, error) {
	suffix := strings.TrimLeft(s, "-.1234567890")
	s = s[:len(s)-len(suffix)]
	var attofil bool
	if suffix != "" {
		norm := strings.ToLower(strings.TrimSpace(suffix))
		switch norm {
		case "", "fil":
		case "attofil", "afil":
			attofil = true
		default:
			return FIL{}, fmt.Errorf("unrecognized suffix: %q", suffix)
		}
	}

	if len(s) > 50 {
		return FIL{}, fmt.Errorf("string length too large: %d", len(s))
	}

	r, ok := new(big.Rat).SetString(s) //nolint:gosec
	if !ok {
		return FIL{}, fmt.Errorf("failed to parse %q as a decimal number", s)
	}

	if !attofil {
		r = r.Mul(r, big.NewRat(int64(buildconstants.FilecoinPrecision), 1))
	}

	if !r.IsInt() {
		var pref string
		if attofil {
			pref = "atto"
		}
		return FIL{}, fmt.Errorf("invalid %sFIL value: %q", pref, s)
	}

	return FIL{r.Num()}, nil
}

func MustParseFIL(s string) FIL {
	n, err := ParseFIL(s)
	if err != nil {
		panic(err)
	}

	return n
}

func (f FIL) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:    "string",
		Pattern: `^((\d+(\.\d+)?|0x[0-9a-fA-F]+))( ([aA]([tT][tT][oO])?)?[fF][iI][lL])?$`,
	}
}

var _ encoding.TextMarshaler = (*FIL)(nil)
var _ encoding.TextUnmarshaler = (*FIL)(nil)

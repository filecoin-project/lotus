package types

import (
	"fmt"
	"math"
	"strconv"

	"golang.org/x/xerrors"
)

// Percent stores a signed percentage as an int64. When converted to a string (or json), it's stored
// as a decimal with two places (e.g., 100% -> 1.00).
type Percent int64

func (p Percent) String() string {
	abs := p
	sign := ""
	if abs < 0 {
		abs = -abs
		sign = "-"
	}
	return fmt.Sprintf(`%s%d.%d`, sign, abs/100, abs%100)
}

func (p Percent) MarshalJSON() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *Percent) UnmarshalJSON(b []byte) error {
	flt, err := strconv.ParseFloat(string(b)+"e2", 64)
	if err != nil {
		return xerrors.Errorf("unable to parse ratio %s: %w", string(b), err)
	}
	if math.Trunc(flt) != flt {
		return xerrors.Errorf("ratio may only have two decimals: %s", string(b))
	}
	*p = Percent(flt)
	return nil
}

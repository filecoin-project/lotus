package cli

import (
	"fmt"
	"math/big"

	"github.com/filecoin-project/lotus/chain/types"
)

var Units = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB"}

func SizeStr(size types.BigInt) string {
	r := new(big.Rat).SetInt(size.Int)
	den := big.NewRat(1, 1024)

	var i int
	for f, _ := r.Float64(); f >= 1024 && 1 < len(Units); f, _ = r.Float64() {
		i++
		r = r.Mul(r, den)
	}

	f, _ := r.Float64()
	return fmt.Sprintf("%.3f %s", f, Units[i])
}

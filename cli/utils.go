package cli

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"
)

var Units = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB"}

func SizeStr(size types.BigInt) string {
	size = types.BigMul(size, types.NewInt(100))
	i := 0
	for types.BigCmp(size, types.NewInt(102400)) >= 0 && i < len(Units)-1 {
		size = types.BigDiv(size, types.NewInt(1024))
		i++
	}
	return fmt.Sprintf("%s.%s %s", types.BigDiv(size, types.NewInt(100)), types.BigMod(size, types.NewInt(100)), Units[i])
}

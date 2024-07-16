package buildconstants

import (
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-address"
)

// moved from now-defunct build/paramfetch.go
var log = logging.Logger("build/buildtypes")

func SetAddressNetwork(n address.Network) {
	address.CurrentNetwork = n
}

func MustParseAddress(addr string) address.Address {
	ret, err := address.NewFromString(addr)
	if err != nil {
		panic(err)
	}

	return ret
}

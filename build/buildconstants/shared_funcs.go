package buildconstants

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
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

func MustParseCid(c string) cid.Cid {
	ret, err := cid.Decode(c)
	if err != nil {
		panic(err)
	}

	return ret
}

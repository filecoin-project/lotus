package discoveryimpl

import (
	"github.com/filecoin-project/go-fil-markets/discovery"
)

func Multi(r discovery.PeerResolver) discovery.PeerResolver { // TODO: actually support multiple mechanisms
	return r
}

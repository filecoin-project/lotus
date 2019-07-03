package modules

import (
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peerstore"
	record "github.com/libp2p/go-libp2p-record"
)

var log = logging.Logger("modules")

// RecordValidator provides namesys compatible routing record validator
func RecordValidator(ps peerstore.Peerstore) record.Validator {
	return record.NamespacedValidator{
		"pk": record.PublicKeyValidator{},
	}
}

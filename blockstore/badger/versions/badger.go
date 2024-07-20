package versions

import (
	"errors"

	badgerV2 "github.com/dgraph-io/badger/v2"
	badgerV4 "github.com/dgraph-io/badger/v4"
)

// Options embeds the badger options themselves, and augments them with
// blockstore-specific options.
type Options struct {
	// BadgerVersion sets the release version of badger to use
	BadgerVersion int

	// Prefix is an optional prefix to prepend to keys. Default: "".
	Prefix string
}

func OpenBadgerDB(opts Options) (BadgerDB, error) {
	var db BadgerDB
	var err error

	prefix := opts.Prefix
	version := opts.BadgerVersion

	switch version {
	case 4:
		opts := badgerV4.DefaultOptions(prefix)
		var dbV4 *badgerV4.DB
		dbV4, err = badgerV4.Open(opts)
		if err == nil {
			db = BadgerDB(&BadgerV4{dbV4})
		}
	case 2:
		opts := badgerV2.DefaultOptions(prefix)
		var dbV2 *badgerV2.DB
		dbV2, err = badgerV2.Open(opts)
		if err == nil {
			db = BadgerDB(&BadgerV2{dbV2})
		}
	default:
		return nil, errors.New("unsupported badger version")
	}

	if err != nil {
		return nil, err
	}
	return db, nil
}

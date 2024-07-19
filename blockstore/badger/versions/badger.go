package versions

import (
	"errors"

	badgerV2 "github.com/dgraph-io/badger/v2"
	badgerV4 "github.com/dgraph-io/badger/v4"
)

func openBadgerDB(path string, inMemory bool, version string) (BadgerDB, error) {
	var db BadgerDB
	var err error

	switch version {
	case "v4":
		opts := badgerV4.DefaultOptions(path)
		opts.InMemory = inMemory
		var dbV4 *badgerV4.DB
		dbV4, err = badgerV4.Open(opts)
		if err == nil {
			db = BadgerDB(&BadgerV4{dbV4})
		}
	case "v2":
		opts := badgerV2.DefaultOptions(path)
		opts.InMemory = inMemory
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

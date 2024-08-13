package versions

import (
	"errors"

	badgerV2 "github.com/dgraph-io/badger/v2"
	badgerV4 "github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

// BadgerLogger is a local wrapper for go-log to make the interface
// compatible with badger.Logger (namely, aliasing Warnf to Warningf)
type BadgerLogger struct {
	*zap.SugaredLogger // skips 1 caller to get useful line info, skipping over badger.Options.

	Skip2 *zap.SugaredLogger // skips 2 callers, just like above + this logger.
}

// Warningf is required by the badger logger APIs.
func (b *BadgerLogger) Warningf(format string, args ...interface{}) {
	b.Skip2.Warnf(format, args...)
}
func OpenBadgerDB(opts Options) (BadgerDB, error) {
	var db BadgerDB
	var err error

	version := opts.BadgerVersion

	switch version {
	case 4:
		var dbV4 *badgerV4.DB
		dbV4, err = badgerV4.Open(opts.V4Options)
		if err == nil {
			db = BadgerDB(&BadgerV4{dbV4})
		}
	case 2:
		var dbV2 *badgerV2.DB
		dbV2, err = badgerV2.Open(opts.V2Options)
		if err == nil {
			db = BadgerDB(&BadgerV2{dbV2})
		}
	default:
		err = fmt.Errorf("unsupported badger version: %v", opts.BadgerVersion)
	}

	if err != nil {
		return nil, err
	}
	return db, nil
}

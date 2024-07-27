package versions

import (
	"errors"

	badgerV2 "github.com/dgraph-io/badger/v2"
	badgerV4 "github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

// Options embeds the badger options themselves, and augments them with
// blockstore-specific options.
type Options struct {

	badgerV2.Options

	// BadgerVersion sets the release version of badger to use
	BadgerVersion int

	// Prefix is an optional prefix to prepend to keys. Default: "".
	Prefix string

	BadgerLogger badgerLogger
}

// badgerLogger is a local wrapper for go-log to make the interface
// compatible with badger.Logger (namely, aliasing Warnf to Warningf)
type badgerLogger struct {
	*zap.SugaredLogger // skips 1 caller to get useful line info, skipping over badger.Options.

	skip2 *zap.SugaredLogger // skips 2 callers, just like above + this logger.
}

// Warningf is required by the badger logger APIs.
func (b *badgerLogger) Warningf(format string, args ...interface{}) {
	b.skip2.Warnf(format, args...)
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

func clamp(x, min, max int) int {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}

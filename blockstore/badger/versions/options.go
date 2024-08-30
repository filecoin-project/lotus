package versions

import (
	"os"
	"strconv"

	badgerV2 "github.com/dgraph-io/badger/v2"
	optionsV2 "github.com/dgraph-io/badger/v2/options"
	badgerV4 "github.com/dgraph-io/badger/v4"
	optionsV4 "github.com/dgraph-io/badger/v4/options"
)

// Options embeds the badger options themselves, and augments them with
// blockstore-specific options.
type Options struct {
	V2Options badgerV2.Options
	V4Options badgerV4.Options

	// BadgerVersion sets the release version of badger to use
	BadgerVersion int

	Prefix     string
	Dir        string
	ValueDir   string
	SyncWrites bool

	Logger BadgerLogger
}

func (o *Options) SetDir(dir string) {
	o.Dir = dir
	o.V2Options.Dir = dir
	o.V4Options.Dir = dir
}

func (o *Options) SetValueDir(valueDir string) {
	o.ValueDir = valueDir
	o.V2Options.ValueDir = valueDir
	o.V4Options.ValueDir = valueDir
}

func BlockStoreOptions(path string, readonly bool, badgerVersion int) Options {
	opts := DefaultOptions(path, readonly, badgerVersion)

	// Due to legacy usage of blockstore.Blockstore, over a datastore, all
	// blocks are prefixed with this namespace. In the future, this can go away,
	// in order to shorten keys, but it'll require a migration.
	opts.Prefix = "/blocks/"

	// Disable Snappy Compression
	opts.V4Options.Compression = optionsV4.None

	// Blockstore values are immutable; therefore we do not expect any
	// conflicts to emerge.
	opts.V2Options.DetectConflicts = false
	opts.V4Options.DetectConflicts = false

	// This is to optimize the database on close so it can be opened
	// read-only and efficiently queried.
	opts.V2Options.CompactL0OnClose = true
	opts.V4Options.CompactL0OnClose = true

	// The alternative is "crash on start and tell the user to fix it". This
	// will truncate corrupt and unsynced data, which we don't guarantee to
	// persist anyways.
	// Badger V4 has no such option
	opts.V2Options.Truncate = true

	// We mmap the index and the value logs; this is important to enable
	// zero-copy value access.
	opts.V2Options.ValueLogLoadingMode = optionsV2.MemoryMap
	opts.V2Options.TableLoadingMode = optionsV2.MemoryMap

	// Embed only values < 128 bytes in the LSM tree; larger values are stored
	// in value logs.
	opts.V2Options.ValueThreshold = 128
	opts.V4Options.ValueThreshold = 128

	// Default table size is already 64MiB. This is here to make it explicit.
	// Badger V4 removed the option
	opts.V2Options.MaxTableSize = 64 << 20

	// NOTE: The chain blockstore doesn't require any GC (blocks are never
	// deleted). This will change if we move to a tiered blockstore.

	opts.V2Options.ReadOnly = readonly
	opts.V4Options.ReadOnly = readonly

	// Envvar LOTUS_CHAIN_BADGERSTORE_COMPACTIONWORKERNUM
	// Allows the number of compaction workers used by BadgerDB to be adjusted
	// Unset - leaves the default number of compaction workers (4)
	// "0" - disables compaction
	// Positive integer - enables that number of compaction workers
	if badgerNumCompactors, badgerNumCompactorsSet := os.LookupEnv("LOTUS_CHAIN_BADGERSTORE_COMPACTIONWORKERNUM"); badgerNumCompactorsSet {
		if numWorkers, err := strconv.Atoi(badgerNumCompactors); err == nil && numWorkers >= 0 {
			opts.V2Options.NumCompactors = numWorkers
			opts.V4Options.NumCompactors = numWorkers
		}
	}

	return opts
}

func DefaultOptions(path string, readonly bool, badgerVersion int) Options {
	var opts Options

	opts.BadgerVersion = badgerVersion

	opts.SetDir(path)
	opts.SetValueDir(path)

	//v2
	bopts := badgerV2.DefaultOptions(path)
	opts.V2Options = bopts

	//v4
	boptsv4 := badgerV4.DefaultOptions(path)
	opts.V4Options = boptsv4

	return opts
}

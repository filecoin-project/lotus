package repo

import (
	badgerbs "github.com/filecoin-project/lotus/lib/blockstore/badger"
	"github.com/filecoin-project/lotus/system"
)

// BadgerBlockstoreOptions returns the badger options to apply for the provided
// domain.
func BadgerBlockstoreOptions(domain BlockstoreDomain, path string, readonly bool) (badgerbs.Options, error) {
	if domain != BlockstoreChain {
		return badgerbs.Options{}, ErrInvalidBlockstoreDomain
	}

	opts := badgerbs.DefaultOptions(path)

	// Due to legacy usage of blockstore.Blockstore, over a datastore, all
	// blocks are prefixed with this namespace. In the future, this can go away,
	// in order to shorten keys, but it'll require a migration.
	opts.Prefix = "/blocks/"

	// Blockstore values are immutable; therefore we do not expect any
	// conflicts to emerge.
	opts.DetectConflicts = false

	// This is to optimize the database on close so it can be opened
	// read-only and efficiently queried.
	opts.CompactL0OnClose = true

	// The alternative is "crash on start and tell the user to fix it". This
	// will truncate corrupt and unsynced data, which we don't guarantee to
	// persist anyways.
	opts.Truncate = true

	// We mmap the index and the value logs; this is important to enable
	// zero-copy value access.
	opts.ValueLogLoadingMode = badgerbs.MemoryMap
	opts.TableLoadingMode = badgerbs.MemoryMap

	// Embed only values < 128 bytes in the LSM tree; larger values are stored
	// in value logs.
	opts.ValueThreshold = 128

	// Default table size is already 64MiB. This is here to make it explicit.
	opts.MaxTableSize = 64 << 20

	// If we don't set an index cache size, badger will retain all indices from
	// all tables _in memory_. This is quite counter intuitive, but it's true.
	// See badger/table.Table#initIndex.
	//
	// We vary the cache size depending on the configured system limits, taking
	// up to 20% of the configured limit, with a min of 256MiB, and a max
	// of 1GiB.
	var icachesize int64
	switch icachesize = int64(float64(system.ResourceConstraints.EffectiveMemLimit) * 0.20); {
	case icachesize < 256<<20: // 256MiB.
		icachesize = 256 << 20
	case icachesize > 1<<30: // 1GiB.
		icachesize = 1 << 30
	}
	opts.IndexCacheSize = icachesize

	// NOTE: The chain blockstore doesn't require any GC (blocks are never
	// deleted). This will change if we move to a tiered blockstore.

	opts.ReadOnly = readonly

	return opts, nil
}

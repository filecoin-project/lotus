package versions

import (
	"os"
	"strconv"

	badgerV2 "github.com/dgraph-io/badger/v2"
	optionsV2 "github.com/dgraph-io/badger/v2/options"

	badgerV4 "github.com/dgraph-io/badger/v4"
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
	opts.Prefix = "/blocks/"

	opts.V2Options.DetectConflicts = false
	opts.V2Options.CompactL0OnClose = true
	opts.V2Options.Truncate = true
	opts.V2Options.ValueLogLoadingMode = optionsV2.MemoryMap
	opts.V2Options.TableLoadingMode = optionsV2.MemoryMap
	opts.V2Options.ValueThreshold = 128
	opts.V2Options.MaxTableSize = 64 << 20
	opts.V2Options.ReadOnly = readonly

	opts.V4Options.DetectConflicts = false
	opts.V4Options.CompactL0OnClose = true
	opts.V4Options.ValueThreshold = 148
	opts.V4Options.ReadOnly = readonly

	// Envvar LOTUS_CHAIN_BADGERSTORE_COMPACTIONWORKERNUM
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

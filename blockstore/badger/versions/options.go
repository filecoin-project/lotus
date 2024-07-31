package versions

import (
	"os"
	"strconv"

	badgerV2 "github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
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

func DefaultOptions(path string, readonly bool) Options {
	var opts Options

	/*
		TODO determine where this code came from and if it needs to be added back somewhere
		//opts.Prefix = "/blocks/"
	*/

	//TODO remove hard code version # and connect config
	opts.BadgerVersion = 2

	opts.SetDir(path)
	opts.SetValueDir(path)

	//v2
	bopts := badgerV2.DefaultOptions(path)
	bopts.DetectConflicts = false
	bopts.CompactL0OnClose = true
	bopts.Truncate = true
	bopts.ValueLogLoadingMode = options.MemoryMap
	bopts.TableLoadingMode = options.MemoryMap
	bopts.ValueThreshold = 128
	bopts.MaxTableSize = 64 << 20
	bopts.ReadOnly = readonly

	// Envvar LOTUS_CHAIN_BADGERSTORE_COMPACTIONWORKERNUM
	if badgerNumCompactors, badgerNumCompactorsSet := os.LookupEnv("LOTUS_CHAIN_BADGERSTORE_COMPACTIONWORKERNUM"); badgerNumCompactorsSet {
		if numWorkers, err := strconv.Atoi(badgerNumCompactors); err == nil && numWorkers >= 0 {
			bopts.NumCompactors = numWorkers
		}
	}
	opts.V2Options = bopts

	//v4

	boptsv4 := badgerV4.DefaultOptions(path)
	boptsv4.ReadOnly = readonly

	// Envvar LOTUS_CHAIN_BADGERSTORE_COMPACTIONWORKERNUM
	if badgerNumCompactors, badgerNumCompactorsSet := os.LookupEnv("LOTUS_CHAIN_BADGERSTORE_COMPACTIONWORKERNUM"); badgerNumCompactorsSet {
		if numWorkers, err := strconv.Atoi(badgerNumCompactors); err == nil && numWorkers >= 0 {
			boptsv4.NumCompactors = numWorkers
		}
	}
	opts.V4Options = boptsv4

	return opts
}

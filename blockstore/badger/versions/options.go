package versions

import (
	"os"
	"strconv"

	badgerV2 "github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	badgerV4 "github.com/dgraph-io/badger/v4"
)

func DefaultOptions(path string, readonly bool) Options {
	var opts Options
	opts.Prefix = "/blocks/"
	opts.BadgerVersion = 2

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

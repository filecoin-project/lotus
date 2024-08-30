package repo

import (
	versions "github.com/filecoin-project/lotus/blockstore/badger/versions"
)

// BadgerBlockstoreOptions returns the badger options to apply for the provided
// domain.
func BadgerBlockstoreOptions(domain BlockstoreDomain, path string, readonly bool, badgerVersion int) (versions.Options, error) {
	opts := versions.BlockStoreOptions(path, readonly, badgerVersion)
	return opts, nil
}

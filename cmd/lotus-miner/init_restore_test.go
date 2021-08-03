package main

import (
	"os"
	"testing"

	ds "github.com/ipfs/go-datastore"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/lib/backupds"
	"golang.org/x/xerrors"
)

func TestRestore(t *testing.T) {
	require.NoError(t, runBackup())
}

func runBackup() error {
	bf := "/tmp/backup.cbor"

	f, err := os.Open(bf)
	if err != nil {
		return xerrors.Errorf("opening backup file: %w", err)
	}
	defer f.Close() // nolint:errcheck

	dstore := ds.NewMapDatastore()
	err = backupds.RestoreInto(f, dstore)

	if err != nil {
		return xerrors.Errorf("restoring metadata: %w", err)
	}

	return nil
}

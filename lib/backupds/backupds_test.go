package backupds

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"
)

const valSize = 512 << 10

func putVals(t *testing.T, ds datastore.Datastore, start, end int) {
	for i := start; i < end; i++ {
		err := ds.Put(context.TODO(), datastore.NewKey(fmt.Sprintf("%d", i)), []byte(fmt.Sprintf("%d-%s", i, strings.Repeat("~", valSize))))
		require.NoError(t, err)
	}
}

func checkVals(t *testing.T, ds datastore.Datastore, start, end int, exist bool) {
	for i := start; i < end; i++ {
		v, err := ds.Get(context.TODO(), datastore.NewKey(fmt.Sprintf("%d", i)))
		if exist {
			require.NoError(t, err)
			expect := []byte(fmt.Sprintf("%d-%s", i, strings.Repeat("~", valSize)))
			require.EqualValues(t, expect, v)
		} else {
			require.ErrorIs(t, err, datastore.ErrNotFound)
		}
	}
}

func TestNoLogRestore(t *testing.T) {
	ds1 := datastore.NewMapDatastore()

	putVals(t, ds1, 0, 10)

	bds, err := Wrap(ds1, NoLogdir)
	require.NoError(t, err)

	var bup bytes.Buffer
	require.NoError(t, bds.Backup(context.TODO(), &bup))

	putVals(t, ds1, 10, 20)

	ds2 := datastore.NewMapDatastore()
	require.NoError(t, RestoreInto(&bup, ds2))

	checkVals(t, ds2, 0, 10, true)
	checkVals(t, ds2, 10, 20, false)
}

func TestLogRestore(t *testing.T) {
	logdir := t.TempDir()

	ds1 := datastore.NewMapDatastore()

	putVals(t, ds1, 0, 10)

	bds, err := Wrap(ds1, logdir)
	require.NoError(t, err)

	putVals(t, bds, 10, 20)

	require.NoError(t, bds.Close())

	fls, err := os.ReadDir(logdir)
	require.NoError(t, err)
	require.Equal(t, 1, len(fls))

	bf, err := os.ReadFile(filepath.Join(logdir, fls[0].Name()))
	require.NoError(t, err)

	ds2 := datastore.NewMapDatastore()
	require.NoError(t, RestoreInto(bytes.NewReader(bf), ds2))

	checkVals(t, ds2, 0, 20, true)
}

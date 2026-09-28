package paths

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const pathSize = 16 << 20

type TestingLocalStorage struct {
	root string
	c    storiface.StorageConfig
}

func (t *TestingLocalStorage) DiskUsage(path string) (int64, error) {
	return 1, nil
}

func (t *TestingLocalStorage) GetStorage() (storiface.StorageConfig, error) {
	return t.c, nil
}

func (t *TestingLocalStorage) SetStorage(f func(*storiface.StorageConfig)) error {
	f(&t.c)
	return nil
}

func (t *TestingLocalStorage) Stat(path string) (fsutil.FsStat, error) {
	return fsutil.FsStat{
		Capacity:    pathSize,
		Available:   pathSize,
		FSAvailable: pathSize,
	}, nil
}

func (t *TestingLocalStorage) init(subpath string) error {
	path := filepath.Join(t.root, subpath)
	if err := os.Mkdir(path, 0755); err != nil {
		return err
	}

	metaFile := filepath.Join(path, MetaFile)

	meta := &storiface.LocalStorageMeta{
		ID:       storiface.ID(uuid.New().String()),
		Weight:   1,
		CanSeal:  true,
		CanStore: true,
	}

	mb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(metaFile, mb, 0644); err != nil {
		return err
	}

	return nil
}

var _ LocalStorage = &TestingLocalStorage{}

func TestLocalStorage(t *testing.T) {
	ctx := context.TODO()

	root := t.TempDir()

	tstor := &TestingLocalStorage{
		root: root,
	}

	index := NewMemIndex(nil)

	st, err := NewLocal(ctx, tstor, index, nil)
	require.NoError(t, err)

	p1 := "1"
	require.NoError(t, tstor.init("1"))

	err = st.OpenPath(ctx, filepath.Join(tstor.root, p1))
	require.NoError(t, err)

	// TODO: put more things here
}

func TestLocalStorageSkipsHiddenFiles(t *testing.T) {
	ctx := context.TODO()

	root := t.TempDir()

	tstor := &TestingLocalStorage{
		root: root,
	}

	index := NewMemIndex(nil)

	st, err := NewLocal(ctx, tstor, index, nil)
	require.NoError(t, err)

	p1 := filepath.Join(tstor.root, "1")
	require.NoError(t, tstor.init("1"))

	sealedDir := filepath.Join(p1, storiface.FTSealed.String())
	require.NoError(t, os.Mkdir(sealedDir, 0755))

	// hidden files, e.g. NFS silly-rename leftovers, must not break the scan
	require.NoError(t, os.WriteFile(filepath.Join(sealedDir, ".nfs0000000000000001"), nil, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sealedDir, "s-t01000-1"), nil, 0644))

	require.NoError(t, st.OpenPath(ctx, p1))

	decls, err := index.StorageList(ctx)
	require.NoError(t, err)

	var sectors []storiface.Decl
	for _, d := range decls {
		sectors = append(sectors, d...)
	}
	require.Len(t, sectors, 1)
	require.Equal(t, abi.SectorID{Miner: 1000, Number: 1}, sectors[0].SectorID)
}

package stores

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const pathSize = 16 << 20

type TestingLocalStorage struct {
	root string
	c    StorageConfig
}

func (t *TestingLocalStorage) GetStorage() (StorageConfig, error) {
	return t.c, nil
}

func (t *TestingLocalStorage) SetStorage(f func(*StorageConfig)) error {
	f(&t.c)
	return nil
}

func (t *TestingLocalStorage) Stat(path string) (FsStat, error) {
	return FsStat{
		Capacity:  pathSize,
		Available: pathSize,
		Used:      0,
	}, nil
}

func (t *TestingLocalStorage) init(subpath string) error {
	path := filepath.Join(t.root, subpath)
	if err := os.Mkdir(path, 0755); err != nil {
		return err
	}

	metaFile := filepath.Join(path, MetaFile)

	meta := &LocalStorageMeta{
		ID:       ID(uuid.New().String()),
		Weight:   1,
		CanSeal:  true,
		CanStore: true,
	}

	mb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(metaFile, mb, 0644); err != nil {
		return err
	}

	return nil
}

var _ LocalStorage = &TestingLocalStorage{}

func TestLocalStorage(t *testing.T) {
	ctx := context.TODO()

	root, err := ioutil.TempDir("", "sector-storage-teststorage-")
	require.NoError(t, err)

	tstor := &TestingLocalStorage{
		root: root,
	}

	index := NewIndex()

	st, err := NewLocal(ctx, tstor, index, nil)
	require.NoError(t, err)

	p1 := "1"
	require.NoError(t, tstor.init("1"))

	err = st.OpenPath(ctx, filepath.Join(tstor.root, p1))
	require.NoError(t, err)

	// TODO: put more things here
}

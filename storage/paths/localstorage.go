package paths

import (
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type BasicLocalStorage struct {
	PathToJSON string
}

var _ LocalStorage = &BasicLocalStorage{}

func (ls *BasicLocalStorage) GetStorage() (storiface.StorageConfig, error) {
	var def storiface.StorageConfig
	c, err := config.StorageFromFile(ls.PathToJSON, &def)
	if err != nil {
		return storiface.StorageConfig{}, err
	}
	return *c, nil
}

func (ls *BasicLocalStorage) SetStorage(f func(*storiface.StorageConfig)) error {
	var def storiface.StorageConfig
	c, err := config.StorageFromFile(ls.PathToJSON, &def)
	if err != nil {
		return err
	}
	f(c)
	return config.WriteStorageFile(ls.PathToJSON, *c)
}

func (ls *BasicLocalStorage) Stat(path string) (fsutil.FsStat, error) {
	return fsutil.Statfs(path)
}

func (ls *BasicLocalStorage) DiskUsage(path string) (int64, error) {
	si, err := fsutil.FileSize(path)
	if err != nil {
		return 0, err
	}
	return si.OnDisk, nil
}

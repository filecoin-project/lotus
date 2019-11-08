package sectorbuilder

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func TempSectorbuilder(sectorSize uint64, ds dtypes.MetadataDS) (*SectorBuilder, func(), error) {
	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		return nil, nil, err
	}

	sb, err := TempSectorbuilderDir(dir, sectorSize, ds)
	return sb, func() {
		sb.Destroy()

		if err := os.RemoveAll(dir); err != nil {
			log.Warn("failed to clean up temp sectorbuilder: ", err)
		}
	}, err
}

func TempSectorbuilderDir(dir string, sectorSize uint64, ds dtypes.MetadataDS) (*SectorBuilder, error) {
	addr, err := address.NewFromString("t3vfxagwiegrywptkbmyohqqbfzd7xzbryjydmxso4hfhgsnv6apddyihltsbiikjf3lm7x2myiaxhuc77capq")
	if err != nil {
		return nil, err
	}

	metadata := filepath.Join(dir, "meta")
	sealed := filepath.Join(dir, "sealed")
	staging := filepath.Join(dir, "staging")
	cache := filepath.Join(dir, "cache")

	sb, err := New(&Config{
		SectorSize: sectorSize,

		SealedDir:   sealed,
		StagedDir:   staging,
		MetadataDir: metadata,
		CacheDir:    cache,

		WorkerThreads: 2,
		Miner:         addr,
	}, ds)
	if err != nil {
		return nil, err
	}

	return sb, nil
}

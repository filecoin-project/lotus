package sectorbuilder

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func TempSectorbuilderDir(dir string, sectorSize uint64, ds dtypes.MetadataDS) (*SectorBuilder, error) {
	addr, err := address.NewFromString("t3vfxagwiegrywptkbmyohqqbfzd7xzbryjydmxso4hfhgsnv6apddyihltsbiikjf3lm7x2myiaxhuc77capq")
	if err != nil {
		return nil, err
	}

	sb, err := New(&Config{
		SectorSize: sectorSize,

		Dir: dir,

		WorkerThreads: 2,
		Miner:         addr,
	}, ds)
	if err != nil {
		return nil, err
	}

	return sb, nil
}

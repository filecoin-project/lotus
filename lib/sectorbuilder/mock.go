package sectorbuilder

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/filecoin-project/lotus/chain/address"
)

func TempSectorbuilder(sectorSize uint64) (*SectorBuilder, func(), error) {
	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		return nil, nil, err
	}

	addr, err := address.NewFromString("t3vfxagwiegrywptkbmyohqqbfzd7xzbryjydmxso4hfhgsnv6apddyihltsbiikjf3lm7x2myiaxhuc77capq")
	if err != nil {
		return nil, nil, err
	}

	metadata := filepath.Join(dir, "meta")
	sealed := filepath.Join(dir, "sealed")
	staging := filepath.Join(dir, "staging")

	sb, err := New(&SectorBuilderConfig{
		SectorSize:  sectorSize,
		SealedDir:   sealed,
		StagedDir:   staging,
		MetadataDir: metadata,
		Miner:       addr,
	})
	if err != nil {
		return nil, nil, err
	}

	return sb, func() {
		if err := os.RemoveAll(dir); err != nil {
			log.Warn("failed to clean up temp sectorbuilder: ", err)
		}
	}, nil
}

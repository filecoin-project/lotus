package seed

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

func WriteGenesisMiner(maddr address.Address, sbroot string, gm *genesis.GenesisMiner) error {
	output := map[string]genesis.GenesisMiner{
		maddr.String(): *gm,
	}

	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(filepath.Join(sbroot, "pre-seal-"+maddr.String()+".json"), out, 0664); err != nil {
		return err
	}

	return nil
}

func WritePrepInfo(sbroot string, meta *GenesisMinerMeta, tasks []SealBatch) error {
	if err := WriteJson(filepath.Join(sbroot, "meta-"+meta.MinerAddr.String()+".json"), meta); err != nil {
		return xerrors.Errorf("writing preseal metadata: %w", err)
	}

	for i, task := range tasks {
		fname := fmt.Sprintf("batch-%d-%s.json", i, task.MinerAddr.String())

		if err := WriteJson(filepath.Join(sbroot, fname), &task); err != nil {
			return xerrors.Errorf("writing preseal metadata: %w", err)
		}
	}

	return nil
}

func WriteJson(file string, i interface{}) error {
	out, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(file), out, 0664)
}

func mksbcfg(sbroot string, maddr address.Address, ssize uint64) *sectorbuilder.Config {
	return &sectorbuilder.Config{
		Miner:         maddr,
		SectorSize:    ssize,
		CacheDir:      filepath.Join(sbroot, "cache"),
		SealedDir:     filepath.Join(sbroot, "sealed"),
		StagedDir:     filepath.Join(sbroot, "staging"),
		UnsealedDir:   filepath.Join(sbroot, "unsealed"),
		WorkerThreads: 2,
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (n int, err error) {
	// the compiler is probably smart enough to optimize this nicely
	for i := range p {
		p[i] = 0
	}

	return len(p), nil
}

var zeroR io.Reader = &zeroReader{}

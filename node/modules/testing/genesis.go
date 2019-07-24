package testing

import (
	"os"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/node/modules"
)

var glog = logging.Logger("genesis")

func MakeGenesis(outFile string) func(bs blockstore.Blockstore, w *chain.Wallet) modules.Genesis {
	return func(bs blockstore.Blockstore, w *chain.Wallet) modules.Genesis {
		return func() (*chain.BlockHeader, error) {
			glog.Warn("Generating new random genesis block, note that this SHOULD NOT happen unless you are setting up new network")
			b, err := chain.MakeGenesisBlock(bs, w)
			if err != nil {
				return nil, err
			}

			f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return nil, err
			}

			genBytes, err := b.Genesis.Serialize()
			if err != nil {
				return nil, err
			}

			if _, err := f.Write(genBytes); err != nil {
				return nil, err
			}

			glog.Warnf("WRITING GENESIS FILE AT %s", f.Name())

			if err := f.Close(); err != nil {
				return nil, err
			}

			return b.Genesis, nil
		}
	}
}

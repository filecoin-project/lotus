package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var glog = logging.Logger("genesis")

func MakeGenesisMem(out io.Writer, gmc *gen.GenMinerCfg) func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
	return func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
		return func() (*types.BlockHeader, error) {
			glog.Warn("Generating new random genesis block, note that this SHOULD NOT happen unless you are setting up new network")
			defk, err := w.GenerateKey(types.KTBLS)
			if err != nil {
				return nil, err
			}

			alloc := map[address.Address]types.BigInt{
				defk: types.FromFil(1000),
			}

			for _, genm := range gmc.PreSeals {
				waddr, err := w.Import(&genm.Key)
				if err != nil {
					return nil, err
				}
				alloc[waddr] = types.FromFil(10000)
			}

			b, err := gen.MakeGenesisBlock(bs, alloc, gmc, 100000)
			if err != nil {
				return nil, err
			}
			offl := offline.Exchange(bs)
			blkserv := blockservice.New(bs, offl)
			dserv := merkledag.NewDAGService(blkserv)

			if err := car.WriteCar(context.TODO(), dserv, []cid.Cid{b.Genesis.Cid()}, out); err != nil {
				return nil, err
			}

			return b.Genesis, nil
		}
	}
}

func MakeGenesis(outFile, presealInfo, timestamp string) func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
	return func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
		return func() (*types.BlockHeader, error) {
			glog.Warn("Generating new random genesis block, note that this SHOULD NOT happen unless you are setting up new network")
			presealInfo, err := homedir.Expand(presealInfo)
			if err != nil {
				return nil, err
			}

			fdata, err := ioutil.ReadFile(presealInfo)
			if err != nil {
				return nil, xerrors.Errorf("reading preseals json: %w", err)
			}

			var preseals map[string]genesis.GenesisMiner
			if err := json.Unmarshal(fdata, &preseals); err != nil {
				return nil, err
			}

			var fakePeerIDs []peer.ID
			minerAddresses := make([]address.Address, 0, len(preseals))
			for s := range preseals {
				a, err := address.NewFromString(s)
				if err != nil {
					return nil, err
				}
				if a.Protocol() != address.ID {
					return nil, xerrors.New("expected ID address")
				}
				minerAddresses = append(minerAddresses, a)
				fakePeerIDs = append(fakePeerIDs, peer.ID("peer"+a.String()))
			}

			gmc := &gen.GenMinerCfg{
				PeerIDs:    fakePeerIDs,
				PreSeals:   preseals,
				MinerAddrs: minerAddresses,
			}

			addrs := map[address.Address]types.BigInt{}

			for _, miner := range preseals {
				if _, err := w.Import(&miner.Key); err != nil {
					return nil, xerrors.Errorf("importing miner key: %w", err)
				}

				_ = w.SetDefault(miner.Worker)

				addrs[miner.Worker] = types.FromFil(100000)
			}

			ts := uint64(time.Now().Unix())
			if timestamp != "" {
				t, err := time.Parse(time.RFC3339, timestamp)
				if err != nil {
					return nil, xerrors.Errorf("parsing input genesis timestamp: %w", err)
				}

				glog.Infof("will use %s as the genesis timestamp", t)
				ts = uint64(t.Unix())
			}

			b, err := gen.MakeGenesisBlock(bs, addrs, gmc, ts)
			if err != nil {
				return nil, err
			}

			fmt.Println("GENESIS MINER ADDRESS: ", gmc.MinerAddrs[0].String())

			f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return nil, err
			}

			offl := offline.Exchange(bs)
			blkserv := blockservice.New(bs, offl)
			dserv := merkledag.NewDAGService(blkserv)

			if err := car.WriteCar(context.TODO(), dserv, []cid.Cid{b.Genesis.Cid()}, f); err != nil {
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

package testing

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-merkledag"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/node/modules"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
)

var glog = logging.Logger("genesis")

func MakeGenesisMem(out io.Writer) func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
	return func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
		return func() (*types.BlockHeader, error) {
			glog.Warn("Generating new random genesis block, note that this SHOULD NOT happen unless you are setting up new network")
			// TODO: make an address allocation
			w, err := w.GenerateKey(types.KTBLS)
			if err != nil {
				return nil, err
			}

			gmc := &gen.GenMinerCfg{
				Owners:  []address.Address{w},
				Workers: []address.Address{w},
				PeerIDs: []peer.ID{"peerID 1"},
			}
			alloc := map[address.Address]types.BigInt{
				w: types.NewInt(1000000000),
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

func MakeGenesis(outFile string) func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
	return func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
		return func() (*types.BlockHeader, error) {
			glog.Warn("Generating new random genesis block, note that this SHOULD NOT happen unless you are setting up new network")
			minerAddr, err := w.GenerateKey(types.KTBLS)
			if err != nil {
				return nil, err
			}

			gmc := &gen.GenMinerCfg{
				Owners:  []address.Address{minerAddr},
				Workers: []address.Address{minerAddr},
				PeerIDs: []peer.ID{"peer ID 1"},
			}

			addrs := map[address.Address]types.BigInt{
				minerAddr: types.NewInt(5000000000000000000),
			}

			b, err := gen.MakeGenesisBlock(bs, addrs, gmc, 100000)
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

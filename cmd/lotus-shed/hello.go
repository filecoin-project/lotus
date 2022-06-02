package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/addrutil"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var helloCmd = &cli.Command{
	Name:        "hello",
	Description: "Get remote peer hello message by multiaddr",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "multiaddr",
			Usage:    "remote peer multiaddr",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "genesis-path",
			Usage: "genesis car file path",
		},
	},

	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		cf := cctx.String("genesis-path")
		f, err := os.OpenFile(cf, os.O_RDONLY, 0664)
		if err != nil {
			return xerrors.Errorf("opening the car file: %w", err)
		}
		bs := blockstore.FromDatastore(datastore.NewMapDatastore())
		cs := store.NewChainStore(bs, bs, datastore.NewMapDatastore(), filcns.Weight, nil)
		ts, err := cs.Import(cctx.Context, f)
		if err != nil {
			return err
		}
		pis, err := addrInfoFromArg(ctx, cctx)
		if err != nil {
			return err
		}
		priv, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
		if err != nil {
			return err
		}
		h, err := libp2p.New(libp2p.Identity(priv))
		if err != nil {
			return err
		}
		err = h.Connect(ctx, pis[0])
		if err != nil {
			return err
		}
		h.SetStreamHandler(hello.ProtocolID, HandleStream)
		weight, err := cs.Weight(ctx, ts)
		if err != nil {
			return err
		}
		gen := ts.Blocks()[0]
		hmsg := &hello.HelloMessage{
			HeaviestTipSet:       ts.Cids(),
			HeaviestTipSetHeight: ts.Height(),
			HeaviestTipSetWeight: weight,
			GenesisHash:          gen.Cid(),
		}
		err = SayHello(ctx, pis[0].ID, h, hmsg)
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 5)
		return nil
	},
}

func addrInfoFromArg(ctx context.Context, cctx *cli.Context) ([]peer.AddrInfo, error) {
	pis, err := addrutil.ParseAddresses(ctx, []string{cctx.String("multiaddr")})
	if err != nil {
		a, perr := address.NewFromString(cctx.String("multiaddr"))
		if perr != nil {
			return nil, err
		}

		na, fc, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return nil, err
		}
		defer fc()

		mi, err := na.StateMinerInfo(ctx, a, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("getting miner info: %w", err)
		}

		if mi.PeerId == nil {
			return nil, xerrors.Errorf("no PeerID for miner")
		}
		multiaddrs := make([]multiaddr.Multiaddr, 0, len(mi.Multiaddrs))
		for i, a := range mi.Multiaddrs {
			maddr, err := multiaddr.NewMultiaddrBytes(a)
			if err != nil {
				log.Warnf("parsing multiaddr %d (%x): %s", i, a, err)
				continue
			}
			multiaddrs = append(multiaddrs, maddr)
		}

		pi := peer.AddrInfo{
			ID:    *mi.PeerId,
			Addrs: multiaddrs,
		}

		fmt.Printf("%s -> %s\n", a, pi)

		pis = append(pis, pi)
	}

	return pis, err
}

func HandleStream(s inet.Stream) {
	var hmsg hello.HelloMessage
	if err := cborutil.ReadCborRPC(s, &hmsg); err != nil {
		log.Infow("failed to read hello message, disconnecting", "error", err)
		_ = s.Conn().Close()
		return
	}
	data, err := json.Marshal(hmsg)
	if err != nil {
		return
	}
	fmt.Println(string(data))
}

func SayHello(ctx context.Context, pid peer.ID, h host.Host, hmsg *hello.HelloMessage) error {
	s, err := h.NewStream(ctx, pid, hello.ProtocolID)
	if err != nil {
		return xerrors.Errorf("error opening stream: %w", err)
	}
	if err := cborutil.WriteCborRPC(s, hmsg); err != nil {
		return xerrors.Errorf("writing rpc to peer: %w", err)
	}

	return nil
}

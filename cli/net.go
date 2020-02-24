package cli

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"sort"
	"strings"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/lib/addrutil"
)

var netCmd = &cli.Command{
	Name:  "net",
	Usage: "Manage P2P Network",
	Subcommands: []*cli.Command{
		netPeers,
		netConnect,
		netListen,
		netId,
		netFindPeer,
	},
}

var netPeers = &cli.Command{
	Name:  "peers",
	Usage: "Print peers",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		peers, err := api.NetPeers(ctx)
		if err != nil {
			return err
		}

		sort.Slice(peers, func(i, j int) bool {
			return strings.Compare(string(peers[i].ID), string(peers[j].ID)) > 0
		})

		for _, peer := range peers {
			fmt.Println(peer)
		}

		return nil
	},
}

var netListen = &cli.Command{
	Name:  "listen",
	Usage: "List listen addresses",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		addrs, err := api.NetAddrsListen(ctx)
		if err != nil {
			return err
		}

		for _, peer := range addrs.Addrs {
			fmt.Printf("%s/p2p/%s\n", peer, addrs.ID)
		}
		return nil
	},
}

var netConnect = &cli.Command{
	Name:      "connect",
	Usage:     "Connect to a peer",
	ArgsUsage: "<peer multiaddr>",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		pis, err := addrutil.ParseAddresses(ctx, cctx.Args().Slice())
		if err != nil {
			return err
		}

		for _, pi := range pis {
			fmt.Printf("connect %s: ", pi.ID.Pretty())
			err := api.NetConnect(ctx, pi)
			if err != nil {
				fmt.Println("failure")
				return err
			}
			fmt.Println("success")
		}

		return nil
	},
}

var netId = &cli.Command{
	Name:  "id",
	Usage: "Get node identity",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		pid, err := api.ID(ctx)
		if err != nil {
			return err
		}

		fmt.Println(pid)
		return nil
	},
}

var netFindPeer = &cli.Command{
	Name:      "findpeer",
	Usage:     "Find the addresses of a given peerID",
	ArgsUsage: "<peer ID>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			fmt.Println("Usage: findpeer [peer ID]")
			return nil
		}

		pid, err := peer.IDB58Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		addrs, err := api.NetFindPeer(ctx, pid)

		if err != nil {
			return err
		}

		fmt.Println(addrs)
		return nil
	},
}

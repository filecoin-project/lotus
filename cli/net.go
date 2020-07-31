package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/urfave/cli/v2"

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
		netScores,
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
			fmt.Printf("%s, %s\n", peer.ID, peer.Addrs)
		}

		return nil
	},
}

var netScores = &cli.Command{
	Name:  "scores",
	Usage: "Print peers' pubsub scores",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "extended",
			Usage: "print extended peer scores in json",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		scores, err := api.NetPubsubScores(ctx)
		if err != nil {
			return err
		}

		if cctx.Bool("extended") {
			enc := json.NewEncoder(os.Stdout)
			for _, peer := range scores {
				err := enc.Encode(peer)
				if err != nil {
					return err
				}
			}
		} else {
			for _, peer := range scores {
				fmt.Printf("%s, %f\n", peer.ID, peer.Score.Score)
			}
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
	ArgsUsage: "[peerMultiaddr]",
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
	ArgsUsage: "[peerId]",
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

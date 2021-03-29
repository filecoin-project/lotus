package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"

	atypes "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/addrutil"
)

var NetCmd = &cli.Command{
	Name:  "net",
	Usage: "Manage P2P Network",
	Subcommands: []*cli.Command{
		NetPeers,
		NetConnect,
		NetListen,
		NetId,
		NetFindPeer,
		NetScores,
		NetReachability,
		NetBandwidthCmd,
		NetBlockCmd,
	},
}

var NetPeers = &cli.Command{
	Name:  "peers",
	Usage: "Print peers",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "agent",
			Aliases: []string{"a"},
			Usage:   "Print agent name",
		},
		&cli.BoolFlag{
			Name:    "extended",
			Aliases: []string{"x"},
			Usage:   "Print extended peer information in json",
		},
	},
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

		if cctx.Bool("extended") {
			// deduplicate
			seen := make(map[peer.ID]struct{})

			for _, peer := range peers {
				_, dup := seen[peer.ID]
				if dup {
					continue
				}
				seen[peer.ID] = struct{}{}

				info, err := api.NetPeerInfo(ctx, peer.ID)
				if err != nil {
					log.Warnf("error getting extended peer info: %s", err)
				} else {
					bytes, err := json.Marshal(&info)
					if err != nil {
						log.Warnf("error marshalling extended peer info: %s", err)
					} else {
						fmt.Println(string(bytes))
					}
				}
			}
		} else {
			for _, peer := range peers {
				var agent string
				if cctx.Bool("agent") {
					agent, err = api.NetAgentVersion(ctx, peer.ID)
					if err != nil {
						log.Warnf("getting agent version: %s", err)
					} else {
						agent = ", " + agent
					}
				}
				fmt.Printf("%s, %s%s\n", peer.ID, peer.Addrs, agent)
			}
		}

		return nil
	},
}

var NetScores = &cli.Command{
	Name:  "scores",
	Usage: "Print peers' pubsub scores",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "extended",
			Aliases: []string{"x"},
			Usage:   "print extended peer scores in json",
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

var NetListen = &cli.Command{
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

var NetConnect = &cli.Command{
	Name:      "connect",
	Usage:     "Connect to a peer",
	ArgsUsage: "[peerMultiaddr|minerActorAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		pis, err := addrutil.ParseAddresses(ctx, cctx.Args().Slice())
		if err != nil {
			a, perr := address.NewFromString(cctx.Args().First())
			if perr != nil {
				return err
			}

			na, fc, err := GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer fc()

			mi, err := na.StateMinerInfo(ctx, a, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting miner info: %w", err)
			}

			if mi.PeerId == nil {
				return xerrors.Errorf("no PeerID for miner")
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

var NetId = &cli.Command{
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

var NetFindPeer = &cli.Command{
	Name:      "findpeer",
	Usage:     "Find the addresses of a given peerID",
	ArgsUsage: "[peerId]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			fmt.Println("Usage: findpeer [peer ID]")
			return nil
		}

		pid, err := peer.Decode(cctx.Args().First())
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

var NetReachability = &cli.Command{
	Name:  "reachability",
	Usage: "Print information about reachability from the internet",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		i, err := api.NetAutoNatStatus(ctx)
		if err != nil {
			return err
		}

		fmt.Println("AutoNAT status: ", i.Reachability.String())
		if i.PublicAddr != "" {
			fmt.Println("Public address: ", i.PublicAddr)
		}
		return nil
	},
}

var NetBandwidthCmd = &cli.Command{
	Name:  "bandwidth",
	Usage: "Print bandwidth usage information",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "by-peer",
			Usage: "list bandwidth usage by peer",
		},
		&cli.BoolFlag{
			Name:  "by-protocol",
			Usage: "list bandwidth usage by protocol",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		bypeer := cctx.Bool("by-peer")
		byproto := cctx.Bool("by-protocol")

		tw := tabwriter.NewWriter(os.Stdout, 4, 4, 2, ' ', 0)

		fmt.Fprintf(tw, "Segment\tTotalIn\tTotalOut\tRateIn\tRateOut\n")

		if bypeer {
			bw, err := api.NetBandwidthStatsByPeer(ctx)
			if err != nil {
				return err
			}

			var peers []string
			for p := range bw {
				peers = append(peers, p)
			}

			sort.Slice(peers, func(i, j int) bool {
				return peers[i] < peers[j]
			})

			for _, p := range peers {
				s := bw[p]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s/s\t%s/s\n", p, humanize.Bytes(uint64(s.TotalIn)), humanize.Bytes(uint64(s.TotalOut)), humanize.Bytes(uint64(s.RateIn)), humanize.Bytes(uint64(s.RateOut)))
			}
		} else if byproto {
			bw, err := api.NetBandwidthStatsByProtocol(ctx)
			if err != nil {
				return err
			}

			var protos []protocol.ID
			for p := range bw {
				protos = append(protos, p)
			}

			sort.Slice(protos, func(i, j int) bool {
				return protos[i] < protos[j]
			})

			for _, p := range protos {
				s := bw[p]
				if p == "" {
					p = "<unknown>"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s/s\t%s/s\n", p, humanize.Bytes(uint64(s.TotalIn)), humanize.Bytes(uint64(s.TotalOut)), humanize.Bytes(uint64(s.RateIn)), humanize.Bytes(uint64(s.RateOut)))
			}
		} else {

			s, err := api.NetBandwidthStats(ctx)
			if err != nil {
				return err
			}

			fmt.Fprintf(tw, "Total\t%s\t%s\t%s/s\t%s/s\n", humanize.Bytes(uint64(s.TotalIn)), humanize.Bytes(uint64(s.TotalOut)), humanize.Bytes(uint64(s.RateIn)), humanize.Bytes(uint64(s.RateOut)))
		}

		return tw.Flush()

	},
}

var NetBlockCmd = &cli.Command{
	Name:  "block",
	Usage: "Manage network connection gating rules",
	Subcommands: []*cli.Command{
		NetBlockAddCmd,
		NetBlockRemoveCmd,
		NetBlockListCmd,
	},
}

var NetBlockAddCmd = &cli.Command{
	Name:  "add",
	Usage: "Add connection gating rules",
	Subcommands: []*cli.Command{
		NetBlockAddPeer,
		NetBlockAddIP,
		NetBlockAddSubnet,
	},
}

var NetBlockAddPeer = &cli.Command{
	Name:      "peer",
	Usage:     "Block a peer",
	ArgsUsage: "<Peer> ...",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var peers []peer.ID
		for _, s := range cctx.Args().Slice() {
			p, err := peer.Decode(s)
			if err != nil {
				return err
			}

			peers = append(peers, p)
		}

		return api.NetBlockAdd(ctx, atypes.NetBlockList{Peers: peers})
	},
}

var NetBlockAddIP = &cli.Command{
	Name:      "ip",
	Usage:     "Block an IP address",
	ArgsUsage: "<IP> ...",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		return api.NetBlockAdd(ctx, atypes.NetBlockList{IPAddrs: cctx.Args().Slice()})
	},
}

var NetBlockAddSubnet = &cli.Command{
	Name:      "subnet",
	Usage:     "Block an IP subnet",
	ArgsUsage: "<CIDR> ...",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		return api.NetBlockAdd(ctx, atypes.NetBlockList{IPSubnets: cctx.Args().Slice()})
	},
}

var NetBlockRemoveCmd = &cli.Command{
	Name:  "remove",
	Usage: "Remove connection gating rules",
	Subcommands: []*cli.Command{
		NetBlockRemovePeer,
		NetBlockRemoveIP,
		NetBlockRemoveSubnet,
	},
}

var NetBlockRemovePeer = &cli.Command{
	Name:      "peer",
	Usage:     "Unblock a peer",
	ArgsUsage: "<Peer> ...",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var peers []peer.ID
		for _, s := range cctx.Args().Slice() {
			p, err := peer.Decode(s)
			if err != nil {
				return err
			}

			peers = append(peers, p)
		}

		return api.NetBlockRemove(ctx, atypes.NetBlockList{Peers: peers})
	},
}

var NetBlockRemoveIP = &cli.Command{
	Name:      "ip",
	Usage:     "Unblock an IP address",
	ArgsUsage: "<IP> ...",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		return api.NetBlockRemove(ctx, atypes.NetBlockList{IPAddrs: cctx.Args().Slice()})
	},
}

var NetBlockRemoveSubnet = &cli.Command{
	Name:      "subnet",
	Usage:     "Unblock an IP subnet",
	ArgsUsage: "<CIDR> ...",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		return api.NetBlockRemove(ctx, atypes.NetBlockList{IPSubnets: cctx.Args().Slice()})
	},
}

var NetBlockListCmd = &cli.Command{
	Name:  "list",
	Usage: "list connection gating rules",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		acl, err := api.NetBlockList(ctx)
		if err != nil {
			return err
		}

		if len(acl.Peers) != 0 {
			sort.Slice(acl.Peers, func(i, j int) bool {
				return strings.Compare(string(acl.Peers[i]), string(acl.Peers[j])) > 0
			})

			fmt.Println("Blocked Peers:")
			for _, p := range acl.Peers {
				fmt.Printf("\t%s\n", p)
			}
		}

		if len(acl.IPAddrs) != 0 {
			sort.Slice(acl.IPAddrs, func(i, j int) bool {
				return strings.Compare(acl.IPAddrs[i], acl.IPAddrs[j]) < 0
			})

			fmt.Println("Blocked IPs:")
			for _, a := range acl.IPAddrs {
				fmt.Printf("\t%s\n", a)
			}
		}

		if len(acl.IPSubnets) != 0 {
			sort.Slice(acl.IPSubnets, func(i, j int) bool {
				return strings.Compare(acl.IPSubnets[i], acl.IPSubnets[j]) < 0
			})

			fmt.Println("Blocked Subnets:")
			for _, n := range acl.IPSubnets {
				fmt.Printf("\t%s\n", n)
			}
		}

		return nil
	},
}

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

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
		NetPing,
		NetConnect,
		NetDisconnect,
		NetListen,
		NetId,
		NetFindPeer,
		NetScores,
		NetReachability,
		NetBandwidthCmd,
		NetBlockCmd,
		NetStatCmd,
		NetLimitCmd,
		NetProtectAdd,
		NetProtectRemove,
		NetProtectList,
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
		api, closer, err := GetFullNodeAPI(cctx)
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

var NetPing = &cli.Command{
	Name:      "ping",
	Usage:     "Ping peers",
	ArgsUsage: "[peerMultiaddr]",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "count",
			Value:   10,
			Aliases: []string{"c"},
			Usage:   "specify the number of times it should ping",
		},
		&cli.DurationFlag{
			Name:    "interval",
			Value:   time.Second,
			Aliases: []string{"i"},
			Usage:   "minimum time between pings",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		pis, err := AddrInfoFromArg(ctx, cctx)
		if err != nil {
			return err
		}

		count := cctx.Int("count")
		interval := cctx.Duration("interval")

		for _, pi := range pis {
			err := api.NetConnect(ctx, pi)
			if err != nil {
				return xerrors.Errorf("connect: %w", err)
			}

			fmt.Printf("PING %s\n", pi.ID)
			var avg time.Duration
			var successful int

			for i := 0; i < count && ctx.Err() == nil; i++ {
				start := time.Now()

				rtt, err := api.NetPing(ctx, pi.ID)
				if err != nil {
					if ctx.Err() != nil {
						break
					}
					log.Errorf("Ping failed: error=%v", err)
					continue
				}
				fmt.Printf("Pong received: time=%v\n", rtt)
				avg = avg + rtt
				successful++

				wctx, cancel := context.WithTimeout(ctx, time.Until(start.Add(interval)))
				<-wctx.Done()
				cancel()
			}

			if successful > 0 {
				fmt.Printf("Average latency: %v\n", avg/time.Duration(successful))
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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

var NetDisconnect = &cli.Command{
	Name:      "disconnect",
	Usage:     "Disconnect from a peer",
	ArgsUsage: "[peerID]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		ids := cctx.Args().Slice()
		for _, id := range ids {
			pid, err := peer.Decode(id)
			if err != nil {
				fmt.Println("failure")
				return err
			}
			fmt.Printf("disconnect %s: ", pid)
			err = api.NetDisconnect(ctx, pid)
			if err != nil {
				fmt.Println("failure")
				return err
			}
			fmt.Println("success")
		}
		return nil
	},
}

var NetConnect = &cli.Command{
	Name:      "connect",
	Usage:     "Connect to a peer",
	ArgsUsage: "[peerMultiaddr|minerActorAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		pis, err := AddrInfoFromArg(ctx, cctx)
		if err != nil {
			return err
		}

		for _, pi := range pis {
			fmt.Printf("connect %s: ", pi.ID)
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

func AddrInfoFromArg(ctx context.Context, cctx *cli.Context) ([]peer.AddrInfo, error) {
	pis, err := addrutil.ParseAddresses(ctx, cctx.Args().Slice())
	if err != nil {
		a, perr := address.NewFromString(cctx.Args().First())
		if perr != nil {
			return nil, err
		}

		na, fc, err := GetFullNodeAPI(cctx)
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

	return pis, nil
}

var NetId = &cli.Command{
	Name:  "id",
	Usage: "Get node identity",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
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
	Name:      "find-peer",
	Aliases:   []string{"findpeer"},
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

		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		if len(i.PublicAddrs) > 0 {
			fmt.Println("Public address:", i.PublicAddrs)
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
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		bypeer := cctx.Bool("by-peer")
		byproto := cctx.Bool("by-protocol")

		tw := tabwriter.NewWriter(os.Stdout, 4, 4, 2, ' ', 0)

		_, _ = fmt.Fprintf(tw, "Segment\tTotalIn\tTotalOut\tRateIn\tRateOut\n")

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
				_, _ = fmt.Fprintf(
					tw,
					"%s\t%s\t%s\t%s/s\t%s/s\n",
					p,
					humanize.Bytes(uint64(s.TotalIn)),
					humanize.Bytes(uint64(s.TotalOut)),
					humanize.Bytes(uint64(s.RateIn)),
					humanize.Bytes(uint64(s.RateOut)),
				)
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
				_, _ = fmt.Fprintf(
					tw,
					"%s\t%s\t%s\t%s/s\t%s/s\n",
					p,
					humanize.Bytes(uint64(s.TotalIn)),
					humanize.Bytes(uint64(s.TotalOut)),
					humanize.Bytes(uint64(s.RateIn)),
					humanize.Bytes(uint64(s.RateOut)),
				)
			}
		} else {

			s, err := api.NetBandwidthStats(ctx)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				tw,
				"Total\t%s\t%s\t%s/s\t%s/s\n",
				humanize.Bytes(uint64(s.TotalIn)),
				humanize.Bytes(uint64(s.TotalOut)),
				humanize.Bytes(uint64(s.RateIn)),
				humanize.Bytes(uint64(s.RateOut)),
			)
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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
		api, closer, err := GetFullNodeAPI(cctx)
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

var BarCols = float64(64)

func BarString(total, y, g float64) string {
	yBars := int(math.Round(y / total * BarCols))
	gBars := int(math.Round(g / total * BarCols))
	if yBars < 0 {
		yBars = 0
	}
	if gBars < 0 {
		gBars = 0
	}
	eBars := int(BarCols) - yBars - gBars
	var barString = color.YellowString(strings.Repeat("|", yBars)) +
		color.GreenString(strings.Repeat("|", gBars))
	if eBars >= 0 {
		barString += strings.Repeat(" ", eBars)
	}
	return barString
}

var NetStatCmd = &cli.Command{
	Name:  "stat",
	Usage: "Report resource usage for a scope",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "json",
		},
	},
	ArgsUsage: "scope",
	Description: `Report resource usage for a scope.

  The scope can be one of the following:
  - system        -- reports the system aggregate resource usage.
  - transient     -- reports the transient resource usage.
  - svc:<service> -- reports the resource usage of a specific service.
  - proto:<proto> -- reports the resource usage of a specific protocol.
  - peer:<peer>   -- reports the resource usage of a specific peer.
  - all           -- reports the resource usage for all currently active scopes.
`,
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		args := cctx.Args().Slice()
		if len(args) != 1 {
			return xerrors.Errorf("must specify exactly one scope")
		}
		scope := args[0]

		result, err := api.NetStat(ctx, scope)
		if err != nil {
			return xerrors.Errorf("get stat: %w", err)
		}

		if cctx.Bool("json") {
			enc := json.NewEncoder(os.Stdout)
			return enc.Encode(result)
		}

		printScope := func(stat *network.ScopeStat, scope string) {
			if stat == nil {
				return
			}

			limit, err := api.NetLimit(ctx, scope)
			if err != nil {
				fmt.Printf("error: %s\n", color.RedString("%s", err))
			}

			fmt.Printf("%s\n", scope)
			fmt.Printf("\tmemory:      [%s] %s/%s\n", BarString(float64(limit.Memory), 0, float64(stat.Memory)),
				types.SizeStr(types.NewInt(uint64(stat.Memory))),
				types.SizeStr(types.NewInt(uint64(limit.Memory))))

			fmt.Printf("\tstreams in:  [%s] %d/%d\n", BarString(float64(limit.StreamsInbound), 0, float64(stat.NumStreamsInbound)), stat.NumStreamsInbound, limit.StreamsInbound)
			fmt.Printf("\tstreams out: [%s] %d/%d\n", BarString(float64(limit.StreamsOutbound), 0, float64(stat.NumStreamsOutbound)), stat.NumStreamsOutbound, limit.StreamsOutbound)
			fmt.Printf("\tconn in:     [%s] %d/%d\n", BarString(float64(limit.ConnsInbound), 0, float64(stat.NumConnsInbound)), stat.NumConnsInbound, limit.ConnsInbound)
			fmt.Printf("\tconn out:    [%s] %d/%d\n", BarString(float64(limit.ConnsOutbound), 0, float64(stat.NumConnsOutbound)), stat.NumConnsOutbound, limit.ConnsOutbound)
			fmt.Printf("\tfile desc:   [%s] %d/%d\n", BarString(float64(limit.FD), 0, float64(stat.NumFD)), stat.NumFD, limit.FD)
			fmt.Println()
		}

		printScope(result.System, "system")
		printScope(result.Transient, "transient")

		printScopes := func(name string, st map[string]network.ScopeStat) {
			type namedStat struct {
				name string
				stat network.ScopeStat
			}

			stats := make([]namedStat, 0, len(st))
			for n, stat := range st {
				stats = append(stats, namedStat{
					name: n,
					stat: stat,
				})
			}
			sort.Slice(stats, func(i, j int) bool {
				if stats[i].stat.Memory == stats[j].stat.Memory {
					return stats[i].name < stats[j].name
				}

				return stats[i].stat.Memory > stats[j].stat.Memory
			})

			for _, stat := range stats {
				tmp := stat.stat
				printScope(&tmp, name+stat.name)
			}

		}

		printScopes("svc:", result.Services)
		printScopes("proto:", result.Protocols)
		printScopes("peer:", result.Peers)

		return nil
	},
}

var NetLimitCmd = &cli.Command{
	Name:      "limit",
	Usage:     "Get or set resource limits for a scope",
	ArgsUsage: "scope [limit]",
	Description: `Get or set resource limits for a scope.

  The scope can be one of the following:
  - system        -- reports the system aggregate resource usage.
  - transient     -- reports the transient resource usage.
  - svc:<service> -- reports the resource usage of a specific service.
  - proto:<proto> -- reports the resource usage of a specific protocol.
  - peer:<peer>   -- reports the resource usage of a specific peer.

 The limit is json-formatted, with the same structure as the limits file.
`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "set",
			Usage: "set the limit for a scope",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		args := cctx.Args().Slice()

		if cctx.Bool("set") {
			if len(args) != 2 {
				return xerrors.Errorf("must specify exactly a scope and a limit")
			}
			scope := args[0]
			limitStr := args[1]

			var limit atypes.NetLimit
			err := json.Unmarshal([]byte(limitStr), &limit)
			if err != nil {
				return xerrors.Errorf("error decoding limit: %w", err)
			}

			return api.NetSetLimit(ctx, scope, limit)

		}

		if len(args) != 1 {
			return xerrors.Errorf("must specify exactly one scope")
		}
		scope := args[0]

		result, err := api.NetLimit(ctx, scope)
		if err != nil {
			return err
		}

		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(result)
	},
}

var NetProtectAdd = &cli.Command{
	Name:      "protect",
	Usage:     "Add one or more peer IDs to the list of protected peer connections",
	ArgsUsage: "<peer-id> [<peer-id>...]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		pids, err := decodePeerIDsFromArgs(cctx)
		if err != nil {
			return err
		}

		err = api.NetProtectAdd(ctx, pids)
		if err != nil {
			return err
		}

		fmt.Println("added to protected peers:")
		for _, pid := range pids {
			fmt.Printf(" %s\n", pid)
		}
		return nil
	},
}

var NetProtectRemove = &cli.Command{
	Name:      "unprotect",
	Usage:     "Remove one or more peer IDs from the list of protected peer connections.",
	ArgsUsage: "<peer-id> [<peer-id>...]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		pids, err := decodePeerIDsFromArgs(cctx)
		if err != nil {
			return err
		}

		err = api.NetProtectRemove(ctx, pids)
		if err != nil {
			return err
		}

		fmt.Printf("removed from protected peers:")
		for _, pid := range pids {
			fmt.Printf(" %s\n", pid)
		}
		return nil
	},
}

// decodePeerIDsFromArgs decodes all the arguments present in cli.Context.Args as peer.ID.
//
// This function requires at least one argument to be present, and arguments must not be empty
// string. Otherwise, an error is returned.
func decodePeerIDsFromArgs(cctx *cli.Context) ([]peer.ID, error) {
	pidArgs := cctx.Args().Slice()
	if len(pidArgs) == 0 {
		return nil, xerrors.Errorf("must specify at least one peer ID as an argument")
	}
	var pids []peer.ID
	for _, pidStr := range pidArgs {
		if pidStr == "" {
			return nil, xerrors.Errorf("peer ID must not be empty")
		}
		pid, err := peer.Decode(pidStr)
		if err != nil {
			return nil, err
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

var NetProtectList = &cli.Command{
	Name:  "list-protected",
	Usage: "List the peer IDs with protected connection.",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		pids, err := api.NetProtectList(ctx)
		if err != nil {
			return err
		}

		for _, pid := range pids {
			fmt.Printf("%s\n", pid)
		}
		return nil
	},
}

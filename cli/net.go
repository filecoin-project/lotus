package cli

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
	"gopkg.in/urfave/cli.v2"
)

var netCmd = &cli.Command{
	Name:  "net",
	Usage: "Manage P2P Network",
	Subcommands: []*cli.Command{
		netPeers,
		netConnect,
		netListen,
	},
}

var netPeers = &cli.Command{
	Name:  "peers",
	Usage: "Print peers",
	Action: func(cctx *cli.Context) error {
		api := getApi(cctx)
		ctx := reqContext(cctx)
		fmt.Println(api.NetPeers(ctx))

		return nil
	},
}

var netListen = &cli.Command{
	Name:  "listen",
	Usage: "List listen addresses",
	Action: func(cctx *cli.Context) error {
		api := getApi(cctx)
		ctx := reqContext(cctx)
		fmt.Println(api.NetAddrsListen(ctx))

		return nil
	},
}

var netConnect = &cli.Command{
	Name:  "connect",
	Usage: "Connect to a peer",
	Action: func(cctx *cli.Context) error {
		api := getApi(cctx)
		ctx := reqContext(cctx)

		pis, err := parseAddresses(ctx, cctx.Args().Slice())
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

// parseAddresses is a function that takes in a slice of string peer addresses
// (multiaddr + peerid) and returns a slice of properly constructed peers
func parseAddresses(ctx context.Context, addrs []string) ([]peer.AddrInfo, error) {
	// resolve addresses
	maddrs, err := resolveAddresses(ctx, addrs)
	if err != nil {
		return nil, err
	}

	return peer.AddrInfosFromP2pAddrs(maddrs...)
}

const (
	dnsResolveTimeout = 10 * time.Second
)

// resolveAddresses resolves addresses parallelly
func resolveAddresses(ctx context.Context, addrs []string) ([]ma.Multiaddr, error) {
	ctx, cancel := context.WithTimeout(ctx, dnsResolveTimeout)
	defer cancel()

	var maddrs []ma.Multiaddr
	var wg sync.WaitGroup
	resolveErrC := make(chan error, len(addrs))

	maddrC := make(chan ma.Multiaddr)

	for _, addr := range addrs {
		maddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}

		// check whether address ends in `ipfs/Qm...`
		if _, last := ma.SplitLast(maddr); last.Protocol().Code == ma.P_IPFS {
			maddrs = append(maddrs, maddr)
			continue
		}
		wg.Add(1)
		go func(maddr ma.Multiaddr) {
			defer wg.Done()
			raddrs, err := madns.Resolve(ctx, maddr)
			if err != nil {
				resolveErrC <- err
				return
			}
			// filter out addresses that still doesn't end in `ipfs/Qm...`
			found := 0
			for _, raddr := range raddrs {
				if _, last := ma.SplitLast(raddr); last != nil && last.Protocol().Code == ma.P_IPFS {
					maddrC <- raddr
					found++
				}
			}
			if found == 0 {
				resolveErrC <- fmt.Errorf("found no ipfs peers at %s", maddr)
			}
		}(maddr)
	}
	go func() {
		wg.Wait()
		close(maddrC)
	}()

	for maddr := range maddrC {
		maddrs = append(maddrs, maddr)
	}

	select {
	case err := <-resolveErrC:
		return nil, err
	default:
	}

	return maddrs, nil
}

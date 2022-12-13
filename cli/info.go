package cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

var infoCmd = &cli.Command{
	Name:   "info",
	Usage:  "Print node info",
	Action: infoCmdAct,
}

func infoCmdAct(cctx *cli.Context) error {
	fullapi, acloser, err := GetFullNodeAPIV1(cctx)
	if err != nil {
		return err
	}
	defer acloser()
	ctx := ReqContext(cctx)

	network, err := fullapi.StateGetNetworkParams(ctx)
	if err != nil {
		return err
	}

	start, err := fullapi.StartTime(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Network: %s\n", network.NetworkName)
	fmt.Printf("StartTime: %s (started at %s)\n", time.Now().Sub(start).Truncate(time.Second), start.Truncate(time.Second))
	fmt.Print("Chain: ")
	err = SyncBasefeeCheck(ctx, fullapi)
	if err != nil {
		return err
	}

	status, err := fullapi.NodeStatus(ctx, true)
	if err != nil {
		return err
	}

	fmt.Printf(" [epoch %s]\n", color.MagentaString(("%d"), status.SyncStatus.Epoch))
	fmt.Printf("Peers to: [publish messages %d] [publish blocks %d]\n", status.PeerStatus.PeersToPublishMsgs, status.PeerStatus.PeersToPublishBlocks)

	//Chain health calculated as percentage: amount of blocks in last finality / very healthy amount of blocks in a finality (900 epochs * 5 blocks per tipset)
	health := (100 * (900 * status.ChainStatus.BlocksPerTipsetLastFinality) / (900 * 5))
	switch {
	case health > 85:
		fmt.Printf("Chain health: %.f%% [%s]\n", health, color.GreenString("healthy"))
	case health < 85:
		fmt.Printf("Chain health: %.f%% [%s]\n", health, color.RedString("unhealthy"))
	}

	fmt.Println()

	addr, err := fullapi.WalletDefaultAddress(ctx)
	if err == nil {
		fmt.Printf("Default address: \n")
		balance, err := fullapi.WalletBalance(ctx, addr)
		if err != nil {
			return err
		}
		fmt.Printf("      %s [%s]\n", addr.String(), types.FIL(balance).Short())
	} else {
		fmt.Printf("Default address: address not set\n")
	}
	fmt.Println()

	addrs, err := fullapi.WalletList(ctx)
	if err != nil {
		return err
	}

	totalBalance := big.Zero()
	for _, addr := range addrs {
		totbal, err := fullapi.WalletBalance(ctx, addr)
		if err != nil {
			return err
		}
		totalBalance = big.Add(totalBalance, totbal)
	}

	switch {
	case len(addrs) <= 1:
		fmt.Printf("Wallet: %v address\n", len(addrs))
	case len(addrs) > 1:
		fmt.Printf("Wallet: %v addresses\n", len(addrs))
	}
	fmt.Printf("      Total balance: %s\n", types.FIL(totalBalance).Short())

	mbLockedSum := big.Zero()
	mbAvailableSum := big.Zero()
	for _, addr := range addrs {
		mbal, err := fullapi.StateMarketBalance(ctx, addr, types.EmptyTSK)
		if err != nil {
			if strings.Contains(err.Error(), "actor not found") {
				continue
			} else {
				return err
			}
		}
		mbLockedSum = big.Add(mbLockedSum, mbal.Locked)
		mbAvailableSum = big.Add(mbAvailableSum, mbal.Escrow)
	}

	fmt.Printf("      Market locked: %s\n", types.FIL(mbLockedSum).Short())
	fmt.Printf("      Market available: %s\n", types.FIL(mbAvailableSum).Short())

	fmt.Println()

	chs, err := fullapi.PaychList(ctx)
	if err != nil {
		return err
	}

	switch {
	case len(chs) <= 1:
		fmt.Printf("Payment Channels: %v channel\n", len(chs))
	case len(chs) > 1:
		fmt.Printf("Payment Channels: %v channels\n", len(chs))
	}
	fmt.Println()

	localDeals, err := fullapi.ClientListDeals(ctx)
	if err != nil {
		return err
	}

	var totalSize uint64
	byState := map[storagemarket.StorageDealStatus][]uint64{}
	for _, deal := range localDeals {
		totalSize += deal.Size
		byState[deal.State] = append(byState[deal.State], deal.Size)
	}

	fmt.Printf("Deals: %d, %s\n", len(localDeals), types.SizeStr(types.NewInt(totalSize)))

	type stateStat struct {
		state storagemarket.StorageDealStatus
		count int
		bytes uint64
	}

	stateStats := make([]stateStat, 0, len(byState))
	for state, deals := range byState {
		if state == storagemarket.StorageDealActive {
			state = math.MaxUint64 // for sort
		}

		st := stateStat{
			state: state,
			count: len(deals),
		}
		for _, b := range deals {
			st.bytes += b
		}

		stateStats = append(stateStats, st)
	}

	sort.Slice(stateStats, func(i, j int) bool {
		return int64(stateStats[i].state) < int64(stateStats[j].state)
	})

	for _, st := range stateStats {
		if st.state == math.MaxUint64 {
			st.state = storagemarket.StorageDealActive
		}
		fmt.Printf("      %s: %d deals, %s\n", storagemarket.DealStates[st.state], st.count, types.SizeStr(types.NewInt(st.bytes)))
	}

	fmt.Println()

	tw := tabwriter.NewWriter(os.Stdout, 6, 6, 2, ' ', 0)

	s, err := fullapi.NetBandwidthStats(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Bandwidth:\n")
	fmt.Fprintf(tw, "\tTotalIn\tTotalOut\tRateIn\tRateOut\n")
	fmt.Fprintf(tw, "\t%s\t%s\t%s/s\t%s/s\n", humanize.Bytes(uint64(s.TotalIn)), humanize.Bytes(uint64(s.TotalOut)), humanize.Bytes(uint64(s.RateIn)), humanize.Bytes(uint64(s.RateOut)))
	return tw.Flush()

}

func SyncBasefeeCheck(ctx context.Context, fullapi v1api.FullNode) error {
	head, err := fullapi.ChainHead(ctx)
	if err != nil {
		return err
	}

	switch {
	case time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs*3/2): // within 1.5 epochs
		fmt.Printf("[%s]", color.GreenString("sync ok"))
	case time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs*5): // within 5 epochs
		fmt.Printf("[%s]", color.YellowString("sync slow (%s behind)", time.Now().Sub(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second)))
	default:
		fmt.Printf("[%s]", color.RedString("sync behind! (%s behind)", time.Now().Sub(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second)))
	}
	basefee := head.MinTicketBlock().ParentBaseFee
	gasCol := []color.Attribute{color.FgBlue}
	switch {
	case basefee.GreaterThan(big.NewInt(7000_000_000)): // 7 nFIL
		gasCol = []color.Attribute{color.BgRed, color.FgBlack}
	case basefee.GreaterThan(big.NewInt(3000_000_000)): // 3 nFIL
		gasCol = []color.Attribute{color.FgRed}
	case basefee.GreaterThan(big.NewInt(750_000_000)): // 750 uFIL
		gasCol = []color.Attribute{color.FgYellow}
	case basefee.GreaterThan(big.NewInt(100_000_000)): // 100 uFIL
		gasCol = []color.Attribute{color.FgGreen}
	}
	fmt.Printf(" [basefee %s]", color.New(gasCol...).Sprint(types.FIL(basefee).Short()))
	return nil
}

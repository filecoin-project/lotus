package main

import (
	"encoding/json"
	corebig "math/big"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	filbig "github.com/filecoin-project/go-state-types/big"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

// How many epochs back to look at for dealstats
var defaultEpochLookback = abi.ChainEpoch(10)

type networkTotalsOutput struct {
	Epoch    int64         `json:"epoch"`
	Endpoint string        `json:"endpoint"`
	Payload  networkTotals `json:"payload"`
}

type networkTotals struct {
	QaNetworkPower       filbig.Int `json:"total_qa_power"`
	RawNetworkPower      filbig.Int `json:"total_raw_capacity"`
	CapacityCarryingData float64    `json:"capacity_fraction_carrying_data"`
	UniqueCids           int        `json:"total_unique_cids"`
	UniqueProviders      int        `json:"total_unique_providers"`
	UniqueClients        int        `json:"total_unique_clients"`
	TotalDeals           int        `json:"total_num_deals"`
	TotalBytes           int64      `json:"total_stored_data_size"`
	FilplusTotalDeals    int        `json:"filplus_total_num_deals"`
	FilplusTotalBytes    int64      `json:"filplus_total_stored_data_size"`

	seenClient   map[address.Address]bool
	seenProvider map[address.Address]bool
	seenPieceCid map[cid.Cid]bool
}

var storageStatsCmd = &cli.Command{
	Name:  "storage-stats",
	Usage: "Translates current lotus state into a json summary suitable for driving https://storage.filecoin.io/",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name: "height",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		api, apiCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer apiCloser()

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		requestedHeight := cctx.Int64("height")
		if requestedHeight > 0 {
			head, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(requestedHeight), head.Key())
		} else {
			head, err = api.ChainGetTipSetByHeight(ctx, head.Height()-defaultEpochLookback, head.Key())
		}
		if err != nil {
			return err
		}

		power, err := api.StateMinerPower(ctx, address.Address{}, head.Key())
		if err != nil {
			return err
		}

		netTotals := networkTotals{
			QaNetworkPower:  power.TotalPower.QualityAdjPower,
			RawNetworkPower: power.TotalPower.RawBytePower,
			seenClient:      make(map[address.Address]bool),
			seenProvider:    make(map[address.Address]bool),
			seenPieceCid:    make(map[cid.Cid]bool),
		}

		deals, err := api.StateMarketDeals(ctx, head.Key())
		if err != nil {
			return err
		}

		for _, dealInfo := range deals {

			// Only count deals that have properly started, not past/future ones
			// https://github.com/filecoin-project/specs-actors/blob/v0.9.9/actors/builtin/market/deal.go#L81-L85
			// Bail on 0 as well in case SectorStartEpoch is uninitialized due to some bug
			if dealInfo.State.SectorStartEpoch <= 0 ||
				dealInfo.State.SectorStartEpoch > head.Height() {
				continue
			}

			netTotals.seenClient[dealInfo.Proposal.Client] = true
			netTotals.TotalBytes += int64(dealInfo.Proposal.PieceSize)
			netTotals.seenProvider[dealInfo.Proposal.Provider] = true
			netTotals.seenPieceCid[dealInfo.Proposal.PieceCID] = true
			netTotals.TotalDeals++

			if dealInfo.Proposal.VerifiedDeal {
				netTotals.FilplusTotalDeals++
				netTotals.FilplusTotalBytes += int64(dealInfo.Proposal.PieceSize)
			}
		}

		netTotals.UniqueCids = len(netTotals.seenPieceCid)
		netTotals.UniqueClients = len(netTotals.seenClient)
		netTotals.UniqueProviders = len(netTotals.seenProvider)

		netTotals.CapacityCarryingData, _ = new(corebig.Rat).SetFrac(
			corebig.NewInt(netTotals.TotalBytes),
			netTotals.RawNetworkPower.Int,
		).Float64()

		return json.NewEncoder(os.Stdout).Encode(
			networkTotalsOutput{
				Epoch:    int64(head.Height()),
				Endpoint: "NETWORK_WIDE_TOTALS",
				Payload:  netTotals,
			},
		)
	},
}

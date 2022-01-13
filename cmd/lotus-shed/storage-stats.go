package main

import (
	"encoding/json"
	corebig "math/big"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	filbig "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/types"
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

type providerMeta struct {
	nonidentifiable bool
}

type Totals struct {
	TotalDeals           int     `json:"total_num_deals"`
	TotalBytes           int64   `json:"total_stored_data_size"`
	SlashedTotalDeals    int     `json:"slashed_total_num_deals"`
	SlashedTotalBytes    int64   `json:"slashed_total_stored_data_size"`
	PrivateTotalDeals    int     `json:"private_total_num_deals"`
	PrivateTotalBytes    int64   `json:"private_total_stored_data_size"`
	CapacityCarryingData float64 `json:"capacity_fraction_carrying_data"`
}

type networkTotals struct {
	QaNetworkPower         filbig.Int `json:"total_qa_power"`
	RawNetworkPower        filbig.Int `json:"total_raw_capacity"`
	UniqueCids             int        `json:"total_unique_cids"`
	UniqueBytes            int64      `json:"total_unique_data_size"`
	UniqueClients          int        `json:"total_unique_clients"`
	UniqueProviders        int        `json:"total_unique_providers"`
	UniquePrivateProviders int        `json:"total_unique_private_providers"`
	Totals
	FilPlus Totals `json:"filecoin_plus_subset"`

	pieces    map[cid.Cid]struct{}
	clients   map[address.Address]struct{}
	providers map[address.Address]providerMeta
}

var storageStatsCmd = &cli.Command{
	Name:  "storage-stats",
	Usage: "Translates current lotus state into a json summary suitable for driving https://storage.filecoin.io/",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "Comma separated array of cids, or @height",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		api, apiCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer apiCloser()

		var ts *types.TipSet
		if cctx.String("tipset") == "" {
			ts, err = api.ChainHead(ctx)
			if err != nil {
				return err
			}
			ts, err = api.ChainGetTipSetByHeight(ctx, ts.Height()-defaultEpochLookback, ts.Key())
			if err != nil {
				return err
			}
		} else {
			ts, err = lcli.ParseTipSetRef(ctx, api, cctx.String("tipset"))
			if err != nil {
				return err
			}
		}

		power, err := api.StateMinerPower(ctx, address.Address{}, ts.Key())
		if err != nil {
			return err
		}

		netTotals := networkTotals{
			QaNetworkPower:  power.TotalPower.QualityAdjPower,
			RawNetworkPower: power.TotalPower.RawBytePower,
			pieces:          make(map[cid.Cid]struct{}),
			clients:         make(map[address.Address]struct{}),
			providers:       make(map[address.Address]providerMeta),
		}

		deals, err := api.StateMarketDeals(ctx, ts.Key())
		if err != nil {
			return err
		}

		for _, dealInfo := range deals {

			// Only count deals that have properly started, not past/future ones
			// https://github.com/filecoin-project/specs-actors/blob/v0.9.9/actors/builtin/market/deal.go#L81-L85
			// Bail on 0 as well in case SectorStartEpoch is uninitialized due to some bug
			if dealInfo.State.SectorStartEpoch <= 0 ||
				dealInfo.State.SectorStartEpoch > ts.Height() {
				continue
			}

			netTotals.clients[dealInfo.Proposal.Client] = struct{}{}

			if _, seen := netTotals.providers[dealInfo.Proposal.Provider]; !seen {
				pm := providerMeta{}

				mi, err := api.StateMinerInfo(ctx, dealInfo.Proposal.Provider, ts.Key())
				if err != nil {
					return err
				}

				if mi.PeerId == nil || *mi.PeerId == "" {
					log.Infof("private provider %s", dealInfo.Proposal.Provider)
					pm.nonidentifiable = true
					netTotals.UniquePrivateProviders++
				}

				netTotals.providers[dealInfo.Proposal.Provider] = pm
				netTotals.UniqueProviders++
			}

			if _, seen := netTotals.pieces[dealInfo.Proposal.PieceCID]; !seen {
				netTotals.pieces[dealInfo.Proposal.PieceCID] = struct{}{}
				netTotals.UniqueBytes += int64(dealInfo.Proposal.PieceSize)
				netTotals.UniqueCids++
			}

			netTotals.TotalBytes += int64(dealInfo.Proposal.PieceSize)
			netTotals.TotalDeals++
			if dealInfo.State.SlashEpoch > -1 && dealInfo.State.LastUpdatedEpoch < dealInfo.State.SlashEpoch {
				netTotals.SlashedTotalBytes += int64(dealInfo.Proposal.PieceSize)
				netTotals.SlashedTotalDeals++
			}
			if netTotals.providers[dealInfo.Proposal.Provider].nonidentifiable {
				netTotals.PrivateTotalBytes += int64(dealInfo.Proposal.PieceSize)
				netTotals.PrivateTotalDeals++
			}

			if dealInfo.Proposal.VerifiedDeal {
				netTotals.FilPlus.TotalBytes += int64(dealInfo.Proposal.PieceSize)
				netTotals.FilPlus.TotalDeals++
				if dealInfo.State.SlashEpoch > -1 && dealInfo.State.LastUpdatedEpoch < dealInfo.State.SlashEpoch {
					netTotals.FilPlus.SlashedTotalBytes += int64(dealInfo.Proposal.PieceSize)
					netTotals.FilPlus.SlashedTotalDeals++
				}
				if netTotals.providers[dealInfo.Proposal.Provider].nonidentifiable {
					netTotals.FilPlus.PrivateTotalBytes += int64(dealInfo.Proposal.PieceSize)
					netTotals.FilPlus.PrivateTotalDeals++
				}
			}
		}

		netTotals.UniqueClients = len(netTotals.clients)
		netTotals.CapacityCarryingData, _ = new(corebig.Rat).SetFrac(
			corebig.NewInt(netTotals.TotalBytes),
			netTotals.RawNetworkPower.Int,
		).Float64()
		netTotals.FilPlus.CapacityCarryingData, _ = new(corebig.Rat).SetFrac(
			corebig.NewInt(netTotals.FilPlus.TotalBytes),
			netTotals.RawNetworkPower.Int,
		).Float64()

		return json.NewEncoder(os.Stdout).Encode(
			networkTotalsOutput{
				Epoch:    int64(ts.Height()),
				Endpoint: "NETWORK_WIDE_TOTALS",
				Payload:  netTotals,
			},
		)
	},
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

// Requested by @jbenet
// How many epochs back to look at for dealstats
var epochLookback = abi.ChainEpoch(10)

var resolvedWallets = map[address.Address]address.Address{}
var knownAddrMap = map[address.Address]string{}

//
// contents of basic_stats.json
type competitionTotalOutput struct {
	Epoch    int64            `json:"epoch"`
	Endpoint string           `json:"endpoint"`
	Payload  competitionTotal `json:"payload"`
}
type competitionTotal struct {
	UniqueCids      int   `json:"total_unique_cids"`
	UniqueProviders int   `json:"total_unique_providers"`
	UniqueProjects  int   `json:"total_unique_projects"`
	UniqueClients   int   `json:"total_unique_clients"`
	TotalDeals      int   `json:"total_num_deals"`
	TotalBytes      int64 `json:"total_stored_data_size"`

	seenProject  map[string]bool
	seenClient   map[address.Address]bool
	seenProvider map[address.Address]bool
	seenPieceCid map[cid.Cid]bool
}

//
// contents of client_stats.json
type projectAggregateStatsOutput struct {
	Epoch    int64                             `json:"epoch"`
	Endpoint string                            `json:"endpoint"`
	Payload  map[string]*projectAggregateStats `json:"payload"`
}
type projectAggregateStats struct {
	ProjectID           string                           `json:"project_id"`
	DataSizeMaxProvider int64                            `json:"max_data_size_stored_with_single_provider"`
	HighestCidDealCount int                              `json:"max_same_cid_deals"`
	DataSize            int64                            `json:"total_data_size"`
	NumCids             int                              `json:"total_num_cids"`
	NumDeals            int                              `json:"total_num_deals"`
	NumProviders        int                              `json:"total_num_providers"`
	ClientStats         map[string]*clientAggregateStats `json:"clients"`

	dataPerProvider map[address.Address]int64
	cidDeals        map[cid.Cid]int
}
type clientAggregateStats struct {
	Client       string `json:"client"`
	DataSize     int64  `json:"total_data_size"`
	NumCids      int    `json:"total_num_cids"`
	NumDeals     int    `json:"total_num_deals"`
	NumProviders int    `json:"total_num_providers"`

	providers map[address.Address]bool
	cids      map[cid.Cid]bool
}

//
// contents of deals_list_{{projid}}.json
type dealListOutput struct {
	Epoch    int64             `json:"epoch"`
	Endpoint string            `json:"endpoint"`
	Payload  []*individualDeal `json:"payload"`
}
type individualDeal struct {
	ProjectID      string `json:"project_id"`
	Client         string `json:"client"`
	DealID         string `json:"deal_id"`
	DealStartEpoch int64  `json:"deal_start_epoch"`
	MinerID        string `json:"miner_id"`
	PayloadCID     string `json:"payload_cid"`
	PaddedSize     int64  `json:"data_size"`
}

var rollupDealStatsCmd = &cli.Command{
	Name:  "rollup-deal-stats",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {

		if cctx.Args().Len() != 2 || cctx.Args().Get(0) == "" || cctx.Args().Get(1) == "" {
			return errors.New("must supply 2 arguments: a nonexistent target directory to write results to and a source of currently active projects")
		}

		outDirName := cctx.Args().Get(0)
		if _, err := os.Stat(outDirName); err == nil {
			return fmt.Errorf("unable to proceed: supplied stat target '%s' already exists", outDirName)
		}

		if err := os.MkdirAll(outDirName, 0755); err != nil {
			return fmt.Errorf("creation of destination '%s' failed: %s", outDirName, err)
		}

		ctx := lcli.ReqContext(cctx)

		projListName := cctx.Args().Get(1)
		var projListFh *os.File

		{
			// Parses JSON input in the form:
			// {
			// 	"payload": [
			// 		{
			// 			"project": "5fb5f5b3ad3275e236287ce3",
			// 			"address": "f3w3r2c6iukyh3u6f6kx62s5g6n2gf54aqp33ukqrqhje2y6xhf7k55przg4xqgahpcdal6laljz6zonma5pka"
			// 		},
			// 		{
			// 			"project": "5fb608c4ad3275e236287ced",
			// 			"address": "f3rs2khurnubol6ent27lpggidxxujqo2lg5aap5d5bmtam6yjb5wfla5cxxdgj45tqoaawgpzt5lofc3vpzfq"
			// 		},
			//  	...
			//  ]
			// }
			if strings.HasPrefix(projListName, "http://") || strings.HasPrefix(projListName, "https://") {
				req, err := http.NewRequestWithContext(ctx, "GET", projListName, nil)
				if err != nil {
					return err
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close() //nolint:errcheck

				if resp.StatusCode != http.StatusOK {
					return xerrors.Errorf("non-200 response: %d", resp.StatusCode)
				}

				projListFh, err = os.Create(outDirName + "/client_list.json")
				if err != nil {
					return err
				}

				_, err = io.Copy(projListFh, resp.Body)
				if err != nil {
					return err
				}
			} else {
				return errors.New("file inputs not yet supported")
			}

			if _, err := projListFh.Seek(0, 0); err != nil {
				return err
			}
			defer projListFh.Close() //nolint:errcheck

			projList, err := gabs.ParseJSONBuffer(projListFh)
			if err != nil {
				return err
			}
			proj, err := projList.Search("payload").Children()
			if err != nil {
				return err
			}
			for _, p := range proj {
				a, err := address.NewFromString(p.S("address").Data().(string))
				if err != nil {
					return err
				}

				knownAddrMap[a] = p.S("project").Data().(string)
			}

			if len(knownAddrMap) == 0 {
				return fmt.Errorf("no active projects/clients found in '%s': unable to continue", projListName)
			}
		}

		outClientStatsFd, err := os.Create(outDirName + "/client_stats.json")
		if err != nil {
			return err
		}
		defer outClientStatsFd.Close() //nolint:errcheck

		outBasicStatsFd, err := os.Create(outDirName + "/basic_stats.json")
		if err != nil {
			return err
		}
		defer outBasicStatsFd.Close() //nolint:errcheck

		outUnfilteredStatsFd, err := os.Create(outDirName + "/unfiltered_basic_stats.json")
		if err != nil {
			return err
		}
		defer outUnfilteredStatsFd.Close() //nolint:errcheck

		api, apiCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer apiCloser()

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		head, err = api.ChainGetTipSetByHeight(ctx, head.Height()-epochLookback, head.Key())
		if err != nil {
			return err
		}

		grandTotals := competitionTotal{
			seenProject:  make(map[string]bool),
			seenClient:   make(map[address.Address]bool),
			seenProvider: make(map[address.Address]bool),
			seenPieceCid: make(map[cid.Cid]bool),
		}

		unfilteredGrandTotals := competitionTotal{
			seenClient:   make(map[address.Address]bool),
			seenProvider: make(map[address.Address]bool),
			seenPieceCid: make(map[cid.Cid]bool),
		}

		projStats := make(map[string]*projectAggregateStats)
		projDealLists := make(map[string][]*individualDeal)

		deals, err := api.StateMarketDeals(ctx, head.Key())
		if err != nil {
			return err
		}

		for dealID, dealInfo := range deals {

			// Counting no-longer-active deals as per Pooja's request
			// // https://github.com/filecoin-project/specs-actors/blob/v0.9.9/actors/builtin/market/deal.go#L81-L85
			// if d.State.SectorStartEpoch < 0 {
			// 	continue
			// }

			clientAddr, found := resolvedWallets[dealInfo.Proposal.Client]
			if !found {
				var err error
				clientAddr, err = api.StateAccountKey(ctx, dealInfo.Proposal.Client, head.Key())
				if err != nil {
					log.Warnf("failed to resolve id '%s' to wallet address: %s", dealInfo.Proposal.Client, err)
					continue
				}

				resolvedWallets[dealInfo.Proposal.Client] = clientAddr
			}

			unfilteredGrandTotals.seenClient[clientAddr] = true
			unfilteredGrandTotals.TotalBytes += int64(dealInfo.Proposal.PieceSize)
			unfilteredGrandTotals.seenProvider[dealInfo.Proposal.Provider] = true
			unfilteredGrandTotals.seenPieceCid[dealInfo.Proposal.PieceCID] = true
			unfilteredGrandTotals.TotalDeals++

			projID, projKnown := knownAddrMap[clientAddr]
			if !projKnown {
				continue
			}

			grandTotals.seenProject[projID] = true
			grandTotals.seenClient[clientAddr] = true

			projStatEntry, ok := projStats[projID]
			if !ok {
				projStatEntry = &projectAggregateStats{
					ProjectID:       projID,
					ClientStats:     make(map[string]*clientAggregateStats),
					cidDeals:        make(map[cid.Cid]int),
					dataPerProvider: make(map[address.Address]int64),
				}
				projStats[projID] = projStatEntry
			}

			clientStatEntry, ok := projStatEntry.ClientStats[clientAddr.String()]
			if !ok {
				clientStatEntry = &clientAggregateStats{
					Client:    clientAddr.String(),
					cids:      make(map[cid.Cid]bool),
					providers: make(map[address.Address]bool),
				}
				projStatEntry.ClientStats[clientAddr.String()] = clientStatEntry
			}

			grandTotals.TotalBytes += int64(dealInfo.Proposal.PieceSize)
			projStatEntry.DataSize += int64(dealInfo.Proposal.PieceSize)
			clientStatEntry.DataSize += int64(dealInfo.Proposal.PieceSize)

			grandTotals.seenProvider[dealInfo.Proposal.Provider] = true
			projStatEntry.dataPerProvider[dealInfo.Proposal.Provider] += int64(dealInfo.Proposal.PieceSize)
			clientStatEntry.providers[dealInfo.Proposal.Provider] = true

			grandTotals.seenPieceCid[dealInfo.Proposal.PieceCID] = true
			projStatEntry.cidDeals[dealInfo.Proposal.PieceCID]++
			clientStatEntry.cids[dealInfo.Proposal.PieceCID] = true

			grandTotals.TotalDeals++
			projStatEntry.NumDeals++
			clientStatEntry.NumDeals++

			payloadCid := "unknown"
			if c, err := cid.Parse(dealInfo.Proposal.Label); err == nil {
				payloadCid = c.String()
			}

			projDealLists[projID] = append(projDealLists[projID], &individualDeal{
				DealID:         dealID,
				ProjectID:      projID,
				Client:         clientAddr.String(),
				MinerID:        dealInfo.Proposal.Provider.String(),
				PayloadCID:     payloadCid,
				PaddedSize:     int64(dealInfo.Proposal.PieceSize),
				DealStartEpoch: int64(dealInfo.State.SectorStartEpoch),
			})
		}

		//
		// Write out per-project deal lists
		for proj, dl := range projDealLists {
			err := func() error {
				outListFd, err := os.Create(fmt.Sprintf(outDirName+"/deals_list_%s.json", proj))
				if err != nil {
					return err
				}

				defer outListFd.Close() //nolint:errcheck

				ridiculousLintMandatedRebind := dl
				sort.Slice(dl, func(i, j int) bool {
					return ridiculousLintMandatedRebind[j].PaddedSize < ridiculousLintMandatedRebind[i].PaddedSize
				})

				if err := json.NewEncoder(outListFd).Encode(
					dealListOutput{
						Epoch:    int64(head.Height()),
						Endpoint: "DEAL_LIST",
						Payload:  dl,
					},
				); err != nil {
					return err
				}

				return nil
			}()

			if err != nil {
				return err
			}
		}

		//
		// write out basic_stats.json and unfiltered_basic_stats.json
		for _, st := range []*competitionTotal{&grandTotals, &unfilteredGrandTotals} {
			st.UniqueCids = len(st.seenPieceCid)
			st.UniqueClients = len(st.seenClient)
			st.UniqueProviders = len(st.seenProvider)
			if st.seenProject != nil {
				st.UniqueProjects = len(st.seenProject)
			}
		}

		if err := json.NewEncoder(outBasicStatsFd).Encode(
			competitionTotalOutput{
				Epoch:    int64(head.Height()),
				Endpoint: "COMPETITION_TOTALS",
				Payload:  grandTotals,
			},
		); err != nil {
			return err
		}

		if err := json.NewEncoder(outUnfilteredStatsFd).Encode(
			competitionTotalOutput{
				Epoch:    int64(head.Height()),
				Endpoint: "NETWORK_WIDE_TOTALS",
				Payload:  unfilteredGrandTotals,
			},
		); err != nil {
			return err
		}

		//
		// write out client_stats.json
		for _, ps := range projStats {
			ps.NumCids = len(ps.cidDeals)
			ps.NumProviders = len(ps.dataPerProvider)
			for _, dealsForCid := range ps.cidDeals {
				if ps.HighestCidDealCount < dealsForCid {
					ps.HighestCidDealCount = dealsForCid
				}
			}
			for _, dataForProvider := range ps.dataPerProvider {
				if ps.DataSizeMaxProvider < dataForProvider {
					ps.DataSizeMaxProvider = dataForProvider
				}
			}

			for _, cs := range ps.ClientStats {
				cs.NumCids = len(cs.cids)
				cs.NumProviders = len(cs.providers)
			}
		}

		if err := json.NewEncoder(outClientStatsFd).Encode(
			projectAggregateStatsOutput{
				Epoch:    int64(head.Height()),
				Endpoint: "PROJECT_DEAL_STATS",
				Payload:  projStats,
			},
		); err != nil {
			return err
		}

		return nil
	},
}

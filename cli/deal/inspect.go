package deal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	lapi "github.com/filecoin-project/lotus/api"
)

var mockDealInfos []lapi.DealInfo

func InspectDealCmd(ctx context.Context, api lapi.FullNode, proposalCid string, dealId int) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	deals, err := api.ClientListDeals(ctx)
	if err != nil {
		return err
	}

	var di lapi.DealInfo
	found := false
	for _, cdi := range deals {
		if proposalCid != "" && cdi.ProposalCid.String() == proposalCid {
			di = cdi
			found = true
			break
		}

		if dealId != 0 && int(cdi.DealID) == dealId {
			di = cdi
			found = true
			break
		}
	}

	if !found {
		if proposalCid != "" {
			return fmt.Errorf("cannot find deal with proposal cid: %s", proposalCid)
		}
		if dealId != 0 {
			return fmt.Errorf("cannot find deal with deal id: %v", dealId)
		}
		return errors.New("you must specify proposal cid or deal id in order to inspect a deal")
	}

	renderDeal(di)

	return nil
}

func renderDeal(di lapi.DealInfo) {
	color.Blue("Deal ID:      %d\n", int(di.DealID))
	color.Blue("Proposal CID: %s\n\n", di.ProposalCid.String())

	for _, stg := range di.DealStages.Stages {
		msg := fmt.Sprintf("%s %s: %s (%s)", color.BlueString("Stage:"), color.BlueString(strings.TrimPrefix(stg.Name, "StorageDeal")), stg.Description, color.GreenString(stg.ExpectedDuration))
		if stg.UpdatedTime.Time().IsZero() {
			msg = color.YellowString(msg)
		}
		fmt.Println(msg)

		for _, l := range stg.Logs {
			fmt.Printf("  %s %s\n", color.YellowString(l.UpdatedTime.Time().UTC().Round(time.Second).Format(time.Stamp)), l.Log)
		}

		if stg.Name == "StorageDealStartDataTransfer" {
			for _, dt_stg := range di.DataTransfer.Stages.Stages {

				fmt.Printf("        %s %s %s\n", color.YellowString(dt_stg.CreatedTime.Time().UTC().Round(time.Second).Format(time.Stamp)), color.BlueString("Data transfer stage:"), color.BlueString(dt_stg.Name))
				for _, l := range dt_stg.Logs {
					fmt.Printf("              %s %s\n", color.YellowString(l.UpdatedTime.Time().UTC().Round(time.Second).Format(time.Stamp)), l.Log)
				}
			}
		}
	}
}

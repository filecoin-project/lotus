package deal

import (
	"context"
	"fmt"
	"io"
	"time"

	tm "github.com/buger/goterm"
	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/ipfs/go-cid"
	term "github.com/nsf/termbox-go"
)

var mockDealInfos []lapi.DealInfo

func init() {
	dummyCid, err := cid.Parse("bafkqaaa")
	if err != nil {
		panic(err)
	}

	mockDealStages := storagemarket.DealStages{
		At: time.Now(),
		Stages: []storagemarket.DealStage{{
			Name:             "Reserving Funds",
			ExpectedDuration: "5 epochs",
			StartedAt:        time.Now().Add(-5 * time.Minute),
			Logs: []string{
				"Sending AddBalance message for 0.3FIL",
				"Waiting 5 epochs (2:30) for confirmation",
				"Funds for deal reserved successfully",
			},
		}, {
			Name:             "Sending Deal Proposal to Provider",
			ExpectedDuration: "5 epochs",
			StartedAt:        time.Now().Add(-2 * time.Minute),
			Logs: []string{
				"Proposal: 512MB for 300 days @ 0.1FIL / GB / epoch",
				"Deal proposal accepted by provider",
				"Funds for deal reserved successfully",
			},
		}, {
			Name:             "Sending deal data to Provider",
			ExpectedDuration: "13 minutes",
			StartedAt:        time.Now().Add(-53 * time.Second),
			Logs: []string{
				"Progress: 4:02 254MB / 1024MB (1MB / sec)",
				"Connection to Provider f01234 disconnected, retrying in 8s",
			},
		}, {
			Name:             "Waiting for deal to be published by Provider",
			ExpectedDuration: "several hours",
		}, {
			Name:             "Waiting for pre-commit message from Provider",
			ExpectedDuration: "several hours",
		}, {
			Name:             "Waiting for prove-commit message from Provider",
			ExpectedDuration: "several hours",
		}},
	}

	mockDealInfos = []lapi.DealInfo{{
		State:             3,
		Message:           "Reserving Client Funds",
		DealStages:        mockDealStages,
		Provider:          address.TestAddress,
		DataRef:           nil,
		PieceCID:          dummyCid,
		Size:              512 * 1024 * 1024,
		PricePerEpoch:     abi.NewTokenAmount(10),
		Duration:          300,
		DealID:            10,
		CreationTime:      time.Now().Add(-5 * time.Second),
		Verified:          false,
		TransferChannelID: nil,
		DataTransfer:      nil,
	}, {
		State:             5,
		Message:           "Publishing Deal",
		DealStages:        mockDealStages,
		Provider:          address.TestAddress,
		DataRef:           nil,
		PieceCID:          dummyCid,
		Size:              482 * 1024 * 1024,
		PricePerEpoch:     abi.NewTokenAmount(12),
		Duration:          323,
		DealID:            14,
		CreationTime:      time.Now().Add(-23 * time.Minute),
		Verified:          false,
		TransferChannelID: nil,
		DataTransfer:      nil,
	}, {
		State:             7,
		Message:           "Waiting for Pre-Commit",
		DealStages:        mockDealStages,
		Provider:          address.TestAddress,
		DataRef:           nil,
		PieceCID:          dummyCid,
		Size:              2 * 1024 * 1024 * 1024,
		PricePerEpoch:     abi.NewTokenAmount(8),
		Duration:          298,
		DealID:            8,
		CreationTime:      time.Now().Add(-3 * time.Hour),
		Verified:          false,
		TransferChannelID: nil,
		DataTransfer:      nil,
	}, {
		State:             2,
		Message:           "Transferring Data",
		DealStages:        mockDealStages,
		Provider:          address.TestAddress,
		DataRef:           nil,
		PieceCID:          dummyCid,
		Size:              23 * 1024 * 1024,
		PricePerEpoch:     abi.NewTokenAmount(11),
		Duration:          328,
		DealID:            3,
		CreationTime:      time.Now().Add(-49 * time.Hour),
		Verified:          false,
		TransferChannelID: nil,
		DataTransfer:      nil,
	}}
}

func ShowDealsCmd(ctx context.Context, api lapi.FullNode) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	//localDeals, err := api.ClientListDeals(ctx)
	//if err != nil {
	//	return err
	//}
	localDeals := mockDealInfos

	return showDealsUX(ctx, localDeals)
}

func showDealsUX(ctx context.Context, deals []lapi.DealInfo) error {
	err := term.Init()
	if err != nil {
		return err
	}
	defer term.Close()

	renderer := dealRenderer{out: tm.Screen}

	dealIdx := -1
	state := "main"
	highlighted := -1
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		switch state {
		case "main":
			renderMain := func(hlite int) error {
				tm.Clear()
				tm.MoveCursor(1, 1)
				err := renderer.renderDeals(deals, hlite)
				if err != nil {
					return err
				}
				tm.Flush()
				return nil
			}
			err := renderMain(highlighted)
			if err != nil {
				return err
			}

			switch ev := term.PollEvent(); ev.Type {
			case term.EventKey:
				switch {
				case ev.Ch == 'q', ev.Key == term.KeyEsc:
					return nil
				case ev.Key == term.KeyArrowUp:
					term.Sync()
					if highlighted > 0 {
						highlighted--
					}
				case ev.Key == term.KeyArrowDown:
					term.Sync()
					highlighted++
				case ev.Key == term.KeyEnter:
					term.Sync()
					dealIdx = highlighted
					state = "deal"
				}
			case term.EventError:
				return ev.Err
			}
		case "deal":
			tm.Clear()
			tm.MoveCursor(1, 1)
			renderer.renderDeal(deals[dealIdx])
			tm.Flush()

			switch ev := term.PollEvent(); ev.Type {
			case term.EventKey:
				if ev.Ch == 'q' || ev.Key == term.KeyEsc || ev.Key == term.KeyEnter || ev.Key == term.KeyArrowLeft {
					term.Sync()
					state = "main"
				}
			case term.EventError:
				return ev.Err
			}
		}
	}
}

type dealRenderer struct {
	out io.Writer
}

func (r *dealRenderer) renderDeals(deals []lapi.DealInfo, highlighted int) error {
	tw := tablewriter.New(
		tablewriter.Col(""),
		tablewriter.Col("Created"),
		tablewriter.Col("Provider"),
		tablewriter.Col("Size"),
		tablewriter.Col("State"),
	)
	for i, di := range deals {
		lineNum := fmt.Sprintf("%d", i+1)
		cols := map[string]interface{}{
			"":         lineNum,
			"Created":  time.Since(di.CreationTime).Round(time.Second),
			"Provider": di.Provider,
			"Size":     di.Size,
			"State":    di.Message,
		}
		if i == highlighted {
			for k, v := range cols {
				cols[k] = color.YellowString(fmt.Sprint(v))
			}
		}
		tw.Write(cols)
	}
	return tw.Flush(r.out)
}

func (r *dealRenderer) renderDeal(di lapi.DealInfo) error {
	_, err := fmt.Fprintf(r.out, "Deal %d\n", di.DealID)
	if err != nil {
		return err
	}
	for _, stg := range di.DealStages.Stages {
		msg := fmt.Sprintf("%s (%s)", stg.Name, stg.ExpectedDuration)
		if stg.StartedAt.IsZero() {
			msg = color.YellowString(msg)
		}
		_, err := fmt.Fprintf(r.out, "%s\n", msg)
		if err != nil {
			return err
		}
		for _, l := range stg.Logs {
			_, err = fmt.Fprintf(r.out, "  %s\n", l)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

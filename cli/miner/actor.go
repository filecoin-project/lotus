package miner

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	lapi "github.com/filecoin-project/lotus/api"
	builtin2 "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "manipulate the miner actor",
	Subcommands: []*cli.Command{
		spcli.ActorSetAddrsCmd(LMConfigOrActorGetter),
		spcli.ActorDealSettlementCmd(LMConfigOrActorGetter),
		spcli.ActorWithdrawCmd(LMConfigOrActorGetter),
		spcli.ActorRepayDebtCmd(LMConfigOrActorGetter),
		spcli.ActorSetPeeridCmd(LMConfigOrActorGetter),
		spcli.ActorSetOwnerCmd(LMConfigOrActorGetter),
		spcli.ActorControlCmd(LMConfigOrActorGetter, actorControlListCmd),
		spcli.ActorProposeChangeWorkerCmd(LMConfigOrActorGetter),
		spcli.ActorConfirmChangeWorkerCmd(LMConfigOrActorGetter),
		spcli.ActorCompactAllocatedCmd(LMConfigOrActorGetter),
		spcli.ActorProposeChangeBeneficiaryCmd(LMConfigOrActorGetter),
		spcli.ActorConfirmChangeBeneficiaryCmd(LMConfigOrActorGetter),
	},
}

func LMConfigOrActorGetter(cctx *cli.Context) (address.Address, error) {
	ctx := lcli.ReqContext(cctx)
	return getActorAddress(ctx, cctx)
}

func getControlAddresses(cctx *cli.Context, actor address.Address) (lapi.AddressConfig, error) {
	minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return lapi.AddressConfig{}, err
	}
	defer closer()

	ctx := lcli.ReqContext(cctx)
	return minerApi.ActorAddressConfig(ctx)
}

var actorControlListCmd = &cli.Command{
	Name:  "list",
	Usage: "Get currently set control addresses",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "verbose",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, acloser, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := LMActorOrEnvGetter(cctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		tw := tablewriter.New(
			tablewriter.Col("name"),
			tablewriter.Col("ID"),
			tablewriter.Col("key"),
			tablewriter.Col("use"),
			tablewriter.Col("balance"),
		)

		ac, err := getControlAddresses(cctx, maddr)
		if err != nil {
			return err
		}
		commit := map[address.Address]struct{}{}
		precommit := map[address.Address]struct{}{}
		terminate := map[address.Address]struct{}{}
		dealPublish := map[address.Address]struct{}{}
		post := map[address.Address]struct{}{}

		for _, ca := range mi.ControlAddresses {
			post[ca] = struct{}{}
		}

		for _, ca := range ac.PreCommitControl {
			ca, err := api.StateLookupID(ctx, ca, types.EmptyTSK)
			if err != nil {
				return err
			}

			delete(post, ca)
			precommit[ca] = struct{}{}
		}

		for _, ca := range ac.CommitControl {
			ca, err := api.StateLookupID(ctx, ca, types.EmptyTSK)
			if err != nil {
				return err
			}

			delete(post, ca)
			commit[ca] = struct{}{}
		}

		for _, ca := range ac.TerminateControl {
			ca, err := api.StateLookupID(ctx, ca, types.EmptyTSK)
			if err != nil {
				return err
			}

			delete(post, ca)
			terminate[ca] = struct{}{}
		}

		for _, ca := range ac.DealPublishControl {
			ca, err := api.StateLookupID(ctx, ca, types.EmptyTSK)
			if err != nil {
				return err
			}

			delete(post, ca)
			dealPublish[ca] = struct{}{}
		}

		printKey := func(name string, a address.Address) {
			var actor *types.Actor
			if actor, err = api.StateGetActor(ctx, a, types.EmptyTSK); err != nil {
				fmt.Printf("%s\t%s: error getting actor: %s\n", name, a, err)
				return
			}
			b := actor.Balance

			var k = a
			// 'a' maybe a 'robust', in that case, 'StateAccountKey' returns an error.
			if builtin2.IsAccountActor(actor.Code) {
				if k, err = api.StateAccountKey(ctx, a, types.EmptyTSK); err != nil {
					fmt.Printf("%s\t%s: error getting account key: %s\n", name, a, err)
					return
				}
			}
			kstr := k.String()
			if !cctx.Bool("verbose") {
				if len(kstr) > 9 {
					kstr = kstr[:6] + "..."
				}
			}

			bstr := types.FIL(b).String()
			switch {
			case b.LessThan(types.FromFil(10)):
				bstr = color.RedString(bstr)
			case b.LessThan(types.FromFil(50)):
				bstr = color.YellowString(bstr)
			default:
				bstr = color.GreenString(bstr)
			}

			var uses []string
			if a == mi.Worker {
				uses = append(uses, color.YellowString("other"))
			}
			if _, ok := post[a]; ok {
				uses = append(uses, color.GreenString("post"))
			}
			if _, ok := precommit[a]; ok {
				uses = append(uses, color.CyanString("precommit"))
			}
			if _, ok := commit[a]; ok {
				uses = append(uses, color.BlueString("commit"))
			}
			if _, ok := terminate[a]; ok {
				uses = append(uses, color.YellowString("terminate"))
			}
			if _, ok := dealPublish[a]; ok {
				uses = append(uses, color.MagentaString("deals"))
			}

			tw.Write(map[string]interface{}{
				"name":    name,
				"ID":      a,
				"key":     kstr,
				"use":     strings.Join(uses, " "),
				"balance": bstr,
			})
		}

		printKey("owner", mi.Owner)
		printKey("worker", mi.Worker)
		printKey("beneficiary", mi.Beneficiary)
		for i, ca := range mi.ControlAddresses {
			printKey(fmt.Sprintf("control-%d", i), ca)
		}

		return tw.Flush(os.Stdout)
	},
}

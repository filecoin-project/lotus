package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	builtin2 "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "Manage Filecoin Miner Actor Metadata",
	Subcommands: []*cli.Command{
		spcli.ActorSetAddrsCmd(SPTActorGetter),
		spcli.ActorWithdrawCmd(SPTActorGetter),
		spcli.ActorRepayDebtCmd(SPTActorGetter),
		spcli.ActorSetPeeridCmd(SPTActorGetter),
		spcli.ActorSetOwnerCmd(SPTActorGetter),
		spcli.ActorControlCmd(SPTActorGetter, actorControlListCmd(SPTActorGetter)),
		spcli.ActorProposeChangeWorkerCmd(SPTActorGetter),
		spcli.ActorConfirmChangeWorkerCmd(SPTActorGetter),
		spcli.ActorCompactAllocatedCmd(SPTActorGetter),
		spcli.ActorProposeChangeBeneficiaryCmd(SPTActorGetter),
		spcli.ActorConfirmChangeBeneficiaryCmd(SPTActorGetter),
		spcli.ActorNewMinerCmd,
	},
}

func actorControlListCmd(getActor spcli.ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "Get currently set control addresses. Note: This excludes most roles as they are not known to the immediate chain state.",
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

			maddr, err := getActor(cctx)
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

			post := map[address.Address]struct{}{}

			for _, ca := range mi.ControlAddresses {
				post[ca] = struct{}{}
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
}

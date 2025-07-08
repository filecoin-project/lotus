package main

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	lcli "github.com/filecoin-project/lotus/cli"
)

var findMsgCmd = &cli.Command{
	Name: "find-msg",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "tipset or height (@X or @head for latest)",
			Value: "@head",
		},
		&cli.IntFlag{
			Name:  "count",
			Usage: "number of tipsets to inspect, working backwards from the --tipset",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("failed to get full node API: %w", err)
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return xerrors.Errorf("failed to load tipset: %w", err)
		}

		if !cctx.IsSet("count") {
			return fmt.Errorf("count is required")
		}
		count := cctx.Int("count")

		if cctx.Args().Len() != 1 {
			return fmt.Errorf("method (<actor>:<method name>) is required")
		}
		method := cctx.Args().Get(0)
		if len(strings.Split(method, ":")) != 2 {
			return fmt.Errorf("method (<actor>:<method name>) is required")
		}
		actor, methodName := strings.Split(method, ":")[0], strings.Split(method, ":")[1]

		nv, err := api.StateNetworkVersion(ctx, ts.Key())
		if err != nil {
			return xerrors.Errorf("failed to get network version: %w", err)
		}
		av, err := actorstypes.VersionForNetwork(nv)
		if err != nil {
			return xerrors.Errorf("failed to get actor version: %w", err)
		}
		actorKeys := manifest.GetBuiltinActorsKeys(av)
		if !slices.Contains(actorKeys, actor) {
			return fmt.Errorf("actor %s not valid, one one of: %s", actor, strings.Join(actorKeys, ", "))
		}

		methods, err := (func() (any, error) {
			switch actor {
			case "account":
				return builtin.MethodsAccount, nil
			case "cron":
				return builtin.MethodsCron, nil
			case "init":
				return builtin.MethodsInit, nil
			case "storagemarket":
				return builtin.MethodsMarket, nil
			case "storageminer":
				return builtin.MethodsMiner, nil
			case "multisig":
				return builtin.MethodsMultisig, nil
			case "paymentchannel":
				return builtin.MethodsPaych, nil
			case "storagepower":
				return builtin.MethodsPower, nil
			case "reward":
				return builtin.MethodsReward, nil
			case "system":
				return struct{}{}, nil // nothing here
			case "verifiedregistry":
				return builtin.MethodsVerifiedRegistry, nil
			case "datacap":
				return builtin.MethodsDatacap, nil
			case "evm":
				return builtin.MethodsEVM, nil
			case "eam":
				return builtin.MethodsEAM, nil
			case "placeholder":
				return builtin.MethodsPlaceholder, nil
			case "ethaccount":
				return builtin.MethodsEthAccount, nil
			default:
				return nil, fmt.Errorf("actor %s not valid, one one of: %s", actor, strings.Join(actorKeys, ", "))
			}
		}())
		if err != nil {
			return xerrors.Errorf("failed to get methods for actor %s: %w", actor, err)
		}

		// use reflection to find the field name on the methods object, and then the uint64 value of that field

		var methodNum abi.MethodNum
		var found bool
		methodNames := make([]string, 0)
		value := reflect.ValueOf(methods)
		typeOfMethods := value.Type()
		for i := 0; i < typeOfMethods.NumField(); i++ {
			field := typeOfMethods.Field(i)
			if field.Name == methodName {
				methodNum = abi.MethodNum(value.Field(i).Uint())
				found = true
				break
			}
			methodNames = append(methodNames, field.Name)
		}
		if !found && methodName != "Send" {
			return fmt.Errorf("method %s not valid on actor %s, want one of: %s", methodName, actor, strings.Join(methodNames, ", "))
		} // else Send as method 0 is a special case we can handle

		var msgCnt int
		actorIsType := make(map[address.Address]bool)
		metaCache := make(map[cid.Cid]string)

		additional := ""
		switch actor {
		case "storageminer":
			switch methodName {
			case "ProveCommitSectors3":
				additional = ", Sectors, Type"
			}
		}
		_, _ = fmt.Fprintf(cctx.App.Writer, "Epoch, MsgCid, From, To, Method%s\n", additional)

		for i := 0; i < count; i++ {
			if ts.Height()%2880 == 0 {
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Epoch: %d, messages: %d\n", ts.Height(), msgCnt)
			}
			msgs, err := api.ChainGetMessagesInTipset(ctx, ts.Key())
			if err != nil {
				return xerrors.Errorf("failed to get messages in tipset: %w", err)
			}
			msgCnt += len(msgs)

			for _, msg := range msgs {
				if msg.Message.Method != methodNum {
					continue
				}
				if is, ok := actorIsType[msg.Message.To]; ok && !is {
					continue
				}

				var name string
				act, err := api.StateGetActor(ctx, msg.Message.To, ts.Key())
				if err != nil && !strings.Contains(err.Error(), "actor not found") {
					return xerrors.Errorf("failed to get actor: %w", err)
				} else if err != nil {
					// not found, assume it's an account actor (may not be, but that's a TODO)
					name = "account"
				} else {
					var has bool
					if name, has = metaCache[act.Code]; !has {
						var ok bool
						if name, _, ok = actors.GetActorMetaByCode(act.Code); ok {
							metaCache[act.Code] = name
						} else {
							_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Unknown actor code: %s\n", act.Code)
						}
					}
				}
				if name != actor {
					actorIsType[msg.Message.To] = false
					continue
				}
				actorIsType[msg.Message.To] = true

				additional := ""

				switch actor {
				case "storageminer":
					switch methodName {
					case "ProveCommitSectors3":
						var params miner16.ProveCommitSectors3Params
						if err := params.UnmarshalCBOR(bytes.NewReader(msg.Message.Params)); err != nil {
							return err
						}
						typ := "batch"
						if params.AggregateProof != nil {
							typ = "aggregate"
						}
						additional = fmt.Sprintf(", %d, %s", len(params.SectorActivations), typ)
					}
				}

				_, _ = fmt.Fprintf(cctx.App.Writer, "%d, %s, %s, %s, %d%s\n", ts.Height(), msg.Cid, msg.Message.From, msg.Message.To, msg.Message.Method, additional)
			}

			if ts, err = api.ChainGetTipSet(ctx, ts.Parents()); err != nil {
				return xerrors.Errorf("failed to get tipset: %w", err)
			}
		}
		return nil
	},
}

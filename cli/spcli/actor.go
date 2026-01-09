package spcli

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	markettypes14 "github.com/filecoin-project/go-state-types/builtin/v14/market"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	marketactor "github.com/filecoin-project/lotus/chain/actors/builtin/market"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli/createminer"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/node/impl"
)

func ActorDealSettlementCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "settle-deal",
		Usage:     "Settle deals manually, if dealIds are not provided all deals will be settled. Deal IDs can be specified as individual numbers or ranges (e.g., '123 124 125-200 220')",
		ArgsUsage: "[...dealIds]",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "confidence",
				Usage: "number of block confirmations to wait for",
				Value: int(buildconstants.MessageConfidence),
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "specify where to send the message from (any address)",
			},
			&cli.IntFlag{
				Name:  "max-deals",
				Usage: "the maximum number of deals contained in each message",
				Value: 50,
			},
			&cli.BoolFlag{
				Name:  "skip-wait-msg",
				Usage: "skip to check the message status",
			},
			&cli.BoolFlag{
				Name:  "all-deals",
				Usage: "settle all deals. only expired deals are calculated by default",
			},
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
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
				return xerrors.Errorf("Error getting miner info: %w", err)
			}

			fromAddr := mi.Worker
			if addr := cctx.String("from"); addr != "" {
				fromAddr, err = address.NewFromString(addr)
				if err != nil {
					return err
				}
			}

			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}
			alldeals := cctx.Bool("all-deals")

			var (
				dealIDs []uint64
				dealId  uint64
			)

			// if no deal ids are provided, get all the deals for the miner
			if dealsIdArgs := cctx.Args().Slice(); len(dealsIdArgs) != 0 {
				for _, d := range dealsIdArgs {
					// Check if it's a range (e.g., "125-200")
					if strings.Contains(d, "-") {
						parts := strings.Split(d, "-")
						if len(parts) != 2 {
							return xerrors.Errorf("Invalid range format: %s (expected format: start-end)", d)
						}

						start, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
						if err != nil {
							return xerrors.Errorf("Error parsing start of range %s: %w", d, err)
						}

						end, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
						if err != nil {
							return xerrors.Errorf("Error parsing end of range %s: %w", d, err)
						}

						if start > end {
							return xerrors.Errorf("Invalid range %s: start must be less than or equal to end", d)
						}

						// Add all deal IDs in the range (inclusive)
						for id := start; id <= end; id++ {
							dealIDs = append(dealIDs, id)
						}
					} else {
						// Parse as a single deal ID
						dealId, err = strconv.ParseUint(d, 10, 64)
						if err != nil {
							return xerrors.Errorf("Error parsing deal id: %w", err)
						}

						dealIDs = append(dealIDs, dealId)
					}
				}
			} else {
				sectors, err := api.StateMinerSectors(ctx, maddr, nil, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("Error getting StateMinerSectors for miner: %w", err)
				}
				wapi := &v0api.WrapperV1Full{FullNode: api}
				for _, sector := range sectors {
					data, err := lcli.GetMarketDealIDs(ctx, wapi, maddr, sector.SectorNumber, types.EmptyTSK)
					if err != nil {
						return xerrors.Errorf("Error getting all deals for miner: %w", err)
					}
					for _, deal := range data {
						marketDeal, err := api.StateMarketStorageDeal(ctx, deal, types.EmptyTSK)
						if err != nil {
							continue
						}
						if marketDeal.Proposal.EndEpoch < head.Height() || alldeals {
							dealIDs = append(dealIDs, uint64(deal))
						}
					}
				}
			}

			msgLen := (len(dealIDs) + cctx.Int("max-deals") - 1) / cctx.Int("max-deals")
			fmt.Printf("There are a total of %v deals, and about %v messages will be sent out.\n", len(dealIDs), msgLen)

			if !cctx.Bool("really-do-it") {
				return fmt.Errorf("pass --really-do-it to confirm this action")
			}

			sort.Slice(dealIDs, func(i, j int) bool {
				return dealIDs[i] < dealIDs[j]
			})
			dealChucks := lo.Chunk(dealIDs, cctx.Int("max-deals"))

			var msgs []*types.Message
			for _, deals := range dealChucks {
				dealParams := bitfield.NewFromSet(deals)
				params, err := actors.SerializeParams(&dealParams)
				if err != nil {
					return err
				}
				msg := &types.Message{
					To:     marketactor.Address,
					From:   fromAddr,
					Value:  types.NewInt(0),
					Method: marketactor.Methods.SettleDealPaymentsExported,
					Params: params,
				}
				msgs = append(msgs, msg)
			}

			// MpoolBatchPushMessage method will take care of gas estimation and funds check
			smsgs, err := api.MpoolBatchPushMessage(ctx, msgs, nil)
			if smsgs == nil && err != nil {
				return err
			}

			if cctx.Bool("skip-wait-msg") {
				fmt.Printf("skip the check status, please pay attention to the message status on the chain by yourself.\n")
				return nil
			}

			// wait for msgs to get mined into a block
			eg := errgroup.Group{}
			eg.SetLimit(10)
			for _, smsg := range smsgs {
				res := smsg.Cid()
				fmt.Printf("Requested deal settlement in message: %s\nwaiting for it to be included in a block..\n", res)
				eg.Go(func() error {
					// wait for it to get mined into a block
					wait, err := api.StateWaitMsg(ctx, res, uint64(cctx.Int("confidence")), lapi.LookbackNoLimit, true)
					if err != nil {
						return xerrors.Errorf("Timeout waiting for deal settlement message %s", res)
					}

					if wait.Receipt.ExitCode.IsError() {
						return xerrors.Errorf("Failed to execute withdrawal message %s: %w", wait.Message, wait.Receipt.ExitCode.Error())
					}

					var settlementReturn markettypes14.SettleDealPaymentsReturn
					if err = settlementReturn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
						return err
					}

					var (
						totalPayment        = big.Zero()
						totalCompletedDeals = 0
					)

					for _, s := range settlementReturn.Settlements {
						totalPayment = big.Add(totalPayment, s.Payment)
						if s.Completed {
							totalCompletedDeals++
						}
					}

					fmt.Printf("Message CID: %s\nSettled %d out of %d deals\nTotal payment: %s\nTotal number of deals finished their lifetime: %d\n",
						res, settlementReturn.Results.SuccessCount, len(dealIDs), types.FIL(totalPayment), totalCompletedDeals)
					return nil
				})
			}
			return eg.Wait()
		},
	}
}

func ActorWithdrawCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "withdraw",
		Usage:     "withdraw available balance to beneficiary",
		ArgsUsage: "[amount (FIL)]",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "confidence",
				Usage: "number of block confirmations to wait for",
				Value: int(buildconstants.MessageConfidence),
			},
			&cli.BoolFlag{
				Name:  "beneficiary",
				Usage: "send withdraw message from the beneficiary address",
			},
		},
		Action: func(cctx *cli.Context) error {
			amount := abi.NewTokenAmount(0)

			if cctx.Args().Present() {
				f, err := types.ParseFIL(cctx.Args().First())
				if err != nil {
					return xerrors.Errorf("parsing 'amount' argument: %w", err)
				}

				amount = abi.TokenAmount(f)
			}

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

			res, err := impl.WithdrawBalance(ctx, api, maddr, amount, !cctx.IsSet("beneficiary"))
			if err != nil {
				return err
			}

			fmt.Printf("Requested withdrawal in message %s\nwaiting for it to be included in a block..\n", res)

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, res, uint64(cctx.Int("confidence")), lapi.LookbackNoLimit, true)
			if err != nil {
				return xerrors.Errorf("Timeout waiting for withdrawal message %s", res)
			}

			if wait.Receipt.ExitCode.IsError() {
				return xerrors.Errorf("Failed to execute withdrawal message %s: %w", wait.Message, wait.Receipt.ExitCode.Error())
			}

			nv, err := api.StateNetworkVersion(ctx, wait.TipSet)
			if err != nil {
				return err
			}

			if nv >= network.Version14 {
				var withdrawn abi.TokenAmount
				if err := withdrawn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
					return err
				}

				fmt.Printf("Successfully withdrew %s \n", types.FIL(withdrawn))
				if withdrawn.LessThan(amount) {
					fmt.Printf("Note that this is less than the requested amount of %s\n", types.FIL(amount))
				}
			}

			return nil
		},
	}
}

func ActorSetAddrsCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "set-addresses",
		Aliases:   []string{"set-addrs"},
		Usage:     "set addresses that your miner can be publicly dialed on",
		ArgsUsage: "<multiaddrs>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "from",
				Usage: "optionally specify the account to send the message from",
			},
			&cli.Int64Flag{
				Name:  "gas-limit",
				Usage: "set gas limit",
				Value: 0,
			},
			&cli.BoolFlag{
				Name:  "unset",
				Usage: "unset address",
				Value: false,
			},
		},
		Action: func(cctx *cli.Context) error {
			args := cctx.Args().Slice()
			unset := cctx.Bool("unset")
			if len(args) == 0 && !unset {
				return cli.ShowSubcommandHelp(cctx)
			}
			if len(args) > 0 && unset {
				return fmt.Errorf("unset can only be used with no arguments")
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			var addrs []abi.Multiaddrs
			for _, a := range args {
				maddr, err := ma.NewMultiaddr(a)
				if err != nil {
					return fmt.Errorf("failed to parse %q as a multiaddr: %w", a, err)
				}

				maddrNop2p, strip := ma.SplitFunc(maddr, func(c ma.Component) bool {
					return c.Protocol().Code == ma.P_P2P
				})

				if strip != nil {
					fmt.Println("Stripping peerid ", strip, " from ", maddr)
				}
				addrs = append(addrs, maddrNop2p.Bytes())
			}

			maddr, err := getActor(cctx)
			if err != nil {
				return err
			}

			minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			fromAddr := minfo.Worker
			if from := cctx.String("from"); from != "" {
				addr, err := address.NewFromString(from)
				if err != nil {
					return err
				}

				fromAddr = addr
			}

			fromId, err := api.StateLookupID(ctx, fromAddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			if !isController(minfo, fromId) {
				return xerrors.Errorf("sender isn't a controller of miner: %s", fromId)
			}

			params, err := actors.SerializeParams(&miner.ChangeMultiaddrsParams{NewMultiaddrs: addrs})
			if err != nil {
				return err
			}

			gasLimit := cctx.Int64("gas-limit")

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				To:       maddr,
				From:     fromId,
				Value:    types.NewInt(0),
				GasLimit: gasLimit,
				Method:   builtin.MethodsMiner.ChangeMultiaddrs,
				Params:   params,
			}, nil)
			if err != nil {
				return err
			}

			fmt.Printf("Requested multiaddrs change in message %s\n", smsg.Cid())
			return nil

		},
	}
}

func ActorSetPeeridCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "set-peer-id",
		Usage:     "set the peer id of your miner",
		ArgsUsage: "<peer id>",
		Flags: []cli.Flag{
			&cli.Int64Flag{
				Name:  "gas-limit",
				Usage: "set gas limit",
				Value: 0,
			},
		},
		Action: func(cctx *cli.Context) error {

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			if cctx.NArg() != 1 {
				return lcli.IncorrectNumArgs(cctx)
			}

			pid, err := peer.Decode(cctx.Args().Get(0))
			if err != nil {
				return fmt.Errorf("failed to parse input as a peerId: %w", err)
			}

			maddr, err := getActor(cctx)
			if err != nil {
				return err
			}

			minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			params, err := actors.SerializeParams(&miner.ChangePeerIDParams{NewID: abi.PeerID(pid)})
			if err != nil {
				return err
			}

			gasLimit := cctx.Int64("gas-limit")

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				To:       maddr,
				From:     minfo.Worker,
				Value:    types.NewInt(0),
				GasLimit: gasLimit,
				Method:   builtin.MethodsMiner.ChangePeerID,
				Params:   params,
			}, nil)
			if err != nil {
				return err
			}

			fmt.Printf("Requested peerid change in message %s\n", smsg.Cid())
			return nil

		},
	}
}

func ActorRepayDebtCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "repay-debt",
		Usage:     "pay down a miner's debt",
		ArgsUsage: "[amount (FIL)]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "from",
				Usage: "optionally specify the account to send funds from",
			},
		},
		Action: func(cctx *cli.Context) error {
			api, acloser, err := lcli.GetFullNodeAPI(cctx)
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

			var amount abi.TokenAmount
			if cctx.Args().Present() {
				f, err := types.ParseFIL(cctx.Args().First())
				if err != nil {
					return xerrors.Errorf("parsing 'amount' argument: %w", err)
				}

				amount = abi.TokenAmount(f)
			} else {
				mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
				if err != nil {
					return err
				}

				store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))

				mst, err := lminer.Load(store, mact)
				if err != nil {
					return err
				}

				amount, err = mst.FeeDebt()
				if err != nil {
					return err
				}

			}

			fromAddr := mi.Worker
			if from := cctx.String("from"); from != "" {
				addr, err := address.NewFromString(from)
				if err != nil {
					return err
				}

				fromAddr = addr
			}

			fromId, err := api.StateLookupID(ctx, fromAddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			if !isController(mi, fromId) {
				return xerrors.Errorf("sender isn't a controller of miner: %s", fromId)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				To:     maddr,
				From:   fromId,
				Value:  amount,
				Method: builtin.MethodsMiner.RepayDebt,
				Params: nil,
			}, nil)
			if err != nil {
				return err
			}

			fmt.Printf("Sent repay debt message %s\n", smsg.Cid())

			return nil
		},
	}
}

func ActorControlCmd(getActor ActorAddressGetter, actorControlListCmd *cli.Command) *cli.Command {
	return &cli.Command{
		Name:  "control",
		Usage: "Manage control addresses",
		Subcommands: []*cli.Command{
			actorControlListCmd,
			actorControlSet(getActor),
		},
	}
}

func actorControlSet(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Set control address(-es)",
		ArgsUsage: "[...address]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
				Value: false,
			},
		},
		Action: func(cctx *cli.Context) error {

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
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

			del := map[address.Address]struct{}{}
			existing := map[address.Address]struct{}{}
			for _, controlAddress := range mi.ControlAddresses {
				ka, err := api.StateAccountKey(ctx, controlAddress, types.EmptyTSK)
				if err != nil {
					return err
				}

				del[ka] = struct{}{}
				existing[ka] = struct{}{}
			}

			var toSet []address.Address

			for i, as := range cctx.Args().Slice() {
				a, err := address.NewFromString(as)
				if err != nil {
					return xerrors.Errorf("parsing address %d: %w", i, err)
				}

				ka, err := api.StateAccountKey(ctx, a, types.EmptyTSK)
				if err != nil {
					return err
				}

				// make sure the address exists on chain
				_, err = api.StateLookupID(ctx, ka, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("looking up %s: %w", ka, err)
				}

				delete(del, ka)
				toSet = append(toSet, ka)
			}

			for a := range del {
				fmt.Println("Remove", a)
			}
			for _, a := range toSet {
				if _, exists := existing[a]; !exists {
					fmt.Println("Add", a)
				}
			}

			if !cctx.Bool("really-do-it") {
				fmt.Println("Pass --really-do-it to actually execute this action")
				return nil
			}

			cwp := &miner.ChangeWorkerAddressParams{
				NewWorker:       mi.Worker,
				NewControlAddrs: toSet,
			}

			sp, err := actors.SerializeParams(cwp)
			if err != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Owner,
				To:     maddr,
				Method: builtin.MethodsMiner.ChangeWorkerAddress,

				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push: %w", err)
			}

			fmt.Println("Message CID:", smsg.Cid())

			return nil
		},
	}
}

func ActorSetOwnerCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "set-owner",
		Usage:     "Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)",
		ArgsUsage: "[newOwnerAddress senderAddress]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
				Value: false,
			},
		},
		Action: func(cctx *cli.Context) error {
			if cctx.NArg() != 2 {
				return lcli.IncorrectNumArgs(cctx)
			}

			if !cctx.Bool("really-do-it") {
				fmt.Println("Pass --really-do-it to actually execute this action")
				return nil
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			na, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}

			newAddrId, err := api.StateLookupID(ctx, na, types.EmptyTSK)
			if err != nil {
				return err
			}

			fa, err := address.NewFromString(cctx.Args().Get(1))
			if err != nil {
				return err
			}

			fromAddrId, err := api.StateLookupID(ctx, fa, types.EmptyTSK)
			if err != nil {
				return err
			}

			maddr, err := getActor(cctx)
			if err != nil {
				return err
			}

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			if fromAddrId != mi.Owner && fromAddrId != newAddrId {
				return xerrors.New("from address must either be the old owner or the new owner")
			}

			sp, err := actors.SerializeParams(&newAddrId)
			if err != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   fromAddrId,
				To:     maddr,
				Method: builtin.MethodsMiner.ChangeOwnerAddress,
				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push: %w", err)
			}

			fmt.Println("Message CID:", smsg.Cid())

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
			if err != nil {
				return err
			}

			// check it executed successfully
			if wait.Receipt.ExitCode.IsError() {
				fmt.Println("owner change failed!")
				return err
			}

			fmt.Println("message succeeded!")

			return nil
		},
	}
}

func ActorProposeChangeWorkerCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "propose-change-worker",
		Usage:     "Propose a worker address change",
		ArgsUsage: "[address]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
				Value: false,
			},
		},
		Action: func(cctx *cli.Context) error {
			if !cctx.Args().Present() {
				return fmt.Errorf("must pass address of new worker address")
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			na, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}

			newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
			if err != nil {
				return err
			}

			maddr, err := getActor(cctx)
			if err != nil {
				return err
			}

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			if mi.NewWorker.Empty() {
				if mi.Worker == newAddr {
					return fmt.Errorf("worker address already set to %s", na)
				}
			} else {
				if mi.NewWorker == newAddr {
					return fmt.Errorf("change to worker address %s already pending", na)
				}
			}

			if !cctx.Bool("really-do-it") {
				_, _ = fmt.Fprintln(cctx.App.Writer, "Pass --really-do-it to actually execute this action")
				return nil
			}

			cwp := &miner.ChangeWorkerAddressParams{
				NewWorker:       newAddr,
				NewControlAddrs: mi.ControlAddresses,
			}

			sp, err := actors.SerializeParams(cwp)
			if err != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Owner,
				To:     maddr,
				Method: builtin.MethodsMiner.ChangeWorkerAddress,
				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push: %w", err)
			}

			_, _ = fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", smsg.Cid())

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
			if err != nil {
				return err
			}

			// check it executed successfully
			if wait.Receipt.ExitCode.IsError() {
				return fmt.Errorf("propose worker change failed")
			}

			mi, err = api.StateMinerInfo(ctx, maddr, wait.TipSet)
			if err != nil {
				return err
			}
			if mi.NewWorker != newAddr {
				return fmt.Errorf("proposed worker address change not reflected on chain: expected '%s', found '%s'", na, mi.NewWorker)
			}

			_, _ = fmt.Fprintf(cctx.App.Writer, "Worker key change to %s successfully sent, change happens at height %d.\n", na, mi.WorkerChangeEpoch)
			_, _ = fmt.Fprintf(cctx.App.Writer, "If you have no active deadlines, call 'confirm-change-worker' at or after height %d to complete.\n", mi.WorkerChangeEpoch)

			return nil
		},
	}
}

func ActorProposeChangeBeneficiaryCmd(getActor ActorAddressGetter) *cli.Command {

	return &cli.Command{
		Name:      "propose-change-beneficiary",
		Usage:     "Propose a beneficiary address change",
		ArgsUsage: "[beneficiaryAddress quota expiration]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "overwrite-pending-change",
				Usage: "Overwrite the current beneficiary change proposal",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "actor",
				Usage: "specify the address of miner actor",
			},
		},
		Action: func(cctx *cli.Context) error {
			if cctx.NArg() != 3 {
				return lcli.IncorrectNumArgs(cctx)
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return xerrors.Errorf("getting fullnode api: %w", err)
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			na, err := address.NewFromString(cctx.Args().Get(0))
			if err != nil {
				return xerrors.Errorf("parsing beneficiary address: %w", err)
			}

			newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("looking up new beneficiary address: %w", err)
			}

			quota, err := types.ParseFIL(cctx.Args().Get(1))
			if err != nil {
				return xerrors.Errorf("parsing quota: %w", err)
			}

			expiration, err := strconv.ParseInt(cctx.Args().Get(2), 10, 64)
			if err != nil {
				return xerrors.Errorf("parsing expiration: %w", err)
			}

			maddr, err := getActor(cctx)
			if err != nil {
				return xerrors.Errorf("getting miner address: %w", err)
			}

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting miner info: %w", err)
			}

			if mi.Beneficiary == mi.Owner && newAddr == mi.Owner {
				return fmt.Errorf("beneficiary %s already set to owner address", mi.Beneficiary)
			}

			if mi.PendingBeneficiaryTerm != nil {
				fmt.Println("WARNING: replacing Pending Beneficiary Term of:")
				fmt.Println("Beneficiary: ", mi.PendingBeneficiaryTerm.NewBeneficiary)
				fmt.Println("Quota:", mi.PendingBeneficiaryTerm.NewQuota)
				fmt.Println("Expiration Epoch:", mi.PendingBeneficiaryTerm.NewExpiration)

				if !cctx.Bool("overwrite-pending-change") {
					return fmt.Errorf("must pass --overwrite-pending-change to replace current pending beneficiary change. Please review CAREFULLY")
				}
			}

			if !cctx.Bool("really-do-it") {
				fmt.Println("Pass --really-do-it to actually execute this action. Review what you're about to approve CAREFULLY please")
				return nil
			}

			params := &miner.ChangeBeneficiaryParams{
				NewBeneficiary: newAddr,
				NewQuota:       abi.TokenAmount(quota),
				NewExpiration:  abi.ChainEpoch(expiration),
			}

			sp, err := actors.SerializeParams(params)
			if err != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Owner,
				To:     maddr,
				Method: builtin.MethodsMiner.ChangeBeneficiary,
				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push: %w", err)
			}

			fmt.Println("Propose Message CID:", smsg.Cid())

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
			if err != nil {
				return xerrors.Errorf("waiting for message to be included in block: %w", err)
			}

			// check it executed successfully
			if wait.Receipt.ExitCode.IsError() {
				return fmt.Errorf("propose beneficiary change failed")
			}

			updatedMinerInfo, err := api.StateMinerInfo(ctx, maddr, wait.TipSet)
			if err != nil {
				return xerrors.Errorf("getting miner info: %w", err)
			}

			if updatedMinerInfo.PendingBeneficiaryTerm == nil && updatedMinerInfo.Beneficiary == newAddr {
				fmt.Println("Beneficiary address successfully changed")
			} else {
				fmt.Println("Beneficiary address change awaiting additional confirmations")
			}

			return nil
		},
	}
}

func ActorConfirmChangeWorkerCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "confirm-change-worker",
		Usage:     "Confirm a worker address change",
		ArgsUsage: "[address]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
				Value: false,
			},
		},
		Action: func(cctx *cli.Context) error {
			if !cctx.Args().Present() {
				return fmt.Errorf("must pass address of new worker address")
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			na, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}

			newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
			if err != nil {
				return err
			}

			maddr, err := getActor(cctx)
			if err != nil {
				return err
			}

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			if mi.NewWorker.Empty() {
				return xerrors.Errorf("no worker key change proposed")
			} else if mi.NewWorker != newAddr {
				return xerrors.Errorf("worker key %s does not match current worker key proposal %s", newAddr, mi.NewWorker)
			}

			if head, err := api.ChainHead(ctx); err != nil {
				return xerrors.Errorf("failed to get the chain head: %w", err)
			} else if head.Height() < mi.WorkerChangeEpoch {
				return xerrors.Errorf("worker key change cannot be confirmed until %d, current height is %d", mi.WorkerChangeEpoch, head.Height())
			}

			if !cctx.Bool("really-do-it") {
				fmt.Println("Pass --really-do-it to actually execute this action")
				return nil
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Owner,
				To:     maddr,
				Method: builtin.MethodsMiner.ConfirmChangeWorkerAddress,
				Value:  big.Zero(),
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push: %w", err)
			}

			fmt.Println("Confirm Message CID:", smsg.Cid())

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
			if err != nil {
				return err
			}

			// check it executed successfully
			if wait.Receipt.ExitCode.IsError() {
				_, _ = fmt.Fprintln(cctx.App.Writer, "Worker change failed!")
				return err
			}

			mi, err = api.StateMinerInfo(ctx, maddr, wait.TipSet)
			if err != nil {
				return err
			}
			if mi.Worker != newAddr {
				return fmt.Errorf("confirmed worker address change not reflected on chain: expected '%s', found '%s'", newAddr, mi.Worker)
			}

			return nil
		},
	}
}

func ActorConfirmChangeBeneficiaryCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "confirm-change-beneficiary",
		Usage:     "Confirm a beneficiary address change",
		ArgsUsage: "[minerID]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "existing-beneficiary",
				Usage: "send confirmation from the existing beneficiary address",
			},
			&cli.BoolFlag{
				Name:  "new-beneficiary",
				Usage: "send confirmation from the new beneficiary address",
			},
		},
		Action: func(cctx *cli.Context) error {
			if cctx.NArg() != 1 {
				return lcli.IncorrectNumArgs(cctx)
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return xerrors.Errorf("getting fullnode api: %w", err)
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			maddr, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing beneficiary address: %w", err)
			}

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting miner info: %w", err)
			}

			if mi.PendingBeneficiaryTerm == nil {
				return fmt.Errorf("no pending beneficiary term found for miner %s", maddr)
			}

			if (cctx.IsSet("existing-beneficiary") && cctx.IsSet("new-beneficiary")) || (!cctx.IsSet("existing-beneficiary") && !cctx.IsSet("new-beneficiary")) {
				return lcli.ShowHelp(cctx, fmt.Errorf("must pass exactly one of --existing-beneficiary or --new-beneficiary"))
			}

			var fromAddr address.Address
			if cctx.IsSet("existing-beneficiary") {
				if mi.PendingBeneficiaryTerm.ApprovedByBeneficiary {
					return fmt.Errorf("beneficiary change already approved by current beneficiary")
				}
				fromAddr = mi.Beneficiary
			} else {
				if mi.PendingBeneficiaryTerm.ApprovedByNominee {
					return fmt.Errorf("beneficiary change already approved by new beneficiary")
				}
				fromAddr = mi.PendingBeneficiaryTerm.NewBeneficiary
			}

			fmt.Println("Confirming Pending Beneficiary Term of:")
			fmt.Println("Beneficiary: ", mi.PendingBeneficiaryTerm.NewBeneficiary)
			fmt.Println("Quota:", mi.PendingBeneficiaryTerm.NewQuota)
			fmt.Println("Expiration Epoch:", mi.PendingBeneficiaryTerm.NewExpiration)

			if !cctx.Bool("really-do-it") {
				fmt.Println("Pass --really-do-it to actually execute this action. Review what you're about to approve CAREFULLY please")
				return nil
			}

			params := &miner.ChangeBeneficiaryParams{
				NewBeneficiary: mi.PendingBeneficiaryTerm.NewBeneficiary,
				NewQuota:       mi.PendingBeneficiaryTerm.NewQuota,
				NewExpiration:  mi.PendingBeneficiaryTerm.NewExpiration,
			}

			sp, err := actors.SerializeParams(params)
			if err != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   fromAddr,
				To:     maddr,
				Method: builtin.MethodsMiner.ChangeBeneficiary,
				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push: %w", err)
			}

			fmt.Println("Confirm Message CID:", smsg.Cid())

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
			if err != nil {
				return xerrors.Errorf("waiting for message to be included in block: %w", err)
			}

			// check it executed successfully
			if wait.Receipt.ExitCode.IsError() {
				return fmt.Errorf("confirm beneficiary change failed with code %d", wait.Receipt.ExitCode)
			}

			updatedMinerInfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			if updatedMinerInfo.PendingBeneficiaryTerm == nil && updatedMinerInfo.Beneficiary == mi.PendingBeneficiaryTerm.NewBeneficiary {
				fmt.Println("Beneficiary address successfully changed")
			} else {
				fmt.Println("Beneficiary address change awaiting additional confirmations")
			}

			return nil
		},
	}
}

func ActorCompactAllocatedCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "compact-allocated",
		Usage: "compact allocated sectors bitfield",
		Flags: []cli.Flag{
			&cli.Uint64Flag{
				Name:  "mask-last-offset",
				Usage: "Mask sector IDs from 0 to 'highest_allocated - offset'",
			},
			&cli.Uint64Flag{
				Name:  "mask-upto-n",
				Usage: "Mask sector IDs from 0 to 'n'",
			},
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "Actually send transaction performing the action",
				Value: false,
			},
		},
		Action: func(cctx *cli.Context) error {
			if !cctx.Bool("really-do-it") {
				fmt.Println("Pass --really-do-it to actually execute this action")
				return nil
			}

			if !cctx.Args().Present() {
				return xerrors.Errorf("must pass address of new owner address")
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			maddr, err := getActor(cctx)
			if err != nil {
				return err
			}

			mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))

			mst, err := lminer.Load(store, mact)
			if err != nil {
				return err
			}

			allocs, err := mst.GetAllocatedSectors()
			if err != nil {
				return err
			}

			var maskBf bitfield.BitField

			{
				exclusiveFlags := []string{"mask-last-offset", "mask-upto-n"}
				hasFlag := false
				for _, f := range exclusiveFlags {
					if hasFlag && cctx.IsSet(f) {
						return xerrors.Errorf("more than one 'mask` flag set")
					}
					hasFlag = hasFlag || cctx.IsSet(f)
				}
			}
			switch {
			case cctx.IsSet("mask-last-offset"):
				last, err := allocs.Last()
				if err != nil {
					return err
				}

				m := cctx.Uint64("mask-last-offset")
				if last <= m+1 {
					return xerrors.Errorf("highest allocated sector lower than mask offset %d: %d", m+1, last)
				}
				// security to not brick a miner
				if last > 1<<60 {
					return xerrors.Errorf("very high last sector number, refusing to mask: %d", last)
				}

				maskBf, err = bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{
					Runs: []rlepluslazy.Run{{Val: true, Len: last - m}}})
				if err != nil {
					return xerrors.Errorf("forming bitfield: %w", err)
				}
			case cctx.IsSet("mask-upto-n"):
				n := cctx.Uint64("mask-upto-n")
				maskBf, err = bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{
					Runs: []rlepluslazy.Run{{Val: true, Len: n}}})
				if err != nil {
					return xerrors.Errorf("forming bitfield: %w", err)
				}
			default:
				return xerrors.Errorf("no 'mask' flags set")
			}

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			params := &miner.CompactSectorNumbersParams{
				MaskSectorNumbers: maskBf,
			}

			sp, err := actors.SerializeParams(params)
			if err != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Worker,
				To:     maddr,
				Method: builtin.MethodsMiner.CompactSectorNumbers,
				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push: %w", err)
			}

			fmt.Println("CompactSectorNumbers Message CID:", smsg.Cid())

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
			if err != nil {
				return err
			}

			// check it executed successfully
			if wait.Receipt.ExitCode.IsError() {
				fmt.Println("Sector Bitfield compaction failed")
				return err
			}

			return nil
		},
	}
}

func isController(mi lapi.MinerInfo, addr address.Address) bool {
	if addr == mi.Owner || addr == mi.Worker {
		return true
	}

	for _, ca := range mi.ControlAddresses {
		if addr == ca {
			return true
		}
	}

	return false
}

var ActorNewMinerCmd = &cli.Command{
	Name:  "new-miner",
	Usage: "Initializes a new miner actor",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "worker",
			Aliases: []string{"w"},
			Usage:   "worker key to use for new miner initialisation",
		},
		&cli.StringFlag{
			Name:    "owner",
			Aliases: []string{"o"},
			Usage:   "owner key to use for new miner initialisation",
		},
		&cli.StringFlag{
			Name:    "from",
			Aliases: []string{"f"},
			Usage:   "address to send actor(miner) creation message from",
		},
		&cli.StringFlag{
			Name:  "sector-size",
			Usage: "specify sector size to use for new miner initialisation",
		},
		&cli.IntFlag{
			Name:  "confidence",
			Usage: "number of block confirmations to wait for",
			Value: int(buildconstants.MessageConfidence),
		},
		&cli.Float64Flag{
			Name:  "deposit-margin-factor",
			Usage: "Multiplier (>=1.0) to scale the suggested deposit for on-chain variance (e.g. 1.01 adds 1%)",
			Value: 1.01,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		full, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return xerrors.Errorf("connecting to full node: %w", err)
		}
		defer closer()

		var owner address.Address
		if cctx.String("owner") == "" {
			return xerrors.Errorf("must provide an owner address")
		}
		owner, err = address.NewFromString(cctx.String("owner"))

		if err != nil {
			return err
		}

		worker := owner
		if cctx.String("worker") != "" {
			worker, err = address.NewFromString(cctx.String("worker"))
			if err != nil {
				return xerrors.Errorf("could not parse worker address: %w", err)
			}
		}

		sender := owner
		if fromstr := cctx.String("from"); fromstr != "" {
			faddr, err := address.NewFromString(fromstr)
			if err != nil {
				return xerrors.Errorf("could not parse from address: %w", err)
			}
			sender = faddr
		}

		if !cctx.IsSet("sector-size") {
			return xerrors.Errorf("must define sector size")
		}

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		ssize := abi.SectorSize(sectorSizeInt)

		_, err = createminer.CreateStorageMiner(ctx, full, owner, worker, sender, ssize, cctx.Uint64("confidence"), cctx.Float64("deposit-margin-factor"))
		if err != nil {
			return err
		}
		return nil
	},
}

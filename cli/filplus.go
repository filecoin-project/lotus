package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/manifoldco/promptui"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	verifregtypes13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	verifregtypes8 "github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	datacap2 "github.com/filecoin-project/go-state-types/builtin/v9/datacap"
	verifregtypes9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var FilplusCmd = &cli.Command{
	Name:  "filplus",
	Usage: "Interact with the verified registry actor used by Filplus",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		filplusVerifyClientCmd,
		filplusListNotariesCmd,
		filplusListClientsCmd,
		filplusCheckClientCmd,
		filplusCheckNotaryCmd,
		filplusSignRemoveDataCapProposal,
		filplusListAllocationsCmd,
		filplusListClaimsCmd,
		filplusRemoveExpiredAllocationsCmd,
		filplusRemoveExpiredClaimsCmd,
		filplusExtendClaimCmd,
	},
}

var filplusVerifyClientCmd = &cli.Command{
	Name:      "grant-datacap",
	Usage:     "give allowance to the specified verified client address",
	ArgsUsage: "[clientAddress datacap]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "from",
			Usage:    "specify your notary address to send the message from",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		froms := cctx.String("from")
		if froms == "" {
			return fmt.Errorf("must specify from address with --from")
		}

		fromk, err := address.NewFromString(froms)
		if err != nil {
			return err
		}

		if cctx.NArg() != 2 {
			return IncorrectNumArgs(cctx)
		}

		target, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		allowance, err := types.BigFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		found, dcap, err := checkNotary(ctx, api, fromk)
		if err != nil {
			return err
		}

		if !found {
			return xerrors.New("sender address must be a notary")
		}

		if dcap.Cmp(allowance.Int) < 0 {
			return xerrors.Errorf("cannot allot more allowance than notary data cap: %s < %s", dcap, allowance)
		}

		// TODO: This should be abstracted over actor versions
		params, err := actors.SerializeParams(&verifregtypes9.AddVerifiedClientParams{Address: target, Allowance: allowance})
		if err != nil {
			return err
		}

		msg := &types.Message{
			To:     verifreg.Address,
			From:   fromk,
			Method: verifreg.Methods.AddVerifiedClient,
			Params: params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return err
		}

		fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

		mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("failed to add verified client: %d", mwait.Receipt.ExitCode)
		}

		return nil
	},
}

var filplusListNotariesCmd = &cli.Command{
	Name:  "list-notaries",
	Usage: "list all notaries",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 0 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
		if err != nil {
			return err
		}

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		st, err := verifreg.Load(store, act)
		if err != nil {
			return err
		}
		return st.ForEachVerifier(func(addr address.Address, dcap abi.StoragePower) error {
			_, err := fmt.Printf("%s: %s\n", addr, dcap)
			return err
		})
	},
}

var filplusListClientsCmd = &cli.Command{
	Name:  "list-clients",
	Usage: "list all verified clients",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 0 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		av, err := actorstypes.VersionForNetwork(nv)
		if err != nil {
			return err
		}

		if av <= 8 {
			act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
			if err != nil {
				return err
			}

			st, err := verifreg.Load(store, act)
			if err != nil {
				return err
			}
			return st.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
				_, err := fmt.Printf("%s: %s\n", addr, dcap)
				return err
			})
		}
		act, err := api.StateGetActor(ctx, datacap.Address, types.EmptyTSK)
		if err != nil {
			return err
		}

		st, err := datacap.Load(store, act)
		if err != nil {
			return err
		}
		return st.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
			_, err := fmt.Printf("%s: %s\n", addr, dcap)
			return err
		})
	},
}

var filplusListAllocationsCmd = &cli.Command{
	Name:      "list-allocations",
	Usage:     "List allocations available in verified registry actor or made by a client if specified",
	ArgsUsage: "clientAddress",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "expired",
			Usage: "list only expired allocations",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "output results in json format",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() > 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		writeOut := func(tsHeight abi.ChainEpoch, allocations map[verifreg.AllocationId]verifreg.Allocation, json, expired bool) error {
			// Map Keys. Corresponds to the standard tablewriter output
			allocationID := "AllocationID"
			client := "Client"
			provider := "Miner"
			pieceCid := "PieceCid"
			pieceSize := "PieceSize"
			tMin := "TermMin"
			tMax := "TermMax"
			expr := "Expiration"

			// One-to-one mapping between tablewriter keys and JSON keys
			tableKeysToJsonKeys := map[string]string{
				allocationID: strings.ToLower(allocationID),
				client:       strings.ToLower(client),
				provider:     strings.ToLower(provider),
				pieceCid:     strings.ToLower(pieceCid),
				pieceSize:    strings.ToLower(pieceSize),
				tMin:         strings.ToLower(tMin),
				tMax:         strings.ToLower(tMax),
				expr:         strings.ToLower(expr),
			}

			var allocs []map[string]interface{}

			for key, val := range allocations {
				if tsHeight > val.Expiration || !expired {
					alloc := map[string]interface{}{
						allocationID: key,
						client:       val.Client,
						provider:     val.Provider,
						pieceCid:     val.Data,
						pieceSize:    val.Size,
						tMin:         val.TermMin,
						tMax:         val.TermMax,
						expr:         val.Expiration,
					}
					allocs = append(allocs, alloc)
				}
			}

			if json {
				// get a new list of allocations with json keys instead of tablewriter keys
				var jsonAllocs []map[string]interface{}
				for _, alloc := range allocs {
					jsonAlloc := make(map[string]interface{})
					for k, v := range alloc {
						jsonAlloc[tableKeysToJsonKeys[k]] = v
					}
					jsonAllocs = append(jsonAllocs, jsonAlloc)
				}
				// then return this!
				return PrintJson(jsonAllocs)
			}
			// Init the tablewriter's columns
			tw := tablewriter.New(
				tablewriter.Col(allocationID),
				tablewriter.Col(client),
				tablewriter.Col(provider),
				tablewriter.Col(pieceCid),
				tablewriter.Col(pieceSize),
				tablewriter.Col(tMin),
				tablewriter.Col(tMax),
				tablewriter.Col(expr))
			// populate it with content
			for _, alloc := range allocs {
				tw.Write(alloc)
			}
			// return the corresponding string
			return tw.Flush(os.Stdout)
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))

		verifregActor, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
		if err != nil {
			return err
		}

		verifregState, err := verifreg.Load(store, verifregActor)
		if err != nil {
			return err
		}

		ts, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		if cctx.NArg() == 1 {
			clientAddr, err := address.NewFromString(cctx.Args().Get(0))
			if err != nil {
				return err
			}

			clientIdAddr, err := api.StateLookupID(ctx, clientAddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			allocationsMap, err := verifregState.GetAllocations(clientIdAddr)
			if err != nil {
				return err
			}

			return writeOut(ts.Height(), allocationsMap, cctx.Bool("json"), cctx.Bool("expired"))
		}

		allocationsMap, err := verifregState.GetAllAllocations()
		if err != nil {
			return err
		}

		return writeOut(ts.Height(), allocationsMap, cctx.Bool("json"), cctx.Bool("expired"))

	},
}

var filplusListClaimsCmd = &cli.Command{
	Name:      "list-claims",
	Usage:     "List claims available in verified registry actor or made by provider if specified",
	ArgsUsage: "providerAddress",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "expired",
			Usage: "list only expired claims",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "output results in json format",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() > 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		writeOut := func(tsHeight abi.ChainEpoch, claims map[verifreg.ClaimId]verifreg.Claim, json, expired bool) error {
			// Map Keys. Corresponds to the standard tablewriter output
			claimID := "ClaimID"
			provider := "Provider"
			client := "Client"
			data := "Data"
			size := "Size"
			tMin := "TermMin"
			tMax := "TermMax"
			tStart := "TermStart"
			sector := "Sector"

			// One-to-one mapping between tablewriter keys and JSON keys
			tableKeysToJsonKeys := map[string]string{
				claimID:  strings.ToLower(claimID),
				provider: strings.ToLower(provider),
				client:   strings.ToLower(client),
				data:     strings.ToLower(data),
				size:     strings.ToLower(size),
				tMin:     strings.ToLower(tMin),
				tMax:     strings.ToLower(tMax),
				tStart:   strings.ToLower(tStart),
				sector:   strings.ToLower(sector),
			}

			var claimList []map[string]interface{}

			for key, val := range claims {
				if tsHeight > val.TermStart+val.TermMax || !expired {
					claim := map[string]interface{}{
						claimID:  key,
						provider: val.Provider,
						client:   val.Client,
						data:     val.Data,
						size:     val.Size,
						tMin:     val.TermMin,
						tMax:     val.TermMax,
						tStart:   val.TermStart,
						sector:   val.Sector,
					}
					claimList = append(claimList, claim)
				}
			}

			if json {
				// get a new list of claims with json keys instead of tablewriter keys
				var jsonClaims []map[string]interface{}
				for _, claim := range claimList {
					jsonClaim := make(map[string]interface{})
					for k, v := range claim {
						jsonClaim[tableKeysToJsonKeys[k]] = v
					}
					jsonClaims = append(jsonClaims, jsonClaim)
				}
				// then return this!
				return PrintJson(jsonClaims)
			}
			// Init the tablewriter's columns
			tw := tablewriter.New(
				tablewriter.Col(claimID),
				tablewriter.Col(client),
				tablewriter.Col(provider),
				tablewriter.Col(data),
				tablewriter.Col(size),
				tablewriter.Col(tMin),
				tablewriter.Col(tMax),
				tablewriter.Col(tStart),
				tablewriter.Col(sector))
			// populate it with content
			for _, alloc := range claimList {

				tw.Write(alloc)
			}
			// return the corresponding string
			return tw.Flush(os.Stdout)
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))

		verifregActor, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
		if err != nil {
			return err
		}

		verifregState, err := verifreg.Load(store, verifregActor)
		if err != nil {
			return err
		}

		ts, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		if cctx.NArg() == 1 {
			providerAddr, err := address.NewFromString(cctx.Args().Get(0))
			if err != nil {
				return err
			}

			providerIdAddr, err := api.StateLookupID(ctx, providerAddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			claimsMap, err := verifregState.GetClaims(providerIdAddr)
			if err != nil {
				return err
			}

			return writeOut(ts.Height(), claimsMap, cctx.Bool("json"), cctx.Bool("expired"))
		}

		claimsMap, err := verifregState.GetAllClaims()
		if err != nil {
			return err
		}

		return writeOut(ts.Height(), claimsMap, cctx.Bool("json"), cctx.Bool("expired"))
	},
}

var filplusRemoveExpiredAllocationsCmd = &cli.Command{
	Name:      "remove-expired-allocations",
	Usage:     "remove expired allocations (if no allocations are specified all eligible allocations are removed)",
	ArgsUsage: "clientAddress Optional[...allocationId]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send the message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		args := cctx.Args().Slice()

		clientAddr, err := address.NewFromString(args[0])
		if err != nil {
			return err
		}

		clientIdAddr, err := api.StateLookupID(ctx, clientAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		clientId, err := address.IDFromAddress(clientIdAddr)
		if err != nil {
			return err
		}

		fromAddr := clientIdAddr
		if from := cctx.String("from"); from != "" {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			fromAddr = addr
		}

		allocationIDs := make([]verifregtypes9.AllocationId, len(args)-1)
		for i, allocationString := range args[1:] {
			id, err := strconv.ParseUint(allocationString, 10, 64)
			if err != nil {
				return err
			}
			allocationIDs[i] = verifregtypes9.AllocationId(id)
		}

		params, err := actors.SerializeParams(&verifregtypes9.RemoveExpiredAllocationsParams{
			Client:        abi.ActorID(clientId),
			AllocationIds: allocationIDs,
		})
		if err != nil {
			return err
		}

		msg := &types.Message{
			To:     verifreg.Address,
			From:   fromAddr,
			Method: verifreg.Methods.RemoveExpiredAllocations,
			Params: params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return err
		}

		fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

		mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("failed to remove expired allocations: %d", mwait.Receipt.ExitCode)
		}

		return nil
	},
}

var filplusRemoveExpiredClaimsCmd = &cli.Command{
	Name:      "remove-expired-claims",
	Usage:     "remove expired claims (if no claims are specified all eligible claims are removed)",
	ArgsUsage: "providerAddress Optional[...claimId]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send the message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		args := cctx.Args().Slice()

		providerAddr, err := address.NewFromString(args[0])
		if err != nil {
			return err
		}

		providerIdAddr, err := api.StateLookupID(ctx, providerAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		providerId, err := address.IDFromAddress(providerIdAddr)
		if err != nil {
			return err
		}

		fromAddr := providerIdAddr
		if from := cctx.String("from"); from != "" {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			fromAddr = addr
		}

		claimIDs := make([]verifregtypes9.ClaimId, len(args)-1)
		for i, claimStr := range args[1:] {
			id, err := strconv.ParseUint(claimStr, 10, 64)
			if err != nil {
				return err
			}
			claimIDs[i] = verifregtypes9.ClaimId(id)
		}

		params, err := actors.SerializeParams(&verifregtypes9.RemoveExpiredClaimsParams{
			Provider: abi.ActorID(providerId),
			ClaimIds: claimIDs,
		})
		if err != nil {
			return err
		}

		msg := &types.Message{
			To:     verifreg.Address,
			From:   fromAddr,
			Method: verifreg.Methods.RemoveExpiredClaims,
			Params: params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return err
		}

		fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

		mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("failed to remove expired allocations: %d", mwait.Receipt.ExitCode)
		}

		return nil
	},
}

var filplusCheckClientCmd = &cli.Command{
	Name:      "check-client-datacap",
	Usage:     "check verified client remaining bytes",
	ArgsUsage: "clientAddress",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("must specify client address to check")
		}

		caddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		dcap, err := api.StateVerifiedClientStatus(ctx, caddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		if dcap == nil {
			return xerrors.Errorf("client %s is not a verified client", caddr)
		}

		fmt.Println(*dcap)

		return nil
	},
}

var filplusCheckNotaryCmd = &cli.Command{
	Name:      "check-notary-datacap",
	Usage:     "check a notary's remaining bytes",
	ArgsUsage: "notaryAddress",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("must specify notary address to check")
		}

		vaddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		found, dcap, err := checkNotary(ctx, api, vaddr)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("not found")
		}

		fmt.Println(dcap)

		return nil
	},
}

func checkNotary(ctx context.Context, api v0api.FullNode, vaddr address.Address) (bool, abi.StoragePower, error) {
	vid, err := api.StateLookupID(ctx, vaddr, types.EmptyTSK)
	if err != nil {
		return false, big.Zero(), err
	}

	act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
	if err != nil {
		return false, big.Zero(), err
	}

	apibs := blockstore.NewAPIBlockstore(api)
	store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

	st, err := verifreg.Load(store, act)
	if err != nil {
		return false, big.Zero(), err
	}

	return st.VerifierDataCap(vid)
}

var filplusSignRemoveDataCapProposal = &cli.Command{
	Name:      "sign-remove-data-cap-proposal",
	Usage:     "allows a notary to sign a Remove Data Cap Proposal",
	ArgsUsage: "[verifierAddress clientAddress allowanceToRemove]",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:     "id",
			Usage:    "specify the RemoveDataCapProposal ID (will look up on chain if unspecified)",
			Required: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("failed to get full node api: %w", err)
		}
		defer closer()
		ctx := ReqContext(cctx)

		act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to get verifreg actor: %w", err)
		}

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		st, err := verifreg.Load(store, act)
		if err != nil {
			return xerrors.Errorf("failed to load verified registry state: %w", err)
		}

		verifier, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}
		verifierIdAddr, err := api.StateLookupID(ctx, verifier, types.EmptyTSK)
		if err != nil {
			return err
		}

		client, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}
		clientIdAddr, err := api.StateLookupID(ctx, client, types.EmptyTSK)
		if err != nil {
			return err
		}

		allowanceToRemove, err := types.BigFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		dataCap, err := api.StateVerifiedClientStatus(ctx, clientIdAddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to find verified client data cap: %w", err)
		}
		if dataCap.LessThanEqual(big.Zero()) {
			return xerrors.Errorf("client data cap %s is less than amount requested to be removed %s", dataCap.String(), allowanceToRemove.String())
		}

		found, _, err := checkNotary(ctx, api, verifier)
		if err != nil {
			return xerrors.Errorf("failed to check notary status: %w", err)
		}

		if !found {
			return xerrors.New("verifier address must be a notary")
		}

		id := cctx.Uint64("id")
		if id == 0 {
			_, id, err = st.RemoveDataCapProposalID(verifierIdAddr, clientIdAddr)
			if err != nil {
				return xerrors.Errorf("failed find remove data cap proposal id: %w", err)
			}
		}

		nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to get network version: %w", err)
		}

		paramBuf := new(bytes.Buffer)
		paramBuf.WriteString(verifregtypes9.SignatureDomainSeparation_RemoveDataCap)
		if nv <= network.Version16 {
			params := verifregtypes8.RemoveDataCapProposal{
				RemovalProposalID: id,
				DataCapAmount:     allowanceToRemove,
				VerifiedClient:    clientIdAddr,
			}

			err = params.MarshalCBOR(paramBuf)
		} else {
			params := verifregtypes9.RemoveDataCapProposal{
				RemovalProposalID: verifregtypes9.RmDcProposalID{ProposalID: id},
				DataCapAmount:     allowanceToRemove,
				VerifiedClient:    clientIdAddr,
			}

			err = params.MarshalCBOR(paramBuf)
		}
		if err != nil {
			return xerrors.Errorf("failed to marshall paramBuf: %w", err)
		}

		sig, err := api.WalletSign(ctx, verifier, paramBuf.Bytes())
		if err != nil {
			return xerrors.Errorf("failed to sign message: %w", err)
		}

		sigBytes := append([]byte{byte(sig.Type)}, sig.Data...)

		fmt.Println(hex.EncodeToString(sigBytes))

		return nil
	},
}

var filplusExtendClaimCmd = &cli.Command{
	Name:  "extend-claim",
	Usage: "extends claim expiration (TermMax)",
	UsageText: `Extends claim expiration (TermMax).
If the client is original client then claim can be extended to maximum 5 years and no Datacap is required.
If the client id different then claim can be extended up to maximum 5 years from now and Datacap is required.
`,
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:    "term-max",
			Usage:   "The maximum period for which a provider can earn quality-adjusted power for the piece (epochs). Default is 5 years.",
			Aliases: []string{"tmax"},
			Value:   verifregtypes13.MaximumVerifiedAllocationTerm,
		},
		&cli.StringFlag{
			Name:     "client",
			Usage:    "the client address that will used to send the message",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "all",
			Usage: "automatically extend TermMax of all claims for specified miner[s] to --term-max (default: 5 years from claim start epoch)",
		},
		&cli.StringSliceFlag{
			Name:    "miner",
			Usage:   "storage provider address[es]",
			Aliases: []string{"m", "provider", "p"},
		},
		&cli.BoolFlag{
			Name:    "assume-yes",
			Usage:   "automatic yes to prompts; assume 'yes' as answer to all prompts and run non-interactively",
			Aliases: []string{"y", "yes"},
		},
		&cli.IntFlag{
			Name:  "confidence",
			Usage: "number of block confirmations to wait for",
			Value: int(buildconstants.MessageConfidence),
		},
		&cli.IntFlag{
			Name:  "batch-size",
			Usage: "number of extend requests per batch. If set incorrectly, this will lead to out of gas error",
			Value: 500,
		},
	},
	ArgsUsage: "<claim1> <claim2> ... or <miner1=claim1> <miner2=claims2> ...",
	Action: func(cctx *cli.Context) error {

		miners := cctx.StringSlice("miner")
		all := cctx.Bool("all")
		client := cctx.String("client")
		tmax := cctx.Int64("term-max")

		// No miner IDs and no arguments
		if len(miners) == 0 && cctx.Args().Len() == 0 {
			return xerrors.Errorf("must specify at least one miner ID or argument[s]")
		}

		// Single Miner with no claimID and no --all flag
		if len(miners) == 1 && cctx.Args().Len() == 0 && !all {
			return xerrors.Errorf("must specify either --all flag or claim IDs to extend in argument")
		}

		// Multiple Miner with claimIDs
		if len(miners) > 1 && cctx.Args().Len() > 0 {
			return xerrors.Errorf("either specify multiple miner IDs or multiple arguments")
		}

		// Multiple Miner with no claimID and no --all flag
		if len(miners) > 1 && cctx.Args().Len() == 0 && !all {
			return xerrors.Errorf("must specify --all flag with multiple miner IDs")
		}

		// Tmax can't be more than policy max
		if tmax > verifregtypes13.MaximumVerifiedAllocationTerm {
			return xerrors.Errorf("specified term-max %d is larger than %d maximum allowed by verified regirty actor policy", tmax, verifregtypes13.MaximumVerifiedAllocationTerm)
		}

		api, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return xerrors.Errorf("failed to get full node api: %s", err)
		}
		defer closer()
		ctx := ReqContext(cctx)

		clientAddr, err := address.NewFromString(client)
		if err != nil {
			return err
		}

		claimMap := make(map[verifregtypes13.ClaimId]ProvInfo)

		// If no miners and arguments are present
		if len(miners) == 0 && cctx.Args().Len() > 0 {
			for _, arg := range cctx.Args().Slice() {
				detail := strings.Split(arg, "=")
				if len(detail) > 2 {
					return xerrors.Errorf("incorrect argument format: %s", detail)
				}

				n, err := strconv.ParseInt(detail[1], 10, 64)
				if err != nil {
					return xerrors.Errorf("failed to parse the claim ID for %s for argument %s: %s", detail[0], detail, err)
				}

				maddr, err := address.NewFromString(detail[0])
				if err != nil {
					return err
				}

				// Verify that minerID exists
				_, err = api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
				if err != nil {
					return err
				}

				mid, err := address.IDFromAddress(maddr)
				if err != nil {
					return err
				}

				pi := ProvInfo{
					Addr: maddr,
					ID:   abi.ActorID(mid),
				}

				claimMap[verifregtypes13.ClaimId(n)] = pi
			}
		}

		// If 1 miner ID and multiple arguments
		if len(miners) == 1 && cctx.Args().Len() > 0 && !all {
			for _, arg := range cctx.Args().Slice() {
				detail := strings.Split(arg, "=")
				if len(detail) > 1 {
					return xerrors.Errorf("incorrect argument format %s. Must provide only claim IDs with single miner ID", detail)
				}

				n, err := strconv.ParseInt(detail[0], 10, 64)
				if err != nil {
					return xerrors.Errorf("failed to parse the claim ID for %s for argument %s: %s", detail[0], detail, err)
				}

				claimMap[verifregtypes13.ClaimId(n)] = ProvInfo{}
			}
		}

		msgs, err := CreateExtendClaimMsg(ctx, api, claimMap, miners, clientAddr, abi.ChainEpoch(tmax), all, cctx.Bool("assume-yes"), cctx.Int("batch-size"))
		if err != nil {
			return err
		}

		// If not msgs are found then no claims can be extended
		if msgs == nil {
			fmt.Println("No eligible claims to extend")
			return nil
		}

		// MpoolBatchPushMessage method will take care of gas estimation and funds check
		smsgs, err := api.MpoolBatchPushMessage(ctx, msgs, nil)
		if err != nil {
			return err
		}

		// wait for msgs to get mined into a block
		eg := errgroup.Group{}
		eg.SetLimit(10)
		for _, msg := range smsgs {
			msg := msg
			eg.Go(func() error {
				wait, err := api.StateWaitMsg(ctx, msg.Cid(), uint64(cctx.Int("confidence")), 2000, true)
				if err != nil {
					return xerrors.Errorf("timeout waiting for message to land on chain %s", wait.Message)

				}

				if wait.Receipt.ExitCode.IsError() {
					return xerrors.Errorf("failed to execute message %s: %s", wait.Message, wait.Receipt.ExitCode)
				}
				return nil
			})
		}
		return eg.Wait()
	},
}

type ProvInfo struct {
	Addr address.Address
	ID   abi.ActorID
}

// CreateExtendClaimMsg creates extend message[s] based on the following conditions
// 1. Extend all claims for a miner ID
// 2. Extend all claims for multiple miner IDs
// 3. Extend specified claims for a miner ID
// 4. Extend specific claims for specific miner ID
// 5. Extend all claims for a miner ID with different client address (2 messages)
// 6. Extend all claims for multiple miner IDs with different client address (2 messages)
// 7. Extend specified claims for a miner ID with different client address (2 messages)
// 8. Extend specific claims for specific miner ID with different client address (2 messages)
func CreateExtendClaimMsg(ctx context.Context, api api.FullNode, pcm map[verifregtypes13.ClaimId]ProvInfo, miners []string, wallet address.Address, tmax abi.ChainEpoch, all, assumeYes bool, batchSize int) ([]*types.Message, error) {

	ac, err := api.StateLookupID(ctx, wallet, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	w, err := address.IDFromAddress(ac)
	if err != nil {
		return nil, xerrors.Errorf("converting wallet address to ID: %w", err)
	}

	wid := abi.ActorID(w)

	head, err := api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	var terms []verifregtypes13.ClaimTerm
	newClaims := make(map[verifregtypes13.ClaimExtensionRequest]big.Int)
	rDataCap := big.NewInt(0)

	// If --all is set
	if all {
		for _, id := range miners {
			maddr, err := address.NewFromString(id)
			if err != nil {
				return nil, xerrors.Errorf("parsing miner %s: %s", id, err)
			}
			mid, err := address.IDFromAddress(maddr)
			if err != nil {
				return nil, xerrors.Errorf("converting miner address to miner ID: %s", err)
			}
			claims, err := api.StateGetClaims(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return nil, xerrors.Errorf("getting claims for miner %s: %s", maddr, err)
			}
			for claimID, claim := range claims {
				claimID := claimID
				claim := claim
				// If the client is not the original client - burn datacap
				if claim.Client != wid {
					// The new duration should be greater than the original deal duration and claim should not already be expired
					if head.Height()+tmax-claim.TermStart > claim.TermMax-claim.TermStart && claim.TermStart+claim.TermMax > head.Height() {
						req := verifregtypes13.ClaimExtensionRequest{
							Claim:    verifregtypes13.ClaimId(claimID),
							Provider: abi.ActorID(mid),
							TermMax:  head.Height() + tmax - claim.TermStart,
						}
						newClaims[req] = big.NewInt(int64(claim.Size))
						rDataCap.Add(big.NewInt(int64(claim.Size)).Int, rDataCap.Int)
					}
					// If new duration shorter than the original duration then do nothing
					continue
				}
				// For original client, compare duration(TermMax) and claim should not already be expired
				if claim.TermMax < tmax && claim.TermStart+claim.TermMax > head.Height() {
					terms = append(terms, verifregtypes13.ClaimTerm{
						ClaimId:  verifregtypes13.ClaimId(claimID),
						TermMax:  tmax,
						Provider: abi.ActorID(mid),
					})
				}
			}
		}
	}

	// Single miner and specific claims
	if len(miners) == 1 && len(pcm) > 0 {
		maddr, err := address.NewFromString(miners[0])
		if err != nil {
			return nil, xerrors.Errorf("parsing miner %s: %s", miners[0], err)
		}
		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return nil, xerrors.Errorf("converting miner address to miner ID: %s", err)
		}
		claims, err := api.StateGetClaims(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("getting claims for miner %s: %s", maddr, err)
		}

		for claimID := range pcm {
			claimID := claimID
			claim, ok := claims[verifregtypes9.ClaimId(claimID)]
			if !ok {
				return nil, xerrors.Errorf("claim %d not found for provider %s", claimID, miners[0])
			}
			// If the client is not the original client - burn datacap
			if claim.Client != wid {
				// The new duration should be greater than the original deal duration and claim should not already be expired
				if head.Height()+tmax-claim.TermStart > claim.TermMax-claim.TermStart && claim.TermStart+claim.TermMax > head.Height() {
					req := verifregtypes13.ClaimExtensionRequest{
						Claim:    claimID,
						Provider: abi.ActorID(mid),
						TermMax:  head.Height() + tmax - claim.TermStart,
					}
					newClaims[req] = big.NewInt(int64(claim.Size))
					rDataCap.Add(big.NewInt(int64(claim.Size)).Int, rDataCap.Int)
				}
				// If new duration shorter than the original duration then do nothing
				continue
			}
			// For original client, compare duration(TermMax) and claim should not already be expired
			if claim.TermMax < tmax && claim.TermStart+claim.TermMax > head.Height() {
				terms = append(terms, verifregtypes13.ClaimTerm{
					ClaimId:  claimID,
					TermMax:  tmax,
					Provider: abi.ActorID(mid),
				})
			}
		}
	}

	if len(miners) == 0 && len(pcm) > 0 {
		for claimID, prov := range pcm {
			prov := prov
			claimID := claimID
			claim, err := api.StateGetClaim(ctx, prov.Addr, verifregtypes9.ClaimId(claimID), types.EmptyTSK)
			if err != nil {
				return nil, xerrors.Errorf("could not load the claim %d: %s", claimID, err)
			}
			if claim == nil {
				return nil, xerrors.Errorf("claim %d not found in the actor state", claimID)
			}
			// If the client is not the original client - burn datacap
			if claim.Client != wid {
				// The new duration should be greater than the original deal duration and claim should not already be expired
				if head.Height()+tmax-claim.TermStart > claim.TermMax-claim.TermStart && claim.TermStart+claim.TermMax > head.Height() {
					req := verifregtypes13.ClaimExtensionRequest{
						Claim:    claimID,
						Provider: prov.ID,
						TermMax:  head.Height() + tmax - claim.TermStart,
					}
					newClaims[req] = big.NewInt(int64(claim.Size))
					rDataCap.Add(big.NewInt(int64(claim.Size)).Int, rDataCap.Int)
				}
				// If new duration shorter than the original duration then do nothing
				continue
			}
			// For original client, compare duration(TermMax) and claim should not already be expired
			if claim.TermMax < tmax && claim.TermStart+claim.TermMax > head.Height() {
				terms = append(terms, verifregtypes13.ClaimTerm{
					ClaimId:  claimID,
					TermMax:  tmax,
					Provider: prov.ID,
				})
			}
		}
	}

	var msgs []*types.Message

	if len(terms) > 0 {
		// Batch in 500 to avoid running out of gas
		for i := 0; i < len(terms); i += batchSize {
			batchEnd := i + batchSize
			if batchEnd > len(terms) {
				batchEnd = len(terms)
			}

			batch := terms[i:batchEnd]

			params, err := actors.SerializeParams(&verifregtypes13.ExtendClaimTermsParams{
				Terms: batch,
			})
			if err != nil {
				return nil, xerrors.Errorf("failed to searialise the parameters: %s", err)
			}
			oclaimMsg := &types.Message{
				To:     verifreg.Address,
				From:   wallet,
				Method: verifreg.Methods.ExtendClaimTerms,
				Params: params,
			}
			msgs = append(msgs, oclaimMsg)
		}
	}

	if len(newClaims) > 0 {
		if !assumeYes {
			out := fmt.Sprintf("Some of the specified allocation have a different client address and will require %d Datacap to extend. Proceed? Yes [Y/y] / No [N/n], Ctrl+C (^C) to exit", rDataCap.Int)
			validate := func(input string) error {
				if strings.EqualFold(input, "y") || strings.EqualFold(input, "yes") {
					return nil
				}
				if strings.EqualFold(input, "n") || strings.EqualFold(input, "no") {
					return nil
				}
				return errors.New("incorrect input")
			}

			templates := &promptui.PromptTemplates{
				Prompt:  "{{ . }} ",
				Valid:   "{{ . | green }} ",
				Invalid: "{{ . | red }} ",
				Success: "{{ . | cyan | bold }} ",
			}

			prompt := promptui.Prompt{
				Label:     out,
				Templates: templates,
				Validate:  validate,
			}

			input, err := prompt.Run()
			if err != nil {
				return nil, err
			}
			if strings.Contains(strings.ToLower(input), "n") {
				fmt.Println("Dropping the extension for claims that require Datacap")
				return msgs, nil
			}
		}

		// Get datacap balance
		aDataCap, err := api.StateVerifiedClientStatus(ctx, wallet, types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		if aDataCap == nil {
			return nil, xerrors.Errorf("wallet %s does not have any datacap", wallet)
		}

		// Check that we have enough data cap to make the allocation
		if rDataCap.GreaterThan(big.NewInt(aDataCap.Int64())) {
			return nil, xerrors.Errorf("requested datacap %s is greater then the available datacap %s", rDataCap, aDataCap)
		}

		// Create a map of just keys, so we can easily batch based on the numeric keys
		keys := make([]verifregtypes13.ClaimExtensionRequest, 0, len(newClaims))
		for k := range newClaims {
			keys = append(keys, k)
		}

		// Batch in 500 to avoid running out of gas
		for i := 0; i < len(keys); i += batchSize {
			batchEnd := i + batchSize
			if batchEnd > len(newClaims) {
				batchEnd = len(newClaims)
			}

			batch := keys[i:batchEnd]

			// Calculate Datacap for this batch
			dcap := big.NewInt(0)
			for _, k := range batch {
				dc := newClaims[k]
				dcap.Add(dcap.Int, dc.Int)
			}

			ncparams, err := actors.SerializeParams(&verifregtypes13.AllocationRequests{
				Extensions: batch,
			})
			if err != nil {
				return nil, xerrors.Errorf("failed to searialise the parameters: %s", err)
			}

			transferParams, err := actors.SerializeParams(&datacap2.TransferParams{
				To:           builtin.VerifiedRegistryActorAddr,
				Amount:       big.Mul(dcap, builtin.TokenPrecision),
				OperatorData: ncparams,
			})

			if err != nil {
				return nil, xerrors.Errorf("failed to serialize transfer parameters: %s", err)
			}

			nclaimMsg := &types.Message{
				To:     builtin.DatacapActorAddr,
				From:   wallet,
				Method: datacap.Methods.TransferExported,
				Params: transferParams,
				Value:  big.Zero(),
			}
			msgs = append(msgs, nclaimMsg)
		}
	}

	return msgs, nil
}

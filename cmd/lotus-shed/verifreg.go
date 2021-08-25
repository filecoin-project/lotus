package main

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	verifreg2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var verifRegCmd = &cli.Command{
	Name:  "verifreg",
	Usage: "Interact with the verified registry actor",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		verifRegAddVerifierFromMsigCmd,
		verifRegAddVerifierFromAccountCmd,
		verifRegVerifyClientCmd,
		verifRegListVerifiersCmd,
		verifRegListClientsCmd,
		verifRegCheckClientCmd,
		verifRegCheckVerifierCmd,
	},
}

var verifRegAddVerifierFromMsigCmd = &cli.Command{
	Name:      "add-verifier",
	Usage:     "make a given account a verifier",
	ArgsUsage: "<message sender> <new verifier> <allowance>",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return fmt.Errorf("must specify three arguments: sender, verifier, and allowance")
		}

		sender, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		verifier, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		allowance, err := types.BigFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		// TODO: ActorUpgrade: Abstract
		params, err := actors.SerializeParams(&verifreg2.AddVerifierParams{Address: verifier, Allowance: allowance})
		if err != nil {
			return err
		}

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		vrk, err := api.StateVerifiedRegistryRootKey(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		proto, err := api.MsigPropose(ctx, vrk, verifreg.Address, big.Zero(), sender, uint64(verifreg.Methods.AddVerifier), params)
		if err != nil {
			return err
		}

		sm, _, err := srv.PublishMessage(ctx, proto, false)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Printf("message sent, now waiting on cid: %s\n", msgCid)

		mwait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("failed to add verifier: %d", mwait.Receipt.ExitCode)
		}

		//TODO: Internal msg might still have failed
		return nil

	},
}

var verifRegAddVerifierFromAccountCmd = &cli.Command{
	Name:      "add-verifier-from-account",
	Usage:     "make a given account a verifier",
	ArgsUsage: "<verifier root key> <new verifier> <allowance>",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return fmt.Errorf("must specify three arguments: sender, verifier, and allowance")
		}

		sender, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		verifier, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		allowance, err := types.BigFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		// TODO: ActorUpgrade: Abstract
		params, err := actors.SerializeParams(&verifreg2.AddVerifierParams{Address: verifier, Allowance: allowance})
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		msg := &types.Message{
			To:     verifreg.Address,
			From:   sender,
			Method: verifreg.Methods.AddVerifier,
			Params: params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return err
		}

		fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

		mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("failed to add verified client: %d", mwait.Receipt.ExitCode)
		}

		return nil

	},
}

var verifRegVerifyClientCmd = &cli.Command{
	Name:   "verify-client",
	Usage:  "make a given account a verified client",
	Hidden: true,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "specify your verifier address to send the message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus verifreg`")
		froms := cctx.String("from")
		if froms == "" {
			return fmt.Errorf("must specify from address with --from")
		}

		fromk, err := address.NewFromString(froms)
		if err != nil {
			return err
		}

		if cctx.Args().Len() != 2 {
			return fmt.Errorf("must specify two arguments: address and allowance")
		}

		target, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		allowance, err := types.BigFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&verifreg2.AddVerifiedClientParams{Address: target, Allowance: allowance})
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

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

		mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("failed to add verified client: %d", mwait.Receipt.ExitCode)
		}

		return nil
	},
}

var verifRegListVerifiersCmd = &cli.Command{
	Name:   "list-verifiers",
	Usage:  "list all verifiers",
	Hidden: true,
	Action: func(cctx *cli.Context) error {
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus verifreg`")
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

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

var verifRegListClientsCmd = &cli.Command{
	Name:   "list-clients",
	Usage:  "list all verified clients",
	Hidden: true,
	Action: func(cctx *cli.Context) error {
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus verifreg`")
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

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
		return st.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
			_, err := fmt.Printf("%s: %s\n", addr, dcap)
			return err
		})
	},
}

var verifRegCheckClientCmd = &cli.Command{
	Name:   "check-client",
	Usage:  "check verified client remaining bytes",
	Hidden: true,
	Action: func(cctx *cli.Context) error {
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus verifreg`")
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify client address to check")
		}

		caddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		dcap, err := api.StateVerifiedClientStatus(ctx, caddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		if dcap == nil {
			return xerrors.Errorf("client %s is not a verified client", err)
		}

		fmt.Println(*dcap)

		return nil
	},
}

var verifRegCheckVerifierCmd = &cli.Command{
	Name:   "check-verifier",
	Usage:  "check verifiers remaining bytes",
	Hidden: true,
	Action: func(cctx *cli.Context) error {
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus verifreg`")
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify verifier address to check")
		}

		vaddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		vid, err := api.StateLookupID(ctx, vaddr, head.Key())
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, verifreg.Address, head.Key())
		if err != nil {
			return err
		}

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		st, err := verifreg.Load(store, act)
		if err != nil {
			return err
		}

		found, dcap, err := st.VerifierDataCap(vid)
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

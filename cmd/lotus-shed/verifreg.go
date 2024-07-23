package main

import (
	"encoding/hex"
	"fmt"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	verifreg2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
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
		verifRegCheckClientCmd,
		verifRegCheckVerifierCmd,
		verifRegRemoveVerifiedClientDataCapCmd,
	},
}

var verifRegAddVerifierFromMsigCmd = &cli.Command{
	Name:      "add-verifier",
	Usage:     "make a given account a verifier",
	ArgsUsage: "<message sender> <new verifier> <allowance>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return lcli.IncorrectNumArgs(cctx)
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

		mwait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 3 {
			return lcli.IncorrectNumArgs(cctx)
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
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus filplus`")
		froms := cctx.String("from")
		if froms == "" {
			return fmt.Errorf("must specify from address with --from")
		}

		fromk, err := address.NewFromString(froms)
		if err != nil {
			return err
		}

		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
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

var verifRegListVerifiersCmd = &cli.Command{
	Name:   "list-verifiers",
	Usage:  "list all verifiers",
	Hidden: true,
	Action: func(cctx *cli.Context) error {
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus filplus`")
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

var verifRegCheckClientCmd = &cli.Command{
	Name:   "check-client",
	Usage:  "check verified client remaining bytes",
	Hidden: true,
	Action: func(cctx *cli.Context) error {
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus filplus`")
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
		fmt.Println("DEPRECATED: This behavior is being moved to `lotus filplus`")
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

var verifRegRemoveVerifiedClientDataCapCmd = &cli.Command{
	Name:      "remove-verified-client-data-cap",
	Usage:     "Remove data cap from verified client",
	ArgsUsage: "<message sender> <client address> <allowance to remove> <verifier 1 address> <verifier 1 signature> <verifier 2 address> <verifier 2 signature>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 7 {
			return lcli.IncorrectNumArgs(cctx)
		}

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		sender, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		client, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		allowanceToRemove, err := types.BigFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		verifier1Addr, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		verifier1Sig, err := hex.DecodeString(cctx.Args().Get(4))
		if err != nil {
			return err
		}

		verifier2Addr, err := address.NewFromString(cctx.Args().Get(5))
		if err != nil {
			return err
		}

		verifier2Sig, err := hex.DecodeString(cctx.Args().Get(6))
		if err != nil {
			return err
		}

		var sig1 crypto.Signature
		if err := sig1.UnmarshalBinary(verifier1Sig); err != nil {
			return xerrors.Errorf("couldn't unmarshal sig: %w", err)
		}

		var sig2 crypto.Signature
		if err := sig2.UnmarshalBinary(verifier2Sig); err != nil {
			return xerrors.Errorf("couldn't unmarshal sig: %w", err)
		}

		params, err := actors.SerializeParams(&verifregtypes.RemoveDataCapParams{
			VerifiedClientToRemove: client,
			DataCapAmountToRemove:  allowanceToRemove,
			VerifierRequest1: verifregtypes.RemoveDataCapRequest{
				Verifier:          verifier1Addr,
				VerifierSignature: sig1,
			},
			VerifierRequest2: verifregtypes.RemoveDataCapRequest{
				Verifier:          verifier2Addr,
				VerifierSignature: sig2,
			},
		})
		if err != nil {
			return err
		}

		vrk, err := api.StateVerifiedRegistryRootKey(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		vrkState, err := api.StateGetActor(ctx, vrk, types.EmptyTSK)
		if err != nil {
			return err
		}

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		st, err := multisig.Load(store, vrkState)
		if err != nil {
			return fmt.Errorf("load vrk failed: %w ", err)
		}

		signers, err := st.Signers()
		if err != nil {
			return err
		}

		senderIsSigner := false
		senderIdAddr, err := address.IDFromAddress(sender)
		if err != nil {
			return err
		}

		for _, signer := range signers {
			signerIdAddr, err := address.IDFromAddress(signer)
			if err != nil {
				return err
			}

			if signerIdAddr == senderIdAddr {
				senderIsSigner = true
			}
		}

		if !senderIsSigner {
			return fmt.Errorf("sender must be a vrk signer")
		}

		proto, err := api.MsigPropose(ctx, vrk, verifreg.Address, big.Zero(), sender, uint64(verifreg.Methods.RemoveVerifiedClientDataCap), params)
		if err != nil {
			return err
		}

		sm, err := lcli.InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()
		fmt.Println("sending msg: ", msgCid)

		mwait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("failed to removed verified data cap: %d", mwait.Receipt.ExitCode)
		}

		//TODO: Internal msg might still have failed
		return nil
	},
}

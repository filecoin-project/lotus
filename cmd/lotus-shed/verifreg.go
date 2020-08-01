package main

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"

	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var verifRegCmd = &cli.Command{
	Name:  "verifreg",
	Usage: "Interact with the verified registry actor",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		verifRegAddVerifierCmd,
		verifRegVerifyClientCmd,
		verifRegListVerifiersCmd,
		verifRegListClientsCmd,
		verifRegCheckClientCmd,
		verifRegCheckVerifierCmd,
	},
}

var verifRegAddVerifierCmd = &cli.Command{
	Name:  "add-verifier",
	Usage: "make a given account a verifier",
	Action: func(cctx *cli.Context) error {
		fromk, err := address.NewFromString("t3qfoulel6fy6gn3hjmbhpdpf6fs5aqjb5fkurhtwvgssizq4jey5nw4ptq5up6h7jk7frdvvobv52qzmgjinq")
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

		params, err := actors.SerializeParams(&verifreg.AddVerifierParams{Address: target, Allowance: allowance})
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
			To:     builtin.VerifiedRegistryActorAddr,
			From:   fromk,
			Method: builtin.MethodsVerifiedRegistry.AddVerifier,
			Params: params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return err
		}

		fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

		mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("failed to add verifier: %d", mwait.Receipt.ExitCode)
		}

		return nil

	},
}

var verifRegVerifyClientCmd = &cli.Command{
	Name:  "verify-client",
	Usage: "make a given account a verified client",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "specify your verifier address to send the message from",
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

		params, err := actors.SerializeParams(&verifreg.AddVerifiedClientParams{Address: target, Allowance: allowance})
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
			To:       builtin.VerifiedRegistryActorAddr,
			From:     fromk,
			Method:   builtin.MethodsVerifiedRegistry.AddVerifiedClient,
			GasPrice: types.NewInt(1),
			GasLimit: 0,
			Params:   params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg)
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
	Name:  "list-verifiers",
	Usage: "list all verifiers",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		apibs := apibstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		var st verifreg.State
		if err := store.Get(ctx, act.Head, &st); err != nil {
			return err
		}

		vh, err := adt.AsMap(store, st.Verifiers)
		if err != nil {
			return err
		}

		var dcap verifreg.DataCap
		if err := vh.ForEach(&dcap, func(k string) error {
			addr, err := address.NewFromBytes([]byte(k))
			if err != nil {
				return err
			}

			fmt.Printf("%s: %s\n", addr, dcap)

			return nil
		}); err != nil {
			return err
		}

		return nil
	},
}

var verifRegListClientsCmd = &cli.Command{
	Name:  "list-clients",
	Usage: "list all verified clients",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		apibs := apibstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		var st verifreg.State
		if err := store.Get(ctx, act.Head, &st); err != nil {
			return err
		}

		vh, err := adt.AsMap(store, st.VerifiedClients)
		if err != nil {
			return err
		}

		var dcap verifreg.DataCap
		if err := vh.ForEach(&dcap, func(k string) error {
			addr, err := address.NewFromBytes([]byte(k))
			if err != nil {
				return err
			}

			fmt.Printf("%s: %s\n", addr, dcap)

			return nil
		}); err != nil {
			return err
		}

		return nil
	},
}

var verifRegCheckClientCmd = &cli.Command{
	Name:  "check-client",
	Usage: "check verified client remaining bytes",
	Action: func(cctx *cli.Context) error {
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
	Name:  "check-verifier",
	Usage: "check verifiers remaining bytes",
	Action: func(cctx *cli.Context) error {
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

		act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		apibs := apibstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		var st verifreg.State
		if err := store.Get(ctx, act.Head, &st); err != nil {
			return err
		}

		vh, err := adt.AsMap(store, st.Verifiers)
		if err != nil {
			return err
		}

		var dcap verifreg.DataCap
		if found, err := vh.Get(adt.AddrKey(vaddr), &dcap); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("not found")
		}

		fmt.Println(dcap)

		return nil
	},
}

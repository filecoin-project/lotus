package cli

import (
	"context"
	"fmt"

	verifreg4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/verifreg"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var filplusCmd = &cli.Command{
	Name:  "filplus",
	Usage: "Interact with the verified registry actor used by Filplus",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		filplusVerifyClientCmd,
		filplusListNotariesCmd,
		filplusListClientsCmd,
		filplusCheckClientCmd,
		filplusCheckNotaryCmd,
	},
}

var filplusVerifyClientCmd = &cli.Command{
	Name:  "grant-datacap",
	Usage: "give allowance to the specified verified client address",
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
		params, err := actors.SerializeParams(&verifreg4.AddVerifiedClientParams{Address: target, Allowance: allowance})
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

var filplusListNotariesCmd = &cli.Command{
	Name:  "list-notaries",
	Usage: "list all notaries",
	Action: func(cctx *cli.Context) error {
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
		return st.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
			_, err := fmt.Printf("%s: %s\n", addr, dcap)
			return err
		})
	},
}

var filplusCheckClientCmd = &cli.Command{
	Name:  "check-client-datacap",
	Usage: "check verified client remaining bytes",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
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
	Name:  "check-notary-datacap",
	Usage: "check a notary's remaining bytes",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
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

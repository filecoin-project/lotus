package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	actors "github.com/filecoin-project/lotus/chain/actors"
	types "github.com/filecoin-project/lotus/chain/types"
)

var multisigCmd = &cli.Command{
	Name:  "msig",
	Usage: "Interact with a multisig wallet",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "source",
			Usage: "specify the account to send propose from",
		},
	},
	Subcommands: []*cli.Command{
		msigCreateCmd,
		msigInspectCmd,
		msigProposeCmd,
		msigApproveCmd,
	},
}

var msigCreateCmd = &cli.Command{
	Name:  "create",
	Usage: "Create a new multisig wallet",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name: "required",
		},
		&cli.StringFlag{
			Name:  "value",
			Usage: "initial funds to give to multisig",
			Value: "0",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var addrs []address.Address
		for _, a := range cctx.Args().Slice() {
			addr, err := address.NewFromString(a)
			if err != nil {
				return err
			}
			addrs = append(addrs, addr)
		}

		// get the address we're going to use to create the multisig (can be one of the above, as long as they have funds)
		sendAddr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}

		val := cctx.String("value")
		filval, err := types.ParseFIL(val)
		if err != nil {
			return err
		}

		required := cctx.Int64("required")
		if required == 0 {
			required = int64(len(addrs))
		}

		// Set up constructor parameters for multisig
		msigParams := &samsig.ConstructorParams{
			Signers:               addrs,
			NumApprovalsThreshold: required,
		}

		enc, err := actors.SerializeParams(msigParams)
		if err != nil {
			return err
		}

		// new actors are created by invoking 'exec' on the init actor with the constructor params
		execParams := &init_.ExecParams{
			CodeCID:           builtin.MultisigActorCodeID,
			ConstructorParams: enc,
		}

		enc, err = actors.SerializeParams(execParams)
		if err != nil {
			return err
		}

		// now we create the message to send this with
		msg := types.Message{
			To:       builtin.InitActorAddr,
			From:     sendAddr,
			Method:   builtin.MethodsInit.Exec,
			Params:   enc,
			GasPrice: types.NewInt(1),
			GasLimit: types.NewInt(1000000),
			Value:    types.BigInt(filval),
		}

		// send the message out to the network
		smsg, err := api.MpoolPushMessage(ctx, &msg)
		if err != nil {
			return err
		}

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("actor creation failed!")
			return err
		}

		// get address of newly created miner
		msigaddr, err := address.NewFromBytes(wait.Receipt.Return)
		if err != nil {
			return err
		}

		fmt.Println("Created new multisig: ", msigaddr.String())
		// TODO: maybe register this somewhere
		return nil
	},
}

var msigInspectCmd = &cli.Command{
	Name:  "inspect",
	Usage: "Inspect a multisig wallet",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify address of multisig to inspect")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		obj, err := api.ChainReadObj(ctx, act.Head)
		if err != nil {
			return err
		}

		var mstate samsig.State
		if err := mstate.UnmarshalCBOR(bytes.NewReader(obj)); err != nil {
			return err
		}

		fmt.Printf("Balance: %sfil\n", types.FIL(act.Balance))
		fmt.Printf("Threshold: %d / %d\n", mstate.NumApprovalsThreshold, len(mstate.Signers))
		fmt.Println("Signers:")
		for _, s := range mstate.Signers {
			fmt.Printf("\t%s\n", s)
		}

		pending, err := GetMultisigPending(ctx, api, mstate.PendingTxns)
		if err != nil {
			return fmt.Errorf("reading pending transactions: %w", err)
		}

		fmt.Println("Transactions: ", len(pending))
		if len(pending) > 0 {
			var txids []int64
			for txid := range pending {
				txids = append(txids, txid)
			}
			sort.Slice(txids, func(i, j int) bool {
				return txids[i] < txids[j]
			})

			w := tabwriter.NewWriter(os.Stdout, 8, 4, 0, ' ', 0)
			fmt.Fprintf(w, "ID\tState\tTo\tValue\tMethod\tParams\n")
			for _, txid := range txids {
				tx := pending[txid]
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\t%x\n", txid, state(tx), tx.To, types.FIL(tx.Value), tx.Method, tx.Params)
			}
			w.Flush()
		}

		return nil
	},
}

func GetMultisigPending(ctx context.Context, lapi api.FullNode, hroot cid.Cid) (map[int64]*samsig.Transaction, error) {
	bs := apibstore.NewAPIBlockstore(lapi)
	cst := cbor.NewCborStore(bs)

	nd, err := hamt.LoadNode(ctx, cst, hroot, hamt.UseTreeBitWidth(5))
	if err != nil {
		return nil, err
	}

	txs := make(map[int64]*samsig.Transaction)
	err = nd.ForEach(ctx, func(k string, val interface{}) error {
		d := val.(*cbg.Deferred)
		var tx samsig.Transaction
		if err := tx.UnmarshalCBOR(bytes.NewReader(d.Raw)); err != nil {
			return err
		}

		txid, _ := binary.Varint([]byte(k))

		txs[txid] = &tx
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to iterate transactions hamt: %w", err)
	}

	return txs, nil
}

func state(tx *samsig.Transaction) string {
	/* // TODO(why): I strongly disagree with not having these... but i need to move forward
	if tx.Complete {
		return "done"
	}
	if tx.Canceled {
		return "canceled"
	}
	*/
	return "pending"
}

var msigProposeCmd = &cli.Command{
	Name:  "propose",
	Usage: "Propose a multisig transaction",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Args().Len() < 3 {
			return fmt.Errorf("must pass multisig address, destination, and value")
		}

		if cctx.Args().Len() > 3 && cctx.Args().Len() != 5 {
			return fmt.Errorf("usage: msig propose <msig addr> <desination> <value> [ <method> <params> ]")
		}

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		dest, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		value, err := types.ParseFIL(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		var method uint64
		var params []byte
		if cctx.Args().Len() == 5 {
			m, err := strconv.ParseUint(cctx.Args().Get(3), 10, 64)
			if err != nil {
				return err
			}
			method = m

			p, err := hex.DecodeString(cctx.Args().Get(4))
			if err != nil {
				return err
			}
			params = p
		}

		enc, err := actors.SerializeParams(&samsig.ProposeParams{
			To:     dest,
			Value:  abi.TokenAmount(types.BigInt(value)),
			Method: abi.MethodNum(method),
			Params: params,
		})
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("source") {
			f, err := address.NewFromString(cctx.String("source"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msg := &types.Message{
			To:       msig,
			From:     from,
			Value:    types.NewInt(0),
			Method:   builtin.MethodsMultisig.Propose,
			Params:   enc,
			GasLimit: types.NewInt(100000),
			GasPrice: types.NewInt(1),
		}

		smsg, err := api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return err
		}

		fmt.Println("send proposal in message: ", smsg.Cid())

		wait, err := api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("proposal returned exit %d", wait.Receipt.ExitCode)
		}

		_, v, err := cbg.CborReadHeader(bytes.NewReader(wait.Receipt.Return))
		if err != nil {
			return err
		}

		fmt.Printf("Transaction ID: %d\n", v)

		return nil
	},
}

var msigApproveCmd = &cli.Command{
	Name:  "approve",
	Usage: "Approve a multisig transaction",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Args().Len() < 2 {
			return fmt.Errorf("must pass multisig address and transaction ID")
		}

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		enc, err := actors.SerializeParams(&samsig.TxnIDParams{
			ID: samsig.TxnID(txid),
		})
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("source") {
			f, err := address.NewFromString(cctx.String("source"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msg := &types.Message{
			To:       msig,
			From:     from,
			Value:    types.NewInt(0),
			Method:   builtin.MethodsMultisig.Approve,
			Params:   enc,
			GasLimit: types.NewInt(100000),
			GasPrice: types.NewInt(1),
		}

		smsg, err := api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return err
		}

		fmt.Println("sent approval in message: ", smsg.Cid())

		wait, err := api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("approve returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

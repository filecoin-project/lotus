package main

import (
	"context"
	"fmt"
	"io"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
)

var evmBalanceCmd = &cli.Command{
	Name:      "evm-balance",
	Usage:     "Balances in eth accounts, evm contracts and placeholders",
	ArgsUsage: "[state root]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},

	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return xerrors.New("only needs state root")
		}

		ctx := context.TODO()
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass state root")
		}

		sroot, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		bs, err := lkrepo.Blockstore(ctx, repo.HotBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open blockstore: %w", err)
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		cst := cbor.NewCborStore(bs)
		st, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		fmt.Println("iterating over all actors")
		count := 0
		tvlEvm := abi.NewTokenAmount(0)
		tvlEthAccount := abi.NewTokenAmount(0)
		tvlPlaceholder := abi.NewTokenAmount(0)

		err = st.ForEach(func(addr address.Address, act *types.Actor) error {
			if count%200000 == 0 {
				fmt.Println("processed ", count, " actors building maps")
			}
			count++

			if builtin.IsEvmActor(act.Code) {
				tvlEvm = types.BigAdd(tvlEvm, act.Balance)
			}

			if builtin.IsEthAccountActor(act.Code) {
				tvlEthAccount = types.BigAdd(tvlEthAccount, act.Balance)
			}

			if builtin.IsPlaceholderActor(act.Code) {
				tvlPlaceholder = types.BigAdd(tvlPlaceholder, act.Balance)
			}

			return nil
		})

		fmt.Println("TVL in Eth contracts: ", tvlEvm)
		fmt.Println("TVL in Eth accounts: ", tvlEthAccount)
		fmt.Println("TVL in placeholder: ", tvlPlaceholder)
		fmt.Println("Total TVL: ", big.Add(big.Add(tvlEthAccount, tvlPlaceholder), tvlEvm))
		return nil
	},
}

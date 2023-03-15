package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/filecoin-project/go-state-types/big"

	badgerbs "github.com/filecoin-project/lotus/blockstore/badger"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/state"
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

		path, err := lkrepo.SplitstorePath()
		if err != nil {
			return err
		}

		path = filepath.Join(path, "hot.badger")
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}

		opts, err := repo.BadgerBlockstoreOptions(repo.HotBlockstore, path, lkrepo.Readonly())
		if err != nil {
			return err
		}

		bs, err := badgerbs.Open(opts)
		if err != nil {
			return err
		}

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

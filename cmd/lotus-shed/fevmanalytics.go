package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	badgerbs "github.com/filecoin-project/lotus/blockstore/badger"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	evm2 "github.com/filecoin-project/lotus/chain/actors/builtin/evm"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
)

var FevmAnalyticsCmd = &cli.Command{
	Name:  "evm-analytics",
	Usage: "Get FEVM related metrics",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Subcommands: []*cli.Command{
		FevmBalanceCmd,
		FevmActorsCmd,
	},
}

var FevmBalanceCmd = &cli.Command{
	Name:      "evm-balance",
	Usage:     "Balances in eth accounts, evm contracts and placeholders",
	ArgsUsage: "[state root]",

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
		balanceEvm := abi.NewTokenAmount(0)
		balanceEthAccount := abi.NewTokenAmount(0)
		balancePlaceholder := abi.NewTokenAmount(0)

		err = st.ForEach(func(addr address.Address, act *types.Actor) error {
			if count%200000 == 0 {
				fmt.Println("processed /n", count)
			}
			count++

			if builtin.IsEvmActor(act.Code) {
				balanceEvm = types.BigAdd(balanceEvm, act.Balance)
			}

			if builtin.IsEthAccountActor(act.Code) {
				balanceEthAccount = types.BigAdd(balanceEthAccount, act.Balance)
			}

			if builtin.IsPlaceholderActor(act.Code) {
				balancePlaceholder = types.BigAdd(balancePlaceholder, act.Balance)
			}

			return nil
		})
		if err != nil {
			return err
		}

		fmt.Println("balances in Eth contracts: ", balanceEvm)
		fmt.Println("balances in Eth accounts: ", balanceEthAccount)
		fmt.Println("balances in placeholder: ", balancePlaceholder)
		fmt.Println("Total balances: ", big.Add(big.Add(balanceEthAccount, balancePlaceholder), balanceEvm))
		return nil
	},
}

var FevmActorsCmd = &cli.Command{
	Name:      "evm-actors",
	Usage:     "actors # in eth accounts, evm contracts and placeholders",
	ArgsUsage: "[state root]",

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

		ctx := context.TODO()
		cst := cbor.NewCborStore(bs)
		store := adt.WrapStore(ctx, cst)

		st, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		fmt.Println("iterating over all actors")
		count := 0
		EvmCount := 0
		EthAccountCount := 0
		PlaceholderCount := 0
		ea := []cid.Cid{}

		err = st.ForEach(func(addr address.Address, act *types.Actor) error {
			if count%200000 == 0 {
				fmt.Println("processed /n", count)
			}
			count++

			if builtin.IsEvmActor(act.Code) {
				EvmCount++
				e, err := evm2.Load(store, act)
				if err != nil {
					return xerrors.Errorf("fail to load evm actor: %w", err)
				}
				bcid, err := e.GetBytecodeCID()
				if err != nil {
					return err
				}

				ea = append(ea, bcid)
			}

			if builtin.IsEthAccountActor(act.Code) {
				EthAccountCount++
			}

			if builtin.IsPlaceholderActor(act.Code) {
				PlaceholderCount++
			}

			return nil
		})
		if err != nil {
			return err
		}

		uniquesa := unique(ea)
		fmt.Println("# of EVM contracts: ", EvmCount)
		fmt.Println("# of unique EVM contracts: ", len(uniquesa))
		fmt.Println("b# of Eth accounts: ", EthAccountCount)
		fmt.Println("# of placeholder: ", PlaceholderCount)
		return nil
	},
}

func unique(intSlice []cid.Cid) []cid.Cid {
	keys := make(map[cid.Cid]bool)
	list := []cid.Cid{}
	for _, entry := range intSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

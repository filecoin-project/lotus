package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	gstbuiltin "github.com/filecoin-project/go-state-types/builtin"
	adt15 "github.com/filecoin-project/go-state-types/builtin/v15/util/adt"

	badgerbs "github.com/filecoin-project/lotus/blockstore/badger"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	evm2 "github.com/filecoin-project/lotus/chain/actors/builtin/evm"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	lcli "github.com/filecoin-project/lotus/cli"
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
		FevmStorageCmd,
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

var FevmStorageCmd = &cli.Command{
	Name:      "evm-storage",
	Usage:     "Dump every populated storage slot of an FEVM contract",
	ArgsUsage: "[eth or filecoin address]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to look up the actor at (pass comma separated array of cids, or @height, or @head)",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		addr, err := parseEvmActorAddress(cctx.Args().First())
		if err != nil {
			return err
		}

		ctx := lcli.ReqContext(cctx)

		h, err := loadChainStore(ctx, cctx.String("repo"))
		if err != nil {
			return err
		}
		defer h.closer()

		ts, err := lcli.LoadTipSet(ctx, cctx, &ChainStoreTipSetResolver{Chain: h.cs})
		if err != nil {
			return err
		}

		act, err := h.sm.LoadActor(ctx, addr, ts)
		if err != nil {
			return xerrors.Errorf("failed to load actor %s: %w", addr, err)
		}

		if !builtin.IsEvmActor(act.Code) {
			return xerrors.Errorf("actor %s is not an EVM actor (type: %s)", addr, builtin.ActorNameByCode(act.Code))
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(h.bs))

		est, err := evm2.Load(store, act)
		if err != nil {
			return xerrors.Errorf("failed to load evm actor state: %w", err)
		}

		root, err := contractStateRoot(est)
		if err != nil {
			return err
		}

		m, err := adt15.AsMap(store, root, gstbuiltin.DefaultHamtBitwidth)
		if err != nil {
			return xerrors.Errorf("failed to load storage tree %s: %w", root, err)
		}

		count := 0
		var val abi.CborBytes
		err = m.ForEach(&val, func(k string) error {
			if len(k) != 32 {
				return xerrors.Errorf("unexpected storage key length %d (want 32)", len(k))
			}

			fmt.Printf("0x%x: 0x%x\n", []byte(k), []byte(val))
			count++
			return nil
		})
		if err != nil {
			return xerrors.Errorf("failed to walk storage tree %s: %w", root, err)
		}

		fmt.Fprintf(os.Stderr, "%d populated storage slot(s)\n", count)

		return nil
	},
}

// parseEvmActorAddress accepts either a native filecoin address (including
// f410 delegated addresses) or a 0x-prefixed hex Ethereum address.
func parseEvmActorAddress(s string) (address.Address, error) {
	if addr, err := address.NewFromString(s); err == nil {
		return addr, nil
	}

	eaddr, err := ethtypes.ParseEthAddress(s)
	if err != nil {
		return address.Undef, xerrors.Errorf("%q is neither a filecoin address nor an eth address", s)
	}

	return eaddr.ToFilecoinAddress()
}

// contractStateRoot extracts the ContractState field (the root of the
// contract's storage tree) from a versioned evm actor state via reflection,
// since the field is present in every actor version but the concrete state
// struct differs per version.
func contractStateRoot(st evm2.State) (cid.Cid, error) {
	v := reflect.ValueOf(st.GetState())
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	f := v.FieldByName("ContractState")
	if !f.IsValid() {
		return cid.Undef, xerrors.Errorf("evm actor state has no ContractState field")
	}

	root, ok := f.Interface().(cid.Cid)
	if !ok {
		return cid.Undef, xerrors.Errorf("ContractState field is not a cid")
	}

	return root, nil
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

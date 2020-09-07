package main

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type accountInfo struct {
	Address       address.Address
	Balance       types.FIL
	Type          string
	Power         abi.StoragePower
	Worker        address.Address
	Owner         address.Address
	InitialPledge types.FIL
	PreCommits    types.FIL
	LockedFunds   types.FIL
	Sectors       uint64
}

var auditsCmd = &cli.Command{
	Name:        "audits",
	Description: "a collection of utilities for auditing the filecoin chain",
	Subcommands: []*cli.Command{
		chainBalanceCmd,
		chainBalanceStateCmd,
	},
}

var chainBalanceCmd = &cli.Command{
	Name:        "chain-balances",
	Description: "Produces a csv file of all account balances",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to start from",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		tsk := ts.Key()
		actors, err := api.StateListActors(ctx, tsk)
		if err != nil {
			return err
		}

		var infos []accountInfo
		for _, addr := range actors {
			act, err := api.StateGetActor(ctx, addr, tsk)
			if err != nil {
				return err
			}

			ai := accountInfo{
				Address: addr,
				Balance: types.FIL(act.Balance),
				Type:    string(act.Code.Hash()[2:]),
			}

			if act.Code == builtin.StorageMinerActorCodeID {
				pow, err := api.StateMinerPower(ctx, addr, tsk)
				if err != nil {
					return xerrors.Errorf("failed to get power: %w", err)
				}

				ai.Power = pow.MinerPower.RawBytePower
				info, err := api.StateMinerInfo(ctx, addr, tsk)
				if err != nil {
					return xerrors.Errorf("failed to get miner info: %w", err)
				}
				ai.Worker = info.Worker
				ai.Owner = info.Owner

			}
			infos = append(infos, ai)
		}

		fmt.Printf("Address,Balance,Type,Power,Worker,Owner\n")
		for _, acc := range infos {
			fmt.Printf("%s,%s,%s,%s,%s,%s\n", acc.Address, acc.Balance, acc.Type, acc.Power, acc.Worker, acc.Owner)
		}
		return nil
	},
}

var chainBalanceStateCmd = &cli.Command{
	Name:        "stateroot-balances",
	Description: "Produces a csv file of all account balances from a given stateroot",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.BoolFlag{
			Name: "miner-info",
		},
	},
	Action: func(cctx *cli.Context) error {
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

		ds, err := lkrepo.Datastore("/chain")
		if err != nil {
			return err
		}

		mds, err := lkrepo.Datastore("/metadata")
		if err != nil {
			return err
		}

		bs := blockstore.NewBlockstore(ds)

		cs := store.NewChainStore(bs, mds, vm.Syscalls(ffiwrapper.ProofVerifier))

		cst := cbor.NewCborStore(bs)

		sm := stmgr.NewStateManager(cs)

		tree, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		minerInfo := cctx.Bool("miner-info")

		var infos []accountInfo
		err = tree.ForEach(func(addr address.Address, act *types.Actor) error {

			ai := accountInfo{
				Address:       addr,
				Balance:       types.FIL(act.Balance),
				Type:          string(act.Code.Hash()[2:]),
				Power:         big.NewInt(0),
				LockedFunds:   types.FIL(big.NewInt(0)),
				InitialPledge: types.FIL(big.NewInt(0)),
				PreCommits:    types.FIL(big.NewInt(0)),
			}

			if act.Code == builtin.StorageMinerActorCodeID && minerInfo {
				pow, _, err := stmgr.GetPowerRaw(ctx, sm, sroot, addr)
				if err != nil {
					return xerrors.Errorf("failed to get power: %w", err)
				}

				ai.Power = pow.RawBytePower

				var st miner.State
				if err := cst.Get(ctx, act.Head, &st); err != nil {
					return xerrors.Errorf("failed to read miner state: %w", err)
				}

				sectors, err := adt.AsArray(cs.Store(ctx), st.Sectors)
				if err != nil {
					return xerrors.Errorf("failed to load sector set: %w", err)
				}

				ai.InitialPledge = types.FIL(st.InitialPledgeRequirement)
				ai.LockedFunds = types.FIL(st.LockedFunds)
				ai.PreCommits = types.FIL(st.PreCommitDeposits)
				ai.Sectors = sectors.Length()

				var minfo miner.MinerInfo
				if err := cst.Get(ctx, st.Info, &minfo); err != nil {
					return xerrors.Errorf("failed to read miner info: %w", err)
				}

				ai.Worker = minfo.Worker
				ai.Owner = minfo.Owner
			}
			infos = append(infos, ai)
			return nil
		})
		if err != nil {
			return xerrors.Errorf("failed to loop over actors: %w", err)
		}

		if minerInfo {
			fmt.Printf("Address,Balance,Type,Sectors,Worker,Owner,InitialPledge,Locked,PreCommits\n")
			for _, acc := range infos {
				fmt.Printf("%s,%s,%s,%d,%s,%s,%s,%s,%s\n", acc.Address, acc.Balance, acc.Type, acc.Sectors, acc.Worker, acc.Owner, acc.InitialPledge, acc.LockedFunds, acc.PreCommits)
			}
		} else {
			fmt.Printf("Address,Balance,Type\n")
			for _, acc := range infos {
				fmt.Printf("%s,%s,%s\n", acc.Address, acc.Balance, acc.Type)
			}
		}

		return nil
	},
}

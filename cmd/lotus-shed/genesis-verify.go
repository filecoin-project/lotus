package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	saacc "github.com/filecoin-project/specs-actors/actors/builtin/account"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
)

type addrInfo struct {
	Key     address.Address
	Balance types.FIL
}

type msigInfo struct {
	Signers   []address.Address
	Balance   types.FIL
	Threshold uint64
}

type minerInfo struct {
}

var genesisVerifyCmd = &cli.Command{
	Name:        "verify-genesis",
	Description: "verify some basic attributes of a genesis car file",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass genesis car file")
		}
		bs := blockstore.NewBlockstore(datastore.NewMapDatastore())

		cs := store.NewChainStore(bs, datastore.NewMapDatastore(), nil)

		cf := cctx.Args().Get(0)
		f, err := os.Open(cf)
		if err != nil {
			return xerrors.Errorf("opening the car file: %w", err)
		}

		ts, err := cs.Import(f)
		if err != nil {
			return err
		}

		sm := stmgr.NewStateManager(cs)

		total, err := stmgr.CheckTotalFIL(context.TODO(), sm, ts)
		if err != nil {
			return err
		}

		fmt.Println("Genesis: ", ts.Key())
		expFIL := big.Mul(big.NewInt(int64(build.FilBase)), big.NewInt(int64(build.FilecoinPrecision)))
		fmt.Printf("Total FIL: %s", types.FIL(total))
		if !expFIL.Equals(total) {
			color.Red("  INCORRECT!")
		}
		fmt.Println()

		cst := cbor.NewCborStore(bs)

		stree, err := state.LoadStateTree(cst, ts.ParentState())
		if err != nil {
			return err
		}

		var accAddrs, msigAddrs []address.Address
		kaccounts := make(map[address.Address]addrInfo)
		kmultisigs := make(map[address.Address]msigInfo)
		kminers := make(map[address.Address]minerInfo)

		ctx := context.TODO()

		if err := stree.ForEach(func(addr address.Address, act *types.Actor) error {
			switch act.Code {
			case builtin.StorageMinerActorCodeID:
				var st saminer.State
				if err := cst.Get(ctx, act.Head, &st); err != nil {
					return err
				}

				kminers[addr] = minerInfo{}
			case builtin.MultisigActorCodeID:
				var st samsig.State
				if err := cst.Get(ctx, act.Head, &st); err != nil {
					return xerrors.Errorf("multisig actor: %w", err)
				}

				kmultisigs[addr] = msigInfo{
					Balance:   types.FIL(act.Balance),
					Signers:   st.Signers,
					Threshold: st.NumApprovalsThreshold,
				}
				msigAddrs = append(msigAddrs, addr)
			case builtin.AccountActorCodeID:
				var st saacc.State
				if err := cst.Get(ctx, act.Head, &st); err != nil {
					log.Warn(xerrors.Errorf("account actor %s: %w", addr, err))
				}

				kaccounts[addr] = addrInfo{
					Key:     st.Address,
					Balance: types.FIL(act.Balance.Copy()),
				}
				accAddrs = append(accAddrs, addr)
			}
			return nil
		}); err != nil {
			return err
		}

		sort.Slice(accAddrs, func(i, j int) bool {
			return accAddrs[i].String() < accAddrs[j].String()
		})

		sort.Slice(msigAddrs, func(i, j int) bool {
			return msigAddrs[i].String() < msigAddrs[j].String()
		})

		fmt.Println("Account Actors:")
		for _, acc := range accAddrs {
			a := kaccounts[acc]
			fmt.Printf("%s\t%s\t%s\n", acc, a.Key, a.Balance)
		}

		fmt.Println("Multisig Actors:")
		for _, acc := range msigAddrs {
			m := kmultisigs[acc]
			fmt.Printf("%s\t%s\t%d\t[", acc, m.Balance, m.Threshold)
			for i, s := range m.Signers {
				fmt.Print(s)
				if i != len(m.Signers)-1 {
					fmt.Print(",")
				}
			}
			fmt.Printf("]\n")
		}
		return nil
	},
}

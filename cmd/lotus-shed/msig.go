package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
)

type msigBriefInfo struct {
	ID        address.Address
	Signer    []address.Address
	Balance   abi.TokenAmount
	Threshold uint64
}

var msigCmd = &cli.Command{
	Name: "msig",
	Subcommands: []*cli.Command{
		multisigGetAllCmd,
	},
}

var multisigGetAllCmd = &cli.Command{
	Name:      "all",
	Usage:     "get all multisig actor on chain with id, signers, threshold and balance at a tipset",
	ArgsUsage: "[state root]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
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

		bs, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
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
		store := adt.WrapStore(ctx, cst)

		tree, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		var msigActorsInfo []msigBriefInfo
		err = tree.ForEach(func(addr address.Address, act *types.Actor) error {
			if builtin.IsMultisigActor(act.Code) {
				ms, err := multisig.Load(store, act)
				if err != nil {
					return fmt.Errorf("load msig failed %v", err)

				}

				signers, _ := ms.Signers()
				threshold, _ := ms.Threshold()
				info := msigBriefInfo{
					ID:        addr,
					Signer:    signers,
					Balance:   act.Balance,
					Threshold: threshold,
				}
				msigActorsInfo = append(msigActorsInfo, info)

			}
			return nil
		})
		if err != nil {
			return err
		}

		out, err := json.MarshalIndent(msigActorsInfo, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	},
}

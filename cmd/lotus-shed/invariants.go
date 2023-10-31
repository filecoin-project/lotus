package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	v10 "github.com/filecoin-project/go-state-types/builtin/v10"
	v11 "github.com/filecoin-project/go-state-types/builtin/v11"
	v12 "github.com/filecoin-project/go-state-types/builtin/v12"
	v8 "github.com/filecoin-project/go-state-types/builtin/v8"
	v9 "github.com/filecoin-project/go-state-types/builtin/v9"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
)

var invariantsCmd = &cli.Command{
	Name:        "check-invariants",
	Description: "Check state invariants",
	ArgsUsage:   "[StateRootCid, height]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		stateRootCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse state root cid: %w", err)
		}

		epoch, err := strconv.ParseInt(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse epoch: %w", err)
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

		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(ffiwrapper.ProofVerifier), filcns.DefaultUpgradeSchedule(), nil, mds, index.DummyMsgIndex)
		if err != nil {
			return err
		}

		nv := sm.GetNetworkVersion(ctx, abi.ChainEpoch(epoch))
		fmt.Println("Network Version ", nv)

		av, err := actorstypes.VersionForNetwork(nv)
		if err != nil {
			return err
		}
		fmt.Println("Actors Version ", av)

		actorCodeCids, err := actors.GetActorCodeIDs(av)
		if err != nil {
			return err
		}

		actorStore := store.ActorStore(ctx, blockstore.NewTieredBstore(bs, blockstore.NewMemorySync()))

		// Load the state root.
		var stateRoot types.StateRoot
		if err := actorStore.Get(ctx, stateRootCid, &stateRoot); err != nil {
			return xerrors.Errorf("failed to decode state root: %w", err)
		}

		actorTree, err := builtin.LoadTree(actorStore, stateRoot.Actors)
		if err != nil {
			return err
		}

		startTime := time.Now()

		var messages *builtin.MessageAccumulator
		switch av {
		case actorstypes.Version8:
			messages, err = v8.CheckStateInvariants(actorTree, abi.ChainEpoch(epoch), actorCodeCids)
			if err != nil {
				return xerrors.Errorf("checking state invariants: %w", err)
			}
		case actorstypes.Version9:
			messages, err = v9.CheckStateInvariants(actorTree, abi.ChainEpoch(epoch), actorCodeCids)
			if err != nil {
				return xerrors.Errorf("checking state invariants: %w", err)
			}
		case actorstypes.Version10:
			messages, err = v10.CheckStateInvariants(actorTree, abi.ChainEpoch(epoch), actorCodeCids)
			if err != nil {
				return xerrors.Errorf("checking state invariants: %w", err)
			}
		case actorstypes.Version11:
			messages, err = v11.CheckStateInvariants(actorTree, abi.ChainEpoch(epoch), actorCodeCids)
			if err != nil {
				return xerrors.Errorf("checking state invariants: %w", err)
			}
		case actorstypes.Version12:
			messages, err = v12.CheckStateInvariants(actorTree, abi.ChainEpoch(epoch), actorCodeCids)
			if err != nil {
				return xerrors.Errorf("checking state invariants: %w", err)
			}
		default:
			return xerrors.Errorf("unsupported actor version: %v", av)
		}

		fmt.Println("completed, took ", time.Since(startTime))

		for _, message := range messages.Messages() {
			fmt.Println("got the following error: ", message)
		}

		return nil
	},
}

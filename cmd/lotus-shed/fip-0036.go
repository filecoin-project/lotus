package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/store"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	"github.com/filecoin-project/lotus/chain/state"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/mitchellh/go-homedir"

	"golang.org/x/xerrors"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
)

const APPROVE = 49

type Vote struct {
	OptionID      uint64          `json: "optionId"`
	SignerAddress address.Address `json: "signerAddress"`
}

type msigVote struct {
	Multisig          msigBriefInfo
	AcceptanceSingers []address.Address
	RejectionSigners  []address.Address
}

// https://filpoll.io/poll/16
// snapshot height: 2162760
// state root: bafy2bzacebdnzh43hw66bmvguk65wiwr5ssaejlq44fpdei2ysfh3eefpdlqs
var fip0036PollResultcmd = &cli.Command{
	Name:      "fip0036poll",
	Usage:     "Process the FIP0036 FilPoll result",
	ArgsUsage: "[state root, votes]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Subcommands: []*cli.Command{
		//spRBPCmd,
		//spDealByteCmd,
		clientCmd,
		tokenHolderCmd,
		//coreDevCmd,
		//finalResultCmd,
	},
}

func getVoters(file string) ([]Vote, error) {

	var votes []Vote
	vb, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, xerrors.Errorf("read vote: %w", err)
	}

	if err := json.Unmarshal(vb, &votes); err != nil {
		return nil, xerrors.Errorf("unmarshal vote: %w", err)
	}
	return votes, nil
}

func getAllMsig(st *state.StateTree, store adt.Store) ([]msigBriefInfo, error) {

	var msigActorsInfo []msigBriefInfo
	err := st.ForEach(func(addr address.Address, act *types.Actor) error {
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
		return nil, err
	}
	return msigActorsInfo, nil
}

var tokenHolderCmd = &cli.Command{
	Name:      "token-holder",
	Usage:     "get poll result for token holder group",
	ArgsUsage: "[state root]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return xerrors.New("filpoll0036 token-holder [state root] [votes.json]")
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

		//get all the votes' singer address && their vote
		vj, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("fail to get votes json")
		}
		votes, err := getVoters(vj)
		if err != nil {
			return xerrors.Errorf("fail to get votesrs: ", err)
		}
		//get all the msig
		msigs, err := getAllMsig(tree, store)

		acceptanceVote := 0
		rejectionVote := 0
		acceptanceBalance := abi.NewTokenAmount(0)
		rejectionBalance := abi.NewTokenAmount(0)
		msigPendingVotes := make(map[address.Address]msigVote)
		msigFinalVotes := make(map[address.Address]msigVote)

		for _, v := range votes {
			a, err := tree.GetActor(v.SignerAddress)
			if err != nil {
				return xerrors.Errorf("fail to get account account for signer: ", v.SignerAddress)
			}
			//regular account
			if v.OptionID == APPROVE {
				acceptanceVote += 1
				acceptanceBalance = types.BigAdd(acceptanceBalance, a.Balance)
			} else {
				rejectionVote += 1
				rejectionBalance = types.BigAdd(rejectionBalance, a.Balance)
			}

			//msig
			si, err := tree.LookupID(v.SignerAddress)
			if err != nil {
				return xerrors.Errorf("cannot resolve singer: ", si)
			}
			for _, m := range msigs {
				for _, ms := range m.Signer {
					if ms == si {
						if mv, found := msigPendingVotes[m.ID]; found { //other singer has voted

							if v.OptionID == APPROVE {
								mv.AcceptanceSingers = append(mv.AcceptanceSingers, v.SignerAddress)
							} else {
								mv.RejectionSigners = append(mv.RejectionSigners, v.SignerAddress)
							}

							//check if threshold meet
							if uint64(len(mv.AcceptanceSingers)) == m.Threshold {
								delete(msigPendingVotes, m.ID)
								msigFinalVotes[m.ID] = mv
								acceptanceBalance = types.BigAdd(acceptanceBalance, m.Balance)
							} else if uint64(len(mv.RejectionSigners)) == m.Threshold {
								delete(msigPendingVotes, m.ID)
								msigFinalVotes[m.ID] = mv
								rejectionBalance = types.BigAdd(rejectionBalance, m.Balance)
							} else {
								msigPendingVotes[m.ID] = mv
							}
						} else {

							n := msigVote{
								Multisig: m,
							}
							if v.OptionID == APPROVE {
								n.AcceptanceSingers = append(n.AcceptanceSingers, v.SignerAddress)
							} else {
								n.RejectionSigners = append(n.RejectionSigners, v.SignerAddress)
							}

							//check if threshold meet
							if uint64(len(mv.AcceptanceSingers)) == m.Threshold {
								delete(msigPendingVotes, m.ID)
								msigFinalVotes[m.ID] = mv
								acceptanceBalance = types.BigAdd(acceptanceBalance, m.Balance)
							} else if uint64(len(mv.RejectionSigners)) == m.Threshold {
								delete(msigPendingVotes, m.ID)
								msigFinalVotes[m.ID] = mv
								rejectionBalance = types.BigAdd(rejectionBalance, m.Balance)
							} else {
								msigPendingVotes[m.ID] = mv
							}
						}
					}
				}
			}
		}

		fmt.Printf("\nTotal amount of singers: %v\n ", len(votes))
		fmt.Printf("Total amount of valid multisig vote: %v\n ", len(msigFinalVotes))
		fmt.Printf("Total balance: %v\n", types.BigAdd(acceptanceBalance, rejectionBalance).String())
		fmt.Printf("Approve (#): %v\n", acceptanceVote)
		fmt.Printf("Reject (#): %v\n", rejectionVote)
		fmt.Printf("Approve (FIL): %v\n", acceptanceBalance.String())
		fmt.Printf("Reject (FIL): %v\n", rejectionBalance.String())
		av := types.BigDivFloat(acceptanceBalance, types.BigAdd(acceptanceBalance, rejectionBalance))
		rv := types.BigDivFloat(rejectionBalance, types.BigAdd(rejectionBalance, acceptanceBalance))
		if av > 0.05 {
			fmt.Printf("Token Holder Group Result: Pass. approve: %.5f, reject: %.5f\n", av, rv)
		} else {
			fmt.Printf("Token Holder Group Result: Not Pass. approve: %.5f, reject: %.5f\n", av, rv)
		}
		return nil
	},
}

var clientCmd = &cli.Command{
	Name:      "client",
	Usage:     "get poll result for client",
	ArgsUsage: "[state root]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return xerrors.New("filpoll0036 token-holder [state root] [votes.json]")
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

		//get all the votes' singer address && their vote
		vj, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("fail to get votes json")
		}
		votes, err := getVoters(vj)
		if err != nil {
			return xerrors.Errorf("fail to get votesrs: ", err)
		}

		//market actor
		ma, err := tree.GetActor(market.Address)
		if err != nil {
			return xerrors.Errorf("fail to get market actor: ", err)
		}

		ms, err := market.Load(store, ma)
		if err != nil {
			return xerrors.Errorf("fail to load market actor: ", err)
		}

		dps, _ := ms.Proposals()
		acceptanceBytes := abi.PaddedPieceSize(0)
		rejectionBytes := abi.PaddedPieceSize(0)
		acceptanceCount := 0
		rejectionCount := 0
		for _, v := range votes {
			if err != nil {
				return xerrors.Errorf("fail to get account account for signer: ", v.SignerAddress)
			}
			ai, err := tree.LookupID(v.SignerAddress)
			if err != nil {
				return xerrors.Errorf("cannot resolve singer: ", ai)
			}
			if err := dps.ForEach(func(dealID abi.DealID, d market.DealProposal) error {

				if d.Client == ai {
					if v.OptionID == APPROVE {
						acceptanceBytes += d.PieceSize
						acceptanceCount += 1
					} else {
						rejectionBytes += d.PieceSize
						rejectionCount += 1
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		fmt.Printf("\nTotal amount of clients: %v\n", acceptanceCount+rejectionCount)
		fmt.Printf("Total deal bytes: %v\n", acceptanceBytes+rejectionBytes)
		fmt.Printf("Approve (#): %v\n", acceptanceCount)
		fmt.Printf("Reject (#): %v\n", rejectionCount)
		fmt.Printf("Approve (byte): %v\n", acceptanceBytes)
		fmt.Printf("Reject (byte): %v\n", rejectionBytes)
		av := float64(acceptanceBytes) / float64(acceptanceBytes+rejectionBytes)
		rv := float64(rejectionBytes) / float64(acceptanceBytes+rejectionBytes)
		if av > 0.05 {
			fmt.Printf("Deal Client Group Result: Pass. approve: %.5f, reject: %.5f\n", av, rv)
		} else {
			fmt.Printf("Deal Client Group Result: Not Pass. approve: %.5f, reject: %.5f\n", av, rv)
		}
		return nil
	},
}

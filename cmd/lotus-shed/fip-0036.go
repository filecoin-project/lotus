package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/mitchellh/go-homedir"

	"github.com/filecoin-project/lotus/chain/actors/builtin/power"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/node/repo"

	"github.com/filecoin-project/lotus/chain/state"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/types"

	"golang.org/x/xerrors"
)

const APPROVE = 49
const Reject = 50

type Vote struct {
	OptionID      uint64          `json: "optionId"`
	SignerAddress address.Address `json: "signerAddress"`
}

type msigVote struct {
	Multisig     msigBriefInfo
	ApproveCount uint64
	RejectCount  uint64
}

// https://filpoll.io/poll/16
// snapshot height: 2162760
// state root: bafy2bzacebdnzh43hw66bmvguk65wiwr5ssaejlq44fpdei2ysfh3eefpdlqs
var fip36PollCmd = &cli.Command{
	Name:      "fip36poll",
	Usage:     "Process the FIP0036 FilPoll result",
	ArgsUsage: "[state root, votes]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Subcommands: []*cli.Command{
		spRBPAndDealCmd,
		clientCmd,
		tokenHolderCmd,
		//coreDevCmd,
		//finalResultCmd,
	},
}

func getVotesMap(file string, st *state.StateTree) (map[address.Address]uint64 /*map[Signer ID address]Option*/, error) {
	var votes []Vote
	vb, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, xerrors.Errorf("read vote: %w", err)
	}

	if err := json.Unmarshal(vb, &votes); err != nil {
		return nil, xerrors.Errorf("unmarshal vote: %w", err)
	}

	vm := make(map[address.Address]uint64)
	for _, v := range votes {
		si, err := st.LookupID(v.SignerAddress)
		if err != nil {
			return nil, xerrors.Errorf("fail to lookup address", err)
		}
		vm[si] = v.OptionID
	}
	return vm, nil
}

func getAllMsigSingerMap(st *state.StateTree, store adt.Store) (map[address.Address][]address.Address /*map[Singer ID address]msigActorId[]*/, error) {
	sm := make(map[address.Address][]address.Address)
	err := st.ForEach(func(addr address.Address, act *types.Actor) error {
		if builtin.IsMultisigActor(act.Code) {
			ms, err := multisig.Load(store, act)
			if err != nil {
				return fmt.Errorf("load msig failed %v", err)

			}

			ss, err := ms.Signers()
			if err != nil {
				return xerrors.Errorf("fail to get msig signers", err)
			}
			for _, s := range ss {
				if m, found := sm[s]; found { //add msig id to signer's collection
					m = append(m, addr)
					sm[s] = m
				} else {
					n := []address.Address{addr}
					sm[s] = n
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sm, nil
}

func getAllMsigIDMap(st *state.StateTree, store adt.Store) (map[address.Address]msigBriefInfo /*map[Multisig Actor ID]msigBriefInfo*/, error) {

	msigActorsInfo := make(map[address.Address]msigBriefInfo)
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
			msigActorsInfo[addr] = info
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return msigActorsInfo, nil
}

type MinerVoteInfo struct {
	AvailableBalance abi.TokenAmount
	DealBytes        abi.PaddedPieceSize
	RBP              abi.StoragePower
	Option           uint64
}

func getStorageMinerRBP(miner address.Address, st *state.StateTree, store adt.Store) (abi.StoragePower, error) {
	pa, err := st.GetActor(power.Address)
	if err != nil {
		return types.NewInt(0), xerrors.Errorf("failed to get power actor: \n", err)
	}

	ps, err := power.Load(store, pa)
	if err != nil {
		return types.NewInt(0), xerrors.Errorf("failed to get power state: \n", err)
	}

	mp, _, err := ps.MinerPower(miner)
	if err != nil {
		return types.NewInt(0), xerrors.Errorf("failed to get miner power: \n", err)
	}

	return mp.RawBytePower, nil
}

func getStorageMinerVotes(st *state.StateTree, votes map[address.Address]uint64, store adt.Store) (map[address.Address]MinerVoteInfo /*map[Storage Miner Actor]Option*/, error) {
	smv := make(map[address.Address]MinerVoteInfo)
	msigs, err := getAllMsigIDMap(st, store)
	if err != nil {
		xerrors.Errorf("fail to get msigs", err)
	}
	err = st.ForEach(func(addr address.Address, act *types.Actor) error {
		if builtin.IsStorageMinerActor(act.Code) {
			m, err := miner.Load(store, act)
			if err != nil {
				return xerrors.Errorf("fail to load miner actor: \n", err)
			}

			info, err := m.Info()
			if err != nil {
				return xerrors.Errorf("fail to get miner info: \n", err)
			}
			//check if owner voted
			o := info.Owner
			if v, found := votes[o]; found {
				//check if owner is msig
				if ms, found := msigs[o]; found { //owner is a msig
					ac := uint64(0)
					rc := uint64(0)
					for _, s := range ms.Signer {
						if v, found := votes[s]; found {
							if v == APPROVE {
								ac += 1
							} else {
								rc += 1
							}
						}
						mp, err := getStorageMinerRBP(addr, st, store)
						if err != nil {
							return xerrors.Errorf("fail to get miner actor rbp", err)
						}
						m := MinerVoteInfo{
							AvailableBalance: act.Balance,
							RBP:              mp,
						}

						//check if msig threshold is met for casting the vote
						if ac == ms.Threshold {
							m.Option = APPROVE
							smv[addr] = m
						} else if rc == ms.Threshold {
							m.Option = Reject
							smv[addr] = m
						} else {
							smv[addr] = m //no valid vote yet, option value should be 0
						}
					}
				} else { //owner is a regular wallet account
					mp, err := getStorageMinerRBP(addr, st, store)
					if err != nil {
						return xerrors.Errorf("fail to get miner actor rbp", err)
					}
					m := MinerVoteInfo{
						AvailableBalance: act.Balance,
						RBP:              mp,
						Option:           v,
					}
					smv[addr] = m
				}
			}

			//check if owner has not voted but worker voted
			if _, found := smv[addr]; !found {
				w := info.Worker
				if v, found := votes[w]; found {
					mp, err := getStorageMinerRBP(addr, st, store)
					if err != nil {
						return xerrors.Errorf("fail to get miner actor rbp", err)
					}
					m := MinerVoteInfo{
						AvailableBalance: act.Balance,
						RBP:              mp,
						Option:           v,
					}
					smv[addr] = m
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return smv, nil
}

var spRBPAndDealCmd = &cli.Command{
	Name: "sp",
	Usage: "get poll result for storage group. Weighted by RBP(raw byte power) and deal bytes stored in valid deals." +
		"\nNote that: if both owner key and worker key has voted, the vote made by owner key will be casted the storage provider actor.",
	ArgsUsage: "[state root]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.BoolFlag{
			Name:  "rbp-only",
			Value: false,
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

		//get all the votes' singer ID address && their vote
		vj, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("fail to get votes json")
		}
		votes, err := getVotesMap(vj, tree)
		if err != nil {
			return xerrors.Errorf("fail to get votesrs: ", err)
		}

		//get all storage miner votes
		smv, err := getStorageMinerVotes(tree, votes, store)
		if err != nil {
			return xerrors.Errorf("fail to get storage miner votes", err)
		}
		approveVote := 0
		rejectionVote := 0
		approveRBP := abi.NewStoragePower(0)
		rejectRBP := abi.NewStoragePower(0)
		approveDealBytes := abi.PaddedPieceSize(0)
		rejectDealBytes := abi.PaddedPieceSize(0)
		ownerMsigUnderThreshold := 0

		//get all market proposals, add load the # of the bytes each provider has made
		pds := make(map[address.Address]abi.PaddedPieceSize)
		if !cctx.Bool("rbp-only") {
			ma, err := tree.GetActor(market.Address)
			if err != nil {
				return xerrors.Errorf("fail to get market actor: ", err)
			}

			ms, err := market.Load(store, ma)
			if err != nil {
				return xerrors.Errorf("fail to load market actor: ", err)
			}
			dps, _ := ms.Proposals()
			if err := dps.ForEach(func(dealID abi.DealID, d market.DealProposal) error {
				if pd, found := pds[d.Provider]; found {
					s := d.PieceSize + pd
					pds[d.Provider] = s
				} else {
					pds[d.Provider] = d.PieceSize
				}
				return xerrors.Errorf("fail to load deals")
			}); err != nil {
				return xerrors.Errorf("fail to get deals")
			}
		}

		//process the vote
		for m, mvi := range smv {
			if mvi.Option == APPROVE {
				approveVote += 1
				approveRBP = types.BigAdd(approveRBP, mvi.RBP)
				if d, found := pds[m]; !cctx.Bool("rbp-only") && found {
					approveDealBytes += d
				}
			} else if mvi.Option == Reject {
				rejectionVote += 1
				rejectRBP = types.BigAdd(rejectRBP, mvi.RBP)
				if d, found := pds[m]; !cctx.Bool("rbp-only") && found {
					rejectDealBytes += d
				}
			} else { //owner is msig and didnt reach threshold
				ownerMsigUnderThreshold += 1
			}
		}
		fmt.Printf("\nTotal amount of storage provider: %v\n", len(smv))
		fmt.Printf("Approve (#): %v\n", approveVote)
		fmt.Printf("Reject (#): %v\n", rejectionVote)
		fmt.Printf("SP owner is multisig and the vote is invalid due to under threshold: %v\n", ownerMsigUnderThreshold)
		fmt.Printf("Total RPB voted: %v\n", types.BigAdd(approveRBP, rejectRBP))
		fmt.Printf("Approve (rbp): %v\n", approveRBP)
		fmt.Printf("Reject (rbp): %v\n", rejectRBP)
		av := types.BigDivFloat(approveRBP, types.BigAdd(approveRBP, rejectRBP))
		rv := types.BigDivFloat(rejectRBP, types.BigAdd(approveRBP, rejectRBP))
		if av > 0.05 {
			fmt.Printf("Storage Miner Group By RBP Result: Pass. approve: %.5f, reject: %.5f\n", av, rv)
		} else {
			fmt.Printf("Storage Miner Group By RBP Result: Not Pass. approve: %.5f, reject: %.5f\n", av, rv)
		}

		if !cctx.Bool("rbp-only") {
			fmt.Printf("Total deal bytes: %v\n", approveDealBytes+rejectDealBytes)
			fmt.Printf("Approve (deal bytes): %v\n", approveDealBytes)
			fmt.Printf("Reject (byte): %v\n", rejectDealBytes)
			av := float64(approveDealBytes) / float64(approveDealBytes+rejectDealBytes)
			rv := float64(rejectDealBytes) / float64(approveDealBytes+rejectDealBytes)
			if av > 0.05 {
				fmt.Printf("Storage Miner Group By Deal Bytes Result: Pass. approve: %.5f, reject: %.5f\n", av, rv)
			} else {
				fmt.Printf("Storage Miner Group By Deal Bytes: Not Pass. approve: %.5f, reject: %.5f\n", av, rv)
			}
		}

		return nil
	},
}
var tokenHolderCmd = &cli.Command{
	Name:      "token-holder",
	Usage:     "get poll result for token holder group. balance includes, regular wallet accounts, multisig wallet that has valid vote that meets threshold, and the available balance of the storage miner actors that voted",
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
		vm, err := getVotesMap(vj, tree)
		if err != nil {
			return xerrors.Errorf("fail to get voter map ", err)
		}

		//get all the msig singers & their msigs
		msigs, err := getAllMsigSingerMap(tree, store)

		approveVote := 0
		rejectionVote := 0
		approveBalance := abi.NewTokenAmount(0)
		rejectionBalance := abi.NewTokenAmount(0)
		msigPendingVotes := make(map[address.Address]msigVote) //map[msig ID]msigVote
		msigFinalVotes := 0

		for s, v := range vm {
			//process votes for regular accounts
			a, err := tree.GetActor(s)
			if err != nil {
				return xerrors.Errorf("fail to get account account for signer: ", err)
			}
			if v == APPROVE {
				approveVote += 1
				approveBalance = types.BigAdd(approveBalance, a.Balance)
			} else {
				rejectionVote += 1
				rejectionBalance = types.BigAdd(rejectionBalance, a.Balance)
			}

			//process msigs
			if mss, found := msigs[s]; found {
				for _, ms := range mss { //get all the msig singer has
					if mpv, found := msigPendingVotes[ms]; found { //other signers of the multisig have voted, yet the threshold has not met
						if v == APPROVE {
							if mpv.ApproveCount+1 == mpv.Multisig.Threshold { //met threshold
								approveVote += 1
								approveBalance = types.BigAdd(approveBalance, mpv.Multisig.Balance)
								msigFinalVotes += 1
								delete(msigPendingVotes, ms) //threshold, can skip later signer votes
							} else {
								mpv.ApproveCount += 1
								msigPendingVotes[ms] = mpv
							}
						} else {
							if mpv.RejectCount+1 == mpv.Multisig.Threshold { //met threshold
								rejectionVote += 1
								rejectionBalance = types.BigAdd(rejectionBalance, mpv.Multisig.Balance)
								msigFinalVotes += 1
								delete(msigPendingVotes, ms) //threshold, can skip later signer votes
							} else {
								mpv.RejectCount += 1
								msigPendingVotes[ms] = mpv
							}
						}
					} else { //first vote received from one of the signers of the msig
						msa, err := tree.GetActor(ms)
						if err != nil {
							return fmt.Errorf("load msig actor failed %v", err)
						}
						msas, err := multisig.Load(store, msa)
						if err != nil {
							return fmt.Errorf("load msig failed %v", err)

						}
						t, _ := msas.Threshold()
						if t == 1 { //met threshold with this signer's single vote
							if v == APPROVE {
								approveVote += 1
								approveBalance = types.BigAdd(approveBalance, msa.Balance)
							} else {
								rejectionVote += 1
								rejectionBalance = types.BigAdd(rejectionBalance, msa.Balance)
							}
							msigFinalVotes += 1
						} else { //threshold not met, add to pending vote
							if v == APPROVE {
								msigPendingVotes[ms] = msigVote{
									Multisig: msigBriefInfo{
										Balance:   msa.Balance,
										Threshold: t,
									},
									ApproveCount: 1,
								}
							} else {
								msigPendingVotes[ms] = msigVote{
									Multisig: msigBriefInfo{
										Balance:   msa.Balance,
										Threshold: t,
									},
									RejectCount: 1,
								}
							}
						}
					}
				}
			}
		}

		// add miner available balance
		mvs, err := getStorageMinerVotes(tree, vm, store)
		if err != nil {
			return xerrors.Errorf("fail to get storage miner vote ", err)
		}

		spApproveAvailableBalance := abi.NewTokenAmount(0)
		spRejectionAvailableBalance := abi.NewTokenAmount(0)
		for _, mv := range mvs {
			if mv.Option == APPROVE {
				approveVote += 1
				approveBalance = types.BigAdd(approveBalance, mv.AvailableBalance)
				spApproveAvailableBalance = types.BigAdd(approveBalance, spApproveAvailableBalance)
			} else if mv.Option == Reject {
				rejectionVote += 1
				rejectionBalance = types.BigAdd(rejectionBalance, mv.AvailableBalance)
				spRejectionAvailableBalance = types.BigAdd(rejectionBalance, spRejectionAvailableBalance)
			} else {
				continue
			}
		}

		fmt.Printf("\nTotal amount of singers: %v\n ", len(vm))
		fmt.Printf("Total amount of valid multisig vote: %v\n ", msigFinalVotes)
		fmt.Printf("Total balance: %v\n", types.BigAdd(approveBalance, rejectionBalance).String())
		fmt.Printf("Approve (#): %v\n", approveVote)
		fmt.Printf("Reject (#): %v\n", rejectionVote)
		fmt.Printf("Approve (FIL): %v\n", approveBalance.String())
		fmt.Printf("Reject (FIL): %v\n", rejectionBalance.String())
		fmt.Printf("Approve - Storage Miner Portion (FIL in account): %v\n", spApproveAvailableBalance.String())
		fmt.Printf("Reject - Storage Miner Portion (FIL in account): %v\n", spRejectionAvailableBalance.String())
		av := types.BigDivFloat(approveBalance, types.BigAdd(approveBalance, rejectionBalance))
		rv := types.BigDivFloat(rejectionBalance, types.BigAdd(rejectionBalance, approveBalance))
		if av > 0.05 {
			fmt.Printf("Token Holder Group Result: Pass. approve: %.5f, reject: %.5f,\n", av, rv)
		} else {
			fmt.Printf("Token Holder Group Result: Not Pass. approve: %.5f, reject: %.5f\n", av, rv)
		}
		return nil
	},
}

var clientCmd = &cli.Command{
	Name:      "client",
	Usage:     "get poll result for client, weighted by deal bytes that are in valid deals in the storage market.",
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

		//get all the votes' singer ID address && their voted option
		vj, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("fail to get votes json")
		}
		vm, err := getVotesMap(vj, tree)
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

		//get all market proposals, add load the # of the bytes each client has made
		dps, _ := ms.Proposals()
		cds := make(map[address.Address]abi.PaddedPieceSize)
		if err := dps.ForEach(func(dealID abi.DealID, d market.DealProposal) error {
			if cd, found := cds[d.Client]; found {
				s := d.PieceSize + cd
				cds[d.Client] = s
			} else {
				cds[d.Client] = d.PieceSize
			}
			return xerrors.Errorf("fail to load deals")
		}); err != nil {
			return xerrors.Errorf("fail to get deals")
		}

		approveBytes := abi.PaddedPieceSize(0)
		rejectionBytes := abi.PaddedPieceSize(0)
		approveCount := 0
		rejectionCount := 0
		for s, o := range vm {
			if ds, found := cds[s]; found {
				if o == APPROVE {
					approveBytes += ds
					approveCount += 1
				} else {
					rejectionBytes += ds
					rejectionCount += 1
				}
			}
		}
		fmt.Printf("\nTotal amount of clients: %v\n", approveCount+rejectionCount)
		fmt.Printf("Total deal bytes: %v\n", approveBytes+rejectionBytes)
		fmt.Printf("Approve (#): %v\n", approveCount)
		fmt.Printf("Reject (#): %v\n", rejectionCount)
		fmt.Printf("Approve (byte): %v\n", approveBytes)
		fmt.Printf("Reject (byte): %v\n", rejectionBytes)
		av := float64(approveBytes) / float64(approveBytes+rejectionBytes)
		rv := float64(rejectionBytes) / float64(approveBytes+rejectionBytes)
		if av > 0.05 {
			fmt.Printf("Deal Client Group Result: Pass. approve: %.5f, reject: %.5f\n", av, rv)
		} else {
			fmt.Printf("Deal Client Group Result: Not Pass. approve: %.5f, reject: %.5f\n", av, rv)
		}
		return nil
	},
}

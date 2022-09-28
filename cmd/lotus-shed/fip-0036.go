package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	_init "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
)

const APPROVE = 49
const Reject = 50

type Vote struct {
	OptionID      uint64
	SignerAddress address.Address
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
		finalResultCmd,
	},
}

var finalResultCmd = &cli.Command{
	Name:      "results",
	Usage:     "get poll results",
	ArgsUsage: "[state root] [height] [votes json]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},

	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return xerrors.New("filpoll0036 results [state root] [height] [votes.json]")
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

		st, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		height, err := strconv.Atoi(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		//get all the votes' signer ID address && their vote
		vj, err := homedir.Expand(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("fail to get votes json")
		}
		votes, err := getVotesMap(vj, st)
		if err != nil {
			return xerrors.Errorf("failed to get voters: ", err)
		}

		type minerBriefInfo struct {
			rawBytePower abi.StoragePower
			dealPower    abi.StoragePower
			balance      abi.TokenAmount
		}

		// power actor
		pa, err := st.GetActor(power.Address)
		if err != nil {
			return xerrors.Errorf("failed to get power actor: \n", err)
		}

		powerState, err := power.Load(store, pa)
		if err != nil {
			return xerrors.Errorf("failed to get power state: \n", err)
		}

		//market actor
		ma, err := st.GetActor(market.Address)
		if err != nil {
			return xerrors.Errorf("fail to get market actor: ", err)
		}

		marketState, err := market.Load(store, ma)
		if err != nil {
			return xerrors.Errorf("fail to load market state: ", err)
		}

		//init actor
		ia, err := st.GetActor(_init.Address)
		if err != nil {
			return xerrors.Errorf("fail to get init actor: ", err)
		}

		initState, err := _init.Load(store, ia)
		if err != nil {
			return xerrors.Errorf("fail to load init state: ", err)
		}

		initMap, err := initState.AddressMap()
		if err != nil {
			return xerrors.Errorf("fail to load init map: ", err)
		}

		lookupId := func(addr address.Address) address.Address {
			if addr.Protocol() == address.ID {
				return addr
			}

			var actorID cbg.CborInt
			if found, err := initMap.Get(abi.AddrKey(addr), &actorID); err != nil {
				panic(err)
			} else if found {
				// Reconstruct address from the ActorID.
				idAddr, err := address.NewIDAddress(uint64(actorID))
				if err != nil {
					panic(err)
				}
				return idAddr
			}

			panic("didn't find addr")
		}

		// we need to build several pieces of information, as we traverse the state tree:
		// a map of accounts to every msig that they are a signer of
		accountsToMultisigs := make(map[address.Address][]address.Address)
		// a map of multisigs to some info about them for quick lookup
		msigActorsInfo := make(map[address.Address]msigBriefInfo)
		// a map of accounts to every miner that they are an owner of
		ownerMap := make(map[address.Address][]address.Address)
		// a map of accounts to every miner that they are a worker of
		workerMap := make(map[address.Address][]address.Address)
		// a map of miners to some info aboout them for quick lookup
		minerActorsInfo := make(map[address.Address]minerBriefInfo)
		// a map of client addresses to deal data stored in proposals
		clientToDealStorage := make(map[address.Address]abi.StoragePower)

		fmt.Println("iterating over all actors")
		count := 0
		err = st.ForEach(func(addr address.Address, act *types.Actor) error {
			if count%200000 == 0 {
				fmt.Println("processed ", count, " actors building maps")
			}
			count++
			if builtin.IsMultisigActor(act.Code) {
				ms, err := multisig.Load(store, act)
				if err != nil {
					return fmt.Errorf("load msig failed %v", err)

				}

				// TODO: Confirm that these are always ID addresses
				signers, err := ms.Signers()
				if err != nil {
					return xerrors.Errorf("fail to get msig signers", err)
				}
				for _, s := range signers {
					signerId := lookupId(s)
					if m, found := accountsToMultisigs[signerId]; found { //add msig id to signer's collection
						m = append(m, addr)
						accountsToMultisigs[signerId] = m
					} else {
						n := []address.Address{addr}
						accountsToMultisigs[signerId] = n
					}
				}

				locked, err := ms.LockedBalance(abi.ChainEpoch(height))
				if err != nil {
					return xerrors.Errorf("failed to compute locked multisig balance: %w", err)
				}

				threshold, _ := ms.Threshold()
				info := msigBriefInfo{
					ID:        addr,
					Signer:    signers,
					Balance:   big.Min(big.Zero(), types.BigSub(act.Balance, locked)),
					Threshold: threshold,
				}
				msigActorsInfo[addr] = info
			}

			if builtin.IsStorageMinerActor(act.Code) {
				m, err := miner.Load(store, act)
				if err != nil {
					return xerrors.Errorf("fail to load miner actor: \n", err)
				}

				info, err := m.Info()
				if err != nil {
					return xerrors.Errorf("fail to get miner info: \n", err)
				}

				ownerId := lookupId(info.Owner)
				if m, found := ownerMap[ownerId]; found { //add miner id to owner list
					m = append(m, addr)
					ownerMap[ownerId] = m
				} else {
					n := []address.Address{addr}
					ownerMap[ownerId] = n
				}

				workerId := lookupId(info.Worker)
				if m, found := workerMap[workerId]; found { //add miner id to worker list
					m = append(m, addr)
					workerMap[workerId] = m
				} else {
					n := []address.Address{addr}
					workerMap[workerId] = n
				}

				bal, err := m.AvailableBalance(act.Balance)
				if err != nil {
					return err
				}

				pow, ok, err := powerState.MinerPower(addr)
				if err != nil {
					return err
				}

				if !ok {
					pow.RawBytePower = big.Zero()
				}

				minerActorsInfo[addr] = minerBriefInfo{
					rawBytePower: pow.RawBytePower,
					// gets added up outside this loop
					dealPower: big.Zero(),
					balance:   bal,
				}
			}

			return nil
		})

		if err != nil {
			return err
		}

		fmt.Println("iterating over proposals")
		dps, _ := marketState.Proposals()
		if err := dps.ForEach(func(dealID abi.DealID, d market.DealProposal) error {
			clientId := lookupId(d.Client)
			if cd, found := clientToDealStorage[clientId]; found {
				clientToDealStorage[clientId] = big.Add(cd, big.NewInt(int64(d.PieceSize)))
			} else {
				clientToDealStorage[clientId] = big.NewInt(int64(d.PieceSize))
			}

			providerId := lookupId(d.Provider)
			mai, found := minerActorsInfo[providerId]

			if !found {
				return xerrors.Errorf("didn't find miner %s", providerId)
			}

			mai.dealPower = big.Add(mai.dealPower, big.NewInt(int64(d.PieceSize)))
			minerActorsInfo[providerId] = mai
			return nil
		}); err != nil {
			return xerrors.Errorf("fail to get deals")
		}

		// now tabulate votes

		approveBalance := abi.NewTokenAmount(0)
		rejectionBalance := abi.NewTokenAmount(0)
		clientApproveBytes := big.Zero()
		clientRejectBytes := big.Zero()
		msigPendingVotes := make(map[address.Address]msigVote) //map[msig ID]msigVote
		votedMsigs := make(map[address.Address]struct{})
		votesIncludingMsigs := make(map[address.Address]uint64)
		fmt.Println("counting account and multisig votes")
		for signer, v := range votes {
			signerId := lookupId(signer)
			//process votes for regular accounts
			accountActor, err := st.GetActor(signerId)
			if err != nil {
				return xerrors.Errorf("fail to get account account for signer: ", err)
			}

			clientBytes, ok := clientToDealStorage[signerId]
			if !ok {
				clientBytes = big.Zero()
			}

			if v == APPROVE {
				approveBalance = types.BigAdd(approveBalance, accountActor.Balance)
				votesIncludingMsigs[signerId] = APPROVE
				clientApproveBytes = big.Add(clientApproveBytes, clientBytes)
			} else {
				rejectionBalance = types.BigAdd(rejectionBalance, accountActor.Balance)
				votesIncludingMsigs[signerId] = Reject
				clientRejectBytes = big.Add(clientRejectBytes, clientBytes)
			}

			//process msigs
			// TODO: Oh god, oh god, there's a possibility that a multisig has voted in both directions
			// and we'll pick a winner non-deterministically as we iterate...
			// We need to factor in vote time if that happens and pick whoever went first
			if mss, found := accountsToMultisigs[signerId]; found {
				for _, ms := range mss { //get all the msig signer has
					if _, ok := votedMsigs[ms]; ok {
						// msig has already voted, skip
						continue
					}
					if mpv, found := msigPendingVotes[ms]; found { //other signers of the multisig have voted, yet the threshold has not met
						if v == APPROVE {
							if mpv.ApproveCount+1 == mpv.Multisig.Threshold { //met threshold
								approveBalance = types.BigAdd(approveBalance, mpv.Multisig.Balance)
								delete(msigPendingVotes, ms) //threshold, can skip later signer votes
								votedMsigs[ms] = struct{}{}
								votesIncludingMsigs[ms] = APPROVE

							} else {
								mpv.ApproveCount++
								msigPendingVotes[ms] = mpv
							}
						} else {
							if mpv.RejectCount+1 == mpv.Multisig.Threshold { //met threshold
								rejectionBalance = types.BigAdd(rejectionBalance, mpv.Multisig.Balance)
								delete(msigPendingVotes, ms) //threshold, can skip later signer votes
								votedMsigs[ms] = struct{}{}
								votesIncludingMsigs[ms] = Reject

							} else {
								mpv.RejectCount++
								msigPendingVotes[ms] = mpv
							}
						}
					} else { //first vote received from one of the signers of the msig
						msi, ok := msigActorsInfo[ms]
						if !ok {
							return xerrors.Errorf("didn't find msig %s in msig map", ms)
						}

						if msi.Threshold == 1 { //met threshold with this signer's single vote
							if v == APPROVE {
								approveBalance = types.BigAdd(approveBalance, msi.Balance)
								votesIncludingMsigs[ms] = APPROVE

							} else {
								rejectionBalance = types.BigAdd(rejectionBalance, msi.Balance)
								votesIncludingMsigs[ms] = Reject
							}
							votedMsigs[ms] = struct{}{}
						} else { //threshold not met, add to pending vote
							if v == APPROVE {
								msigPendingVotes[ms] = msigVote{
									Multisig:     msi,
									ApproveCount: 1,
								}
							} else {
								msigPendingVotes[ms] = msigVote{
									Multisig:    msi,
									RejectCount: 1,
								}
							}
						}
					}
				}
			}
		}

		// time to process miners based on what we know about votesIncludingMsigs
		minerVotes := make(map[address.Address]uint64)
		fmt.Println("counting miner votes")
		for s, v := range votes {
			if minerInfos, found := ownerMap[s]; found {
				for _, minerInfo := range minerInfos {
					minerVotes[minerInfo] = v
				}
			}
			if minerInfos, found := workerMap[s]; found {
				for _, minerInfo := range minerInfos {
					if _, ok := minerVotes[minerInfo]; !ok {
						minerVotes[minerInfo] = v
					}
				}
			}
		}

		approveRBP := big.Zero()
		approveDealPower := big.Zero()
		rejectionRBP := big.Zero()
		rejectionDealPower := big.Zero()
		fmt.Println("adding up miner votes")
		for minerAddr, vote := range minerVotes {
			mbi, ok := minerActorsInfo[minerAddr]
			if !ok {
				return xerrors.Errorf("failed to find miner info for %s", minerAddr)
			}

			if vote == APPROVE {
				approveBalance = big.Add(approveBalance, mbi.balance)
				approveRBP = big.Add(approveRBP, mbi.rawBytePower)
				approveDealPower = big.Add(approveDealPower, mbi.dealPower)
			} else {
				rejectionBalance = big.Add(rejectionBalance, mbi.balance)
				rejectionRBP = big.Add(rejectionRBP, mbi.rawBytePower)
				rejectionDealPower = big.Add(rejectionDealPower, mbi.dealPower)
			}
		}

		fmt.Println("Total acceptance token: ", approveBalance)
		fmt.Println("Total rejection token: ", rejectionBalance)

		fmt.Println("Total acceptance SP deal power: ", approveDealPower)
		fmt.Println("Total rejection SP deal power: ", rejectionDealPower)

		fmt.Println("Total acceptance SP rb power: ", approveRBP)
		fmt.Println("Total rejection SP rb power: ", rejectionRBP)

		fmt.Println("Total acceptance Client rb power: ", clientApproveBytes)
		fmt.Println("Total rejection Client rb power: ", clientRejectBytes)

		fmt.Println("\n\nFinal results **drumroll**")
		if rejectionBalance.GreaterThanEqual(big.Mul(approveBalance, big.NewInt(2))) {
			fmt.Println("token holders VETO FIP-0036!!!")
		} else if approveBalance.LessThanEqual(rejectionBalance) {
			fmt.Println("token holders REJECT FIP-0036 :(")
		} else {
			fmt.Println("token holders ACCEPT FIP-0036 :)")
		}

		if rejectionDealPower.GreaterThanEqual(big.Mul(approveDealPower, big.NewInt(2))) {
			fmt.Println("SPs by deall data stored VETO FIP-0036!!!")
		} else if approveDealPower.LessThanEqual(rejectionDealPower) {
			fmt.Println("SPs by deal data stored REJECT FIP-0036 :(")
		} else {
			fmt.Println("SPs by deal data stored ACCEPT FIP-0036 :)")
		}

		if rejectionRBP.GreaterThanEqual(big.Mul(approveRBP, big.NewInt(2))) {
			fmt.Println("SPs by total raw byte power VETO FIP-0036!!!")
		} else if approveRBP.LessThanEqual(rejectionRBP) {
			fmt.Println("SPs by total raw byte power REJECT FIP-0036 :(")
		} else {
			fmt.Println("SPs by total raw byte power ACCEPT FIP-0036 :)")
		}

		if clientRejectBytes.GreaterThanEqual(big.Mul(clientApproveBytes, big.NewInt(2))) {
			fmt.Println("Storage Clients VETO FIP-0036!!!")
		} else if clientApproveBytes.LessThanEqual(clientRejectBytes) {
			fmt.Println("Storage Clients REJECT FIP-0036 :(")
		} else {
			fmt.Println("Storage Clients ACCEPT FIP-0036 :)")
		}

		return nil
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

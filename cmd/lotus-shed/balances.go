package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/go-units"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	smoothing14 "github.com/filecoin-project/go-state-types/builtin/v14/util/smoothing"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	smoothing15 "github.com/filecoin-project/go-state-types/builtin/v15/util/smoothing"
	gststore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	_init "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen/genesis"
	proofsffi "github.com/filecoin-project/lotus/chain/proofs/ffi"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

type accountInfo struct {
	Address         address.Address
	Balance         types.FIL
	Type            string
	Power           abi.StoragePower
	Worker          address.Address
	Owner           address.Address
	InitialPledge   types.FIL
	PreCommits      types.FIL
	LockedFunds     types.FIL
	Sectors         uint64
	VestingStart    abi.ChainEpoch
	VestingDuration abi.ChainEpoch
	VestingAmount   types.FIL
}

var auditsCmd = &cli.Command{
	Name:        "audits",
	Description: "a collection of utilities for auditing the filecoin chain",
	Subcommands: []*cli.Command{
		chainBalanceCmd,
		chainBalanceSanityCheckCmd,
		chainBalanceStateCmd,
		chainPledgeCmd,
		chainFip0081PledgeCmd,
		fillBalancesCmd,
		duplicatedMessagesCmd,
	},
}

var duplicatedMessagesCmd = &cli.Command{
	Name:  "duplicate-messages",
	Usage: "Check for duplicate messages included in a tipset.",
	UsageText: `Check for duplicate messages included in a tipset.

Due to Filecoin's expected consensus, a tipset may include the same message multiple times in
different blocks. The message will only be executed once.

This command will find such duplicate messages and print them to standard out as newline-delimited
JSON. Status messages in the form of "H: $HEIGHT ($PROGRESS%)" will be printed to standard error for
every day of chain processed.
`,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:        "parallel",
			Usage:       "the number of parallel threads for block processing",
			DefaultText: "half the number of cores",
		},
		&cli.IntFlag{
			Name:        "start",
			Usage:       "the first epoch to check",
			DefaultText: "genesis",
		},
		&cli.IntFlag{
			Name:        "end",
			Usage:       "the last epoch to check",
			DefaultText: "the current head",
		},
		&cli.IntSliceFlag{
			Name:        "method",
			Usage:       "filter results by method number",
			DefaultText: "all methods",
		},
		&cli.StringSliceFlag{
			Name:        "include-to",
			Usage:       "include only messages to the given address (does not perform address resolution)",
			DefaultText: "all recipients",
		},
		&cli.StringSliceFlag{
			Name:        "include-from",
			Usage:       "include only messages from the given address (does not perform address resolution)",
			DefaultText: "all senders",
		},
		&cli.StringSliceFlag{
			Name:  "exclude-to",
			Usage: "exclude messages to the given address (does not perform address resolution)",
		},
		&cli.StringSliceFlag{
			Name:  "exclude-from",
			Usage: "exclude messages from the given address (does not perform address resolution)",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		var head *types.TipSet
		if cctx.IsSet("end") {
			epoch := abi.ChainEpoch(cctx.Int("end"))
			head, err = api.ChainGetTipSetByHeight(ctx, epoch, types.EmptyTSK)
		} else {
			head, err = api.ChainHead(ctx)
		}
		if err != nil {
			return err
		}

		var printLk sync.Mutex

		threads := runtime.NumCPU() / 2
		if cctx.IsSet("parallel") {
			threads = cctx.Int("int")
			if threads <= 0 {
				return fmt.Errorf("parallelism needs to be at least 1")
			}
		} else if threads == 0 {
			threads = 1 // if we have one core, but who are we kidding...
		}

		throttle := make(chan struct{}, threads)

		methods := map[abi.MethodNum]bool{}
		for _, m := range cctx.IntSlice("method") {
			if m < 0 {
				return fmt.Errorf("expected method numbers to be non-negative")
			}
			methods[abi.MethodNum(m)] = true
		}

		addressSet := func(flag string) (map[address.Address]bool, error) {
			if !cctx.IsSet(flag) {
				return nil, nil
			}
			addrs := cctx.StringSlice(flag)
			set := make(map[address.Address]bool, len(addrs))
			for _, addrStr := range addrs {
				addr, err := address.NewFromString(addrStr)
				if err != nil {
					return nil, fmt.Errorf("failed to parse address %s: %w", addrStr, err)
				}
				set[addr] = true
			}
			return set, nil
		}

		onlyFrom, err := addressSet("include-from")
		if err != nil {
			return err
		}
		onlyTo, err := addressSet("include-to")
		if err != nil {
			return err
		}
		excludeFrom, err := addressSet("exclude-from")
		if err != nil {
			return err
		}
		excludeTo, err := addressSet("exclude-to")
		if err != nil {
			return err
		}

		target := abi.ChainEpoch(cctx.Int("start"))
		if target < 0 || target > head.Height() {
			return fmt.Errorf("start height must be greater than 0 and less than the end height")
		}
		totalEpochs := head.Height() - target

		for target <= head.Height() {
			select {
			case throttle <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}

			go func(ts *types.TipSet) {
				defer func() {
					<-throttle
				}()

				type addrNonce struct {
					s address.Address
					n uint64
				}
				anonce := func(m *types.Message) addrNonce {
					return addrNonce{
						s: m.From,
						n: m.Nonce,
					}
				}

				msgs := map[addrNonce]map[cid.Cid]*types.Message{}

				processMessage := func(c cid.Cid, m *types.Message) {
					// Filter
					if len(methods) > 0 && !methods[m.Method] {
						return
					}
					if len(onlyFrom) > 0 && !onlyFrom[m.From] {
						return
					}
					if len(onlyTo) > 0 && !onlyTo[m.To] {
						return
					}
					if excludeFrom[m.From] || excludeTo[m.To] {
						return
					}

					// Record
					msgSet, ok := msgs[anonce(m)]
					if !ok {
						msgSet = make(map[cid.Cid]*types.Message, 1)
						msgs[anonce(m)] = msgSet
					}
					msgSet[c] = m
				}

				encoder := json.NewEncoder(os.Stdout)

				for _, bh := range ts.Blocks() {
					bms, err := api.ChainGetBlockMessages(ctx, bh.Cid())
					if err != nil {
						fmt.Fprintln(os.Stderr, "ERROR: ", err)
						return
					}

					for i, m := range bms.BlsMessages {
						processMessage(bms.Cids[i], m)
					}

					for i, m := range bms.SecpkMessages {
						processMessage(bms.Cids[len(bms.BlsMessages)+i], &m.Message)
					}
				}
				for _, ms := range msgs {
					if len(ms) == 1 {
						continue
					}
					type Msg struct {
						Cid    string
						Value  string
						Method uint64
					}
					grouped := map[string][]Msg{}
					for c, m := range ms {
						addr := m.To.String()
						grouped[addr] = append(grouped[addr], Msg{
							Cid:    c.String(),
							Value:  types.FIL(m.Value).String(),
							Method: uint64(m.Method),
						})
					}
					printLk.Lock()
					err := encoder.Encode(grouped)
					if err != nil {
						fmt.Fprintln(os.Stderr, "ERROR: ", err)
					}
					printLk.Unlock()
				}
			}(head)

			if head.Parents().IsEmpty() {
				break
			}

			head, err = api.ChainGetTipSet(ctx, head.Parents())
			if err != nil {
				return err
			}

			if head.Height()%2880 == 0 {
				printLk.Lock()
				fmt.Fprintf(os.Stderr, "H: %s (%d%%)\n", head.Height(), (100*(head.Height()-target))/totalEpochs)
				printLk.Unlock()
			}
		}

		for i := 0; i < threads; i++ {
			select {
			case throttle <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}

		}

		printLk.Lock()
		fmt.Fprintf(os.Stderr, "H: %s (100%%)\n", head.Height())
		printLk.Unlock()

		return nil
	},
}

var chainBalanceSanityCheckCmd = &cli.Command{
	Name:        "chain-balance-sanity",
	Description: "Confirms that the total balance of every actor in state is still 2 billion",
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

		bal := big.Zero()
		for _, addr := range actors {
			act, err := api.StateGetActor(ctx, addr, tsk)
			if err != nil {
				return err
			}

			bal = big.Add(bal, act.Balance)
		}

		attoBase := big.Mul(big.NewInt(int64(buildconstants.FilBase)), big.NewInt(int64(buildconstants.FilecoinPrecision)))

		if big.Cmp(attoBase, bal) != 0 {
			return xerrors.Errorf("sanity check failed (expected %s, actual %s)", attoBase, bal)
		}

		fmt.Println("sanity check successful")

		return nil
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

			if builtin.IsStorageMinerActor(act.Code) {
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

		printAccountInfos(infos, false)

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
		&cli.BoolFlag{
			Name: "robust-addresses",
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

		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(proofsffi.ProofVerifier), filcns.DefaultUpgradeSchedule(), nil, mds, nil)
		if err != nil {
			return err
		}

		tree, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		minerInfo := cctx.Bool("miner-info")

		robustMap := make(map[address.Address]address.Address)
		if cctx.Bool("robust-addresses") {
			iact, err := tree.GetActor(_init.Address)
			if err != nil {
				return xerrors.Errorf("failed to load init actor: %w", err)
			}

			ist, err := _init.Load(store, iact)
			if err != nil {
				return xerrors.Errorf("failed to load init actor state: %w", err)
			}

			err = ist.ForEachActor(func(id abi.ActorID, addr address.Address) error {
				idAddr, err := address.NewIDAddress(uint64(id))
				if err != nil {
					return xerrors.Errorf("failed to write to addr map: %w", err)
				}

				robustMap[idAddr] = addr

				return nil
			})
			if err != nil {
				return xerrors.Errorf("failed to invert init address map: %w", err)
			}
		}

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
				VestingAmount: types.FIL(big.NewInt(0)),
			}

			if cctx.Bool("robust-addresses") {
				robust, found := robustMap[addr]
				if found {
					ai.Address = robust
				} else {
					id, err := address.IDFromAddress(addr)
					if err != nil {
						return xerrors.Errorf("failed to get ID address: %w", err)
					}

					// TODO: This is not the correctest way to determine whether a robust address should exist
					if id >= genesis.MinerStart {
						return xerrors.Errorf("address doesn't have a robust address: %s", addr)
					}
				}
			}

			if minerInfo && builtin.IsStorageMinerActor(act.Code) {
				pow, _, _, err := stmgr.GetPowerRaw(ctx, sm, sroot, addr)
				if err != nil {
					return xerrors.Errorf("failed to get power: %w", err)
				}

				ai.Power = pow.RawBytePower

				st, err := miner.Load(store, act)
				if err != nil {
					return xerrors.Errorf("failed to read miner state: %w", err)
				}

				liveSectorCount, err := st.NumLiveSectors()
				if err != nil {
					return xerrors.Errorf("failed to compute live sector count: %w", err)
				}

				lockedFunds, err := st.LockedFunds()
				if err != nil {
					return xerrors.Errorf("failed to compute locked funds: %w", err)
				}

				ai.InitialPledge = types.FIL(lockedFunds.InitialPledgeRequirement)
				ai.LockedFunds = types.FIL(lockedFunds.VestingFunds)
				ai.PreCommits = types.FIL(lockedFunds.PreCommitDeposits)
				ai.Sectors = liveSectorCount

				minfo, err := st.Info()
				if err != nil {
					return xerrors.Errorf("failed to get miner info: %w", err)
				}

				ai.Worker = minfo.Worker
				ai.Owner = minfo.Owner
			}

			if builtin.IsMultisigActor(act.Code) {
				mst, err := multisig.Load(store, act)
				if err != nil {
					return err
				}

				ai.VestingStart, err = mst.StartEpoch()
				if err != nil {
					return err
				}

				ib, err := mst.InitialBalance()
				if err != nil {
					return err
				}

				ai.VestingAmount = types.FIL(ib)

				ai.VestingDuration, err = mst.UnlockDuration()
				if err != nil {
					return err
				}

			}

			infos = append(infos, ai)
			return nil
		})
		if err != nil {
			return xerrors.Errorf("failed to loop over actors: %w", err)
		}

		printAccountInfos(infos, minerInfo)

		return nil
	},
}

func printAccountInfos(infos []accountInfo, minerInfo bool) {
	if minerInfo {
		fmt.Printf("Address,Balance,Type,Sectors,Worker,Owner,InitialPledge,Locked,PreCommits,VestingStart,VestingDuration,VestingAmount\n")
		for _, acc := range infos {
			fmt.Printf("%s,%s,%s,%d,%s,%s,%s,%s,%s,%d,%d,%s\n", acc.Address, acc.Balance.Unitless(), acc.Type, acc.Sectors, acc.Worker, acc.Owner, acc.InitialPledge.Unitless(), acc.LockedFunds.Unitless(), acc.PreCommits.Unitless(), acc.VestingStart, acc.VestingDuration, acc.VestingAmount.Unitless())
		}
	} else {
		fmt.Printf("Address,Balance,Type\n")
		for _, acc := range infos {
			fmt.Printf("%s,%s,%s\n", acc.Address, acc.Balance.Unitless(), acc.Type)
		}
	}

}

var chainPledgeCmd = &cli.Command{
	Name:        "stateroot-pledge",
	Description: "Calculate sector pledge numbers",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	ArgsUsage: "[stateroot epoch]",
	Action: func(cctx *cli.Context) error {
		_ = logging.SetLogLevel("badger", "ERROR")
		ctx := context.TODO()

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass state root")
		}

		sroot, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		epoch, err := strconv.ParseInt(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing epoch arg: %w", err)
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
			return xerrors.Errorf("failed to open blockstore: %w", err)
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

		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc),
			vm.Syscalls(proofsffi.ProofVerifier), filcns.DefaultUpgradeSchedule(), nil, mds, nil)
		if err != nil {
			return err
		}
		state, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		var (
			powerSmoothed    builtin.FilterEstimate
			pledgeCollateral abi.TokenAmount
		)
		if act, err := state.GetActor(power.Address); err != nil {
			return xerrors.Errorf("loading miner actor: %w", err)
		} else if s, err := power.Load(store, act); err != nil {
			return xerrors.Errorf("loading power actor state: %w", err)
		} else if p, err := s.TotalPowerSmoothed(); err != nil {
			return xerrors.Errorf("failed to determine total power: %w", err)
		} else if c, err := s.TotalLocked(); err != nil {
			return xerrors.Errorf("failed to determine pledge collateral: %w", err)
		} else {
			powerSmoothed = p
			pledgeCollateral = c
		}

		circ, err := sm.GetVMCirculatingSupplyDetailed(ctx, abi.ChainEpoch(epoch), state)
		if err != nil {
			return err
		}

		fmt.Println("(real) circulating supply: ", types.FIL(circ.FilCirculating))
		if circ.FilCirculating.LessThan(big.Zero()) {
			circ.FilCirculating = big.Zero()
		}

		var epochsSinceRampStart int64
		var rampDurationEpochs uint64

		if powerActor, err := state.GetActor(power.Address); err != nil {
			return xerrors.Errorf("loading power actor: %w", err)
		} else if powerState, err := power.Load(store, powerActor); err != nil {
			return xerrors.Errorf("loading power actor state: %w", err)
		} else if powerState.RampStartEpoch() > 0 {
			epochsSinceRampStart = epoch - powerState.RampStartEpoch()
			rampDurationEpochs = powerState.RampDurationEpochs()
		}

		rewardActor, err := state.GetActor(reward.Address)
		if err != nil {
			return xerrors.Errorf("loading miner actor: %w", err)
		}
		rewardState, err := reward.Load(store, rewardActor)
		if err != nil {
			return xerrors.Errorf("loading reward actor state: %w", err)
		}

		fmt.Println("FilVested", types.FIL(circ.FilVested))
		fmt.Println("FilMined", types.FIL(circ.FilMined))
		fmt.Println("FilBurnt", types.FIL(circ.FilBurnt))
		fmt.Println("FilLocked", types.FIL(circ.FilLocked))
		fmt.Println("FilCirculating", types.FIL(circ.FilCirculating))

		for _, sectorWeight := range []abi.StoragePower{
			types.NewInt(32 << 30),
			types.NewInt(64 << 30),
			types.NewInt(32 << 30 * 10),
			types.NewInt(64 << 30 * 10),
		} {
			initialPledge, err := rewardState.InitialPledgeForPower(
				sectorWeight,
				pledgeCollateral,
				&powerSmoothed,
				circ.FilCirculating,
				epochsSinceRampStart,
				rampDurationEpochs,
			)
			if err != nil {
				return xerrors.Errorf("calculating initial pledge: %w", err)
			}

			fmt.Println("IP ", units.HumanSize(float64(sectorWeight.Uint64())), types.FIL(initialPledge))
		}

		return nil
	},
}

const dateFmt = "1/02/06"

func parseCsv(inp string) ([]time.Time, []address.Address, error) {
	fi, err := os.Open(inp)
	if err != nil {
		return nil, nil, err
	}

	r := csv.NewReader(fi)
	recs, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	var addrs []address.Address
	for _, rec := range recs[1:] {
		a, err := address.NewFromString(rec[0])
		if err != nil {
			return nil, nil, err
		}
		addrs = append(addrs, a)
	}

	var dates []time.Time
	for _, d := range recs[0][1:] {
		if len(d) == 0 {
			continue
		}
		p := strings.Split(d, " ")
		t, err := time.Parse(dateFmt, p[len(p)-1])
		if err != nil {
			return nil, nil, err
		}

		dates = append(dates, t)
	}

	return dates, addrs, nil
}

func heightForDate(d time.Time, ts *types.TipSet) abi.ChainEpoch {
	secs := d.Unix()
	gents := ts.Blocks()[0].Timestamp
	gents -= uint64(30 * ts.Height())
	return abi.ChainEpoch((secs - int64(gents)) / 30)
}

var fillBalancesCmd = &cli.Command{
	Name:        "fill-balances",
	Description: "fill out balances for addresses on dates in given spreadsheet",
	Flags:       []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		dates, addrs, err := parseCsv(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		var tipsets []*types.TipSet
		for _, d := range dates {
			h := heightForDate(d, ts)
			hts, err := api.ChainGetTipSetByHeight(ctx, h, ts.Key())
			if err != nil {
				return err
			}
			tipsets = append(tipsets, hts)
		}

		var balances [][]abi.TokenAmount
		for _, a := range addrs {
			var b []abi.TokenAmount
			for _, hts := range tipsets {
				act, err := api.StateGetActor(ctx, a, hts.Key())
				if err != nil {
					if !strings.Contains(err.Error(), "actor not found") {
						return fmt.Errorf("error for %s at %s: %w", a, hts.Key(), err)
					}
					b = append(b, types.NewInt(0))
					continue
				}
				b = append(b, act.Balance)
			}
			balances = append(balances, b)
		}

		var datestrs []string
		for _, d := range dates {
			datestrs = append(datestrs, "Balance at "+d.Format(dateFmt))
		}

		w := csv.NewWriter(os.Stdout)
		_ = w.Write(append([]string{"Wallet Address"}, datestrs...))
		for i := 0; i < len(addrs); i++ {
			row := []string{addrs[i].String()}
			for _, b := range balances[i] {
				row = append(row, types.FIL(b).String())
			}
			_ = w.Write(row)
		}
		w.Flush()
		return nil
	},
}

var chainFip0081PledgeCmd = &cli.Command{
	Name:        "fip0081-pledge",
	Description: "Calculate sector pledge values comparing current to pre-FIP-0081",
	ArgsUsage:   "[epoch number]",
	Action: func(cctx *cli.Context) error {

		ctx := lcli.ReqContext(cctx)

		api, acloser, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		var ts *types.TipSet
		if cctx.Args().Present() {
			epoch, err := strconv.ParseInt(cctx.Args().First(), 10, 64)
			if err != nil {
				return xerrors.Errorf("parsing epoch arg: %w", err)
			}
			ts, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(epoch), types.EmptyTSK)
			if err != nil {
				return err
			}
		} else {
			ts, err = api.ChainHead(ctx)
			if err != nil {
				return err
			}
		}

		cases := []struct {
			sectorSize   abi.SectorSize
			verifiedSize uint64
			duration     abi.ChainEpoch
		}{
			{
				sectorSize:   2 << 10,
				verifiedSize: 2 << 10,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   2 << 10,
				verifiedSize: (2 << 10) / 2,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   2 << 10,
				verifiedSize: 0,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   2 << 10,
				verifiedSize: 2 << 10,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   2 << 10,
				verifiedSize: (2 << 10) / 2,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   2 << 10,
				verifiedSize: 0,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   32 << 30,
				verifiedSize: 32 << 30,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   32 << 30,
				verifiedSize: (32 << 30) / 2,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   32 << 30,
				verifiedSize: 0,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   32 << 30,
				verifiedSize: 32 << 30,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   32 << 30,
				verifiedSize: (32 << 30) / 2,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   32 << 30,
				verifiedSize: 0,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   64 << 30,
				verifiedSize: 64 << 30,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   64 << 30,
				verifiedSize: (64 << 30) / 2,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   64 << 30,
				verifiedSize: 0,
				duration:     builtin.EpochsInYear,
			},
			{
				sectorSize:   64 << 30,
				verifiedSize: 64 << 30,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   64 << 30,
				verifiedSize: (64 << 30) / 2,
				duration:     3 * builtin.EpochsInYear,
			},
			{
				sectorSize:   64 << 30,
				verifiedSize: 0,
				duration:     3 * builtin.EpochsInYear,
			},
		}

		fmt.Printf("\033[3mCalculating at epoch %d\033[0m\n", ts.Height())
		fmt.Printf(" \033[1mSector Size\033[0m | \033[1mVerified %%\033[0m | \033[1mDuration\033[0m  | \033[1mActual\033[0m                   | \033[1mPre-FIP-0081\033[0m             | \033[1mDifference\033[0m\n")
		fmt.Println(strings.Repeat("-", 119))

		for _, c := range cases {
			pledge, err := api.StateMinerInitialPledgeForSector(ctx, c.duration, c.sectorSize, c.verifiedSize, ts.Key())
			if err != nil {
				return err
			}
			newPledge, err := postFip0081StateMinerInitialPledgeForSector(ctx, api, c.duration, c.sectorSize, c.verifiedSize, ts)
			if err != nil {
				return err
			}
			if !pledge.Equals(newPledge) {
				return xerrors.Errorf("failed to sanity check StateMinerInitialPledgeForSector calculation!")
			}
			oldPledge, err := preFip0081StateMinerInitialPledgeForSector(ctx, api, c.duration, c.sectorSize, c.verifiedSize, ts)
			if err != nil {
				return err
			}

			fmt.Printf(" %-11s | % 4.f%%      | %0.f year(s) | %-24s | %-24s | %s\n",
				c.sectorSize.ShortString(),
				float64(c.verifiedSize)/float64(c.sectorSize)*100,
				float64(c.duration)/builtin.EpochsInYear,
				types.FIL(pledge).String(),
				types.FIL(oldPledge).String(),
				types.FIL(types.BigSub(pledge, oldPledge)).String(),
			)
		}
		fmt.Println(strings.Repeat("-", 119))

		return nil
	},
}

// from itests/migration_test.go

// preFip0081StateMinerInitialPledgeForSector is the same calculation as StateMinerInitialPledgeForSector
// but uses miner14's version of the calculation without the FIP-0081 changes.
func preFip0081StateMinerInitialPledgeForSector(
	ctx context.Context,
	client api.FullNode,
	sectorDuration abi.ChainEpoch,
	sectorSize abi.SectorSize,
	verifiedSize uint64,
	ts *types.TipSet,
) (types.BigInt, error) {
	bs := blockstore.NewAPIBlockstore(client)
	ctxStore := gststore.WrapBlockStore(ctx, bs)

	circSupply, err := client.StateVMCirculatingSupplyInternal(ctx, ts.Key())
	if err != nil {
		return types.NewInt(0), err
	}

	powerActor, err := client.StateGetActor(ctx, power.Address, ts.Key())
	if err != nil {
		return types.NewInt(0), err
	}

	powerState, err := power.Load(ctxStore, powerActor)
	if err != nil {
		return types.NewInt(0), err
	}

	rewardActor, err := client.StateGetActor(ctx, reward.Address, ts.Key())
	if err != nil {
		return types.NewInt(0), err
	}

	rewardState, err := reward.Load(ctxStore, rewardActor)
	if err != nil {
		return types.NewInt(0), err
	}

	networkQAPower, err := powerState.TotalPowerSmoothed()
	if err != nil {
		return types.NewInt(0), err
	}

	verifiedWeight := big.Mul(big.NewIntUnsigned(verifiedSize), big.NewInt(int64(sectorDuration)))
	sectorWeight := builtin.QAPowerForWeight(sectorSize, sectorDuration, verifiedWeight)

	thisEpochBaselinePower, err := rewardState.(interface {
		ThisEpochBaselinePower() (abi.StoragePower, error)
	}).ThisEpochBaselinePower()
	if err != nil {
		return types.NewInt(0), err
	}
	thisEpochRewardSmoothed, err := rewardState.(interface {
		ThisEpochRewardSmoothed() (builtin.FilterEstimate, error)
	}).ThisEpochRewardSmoothed()
	if err != nil {
		return types.NewInt(0), err
	}

	rewardEstimate := smoothing14.FilterEstimate{
		PositionEstimate: thisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: thisEpochRewardSmoothed.VelocityEstimate,
	}
	networkQAPowerEstimate := smoothing14.FilterEstimate{
		PositionEstimate: networkQAPower.PositionEstimate,
		VelocityEstimate: networkQAPower.VelocityEstimate,
	}

	initialPledge := miner14.InitialPledgeForPower(
		sectorWeight,
		thisEpochBaselinePower,
		rewardEstimate,
		networkQAPowerEstimate,
		circSupply.FilCirculating,
	)

	var initialPledgeNum = types.NewInt(110)
	var initialPledgeDen = types.NewInt(100)

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen), nil
}

// postFip0081StateMinerInitialPledgeForSector should be the same calculation as StateMinerInitialPledgeForSector,
// it's here for sanity checking.
func postFip0081StateMinerInitialPledgeForSector(
	ctx context.Context,
	client api.FullNode,
	sectorDuration abi.ChainEpoch,
	sectorSize abi.SectorSize,
	verifiedSize uint64,
	ts *types.TipSet,
) (types.BigInt, error) {
	bs := blockstore.NewAPIBlockstore(client)
	ctxStore := gststore.WrapBlockStore(ctx, bs)

	circSupply, err := client.StateVMCirculatingSupplyInternal(ctx, ts.Key())
	if err != nil {
		return types.NewInt(0), err
	}

	powerActor, err := client.StateGetActor(ctx, power.Address, ts.Key())
	if err != nil {
		return types.NewInt(0), err
	}

	powerState, err := power.Load(ctxStore, powerActor)
	if err != nil {
		return types.NewInt(0), err
	}

	rewardActor, err := client.StateGetActor(ctx, reward.Address, ts.Key())
	if err != nil {
		return types.NewInt(0), err
	}

	rewardState, err := reward.Load(ctxStore, rewardActor)
	if err != nil {
		return types.NewInt(0), err
	}

	networkQAPower, err := powerState.TotalPowerSmoothed()
	if err != nil {
		return types.NewInt(0), err
	}

	verifiedWeight := big.Mul(big.NewIntUnsigned(verifiedSize), big.NewInt(int64(sectorDuration)))
	sectorWeight := builtin.QAPowerForWeight(sectorSize, sectorDuration, verifiedWeight)

	thisEpochBaselinePower, err := rewardState.(interface {
		ThisEpochBaselinePower() (abi.StoragePower, error)
	}).ThisEpochBaselinePower()
	if err != nil {
		return types.NewInt(0), err
	}
	thisEpochRewardSmoothed, err := rewardState.(interface {
		ThisEpochRewardSmoothed() (builtin.FilterEstimate, error)
	}).ThisEpochRewardSmoothed()
	if err != nil {
		return types.NewInt(0), err
	}

	rewardEstimate := smoothing15.FilterEstimate{
		PositionEstimate: thisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: thisEpochRewardSmoothed.VelocityEstimate,
	}
	networkQAPowerEstimate := smoothing15.FilterEstimate{
		PositionEstimate: networkQAPower.PositionEstimate,
		VelocityEstimate: networkQAPower.VelocityEstimate,
	}

	initialPledge := miner15.InitialPledgeForPower(
		sectorWeight,
		thisEpochBaselinePower,
		rewardEstimate,
		networkQAPowerEstimate,
		circSupply.FilCirculating,
		int64(ts.Height())-powerState.RampStartEpoch(),
		powerState.RampDurationEpochs(),
	)

	var initialPledgeNum = types.NewInt(110)
	var initialPledgeDen = types.NewInt(100)

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen), nil
}

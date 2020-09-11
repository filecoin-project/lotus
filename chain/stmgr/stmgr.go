package stmgr

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/specs-actors/actors/builtin/power"

	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
)

var log = logging.Logger("statemgr")

type StateManager struct {
	cs *store.ChainStore

	stCache       map[string][]cid.Cid
	compWait      map[string]chan struct{}
	stlk          sync.Mutex
	genesisMsigLk sync.Mutex
	newVM         func(*vm.VMOpts) (*vm.VM, error)
	genInfo       *genesisInfo
}

func NewStateManager(cs *store.ChainStore) *StateManager {
	return &StateManager{
		newVM:    vm.NewVM,
		cs:       cs,
		stCache:  make(map[string][]cid.Cid),
		compWait: make(map[string]chan struct{}),
	}
}

func cidsToKey(cids []cid.Cid) string {
	var out string
	for _, c := range cids {
		out += c.KeyString()
	}
	return out
}

func (sm *StateManager) TipSetState(ctx context.Context, ts *types.TipSet) (st cid.Cid, rec cid.Cid, err error) {
	ctx, span := trace.StartSpan(ctx, "tipSetState")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(trace.StringAttribute("tipset", fmt.Sprint(ts.Cids())))
	}

	ck := cidsToKey(ts.Cids())
	sm.stlk.Lock()
	cw, cwok := sm.compWait[ck]
	if cwok {
		sm.stlk.Unlock()
		span.AddAttributes(trace.BoolAttribute("waited", true))
		select {
		case <-cw:
			sm.stlk.Lock()
		case <-ctx.Done():
			return cid.Undef, cid.Undef, ctx.Err()
		}
	}
	cached, ok := sm.stCache[ck]
	if ok {
		sm.stlk.Unlock()
		span.AddAttributes(trace.BoolAttribute("cache", true))
		return cached[0], cached[1], nil
	}
	ch := make(chan struct{})
	sm.compWait[ck] = ch

	defer func() {
		sm.stlk.Lock()
		delete(sm.compWait, ck)
		if st != cid.Undef {
			sm.stCache[ck] = []cid.Cid{st, rec}
		}
		sm.stlk.Unlock()
		close(ch)
	}()

	sm.stlk.Unlock()

	if ts.Height() == 0 {
		// NB: This is here because the process that executes blocks requires that the
		// block miner reference a valid miner in the state tree. Unless we create some
		// magical genesis miner, this won't work properly, so we short circuit here
		// This avoids the question of 'who gets paid the genesis block reward'
		return ts.Blocks()[0].ParentStateRoot, ts.Blocks()[0].ParentMessageReceipts, nil
	}

	st, rec, err = sm.computeTipSetState(ctx, ts, nil)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}

	return st, rec, nil
}

func (sm *StateManager) ExecutionTrace(ctx context.Context, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error) {
	var trace []*api.InvocResult
	st, _, err := sm.computeTipSetState(ctx, ts, func(mcid cid.Cid, msg *types.Message, ret *vm.ApplyRet) error {
		ir := &api.InvocResult{
			Msg:            msg,
			MsgRct:         &ret.MessageReceipt,
			ExecutionTrace: ret.ExecutionTrace,
			Duration:       ret.Duration,
		}
		if ret.ActorErr != nil {
			ir.Error = ret.ActorErr.Error()
		}
		trace = append(trace, ir)
		return nil
	})
	if err != nil {
		return cid.Undef, nil, err
	}

	return st, trace, nil
}

type ExecCallback func(cid.Cid, *types.Message, *vm.ApplyRet) error

func (sm *StateManager) ApplyBlocks(ctx context.Context, parentEpoch abi.ChainEpoch, pstate cid.Cid, bms []store.BlockMessages, epoch abi.ChainEpoch, r vm.Rand, cb ExecCallback, baseFee abi.TokenAmount, ts *types.TipSet) (cid.Cid, cid.Cid, error) {

	vmopt := &vm.VMOpts{
		StateBase:      pstate,
		Epoch:          epoch,
		Rand:           r,
		Bstore:         sm.cs.Blockstore(),
		Syscalls:       sm.cs.VMSys(),
		CircSupplyCalc: sm.GetCirculatingSupply,
		NtwkVersion:    sm.GetNtwkVersion,
		BaseFee:        baseFee,
	}

	vmi, err := sm.newVM(vmopt)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("instantiating VM failed: %w", err)
	}

	runCron := func() error {
		// TODO: this nonce-getting is a tiny bit ugly
		ca, err := vmi.StateTree().GetActor(builtin.SystemActorAddr)
		if err != nil {
			return err
		}

		cronMsg := &types.Message{
			To:         builtin.CronActorAddr,
			From:       builtin.SystemActorAddr,
			Nonce:      ca.Nonce,
			Value:      types.NewInt(0),
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
			GasLimit:   build.BlockGasLimit * 10000, // Make super sure this is never too little
			Method:     builtin.MethodsCron.EpochTick,
			Params:     nil,
		}
		ret, err := vmi.ApplyImplicitMessage(ctx, cronMsg)
		if err != nil {
			return err
		}
		if cb != nil {
			if err := cb(cronMsg.Cid(), cronMsg, ret); err != nil {
				return xerrors.Errorf("callback failed on cron message: %w", err)
			}
		}
		if ret.ExitCode != 0 {
			return xerrors.Errorf("CheckProofSubmissions exit was non-zero: %d", ret.ExitCode)
		}

		return nil
	}

	for i := parentEpoch; i < epoch; i++ {
		// handle state forks
		err = sm.handleStateForks(ctx, vmi.StateTree(), i, ts)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("error handling state forks: %w", err)
		}

		if i > parentEpoch {
			// run cron for null rounds if any
			if err := runCron(); err != nil {
				return cid.Cid{}, cid.Cid{}, err
			}
		}

		vmi.SetBlockHeight(i + 1)
	}

	var receipts []cbg.CBORMarshaler
	processedMsgs := map[cid.Cid]bool{}
	for _, b := range bms {
		penalty := types.NewInt(0)
		gasReward := big.Zero()

		for _, cm := range append(b.BlsMessages, b.SecpkMessages...) {
			m := cm.VMMessage()
			if _, found := processedMsgs[m.Cid()]; found {
				continue
			}
			r, err := vmi.ApplyMessage(ctx, cm)
			if err != nil {
				return cid.Undef, cid.Undef, err
			}

			receipts = append(receipts, &r.MessageReceipt)
			gasReward = big.Add(gasReward, r.MinerTip)
			penalty = big.Add(penalty, r.Penalty)

			if cb != nil {
				if err := cb(cm.Cid(), m, r); err != nil {
					return cid.Undef, cid.Undef, err
				}
			}
			processedMsgs[m.Cid()] = true
		}

		var err error
		params, err := actors.SerializeParams(&reward.AwardBlockRewardParams{
			Miner:     b.Miner,
			Penalty:   penalty,
			GasReward: gasReward,
			WinCount:  b.WinCount,
		})
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to serialize award params: %w", err)
		}

		sysAct, err := vmi.StateTree().GetActor(builtin.SystemActorAddr)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
		}

		rwMsg := &types.Message{
			From:       builtin.SystemActorAddr,
			To:         builtin.RewardActorAddr,
			Nonce:      sysAct.Nonce,
			Value:      types.NewInt(0),
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
			GasLimit:   1 << 30,
			Method:     builtin.MethodsReward.AwardBlockReward,
			Params:     params,
		}
		ret, err := vmi.ApplyImplicitMessage(ctx, rwMsg)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to apply reward message for miner %s: %w", b.Miner, err)
		}
		if cb != nil {
			if err := cb(rwMsg.Cid(), rwMsg, ret); err != nil {
				return cid.Undef, cid.Undef, xerrors.Errorf("callback failed on reward message: %w", err)
			}
		}

		if ret.ExitCode != 0 {
			return cid.Undef, cid.Undef, xerrors.Errorf("reward application message failed (exit %d): %s", ret.ExitCode, ret.ActorErr)
		}
	}

	if err := runCron(); err != nil {
		return cid.Cid{}, cid.Cid{}, err
	}

	rectarr := adt.MakeEmptyArray(sm.cs.Store(ctx))
	for i, receipt := range receipts {
		if err := rectarr.Set(uint64(i), receipt); err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to build receipts amt: %w", err)
		}
	}
	rectroot, err := rectarr.Root()
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("failed to build receipts amt: %w", err)
	}

	st, err := vmi.Flush(ctx)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("vm flush failed: %w", err)
	}

	return st, rectroot, nil
}

func (sm *StateManager) computeTipSetState(ctx context.Context, ts *types.TipSet, cb ExecCallback) (cid.Cid, cid.Cid, error) {
	ctx, span := trace.StartSpan(ctx, "computeTipSetState")
	defer span.End()

	blks := ts.Blocks()

	for i := 0; i < len(blks); i++ {
		for j := i + 1; j < len(blks); j++ {
			if blks[i].Miner == blks[j].Miner {
				return cid.Undef, cid.Undef,
					xerrors.Errorf("duplicate miner in a tipset (%s %s)",
						blks[i].Miner, blks[j].Miner)
			}
		}
	}

	var parentEpoch abi.ChainEpoch
	pstate := blks[0].ParentStateRoot
	if blks[0].Height > 0 {
		parent, err := sm.cs.GetBlock(blks[0].Parents[0])
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("getting parent block: %w", err)
		}

		parentEpoch = parent.Height
	}

	cids := make([]cid.Cid, len(blks))
	for i, v := range blks {
		cids[i] = v.Cid()
	}

	r := store.NewChainRand(sm.cs, cids)

	blkmsgs, err := sm.cs.BlockMsgsForTipset(ts)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("getting block messages for tipset: %w", err)
	}

	baseFee := blks[0].ParentBaseFee

	return sm.ApplyBlocks(ctx, parentEpoch, pstate, blkmsgs, blks[0].Height, r, cb, baseFee, ts)
}

func (sm *StateManager) parentState(ts *types.TipSet) cid.Cid {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	return ts.ParentState()
}

func (sm *StateManager) ChainStore() *store.ChainStore {
	return sm.cs
}

// ResolveToKeyAddress is similar to `vm.ResolveToKeyAddr` but does not allow `Actor` type of addresses.
// Uses the `TipSet` `ts` to generate the VM state.
func (sm *StateManager) ResolveToKeyAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	switch addr.Protocol() {
	case address.BLS, address.SECP256K1:
		return addr, nil
	case address.Actor:
		return address.Undef, xerrors.New("cannot resolve actor address to key address")
	default:
	}

	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	st, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return address.Undef, xerrors.Errorf("resolve address failed to get tipset state: %w", err)
	}

	cst := cbor.NewCborStore(sm.cs.Blockstore())
	tree, err := state.LoadStateTree(cst, st)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to load state tree")
	}

	return vm.ResolveToKeyAddr(tree, cst, addr)
}

func (sm *StateManager) GetBlsPublicKey(ctx context.Context, addr address.Address, ts *types.TipSet) (pubk []byte, err error) {
	kaddr, err := sm.ResolveToKeyAddress(ctx, addr, ts)
	if err != nil {
		return pubk, xerrors.Errorf("failed to resolve address to key address: %w", err)
	}

	if kaddr.Protocol() != address.BLS {
		return pubk, xerrors.Errorf("address must be BLS address to load bls public key")
	}

	return kaddr.Payload(), nil
}

func (sm *StateManager) LookupID(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	cst := cbor.NewCborStore(sm.cs.Blockstore())
	state, err := state.LoadStateTree(cst, sm.parentState(ts))
	if err != nil {
		return address.Undef, xerrors.Errorf("load state tree: %w", err)
	}
	return state.LookupID(addr)
}

func (sm *StateManager) GetReceipt(ctx context.Context, msg cid.Cid, ts *types.TipSet) (*types.MessageReceipt, error) {
	m, err := sm.cs.GetCMessage(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to load message: %w", err)
	}

	r, _, err := sm.tipsetExecutedMessage(ts, msg, m.VMMessage())
	if err != nil {
		return nil, err
	}

	if r != nil {
		return r, nil
	}

	_, r, _, err = sm.searchBackForMsg(ctx, ts, m)
	if err != nil {
		return nil, fmt.Errorf("failed to look back through chain for message: %w", err)
	}

	return r, nil
}

// WaitForMessage blocks until a message appears on chain. It looks backwards in the chain to see if this has already
// happened. It guarantees that the message has been on chain for at least confidence epochs without being reverted
// before returning.
func (sm *StateManager) WaitForMessage(ctx context.Context, mcid cid.Cid, confidence uint64) (*types.TipSet, *types.MessageReceipt, cid.Cid, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	msg, err := sm.cs.GetCMessage(mcid)
	if err != nil {
		return nil, nil, cid.Undef, fmt.Errorf("failed to load message: %w", err)
	}

	tsub := sm.cs.SubHeadChanges(ctx)

	head, ok := <-tsub
	if !ok {
		return nil, nil, cid.Undef, fmt.Errorf("SubHeadChanges stream was invalid")
	}

	if len(head) != 1 {
		return nil, nil, cid.Undef, fmt.Errorf("SubHeadChanges first entry should have been one item")
	}

	if head[0].Type != store.HCCurrent {
		return nil, nil, cid.Undef, fmt.Errorf("expected current head on SHC stream (got %s)", head[0].Type)
	}

	r, foundMsg, err := sm.tipsetExecutedMessage(head[0].Val, mcid, msg.VMMessage())
	if err != nil {
		return nil, nil, cid.Undef, err
	}

	if r != nil {
		return head[0].Val, r, foundMsg, nil
	}

	var backTs *types.TipSet
	var backRcp *types.MessageReceipt
	var backFm cid.Cid
	backSearchWait := make(chan struct{})
	go func() {
		fts, r, foundMsg, err := sm.searchBackForMsg(ctx, head[0].Val, msg)
		if err != nil {
			log.Warnf("failed to look back through chain for message: %w", err)
			return
		}

		backTs = fts
		backRcp = r
		backFm = foundMsg
		close(backSearchWait)
	}()

	var candidateTs *types.TipSet
	var candidateRcp *types.MessageReceipt
	var candidateFm cid.Cid
	heightOfHead := head[0].Val.Height()
	reverts := map[types.TipSetKey]bool{}

	for {
		select {
		case notif, ok := <-tsub:
			if !ok {
				return nil, nil, cid.Undef, ctx.Err()
			}
			for _, val := range notif {
				switch val.Type {
				case store.HCRevert:
					if val.Val.Equals(candidateTs) {
						candidateTs = nil
						candidateRcp = nil
						candidateFm = cid.Undef
					}
					if backSearchWait != nil {
						reverts[val.Val.Key()] = true
					}
				case store.HCApply:
					if candidateTs != nil && val.Val.Height() >= candidateTs.Height()+abi.ChainEpoch(confidence) {
						return candidateTs, candidateRcp, candidateFm, nil
					}
					r, foundMsg, err := sm.tipsetExecutedMessage(val.Val, mcid, msg.VMMessage())
					if err != nil {
						return nil, nil, cid.Undef, err
					}
					if r != nil {
						if confidence == 0 {
							return val.Val, r, foundMsg, err
						}
						candidateTs = val.Val
						candidateRcp = r
						candidateFm = foundMsg
					}
					heightOfHead = val.Val.Height()
				}
			}
		case <-backSearchWait:
			// check if we found the message in the chain and that is hasn't been reverted since we started searching
			if backTs != nil && !reverts[backTs.Key()] {
				// if head is at or past confidence interval, return immediately
				if heightOfHead >= backTs.Height()+abi.ChainEpoch(confidence) {
					return backTs, backRcp, backFm, nil
				}

				// wait for confidence interval
				candidateTs = backTs
				candidateRcp = backRcp
				candidateFm = backFm
			}
			reverts = nil
			backSearchWait = nil
		case <-ctx.Done():
			return nil, nil, cid.Undef, ctx.Err()
		}
	}
}

func (sm *StateManager) SearchForMessage(ctx context.Context, mcid cid.Cid) (*types.TipSet, *types.MessageReceipt, cid.Cid, error) {
	msg, err := sm.cs.GetCMessage(mcid)
	if err != nil {
		return nil, nil, cid.Undef, fmt.Errorf("failed to load message: %w", err)
	}

	head := sm.cs.GetHeaviestTipSet()

	r, foundMsg, err := sm.tipsetExecutedMessage(head, mcid, msg.VMMessage())
	if err != nil {
		return nil, nil, cid.Undef, err
	}

	if r != nil {
		return head, r, foundMsg, nil
	}

	fts, r, foundMsg, err := sm.searchBackForMsg(ctx, head, msg)

	if err != nil {
		log.Warnf("failed to look back through chain for message %s", mcid)
		return nil, nil, cid.Undef, err
	}

	if fts == nil {
		return nil, nil, cid.Undef, nil
	}

	return fts, r, foundMsg, nil
}

func (sm *StateManager) searchBackForMsg(ctx context.Context, from *types.TipSet, m types.ChainMsg) (*types.TipSet, *types.MessageReceipt, cid.Cid, error) {

	cur := from
	for {
		if cur.Height() == 0 {
			// it ain't here!
			return nil, nil, cid.Undef, nil
		}

		select {
		case <-ctx.Done():
			return nil, nil, cid.Undef, nil
		default:
		}

		var act types.Actor
		err := sm.WithParentState(cur, sm.WithActor(m.VMMessage().From, GetActor(&act)))
		if err != nil {
			return nil, nil, cid.Undef, err
		}

		// we either have no messages from the sender, or the latest message we found has a lower nonce than the one being searched for,
		// either way, no reason to lookback, it ain't there
		if act.Nonce == 0 || act.Nonce < m.VMMessage().Nonce {
			return nil, nil, cid.Undef, nil
		}

		ts, err := sm.cs.LoadTipSet(cur.Parents())
		if err != nil {
			return nil, nil, cid.Undef, fmt.Errorf("failed to load tipset during msg wait searchback: %w", err)
		}

		r, foundMsg, err := sm.tipsetExecutedMessage(ts, m.Cid(), m.VMMessage())
		if err != nil {
			return nil, nil, cid.Undef, fmt.Errorf("checking for message execution during lookback: %w", err)
		}

		if r != nil {
			return ts, r, foundMsg, nil
		}

		cur = ts
	}
}

func (sm *StateManager) tipsetExecutedMessage(ts *types.TipSet, msg cid.Cid, vmm *types.Message) (*types.MessageReceipt, cid.Cid, error) {
	// The genesis block did not execute any messages
	if ts.Height() == 0 {
		return nil, cid.Undef, nil
	}

	pts, err := sm.cs.LoadTipSet(ts.Parents())
	if err != nil {
		return nil, cid.Undef, err
	}

	cm, err := sm.cs.MessagesForTipset(pts)
	if err != nil {
		return nil, cid.Undef, err
	}

	for ii := range cm {
		// iterate in reverse because we going backwards through the chain
		i := len(cm) - ii - 1
		m := cm[i]

		if m.VMMessage().From == vmm.From { // cheaper to just check origin first
			if m.VMMessage().Nonce == vmm.Nonce {
				if m.VMMessage().EqualCall(vmm) {
					if m.Cid() != msg {
						log.Warnw("found message with equal nonce and call params but different CID",
							"wanted", msg, "found", m.Cid(), "nonce", vmm.Nonce, "from", vmm.From)
					}

					pr, err := sm.cs.GetParentReceipt(ts.Blocks()[0], i)
					if err != nil {
						return nil, cid.Undef, err
					}
					return pr, m.Cid(), nil
				}

				// this should be that message
				return nil, cid.Undef, xerrors.Errorf("found message with equal nonce as the one we are looking for (F:%s n %d, TS: %s n%d)",
					msg, vmm.Nonce, m.Cid(), m.VMMessage().Nonce)
			}
			if m.VMMessage().Nonce < vmm.Nonce {
				return nil, cid.Undef, nil // don't bother looking further
			}
		}
	}

	return nil, cid.Undef, nil
}

func (sm *StateManager) ListAllActors(ctx context.Context, ts *types.TipSet) ([]address.Address, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}
	st, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return nil, err
	}

	r, err := adt.AsMap(sm.cs.Store(ctx), st)
	if err != nil {
		return nil, err
	}

	var out []address.Address
	err = r.ForEach(nil, func(k string) error {
		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return xerrors.Errorf("address in state tree was not valid: %w", err)
		}
		out = append(out, addr)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (sm *StateManager) MarketBalance(ctx context.Context, addr address.Address, ts *types.TipSet) (api.MarketBalance, error) {
	var state market.State
	_, err := sm.LoadActorState(ctx, builtin.StorageMarketActorAddr, &state, ts)
	if err != nil {
		return api.MarketBalance{}, err
	}

	addr, err = sm.LookupID(ctx, addr, ts)
	if err != nil {
		return api.MarketBalance{}, err
	}

	var out api.MarketBalance

	et, err := adt.AsBalanceTable(sm.cs.Store(ctx), state.EscrowTable)
	if err != nil {
		return api.MarketBalance{}, err
	}
	out.Escrow, err = et.Get(addr)
	if err != nil {
		return api.MarketBalance{}, xerrors.Errorf("getting escrow balance: %w", err)
	}

	lt, err := adt.AsBalanceTable(sm.cs.Store(ctx), state.LockedTable)
	if err != nil {
		return api.MarketBalance{}, err
	}
	out.Locked, err = lt.Get(addr)
	if err != nil {
		return api.MarketBalance{}, xerrors.Errorf("getting locked balance: %w", err)
	}

	return out, nil
}

func (sm *StateManager) ValidateChain(ctx context.Context, ts *types.TipSet) error {
	tschain := []*types.TipSet{ts}
	for ts.Height() != 0 {
		next, err := sm.cs.LoadTipSet(ts.Parents())
		if err != nil {
			return err
		}

		tschain = append(tschain, next)
		ts = next
	}

	lastState := tschain[len(tschain)-1].ParentState()
	for i := len(tschain) - 1; i >= 0; i-- {
		cur := tschain[i]
		log.Infof("computing state (height: %d, ts=%s)", cur.Height(), cur.Cids())
		if cur.ParentState() != lastState {
			return xerrors.Errorf("tipset chain had state mismatch at height %d", cur.Height())
		}
		st, _, err := sm.TipSetState(ctx, cur)
		if err != nil {
			return err
		}
		lastState = st
	}

	return nil
}

func (sm *StateManager) SetVMConstructor(nvm func(*vm.VMOpts) (*vm.VM, error)) {
	sm.newVM = nvm
}

type genesisInfo struct {
	genesisMsigs []multisig.State
	// info about the Accounts in the genesis state
	genesisActors      []genesisActor
	genesisPledge      abi.TokenAmount
	genesisMarketFunds abi.TokenAmount
}

type genesisActor struct {
	addr    address.Address
	initBal abi.TokenAmount
}

// sets up information about the actors in the genesis state
func (sm *StateManager) setupGenesisActors(ctx context.Context) error {

	gi := genesisInfo{}

	gb, err := sm.cs.GetGenesis()
	if err != nil {
		return xerrors.Errorf("getting genesis block: %w", err)
	}

	gts, err := types.NewTipSet([]*types.BlockHeader{gb})
	if err != nil {
		return xerrors.Errorf("getting genesis tipset: %w", err)
	}

	st, _, err := sm.TipSetState(ctx, gts)
	if err != nil {
		return xerrors.Errorf("getting genesis tipset state: %w", err)
	}

	cst := cbor.NewCborStore(sm.cs.Blockstore())
	sTree, err := state.LoadStateTree(cst, st)
	if err != nil {
		return xerrors.Errorf("loading state tree: %w", err)
	}

	gi.genesisMarketFunds, err = getFilMarketLocked(ctx, sTree)
	if err != nil {
		return xerrors.Errorf("setting up genesis market funds: %w", err)
	}

	gi.genesisPledge, err = getFilPowerLocked(ctx, sTree)
	if err != nil {
		return xerrors.Errorf("setting up genesis pledge: %w", err)
	}

	r, err := adt.AsMap(sm.cs.Store(ctx), st)
	if err != nil {
		return xerrors.Errorf("getting genesis actors: %w", err)
	}

	totalsByEpoch := make(map[abi.ChainEpoch]abi.TokenAmount)
	var act types.Actor
	err = r.ForEach(&act, func(k string) error {
		if act.Code == builtin.MultisigActorCodeID {
			var s multisig.State
			err := sm.cs.Store(ctx).Get(ctx, act.Head, &s)
			if err != nil {
				return err
			}

			if s.StartEpoch != 0 {
				return xerrors.New("genesis multisig doesn't start vesting at epoch 0!")
			}

			ot, f := totalsByEpoch[s.UnlockDuration]
			if f {
				totalsByEpoch[s.UnlockDuration] = big.Add(ot, s.InitialBalance)
			} else {
				totalsByEpoch[s.UnlockDuration] = s.InitialBalance
			}

		} else if act.Code == builtin.AccountActorCodeID {
			// should exclude burnt funds actor and "remainder account actor"
			// should only ever be "faucet" accounts in testnets
			kaddr, err := address.NewFromBytes([]byte(k))
			if err != nil {
				return xerrors.Errorf("decoding address: %w", err)
			}

			if kaddr != builtin.BurntFundsActorAddr {
				kid, err := sTree.LookupID(kaddr)
				if err != nil {
					return xerrors.Errorf("resolving address: %w", err)
				}

				gi.genesisActors = append(gi.genesisActors, genesisActor{
					addr:    kid,
					initBal: act.Balance,
				})
			}
		}
		return nil
	})

	if err != nil {
		return xerrors.Errorf("error setting up genesis infos: %w", err)
	}

	gi.genesisMsigs = make([]multisig.State, 0, len(totalsByEpoch))
	for k, v := range totalsByEpoch {
		ns := multisig.State{
			InitialBalance: v,
			UnlockDuration: k,
			PendingTxns:    cid.Undef,
		}
		gi.genesisMsigs = append(gi.genesisMsigs, ns)
	}

	sm.genInfo = &gi

	return nil
}

// sets up information about the actors in the genesis state
// For testnet we use a hardcoded set of multisig states, instead of what's actually in the genesis multisigs
// We also do not consider ANY account actors (including the faucet)
func (sm *StateManager) setupGenesisActorsTestnet(ctx context.Context) error {

	gi := genesisInfo{}

	gb, err := sm.cs.GetGenesis()
	if err != nil {
		return xerrors.Errorf("getting genesis block: %w", err)
	}

	gts, err := types.NewTipSet([]*types.BlockHeader{gb})
	if err != nil {
		return xerrors.Errorf("getting genesis tipset: %w", err)
	}

	st, _, err := sm.TipSetState(ctx, gts)
	if err != nil {
		return xerrors.Errorf("getting genesis tipset state: %w", err)
	}

	cst := cbor.NewCborStore(sm.cs.Blockstore())
	sTree, err := state.LoadStateTree(cst, st)
	if err != nil {
		return xerrors.Errorf("loading state tree: %w", err)
	}

	gi.genesisMarketFunds, err = getFilMarketLocked(ctx, sTree)
	if err != nil {
		return xerrors.Errorf("setting up genesis market funds: %w", err)
	}

	gi.genesisPledge, err = getFilPowerLocked(ctx, sTree)
	if err != nil {
		return xerrors.Errorf("setting up genesis pledge: %w", err)
	}

	totalsByEpoch := make(map[abi.ChainEpoch]abi.TokenAmount)

	// 6 months
	sixMonths := abi.ChainEpoch(183 * builtin.EpochsInDay)
	totalsByEpoch[sixMonths] = big.NewInt(49_929_341)
	totalsByEpoch[sixMonths] = big.Add(totalsByEpoch[sixMonths], big.NewInt(32_787_700))

	// 1 year
	oneYear := abi.ChainEpoch(365 * builtin.EpochsInDay)
	totalsByEpoch[oneYear] = big.NewInt(22_421_712)

	// 2 years
	twoYears := abi.ChainEpoch(2 * 365 * builtin.EpochsInDay)
	totalsByEpoch[twoYears] = big.NewInt(7_223_364)

	// 3 years
	threeYears := abi.ChainEpoch(3 * 365 * builtin.EpochsInDay)
	totalsByEpoch[threeYears] = big.NewInt(87_637_883)

	// 6 years
	sixYears := abi.ChainEpoch(6 * 365 * builtin.EpochsInDay)
	totalsByEpoch[sixYears] = big.NewInt(100_000_000)
	totalsByEpoch[sixYears] = big.Add(totalsByEpoch[sixYears], big.NewInt(300_000_000))

	gi.genesisMsigs = make([]multisig.State, 0, len(totalsByEpoch))
	for k, v := range totalsByEpoch {
		ns := multisig.State{
			InitialBalance: v,
			UnlockDuration: k,
			PendingTxns:    cid.Undef,
		}
		gi.genesisMsigs = append(gi.genesisMsigs, ns)
	}

	sm.genInfo = &gi

	return nil
}

// GetVestedFunds returns all funds that have "left" actors that are in the genesis state:
// - For Multisigs, it counts the actual amounts that have vested at the given epoch
// - For Accounts, it counts max(currentBalance - genesisBalance, 0).
func (sm *StateManager) GetFilVested(ctx context.Context, height abi.ChainEpoch, st *state.StateTree) (abi.TokenAmount, error) {
	vf := big.Zero()
	for _, v := range sm.genInfo.genesisMsigs {
		au := big.Sub(v.InitialBalance, v.AmountLocked(height))
		vf = big.Add(vf, au)
	}

	// there should not be any such accounts in testnet (and also none in mainnet?)
	for _, v := range sm.genInfo.genesisActors {
		act, err := st.GetActor(v.addr)
		if err != nil {
			return big.Zero(), xerrors.Errorf("failed to get actor: %w", err)
		}

		diff := big.Sub(v.initBal, act.Balance)
		if diff.GreaterThan(big.Zero()) {
			vf = big.Add(vf, diff)
		}
	}

	vf = big.Add(vf, sm.genInfo.genesisPledge)
	vf = big.Add(vf, sm.genInfo.genesisMarketFunds)

	return vf, nil
}

func GetFilMined(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	ractor, err := st.GetActor(builtin.RewardActorAddr)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load reward actor state: %w", err)
	}

	var rst reward.State
	if err := st.Store.Get(ctx, ractor.Head, &rst); err != nil {
		return big.Zero(), xerrors.Errorf("failed to load reward state: %w", err)
	}

	return rst.TotalMined, nil
}

func getFilMarketLocked(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	mactor, err := st.GetActor(builtin.StorageMarketActorAddr)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load market actor: %w", err)
	}

	var mst market.State
	if err := st.Store.Get(ctx, mactor.Head, &mst); err != nil {
		return big.Zero(), xerrors.Errorf("failed to load market state: %w", err)
	}

	fml := types.BigAdd(mst.TotalClientLockedCollateral, mst.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, mst.TotalClientStorageFee)
	return fml, nil
}

func getFilPowerLocked(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	pactor, err := st.GetActor(builtin.StoragePowerActorAddr)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load power actor: %w", err)
	}

	var pst power.State
	if err := st.Store.Get(ctx, pactor.Head, &pst); err != nil {
		return big.Zero(), xerrors.Errorf("failed to load power state: %w", err)
	}
	return pst.TotalPledgeCollateral, nil
}

func (sm *StateManager) GetFilLocked(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {

	filMarketLocked, err := getFilMarketLocked(ctx, st)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to get filMarketLocked: %w", err)
	}

	filPowerLocked, err := getFilPowerLocked(ctx, st)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to get filPowerLocked: %w", err)
	}

	return types.BigAdd(filMarketLocked, filPowerLocked), nil
}

func GetFilBurnt(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	burnt, err := st.GetActor(builtin.BurntFundsActorAddr)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load burnt actor: %w", err)
	}

	return burnt.Balance, nil
}

func (sm *StateManager) GetCirculatingSupplyDetailed(ctx context.Context, height abi.ChainEpoch, st *state.StateTree) (api.CirculatingSupply, error) {
	sm.genesisMsigLk.Lock()
	defer sm.genesisMsigLk.Unlock()
	if sm.genInfo == nil {
		err := sm.setupGenesisActorsTestnet(ctx)
		if err != nil {
			return api.CirculatingSupply{}, xerrors.Errorf("failed to setup genesis information: %w", err)
		}
	}

	filVested, err := sm.GetFilVested(ctx, height, st)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filVested: %w", err)
	}

	filMined, err := GetFilMined(ctx, st)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filMined: %w", err)
	}

	filBurnt, err := GetFilBurnt(ctx, st)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filBurnt: %w", err)
	}

	filLocked, err := sm.GetFilLocked(ctx, st)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filLocked: %w", err)
	}

	ret := types.BigAdd(filVested, filMined)
	ret = types.BigSub(ret, filBurnt)
	ret = types.BigSub(ret, filLocked)

	if ret.LessThan(big.Zero()) {
		ret = big.Zero()
	}

	return api.CirculatingSupply{
		FilVested:      filVested,
		FilMined:       filMined,
		FilBurnt:       filBurnt,
		FilLocked:      filLocked,
		FilCirculating: ret,
	}, nil
}

func (sm *StateManager) GetCirculatingSupply(ctx context.Context, height abi.ChainEpoch, st *state.StateTree) (abi.TokenAmount, error) {
	csi, err := sm.GetCirculatingSupplyDetailed(ctx, height, st)
	if err != nil {
		return big.Zero(), err
	}

	return csi.FilCirculating, nil
}

func (sm *StateManager) GetNtwkVersion(ctx context.Context, height abi.ChainEpoch) network.Version {
	if build.UseNewestNetwork() {
		return build.NewestNetworkVersion
	}

	if height <= build.UpgradeBreezeHeight {
		return network.Version0
	}

	if height <= build.UpgradeSmokeHeight {
		return network.Version1
	}

	return build.NewestNetworkVersion
}

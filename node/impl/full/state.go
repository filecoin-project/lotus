package full

import (
	"context"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/bufbstore"
)

type StateAPI struct {
	fx.In

	// TODO: the wallet here is only needed because we have the MinerCreateBlock
	// API attached to the state API. It probably should live somewhere better
	Wallet *wallet.Wallet

	StateManager *stmgr.StateManager
	Chain        *store.ChainStore
}

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address) ([]*api.SectorInfo, error) {
	return stmgr.GetMinerSectorSet(ctx, a.StateManager, nil, addr)
}

func (a *StateAPI) StateMinerProvingSet(ctx context.Context, addr address.Address, ts *types.TipSet) ([]*api.SectorInfo, error) {
	return stmgr.GetMinerProvingSet(ctx, a.StateManager, ts, addr)
}

func (a *StateAPI) StateMinerPower(ctx context.Context, maddr address.Address, ts *types.TipSet) (api.MinerPower, error) {
	mpow, tpow, err := stmgr.GetPower(ctx, a.StateManager, ts, maddr)
	if err != nil {
		return api.MinerPower{}, err
	}

	return api.MinerPower{
		MinerPower: mpow,
		TotalPower: tpow,
	}, nil
}

func (a *StateAPI) StateMinerWorker(ctx context.Context, m address.Address, ts *types.TipSet) (address.Address, error) {
	ret, err := a.StateManager.Call(ctx, &types.Message{
		From:   m,
		To:     m,
		Method: actors.MAMethods.GetWorkerAddr,
	}, ts)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get miner worker addr: %w", err)
	}

	if ret.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("failed to get miner worker addr (exit code %d)", ret.ExitCode)
	}

	w, err := address.NewFromBytes(ret.Return)
	if err != nil {
		return address.Undef, xerrors.Errorf("GetWorkerAddr returned malformed address: %w", err)
	}

	return w, nil
}

func (a *StateAPI) StateMinerPeerID(ctx context.Context, m address.Address, ts *types.TipSet) (peer.ID, error) {
	return stmgr.GetMinerPeerID(ctx, a.StateManager, ts, m)
}

func (a *StateAPI) StateMinerProvingPeriodEnd(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error) {
	return stmgr.GetMinerProvingPeriodEnd(ctx, a.StateManager, ts, actor)
}

func (a *StateAPI) StateCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	return a.StateManager.Call(ctx, msg, ts)
}

func (a *StateAPI) StateReplay(ctx context.Context, ts *types.TipSet, mc cid.Cid) (*api.ReplayResults, error) {
	m, r, err := a.StateManager.Replay(ctx, ts, mc)
	if err != nil {
		return nil, err
	}

	var errstr string
	if r.ActorErr != nil {
		errstr = r.ActorErr.Error()
	}

	return &api.ReplayResults{
		Msg:     m,
		Receipt: &r.MessageReceipt,
		Error:   errstr,
	}, nil
}

func (a *StateAPI) stateForTs(ts *types.TipSet) (*state.StateTree, error) {
	if ts == nil {
		ts = a.Chain.GetHeaviestTipSet()
	}

	st, err := a.StateManager.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	buf := bufbstore.NewBufferedBstore(a.Chain.Blockstore())
	cst := hamt.CSTFromBstore(buf)
	return state.LoadStateTree(cst, st)
}

func (a *StateAPI) StateGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error) {
	state, err := a.stateForTs(ts)
	if err != nil {
		return nil, err
	}

	return state.GetActor(actor)
}

func (a *StateAPI) StateReadState(ctx context.Context, act *types.Actor, ts *types.TipSet) (*api.ActorState, error) {
	state, err := a.stateForTs(ts)
	if err != nil {
		return nil, err
	}

	blk, err := state.Store.Blocks.GetBlock(ctx, act.Head)
	if err != nil {
		return nil, err
	}

	oif, err := vm.DumpActorState(act.Code, blk.RawData())
	if err != nil {
		return nil, err
	}

	return &api.ActorState{
		Balance: act.Balance,
		State:   oif,
	}, nil
}

// This is on StateAPI because miner.Miner requires this, and MinerAPI requires miner.Miner
func (a *StateAPI) MinerCreateBlock(ctx context.Context, addr address.Address, parents *types.TipSet, tickets []*types.Ticket, proof types.ElectionProof, msgs []*types.SignedMessage, ts uint64) (*types.BlockMsg, error) {
	fblk, err := gen.MinerCreateBlock(ctx, a.StateManager, a.Wallet, addr, parents, tickets, proof, msgs, ts)
	if err != nil {
		return nil, err
	}

	var out types.BlockMsg
	out.Header = fblk.Header
	for _, msg := range fblk.BlsMessages {
		out.BlsMessages = append(out.BlsMessages, msg.Cid())
	}
	for _, msg := range fblk.SecpkMessages {
		out.SecpkMessages = append(out.SecpkMessages, msg.Cid())
	}

	return &out, nil
}

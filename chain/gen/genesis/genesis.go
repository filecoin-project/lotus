package genesis

import (
	"context"

	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
	actors "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("genesis")

type GenesisBootstrap struct {
	Genesis *types.BlockHeader
}

func MakeGenesisBlock(bs bstore.Blockstore, sys *types.VMSyscalls, balances map[address.Address]types.BigInt, gmcfg *GenMinerCfg, ts uint64) (*GenesisBootstrap, error) {
	ctx := context.TODO()

	state, err := MakeInitialStateTree(bs, balances)
	if err != nil {
		return nil, xerrors.Errorf("make initial state tree failed: %w", err)
	}

	stateroot, err := state.Flush(ctx)
	if err != nil {
		return nil, xerrors.Errorf("flush state tree failed: %w", err)
	}

	// temp chainstore
	cs := store.NewChainStore(bs, datastore.NewMapDatastore(), sys)
	stateroot, deals, err := SetupStorageMiners(ctx, cs, stateroot, gmcfg)
	if err != nil {
		return nil, xerrors.Errorf("setup storage miners failed: %w", err)
	}

	stateroot, err = SetupStorageMarketActor(bs, stateroot, deals)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}

	stateroot, err = AdjustInitActorStartID(ctx, bs, stateroot, 1000)
	if err != nil {
		return nil, xerrors.Errorf("failed to adjust init actor start ID: %w", err)
	}

	cst := cbor.NewCborStore(bs)

	emptyroot, err := amt.FromArray(ctx, cst, nil)
	if err != nil {
		return nil, xerrors.Errorf("amt build failed: %w", err)
	}

	mm := &types.MsgMeta{
		BlsMessages:   emptyroot,
		SecpkMessages: emptyroot,
	}
	mmb, err := mm.ToStorageBlock()
	if err != nil {
		return nil, xerrors.Errorf("serializing msgmeta failed: %w", err)
	}
	if err := bs.Put(mmb); err != nil {
		return nil, xerrors.Errorf("putting msgmeta block to blockstore: %w", err)
	}

	log.Infof("Empty Genesis root: %s", emptyroot)

	genesisticket := &types.Ticket{
		VRFProof: []byte("vrf proof0000000vrf proof0000000"),
	}

	b := &types.BlockHeader{
		Miner:  actors.InitAddress,
		Ticket: genesisticket,
		EPostProof: types.EPostProof{
			Proof:    []byte("not a real proof"),
			PostRand: []byte("i guess this is kinda random"),
		},
		Parents:               []cid.Cid{},
		Height:                0,
		ParentWeight:          types.NewInt(0),
		ParentStateRoot:       stateroot,
		Messages:              mmb.Cid(),
		ParentMessageReceipts: emptyroot,
		BLSAggregate:          types.Signature{Type: types.KTBLS, Data: []byte("signatureeee")},
		BlockSig:              &types.Signature{Type: types.KTBLS, Data: []byte("block signatureeee")},
		Timestamp:             ts,
	}

	sb, err := b.ToStorageBlock()
	if err != nil {
		return nil, xerrors.Errorf("serializing block header failed: %w", err)
	}

	if err := bs.Put(sb); err != nil {
		return nil, xerrors.Errorf("putting header to blockstore: %w", err)
	}

	return &GenesisBootstrap{
		Genesis: b,
	}, nil
}

func MakeInitialStateTree(bs bstore.Blockstore, actmap map[address.Address]types.BigInt) (*state.StateTree, error) {
	cst := cbor.NewCborStore(bs)
	state, err := state.NewStateTree(cst)
	if err != nil {
		return nil, xerrors.Errorf("making new state tree: %w", err)
	}

	emptyobject, err := cst.Put(context.TODO(), []struct{}{})
	if err != nil {
		return nil, xerrors.Errorf("failed putting empty object: %w", err)
	}

	var addrs []address.Address
	for a := range actmap {
		addrs = append(addrs, a)
	}

	initact, err := SetupInitActor(bs, addrs)
	if err != nil {
		return nil, xerrors.Errorf("setup init actor: %w", err)
	}

	if err := state.SetActor(actors.InitAddress, initact); err != nil {
		return nil, xerrors.Errorf("set init actor: %w", err)
	}

	cronact, err := SetupCronActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup cron actor: %w", err)
	}

	if err := state.SetActor(actors.CronAddress, cronact); err != nil {
		return nil, xerrors.Errorf("set cron actor: %w", err)
	}

	spact, err := SetupStoragePowerActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}

	if err := state.SetActor(actors.StoragePowerAddress, spact); err != nil {
		return nil, xerrors.Errorf("set storage market actor: %w", err)
	}

	netAmt := types.FromFil(build.TotalFilecoin)
	for _, amt := range actmap {
		netAmt = types.BigSub(netAmt, amt)
	}

	err = state.SetActor(actors.NetworkAddress, &types.Actor{
		Code:    actors.AccountCodeCid,
		Balance: netAmt,
		Head:    emptyobject,
	})
	if err != nil {
		return nil, xerrors.Errorf("set network account actor: %w", err)
	}

	err = state.SetActor(actors.BurntFundsAddress, &types.Actor{
		Code:    actors.AccountCodeCid,
		Balance: types.NewInt(0),
		Head:    emptyobject,
	})
	if err != nil {
		return nil, xerrors.Errorf("set burnt funds account actor: %w", err)
	}

	for a, v := range actmap {
		err = state.SetActor(a, &types.Actor{
			Code:    actors.AccountCodeCid,
			Balance: v,
			Head:    emptyobject,
		})
		if err != nil {
			return nil, xerrors.Errorf("setting account from actmap: %w", err)
		}
	}

	return state, nil
}

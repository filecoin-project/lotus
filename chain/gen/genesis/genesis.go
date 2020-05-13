package genesis

import (
	"context"
	"encoding/json"

	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
)

const AccountStart = 100
const MinerStart = 1000
const MaxAccounts = MinerStart - AccountStart

var log = logging.Logger("genesis")

type GenesisBootstrap struct {
	Genesis *types.BlockHeader
}

/*
From a list of parameters, create a genesis block / initial state

The process:
- Bootstrap state (MakeInitialStateTree)
  - Create empty state
  - Create system actor
  - Make init actor
    - Create accounts mappings
    - Set NextID to MinerStart
  - Setup Reward (1.4B fil)
  - Setup Cron
  - Create empty power actor
  - Create empty market
  - Create verified registry
  - Setup burnt fund address
  - Initialize account / msig balances
- Instantiate early vm with genesis syscalls
  - Create miners
    - Each:
      - power.CreateMiner, set msg value to PowerBalance
      - market.AddFunds with correct value
      - market.PublishDeals for related sectors
      - Set precommits
      - Commit presealed sectors

Data Types:

PreSeal :{
  CommR    CID
  CommD    CID
  SectorID SectorNumber
  Deal     market.DealProposal # Start at 0, self-deal!
}

Genesis: {
	Accounts: [ # non-miner, non-singleton actors, max len = MaxAccounts
		{
			Type: "account" / "multisig",
			Value: "attofil",
			[Meta: {msig settings, account key..}]
		},...
	],
	Miners: [
		{
			Owner, Worker Addr # ID
			MarketBalance, PowerBalance TokenAmount
			SectorSize uint64
			PreSeals []PreSeal
		},...
	],
}

*/

func MakeInitialStateTree(ctx context.Context, bs bstore.Blockstore, template genesis.Template) (*state.StateTree, error) {
	// Create empty state tree

	cst := cbor.NewCborStore(bs)
	_, err := cst.Put(context.TODO(), []struct{}{})
	if err != nil {
		return nil, xerrors.Errorf("putting empty object: %w", err)
	}

	state, err := state.NewStateTree(cst)
	if err != nil {
		return nil, xerrors.Errorf("making new state tree: %w", err)
	}

	emptyobject, err := cst.Put(context.TODO(), []struct{}{})
	if err != nil {
		return nil, xerrors.Errorf("failed putting empty object: %w", err)
	}

	// Create system actor

	sysact, err := SetupSystemActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup init actor: %w", err)
	}
	if err := state.SetActor(builtin.SystemActorAddr, sysact); err != nil {
		return nil, xerrors.Errorf("set init actor: %w", err)
	}

	// Create init actor

	initact, err := SetupInitActor(bs, template.NetworkName, template.Accounts)
	if err != nil {
		return nil, xerrors.Errorf("setup init actor: %w", err)
	}
	if err := state.SetActor(builtin.InitActorAddr, initact); err != nil {
		return nil, xerrors.Errorf("set init actor: %w", err)
	}

	// Setup reward
	rewact, err := SetupRewardActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup init actor: %w", err)
	}

	err = state.SetActor(builtin.RewardActorAddr, rewact)
	if err != nil {
		return nil, xerrors.Errorf("set network account actor: %w", err)
	}

	// Setup cron
	cronact, err := SetupCronActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup cron actor: %w", err)
	}
	if err := state.SetActor(builtin.CronActorAddr, cronact); err != nil {
		return nil, xerrors.Errorf("set cron actor: %w", err)
	}

	// Create empty power actor
	spact, err := SetupStoragePowerActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}
	if err := state.SetActor(builtin.StoragePowerActorAddr, spact); err != nil {
		return nil, xerrors.Errorf("set storage market actor: %w", err)
	}

	// Create empty market actor
	marketact, err := SetupStorageMarketActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}
	if err := state.SetActor(builtin.StorageMarketActorAddr, marketact); err != nil {
		return nil, xerrors.Errorf("set market actor: %w", err)
	}

	// Create verified registry
	verifact, err := SetupVerifiedRegistryActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}
	if err := state.SetActor(builtin.VerifiedRegistryActorAddr, verifact); err != nil {
		return nil, xerrors.Errorf("set market actor: %w", err)
	}

	// Setup burnt-funds
	err = state.SetActor(builtin.BurntFundsActorAddr, &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: types.NewInt(0),
		Head:    emptyobject,
	})
	if err != nil {
		return nil, xerrors.Errorf("set burnt funds account actor: %w", err)
	}

	// Create accounts
	for id, info := range template.Accounts {
		if info.Type != genesis.TAccount {
			return nil, xerrors.New("unsupported account type") // TODO: msigs
		}

		ida, err := address.NewIDAddress(uint64(AccountStart + id))
		if err != nil {
			return nil, err
		}

		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(info.Meta, &ainfo); err != nil {
			return nil, xerrors.Errorf("unmarshaling account meta: %w", err)
		}

		st, err := cst.Put(ctx, &account.State{Address: ainfo.Owner})
		if err != nil {
			return nil, err
		}

		err = state.SetActor(ida, &types.Actor{
			Code:    builtin.AccountActorCodeID,
			Balance: info.Balance,
			Head:    st,
		})
		if err != nil {
			return nil, xerrors.Errorf("setting account from actmap: %w", err)
		}
	}

	return state, nil
}

func MakeGenesisBlock(ctx context.Context, bs bstore.Blockstore, sys runtime.Syscalls, template genesis.Template) (*GenesisBootstrap, error) {
	st, err := MakeInitialStateTree(ctx, bs, template)
	if err != nil {
		return nil, xerrors.Errorf("make initial state tree failed: %w", err)
	}

	stateroot, err := st.Flush(ctx)
	if err != nil {
		return nil, xerrors.Errorf("flush state tree failed: %w", err)
	}

	// temp chainstore
	cs := store.NewChainStore(bs, datastore.NewMapDatastore(), sys)
	stateroot, err = SetupStorageMiners(ctx, cs, stateroot, template.Miners)
	if err != nil {
		return nil, xerrors.Errorf("setup storage miners failed: %w", err)
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
		Miner:                 builtin.SystemActorAddr,
		Ticket:                genesisticket,
		Parents:               []cid.Cid{},
		Height:                0,
		ParentWeight:          types.NewInt(0),
		ParentStateRoot:       stateroot,
		Messages:              mmb.Cid(),
		ParentMessageReceipts: emptyroot,
		BLSAggregate:          nil,
		BlockSig:              nil,
		Timestamp:             template.Timestamp,
		ElectionProof:         new(types.ElectionProof),
		BeaconEntries: []types.BeaconEntry{
			{
				Round: 0,
				Data:  make([]byte, 32),
			},
		},
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

package genesis

import (
	"context"
	"encoding/json"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/genesis"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/sigs"
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
    - Set network power in the power actor to what we'll have after genesis creation
	- Recreate reward actor state with the right power
    - For each precommitted sector
      - Get deal weight
      - Calculate QA Power
      - Remove fake power from the power actor
      - Calculate pledge
      - Precommit
      - Confirm valid

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

func MakeInitialStateTree(ctx context.Context, bs bstore.Blockstore, template genesis.Template) (*state.StateTree, map[address.Address]address.Address, error) {
	// Create empty state tree

	cst := cbor.NewCborStore(bs)
	_, err := cst.Put(context.TODO(), []struct{}{})
	if err != nil {
		return nil, nil, xerrors.Errorf("putting empty object: %w", err)
	}

	state, err := state.NewStateTree(cst)
	if err != nil {
		return nil, nil, xerrors.Errorf("making new state tree: %w", err)
	}

	emptyobject, err := cst.Put(context.TODO(), []struct{}{})
	if err != nil {
		return nil, nil, xerrors.Errorf("failed putting empty object: %w", err)
	}

	// Create system actor

	sysact, err := SetupSystemActor(bs)
	if err != nil {
		return nil, nil, xerrors.Errorf("setup init actor: %w", err)
	}
	if err := state.SetActor(builtin.SystemActorAddr, sysact); err != nil {
		return nil, nil, xerrors.Errorf("set init actor: %w", err)
	}

	// Create init actor

	initact, keyIDs, err := SetupInitActor(bs, template.NetworkName, template.Accounts, template.VerifregRootKey)
	if err != nil {
		return nil, nil, xerrors.Errorf("setup init actor: %w", err)
	}
	if err := state.SetActor(builtin.InitActorAddr, initact); err != nil {
		return nil, nil, xerrors.Errorf("set init actor: %w", err)
	}

	// Setup reward
	// RewardActor's state is overrwritten by SetupStorageMiners
	rewact, err := SetupRewardActor(bs, big.Zero())
	if err != nil {
		return nil, nil, xerrors.Errorf("setup init actor: %w", err)
	}

	err = state.SetActor(builtin.RewardActorAddr, rewact)
	if err != nil {
		return nil, nil, xerrors.Errorf("set network account actor: %w", err)
	}

	// Setup cron
	cronact, err := SetupCronActor(bs)
	if err != nil {
		return nil, nil, xerrors.Errorf("setup cron actor: %w", err)
	}
	if err := state.SetActor(builtin.CronActorAddr, cronact); err != nil {
		return nil, nil, xerrors.Errorf("set cron actor: %w", err)
	}

	// Create empty power actor
	spact, err := SetupStoragePowerActor(bs)
	if err != nil {
		return nil, nil, xerrors.Errorf("setup storage market actor: %w", err)
	}
	if err := state.SetActor(builtin.StoragePowerActorAddr, spact); err != nil {
		return nil, nil, xerrors.Errorf("set storage market actor: %w", err)
	}

	// Create empty market actor
	marketact, err := SetupStorageMarketActor(bs)
	if err != nil {
		return nil, nil, xerrors.Errorf("setup storage market actor: %w", err)
	}
	if err := state.SetActor(builtin.StorageMarketActorAddr, marketact); err != nil {
		return nil, nil, xerrors.Errorf("set market actor: %w", err)
	}

	// Create verified registry
	verifact, err := SetupVerifiedRegistryActor(bs)
	if err != nil {
		return nil, nil, xerrors.Errorf("setup storage market actor: %w", err)
	}
	if err := state.SetActor(builtin.VerifiedRegistryActorAddr, verifact); err != nil {
		return nil, nil, xerrors.Errorf("set market actor: %w", err)
	}

	// Setup burnt-funds
	err = state.SetActor(builtin.BurntFundsActorAddr, &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: types.NewInt(0),
		Head:    emptyobject,
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("set burnt funds account actor: %w", err)
	}

	// Create accounts
	for id, info := range template.Accounts {
		if info.Type != genesis.TAccount && info.Type != genesis.TMultisig {
			return nil, nil, xerrors.New("unsupported account type")
		}

		ida, err := address.NewIDAddress(uint64(AccountStart + id))
		if err != nil {
			return nil, nil, err
		}

		if err = createAccount(ctx, bs, cst, state, ida, info); err != nil {
			return nil, nil, err
		}

	}

	vregroot, err := address.NewIDAddress(80)
	if err != nil {
		return nil, nil, err
	}

	if err = createAccount(ctx, bs, cst, state, vregroot, template.VerifregRootKey); err != nil {
		return nil, nil, err
	}

	// Setup the first verifier as ID-address 81
	// TODO: remove this
	skBytes, err := sigs.Generate(crypto.SigTypeBLS)
	if err != nil {
		return nil, nil, xerrors.Errorf("creating random verifier secret key: %w", err)
	}

	verifierPk, err := sigs.ToPublic(crypto.SigTypeBLS, skBytes)
	if err != nil {
		return nil, nil, xerrors.Errorf("creating random verifier public key: %w", err)
	}

	verifierAd, err := address.NewBLSAddress(verifierPk)
	if err != nil {
		return nil, nil, xerrors.Errorf("creating random verifier address: %w", err)
	}

	verifierId, err := address.NewIDAddress(81)
	if err != nil {
		return nil, nil, err
	}

	verifierState, err := cst.Put(ctx, &account.State{Address: verifierAd})
	if err != nil {
		return nil, nil, err
	}

	err = state.SetActor(verifierId, &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: types.NewInt(0),
		Head:    verifierState,
	})

	if err != nil {
		return nil, nil, xerrors.Errorf("setting account from actmap: %w", err)
	}

	return state, keyIDs, nil
}

func createAccount(ctx context.Context, bs bstore.Blockstore, cst cbor.IpldStore, state *state.StateTree, ida address.Address, info genesis.Actor) error {
	if info.Type == genesis.TAccount {
		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(info.Meta, &ainfo); err != nil {
			return xerrors.Errorf("unmarshaling account meta: %w", err)
		}
		st, err := cst.Put(ctx, &account.State{Address: ainfo.Owner})
		if err != nil {
			return err
		}
		err = state.SetActor(ida, &types.Actor{
			Code:    builtin.AccountActorCodeID,
			Balance: info.Balance,
			Head:    st,
		})
		if err != nil {
			return xerrors.Errorf("setting account from actmap: %w", err)
		}
	} else if info.Type == genesis.TMultisig {
		var ainfo genesis.MultisigMeta
		if err := json.Unmarshal(info.Meta, &ainfo); err != nil {
			return xerrors.Errorf("unmarshaling account meta: %w", err)
		}
		pending, err := adt.MakeEmptyMap(adt.WrapStore(ctx, cst)).Root()
		if err != nil {
			return xerrors.Errorf("failed to create empty map: %v", err)
		}

		st, err := cst.Put(ctx, &multisig.State{
			Signers:               ainfo.Signers,
			NumApprovalsThreshold: uint64(ainfo.Threshold),
			StartEpoch:            abi.ChainEpoch(ainfo.VestingStart),
			UnlockDuration:        abi.ChainEpoch(ainfo.VestingDuration),
			PendingTxns:           pending,
			InitialBalance:        info.Balance,
		})
		if err != nil {
			return err
		}
		err = state.SetActor(ida, &types.Actor{
			Code:    builtin.MultisigActorCodeID,
			Balance: info.Balance,
			Head:    st,
		})
		if err != nil {
			return xerrors.Errorf("setting account from actmap: %w", err)
		}
	}

	return nil
}

func VerifyPreSealedData(ctx context.Context, cs *store.ChainStore, stateroot cid.Cid, template genesis.Template, keyIDs map[address.Address]address.Address) (cid.Cid, error) {
	verifNeeds := make(map[address.Address]abi.PaddedPieceSize)
	var sum abi.PaddedPieceSize

	vmopt := vm.VMOpts{
		StateBase:      stateroot,
		Epoch:          0,
		Rand:           &fakeRand{},
		Bstore:         cs.Blockstore(),
		Syscalls:       mkFakedSigSyscalls(cs.VMSys()),
		CircSupplyCalc: nil,
		BaseFee:        types.NewInt(0),
	}
	vm, err := vm.NewVM(&vmopt)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	for _, m := range template.Miners {

		// Add the miner to the market actor's balance table
		_, err = doExec(ctx, vm, builtin.StorageMarketActorAddr, m.Owner, builtin.MethodsMarket.AddBalance, mustEnc(adt.Empty))
		for _, s := range m.Sectors {
			amt := s.Deal.PieceSize
			verifNeeds[keyIDs[s.Deal.Client]] += amt
			sum += amt
		}
	}

	verifregRoot, err := address.NewIDAddress(80)
	if err != nil {
		return cid.Undef, err
	}

	verifier, err := address.NewIDAddress(81)
	if err != nil {
		return cid.Undef, err
	}

	_, err = doExecValue(ctx, vm, builtin.VerifiedRegistryActorAddr, verifregRoot, types.NewInt(0), builtin.MethodsVerifiedRegistry.AddVerifier, mustEnc(&verifreg.AddVerifierParams{

		Address:   verifier,
		Allowance: abi.NewStoragePower(int64(sum)), // eh, close enough

	}))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create verifier: %w", err)
	}

	for c, amt := range verifNeeds {
		_, err := doExecValue(ctx, vm, builtin.VerifiedRegistryActorAddr, verifier, types.NewInt(0), builtin.MethodsVerifiedRegistry.AddVerifiedClient, mustEnc(&verifreg.AddVerifiedClientParams{
			Address:   c,
			Allowance: abi.NewStoragePower(int64(amt)),
		}))
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to add verified client: %w", err)
		}
	}

	st, err := vm.Flush(ctx)
	if err != nil {
		return cid.Cid{}, xerrors.Errorf("vm flush: %w", err)
	}

	return st, nil
}

func MakeGenesisBlock(ctx context.Context, bs bstore.Blockstore, sys vm.SyscallBuilder, template genesis.Template) (*GenesisBootstrap, error) {
	st, keyIDs, err := MakeInitialStateTree(ctx, bs, template)
	if err != nil {
		return nil, xerrors.Errorf("make initial state tree failed: %w", err)
	}

	stateroot, err := st.Flush(ctx)
	if err != nil {
		return nil, xerrors.Errorf("flush state tree failed: %w", err)
	}

	// temp chainstore
	cs := store.NewChainStore(bs, datastore.NewMapDatastore(), sys)

	// Verify PreSealed Data
	stateroot, err = VerifyPreSealedData(ctx, cs, stateroot, template, keyIDs)
	if err != nil {
		return nil, xerrors.Errorf("failed to verify presealed data: %w", err)
	}

	stateroot, err = SetupStorageMiners(ctx, cs, stateroot, template.Miners)
	if err != nil {
		return nil, xerrors.Errorf("setup miners failed: %w", err)
	}

	store := adt.WrapStore(ctx, cbor.NewCborStore(bs))
	emptyroot, err := adt.MakeEmptyArray(store).Root()
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
		ParentBaseFee: abi.NewTokenAmount(build.InitialBaseFee),
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

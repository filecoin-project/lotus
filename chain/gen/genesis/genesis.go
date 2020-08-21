package genesis

import (
	"context"
	"encoding/json"
	"fmt"

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

	idStart, initact, keyIDs, err := SetupInitActor(bs, template.NetworkName, template.Accounts, template.VerifregRootKey)
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
	for _, info := range template.Accounts {

		switch info.Type {
		case genesis.TAccount:
			if err := createAccountActor(ctx, cst, state, info, keyIDs); err != nil {
				return nil, nil, xerrors.Errorf("failed to create account actor: %w", err)
			}

		case genesis.TMultisig:

			ida, err := address.NewIDAddress(uint64(idStart))
			if err != nil {
				return nil, nil, err
			}
			idStart++

			if err := createMultisigAccount(ctx, bs, cst, state, ida, info, keyIDs); err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, xerrors.New("unsupported account type")
		}

	}

	vregroot, err := address.NewIDAddress(80)
	if err != nil {
		return nil, nil, err
	}

	if err = createMultisigAccount(ctx, bs, cst, state, vregroot, template.VerifregRootKey, keyIDs); err != nil {
		return nil, nil, xerrors.Errorf("failed to set up verified registry signer: %w", err)
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

	totalFilAllocated := big.Zero()

	// flush as ForEach works on the HAMT
	if _, err := state.Flush(ctx); err != nil {
		return nil, nil, err
	}
	err = state.ForEach(func(addr address.Address, act *types.Actor) error {
		totalFilAllocated = big.Add(totalFilAllocated, act.Balance)
		return nil
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("summing account balances in state tree: %w", err)
	}

	totalFil := big.Mul(big.NewInt(int64(build.FilBase)), big.NewInt(int64(build.FilecoinPrecision)))
	remainingFil := big.Sub(totalFil, totalFilAllocated)
	if remainingFil.Sign() < 0 {
		return nil, nil, xerrors.Errorf("somehow overallocated filecoin (allocated = %s)", types.FIL(totalFilAllocated))
	}

	remAccKey, err := address.NewIDAddress(90)
	if err != nil {
		return nil, nil, err
	}

	template.RemainderAccount.Balance = remainingFil

	if err := createMultisigAccount(ctx, bs, cst, state, remAccKey, template.RemainderAccount, keyIDs); err != nil {
		return nil, nil, xerrors.Errorf("failed to set up remainder account: %w", err)
	}

	return state, keyIDs, nil
}

func createAccountActor(ctx context.Context, cst cbor.IpldStore, state *state.StateTree, info genesis.Actor, keyIDs map[address.Address]address.Address) error {
	var ainfo genesis.AccountMeta
	if err := json.Unmarshal(info.Meta, &ainfo); err != nil {
		return xerrors.Errorf("unmarshaling account meta: %w", err)
	}
	st, err := cst.Put(ctx, &account.State{Address: ainfo.Owner})
	if err != nil {
		return err
	}

	ida, ok := keyIDs[ainfo.Owner]
	if !ok {
		return fmt.Errorf("no registered ID for account actor: %s", ainfo.Owner)
	}

	err = state.SetActor(ida, &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: info.Balance,
		Head:    st,
	})
	if err != nil {
		return xerrors.Errorf("setting account from actmap: %w", err)
	}
	return nil
}

func createMultisigAccount(ctx context.Context, bs bstore.Blockstore, cst cbor.IpldStore, state *state.StateTree, ida address.Address, info genesis.Actor, keyIDs map[address.Address]address.Address) error {
	if info.Type != genesis.TMultisig {
		return fmt.Errorf("can only call createMultisigAccount with multisig Actor info")
	}
	var ainfo genesis.MultisigMeta
	if err := json.Unmarshal(info.Meta, &ainfo); err != nil {
		return xerrors.Errorf("unmarshaling account meta: %w", err)
	}
	pending, err := adt.MakeEmptyMap(adt.WrapStore(ctx, cst)).Root()
	if err != nil {
		return xerrors.Errorf("failed to create empty map: %v", err)
	}

	var signers []address.Address

	for _, e := range ainfo.Signers {
		idAddress, ok := keyIDs[e]
		if !ok {
			return fmt.Errorf("no registered key ID for signer: %s", e)
		}

		// Check if actor already exists
		_, err := state.GetActor(e)
		if err == nil {
			signers = append(signers, idAddress)
			continue
		}

		st, err := cst.Put(ctx, &account.State{Address: e})
		if err != nil {
			return err
		}
		err = state.SetActor(idAddress, &types.Actor{
			Code:    builtin.AccountActorCodeID,
			Balance: types.NewInt(0),
			Head:    st,
		})
		if err != nil {
			return xerrors.Errorf("setting account from actmap: %w", err)
		}
		signers = append(signers, idAddress)
	}

	st, err := cst.Put(ctx, &multisig.State{
		Signers:               signers,
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

	for mi, m := range template.Miners {
		for si, s := range m.Sectors {
			if s.Deal.Provider != m.ID {
				return cid.Undef, xerrors.Errorf("Sector %d in miner %d in template had mismatch in provider and miner ID: %s != %s", si, mi, s.Deal.Provider, m.ID)
			}

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

	filecoinGenesisCid, err := cid.Decode("bafyreiaqpwbbyjo4a42saasj36kkrpv4tsherf2e7bvezkert2a7dhonoi")
	if err != nil {
		return nil, xerrors.Errorf("failed to decode filecoin genesis block CID: %w", err)
	}

	if !expectedCid().Equals(filecoinGenesisCid) {
		return nil, xerrors.Errorf("expectedCid != filecoinGenesisCid")
	}

	gblk, err := getGenesisBlock()
	if err != nil {
		return nil, xerrors.Errorf("failed to construct filecoin genesis block: %w", err)
	}

	if !filecoinGenesisCid.Equals(gblk.Cid()) {
		return nil, xerrors.Errorf("filecoinGenesisCid != gblk.Cid")
	}

	if err := bs.Put(gblk); err != nil {
		return nil, xerrors.Errorf("failed writing filecoin genesis block to blockstore: %w", err)
	}

	b := &types.BlockHeader{
		Miner:                 builtin.SystemActorAddr,
		Ticket:                genesisticket,
		Parents:               []cid.Cid{filecoinGenesisCid},
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

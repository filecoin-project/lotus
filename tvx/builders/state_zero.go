package builders

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	account_spec "github.com/filecoin-project/specs-actors/actors/builtin/account"
	cron_spec "github.com/filecoin-project/specs-actors/actors/builtin/cron"
	init_spec "github.com/filecoin-project/specs-actors/actors/builtin/init"
	market_spec "github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	power_spec "github.com/filecoin-project/specs-actors/actors/builtin/power"
	reward_spec "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/builtin/system"
	runtime_spec "github.com/filecoin-project/specs-actors/actors/runtime"
	adt_spec "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
)

const (
	totalFilecoin     = 2_000_000_000
	filecoinPrecision = 1_000_000_000_000_000_000
)

var (
	TotalNetworkBalance = big_spec.Mul(big_spec.NewInt(totalFilecoin), big_spec.NewInt(filecoinPrecision))
	EmptyReturnValue    = []byte{}
)

var (
	// initialized by calling initializeStoreWithAdtRoots
	EmptyArrayCid        cid.Cid
	EmptyDeadlinesCid    cid.Cid
	EmptyMapCid          cid.Cid
	EmptyMultiMapCid     cid.Cid
	EmptyBitfieldCid     cid.Cid
	EmptyVestingFundsCid cid.Cid
)

const (
	TestSealProofType = abi_spec.RegisteredSealProof_StackedDrg2KiBV1
)

func (b *Builder) initializeZeroState() {
	if err := insertEmptyStructures(b.Stores.ADTStore); err != nil {
		panic(err)
	}

	type ActorState struct {
		Addr    address.Address
		Balance abi_spec.TokenAmount
		Code    cid.Cid
		State   runtime_spec.CBORMarshaler
	}

	var actors []ActorState

	actors = append(actors, ActorState{
		Addr:    builtin_spec.InitActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.InitActorCodeID,
		State:   init_spec.ConstructState(EmptyMapCid, "chain-validation"),
	})

	zeroRewardState := reward_spec.ConstructState(big_spec.Zero())
	zeroRewardState.ThisEpochReward = big_spec.NewInt(1e17)

	actors = append(actors, ActorState{
		Addr:    builtin_spec.RewardActorAddr,
		Balance: TotalNetworkBalance,
		Code:    builtin_spec.RewardActorCodeID,
		State:   zeroRewardState,
	})

	actors = append(actors, ActorState{
		Addr:    builtin_spec.BurntFundsActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.AccountActorCodeID,
		State:   &account_spec.State{Address: builtin_spec.BurntFundsActorAddr},
	})

	actors = append(actors, ActorState{
		Addr:    builtin_spec.StoragePowerActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.StoragePowerActorCodeID,
		State:   power_spec.ConstructState(EmptyMapCid, EmptyMultiMapCid),
	})

	actors = append(actors, ActorState{
		Addr:    builtin_spec.StorageMarketActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.StorageMarketActorCodeID,
		State: &market_spec.State{
			Proposals:        EmptyArrayCid,
			States:           EmptyArrayCid,
			PendingProposals: EmptyMapCid,
			EscrowTable:      EmptyMapCid,
			LockedTable:      EmptyMapCid,
			NextID:           abi_spec.DealID(0),
			DealOpsByEpoch:   EmptyMultiMapCid,
			LastCron:         0,
		},
	})

	actors = append(actors, ActorState{
		Addr:    builtin_spec.SystemActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.SystemActorCodeID,
		State:   &system.State{},
	})

	actors = append(actors, ActorState{
		Addr:    builtin_spec.CronActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.CronActorCodeID,
		State: &cron_spec.State{Entries: []cron_spec.Entry{
			{
				Receiver:  builtin_spec.StoragePowerActorAddr,
				MethodNum: builtin_spec.MethodsPower.OnEpochTickEnd,
			},
		}},
	})

	for _, act := range actors {
		_ = b.Actors.CreateActor(act.Code, act.Addr, act.Balance, act.State)
	}
}

func insertEmptyStructures(store adt_spec.Store) error {
	var err error
	_, err = store.Put(context.TODO(), []struct{}{})
	if err != nil {
		return err
	}

	EmptyArrayCid, err = adt_spec.MakeEmptyArray(store).Root()
	if err != nil {
		return err
	}

	EmptyMapCid, err = adt_spec.MakeEmptyMap(store).Root()
	if err != nil {
		return err
	}

	EmptyMultiMapCid, err = adt_spec.MakeEmptyMultimap(store).Root()
	if err != nil {
		return err
	}

	EmptyDeadlinesCid, err = store.Put(context.TODO(), miner.ConstructDeadline(EmptyArrayCid))
	if err != nil {
		return err
	}

	emptyBitfield := bitfield.NewFromSet(nil)
	EmptyBitfieldCid, err = store.Put(context.TODO(), emptyBitfield)
	if err != nil {
		return err
	}

	EmptyVestingFundsCid, err = store.Put(context.Background(), miner.ConstructVestingFunds())
	if err != nil {
		return err
	}

	return nil
}

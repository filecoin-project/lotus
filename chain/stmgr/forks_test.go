package stmgr_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	ipldcbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/network"
	rtt "github.com/filecoin-project/go-state-types/rt"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	rt2 "github.com/filecoin-project/specs-actors/v2/actors/runtime"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	_init "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/stmgr"
	. "github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

const testForkHeight = 40

type testActor struct {
}

// must use existing actor that an account is allowed to exec.
func (testActor) Code() cid.Cid  { return builtin0.PaymentChannelActorCodeID }
func (testActor) State() cbor.Er { return new(testActorState) }

type testActorState struct {
	HasUpgraded uint64
}

func (tas *testActorState) MarshalCBOR(w io.Writer) error {
	return cbg.CborWriteHeader(w, cbg.MajUnsignedInt, tas.HasUpgraded)
}

func (tas *testActorState) UnmarshalCBOR(r io.Reader) error {
	t, v, err := cbg.CborReadHeader(r)
	if err != nil {
		return err
	}
	if t != cbg.MajUnsignedInt {
		return fmt.Errorf("wrong type in test actor state (got %d)", t)
	}
	tas.HasUpgraded = v
	return nil
}

func (ta testActor) Exports() []interface{} {
	return []interface{}{
		1: ta.Constructor,
		2: ta.TestMethod,
	}
}

func (ta *testActor) Constructor(rt rt2.Runtime, params *abi.EmptyValue) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	rt.StateCreate(&testActorState{11})
	//fmt.Println("NEW ACTOR ADDRESS IS: ", rt.Receiver())

	return abi.Empty
}

func (ta *testActor) TestMethod(rt rt2.Runtime, params *abi.EmptyValue) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	var st testActorState
	rt.StateReadonly(&st)

	if rt.CurrEpoch() > testForkHeight {
		if st.HasUpgraded != 55 {
			panic(aerrors.Fatal("fork updating applied in wrong order"))
		}
	} else {
		if st.HasUpgraded != 11 {
			panic(aerrors.Fatal("fork updating happened too early"))
		}
	}

	return abi.Empty
}

func TestForkHeightTriggers(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	ctx := context.TODO()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	// predicting the address here... may break if other assumptions change
	taddr, err := address.NewIDAddress(1002)
	if err != nil {
		t.Fatal(err)
	}

	sm, err := NewStateManager(
		cg.ChainStore(), consensus.NewTipSetExecutor(filcns.RewardFunc), cg.StateManager().VMSys(), UpgradeSchedule{{
			Network: network.Version1,
			Height:  testForkHeight,
			Migration: func(ctx context.Context, sm *StateManager, cache MigrationCache, cb ExecMonitor,
				root cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
				cst := ipldcbor.NewCborStore(sm.ChainStore().StateBlockstore())

				st, err := sm.StateTree(root)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
				}

				act, err := st.GetActor(taddr)
				if err != nil {
					return cid.Undef, err
				}

				var tas testActorState
				if err := cst.Get(ctx, act.Head, &tas); err != nil {
					return cid.Undef, xerrors.Errorf("in fork handler, failed to run get: %w", err)
				}

				tas.HasUpgraded = 55

				ns, err := cst.Put(ctx, &tas)
				if err != nil {
					return cid.Undef, err
				}

				act.Head = ns

				if err := st.SetActor(taddr, act); err != nil {
					return cid.Undef, err
				}

				return st.Flush(ctx)
			}}}, cg.BeaconSchedule(), datastore.NewMapDatastore(), nil)
	if err != nil {
		t.Fatal(err)
	}

	inv := consensus.NewActorRegistry()
	registry := builtin.MakeRegistryLegacy([]rtt.VMActor{testActor{}})
	inv.Register(actorstypes.Version0, nil, registry)

	sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (vm.Interface, error) {
		nvm, err := vm.NewLegacyVM(ctx, vmopt)
		if err != nil {
			return nil, err
		}
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	var msgs []*types.SignedMessage

	enc, err := actors.SerializeParams(&init2.ExecParams{CodeCID: (testActor{}).Code()})
	if err != nil {
		t.Fatal(err)
	}

	m := &types.Message{
		From:     cg.Banker(),
		To:       _init.Address,
		Method:   _init.Methods.Exec,
		Params:   enc,
		GasLimit: types.TestGasLimit,
	}
	sig, err := cg.Wallet().WalletSign(ctx, cg.Banker(), m.Cid().Bytes(), api.MsgMeta{})
	if err != nil {
		t.Fatal(err)
	}
	msgs = append(msgs, &types.SignedMessage{
		Signature: *sig,
		Message:   *m,
	})

	nonce := uint64(1)
	cg.GetMessages = func(cg *gen.ChainGen) ([]*types.SignedMessage, error) {
		if len(msgs) > 0 {
			fmt.Println("added construct method")
			m := msgs
			msgs = nil
			return m, nil
		}

		m := &types.Message{
			From:     cg.Banker(),
			To:       taddr,
			Method:   2,
			Params:   nil,
			Nonce:    nonce,
			GasLimit: types.TestGasLimit,
		}
		nonce++

		sig, err := cg.Wallet().WalletSign(ctx, cg.Banker(), m.Cid().Bytes(), api.MsgMeta{})
		if err != nil {
			return nil, err
		}

		return []*types.SignedMessage{
			{
				Signature: *sig,
				Message:   *m,
			},
		}, nil
	}

	for i := 0; i < 50; i++ {
		_, err = cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestForkRefuseCall(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	for after := 0; after < 3; after++ {
		for before := 0; before < 3; before++ {
			// Makes the lints happy...
			after := after
			before := before
			t.Run(fmt.Sprintf("after:%d,before:%d", after, before), func(t *testing.T) {
				testForkRefuseCall(t, before, after)
			})
		}
	}

}
func testForkRefuseCall(t *testing.T, nullsBefore, nullsAfter int) {
	ctx := context.TODO()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	var migrationCount int
	sm, err := NewStateManager(
		cg.ChainStore(), consensus.NewTipSetExecutor(filcns.RewardFunc), cg.StateManager().VMSys(), UpgradeSchedule{{
			Network:   network.Version1,
			Expensive: true,
			Height:    testForkHeight,
			Migration: func(ctx context.Context, sm *StateManager, cache MigrationCache, cb ExecMonitor,
				root cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
				migrationCount++
				return root, nil
			}}}, cg.BeaconSchedule(), datastore.NewMapDatastore(), nil)
	if err != nil {
		t.Fatal(err)
	}

	inv := consensus.NewActorRegistry()
	registry := builtin.MakeRegistryLegacy([]rtt.VMActor{testActor{}})
	inv.Register(actorstypes.Version0, nil, registry)

	sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (vm.Interface, error) {
		nvm, err := vm.NewLegacyVM(ctx, vmopt)
		if err != nil {
			return nil, err
		}
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	enc, err := actors.SerializeParams(&init2.ExecParams{CodeCID: (testActor{}).Code()})
	if err != nil {
		t.Fatal(err)
	}

	m := &types.Message{
		From:       cg.Banker(),
		To:         _init.Address,
		Method:     _init.Methods.Exec,
		Params:     enc,
		GasLimit:   types.TestGasLimit,
		Value:      types.NewInt(0),
		GasPremium: types.NewInt(0),
		GasFeeCap:  types.NewInt(0),
	}

	nullStart := abi.ChainEpoch(testForkHeight - nullsBefore)
	nullLength := abi.ChainEpoch(nullsBefore + nullsAfter)

	for i := 0; i < testForkHeight*2; i++ {
		pts := cg.CurTipset.TipSet()
		skip := abi.ChainEpoch(0)
		if pts.Height() == nullStart {
			skip = nullLength
		}
		ts, err := cg.NextTipSetFromMiners(pts, cg.Miners, skip)
		if err != nil {
			t.Fatal(err)
		}

		parentHeight := pts.Height()
		currentHeight := ts.TipSet.TipSet().Height()

		// CallWithGas calls on top of the given tipset.
		ret, err := sm.CallWithGas(ctx, m, nil, ts.TipSet.TipSet(), true)
		if parentHeight <= testForkHeight && currentHeight >= testForkHeight {
			// If I had a fork, or I _will_ have a fork, it should fail.
			require.Equal(t, ErrExpensiveFork, err)
		} else {
			require.NoError(t, err)
			require.True(t, ret.MsgRct.ExitCode.IsSuccess())
		}

		// Call always applies the message to the "next block" after the tipset's parent state.
		ret, err = sm.Call(ctx, m, ts.TipSet.TipSet())
		if parentHeight <= testForkHeight && currentHeight >= testForkHeight {
			require.Equal(t, ErrExpensiveFork, err)
		} else {
			require.NoError(t, err)
			require.True(t, ret.MsgRct.ExitCode.IsSuccess())
		}

		// Calls without a tipset should walk back to the last non-fork tipset.
		// We _verify_ that the migration wasn't run multiple times at the end of the
		// test.
		ret, err = sm.CallWithGas(ctx, m, nil, nil, true)
		require.NoError(t, err)
		require.True(t, ret.MsgRct.ExitCode.IsSuccess())

		ret, err = sm.Call(ctx, m, nil)
		require.NoError(t, err)
		require.True(t, ret.MsgRct.ExitCode.IsSuccess())
	}
	// Make sure we didn't execute the migration multiple times.
	require.Equal(t, migrationCount, 1)
}

func TestForkPreMigration(t *testing.T) {
	// Backup the original value of the DISABLE_PRE_MIGRATIONS environment variable
	originalValue, _ := os.LookupEnv("LOTUS_DISABLE_PRE_MIGRATIONS")

	// Unset the DISABLE_PRE_MIGRATIONS environment variable for the test
	if err := os.Unsetenv("LOTUS_DISABLE_PRE_MIGRATIONS"); err != nil {
		t.Fatalf("failed to unset LOTUS_DISABLE_PRE_MIGRATIONS: %v", err)
	}

	// Restore the original DISABLE_PRE_MIGRATIONS environment variable at the end of the test
	defer func() {
		if err := os.Setenv("LOTUS_DISABLE_PRE_MIGRATIONS", originalValue); err != nil {
			t.Fatalf("failed to restore LOTUS_DISABLE_PRE_MIGRATIONS: %v", err)
		}
	}()
	logging.SetAllLoggers(logging.LevelInfo)

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	fooCid, err := abi.CidBuilder.Sum([]byte("foo"))
	require.NoError(t, err)

	barCid, err := abi.CidBuilder.Sum([]byte("bar"))
	require.NoError(t, err)

	failCid, err := abi.CidBuilder.Sum([]byte("fail"))
	require.NoError(t, err)

	var wait20 sync.WaitGroup
	wait20.Add(3)

	wasCanceled := make(chan struct{})

	checkCache := func(t *testing.T, cache MigrationCache) {
		found, value, err := cache.Read("foo")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, fooCid, value)

		found, value, err = cache.Read("bar")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, barCid, value)

		found, _, err = cache.Read("fail")
		require.NoError(t, err)
		require.False(t, found)
	}

	counter := make(chan struct{}, 10)

	sm, err := NewStateManager(
		cg.ChainStore(), consensus.NewTipSetExecutor(filcns.RewardFunc), cg.StateManager().VMSys(), UpgradeSchedule{{
			Network: network.Version1,
			Height:  testForkHeight,
			Migration: func(ctx context.Context, sm *StateManager, cache MigrationCache, cb ExecMonitor,
				root cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {

				// Make sure the test that should be canceled, is canceled.
				select {
				case <-wasCanceled:
				case <-ctx.Done():
					return cid.Undef, ctx.Err()
				}

				// the cache should be setup correctly.
				checkCache(t, cache)

				counter <- struct{}{}

				return root, nil
			},
			PreMigrations: []PreMigration{{
				StartWithin: 20,
				PreMigration: func(ctx context.Context, _ *StateManager, cache MigrationCache,
					_ cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) error {
					wait20.Done()
					wait20.Wait()

					err := cache.Write("foo", fooCid)
					require.NoError(t, err)

					counter <- struct{}{}

					return nil
				},
			}, {
				StartWithin: 20,
				PreMigration: func(ctx context.Context, _ *StateManager, cache MigrationCache,
					_ cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) error {
					wait20.Done()
					wait20.Wait()

					err := cache.Write("bar", barCid)
					require.NoError(t, err)

					counter <- struct{}{}

					return nil
				},
			}, {
				StartWithin: 20,
				PreMigration: func(ctx context.Context, _ *StateManager, cache MigrationCache,
					_ cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) error {
					wait20.Done()
					wait20.Wait()

					err := cache.Write("fail", failCid)
					require.NoError(t, err)

					counter <- struct{}{}

					// Fail this migration. The cached entry should not be persisted.
					return fmt.Errorf("failed")
				},
			}, {
				StartWithin: 15,
				StopWithin:  5,
				PreMigration: func(ctx context.Context, _ *StateManager, cache MigrationCache,
					_ cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) error {

					<-ctx.Done()
					close(wasCanceled)

					counter <- struct{}{}

					return nil
				},
			}, {
				StartWithin: 10,
				PreMigration: func(ctx context.Context, _ *StateManager, cache MigrationCache,
					_ cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) error {

					checkCache(t, cache)

					counter <- struct{}{}

					return nil
				},
			}}},
		}, cg.BeaconSchedule(), datastore.NewMapDatastore(), nil)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, sm.Start(context.Background()))
	defer func() {
		require.NoError(t, sm.Stop(context.Background()))
	}()

	inv := consensus.NewActorRegistry()
	registry := builtin.MakeRegistryLegacy([]rtt.VMActor{testActor{}})
	inv.Register(actorstypes.Version0, nil, registry)

	sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (vm.Interface, error) {
		nvm, err := vm.NewLegacyVM(ctx, vmopt)
		if err != nil {
			return nil, err
		}
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	for i := 0; i < 50; i++ {
		_, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}
	}
	// We have 5 pre-migration steps, and the migration. They should all have written something
	// to this channel.
	require.Equal(t, 6, len(counter))
}

func TestDisablePreMigration(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	cg, err := gen.NewGenerator()
	require.NoError(t, err)

	err = os.Setenv(EnvDisablePreMigrations, "1")
	require.NoError(t, err)

	defer func() {
		err := os.Unsetenv(EnvDisablePreMigrations)
		require.NoError(t, err)
	}()

	counter := make(chan struct{}, 10)

	sm, err := NewStateManager(
		cg.ChainStore(),
		consensus.NewTipSetExecutor(filcns.RewardFunc),
		cg.StateManager().VMSys(),
		UpgradeSchedule{{
			Network: network.Version1,
			Height:  testForkHeight,
			Migration: func(_ context.Context, _ *StateManager, _ MigrationCache, _ ExecMonitor,
				root cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) (cid.Cid, error) {

				counter <- struct{}{}

				return root, nil
			},
			PreMigrations: []PreMigration{{
				StartWithin: 20,
				PreMigration: func(ctx context.Context, _ *StateManager, _ MigrationCache,
					_ cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) error {
					panic("should be skipped")
				},
			}}},
		},
		cg.BeaconSchedule(),
		datastore.NewMapDatastore(),
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, sm.Start(context.Background()))
	defer func() {
		require.NoError(t, sm.Stop(context.Background()))
	}()

	inv := consensus.NewActorRegistry()
	registry := builtin.MakeRegistryLegacy([]rtt.VMActor{testActor{}})
	inv.Register(actorstypes.Version0, nil, registry)

	sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (vm.Interface, error) {
		nvm, err := vm.NewLegacyVM(ctx, vmopt)
		require.NoError(t, err)
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	for i := 0; i < 50; i++ {
		_, err := cg.NextTipSet()
		require.NoError(t, err)
	}

	require.Equal(t, 1, len(counter))
}

func TestMigrtionCache(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	cg, err := gen.NewGenerator()
	require.NoError(t, err)

	counter := make(chan struct{}, 10)
	metadataDs := datastore.NewMapDatastore()

	sm, err := NewStateManager(
		cg.ChainStore(),
		consensus.NewTipSetExecutor(filcns.RewardFunc),
		cg.StateManager().VMSys(),
		UpgradeSchedule{{
			Network: network.Version1,
			Height:  testForkHeight,
			Migration: func(_ context.Context, _ *StateManager, _ MigrationCache, _ ExecMonitor,
				root cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) (cid.Cid, error) {

				counter <- struct{}{}

				return root, nil
			}},
		},
		cg.BeaconSchedule(),
		metadataDs,
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, sm.Start(context.Background()))
	defer func() {
		require.NoError(t, sm.Stop(context.Background()))
	}()

	inv := consensus.NewActorRegistry()
	registry := builtin.MakeRegistryLegacy([]rtt.VMActor{testActor{}})
	inv.Register(actorstypes.Version0, nil, registry)

	sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (vm.Interface, error) {
		nvm, err := vm.NewLegacyVM(ctx, vmopt)
		require.NoError(t, err)
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	for i := 0; i < 50; i++ {
		_, err := cg.NextTipSet()
		require.NoError(t, err)
	}

	ts, err := cg.ChainStore().GetTipsetByHeight(context.Background(), testForkHeight, nil, false)
	require.NoError(t, err)

	root, _, err := stmgr.ComputeState(context.Background(), sm, testForkHeight+1, []*types.Message{}, ts)
	require.NoError(t, err)
	t.Log(root)

	require.Equal(t, 1, len(counter))

	{
		sm, err := NewStateManager(
			cg.ChainStore(),
			consensus.NewTipSetExecutor(filcns.RewardFunc),
			cg.StateManager().VMSys(),
			UpgradeSchedule{{
				Network: network.Version1,
				Height:  testForkHeight,
				Migration: func(_ context.Context, _ *StateManager, _ MigrationCache, _ ExecMonitor,
					root cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) (cid.Cid, error) {

					counter <- struct{}{}

					return root, nil
				}},
			},
			cg.BeaconSchedule(),
			metadataDs,
			nil,
		)
		require.NoError(t, err)
		sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (vm.Interface, error) {
			nvm, err := vm.NewLegacyVM(ctx, vmopt)
			require.NoError(t, err)
			nvm.SetInvoker(inv)
			return nvm, nil
		})

		ctx := context.Background()

		base, _, err := sm.ExecutionTrace(ctx, ts)
		require.NoError(t, err)
		_, err = sm.HandleStateForks(context.Background(), base, ts.Height(), nil, ts)
		require.NoError(t, err)

		// Should not have increased as we should be using the cached results in the metadataDs
		require.Equal(t, 1, len(counter))
	}
}

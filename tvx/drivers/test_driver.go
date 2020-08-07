package drivers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/lotus/chain/state"

	"github.com/filecoin-project/go-address"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	account_spec "github.com/filecoin-project/specs-actors/actors/builtin/account"
	cron_spec "github.com/filecoin-project/specs-actors/actors/builtin/cron"
	init_spec "github.com/filecoin-project/specs-actors/actors/builtin/init"
	market_spec "github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	multisig_spec "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	power_spec "github.com/filecoin-project/specs-actors/actors/builtin/power"
	reward_spec "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/builtin/system"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	runtime_spec "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	adt_spec "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-blockservice"
	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ipld/go-car"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/oni/tvx/chain"
	vtypes "github.com/filecoin-project/oni/tvx/chain/types"
	"github.com/filecoin-project/oni/tvx/schema"
)

var (

	// initialized by calling initializeStoreWithAdtRoots
	EmptyArrayCid     cid.Cid
	EmptyDeadlinesCid cid.Cid
	EmptyMapCid       cid.Cid
	EmptyMultiMapCid  cid.Cid
	EmptyBitfieldCid  cid.Cid
)

var (
	DefaultInitActorState          ActorState
	DefaultRewardActorState        ActorState
	DefaultBurntFundsActorState    ActorState
	DefaultStoragePowerActorState  ActorState
	DefaultStorageMarketActorState ActorState
	DefaultSystemActorState        ActorState
	DefaultCronActorState          ActorState
	DefaultBuiltinActorsState      []ActorState
)

const (
	TestSealProofType = abi_spec.RegisteredSealProof_StackedDrg2KiBV1
)

func init() {
	ms := newMockStore()
	if err := initializeStoreWithAdtRoots(ms); err != nil {
		panic(err)
	}

	DefaultInitActorState = ActorState{
		Addr:    builtin_spec.InitActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.InitActorCodeID,
		State:   init_spec.ConstructState(EmptyMapCid, "chain-validation"),
	}

	firstRewardState := reward_spec.ConstructState(big_spec.Zero())
	firstRewardState.ThisEpochReward = big_spec.NewInt(1e17)

	DefaultRewardActorState = ActorState{
		Addr:    builtin_spec.RewardActorAddr,
		Balance: TotalNetworkBalance,
		Code:    builtin_spec.RewardActorCodeID,
		State:   firstRewardState,
	}

	DefaultBurntFundsActorState = ActorState{
		Addr:    builtin_spec.BurntFundsActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.AccountActorCodeID,
		State:   &account_spec.State{Address: builtin_spec.BurntFundsActorAddr},
	}

	DefaultStoragePowerActorState = ActorState{
		Addr:    builtin_spec.StoragePowerActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.StoragePowerActorCodeID,
		State:   power_spec.ConstructState(EmptyMapCid, EmptyMultiMapCid),
	}

	DefaultStorageMarketActorState = ActorState{
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
	}

	DefaultSystemActorState = ActorState{
		Addr:    builtin_spec.SystemActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.SystemActorCodeID,
		State:   &system.State{},
	}

	DefaultCronActorState = ActorState{
		Addr:    builtin_spec.CronActorAddr,
		Balance: big_spec.Zero(),
		Code:    builtin_spec.CronActorCodeID,
		State: &cron_spec.State{Entries: []cron_spec.Entry{
			{
				Receiver:  builtin_spec.StoragePowerActorAddr,
				MethodNum: builtin_spec.MethodsPower.OnEpochTickEnd,
			},
		}},
	}

	DefaultBuiltinActorsState = []ActorState{
		DefaultInitActorState,
		DefaultRewardActorState,
		DefaultBurntFundsActorState,
		DefaultStoragePowerActorState,
		DefaultStorageMarketActorState,
		DefaultSystemActorState,
		DefaultCronActorState,
	}
}

func initializeStoreWithAdtRoots(store adt_spec.Store) error {
	var err error
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

	EmptyDeadlinesCid, err = store.Put(context.TODO(), &miner.Deadline{
		Partitions:        EmptyArrayCid,
		ExpirationsEpochs: EmptyArrayCid,
		PostSubmissions:   abi_spec.NewBitField(),
		EarlyTerminations: abi_spec.NewBitField(),
		LiveSectors:       0,
	})
	if err != nil {
		return err
	}

	emptyBitfield := bitfield.NewFromSet(nil)
	EmptyBitfieldCid, err = store.Put(context.TODO(), emptyBitfield)
	if err != nil {
		return err
	}

	return nil
}

type mockStore struct {
	ctx context.Context
	cbor.IpldStore
}

func newMockStore() *mockStore {
	bs := blockstore.NewBlockstore(datastore.NewMapDatastore())
	cst := cbor.NewCborStore(bs)
	return &mockStore{
		ctx:       context.Background(),
		IpldStore: cst,
	}
}

func (m mockStore) Context() context.Context {
	return m.ctx
}

func NewTestDriver() *TestDriver {
	syscalls := NewChainValidationSysCalls()
	stateWrapper := NewStateWrapper()
	applier := NewApplier(stateWrapper, func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return syscalls
	})

	sd := NewStateDriver(stateWrapper, newKeyManager())

	err := initializeStoreWithAdtRoots(AsStore(sd.st))
	require.NoError(T, err)

	for _, acts := range DefaultBuiltinActorsState {
		_, _, err := sd.State().CreateActor(acts.Code, acts.Addr, acts.Balance, acts.State)
		require.NoError(T, err)
	}

	minerActorIDAddr := sd.newMinerAccountActor(TestSealProofType, abi_spec.ChainEpoch(0))

	exeCtx := vtypes.NewExecutionContext(1, minerActorIDAddr)
	producer := chain.NewMessageProducer(1000000000, big_spec.NewInt(1)) // gas limit ; gas price

	checkExit := true
	checkRet := true
	config := NewConfig(checkExit, checkRet)

	vector := schema.TestVector{
		Class:    schema.ClassMessage,
		Selector: "",
		Meta: &schema.Metadata{
			ID:      "TK",
			Version: "TK",
			Gen: schema.GenerationData{
				Source:  "TK",
				Version: "TK",
			},
		},
		Pre: &schema.Preconditions{
			StateTree: &schema.StateTree{},
		},
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{},
		},
	}

	return &TestDriver{
		StateDriver: sd,

		MessageProducer: producer,
		ExeCtx:          exeCtx,
		Config:          config,
		SysCalls:        syscalls,

		applier: applier,
		Vector:  &vector,
	}
}

type ActorState struct {
	Addr    address.Address
	Balance abi_spec.TokenAmount
	Code    cid.Cid
	State   runtime_spec.CBORMarshaler
}

type TestDriver struct {
	*StateDriver
	applier *Applier

	MessageProducer      *chain.MessageProducer
	TipSetMessageBuilder *TipSetMessageBuilder
	ExeCtx               *vtypes.ExecutionContext

	Config *Config

	SysCalls *ChainValidationSysCalls
	// Vector is the test vector that is used when methods are called on the
	// driver that apply messages.
	Vector *schema.TestVector
}

//
// Unsigned Message Appliers
//

func (td *TestDriver) ApplyMessage(msg *types.Message) vtypes.ApplyMessageResult {
	result := td.applyMessage(msg)
	return result
}

func (td *TestDriver) ApplyOk(msg *types.Message) vtypes.ApplyMessageResult {
	return td.ApplyExpect(msg, EmptyReturnValue)
}

func (td *TestDriver) ApplyExpect(msg *types.Message, retval []byte) vtypes.ApplyMessageResult {
	return td.applyMessageExpectCodeAndReturn(msg, exitcode.Ok, retval)
}

func (td *TestDriver) ApplyFailure(msg *types.Message, code exitcode.ExitCode) vtypes.ApplyMessageResult {
	return td.applyMessageExpectCodeAndReturn(msg, code, EmptyReturnValue)
}

func (td *TestDriver) applyMessageExpectCodeAndReturn(msg *types.Message, code exitcode.ExitCode, retval []byte) vtypes.ApplyMessageResult {
	result := td.applyMessage(msg)
	td.validateResult(result, code, retval)
	return result
}

func (td *TestDriver) applyMessage(msg *types.Message) (result vtypes.ApplyMessageResult) {
	defer func() {
		if r := recover(); r != nil {
			T.Fatalf("message application panicked: %v", r)
		}
	}()

	epoch := td.ExeCtx.Epoch
	result, err := td.applier.ApplyMessage(epoch, msg)
	td.Vector.ApplyMessages = append(
		td.Vector.ApplyMessages,
		schema.Message{Epoch: &epoch, Bytes: chain.MustSerialize(msg)},
	)
	require.NoError(T, err)
	return result
}

//
// Signed Message Appliers
//

func (td *TestDriver) ApplySigned(msg *types.Message) vtypes.ApplyMessageResult {
	result := td.applyMessageSigned(msg)
	return result
}

func (td *TestDriver) ApplySignedOk(msg *types.Message) vtypes.ApplyMessageResult {
	return td.ApplySignedExpect(msg, EmptyReturnValue)
}

func (td *TestDriver) ApplySignedExpect(msg *types.Message, retval []byte) vtypes.ApplyMessageResult {
	return td.applyMessageSignedExpectCodeAndReturn(msg, exitcode.Ok, retval)
}

func (td *TestDriver) ApplySignedFailure(msg *types.Message, code exitcode.ExitCode) vtypes.ApplyMessageResult {
	return td.applyMessageExpectCodeAndReturn(msg, code, EmptyReturnValue)
}

func (td *TestDriver) applyMessageSignedExpectCodeAndReturn(msg *types.Message, code exitcode.ExitCode, retval []byte) vtypes.ApplyMessageResult {
	result := td.applyMessageSigned(msg)
	td.validateResult(result, code, retval)
	return result
}
func (td *TestDriver) applyMessageSigned(msg *types.Message) (result vtypes.ApplyMessageResult) {
	defer func() {
		if r := recover(); r != nil {
			T.Fatalf("message application panicked: %v", r)
		}
	}()
	serMsg, err := msg.Serialize()
	require.NoError(T, err)

	msgSig, err := td.Wallet().Sign(msg.From, serMsg)
	require.NoError(T, err)

	smsgs := &types.SignedMessage{
		Message:   *msg,
		Signature: msgSig,
	}
	epoch := td.ExeCtx.Epoch
	result, err = td.applier.ApplySignedMessage(epoch, smsgs)
	td.Vector.ApplyMessages = append(
		td.Vector.ApplyMessages,
		schema.Message{Epoch: &epoch, Bytes: chain.MustSerialize(smsgs)},
	)
	require.NoError(T, err)
	return result
}

func (td *TestDriver) validateResult(result vtypes.ApplyMessageResult, code exitcode.ExitCode, retval []byte) {
	if td.Config.ValidateExitCode() {
		assert.Equal(T, code, result.Receipt.ExitCode, "Expected ExitCode: %s Actual ExitCode: %s", code.Error(), result.Receipt.ExitCode.Error())
	}
	if td.Config.ValidateReturnValue() {
		assert.Equal(T, retval, result.Receipt.ReturnValue, "Expected ReturnValue: %v Actual ReturnValue: %v", retval, result.Receipt.ReturnValue)
	}
}

func (td *TestDriver) AssertNoActor(addr address.Address) {
	_, err := td.State().Actor(addr)
	assert.Error(T, err, "expected no such actor %s", addr)
}

func (td *TestDriver) GetBalance(addr address.Address) abi_spec.TokenAmount {
	actr, err := td.State().Actor(addr)
	require.NoError(T, err)
	return actr.Balance()
}

func (td *TestDriver) GetHead(addr address.Address) cid.Cid {
	actr, err := td.State().Actor(addr)
	require.NoError(T, err)
	return actr.Head()
}

// AssertBalance checks an actor has an expected balance.
func (td *TestDriver) AssertBalance(addr address.Address, expected abi_spec.TokenAmount) {
	actr, err := td.State().Actor(addr)
	require.NoError(T, err)
	assert.Equal(T, expected, actr.Balance(), fmt.Sprintf("expected actor %s balance: %s, actual balance: %s", addr, expected, actr.Balance()))
}

// Checks an actor's balance and callSeqNum.
func (td *TestDriver) AssertActor(addr address.Address, balance abi_spec.TokenAmount, callSeqNum uint64) {
	actr, err := td.State().Actor(addr)
	require.NoError(T, err)
	assert.Equal(T, balance, actr.Balance(), fmt.Sprintf("expected actor %s balance: %s, actual balance: %s", addr, balance, actr.Balance()))
	assert.Equal(T, callSeqNum, actr.CallSeqNum(), fmt.Sprintf("expected actor %s callSeqNum: %d, actual : %d", addr, callSeqNum, actr.CallSeqNum()))
}

func (td *TestDriver) AssertHead(addr address.Address, expected cid.Cid) {
	head := td.GetHead(addr)
	assert.Equal(T, expected, head, "expected actor %s head %s, actual %s", addr, expected, head)
}

func (td *TestDriver) AssertBalanceCallback(addr address.Address, thing func(actorBalance abi_spec.TokenAmount) bool) {
	actr, err := td.State().Actor(addr)
	require.NoError(T, err)
	assert.True(T, thing(actr.Balance()))
}

func (td *TestDriver) AssertMultisigTransaction(multisigAddr address.Address, txnID multisig_spec.TxnID, txn multisig_spec.Transaction) {
	var msState multisig_spec.State
	td.GetActorState(multisigAddr, &msState)

	txnMap, err := adt_spec.AsMap(AsStore(td.State()), msState.PendingTxns)
	require.NoError(T, err)

	var actualTxn multisig_spec.Transaction
	found, err := txnMap.Get(txnID, &actualTxn)
	require.NoError(T, err)
	require.True(T, found)

	assert.Equal(T, txn, actualTxn)
}

func (td *TestDriver) AssertMultisigContainsTransaction(multisigAddr address.Address, txnID multisig_spec.TxnID, contains bool) {
	var msState multisig_spec.State
	td.GetActorState(multisigAddr, &msState)

	txnMap, err := adt_spec.AsMap(AsStore(td.State()), msState.PendingTxns)
	require.NoError(T, err)

	var actualTxn multisig_spec.Transaction
	found, err := txnMap.Get(txnID, &actualTxn)
	require.NoError(T, err)

	assert.Equal(T, contains, found)

}

func (td *TestDriver) AssertMultisigState(multisigAddr address.Address, expected multisig_spec.State) {
	var msState multisig_spec.State
	td.GetActorState(multisigAddr, &msState)
	assert.NotNil(T, msState)
	assert.Equal(T, expected.InitialBalance, msState.InitialBalance, fmt.Sprintf("expected InitialBalance: %v, actual InitialBalance: %v", expected.InitialBalance, msState.InitialBalance))
	assert.Equal(T, expected.NextTxnID, msState.NextTxnID, fmt.Sprintf("expected NextTxnID: %v, actual NextTxnID: %v", expected.NextTxnID, msState.NextTxnID))
	assert.Equal(T, expected.NumApprovalsThreshold, msState.NumApprovalsThreshold, fmt.Sprintf("expected NumApprovalsThreshold: %v, actual NumApprovalsThreshold: %v", expected.NumApprovalsThreshold, msState.NumApprovalsThreshold))
	assert.Equal(T, expected.StartEpoch, msState.StartEpoch, fmt.Sprintf("expected StartEpoch: %v, actual StartEpoch: %v", expected.StartEpoch, msState.StartEpoch))
	assert.Equal(T, expected.UnlockDuration, msState.UnlockDuration, fmt.Sprintf("expected UnlockDuration: %v, actual UnlockDuration: %v", expected.UnlockDuration, msState.UnlockDuration))

	for _, e := range expected.Signers {
		assert.Contains(T, msState.Signers, e, fmt.Sprintf("expected Signer: %v, actual Signer: %v", e, msState.Signers))
	}
}

func (td *TestDriver) ComputeInitActorExecReturn(from address.Address, originatorCallSeq uint64, newActorAddressCount uint64, expectedNewAddr address.Address) init_spec.ExecReturn {
	return computeInitActorExecReturn(from, originatorCallSeq, newActorAddressCount, expectedNewAddr)
}

func computeInitActorExecReturn(from address.Address, originatorCallSeq uint64, newActorAddressCount uint64, expectedNewAddr address.Address) init_spec.ExecReturn {
	buf := new(bytes.Buffer)
	if from.Protocol() == address.ID {
		T.Fatal("cannot compute init actor address return from ID address", from)
	}

	require.NoError(T, from.MarshalCBOR(buf))
	require.NoError(T, binary.Write(buf, binary.BigEndian, originatorCallSeq))
	require.NoError(T, binary.Write(buf, binary.BigEndian, newActorAddressCount))

	out, err := address.NewActorAddress(buf.Bytes())
	require.NoError(T, err)

	return init_spec.ExecReturn{
		IDAddress:     expectedNewAddr,
		RobustAddress: out,
	}
}

func (td *TestDriver) MustCreateAndVerifyMultisigActor(nonce uint64, value abi_spec.TokenAmount, multisigAddr address.Address, from address.Address, params *multisig_spec.ConstructorParams, code exitcode.ExitCode, retval []byte) {
	/* Create the Multisig actor*/
	td.applyMessageExpectCodeAndReturn(
		td.MessageProducer.CreateMultisigActor(from, params.Signers, params.UnlockDuration, params.NumApprovalsThreshold, chain.Nonce(nonce), chain.Value(value)),
		code, retval)
	/* Assert the actor state was setup as expected */
	pendingTxMapRoot, err := adt_spec.MakeEmptyMap(newMockStore()).Root()
	require.NoError(T, err)
	initialBalance := big_spec.Zero()
	startEpoch := abi_spec.ChainEpoch(0)
	if params.UnlockDuration > 0 {
		initialBalance = value
		startEpoch = td.ExeCtx.Epoch
	}
	td.AssertMultisigState(multisigAddr, multisig_spec.State{
		NextTxnID:      0,
		InitialBalance: initialBalance,
		StartEpoch:     startEpoch,

		Signers:               params.Signers,
		UnlockDuration:        params.UnlockDuration,
		NumApprovalsThreshold: params.NumApprovalsThreshold,

		PendingTxns: pendingTxMapRoot,
	})
	td.AssertBalance(multisigAddr, value)
}

type RewardSummary struct {
	Treasury           abi_spec.TokenAmount
	SimpleSupply       abi_spec.TokenAmount
	BaselineSupply     abi_spec.TokenAmount
	NextPerEpochReward abi_spec.TokenAmount
	NextPerBlockReward abi_spec.TokenAmount
}

func (td *TestDriver) GetRewardSummary() *RewardSummary {
	var rst reward_spec.State
	td.GetActorState(builtin_spec.RewardActorAddr, &rst)

	return &RewardSummary{
		Treasury:           td.GetBalance(builtin_spec.RewardActorAddr),
		NextPerEpochReward: rst.ThisEpochReward,
		NextPerBlockReward: big_spec.Div(rst.ThisEpochReward, big_spec.NewInt(builtin_spec.ExpectedLeadersPerEpoch)),
	}
}

func (td *TestDriver) GetStateRoot() cid.Cid {
	return td.st.stateRoot
}

func (td *TestDriver) MustMarshalGzippedCAR(roots ...cid.Cid) []byte {
	var b bytes.Buffer
	gw := gzip.NewWriter(&b)

	err := td.MarshalCAR(gw, roots...)
	if err != nil {
		panic(err)
	}

	gw.Close()
	return b.Bytes()
}

func (td *TestDriver) MarshalCAR(w io.Writer, roots ...cid.Cid) error {
	ctx := context.Background()

	offl := offline.Exchange(td.st.bs)
	blkserv := blockservice.New(td.st.bs, offl)
	dserv := merkledag.NewDAGService(blkserv)

	var cids []cid.Cid
	cids = append(cids, roots...)
	if err := car.WriteCarWithWalker(ctx, dserv, cids, w, walker); err != nil {
		return fmt.Errorf("failed to write car file: %w", err)
	}

	return nil
}

func walker(nd format.Node) (out []*format.Link, err error) {
	for _, link := range nd.Links() {
		if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
			continue
		}
		out = append(out, link)
	}

	return out, nil
}

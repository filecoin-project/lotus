package kit

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-varint"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/lib/sigs"
)

// EVM groups EVM-related actions.
type EVM struct{ *TestFullNode }

func (f *TestFullNode) EVM() *EVM {
	return &EVM{f}
}

// SignLegacyEIP155Transaction signs a legacy Homstead Ethereum transaction in place with the supplied private key.
func (e *EVM) SignLegacyEIP155Transaction(tx *ethtypes.EthLegacy155TxArgs, privKey []byte, chainID big.Int) {
	preimage, err := tx.ToRlpUnsignedMsg()
	require.NoError(e.t, err)

	// sign the RLP payload
	signature, err := sigs.Sign(crypto.SigTypeDelegated, privKey, preimage)
	require.NoError(e.t, err)

	signature.Data = append([]byte{ethtypes.EthLegacy155TxSignaturePrefix}, signature.Data...)

	chainIdMul := big.Mul(chainID, big.NewInt(2))
	vVal := big.Add(chainIdMul, big.NewIntUnsigned(35))

	switch signature.Data[len(signature.Data)-1] {
	case 0:
		vVal = big.Add(vVal, big.NewInt(0))
	case 1:
		vVal = big.Add(vVal, big.NewInt(1))
	}

	signature.Data = append(signature.Data[:65], vVal.Int.Bytes()...)

	err = tx.InitialiseSignature(*signature)
	require.NoError(e.t, err)
}

// SignLegacyHomesteadTransaction signs a legacy Homstead Ethereum transaction in place with the supplied private key.
func (e *EVM) SignLegacyHomesteadTransaction(tx *ethtypes.EthLegacyHomesteadTxArgs, privKey []byte) {
	preimage, err := tx.ToRlpUnsignedMsg()
	require.NoError(e.t, err)

	// sign the RLP payload
	signature, err := sigs.Sign(crypto.SigTypeDelegated, privKey, preimage)
	require.NoError(e.t, err)

	signature.Data = append([]byte{ethtypes.EthLegacyHomesteadTxSignaturePrefix}, signature.Data...)
	signature.Data[len(signature.Data)-1] += 27

	err = tx.InitialiseSignature(*signature)
	require.NoError(e.t, err)
}

func (e *EVM) DeployContractWithValue(ctx context.Context, sender address.Address, bytecode []byte, value big.Int) eam.CreateReturn {
	require := require.New(e.t)

	method := builtintypes.MethodsEAM.CreateExternal
	initcode := abi.CborBytes(bytecode)
	params, errActors := actors.SerializeParams(&initcode)
	require.NoError(errActors)

	msg := &types.Message{
		To:     builtintypes.EthereumAddressManagerActorAddr,
		From:   sender,
		Value:  value,
		Method: method,
		Params: params,
	}

	e.t.Log("sending create message")
	smsg, err := e.MpoolPushMessage(ctx, msg, nil)
	require.NoError(err)

	e.t.Log("waiting for message to execute")
	wait, err := e.StateWaitMsg(ctx, smsg.Cid(), 3, 0, false)
	require.NoError(err)

	require.True(wait.Receipt.ExitCode.IsSuccess(), "contract installation failed")

	var result eam.CreateReturn
	r := bytes.NewReader(wait.Receipt.Return)
	err = result.UnmarshalCBOR(r)
	require.NoError(err)

	return result
}
func (e *EVM) DeployContract(ctx context.Context, sender address.Address, bytecode []byte) eam.CreateReturn {
	return e.DeployContractWithValue(ctx, sender, bytecode, big.Zero())
}

func (e *EVM) DeployContractFromFilenameWithValue(ctx context.Context, binFilename string, value big.Int) (address.Address, address.Address) {
	contractHex, err := os.ReadFile(binFilename)
	require.NoError(e.t, err)

	// strip any trailing newlines from the file
	contractHex = bytes.TrimRight(contractHex, "\n")

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(e.t, err)

	fromAddr, err := e.WalletDefaultAddress(ctx)
	require.NoError(e.t, err)

	result := e.DeployContractWithValue(ctx, fromAddr, contract, value)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(e.t, err)
	return fromAddr, idAddr
}
func (e *EVM) DeployContractFromFilename(ctx context.Context, binFilename string) (address.Address, address.Address) {
	return e.DeployContractFromFilenameWithValue(ctx, binFilename, big.Zero())
}

func (e *EVM) InvokeSolidity(ctx context.Context, sender address.Address, target address.Address, selector []byte, inputData []byte) (*api.MsgLookup, error) {
	return e.InvokeSolidityWithValue(ctx, sender, target, selector, inputData, big.Zero())
}

func (e *EVM) InvokeSolidityWithValue(ctx context.Context, sender address.Address, target address.Address, selector []byte, inputData []byte, value big.Int) (*api.MsgLookup, error) {
	params := append(selector, inputData...)
	var buffer bytes.Buffer
	err := cbg.WriteByteArray(&buffer, params)
	if err != nil {
		return nil, err
	}
	params = buffer.Bytes()

	msg := &types.Message{
		To:       target,
		From:     sender,
		Value:    value,
		Method:   builtintypes.MethodsEVM.InvokeContract,
		GasLimit: buildconstants.BlockGasLimit, // note: we hardcode block gas limit due to slightly broken gas estimation - https://github.com/filecoin-project/lotus/issues/10041
		Params:   params,
	}

	e.t.Log("sending invoke message")
	smsg, err := e.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	e.t.Log("waiting for message to execute")
	wait, err := e.StateWaitMsg(ctx, smsg.Cid(), 3, 0, false)
	if err != nil {
		return nil, err
	}
	if !wait.Receipt.ExitCode.IsSuccess() {
		result, err := e.StateReplay(ctx, types.EmptyTSK, wait.Message)
		require.NoError(e.t, err)
		e.t.Log(result.Error)
	}
	return wait, nil
}

// LoadEvents loads all events in an event AMT.
func (e *EVM) LoadEvents(ctx context.Context, eventsRoot cid.Cid) []types.Event {
	require := require.New(e.t)

	s := &apiIpldStore{ctx, e}
	amt, err := amt4.LoadAMT(ctx, s, eventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
	require.NoError(err)

	ret := make([]types.Event, 0, amt.Len())
	err = amt.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
		var evt types.Event
		if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
			return err
		}
		ret = append(ret, evt)
		return nil
	})
	require.NoError(err)
	return ret
}

func (e *EVM) NewAccount() (*key.Key, ethtypes.EthAddress, address.Address) {
	// Generate a secp256k1 key; this will back the Ethereum identity.
	key, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(e.t, err)

	ethAddr, err := ethtypes.EthAddressFromPubKey(key.PublicKey)
	require.NoError(e.t, err)

	ea, err := ethtypes.CastEthAddress(ethAddr)
	require.NoError(e.t, err)

	addr, err := ea.ToFilecoinAddress()
	require.NoError(e.t, err)

	return key, *(*ethtypes.EthAddress)(ethAddr), addr
}

// AssertAddressBalanceConsistent checks that the balance reported via the
// Filecoin and Ethereum operations for an f410 address is identical, returning
// the balance.
func (e *EVM) AssertAddressBalanceConsistent(ctx context.Context, addr address.Address) big.Int {
	// Validate the arg is an f410 address.
	require.Equal(e.t, address.Delegated, addr.Protocol())
	payload := addr.Payload()
	namespace, _, err := varint.FromUvarint(payload)
	require.NoError(e.t, err)
	require.Equal(e.t, builtintypes.EthereumAddressManagerActorID, namespace)

	fbal, err := e.WalletBalance(ctx, addr)
	require.NoError(e.t, err)

	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(addr)
	require.NoError(e.t, err)

	ebal, err := e.EthGetBalance(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(e.t, err)

	require.Equal(e.t, fbal, types.BigInt(ebal))
	return fbal
}

// SignTransaction signs an Ethereum transaction in place with the supplied private key.
func (e *EVM) SignTransaction(tx *ethtypes.Eth1559TxArgs, privKey []byte) {
	preimage, err := tx.ToRlpUnsignedMsg()
	require.NoError(e.t, err)

	// sign the RLP payload
	signature, err := sigs.Sign(crypto.SigTypeDelegated, privKey, preimage)
	require.NoError(e.t, err)

	err = tx.InitialiseSignature(*signature)
	require.NoError(e.t, err)
}

// SubmitTransaction submits the transaction via the Eth endpoint.
func (e *EVM) SubmitTransaction(ctx context.Context, tx ethtypes.EthTransaction) ethtypes.EthHash {
	signed, err := tx.ToRlpSignedMsg()
	require.NoError(e.t, err)

	hash, err := e.EthSendRawTransaction(ctx, signed)
	require.NoError(e.t, err)

	return hash
}

// ComputeContractAddress computes the address of a contract deployed by the
// specified address with the specified nonce.
func (e *EVM) ComputeContractAddress(deployer ethtypes.EthAddress, nonce uint64) ethtypes.EthAddress {
	nonceRlp, err := formatInt(int(nonce))
	require.NoError(e.t, err)

	encoded, err := ethtypes.EncodeRLP([]interface{}{
		deployer[:],
		nonceRlp,
	})
	require.NoError(e.t, err)

	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(encoded)
	return *(*ethtypes.EthAddress)(hasher.Sum(nil)[12:])
}

// GetEthBlockFromWait returns and eth block from a wait return.
// This necessarily goes back one parent in the chain because wait is one block ahead of execution.
func (e *EVM) GetEthBlockFromWait(ctx context.Context, wait *api.MsgLookup) ethtypes.EthBlock {
	c, err := wait.TipSet.Cid()
	require.NoError(e.t, err)
	hash, err := ethtypes.EthHashFromCid(c)
	require.NoError(e.t, err)

	ethBlockParent, err := e.EthGetBlockByHash(ctx, hash, true)
	require.NoError(e.t, err)
	ethBlock, err := e.EthGetBlockByHash(ctx, ethBlockParent.ParentHash, true)
	require.NoError(e.t, err)

	return ethBlock
}

func (e *EVM) InvokeContractByFuncName(ctx context.Context, fromAddr address.Address, idAddr address.Address, funcSignature string, inputData []byte) ([]byte, *api.MsgLookup, error) {
	entryPoint := CalcFuncSignature(funcSignature)
	wait, err := e.InvokeSolidity(ctx, fromAddr, idAddr, entryPoint, inputData)
	if err != nil {
		return nil, wait, err
	}
	if !wait.Receipt.ExitCode.IsSuccess() {
		result, err := e.StateReplay(ctx, types.EmptyTSK, wait.Message)
		require.NoError(e.t, err)
		return nil, wait, errors.New(result.Error)
	}
	result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
	if err != nil {
		return nil, wait, err
	}
	return result, wait, nil
}

func (e *EVM) InvokeContractByFuncNameExpectExit(ctx context.Context, fromAddr address.Address, idAddr address.Address, funcSignature string, inputData []byte, exit exitcode.ExitCode) {
	entryPoint := CalcFuncSignature(funcSignature)
	wait, _ := e.InvokeSolidity(ctx, fromAddr, idAddr, entryPoint, inputData)
	require.Equal(e.t, exit, wait.Receipt.ExitCode)
}

func (e *EVM) WaitTransaction(ctx context.Context, hash ethtypes.EthHash) (*ethtypes.EthTxReceipt, error) {
	retries := 3
	var mcid *cid.Cid
	var err error

	for retries > 0 {
		if mcid, err = e.EthGetMessageCidByTransactionHash(ctx, &hash); err != nil {
			return nil, err
		} else if mcid == nil {
			retries--
			time.Sleep(100 * time.Millisecond)
			continue
		}

		e.WaitMsg(ctx, *mcid)
		return e.EthGetTransactionReceipt(ctx, hash)
	}
	return nil, xerrors.Errorf("couldn't find message CID for txn hash: %s", hash)
}

// CalcFuncSignature returns the first 4 bytes of the hash of the function name and types
func CalcFuncSignature(funcName string) []byte {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(funcName))
	hash := hasher.Sum(nil)
	return hash[:4]
}

// TODO: cleanup and put somewhere reusable.
type apiIpldStore struct {
	ctx context.Context
	api api.FullNode
}

func (ht *apiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *apiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	raw, err := ht.api.ChainReadObj(ctx, c)
	if err != nil {
		return err
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("object does not implement CBORUnmarshaler")
}

func (ht *apiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	panic("No mutations allowed")
}

func formatInt(val int) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int64(val))
	if err != nil {
		return nil, err
	}
	return removeLeadingZeros(buf.Bytes()), nil
}

func removeLeadingZeros(data []byte) []byte {
	firstNonZeroIndex := len(data)
	for i, b := range data {
		if b > 0 {
			firstNonZeroIndex = i
			break
		}
	}
	return data[firstNonZeroIndex:]
}

func SetupFEVMTest(t *testing.T, opts ...interface{}) (context.Context, context.CancelFunc, *TestFullNode) {
	// make all logs extra quiet for fevm tests
	lvl, err := logging.LevelFromString("error")
	if err != nil {
		panic(err)
	}
	logging.SetAllLoggers(lvl)

	blockTime := 100 * time.Millisecond

	opts = append([]interface{}{MockProofs(), ThroughRPC()}, opts...)
	client, _, ens := EnsembleMinimal(t, opts...)
	ens.InterconnectAll().BeginMining(blockTime)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	// require that the initial balance is 10 million FIL in setup
	// this way other tests can count on this initial wallet balance
	fromAddr := client.DefaultKey.Address
	bal, err := client.WalletBalance(ctx, fromAddr)
	require.NoError(t, err)
	originalBalance := types.FromFil(uint64(10_000_000)) // 10 million FIL
	require.Equal(t, originalBalance, bal)

	return ctx, cancel, client
}

func (e *EVM) TransferValueOrFail(ctx context.Context, fromAddr address.Address, toAddr address.Address, sendAmount big.Int) *api.MsgLookup {
	sendMsg := &types.Message{
		From:  fromAddr,
		To:    toAddr,
		Value: sendAmount,
	}
	signedMsg, err := e.MpoolPushMessage(ctx, sendMsg, nil)
	require.NoError(e.t, err)
	mLookup, err := e.StateWaitMsg(ctx, signedMsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(e.t, err)
	require.Equal(e.t, exitcode.Ok, mLookup.Receipt.ExitCode)
	return mLookup
}

func NewEthFilterBuilder() *EthFilterBuilder {
	return &EthFilterBuilder{}
}

type EthFilterBuilder struct {
	filter ethtypes.EthFilterSpec
}

func (e *EthFilterBuilder) Filter() *ethtypes.EthFilterSpec { return &e.filter }

func (e *EthFilterBuilder) FromBlock(v string) *EthFilterBuilder {
	e.filter.FromBlock = &v
	return e
}

func (e *EthFilterBuilder) FromBlockEpoch(v abi.ChainEpoch) *EthFilterBuilder {
	s := ethtypes.EthUint64(v).Hex()
	e.filter.FromBlock = &s
	return e
}

func (e *EthFilterBuilder) ToBlock(v string) *EthFilterBuilder {
	e.filter.ToBlock = &v
	return e
}

func (e *EthFilterBuilder) ToBlockEpoch(v abi.ChainEpoch) *EthFilterBuilder {
	s := ethtypes.EthUint64(v).Hex()
	e.filter.ToBlock = &s
	return e
}

func (e *EthFilterBuilder) BlockHash(h ethtypes.EthHash) *EthFilterBuilder {
	e.filter.BlockHash = &h
	return e
}

func (e *EthFilterBuilder) AddressOneOf(as ...ethtypes.EthAddress) *EthFilterBuilder {
	e.filter.Address = as
	return e
}

func (e *EthFilterBuilder) Topic1OneOf(hs ...ethtypes.EthHash) *EthFilterBuilder {
	if len(e.filter.Topics) == 0 {
		e.filter.Topics = make(ethtypes.EthTopicSpec, 1)
	}
	e.filter.Topics[0] = hs
	return e
}

func (e *EthFilterBuilder) Topic2OneOf(hs ...ethtypes.EthHash) *EthFilterBuilder {
	for len(e.filter.Topics) < 2 {
		e.filter.Topics = append(e.filter.Topics, nil)
	}
	e.filter.Topics[1] = hs
	return e
}

func (e *EthFilterBuilder) Topic3OneOf(hs ...ethtypes.EthHash) *EthFilterBuilder {
	for len(e.filter.Topics) < 3 {
		e.filter.Topics = append(e.filter.Topics, nil)
	}
	e.filter.Topics[2] = hs
	return e
}

func (e *EthFilterBuilder) Topic4OneOf(hs ...ethtypes.EthHash) *EthFilterBuilder {
	for len(e.filter.Topics) < 4 {
		e.filter.Topics = append(e.filter.Topics, nil)
	}
	e.filter.Topics[3] = hs
	return e
}

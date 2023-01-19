package kit

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-varint"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
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

func (e *EVM) DeployContract(ctx context.Context, sender address.Address, bytecode []byte) eam.CreateReturn {
	require := require.New(e.t)

	nonce, err := e.MpoolGetNonce(ctx, sender)
	if err != nil {
		nonce = 0 // assume a zero nonce on error (e.g. sender doesn't exist).
	}

	var salt [32]byte
	binary.BigEndian.PutUint64(salt[:], nonce)

	method := builtintypes.MethodsEAM.CreateExternal
	initcode := abi.CborBytes(bytecode)
	params, err := actors.SerializeParams(&initcode)
	require.NoError(err)

	msg := &types.Message{
		To:     builtintypes.EthereumAddressManagerActorAddr,
		From:   sender,
		Value:  big.Zero(),
		Method: method,
		Params: params,
	}

	e.t.Log("sending create message")
	smsg, err := e.MpoolPushMessage(ctx, msg, nil)
	require.NoError(err)

	e.t.Log("waiting for message to execute")
	wait, err := e.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
	require.NoError(err)

	require.True(wait.Receipt.ExitCode.IsSuccess(), "contract installation failed")

	var result eam.CreateReturn
	r := bytes.NewReader(wait.Receipt.Return)
	err = result.UnmarshalCBOR(r)
	require.NoError(err)

	return result
}

func (e *EVM) DeployContractFromFilename(ctx context.Context, binFilename string) (address.Address, address.Address) {
	contractHex, err := os.ReadFile(binFilename)
	require.NoError(e.t, err)

	// strip any trailing newlines from the file
	contractHex = bytes.TrimRight(contractHex, "\n")

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(e.t, err)

	fromAddr, err := e.WalletDefaultAddress(ctx)
	require.NoError(e.t, err)

	result := e.DeployContract(ctx, fromAddr, contract)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(e.t, err)
	return fromAddr, idAddr
}

func (e *EVM) InvokeSolidity(ctx context.Context, sender address.Address, target address.Address, selector []byte, inputData []byte) *api.MsgLookup {
	require := require.New(e.t)

	params := append(selector, inputData...)
	var buffer bytes.Buffer
	err := cbg.WriteByteArray(&buffer, params)
	require.NoError(err)
	params = buffer.Bytes()

	msg := &types.Message{
		To:     target,
		From:   sender,
		Value:  big.Zero(),
		Method: builtintypes.MethodsEVM.InvokeContract,
		Params: params,
	}

	e.t.Log("sending invoke message")
	smsg, err := e.MpoolPushMessage(ctx, msg, nil)
	require.NoError(err)

	e.t.Log("waiting for message to execute")
	wait, err := e.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
	require.NoError(err)

	return wait
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

	ebal, err := e.EthGetBalance(ctx, ethAddr, "latest")
	require.NoError(e.t, err)

	require.Equal(e.t, fbal, types.BigInt(ebal))
	return fbal
}

// SignTransaction signs an Ethereum transaction in place with the supplied private key.
func (e *EVM) SignTransaction(tx *ethtypes.EthTxArgs, privKey []byte) {
	preimage, err := tx.ToRlpUnsignedMsg()
	require.NoError(e.t, err)

	// sign the RLP payload
	signature, err := sigs.Sign(crypto.SigTypeDelegated, privKey, preimage)
	require.NoError(e.t, err)

	r, s, v, err := ethtypes.RecoverSignature(*signature)
	require.NoError(e.t, err)

	tx.V = big.Int(v)
	tx.R = big.Int(r)
	tx.S = big.Int(s)
}

// SubmitTransaction submits the transaction via the Eth endpoint.
func (e *EVM) SubmitTransaction(ctx context.Context, tx *ethtypes.EthTxArgs) ethtypes.EthHash {
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

func (e *EVM) InvokeContractByFuncName(ctx context.Context, fromAddr address.Address, idAddr address.Address, funcSignature string, inputData []byte) []byte {
	entryPoint := CalcFuncSignature(funcSignature)
	wait := e.InvokeSolidity(ctx, fromAddr, idAddr, entryPoint, inputData)
	require.True(e.t, wait.Receipt.ExitCode.IsSuccess(), "contract execution failed")
	result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
	require.NoError(e.t, err)
	return result
}

// function signatures are the first 4 bytes of the hash of the function name and types
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

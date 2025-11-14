package eth

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/vm"
)

// The address used in messages to actors that have since been deleted.
//
// 0xff0000000000000000000000ffffffffffffffff
var revertedEthAddress ethtypes.EthAddress

func init() {
	revertedEthAddress[0] = 0xff
	for i := 20 - 8; i < 20; i++ {
		revertedEthAddress[i] = 0xff
	}
}

func newEthBlockFromFilecoinTipSet(ctx context.Context, ts *types.TipSet, fullTxInfo bool, cs ChainStore, sm StateManager) (ethtypes.EthBlock, error) {
	parentKeyCid, err := ts.Parents().Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	parentBlkHash, err := ethtypes.EthHashFromCid(parentKeyCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	bn := ethtypes.EthUint64(ts.Height())

	tsk := ts.Key()
	blkCid, err := tsk.Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	blkHash, err := ethtypes.EthHashFromCid(blkCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	stRoot, msgs, rcpts, err := executeTipset(ctx, ts, cs, sm)
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("failed to retrieve messages and receipts: %w", err)
	}

	st, err := sm.StateTree(stRoot)
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("failed to load state-tree root %q: %w", stRoot, err)
	}

	block := ethtypes.NewEthBlock(len(msgs) > 0, len(ts.Blocks()))

	gasUsed := int64(0)
	for i, msg := range msgs {
		rcpt := rcpts[i]
		ti := ethtypes.EthUint64(i)
		gasUsed += rcpt.GasUsed
		var smsg *types.SignedMessage
		switch msg := msg.(type) {
		case *types.SignedMessage:
			smsg = msg
		case *types.Message:
			smsg = &types.SignedMessage{
				Message: *msg,
				Signature: crypto.Signature{
					Type: crypto.SigTypeBLS,
				},
			}
		default:
			return ethtypes.EthBlock{}, xerrors.Errorf("failed to get signed msg %s: %w", msg.Cid(), err)
		}
		tx, err := newEthTxFromSignedMessage(smsg, st)
		if err != nil {
			return ethtypes.EthBlock{}, xerrors.Errorf("failed to convert msg to ethTx: %w", err)
		}

		tx.BlockHash = &blkHash
		tx.BlockNumber = &bn
		tx.TransactionIndex = &ti

		if fullTxInfo {
			block.Transactions = append(block.Transactions, tx)
		} else {
			block.Transactions = append(block.Transactions, tx.Hash.String())
		}
	}

	block.Hash = blkHash
	block.Number = bn
	block.ParentHash = parentBlkHash
	block.Timestamp = ethtypes.EthUint64(ts.Blocks()[0].Timestamp)
	block.BaseFeePerGas = ethtypes.EthBigInt{Int: ts.Blocks()[0].ParentBaseFee.Int}
	block.GasUsed = ethtypes.EthUint64(gasUsed)
	return block, nil
}

func executeTipset(ctx context.Context, ts *types.TipSet, cs ChainStore, sm StateManager) (cid.Cid, []types.ChainMsg, []types.MessageReceipt, error) {
	msgs, err := cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return cid.Undef, nil, nil, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	stRoot, rcptRoot, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return cid.Undef, nil, nil, xerrors.Errorf("failed to compute tipset state: %w", err)
	}

	rcpts, err := cs.ReadReceipts(ctx, rcptRoot)
	if err != nil {
		return cid.Undef, nil, nil, xerrors.Errorf("error loading receipts for tipset: %v: %w", ts, err)
	}

	if len(msgs) != len(rcpts) {
		return cid.Undef, nil, nil, xerrors.Errorf("receipts and message array lengths didn't match for tipset: %v: %w", ts, err)
	}

	return stRoot, msgs, rcpts, nil
}

// Note: ParseEthRevert moved to chain/types/ethtypes. Use ethtypes.ParseEthRevert.
// lookupEthAddress makes its best effort at finding the Ethereum address for a
// Filecoin address. It does the following:
//
//  1. If the supplied address is an f410 address, we return its payload as the EthAddress.
//  2. Otherwise (f0, f1, f2, f3), we look up the actor on the state tree. If it has a delegated address, we return it if it's f410 address.
//  3. Otherwise, we fall back to returning a masked ID Ethereum address. If the supplied address is an f0 address, we
//     use that ID to form the masked ID address.
//  4. Otherwise, we fetch the actor's ID from the state tree and form the masked ID with it.
//
// If the actor doesn't exist in the state-tree but we have its ID, we use a masked ID address. It could have been deleted.
func lookupEthAddress(addr address.Address, st *state.StateTree) (ethtypes.EthAddress, error) {
	// Attempt to convert directly, if it's an f4 address.
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(addr)
	if err == nil && !ethAddr.IsMaskedID() {
		return ethAddr, nil
	}

	// Otherwise, resolve the ID addr.
	idAddr, err := st.LookupIDAddress(addr)
	if err != nil {
		return ethtypes.EthAddress{}, err
	}

	// revive:disable:empty-block easier to grok when the cases are explicit

	// Lookup on the target actor and try to get an f410 address.
	if actor, err := st.GetActor(idAddr); errors.Is(err, types.ErrActorNotFound) {
		// Not found -> use a masked ID address
	} else if err != nil {
		// Any other error -> fail.
		return ethtypes.EthAddress{}, err
	} else if actor.DelegatedAddress == nil {
		// No delegated address -> use masked ID address.
	} else if ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(*actor.DelegatedAddress); err == nil && !ethAddr.IsMaskedID() {
		// Conversable into an eth address, use it.
		return ethAddr, nil
	}

	// Otherwise, use the masked address.
	return ethtypes.EthAddressFromFilecoinAddress(idAddr)
}

func parseEthTopics(topics ethtypes.EthTopicSpec) (map[string][][]byte, error) {
	keys := map[string][][]byte{}
	for idx, vals := range topics {
		if len(vals) == 0 {
			continue
		}
		// Ethereum topics are emitted using `LOG{0..4}` opcodes resulting in topics1..4
		key := fmt.Sprintf("t%d", idx+1)
		for _, v := range vals {
			v := v // copy the ethhash to avoid repeatedly referencing the same one.
			keys[key] = append(keys[key], v[:])
		}
	}
	return keys, nil
}

func getTransactionHashByCid(ctx context.Context, cs ChainStore, c cid.Cid) (ethtypes.EthHash, error) {
	smsg, err := cs.GetSignedMessage(ctx, c)
	if err == nil {
		// This is an Eth Tx, Secp message, Or BLS message in the mpool
		return ethTxHashFromSignedMessage(smsg)
	}

	_, err = cs.GetMessage(ctx, c)
	if err == nil {
		// This is a BLS message
		return ethtypes.EthHashFromCid(c)
	}

	return ethtypes.EmptyEthHash, nil
}

func ethTxHashFromSignedMessage(smsg *types.SignedMessage) (ethtypes.EthHash, error) {
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		tx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(smsg)
		if err != nil {
			return ethtypes.EthHash{}, xerrors.Errorf("failed to convert from signed message: %w", err)
		}

		return tx.TxHash()
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 {
		return ethtypes.EthHashFromCid(smsg.Cid())
	}
	// else BLS message
	return ethtypes.EthHashFromCid(smsg.Message.Cid())
}

func newEthTxFromSignedMessage(smsg *types.SignedMessage, st *state.StateTree) (ethtypes.EthTx, error) {
	var tx ethtypes.EthTx
	var err error

	// This is an eth tx
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(smsg)
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to convert from signed message: %w", err)
		}
		tx, err = ethTx.ToEthTx(smsg)
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to convert from signed message: %w", err)
		}
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 { // Secp Filecoin Message
		tx, err = ethTxFromNativeMessage(smsg.VMMessage(), st)
		if err != nil {
			return ethtypes.EthTx{}, err
		}
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Cid())
		if err != nil {
			return ethtypes.EthTx{}, err
		}
	} else { // BLS Filecoin message
		tx, err = ethTxFromNativeMessage(smsg.VMMessage(), st)
		if err != nil {
			return ethtypes.EthTx{}, err
		}
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Message.Cid())
		if err != nil {
			return ethtypes.EthTx{}, err
		}
	}

	return tx, nil
}

// Convert a native message to an eth transaction.
//
//   - The state-tree must be from after the message was applied (ideally the following tipset).
//   - In some cases, the "to" address may be `0xff0000000000000000000000ffffffffffffffff`. This
//     means that the "to" address has not been assigned in the passed state-tree and can only
//     happen if the transaction reverted.
//
// ethTxFromNativeMessage does NOT populate:
// - BlockHash
// - BlockNumber
// - TransactionIndex
// - Hash
func ethTxFromNativeMessage(msg *types.Message, st *state.StateTree) (ethtypes.EthTx, error) {
	// Lookup the from address. This must succeed.
	from, err := lookupEthAddress(msg.From, st)
	if err != nil {
		return ethtypes.EthTx{}, xerrors.Errorf("failed to lookup sender address %s when converting a native message to an eth txn: %w", msg.From, err)
	}
	// Lookup the to address. If the recipient doesn't exist, we replace the address with a
	// known sentinel address.
	to, err := lookupEthAddress(msg.To, st)
	if err != nil {
		if !errors.Is(err, types.ErrActorNotFound) {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to lookup receiver address %s when converting a native message to an eth txn: %w", msg.To, err)
		}
		to = revertedEthAddress
	}

	// For empty, we use "0" as the codec. Otherwise, we use CBOR for message
	// parameters.
	var codec uint64
	if len(msg.Params) > 0 {
		codec = uint64(multicodec.Cbor)
	}

	maxFeePerGas := ethtypes.EthBigInt(msg.GasFeeCap)
	maxPriorityFeePerGas := ethtypes.EthBigInt(msg.GasPremium)

	// We decode as a native call first.
	ethTx := ethtypes.EthTx{
		To:                   &to,
		From:                 from,
		Input:                encodeFilecoinParamsAsABI(msg.Method, codec, msg.Params),
		Nonce:                ethtypes.EthUint64(msg.Nonce),
		ChainID:              ethtypes.EthUint64(buildconstants.Eip155ChainId),
		Value:                ethtypes.EthBigInt(msg.Value),
		Type:                 ethtypes.EIP1559TxType,
		Gas:                  ethtypes.EthUint64(msg.GasLimit),
		MaxFeePerGas:         &maxFeePerGas,
		MaxPriorityFeePerGas: &maxPriorityFeePerGas,
		AccessList:           []ethtypes.EthHash{},
	}

	// Then we try to see if it's "special". If we fail, we ignore the error and keep treating
	// it as a native message. Unfortunately, the user is free to send garbage that may not
	// properly decode.
	if msg.Method == builtintypes.MethodsEVM.InvokeContract {
		// try to decode it as a contract invocation first.
		if inp, err := decodePayload(msg.Params, codec); err == nil {
			ethTx.Input = []byte(inp)
		}
	} else if msg.To == builtin.EthereumAddressManagerActorAddr && msg.Method == builtintypes.MethodsEAM.CreateExternal {
		// Then, try to decode it as a contract deployment from an EOA.
		if inp, err := decodePayload(msg.Params, codec); err == nil {
			ethTx.Input = []byte(inp)
			ethTx.To = nil
		}
	}

	return ethTx, nil
}

func getSignedMessage(ctx context.Context, cs ChainStore, msgCid cid.Cid) (*types.SignedMessage, error) {
	smsg, err := cs.GetSignedMessage(ctx, msgCid)
	if err != nil {
		// We couldn't find the signed message, it might be a BLS message, so search for a regular message.
		msg, err := cs.GetMessage(ctx, msgCid)
		if err != nil {
			return nil, xerrors.Errorf("failed to find msg %s: %w", msgCid, err)
		}
		smsg = &types.SignedMessage{
			Message: *msg,
			Signature: crypto.Signature{
				Type: crypto.SigTypeBLS,
			},
		}
	}

	return smsg, nil
}

// newEthTxFromMessageLookup creates an ethereum transaction from filecoin message lookup. If a negative txIdx is passed
// into the function, it looks up the transaction index of the message in the tipset, otherwise it uses the txIdx passed into the
// function
func newEthTxFromMessageLookup(
	ctx context.Context,
	msgLookup *api.MsgLookup,
	txIdx int,
	cs ChainStore,
	sm StateManager,
) (ethtypes.EthTx, error) {
	ts, err := cs.LoadTipSet(ctx, msgLookup.TipSet)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	// This tx is located in the parent tipset
	parentTs, err := cs.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	parentTsCid, err := parentTs.Key().Cid()
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	// lookup the transactionIndex
	if txIdx < 0 {
		msgs, err := cs.MessagesForTipset(ctx, parentTs)
		if err != nil {
			return ethtypes.EthTx{}, err
		}
		for i, msg := range msgs {
			if msg.Cid() == msgLookup.Message {
				txIdx = i
				break
			}
		}
		if txIdx < 0 {
			return ethtypes.EthTx{}, xerrors.New("cannot find the msg in the tipset")
		}
	}

	st, err := sm.StateTree(ts.ParentState())
	if err != nil {
		return ethtypes.EthTx{}, xerrors.Errorf("failed to load message state tree: %w", err)
	}

	return newEthTx(ctx, cs, st, parentTs.Height(), parentTsCid, msgLookup.Message, txIdx)
}

func newEthTx(
	ctx context.Context,
	cs ChainStore,
	st *state.StateTree,
	blockHeight abi.ChainEpoch,
	msgTsCid cid.Cid,
	msgCid cid.Cid,
	txIdx int,
) (ethtypes.EthTx, error) {
	smsg, err := getSignedMessage(ctx, cs, msgCid)
	if err != nil {
		return ethtypes.EthTx{}, xerrors.Errorf("failed to get signed msg: %w", err)
	}

	tx, err := newEthTxFromSignedMessage(smsg, st)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	var (
		bn = ethtypes.EthUint64(blockHeight)
		ti = ethtypes.EthUint64(txIdx)
	)

	blkHash, err := ethtypes.EthHashFromCid(msgTsCid)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	tx.BlockHash = &blkHash
	tx.BlockNumber = &bn
	tx.TransactionIndex = &ti

	return tx, nil
}

func newEthTxReceipt(ctx context.Context, tx ethtypes.EthTx, baseFee big.Int, msgReceipt types.MessageReceipt, ev EthEventsInternal) (ethtypes.EthTxReceipt, error) {
	var (
		transactionIndex ethtypes.EthUint64
		blockHash        ethtypes.EthHash
		blockNumber      ethtypes.EthUint64
	)

	if tx.TransactionIndex != nil {
		transactionIndex = *tx.TransactionIndex
	}
	if tx.BlockHash != nil {
		blockHash = *tx.BlockHash
	}
	if tx.BlockNumber != nil {
		blockNumber = *tx.BlockNumber
	}

	txReceipt := ethtypes.EthTxReceipt{
		TransactionHash:  tx.Hash,
		From:             tx.From,
		To:               tx.To,
		TransactionIndex: transactionIndex,
		BlockHash:        blockHash,
		BlockNumber:      blockNumber,
		Type:             tx.Type,
		Logs:             []ethtypes.EthLog{}, // empty log array is compulsory when no logs, or libraries like ethers.js break
		LogsBloom:        ethtypes.NewEmptyEthBloom(),
	}
	if len(tx.AuthorizationList) > 0 {
		txReceipt.AuthorizationList = tx.AuthorizationList
	}

	// Default: derive status from Filecoin exit code.
	txReceipt.Status = 0
	if msgReceipt.ExitCode.IsSuccess() {
		txReceipt.Status = 1
	}

	// EIP-7702: for typed-0x04 routed to EthAccount.ApplyAndCall, the actor always exits OK and embeds status in return.
	if tx.Type == 0x04 && len(msgReceipt.Return) > 0 {
		if st, ok := decodeApplyAndCallReturnStatus(msgReceipt.Return); ok {
			if st != 0 {
				txReceipt.Status = 1
			} else {
				txReceipt.Status = 0
			}
		} // else: malformed return; keep default ExitCode-derived status
	}

	txReceipt.GasUsed = ethtypes.EthUint64(msgReceipt.GasUsed)

	// TODO: handle CumulativeGasUsed
	txReceipt.CumulativeGasUsed = ethtypes.EmptyEthInt

	gasFeeCap, err := tx.GasFeeCap()
	if err != nil {
		return ethtypes.EthTxReceipt{}, xerrors.Errorf("failed to get gas fee cap: %w", err)
	}
	gasPremium, err := tx.GasPremium()
	if err != nil {
		return ethtypes.EthTxReceipt{}, xerrors.Errorf("failed to get gas premium: %w", err)
	}

	gasOutputs := vm.ComputeGasOutputs(msgReceipt.GasUsed, int64(tx.Gas), baseFee, big.Int(gasFeeCap),
		big.Int(gasPremium), true)
	totalSpent := big.Sum(gasOutputs.BaseFeeBurn, gasOutputs.MinerTip, gasOutputs.OverEstimationBurn)

	effectiveGasPrice := big.Zero()
	if msgReceipt.GasUsed > 0 {
		effectiveGasPrice = big.Div(totalSpent, big.NewInt(msgReceipt.GasUsed))
	}
	txReceipt.EffectiveGasPrice = ethtypes.EthBigInt(effectiveGasPrice)

	if txReceipt.To == nil && msgReceipt.ExitCode.IsSuccess() {
		// Create and Create2 return the same things.
		var ret eam.CreateExternalReturn
		if err := ret.UnmarshalCBOR(bytes.NewReader(msgReceipt.Return)); err != nil {
			return ethtypes.EthTxReceipt{}, xerrors.Errorf("failed to parse contract creation result: %w", err)
		}
		addr := ethtypes.EthAddress(ret.EthAddress)
		txReceipt.ContractAddress = &addr
	}

	if rct := msgReceipt; rct.EventsRoot != nil {
		logs, err := ev.GetEthLogsForBlockAndTransaction(ctx, &blockHash, tx.Hash)
		if err != nil {
			return ethtypes.EthTxReceipt{}, xerrors.Errorf("failed to get eth logs for block and transaction: %w", err)
		}
		if len(logs) > 0 {
			txReceipt.Logs = logs
		}
	}

	for _, log := range txReceipt.Logs {
		for _, topic := range log.Topics {
			ethtypes.EthBloomSet(txReceipt.LogsBloom, topic[:])
		}
		ethtypes.EthBloomSet(txReceipt.LogsBloom, log.Address[:])
	}

	return txReceipt, nil
}

// decodeApplyAndCallReturnStatus parses a CBOR-encoded [status(uint), output_data(bytes)]
// and returns the status if decoding succeeds.
func decodeApplyAndCallReturnStatus(b []byte) (uint64, bool) {
	r := bytes.NewReader(b)
	maj, extra, err := cbg.CborReadHeader(r)
	if err != nil || maj != cbg.MajArray || extra != 2 {
		return 0, false
	}
	maj, status, err := cbg.CborReadHeader(r)
	if err != nil || maj != cbg.MajUnsignedInt {
		return 0, false
	}
	maj, extra, err = cbg.CborReadHeader(r)
	if err != nil || maj != cbg.MajByteString {
		return 0, false
	}
	if extra > 0 {
		buf := make([]byte, extra)
		if _, err := r.Read(buf); err != nil {
			return 0, false
		}
	}
	return status, true
}

func encodeFilecoinParamsAsABI(method abi.MethodNum, codec uint64, params []byte) []byte {
	buf := []byte{0x86, 0x8e, 0x10, 0xc4} // Native method selector.
	return append(buf, encodeAsABIHelper(uint64(method), codec, params)...)
}

func encodeFilecoinReturnAsABI(exitCode exitcode.ExitCode, codec uint64, data []byte) []byte {
	return encodeAsABIHelper(uint64(exitCode), codec, data)
}

// Format 2 numbers followed by an arbitrary byte array as solidity ABI. Both our native
// inputs/outputs follow the same pattern, so we can reuse this code.
func encodeAsABIHelper(param1 uint64, param2 uint64, data []byte) []byte {
	const EVM_WORD_SIZE = 32

	// The first two params are "static" numbers. Then, we record the offset of the "data" arg,
	// then, at that offset, we record the length of the data.
	//
	// In practice, this means we have 4 256-bit words back to back where the third arg (the
	// offset) is _always_ '32*3'.
	staticArgs := []uint64{param1, param2, EVM_WORD_SIZE * 3, uint64(len(data))}
	// We always pad out to the next EVM "word" (32 bytes).
	totalWords := len(staticArgs) + (len(data) / EVM_WORD_SIZE)
	if len(data)%EVM_WORD_SIZE != 0 {
		totalWords++
	}
	sz := totalWords * EVM_WORD_SIZE
	buf := make([]byte, sz)
	offset := 0
	// Below, we use copy instead of "appending" to preserve all the zero padding.
	for _, arg := range staticArgs {
		// Write each "arg" into the last 8 bytes of each 32 byte word.
		offset += EVM_WORD_SIZE
		start := offset - 8
		binary.BigEndian.PutUint64(buf[start:offset], arg)
	}

	// Finally, we copy in the data.
	copy(buf[offset:], data)

	return buf
}

// decodePayload is a utility function which decodes the payload using the given codec
func decodePayload(payload []byte, codec uint64) (ethtypes.EthBytes, error) {
	switch multicodec.Code(codec) {
	case multicodec.Identity:
		return nil, nil
	case multicodec.DagCbor, multicodec.Cbor:
		buf, err := cbg.ReadByteArray(bytes.NewReader(payload), uint64(len(payload)))
		if err != nil {
			return nil, xerrors.Errorf("decodePayload: failed to decode cbor payload: %w", err)
		}
		return buf, nil
	case multicodec.Raw:
		return ethtypes.EthBytes(payload), nil
	}

	return nil, xerrors.Errorf("decodePayload: unsupported codec: %d", codec)
}

func decodeParams[P any, T interface {
	*P
	cbg.CBORUnmarshaler
}](msg *types.MessageTrace) (T, error) {
	var params T = new(P)
	switch msg.ParamsCodec {
	case uint64(multicodec.DagCbor), uint64(multicodec.Cbor):
	default:
		return nil, xerrors.Errorf("Method called with unexpected codec %d", msg.ParamsCodec)
	}

	if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
		return nil, xerrors.Errorf("failed to decode params: %w", err)
	}

	return params, nil
}

func decodeReturn[R any, T interface {
	*R
	cbg.CBORUnmarshaler
}](ret *types.ReturnTrace) (T, error) {
	var retval T = new(R)
	switch ret.ReturnCodec {
	case uint64(multicodec.DagCbor), uint64(multicodec.Cbor):
	default:
		return nil, xerrors.Errorf("Method returned an unexpected codec %d", ret.ReturnCodec)
	}

	if err := retval.UnmarshalCBOR(bytes.NewReader(ret.Return)); err != nil {
		return nil, xerrors.Errorf("failed to decode return value: %w", err)
	}

	return retval, nil
}

func find[T any](values []T, cb func(t *T) *T) *T {
	for i := range values {
		if o := cb(&values[i]); o != nil {
			return o
		}
	}
	return nil
}

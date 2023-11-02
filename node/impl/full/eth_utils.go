package full

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/vm"
)

func getTipsetByBlockNumber(ctx context.Context, chain *store.ChainStore, blkParam string, strict bool) (*types.TipSet, error) {
	if blkParam == "earliest" {
		return nil, fmt.Errorf("block param \"earliest\" is not supported")
	}

	head := chain.GetHeaviestTipSet()
	switch blkParam {
	case "pending":
		return head, nil
	case "latest":
		parent, err := chain.GetTipSetFromKey(ctx, head.Parents())
		if err != nil {
			return nil, fmt.Errorf("cannot get parent tipset")
		}
		return parent, nil
	default:
		var num ethtypes.EthUint64
		err := num.UnmarshalJSON([]byte(`"` + blkParam + `"`))
		if err != nil {
			return nil, fmt.Errorf("cannot parse block number: %v", err)
		}
		if abi.ChainEpoch(num) > head.Height()-1 {
			return nil, fmt.Errorf("requested a future epoch (beyond 'latest')")
		}
		ts, err := chain.GetTipsetByHeight(ctx, abi.ChainEpoch(num), head, true)
		if err != nil {
			return nil, fmt.Errorf("cannot get tipset at height: %v", num)
		}
		if strict && ts.Height() != abi.ChainEpoch(num) {
			return nil, ErrNullRound
		}
		return ts, nil
	}
}

func getTipsetByEthBlockNumberOrHash(ctx context.Context, chain *store.ChainStore, blkParam ethtypes.EthBlockNumberOrHash) (*types.TipSet, error) {
	head := chain.GetHeaviestTipSet()

	predefined := blkParam.PredefinedBlock
	if predefined != nil {
		if *predefined == "earliest" {
			return nil, fmt.Errorf("block param \"earliest\" is not supported")
		} else if *predefined == "pending" {
			return head, nil
		} else if *predefined == "latest" {
			parent, err := chain.GetTipSetFromKey(ctx, head.Parents())
			if err != nil {
				return nil, fmt.Errorf("cannot get parent tipset")
			}
			return parent, nil
		} else {
			return nil, fmt.Errorf("unknown predefined block %s", *predefined)
		}
	}

	if blkParam.BlockNumber != nil {
		height := abi.ChainEpoch(*blkParam.BlockNumber)
		if height > head.Height()-1 {
			return nil, fmt.Errorf("requested a future epoch (beyond 'latest')")
		}
		ts, err := chain.GetTipsetByHeight(ctx, height, head, true)
		if err != nil {
			return nil, fmt.Errorf("cannot get tipset at height: %v", height)
		}
		return ts, nil
	}

	if blkParam.BlockHash != nil {
		ts, err := chain.GetTipSetByCid(ctx, blkParam.BlockHash.ToCid())
		if err != nil {
			return nil, fmt.Errorf("cannot get tipset by hash: %v", err)
		}

		// verify that the tipset is in the canonical chain
		if blkParam.RequireCanonical {
			// walk up the current chain (our head) until we reach ts.Height()
			walkTs, err := chain.GetTipsetByHeight(ctx, ts.Height(), head, true)
			if err != nil {
				return nil, fmt.Errorf("cannot get tipset at height: %v", ts.Height())
			}

			// verify that it equals the expected tipset
			if !walkTs.Equals(ts) {
				return nil, fmt.Errorf("tipset is not canonical")
			}
		}

		return ts, nil
	}

	return nil, errors.New("invalid block param")
}

func ethCallToFilecoinMessage(ctx context.Context, tx ethtypes.EthCall) (*types.Message, error) {
	var from address.Address
	if tx.From == nil || *tx.From == (ethtypes.EthAddress{}) {
		// Send from the filecoin "system" address.
		var err error
		from, err = (ethtypes.EthAddress{}).ToFilecoinAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to construct the ethereum system address: %w", err)
		}
	} else {
		// The from address must be translatable to an f4 address.
		var err error
		from, err = tx.From.ToFilecoinAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to translate sender address (%s): %w", tx.From.String(), err)
		}
		if p := from.Protocol(); p != address.Delegated {
			return nil, fmt.Errorf("expected a class 4 address, got: %d: %w", p, err)
		}
	}

	var params []byte
	if len(tx.Data) > 0 {
		initcode := abi.CborBytes(tx.Data)
		params2, err := actors.SerializeParams(&initcode)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize params: %w", err)
		}
		params = params2
	}

	var to address.Address
	var method abi.MethodNum
	if tx.To == nil {
		// this is a contract creation
		to = builtintypes.EthereumAddressManagerActorAddr
		method = builtintypes.MethodsEAM.CreateExternal
	} else {
		addr, err := tx.To.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
		}
		to = addr
		method = builtintypes.MethodsEVM.InvokeContract
	}

	return &types.Message{
		From:       from,
		To:         to,
		Value:      big.Int(tx.Value),
		Method:     method,
		Params:     params,
		GasLimit:   build.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}, nil
}

func newEthBlockFromFilecoinTipSet(ctx context.Context, ts *types.TipSet, fullTxInfo bool, cs *store.ChainStore, sa StateAPI) (ethtypes.EthBlock, error) {
	parentKeyCid, err := ts.Parents().Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	parentBlkHash, err := ethtypes.EthHashFromCid(parentKeyCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	bn := ethtypes.EthUint64(ts.Height())

	blkCid, err := ts.Key().Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	blkHash, err := ethtypes.EthHashFromCid(blkCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	msgs, rcpts, err := messagesAndReceipts(ctx, ts, cs, sa)
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("failed to retrieve messages and receipts: %w", err)
	}

	block := ethtypes.NewEthBlock(len(msgs) > 0)

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
		tx, err := newEthTxFromSignedMessage(ctx, smsg, sa)
		if err != nil {
			return ethtypes.EthBlock{}, xerrors.Errorf("failed to convert msg to ethTx: %w", err)
		}

		tx.ChainID = ethtypes.EthUint64(build.Eip155ChainId)
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

func messagesAndReceipts(ctx context.Context, ts *types.TipSet, cs *store.ChainStore, sa StateAPI) ([]types.ChainMsg, []types.MessageReceipt, error) {
	msgs, err := cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return nil, nil, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	_, rcptRoot, err := sa.StateManager.TipSetState(ctx, ts)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to compute state: %w", err)
	}

	rcpts, err := cs.ReadReceipts(ctx, rcptRoot)
	if err != nil {
		return nil, nil, xerrors.Errorf("error loading receipts for tipset: %v: %w", ts, err)
	}

	if len(msgs) != len(rcpts) {
		return nil, nil, xerrors.Errorf("receipts and message array lengths didn't match for tipset: %v: %w", ts, err)
	}

	return msgs, rcpts, nil
}

const errorFunctionSelector = "\x08\xc3\x79\xa0" // Error(string)
const panicFunctionSelector = "\x4e\x48\x7b\x71" // Panic(uint256)
// Eth ABI (solidity) panic codes.
var panicErrorCodes map[uint64]string = map[uint64]string{
	0x00: "Panic()",
	0x01: "Assert()",
	0x11: "ArithmeticOverflow()",
	0x12: "DivideByZero()",
	0x21: "InvalidEnumVariant()",
	0x22: "InvalidStorageArray()",
	0x31: "PopEmptyArray()",
	0x32: "ArrayIndexOutOfBounds()",
	0x41: "OutOfMemory()",
	0x51: "CalledUninitializedFunction()",
}

// Parse an ABI encoded revert reason. This reason should be encoded as if it were the parameters to
// an `Error(string)` function call.
//
// See https://docs.soliditylang.org/en/latest/control-structures.html#panic-via-assert-and-error-via-require
func parseEthRevert(ret []byte) string {
	if len(ret) == 0 {
		return "none"
	}
	var cbytes abi.CborBytes
	if err := cbytes.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
		return "ERROR: revert reason is not cbor encoded bytes"
	}
	if len(cbytes) == 0 {
		return "none"
	}
	// If it's not long enough to contain an ABI encoded response, return immediately.
	if len(cbytes) < 4+32 {
		return ethtypes.EthBytes(cbytes).String()
	}
	switch string(cbytes[:4]) {
	case panicFunctionSelector:
		cbytes := cbytes[4 : 4+32]
		// Read the and check the code.
		code, err := ethtypes.EthUint64FromBytes(cbytes)
		if err != nil {
			// If it's too big, just return the raw value.
			codeInt := big.PositiveFromUnsignedBytes(cbytes)
			return fmt.Sprintf("Panic(%s)", ethtypes.EthBigInt(codeInt).String())
		}
		if s, ok := panicErrorCodes[uint64(code)]; ok {
			return s
		}
		return fmt.Sprintf("Panic(0x%x)", code)
	case errorFunctionSelector:
		cbytes := cbytes[4:]
		cbytesLen := ethtypes.EthUint64(len(cbytes))
		// Read the and check the offset.
		offset, err := ethtypes.EthUint64FromBytes(cbytes[:32])
		if err != nil {
			break
		}
		if cbytesLen < offset {
			break
		}

		// Read and check the length.
		if cbytesLen-offset < 32 {
			break
		}
		start := offset + 32
		length, err := ethtypes.EthUint64FromBytes(cbytes[offset : offset+32])
		if err != nil {
			break
		}
		if cbytesLen-start < length {
			break
		}
		// Slice the error message.
		return fmt.Sprintf("Error(%s)", cbytes[start:start+length])
	}
	return ethtypes.EthBytes(cbytes).String()
}

// lookupEthAddress makes its best effort at finding the Ethereum address for a
// Filecoin address. It does the following:
//
//  1. If the supplied address is an f410 address, we return its payload as the EthAddress.
//  2. Otherwise (f0, f1, f2, f3), we look up the actor on the state tree. If it has a delegated address, we return it if it's f410 address.
//  3. Otherwise, we fall back to returning a masked ID Ethereum address. If the supplied address is an f0 address, we
//     use that ID to form the masked ID address.
//  4. Otherwise, we fetch the actor's ID from the state tree and form the masked ID with it.
func lookupEthAddress(ctx context.Context, addr address.Address, sa StateAPI) (ethtypes.EthAddress, error) {
	// BLOCK A: We are trying to get an actual Ethereum address from an f410 address.
	// Attempt to convert directly, if it's an f4 address.
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(addr)
	if err == nil && !ethAddr.IsMaskedID() {
		return ethAddr, nil
	}

	// Lookup on the target actor and try to get an f410 address.
	if actor, err := sa.StateGetActor(ctx, addr, types.EmptyTSK); err != nil {
		return ethtypes.EthAddress{}, err
	} else if actor.Address != nil {
		if ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(*actor.Address); err == nil && !ethAddr.IsMaskedID() {
			return ethAddr, nil
		}
	}

	// BLOCK B: We gave up on getting an actual Ethereum address and are falling back to a Masked ID address.
	// Check if we already have an ID addr, and use it if possible.
	if err == nil && ethAddr.IsMaskedID() {
		return ethAddr, nil
	}

	// Otherwise, resolve the ID addr.
	idAddr, err := sa.StateLookupID(ctx, addr, types.EmptyTSK)
	if err != nil {
		return ethtypes.EthAddress{}, err
	}
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

func ethTxHashFromMessageCid(ctx context.Context, c cid.Cid, sa StateAPI) (ethtypes.EthHash, error) {
	smsg, err := sa.Chain.GetSignedMessage(ctx, c)
	if err == nil {
		// This is an Eth Tx, Secp message, Or BLS message in the mpool
		return ethTxHashFromSignedMessage(ctx, smsg, sa)
	}

	_, err = sa.Chain.GetMessage(ctx, c)
	if err == nil {
		// This is a BLS message
		return ethtypes.EthHashFromCid(c)
	}

	return ethtypes.EmptyEthHash, nil
}

func ethTxHashFromSignedMessage(ctx context.Context, smsg *types.SignedMessage, sa StateAPI) (ethtypes.EthHash, error) {
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		ethTx, err := newEthTxFromSignedMessage(ctx, smsg, sa)
		if err != nil {
			return ethtypes.EmptyEthHash, err
		}
		return ethTx.Hash, nil
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 {
		return ethtypes.EthHashFromCid(smsg.Cid())
	} else { // BLS message
		return ethtypes.EthHashFromCid(smsg.Message.Cid())
	}
}

func newEthTxFromSignedMessage(ctx context.Context, smsg *types.SignedMessage, sa StateAPI) (ethtypes.EthTx, error) {
	var tx ethtypes.EthTx
	var err error

	// This is an eth tx
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		tx, err = ethtypes.EthTxFromSignedEthMessage(smsg)
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to convert from signed message: %w", err)
		}

		tx.Hash, err = tx.TxHash()
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to calculate hash for ethTx: %w", err)
		}

		fromAddr, err := lookupEthAddress(ctx, smsg.Message.From, sa)
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to resolve Ethereum address: %w", err)
		}

		tx.From = fromAddr
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 { // Secp Filecoin Message
		tx = ethTxFromNativeMessage(ctx, smsg.VMMessage(), sa)
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Cid())
		if err != nil {
			return tx, err
		}
	} else { // BLS Filecoin message
		tx = ethTxFromNativeMessage(ctx, smsg.VMMessage(), sa)
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Message.Cid())
		if err != nil {
			return tx, err
		}
	}

	return tx, nil
}

// ethTxFromNativeMessage does NOT populate:
// - BlockHash
// - BlockNumber
// - TransactionIndex
// - Hash
func ethTxFromNativeMessage(ctx context.Context, msg *types.Message, sa StateAPI) ethtypes.EthTx {
	// We don't care if we error here, conversion is best effort for non-eth transactions
	from, _ := lookupEthAddress(ctx, msg.From, sa)
	to, _ := lookupEthAddress(ctx, msg.To, sa)
	return ethtypes.EthTx{
		To:                   &to,
		From:                 from,
		Nonce:                ethtypes.EthUint64(msg.Nonce),
		ChainID:              ethtypes.EthUint64(build.Eip155ChainId),
		Value:                ethtypes.EthBigInt(msg.Value),
		Type:                 ethtypes.Eip1559TxType,
		Gas:                  ethtypes.EthUint64(msg.GasLimit),
		MaxFeePerGas:         ethtypes.EthBigInt(msg.GasFeeCap),
		MaxPriorityFeePerGas: ethtypes.EthBigInt(msg.GasPremium),
		AccessList:           []ethtypes.EthHash{},
	}
}

func getSignedMessage(ctx context.Context, cs *store.ChainStore, msgCid cid.Cid) (*types.SignedMessage, error) {
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
func newEthTxFromMessageLookup(ctx context.Context, msgLookup *api.MsgLookup, txIdx int, cs *store.ChainStore, sa StateAPI) (ethtypes.EthTx, error) {
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
			return ethtypes.EthTx{}, fmt.Errorf("cannot find the msg in the tipset")
		}
	}

	blkHash, err := ethtypes.EthHashFromCid(parentTsCid)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	smsg, err := getSignedMessage(ctx, cs, msgLookup.Message)
	if err != nil {
		return ethtypes.EthTx{}, xerrors.Errorf("failed to get signed msg: %w", err)
	}

	tx, err := newEthTxFromSignedMessage(ctx, smsg, sa)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	var (
		bn = ethtypes.EthUint64(parentTs.Height())
		ti = ethtypes.EthUint64(txIdx)
	)

	tx.ChainID = ethtypes.EthUint64(build.Eip155ChainId)
	tx.BlockHash = &blkHash
	tx.BlockNumber = &bn
	tx.TransactionIndex = &ti
	return tx, nil
}

func newEthTxReceipt(ctx context.Context, tx ethtypes.EthTx, lookup *api.MsgLookup, events []types.Event, cs *store.ChainStore, sa StateAPI) (api.EthTxReceipt, error) {
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

	receipt := api.EthTxReceipt{
		TransactionHash:  tx.Hash,
		From:             tx.From,
		To:               tx.To,
		TransactionIndex: transactionIndex,
		BlockHash:        blockHash,
		BlockNumber:      blockNumber,
		Type:             ethtypes.EthUint64(2),
		Logs:             []ethtypes.EthLog{}, // empty log array is compulsory when no logs, or libraries like ethers.js break
		LogsBloom:        ethtypes.EmptyEthBloom[:],
	}

	if lookup.Receipt.ExitCode.IsSuccess() {
		receipt.Status = 1
	} else {
		receipt.Status = 0
	}

	receipt.GasUsed = ethtypes.EthUint64(lookup.Receipt.GasUsed)

	// TODO: handle CumulativeGasUsed
	receipt.CumulativeGasUsed = ethtypes.EmptyEthInt

	// TODO: avoid loading the tipset twice (once here, once when we convert the message to a txn)
	ts, err := cs.GetTipSetFromKey(ctx, lookup.TipSet)
	if err != nil {
		return api.EthTxReceipt{}, xerrors.Errorf("failed to lookup tipset %s when constructing the eth txn receipt: %w", lookup.TipSet, err)
	}

	// The tx is located in the parent tipset
	parentTs, err := cs.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return api.EthTxReceipt{}, xerrors.Errorf("failed to lookup tipset %s when constructing the eth txn receipt: %w", ts.Parents(), err)
	}

	baseFee := parentTs.Blocks()[0].ParentBaseFee
	gasOutputs := vm.ComputeGasOutputs(lookup.Receipt.GasUsed, int64(tx.Gas), baseFee, big.Int(tx.MaxFeePerGas), big.Int(tx.MaxPriorityFeePerGas), true)
	totalSpent := big.Sum(gasOutputs.BaseFeeBurn, gasOutputs.MinerTip, gasOutputs.OverEstimationBurn)

	effectiveGasPrice := big.Zero()
	if lookup.Receipt.GasUsed > 0 {
		effectiveGasPrice = big.Div(totalSpent, big.NewInt(lookup.Receipt.GasUsed))
	}
	receipt.EffectiveGasPrice = ethtypes.EthBigInt(effectiveGasPrice)

	if receipt.To == nil && lookup.Receipt.ExitCode.IsSuccess() {
		// Create and Create2 return the same things.
		var ret eam.CreateExternalReturn
		if err := ret.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)); err != nil {
			return api.EthTxReceipt{}, xerrors.Errorf("failed to parse contract creation result: %w", err)
		}
		addr := ethtypes.EthAddress(ret.EthAddress)
		receipt.ContractAddress = &addr
	}

	if len(events) > 0 {
		receipt.Logs = make([]ethtypes.EthLog, 0, len(events))
		for i, evt := range events {
			l := ethtypes.EthLog{
				Removed:          false,
				LogIndex:         ethtypes.EthUint64(i),
				TransactionHash:  tx.Hash,
				TransactionIndex: transactionIndex,
				BlockHash:        blockHash,
				BlockNumber:      blockNumber,
			}

			data, topics, ok := ethLogFromEvent(evt.Entries)
			if !ok {
				// not an eth event.
				continue
			}
			for _, topic := range topics {
				ethtypes.EthBloomSet(receipt.LogsBloom, topic[:])
			}
			l.Data = data
			l.Topics = topics

			addr, err := address.NewIDAddress(uint64(evt.Emitter))
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to create ID address: %w", err)
			}

			l.Address, err = lookupEthAddress(ctx, addr, sa)
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to resolve Ethereum address: %w", err)
			}

			ethtypes.EthBloomSet(receipt.LogsBloom, l.Address[:])
			receipt.Logs = append(receipt.Logs, l)
		}
	}

	return receipt, nil
}

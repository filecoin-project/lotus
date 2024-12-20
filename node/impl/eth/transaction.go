package eth

import (
	"context"
	"errors"

	"github.com/hashicorp/golang-lru/arc/v2"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	builtinevm "github.com/filecoin-project/lotus/chain/actors/builtin/evm"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

type EthTransactionAPI interface {
	EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error)

	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error)

	EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error)
	EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error)

	EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error)
	EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error)
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error)

	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error)
	EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error)
	EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*api.EthTxReceipt, error)
	EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*api.EthTxReceipt, error)
}

var (
	_ EthTransactionAPI = (*ethTransaction)(nil)
	_ EthTransactionAPI = (*EthTransactionDisabled)(nil)
)

type ethTransaction struct {
	chainStore   ChainStore
	stateManager StateManager
	stateApi     StateAPI
	mpoolApi     MpoolAPI
	chainIndexer index.Indexer

	ethEvents EthEventsInternal

	blockCache            *arc.ARCCache[cid.Cid, *ethtypes.EthBlock] // caches blocks by their CID but blocks only have the transaction hashes
	blockTransactionCache *arc.ARCCache[cid.Cid, *ethtypes.EthBlock] // caches blocks along with full transaction payload by their CID
}

func NewEthTransactionAPI(
	chainStore ChainStore,
	stateManager StateManager,
	stateApi StateAPI,
	mpoolApi MpoolAPI,
	chainIndexer index.Indexer,
	ethEvents EthEventsInternal,
	blockCacheSize int,
) (EthTransactionAPI, error) {
	t := &ethTransaction{
		chainStore:            chainStore,
		stateManager:          stateManager,
		stateApi:              stateApi,
		mpoolApi:              mpoolApi,
		chainIndexer:          chainIndexer,
		ethEvents:             ethEvents,
		blockCache:            nil,
		blockTransactionCache: nil,
	}

	if blockCacheSize > 0 {
		var err error
		if t.blockCache, err = arc.NewARC[cid.Cid, *ethtypes.EthBlock](blockCacheSize); err != nil {
			return nil, xerrors.Errorf("failed to create block cache: %w", err)
		}
		if t.blockTransactionCache, err = arc.NewARC[cid.Cid, *ethtypes.EthBlock](blockCacheSize); err != nil {
			return nil, xerrors.Errorf("failed to create block transaction cache: %w", err)
		}
	}

	return t, nil
}

func (e *ethTransaction) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	// eth_blockNumber needs to return the height of the latest committed tipset.
	// Ethereum clients expect all transactions included in this block to have execution outputs.
	// This is the parent of the head tipset. The head tipset is speculative, has not been
	// recognized by the network, and its messages are only included, not executed.
	// See https://github.com/filecoin-project/ref-fvm/issues/1135.
	heaviest := e.chainStore.GetHeaviestTipSet()
	if height := heaviest.Height(); height == 0 {
		// we're at genesis.
		return ethtypes.EthUint64(height), nil
	}
	// First non-null parent.
	effectiveParent := heaviest.Parents()
	parent, err := e.chainStore.GetTipSetFromKey(ctx, effectiveParent)
	if err != nil {
		return 0, err
	}
	return ethtypes.EthUint64(parent.Height()), nil
}

func (e *ethTransaction) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error) {
	ts, err := e.chainStore.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	count, err := e.countTipsetMsgs(ctx, ts)
	return ethtypes.EthUint64(count), err
}

func (e *ethTransaction) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	ts, err := e.chainStore.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	count, err := e.countTipsetMsgs(ctx, ts)
	return ethtypes.EthUint64(count), err
}

func (e *ethTransaction) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	cache := e.blockCache
	if fullTxInfo {
		cache = e.blockTransactionCache
	}

	// Attempt to retrieve the Ethereum block from cache
	cid := blkHash.ToCid()
	if cache != nil {
		if ethBlock, found := cache.Get(cid); found {
			if ethBlock != nil {
				return *ethBlock, nil
			}
			// Log and remove the nil entry from cache
			log.Errorw("nil value in eth block cache", "hash", blkHash.String())
			cache.Remove(cid)
		}
	}

	// Fetch the tipset using the block hash
	ts, err := e.chainStore.GetTipSetByCid(ctx, cid)
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("failed to load tipset by CID %s: %w", cid, err)
	}

	// Generate an Ethereum block from the Filecoin tipset
	blk, err := newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo, e.chainStore, e.stateManager)
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("failed to create Ethereum block from Filecoin tipset: %w", err)
	}

	// Add the newly created block to the cache and return
	if cache != nil {
		cache.Add(cid, &blk)
	}
	return blk, nil
}

func (e *ethTransaction) EthGetBlockByNumber(ctx context.Context, blkParam string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	ts, err := getTipsetByBlockNumber(ctx, e.chainStore, blkParam, true)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	return newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo, e.chainStore, e.stateManager)
}

func (e *ethTransaction) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	return e.EthGetTransactionByHashLimited(ctx, txHash, api.LookbackNoLimit)
}

func (e *ethTransaction) EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error) {
	// Ethereum's behavior is to return null when the txHash is invalid, so we use nil to check if txHash is valid
	if txHash == nil {
		return nil, nil
	}
	if e.chainIndexer == nil {
		return nil, ErrChainIndexerDisabled
	}

	var c cid.Cid
	var err error
	c, err = e.chainIndexer.GetCidFromHash(ctx, *txHash)
	if err != nil && errors.Is(err, index.ErrNotFound) {
		log.Debug("could not find transaction hash %s in chain indexer", txHash.String())
	} else if err != nil {
		log.Errorf("failed to lookup transaction hash %s in chain indexer: %s", txHash.String(), err)
		return nil, xerrors.Errorf("failed to lookup transaction hash %s in chain indexer: %w", txHash.String(), err)
	}

	// This isn't an eth transaction we have the mapping for, so let's look it up as a filecoin message
	if c == cid.Undef {
		c = txHash.ToCid()
	}

	// first, try to get the cid from mined transactions
	msgLookup, err := e.stateApi.StateSearchMsg(ctx, types.EmptyTSK, c, limit, true)
	if err == nil && msgLookup != nil {
		tx, err := newEthTxFromMessageLookup(ctx, msgLookup, -1, e.chainStore, e.stateManager)
		if err == nil {
			return &tx, nil
		}
	}

	// if not found, try to get it from the mempool
	pending, err := e.mpoolApi.MpoolPending(ctx, types.EmptyTSK)
	if err != nil {
		// inability to fetch mpool pending transactions is an internal node error
		// that needs to be reported as-is
		return nil, xerrors.Errorf("cannot get pending txs from mpool: %s", err)
	}

	for _, p := range pending {
		if p.Cid() == c {
			// We only return pending eth-account messages because we can't guarantee
			// that the from/to addresses of other messages are conversable to 0x-style
			// addresses. So we just ignore them.
			//
			// This should be "fine" as anyone using an "Ethereum-centric" block
			// explorer shouldn't care about seeing pending messages from native
			// accounts.
			ethtx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(p)
			if err != nil {
				return nil, xerrors.Errorf("could not convert Filecoin message into tx: %w", err)
			}

			tx, err := ethtx.ToEthTx(p)
			if err != nil {
				return nil, xerrors.Errorf("could not convert Eth transaction to EthTx: %w", err)
			}

			return &tx, nil
		}
	}
	// Ethereum clients expect an empty response when the message was not found
	return nil, nil
}

func (e *ethTransaction) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, index ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	ts, err := e.chainStore.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset by cid: %w", err)
	}

	return e.getTransactionByTipsetAndIndex(ctx, ts, index)
}

func (e *ethTransaction) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkParam string, index ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	ts, err := getTipsetByBlockNumber(ctx, e.chainStore, blkParam, true)
	if err != nil {
		return nil, err
	}

	if ts == nil {
		return nil, xerrors.Errorf("tipset not found for block %s", blkParam)
	}

	tx, err := e.getTransactionByTipsetAndIndex(ctx, ts, index)
	if err != nil {
		return nil, xerrors.Errorf("failed to get transaction at index %d: %w", index, err)
	}

	return tx, nil
}

func (e *ethTransaction) EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) {
	// Ethereum's behavior is to return null when the txHash is invalid, so we use nil to check if txHash is valid
	if txHash == nil {
		return nil, nil
	}
	if e.chainIndexer == nil {
		return nil, ErrChainIndexerDisabled
	}

	var c cid.Cid
	var err error
	c, err = e.chainIndexer.GetCidFromHash(ctx, *txHash)
	if err != nil && errors.Is(err, index.ErrNotFound) {
		log.Debug("could not find transaction hash %s in chain indexer", txHash.String())
	} else if err != nil {
		log.Errorf("failed to lookup transaction hash %s in chain indexer: %s", txHash.String(), err)
		return nil, xerrors.Errorf("failed to lookup transaction hash %s in chain indexer: %w", txHash.String(), err)
	}

	if errors.Is(err, index.ErrNotFound) {
		log.Debug("could not find transaction hash %s in lookup table", txHash.String())
	} else if e.chainIndexer != nil {
		return &c, nil
	}

	// This isn't an eth transaction we have the mapping for, so let's try looking it up as a filecoin message
	if c == cid.Undef {
		c = txHash.ToCid()
	}

	_, err = e.chainStore.GetSignedMessage(ctx, c)
	if err == nil {
		// This is an Eth Tx, Secp message, Or BLS message in the mpool
		return &c, nil
	}

	_, err = e.chainStore.GetMessage(ctx, c)
	if err == nil {
		// This is a BLS message
		return &c, nil
	}

	// Ethereum clients expect an empty response when the message was not found
	return nil, nil
}

func (e *ethTransaction) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	if txHash, err := getTransactionHashByCid(ctx, e.chainStore, cid); err != nil {
		return nil, err
	} else if txHash == ethtypes.EmptyEthHash {
		// not found
		return nil, nil
	} else {
		return &txHash, nil
	}
}

func (e *ethTransaction) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error) {
	addr, err := sender.ToFilecoinAddress()
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("invalid address: %w", err)
	}

	// Handle "pending" block parameter separately
	if blkParam.PredefinedBlock != nil && *blkParam.PredefinedBlock == "pending" {
		nonce, err := e.mpoolApi.MpoolGetNonce(ctx, addr)
		if err != nil {
			return ethtypes.EthUint64(0), xerrors.Errorf("failed to get nonce from mpool: %w", err)
		}
		return ethtypes.EthUint64(nonce), nil
	}

	// For all other cases, get the tipset based on the block parameter
	ts, err := getTipsetByEthBlockNumberOrHash(ctx, e.chainStore, blkParam)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("failed to process block param: %v; %w", blkParam, err)
	}

	// Get the actor state at the specified tipset
	actor, err := e.stateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		if errors.Is(err, types.ErrActorNotFound) {
			return 0, nil
		}
		return 0, xerrors.Errorf("failed to lookup actor %s: %w", sender, err)
	}

	// Handle EVM actor case
	if builtinactors.IsEvmActor(actor.Code) {
		evmState, err := builtinevm.Load(e.chainStore.ActorStore(ctx), actor)
		if err != nil {
			return 0, xerrors.Errorf("failed to load evm state: %w", err)
		}
		if alive, err := evmState.IsAlive(); err != nil {
			return 0, err
		} else if !alive {
			return 0, nil
		}
		nonce, err := evmState.Nonce()
		return ethtypes.EthUint64(nonce), err
	}

	// For non-EVM actors, get the nonce from the actor state
	return ethtypes.EthUint64(actor.Nonce), nil
}

func (e *ethTransaction) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	return e.EthGetTransactionReceiptLimited(ctx, txHash, api.LookbackNoLimit)
}

func (e *ethTransaction) EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error) {
	var c cid.Cid
	var err error
	if e.chainIndexer == nil {
		return nil, ErrChainIndexerDisabled
	}

	c, err = e.chainIndexer.GetCidFromHash(ctx, txHash)
	if err != nil && errors.Is(err, index.ErrNotFound) {
		log.Debug("could not find transaction hash %s in chain indexer", txHash.String())
	} else if err != nil {
		log.Errorf("failed to lookup transaction hash %s in chain indexer: %s", txHash.String(), err)
		return nil, xerrors.Errorf("failed to lookup transaction hash %s in chain indexer: %w", txHash.String(), err)
	}

	// This isn't an eth transaction we have the mapping for, so let's look it up as a filecoin message
	if c == cid.Undef {
		c = txHash.ToCid()
	}

	msgLookup, err := e.stateApi.StateSearchMsg(ctx, types.EmptyTSK, c, limit, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to lookup Eth Txn %s as %s: %w", txHash, c, err)
	}
	if msgLookup == nil {
		// This is the best we can do. In theory, we could have just not indexed this
		// transaction, but there's no way to check that here.
		return nil, nil
	}

	tx, err := newEthTxFromMessageLookup(ctx, msgLookup, -1, e.chainStore, e.stateManager)
	if err != nil {
		return nil, xerrors.Errorf("failed to convert %s into an Eth Txn: %w", txHash, err)
	}

	ts, err := e.chainStore.GetTipSetFromKey(ctx, msgLookup.TipSet)
	if err != nil {
		return nil, xerrors.Errorf("failed to lookup tipset %s when constructing the eth txn receipt: %w", msgLookup.TipSet, err)
	}

	// The tx is located in the parent tipset
	parentTs, err := e.chainStore.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to lookup tipset %s when constructing the eth txn receipt: %w", ts.Parents(), err)
	}

	baseFee := parentTs.Blocks()[0].ParentBaseFee

	receipt, err := newEthTxReceipt(ctx, tx, baseFee, msgLookup.Receipt, e.ethEvents)
	if err != nil {
		return nil, xerrors.Errorf("failed to create Eth receipt: %w", err)
	}

	return &receipt, nil
}

func (e *ethTransaction) EthGetBlockReceipts(ctx context.Context, blockParam ethtypes.EthBlockNumberOrHash) ([]*api.EthTxReceipt, error) {
	return e.EthGetBlockReceiptsLimited(ctx, blockParam, api.LookbackNoLimit)
}

func (e *ethTransaction) EthGetBlockReceiptsLimited(ctx context.Context, blockParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*api.EthTxReceipt, error) {
	ts, err := getTipsetByEthBlockNumberOrHash(ctx, e.chainStore, blockParam)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset: %w", err)
	}

	tsCid, err := ts.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	blkHash, err := ethtypes.EthHashFromCid(tsCid)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse eth hash from cid: %w", err)
	}

	// Execute the tipset to get the receipts, messages, and events
	st, msgs, receipts, err := executeTipset(ctx, ts, e.chainStore, e.stateManager)
	if err != nil {
		return nil, xerrors.Errorf("failed to execute tipset: %w", err)
	}

	// Load the state tree
	stateTree, err := e.stateManager.StateTree(st)
	if err != nil {
		return nil, xerrors.Errorf("failed to load state tree: %w", err)
	}

	baseFee := ts.Blocks()[0].ParentBaseFee

	ethReceipts := make([]*api.EthTxReceipt, 0, len(msgs))
	for i, msg := range msgs {
		msg := msg

		tx, err := newEthTx(ctx, e.chainStore, stateTree, ts.Height(), tsCid, msg.Cid(), i)
		if err != nil {
			return nil, xerrors.Errorf("failed to create EthTx: %w", err)
		}

		receipt, err := newEthTxReceipt(ctx, tx, baseFee, receipts[i], e.ethEvents)
		if err != nil {
			return nil, xerrors.Errorf("failed to create Eth receipt: %w", err)
		}

		// Set the correct Ethereum block hash
		receipt.BlockHash = blkHash

		ethReceipts = append(ethReceipts, &receipt)
	}

	return ethReceipts, nil
}

func (e *ethTransaction) getTransactionByTipsetAndIndex(ctx context.Context, ts *types.TipSet, index ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	msgs, err := e.chainStore.MessagesForTipset(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get messages for tipset: %w", err)
	}

	if uint64(index) >= uint64(len(msgs)) {
		return nil, xerrors.Errorf("index %d out of range: tipset contains %d messages", index, len(msgs))
	}

	msg := msgs[index]

	cid, err := ts.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	// First, get the state tree
	st, err := e.stateManager.StateTree(ts.ParentState())
	if err != nil {
		return nil, xerrors.Errorf("failed to load state tree: %w", err)
	}

	tx, err := newEthTx(ctx, e.chainStore, st, ts.Height(), cid, msg.Cid(), int(index))
	if err != nil {
		return nil, xerrors.Errorf("failed to create Ethereum transaction: %w", err)
	}

	return &tx, nil
}

func (e *ethTransaction) countTipsetMsgs(ctx context.Context, ts *types.TipSet) (int, error) {
	blkMsgs, err := e.chainStore.BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return 0, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	count := 0
	for _, blkMsg := range blkMsgs {
		// TODO: may need to run canonical ordering and deduplication here
		count += len(blkMsg.BlsMessages) + len(blkMsg.SecpkMessages)
	}
	return count, nil
}

type EthTransactionDisabled struct{}

func (EthTransactionDisabled) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	return ethtypes.EthBlock{}, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	return ethtypes.EthBlock{}, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetBlockReceipts(ctx context.Context, blockParam ethtypes.EthBlockNumberOrHash) ([]*api.EthTxReceipt, error) {
	return nil, ErrModuleDisabled
}
func (EthTransactionDisabled) EthGetBlockReceiptsLimited(ctx context.Context, blockParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*api.EthTxReceipt, error) {
	return nil, ErrModuleDisabled
}

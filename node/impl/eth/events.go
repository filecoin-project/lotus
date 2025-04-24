package eth

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var (
	_ EthEventsAPI      = (*ethEvents)(nil)
	_ EthEventsInternal = (*ethEvents)(nil)
	_ EthEventsAPI      = (*EthEventsDisabled)(nil)
)

type filterEventCollector interface {
	TakeCollectedEvents(context.Context) []*index.CollectedEvent
}

type filterMessageCollector interface {
	TakeCollectedMessages(context.Context) []*types.SignedMessage
}

type filterTipSetCollector interface {
	TakeCollectedTipSets(context.Context) []types.TipSetKey
}

type ethEvents struct {
	subscriptionCtx      context.Context
	chainStore           ChainStore
	stateManager         StateManager
	chainIndexer         index.Indexer
	eventFilterManager   *filter.EventFilterManager
	tipSetFilterManager  *filter.TipSetFilterManager
	memPoolFilterManager *filter.MemPoolFilterManager
	filterStore          filter.FilterStore
	subscriptionManager  *EthSubscriptionManager
	maxFilterHeightRange abi.ChainEpoch
}

func NewEthEventsAPI(
	subscriptionCtx context.Context,
	chainStore ChainStore,
	stateManager StateManager,
	chainIndexer index.Indexer,
	eventFilterManager *filter.EventFilterManager,
	tipSetFilterManager *filter.TipSetFilterManager,
	memPoolFilterManager *filter.MemPoolFilterManager,
	filterStore filter.FilterStore,
	subscriptionManager *EthSubscriptionManager,
	maxFilterHeightRange abi.ChainEpoch,
) EthEventsInternal {
	return &ethEvents{
		subscriptionCtx:      subscriptionCtx,
		chainStore:           chainStore,
		stateManager:         stateManager,
		chainIndexer:         chainIndexer,
		eventFilterManager:   eventFilterManager,
		tipSetFilterManager:  tipSetFilterManager,
		memPoolFilterManager: memPoolFilterManager,
		filterStore:          filterStore,
		subscriptionManager:  subscriptionManager,
		maxFilterHeightRange: maxFilterHeightRange,
	}
}

func (e *ethEvents) EthGetLogs(ctx context.Context, filterSpec *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) {
	ces, err := e.ethGetEventsForFilter(ctx, filterSpec)
	if err != nil {
		return nil, xerrors.Errorf("failed to get events for filter: %w", err)
	}
	return ethFilterResultFromEvents(ctx, ces, e.chainStore, e.stateManager)
}

func (e *ethEvents) EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if e.filterStore == nil || e.tipSetFilterManager == nil {
		return ethtypes.EthFilterID{}, api.ErrNotSupported
	}

	f, err := e.tipSetFilterManager.Install(ctx)
	if err != nil {
		return ethtypes.EthFilterID{}, err
	}

	if err := e.filterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.tipSetFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return ethtypes.EthFilterID{}, xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return ethtypes.EthFilterID{}, err
	}

	return ethtypes.EthFilterID(f.ID()), nil
}

func (e *ethEvents) EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if e.filterStore == nil || e.memPoolFilterManager == nil {
		return ethtypes.EthFilterID{}, api.ErrNotSupported
	}

	f, err := e.memPoolFilterManager.Install(ctx)
	if err != nil {
		return ethtypes.EthFilterID{}, err
	}

	if err := e.filterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.memPoolFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return ethtypes.EthFilterID{}, xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return ethtypes.EthFilterID{}, err
	}

	return ethtypes.EthFilterID(f.ID()), nil
}

func (e *ethEvents) EthNewFilter(ctx context.Context, filterSpec *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) {
	if e.filterStore == nil || e.eventFilterManager == nil {
		return ethtypes.EthFilterID{}, api.ErrNotSupported
	}

	pf, err := e.parseEthFilterSpec(filterSpec)
	if err != nil {
		return ethtypes.EthFilterID{}, err
	}

	f, err := e.eventFilterManager.Install(ctx, pf.minHeight, pf.maxHeight, pf.tipsetCid, pf.addresses, pf.keys)
	if err != nil {
		return ethtypes.EthFilterID{}, xerrors.Errorf("failed to install event filter: %w", err)
	}

	if err := e.filterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.eventFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return ethtypes.EthFilterID{}, xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return ethtypes.EthFilterID{}, err
	}
	return ethtypes.EthFilterID(f.ID()), nil
}

func (e *ethEvents) EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) {
	if e.filterStore == nil {
		return false, api.ErrNotSupported
	}

	f, err := e.filterStore.Get(ctx, types.FilterID(id))
	if err != nil {
		if errors.Is(err, filter.ErrFilterNotFound) {
			return false, nil
		}
		return false, err
	}

	if err := e.uninstallFilter(ctx, f); err != nil {
		return false, err
	}

	return true, nil
}

func (e *ethEvents) EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if e.filterStore == nil {
		return nil, api.ErrNotSupported
	}

	f, err := e.filterStore.Get(ctx, types.FilterID(id))
	if err != nil {
		return nil, err
	}

	switch fc := f.(type) {
	case filterEventCollector:
		return ethFilterResultFromEvents(ctx, fc.TakeCollectedEvents(ctx), e.chainStore, e.stateManager)
	case filterTipSetCollector:
		return ethFilterResultFromTipSets(fc.TakeCollectedTipSets(ctx))
	case filterMessageCollector:
		return ethFilterResultFromMessages(fc.TakeCollectedMessages(ctx))
	}

	return nil, xerrors.New("unknown filter type")
}

func (e *ethEvents) EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if e.filterStore == nil {
		return nil, api.ErrNotSupported
	}

	f, err := e.filterStore.Get(ctx, types.FilterID(id))
	if err != nil {
		return nil, err
	}

	switch fc := f.(type) {
	case filterEventCollector:
		return ethFilterResultFromEvents(ctx, fc.TakeCollectedEvents(ctx), e.chainStore, e.stateManager)
	}

	return nil, xerrors.New("wrong filter type")
}

func (e *ethEvents) EthSubscribe(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthSubscribeParams](p)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("decoding params: %w", err)
	}

	if e.subscriptionManager == nil {
		return ethtypes.EthSubscriptionID{}, api.ErrNotSupported
	}

	ethCb, ok := jsonrpc.ExtractReverseClient[api.EthSubscriberMethods](ctx)
	if !ok {
		return ethtypes.EthSubscriptionID{}, xerrors.New("connection doesn't support callbacks")
	}

	sub, err := e.subscriptionManager.StartSubscription(e.subscriptionCtx, ethCb.EthSubscription, e.uninstallFilter)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	switch params.EventType {
	case EthSubscribeEventTypeHeads:
		f, err := e.tipSetFilterManager.Install(ctx)
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return ethtypes.EthSubscriptionID{}, err
		}
		sub.addFilter(f)

	case EthSubscribeEventTypeLogs:
		keys := map[string][][]byte{}
		if params.Params != nil {
			var err error
			keys, err = parseEthTopics(params.Params.Topics)
			if err != nil {
				// clean up any previous filters added and stop the sub
				_, _ = e.EthUnsubscribe(ctx, sub.id)
				return ethtypes.EthSubscriptionID{}, err
			}
		}

		var addresses []address.Address
		if params.Params != nil {
			for _, ea := range params.Params.Address {
				a, err := ea.ToFilecoinAddress()
				if err != nil {
					return ethtypes.EthSubscriptionID{}, xerrors.Errorf("invalid address %x", ea)
				}
				addresses = append(addresses, a)
			}
		}

		f, err := e.eventFilterManager.Install(ctx, -1, -1, cid.Undef, addresses, keysToKeysWithCodec(keys))
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return ethtypes.EthSubscriptionID{}, err
		}
		sub.addFilter(f)
	case EthSubscribeEventTypePendingTransactions:
		f, err := e.memPoolFilterManager.Install(ctx)
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return ethtypes.EthSubscriptionID{}, err
		}

		sub.addFilter(f)
	default:
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("unsupported event type: %s", params.EventType)
	}

	return sub.id, nil
}

func (e *ethEvents) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	if e.subscriptionManager == nil {
		return false, api.ErrNotSupported
	}

	err := e.subscriptionManager.StopSubscription(ctx, id)
	if err != nil {
		return false, nil
	}

	return true, nil
}

func (e *ethEvents) GetEthLogsForBlockAndTransaction(ctx context.Context, blockHash *ethtypes.EthHash, txHash ethtypes.EthHash) ([]ethtypes.EthLog, error) {
	ces, err := e.ethGetEventsForFilter(ctx, &ethtypes.EthFilterSpec{BlockHash: blockHash})
	if err != nil {
		return nil, err
	}
	logs, err := ethFilterLogsFromEvents(ctx, ces, e.chainStore, e.stateManager)
	if err != nil {
		return nil, err
	}
	var out []ethtypes.EthLog
	for _, log := range logs {
		if log.TransactionHash == txHash {
			out = append(out, log)
		}
	}
	return out, nil
}

// GC runs a garbage collection loop, deleting filters that have not been used within the ttl window
func (e *ethEvents) GC(ctx context.Context, ttl time.Duration) {
	if e.filterStore == nil {
		return
	}

	tt := time.NewTicker(time.Minute * 30)
	defer tt.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tt.C:
			fs := e.filterStore.NotTakenSince(time.Now().Add(-ttl))
			for _, f := range fs {
				if err := e.uninstallFilter(ctx, f); err != nil {
					log.Warnf("Failed to remove actor event filter during garbage collection: %v", err)
				}
			}
		}
	}
}

func (e *ethEvents) ethGetEventsForFilter(ctx context.Context, filterSpec *ethtypes.EthFilterSpec) ([]*index.CollectedEvent, error) {
	if e.eventFilterManager == nil {
		return nil, api.ErrNotSupported
	}

	if e.chainIndexer == nil {
		return nil, ErrChainIndexerDisabled
	}

	pf, err := e.parseEthFilterSpec(filterSpec)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse eth filter spec: %w", err)
	}

	head := e.chainStore.GetHeaviestTipSet()
	// should not ask for events for a tipset >= head because of deferred execution
	if pf.tipsetCid != cid.Undef {
		ts, err := e.chainStore.GetTipSetByCid(ctx, pf.tipsetCid)
		if err != nil {
			return nil, xerrors.Errorf("failed to get tipset by cid: %w", err)
		}
		if ts.Height() >= head.Height() {
			return nil, xerrors.New("cannot ask for events for a tipset at or greater than head")
		}
	}

	if pf.minHeight >= head.Height() || pf.maxHeight >= head.Height() {
		return nil, xerrors.New("cannot ask for events for a tipset at or greater than head")
	}

	ef := &index.EventFilter{
		MinHeight:     pf.minHeight,
		MaxHeight:     pf.maxHeight,
		TipsetCid:     pf.tipsetCid,
		Addresses:     pf.addresses,
		KeysWithCodec: pf.keys,
		Codec:         multicodec.Raw,
		MaxResults:    e.eventFilterManager.MaxFilterResults,
	}

	ces, err := e.chainIndexer.GetEventsForFilter(ctx, ef)
	if err != nil {
		return nil, xerrors.Errorf("failed to get events for filter from chain indexer: %w", err)
	}

	return ces, nil
}

func ethFilterResultFromEvents(ctx context.Context, evs []*index.CollectedEvent, cs ChainStore, sa StateManager) (*ethtypes.EthFilterResult, error) {
	logs, err := ethFilterLogsFromEvents(ctx, evs, cs, sa)
	if err != nil {
		return nil, err
	}

	res := &ethtypes.EthFilterResult{}
	for _, log := range logs {
		res.Results = append(res.Results, log)
	}

	return res, nil
}

func ethFilterResultFromTipSets(tsks []types.TipSetKey) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}

	for _, tsk := range tsks {
		c, err := tsk.Cid()
		if err != nil {
			return nil, err
		}
		hash, err := ethtypes.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

func ethFilterResultFromMessages(cs []*types.SignedMessage) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}

	for _, c := range cs {
		hash, err := ethTxHashFromSignedMessage(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

func ethFilterLogsFromEvents(ctx context.Context, evs []*index.CollectedEvent, cs ChainStore, sa StateManager) ([]ethtypes.EthLog, error) {
	var logs []ethtypes.EthLog
	for _, ev := range evs {
		log := ethtypes.EthLog{
			Removed:          ev.Reverted,
			LogIndex:         ethtypes.EthUint64(ev.EventIdx),
			TransactionIndex: ethtypes.EthUint64(ev.MsgIdx),
			BlockNumber:      ethtypes.EthUint64(ev.Height),
		}
		var (
			err error
			ok  bool
		)

		log.Data, log.Topics, ok = ethLogFromEvent(ev.Entries)
		if !ok {
			continue
		}

		log.Address, err = ethtypes.EthAddressFromFilecoinAddress(ev.EmitterAddr)
		if err != nil {
			return nil, err
		}

		log.TransactionHash, err = ethTxHashFromMessageCid(ctx, ev.MsgCid, cs)
		if err != nil {
			return nil, err
		}
		if log.TransactionHash == ethtypes.EmptyEthHash {
			// We've garbage collected the message, ignore the events and continue.
			continue
		}
		c, err := ev.TipSetKey.Cid()
		if err != nil {
			return nil, err
		}
		log.BlockHash, err = ethtypes.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		logs = append(logs, log)
	}

	return logs, nil
}

func ethLogFromEvent(entries []types.EventEntry) (data []byte, topics []ethtypes.EthHash, ok bool) {
	var (
		topicsFound      [4]bool
		topicsFoundCount int
		dataFound        bool
	)
	// Topics must be non-nil, even if empty. So we might as well pre-allocate for 4 (the max).
	topics = make([]ethtypes.EthHash, 0, 4)
	for _, entry := range entries {
		// Drop events with non-raw topics. Built-in actors emit CBOR, and anything else would be
		// invalid anyway.
		if entry.Codec != cid.Raw {
			return nil, nil, false
		}
		// Check if the key is t1..t4
		if len(entry.Key) == 2 && "t1" <= entry.Key && entry.Key <= "t4" {
			// '1' - '1' == 0, etc.
			idx := int(entry.Key[1] - '1')

			// Drop events with mis-sized topics.
			if len(entry.Value) != 32 {
				log.Warnw("got an EVM event topic with an invalid size", "key", entry.Key, "size", len(entry.Value))
				return nil, nil, false
			}

			// Drop events with duplicate topics.
			if topicsFound[idx] {
				log.Warnw("got a duplicate EVM event topic", "key", entry.Key)
				return nil, nil, false
			}
			topicsFound[idx] = true
			topicsFoundCount++

			// Extend the topics array
			for len(topics) <= idx {
				topics = append(topics, ethtypes.EthHash{})
			}
			copy(topics[idx][:], entry.Value)
		} else if entry.Key == "d" {
			// Drop events with duplicate data fields.
			if dataFound {
				log.Warnw("got duplicate EVM event data")
				return nil, nil, false
			}

			dataFound = true
			data = entry.Value
		} else {
			// Skip entries we don't understand (makes it easier to extend things).
			// But we warn for now because we don't expect them.
			log.Warnw("unexpected event entry", "key", entry.Key)
		}

	}

	// Drop events with skipped topics.
	if len(topics) != topicsFoundCount {
		log.Warnw("EVM event topic length mismatch", "expected", len(topics), "actual", topicsFoundCount)
		return nil, nil, false
	}
	return data, topics, true
}

func ethTxHashFromMessageCid(ctx context.Context, c cid.Cid, cs ChainStore) (ethtypes.EthHash, error) {
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

// parseBlockRange is similar to actor event's parseHeightRange but with slightly different semantics
//
// * "block" instead of "height"
// * strings that can have "latest" and "earliest" and nil
// * hex strings for actual heights
//
// Note: Ethereum supports more than "latest" and "earliest" (e.g. "finalized" and "safe"), but
// currently we do not. The use-case isn't as clear for this but it's possible to add in the future
// if needed.
func parseBlockRange(heaviest abi.ChainEpoch, fromBlock, toBlock *string, maxRange abi.ChainEpoch) (minHeight abi.ChainEpoch, maxHeight abi.ChainEpoch, err error) {
	if fromBlock == nil || *fromBlock == "latest" || len(*fromBlock) == 0 {
		minHeight = heaviest
	} else if *fromBlock == "earliest" {
		minHeight = 0
	} else {
		if !strings.HasPrefix(*fromBlock, "0x") {
			return 0, 0, xerrors.New("FromBlock is not a hex")
		}
		epoch, err := ethtypes.EthUint64FromHex(*fromBlock)
		if err != nil || epoch > math.MaxInt64 {
			return 0, 0, xerrors.New("invalid epoch")
		}
		minHeight = abi.ChainEpoch(epoch)
	}

	if toBlock == nil || *toBlock == "latest" || len(*toBlock) == 0 {
		// here latest means the latest at the time
		maxHeight = -1
	} else if *toBlock == "earliest" {
		maxHeight = 0
	} else {
		if !strings.HasPrefix(*toBlock, "0x") {
			return 0, 0, xerrors.New("ToBlock is not a hex")
		}
		epoch, err := ethtypes.EthUint64FromHex(*toBlock)
		if err != nil || epoch > math.MaxInt64 {
			return 0, 0, xerrors.New("invalid epoch")
		}
		maxHeight = abi.ChainEpoch(epoch)
	}

	// Validate height ranges are within limits set by node operator
	if minHeight == -1 && maxHeight > 0 {
		// Here the client is looking for events between the head and some future height
		if maxHeight-heaviest > maxRange {
			return 0, 0, xerrors.Errorf("invalid epoch range: to block is too far in the future (maximum: %d)", maxRange)
		}
	} else if minHeight >= 0 && maxHeight == -1 {
		// Here the client is looking for events between some time in the past and the current head
		if heaviest-minHeight > maxRange {
			return 0, 0, xerrors.Errorf("invalid epoch range: from block is too far in the past (maximum: %d)", maxRange)
		}
	} else if minHeight >= 0 && maxHeight >= 0 {
		if minHeight > maxHeight {
			return 0, 0, xerrors.Errorf("invalid epoch range: to block (%d) must be after from block (%d)", minHeight, maxHeight)
		} else if maxHeight-minHeight > maxRange {
			return 0, 0, xerrors.Errorf("invalid epoch range: range between to and from blocks is too large (maximum: %d)", maxRange)
		}
	}
	return minHeight, maxHeight, nil
}

type parsedFilter struct {
	minHeight abi.ChainEpoch
	maxHeight abi.ChainEpoch
	tipsetCid cid.Cid
	addresses []address.Address
	keys      map[string][]types.ActorEventBlock
}

func (e *ethEvents) parseEthFilterSpec(filterSpec *ethtypes.EthFilterSpec) (*parsedFilter, error) {
	var (
		minHeight abi.ChainEpoch
		maxHeight abi.ChainEpoch
		tipsetCid cid.Cid
		addresses []address.Address
		keys      = map[string][][]byte{}
	)

	if filterSpec.BlockHash != nil {
		if filterSpec.FromBlock != nil || filterSpec.ToBlock != nil {
			return nil, xerrors.New("must not specify block hash and from/to block")
		}

		tipsetCid = filterSpec.BlockHash.ToCid()
	} else {
		var err error
		// Because of deferred execution, we need to subtract 1 from the heaviest tipset height for the "heaviest" parameter
		minHeight, maxHeight, err = parseBlockRange(e.chainStore.GetHeaviestTipSet().Height()-1, filterSpec.FromBlock, filterSpec.ToBlock, e.maxFilterHeightRange)
		if err != nil {
			return nil, err
		}
	}

	// Convert all addresses to filecoin f4 addresses
	for _, ea := range filterSpec.Address {
		a, err := ea.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("invalid address %x", ea)
		}
		addresses = append(addresses, a)
	}

	keys, err := parseEthTopics(filterSpec.Topics)
	if err != nil {
		return nil, err
	}

	return &parsedFilter{
		minHeight: minHeight,
		maxHeight: maxHeight,
		tipsetCid: tipsetCid,
		addresses: addresses,
		keys:      keysToKeysWithCodec(keys),
	}, nil
}

func keysToKeysWithCodec(keys map[string][][]byte) map[string][]types.ActorEventBlock {
	keysWithCodec := make(map[string][]types.ActorEventBlock)
	for k, v := range keys {
		for _, vv := range v {
			keysWithCodec[k] = append(keysWithCodec[k], types.ActorEventBlock{
				Codec: uint64(multicodec.Raw), // FEVM smart contract events are always encoded with the `raw` Codec.
				Value: vv,
			})
		}
	}
	return keysWithCodec
}

func (e *ethEvents) uninstallFilter(ctx context.Context, f filter.Filter) error {
	switch f.(type) {
	case filter.EventFilter:
		err := e.eventFilterManager.Remove(ctx, f.ID())
		if err != nil && !errors.Is(err, filter.ErrFilterNotFound) {
			return err
		}
	case *filter.TipSetFilter:
		err := e.tipSetFilterManager.Remove(ctx, f.ID())
		if err != nil && !errors.Is(err, filter.ErrFilterNotFound) {
			return err
		}
	case *filter.MemPoolFilter:
		err := e.memPoolFilterManager.Remove(ctx, f.ID())
		if err != nil && !errors.Is(err, filter.ErrFilterNotFound) {
			return err
		}
	default:
		return xerrors.New("unknown filter type")
	}

	return e.filterStore.Remove(ctx, f.ID())
}

type EthEventsDisabled struct{}

func (EthEventsDisabled) EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) {
	return nil, ErrModuleDisabled
}
func (EthEventsDisabled) EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	return ethtypes.EthFilterID{}, ErrModuleDisabled
}
func (EthEventsDisabled) EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	return ethtypes.EthFilterID{}, ErrModuleDisabled
}
func (EthEventsDisabled) EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) {
	return ethtypes.EthFilterID{}, ErrModuleDisabled
}
func (EthEventsDisabled) EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) {
	return false, ErrModuleDisabled
}
func (EthEventsDisabled) EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	return nil, ErrModuleDisabled
}
func (EthEventsDisabled) EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	return nil, ErrModuleDisabled
}
func (EthEventsDisabled) EthSubscribe(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	return ethtypes.EthSubscriptionID{}, ErrModuleDisabled
}
func (EthEventsDisabled) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	return false, ErrModuleDisabled
}

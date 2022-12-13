package gateway

import (
	"context"
	"fmt"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"golang.org/x/time/rate"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/delegated"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/impl/full"
)

const (
	DefaultLookbackCap            = time.Hour * 24
	DefaultStateWaitLookbackLimit = abi.ChainEpoch(20)
	DefaultRateLimitTimeout       = time.Second * 5
	basicRateLimitTokens          = 1
	walletRateLimitTokens         = 1
	chainRateLimitTokens          = 2
	stateRateLimitTokens          = 3
)

// TargetAPI defines the API methods that the Node depends on
// (to make it easy to mock for tests)
type TargetAPI interface {
	Version(context.Context) (api.APIVersion, error)
	ChainGetParentMessages(context.Context, cid.Cid) ([]api.Message, error)
	ChainGetParentReceipts(context.Context, cid.Cid) ([]*types.MessageReceipt, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	ChainGetNode(ctx context.Context, p string) (*api.IpldObject, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainPutObj(context.Context, blocks.Block) error
	ChainGetGenesis(context.Context) (*types.TipSet, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
	MpoolPushUntrusted(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)
	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
	MsigGetVestingSchedule(context.Context, address.Address, types.TipSetKey) (api.MsigVesting, error)
	MsigGetPending(ctx context.Context, addr address.Address, ts types.TipSetKey) ([]*api.MsigTransaction, error)
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error)
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error)
	StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error)
	StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (network.Version, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*api.MinerPower, error)
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)
	StateMinerRecoveries(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerDeadlines(context.Context, address.Address, types.TipSetKey) ([]api.Deadline, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateCirculatingSupply(context.Context, types.TipSetKey) (abi.TokenAmount, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error)
	StateVMCirculatingSupplyInternal(context.Context, types.TipSetKey) (api.CirculatingSupply, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)

	EthBlockNumber(ctx context.Context) (api.EthUint64, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthUint64) (api.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (api.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash *api.EthHash) (*api.EthTx, error)
	EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkOpt string) (api.EthUint64, error)
	EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (*api.EthTxReceipt, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthUint64) (api.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthUint64, txIndex api.EthUint64) (api.EthTx, error)
	EthGetCode(ctx context.Context, address api.EthAddress, blkOpt string) (api.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthBytes, blkParam string) (api.EthBytes, error)
	EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error)
	EthChainId(ctx context.Context) (api.EthUint64, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (api.EthUint64, error)
	EthGasPrice(ctx context.Context) (api.EthBigInt, error)
	EthFeeHistory(ctx context.Context, blkCount api.EthUint64, newestBlk string, rewardPercentiles []float64) (api.EthFeeHistory, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (api.EthBigInt, error)
	EthEstimateGas(ctx context.Context, tx api.EthCall) (api.EthUint64, error)
	EthCall(ctx context.Context, tx api.EthCall, blkParam string) (api.EthBytes, error)
	EthSendRawTransaction(ctx context.Context, rawTx api.EthBytes) (api.EthHash, error)
	EthGetLogs(ctx context.Context, filter *api.EthFilterSpec) (*api.EthFilterResult, error)
	EthGetFilterChanges(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error)
	EthGetFilterLogs(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error)
	EthNewFilter(ctx context.Context, filter *api.EthFilterSpec) (api.EthFilterID, error)
	EthNewBlockFilter(ctx context.Context) (api.EthFilterID, error)
	EthNewPendingTransactionFilter(ctx context.Context) (api.EthFilterID, error)
	EthUninstallFilter(ctx context.Context, id api.EthFilterID) (bool, error)
	EthSubscribe(ctx context.Context, eventType string, params *api.EthSubscriptionParams) (<-chan api.EthSubscriptionResponse, error)
	EthUnsubscribe(ctx context.Context, id api.EthSubscriptionID) (bool, error)
}

var _ TargetAPI = *new(api.FullNode) // gateway depends on latest

type Node struct {
	target                 TargetAPI
	lookbackCap            time.Duration
	stateWaitLookbackLimit abi.ChainEpoch
	rateLimiter            *rate.Limiter
	rateLimitTimeout       time.Duration
	errLookback            error
}

var (
	_ api.Gateway         = (*Node)(nil)
	_ full.ChainModuleAPI = (*Node)(nil)
	_ full.GasModuleAPI   = (*Node)(nil)
	_ full.MpoolModuleAPI = (*Node)(nil)
	_ full.StateModuleAPI = (*Node)(nil)
)

// NewNode creates a new gateway node.
func NewNode(api TargetAPI, lookbackCap time.Duration, stateWaitLookbackLimit abi.ChainEpoch, rateLimit int64, rateLimitTimeout time.Duration) *Node {
	var limit rate.Limit
	if rateLimit == 0 {
		limit = rate.Inf
	} else {
		limit = rate.Every(time.Second / time.Duration(rateLimit))
	}
	return &Node{
		target:                 api,
		lookbackCap:            lookbackCap,
		stateWaitLookbackLimit: stateWaitLookbackLimit,
		rateLimiter:            rate.NewLimiter(limit, stateRateLimitTokens),
		rateLimitTimeout:       rateLimitTimeout,
		errLookback:            fmt.Errorf("lookbacks of more than %s are disallowed", lookbackCap),
	}
}

func (gw *Node) checkTipsetKey(ctx context.Context, tsk types.TipSetKey) error {
	if tsk.IsEmpty() {
		return nil
	}

	ts, err := gw.target.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return err
	}

	return gw.checkTipset(ts)
}

func (gw *Node) checkTipset(ts *types.TipSet) error {
	at := time.Unix(int64(ts.Blocks()[0].Timestamp), 0)
	if err := gw.checkTimestamp(at); err != nil {
		return fmt.Errorf("bad tipset: %w", err)
	}
	return nil
}

func (gw *Node) checkTipsetHeight(ts *types.TipSet, h abi.ChainEpoch) error {
	tsBlock := ts.Blocks()[0]
	heightDelta := time.Duration(uint64(tsBlock.Height-h)*build.BlockDelaySecs) * time.Second
	timeAtHeight := time.Unix(int64(tsBlock.Timestamp), 0).Add(-heightDelta)

	if err := gw.checkTimestamp(timeAtHeight); err != nil {
		return fmt.Errorf("bad tipset height: %w", err)
	}
	return nil
}

func (gw *Node) checkTimestamp(at time.Time) error {
	if time.Since(at) > gw.lookbackCap {
		return gw.errLookback
	}
	return nil
}

func (gw *Node) limit(ctx context.Context, tokens int) error {
	ctx2, cancel := context.WithTimeout(ctx, gw.rateLimitTimeout)
	defer cancel()
	if perConnLimiter, ok := ctx2.Value(perConnLimiterKey).(*rate.Limiter); ok {
		err := perConnLimiter.WaitN(ctx2, tokens)
		if err != nil {
			return fmt.Errorf("connection limited. %w", err)
		}
	}

	err := gw.rateLimiter.WaitN(ctx2, tokens)
	if err != nil {
		stats.Record(ctx, metrics.RateLimitCount.M(1))
		return fmt.Errorf("server busy. %w", err)
	}
	return nil
}

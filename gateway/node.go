package gateway

import (
	"context"
	"fmt"
	"time"

	logger "github.com/ipfs/go-log/v2"
	"go.opencensus.io/stats"
	"golang.org/x/time/rate"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/delegated"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/lotus/metrics"
)

var log = logger.Logger("gateway")

const (
	DefaultMaxLookbackDuration      = time.Hour * 24     // Default duration that a gateway request can look back in chain history
	DefaultMaxMessageLookbackEpochs = abi.ChainEpoch(20) // Default number of epochs that a gateway message lookup can look back in chain history
	DefaultRateLimitTimeout         = time.Second * 5    // Default timeout for rate limiting requests; where a request would take longer to wait than this value, it will be rejected
	DefaultEthMaxFiltersPerConn     = 16                 // Default maximum number of ETH filters and subscriptions per websocket connection

	basicRateLimitTokens  = 1
	walletRateLimitTokens = 1
	chainRateLimitTokens  = 2
	stateRateLimitTokens  = 3

	MaxRateLimitTokens = stateRateLimitTokens // Number of tokens consumed for the most expensive types of operations
)

type Node struct {
	v1Proxy                  *reverseProxyV1
	v2Proxy                  *reverseProxyV2
	maxLookbackDuration      time.Duration
	maxMessageLookbackEpochs abi.ChainEpoch
	rateLimiter              *rate.Limiter
	rateLimitTimeout         time.Duration
	ethMaxFiltersPerConn     int
	errLookback              error
}

type options struct {
	v1SubHandler             *EthSubHandler
	v2SubHandler             *EthSubHandler
	maxLookbackDuration      time.Duration
	maxMessageLookbackEpochs abi.ChainEpoch
	rateLimit                int
	rateLimitTimeout         time.Duration
	ethMaxFiltersPerConn     int
}

type Option func(*options)

// WithV1EthSubHandler sets the Ethereum subscription handler for the gateway node. This is used for
// the RPC reverse handler for EthSubscribe calls.
func WithV1EthSubHandler(subHandler *EthSubHandler) Option {
	return func(opts *options) {
		opts.v1SubHandler = subHandler
	}
}

// WithV2EthSubHandler sets the Ethereum subscription handler for the gateway node. This is used for
// the RPC reverse handler for EthSubscribe calls.
func WithV2EthSubHandler(subHandler *EthSubHandler) Option {
	return func(opts *options) {
		opts.v2SubHandler = subHandler
	}
}

// WithMaxLookbackDuration sets the maximum lookback duration (time) for state queries.
func WithMaxLookbackDuration(maxLookbackDuration time.Duration) Option {
	return func(opts *options) {
		opts.maxLookbackDuration = maxLookbackDuration
	}
}

// WithMaxMessageLookbackEpochs sets the maximum lookback (epochs) for state queries.
func WithMaxMessageLookbackEpochs(maxMessageLookbackEpochs abi.ChainEpoch) Option {
	return func(opts *options) {
		opts.maxMessageLookbackEpochs = maxMessageLookbackEpochs
	}
}

// WithRateLimit sets the maximum number of requests per second globally that will be allowed
// before the gateway starts to rate limit requests.
func WithRateLimit(rateLimit int) Option {
	return func(opts *options) {
		opts.rateLimit = rateLimit
	}
}

// WithRateLimitTimeout sets the timeout for rate limiting requests such that when rate limiting is
// being applied, if the timeout is reached the request will be allowed.
func WithRateLimitTimeout(rateLimitTimeout time.Duration) Option {
	return func(opts *options) {
		opts.rateLimitTimeout = rateLimitTimeout
	}
}

// WithEthMaxFiltersPerConn sets the maximum number of Ethereum filters and subscriptions that can
// be maintained per websocket connection.
func WithEthMaxFiltersPerConn(ethMaxFiltersPerConn int) Option {
	return func(opts *options) {
		opts.ethMaxFiltersPerConn = ethMaxFiltersPerConn
	}
}

// NewNode creates a new gateway node.
func NewNode(v1 v1api.FullNode, v2 v2api.FullNode, opts ...Option) *Node {
	options := &options{
		maxLookbackDuration:      DefaultMaxLookbackDuration,
		maxMessageLookbackEpochs: DefaultMaxMessageLookbackEpochs,
		rateLimitTimeout:         DefaultRateLimitTimeout,
		ethMaxFiltersPerConn:     DefaultEthMaxFiltersPerConn,
	}
	for _, opt := range opts {
		opt(options)
	}

	limit := rate.Inf
	if options.rateLimit > 0 {
		limit = rate.Every(time.Second / time.Duration(options.rateLimit))
	}
	gateway := &Node{
		maxLookbackDuration:      options.maxLookbackDuration,
		maxMessageLookbackEpochs: options.maxMessageLookbackEpochs,
		rateLimiter:              rate.NewLimiter(limit, MaxRateLimitTokens), // allow for a burst of MaxRateLimitTokens
		rateLimitTimeout:         options.rateLimitTimeout,
		errLookback:              fmt.Errorf("lookbacks of more than %s are disallowed", options.maxLookbackDuration),
		ethMaxFiltersPerConn:     options.ethMaxFiltersPerConn,
	}
	gateway.v1Proxy = &reverseProxyV1{
		gateway:       gateway,
		server:        v1,
		subscriptions: options.v1SubHandler,
	}
	gateway.v2Proxy = &reverseProxyV2{
		gateway:       gateway,
		server:        v2,
		subscriptions: options.v2SubHandler,
	}
	return gateway
}

func (gw *Node) V1ReverseProxy() api.Gateway { return gw.v1Proxy }

func (gw *Node) V2ReverseProxy() v2api.Gateway { return gw.v2Proxy }

func (gw *Node) checkTipSetKey(ctx context.Context, tsk types.TipSetKey) error {
	if tsk.IsEmpty() {
		return nil
	}

	ts, err := gw.v1Proxy.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return err
	}

	return gw.checkTipSet(ts)
}

func (gw *Node) checkTipSet(ts *types.TipSet) error {
	at := time.Unix(int64(ts.Blocks()[0].Timestamp), 0)
	if err := gw.checkTimestamp(at); err != nil {
		return fmt.Errorf("bad tipset: %w", err)
	}
	return nil
}

func (gw *Node) checkKeyedTipSetHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) error {
	var ts *types.TipSet
	if tsk.IsEmpty() {
		head, err := gw.v1Proxy.ChainHead(ctx)
		if err != nil {
			return err
		}
		ts = head
	} else {
		gts, err := gw.v1Proxy.ChainGetTipSet(ctx, tsk)
		if err != nil {
			return err
		}
		ts = gts
	}

	// Check if the tipset key refers to gw tipset that's too far in the past
	if err := gw.checkTipSet(ts); err != nil {
		return err
	}

	// Check if the height is too far in the past
	if err := gw.checkTipSetHeight(ts, h); err != nil {
		return err
	}

	return nil
}

func (gw *Node) checkTipSetHeight(ts *types.TipSet, h abi.ChainEpoch) error {
	if h > ts.Height() {
		return fmt.Errorf("tipset height in future")
	}
	tsBlock := ts.Blocks()[0]
	heightDelta := time.Duration(uint64(tsBlock.Height-h)*buildconstants.BlockDelaySecs) * time.Second
	timeAtHeight := time.Unix(int64(tsBlock.Timestamp), 0).Add(-heightDelta)

	if err := gw.checkTimestamp(timeAtHeight); err != nil {
		return fmt.Errorf("bad tipset height: %w", err)
	}
	return nil
}

func (gw *Node) checkTimestamp(at time.Time) error {
	if time.Since(at) > gw.maxLookbackDuration {
		return gw.errLookback
	}
	return nil
}

func (gw *Node) limit(ctx context.Context, tokens int) error {
	ctx2, cancel := context.WithTimeout(ctx, gw.rateLimitTimeout)
	defer cancel()

	if perConnLimiter, ok := getPerConnectionAPIRateLimiter(ctx); ok {
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

# The Message Pool

The Message Pool (mpool) is the component of lotus that handles
pending messages for inclusion in the chain. Messages are added to the
mpool either directly for locally published messages or through pubsub
propagation.  Whenever a miner is ready to create a block for a
tipset, it invokes the mpool selection algorithm which selects an
appropriate set of messages such that it optimizes miner reward and
chain capacity.

## API

The full node API defines the following methods for interacting with the mpool:
```
    MpoolPending(context.Context, types.TipSetKey) ([]*types.SignedMessage, error)
    MpoolSelect(context.Context, types.TipSetKey, float64) ([]*types.SignedMessage, error)
    MpoolPush(context.Context, *types.SignedMessage) (cid.Cid, error)
    MpoolPushMessage(ctx context.Context, msg *types.Message, spec *MessageSendSpec) (*types.SignedMessage, error)
    MpoolGetNonce(context.Context, address.Address) (uint64, error)
    MpoolSub(context.Context) (<-chan MpoolUpdate, error)
    MpoolGetConfig(context.Context) (*types.MpoolConfig, error)
    MpoolSetConfig(context.Context, *types.MpoolConfig) error
    MpoolClear(context.Context, local bool) error
```

### MpoolPending

Returns the list of messages that are pending for a tipset.

### MpoolSelect

Selects and returns a list of pending messages for inclusion in the next block.

### MpoolPush

Pushes a signed message to the mpool; returns the CID of the message.

### MpoolPushMessage

Atomically assigns a nonce, signs, and pushes a message to the mpool.

The MaxFee field of the spec argument is only used when
GasFeeCap/GasPremium fields aren't specified in the message. When
maxFee is set to 0, MpoolPushMessage will guess appropriate fee based
on current chain conditions.

### MpoolGetNonce

Returns the next nonce for the specified sender. Note that this method may not be atomic.
Use `MpoolPushMessage` instead.

### MpoolSub

Returns a channel to receive notifications about updates to the message pool.
Note that the context *must* be canceled when the caller is done with the subscription.

### MpoolGetConfig

Returns (a copy of) the current mpool configuration.

### MpoolSetConfig

Sets the mpool configuration to (a copy of) the supplied configuration object.

### MpoolClear

Clears pending messages from the mpool; if `local` is `true` then local messages are also cleared and removed from the datastore.

This should be used with extreme care and only in the case of errors during head changes that
would leave the mpool in an inconsistent state.


## Command Line Interface

The lotus command line interface defines an `mpool` command which
allows a user to interact with the mpool.

The following commands are supported:
```
lotus mpool pending [--local]
lotus mpool sub
lotus mpool stat [--local]
lotus mpool replace [--gas-feecap <feecap>] [--gas-premium <premium>] [--gas-limit <limit>] [from] [nonce]
lotus mpool find [--from <address>] [--to <address>] [--method <int>]
lotus mpool config [<configuration>]
lotus mpool clear [--local]
```

### lotus mpool pending
Prints the pending messages in the mpool, as returned by the `MpoolPending` API call.
If `--local` is specified, it only prints pending messages for addresses in the local wallet.

### lotus mpool sub
Subscribes to mpool changes using the `MpoolSub` API call and prints the stream of mpool
updates.

### lotus mpool stat
Prints various mpool statistics.
If `--local` is specified then only prints statistics for addresses in the local wallet.

### lotus mpool replace
Replaces a message in the mpool.

### lotus mpool find
Searches for messages in the mpool.

### lotus mpool config
Gets or sets the current mpool configuration.

### lotus mpool clear
Unconditionally clears pending messages from the mpool.
If the `--local` flag is passed, then local messages are also cleared; otherwise local messages are retained.

*Warning*: this command should only be used in the case of head change errors leaving the mpool in a state.

## Configuration

The mpool has a few parameters that can be configured by the user, either through the API
or the command line interface.

The config struct is defined as follows:
```
type MpoolConfig struct {
	PriorityAddrs          []address.Address
	SizeLimitHigh          int
	SizeLimitLow           int
	ReplaceByFeeRatio      float64
	PruneCooldown          time.Duration
	GasLimitOverestimation float64
}

```

The meaning of these fields is as follows:
- `PriorityAddrs` -- these are the addresses of actors whose pending messages should always
  be included in a block during message selection, as long as they are profitable.
  Miners should configure their own worker addresses so that they include their own messages
  when they produce a new block.
  Default is empty.
- `SizeLimitHigh` -- this is the maximum number of pending messages before triggering a
  prune in the message pool. Note that messages from priority addresses are never pruned.
  Default is 30000.
- `SizeLimitLow` -- this is the number of pending messages that should be kept after a prune.
  Default is 20000.
- `ReplaceByFeeRatio` -- this is the gas fee ratio for replacing messages in the mpool.
  Whenever a message is replaced, the `GasPremium` must be increased by this ratio.
  Default is 1.25.
- `PruneCooldown` -- this is the period of time to wait before triggering a new prune.
  Default is 1min.
- `GasLimitOverestimation` -- this is a parameter that controls the gas limit overestimation for new messages.
  Default is 1.25.


## Message Selection

A few words regarding message selection are pertinent. In message
selection, a miner selects a set of messages from the pending messages
for inclusion to the next block in order to maximize its reward. The
problem however is NP-hard (it's an instance of knapsack packing),
further confounded by the fact that miners don't communicate their
tickets to each other. So at best we can approximate the optimal
selection in a reasonable amount of time.

The mpool employs a sophisticated algorithm for selecting messages for
inclusion, given the ticket quality of a miner. The ticket quality
reflects the probability of execution order for a block in the
tipset. Given the ticket quality the algorithm computes the
probability of each block, and picks dependent chains of messages such
that the reward is maximized, while also optimizing the capacity of
the chain.  If the ticket quality is sufficiently high, then a greedy
selection algorithm is used instead that simply picks dependent chains of
maximum reward.  Note that pending message chains from priority addresses
are always selected, regardless of their profitability.

For algorithm details, please refer to the implementation in
`chain/messagepool/selection.go`.

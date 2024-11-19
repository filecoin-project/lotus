package index

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var ErrNotFound = errors.New("not found in index")
var ErrClosed = errors.New("index closed")

// MsgInfo is the Message metadata the index tracks.
type MsgInfo struct {
	// the message this record refers to
	Message cid.Cid
	// the tipset where this message was included
	TipSet cid.Cid
	// the epoch where this message was included
	Epoch abi.ChainEpoch
}

type CollectedEvent struct {
	Entries     []types.EventEntry
	EmitterAddr address.Address // address of emitter
	EventIdx    int             // index of the event within the list of emitted events in a given tipset
	Reverted    bool
	Height      abi.ChainEpoch
	TipSetKey   types.TipSetKey // tipset that contained the message
	MsgIdx      int             // index of the message in the tipset
	MsgCid      cid.Cid         // cid of message that produced event
}

type EventFilter struct {
	MinHeight abi.ChainEpoch // minimum epoch to apply filter or -1 if no minimum
	MaxHeight abi.ChainEpoch // maximum epoch to apply filter or -1 if no maximum
	TipsetCid cid.Cid
	Addresses []address.Address // list of actor addresses that are extpected to emit the event

	KeysWithCodec map[string][]types.ActorEventBlock // map of key names to a list of alternate values that may match
	MaxResults    int                                // maximum number of results to collect, 0 is unlimited

	Codec multicodec.Code // optional codec filter, only used if KeysWithCodec is not set
}

type Indexer interface {
	Start()
	ReconcileWithChain(ctx context.Context, currHead *types.TipSet) error
	IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error
	IndexEthTxHash(ctx context.Context, txHash ethtypes.EthHash, c cid.Cid) error

	SetActorToDelegatedAddresFunc(idToRobustAddrFunc ActorToDelegatedAddressFunc)
	SetRecomputeTipSetStateFunc(f RecomputeTipSetStateFunc)

	Apply(ctx context.Context, from, to *types.TipSet) error
	Revert(ctx context.Context, from, to *types.TipSet) error

	// Returns (cid.Undef, nil) if the message was not found
	GetCidFromHash(ctx context.Context, hash ethtypes.EthHash) (cid.Cid, error)
	// Returns (nil, ErrNotFound) if the message was not found
	GetMsgInfo(ctx context.Context, m cid.Cid) (*MsgInfo, error)

	GetEventsForFilter(ctx context.Context, f *EventFilter) ([]*CollectedEvent, error)

	ChainValidateIndex(ctx context.Context, epoch abi.ChainEpoch, backfill bool) (*types.IndexValidation, error)

	Close() error
}

type ChainStore interface {
	MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error)
	GetHeaviestTipSet() *types.TipSet
	GetTipSetByCid(ctx context.Context, tsKeyCid cid.Cid) (*types.TipSet, error)
	GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	MessagesForBlock(ctx context.Context, b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
	ActorStore(ctx context.Context) adt.Store
	GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, ts *types.TipSet, prev bool) (*types.TipSet, error)
	IsStoringEvents() bool
}

var _ ChainStore = (*store.ChainStore)(nil)

package paychmgr

import (
	"context"
	"sync"

	"github.com/filecoin-project/lotus/node/modules/helpers"

	"github.com/ipfs/go-datastore"

	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"

	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
)

var log = logging.Logger("paych")

// PaychAPI is used by dependency injection to pass the consituent APIs to NewManager()
type PaychAPI struct {
	fx.In

	full.MpoolAPI
	full.StateAPI
}

// stateManagerAPI defines the methods needed from StateManager
type stateManagerAPI interface {
	LoadActorState(ctx context.Context, a address.Address, out interface{}, ts *types.TipSet) (*types.Actor, error)
	Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error)
	AdtStore(ctx context.Context) adt.Store
}

// paychAPI defines the API methods needed by the payment channel manager
type paychAPI interface {
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message, maxFee *api.MessageSendSpec) (*types.SignedMessage, error)
	WalletHas(ctx context.Context, addr address.Address) (bool, error)
}

// managerAPI defines all methods needed by the manager
type managerAPI interface {
	stateManagerAPI
	paychAPI
}

// managerAPIImpl is used to create a composite that implements managerAPI
type managerAPIImpl struct {
	*stmgr.StateManager
	paychAPI
}

func (m *managerAPIImpl) AdtStore(ctx context.Context) adt.Store {
	return m.ChainStore().Store(ctx)
}

type Manager struct {
	// The Manager context is used to terminate wait operations on shutdown
	ctx      context.Context
	shutdown context.CancelFunc

	store  *Store
	sa     *stateAccessor
	pchapi managerAPI

	lk       sync.RWMutex
	channels map[string]*channelAccessor
}

func NewManager(mctx helpers.MetricsCtx, lc fx.Lifecycle, sm *stmgr.StateManager, pchstore *Store, api PaychAPI) *Manager {
	ctx := helpers.LifecycleCtx(mctx, lc)
	ctx, shutdown := context.WithCancel(ctx)

	impl := &managerAPIImpl{StateManager: sm, paychAPI: &api}
	return &Manager{
		ctx:      ctx,
		shutdown: shutdown,
		store:    pchstore,
		sa:       &stateAccessor{sm: impl},
		channels: make(map[string]*channelAccessor),
		pchapi:   impl,
	}
}

// newManager is used by the tests to supply mocks
func newManager(pchstore *Store, pchapi managerAPI) (*Manager, error) {
	pm := &Manager{
		store:    pchstore,
		sa:       &stateAccessor{sm: pchapi},
		channels: make(map[string]*channelAccessor),
		pchapi:   pchapi,
	}
	return pm, pm.Start()
}

// HandleManager is called by dependency injection to set up hooks
func HandleManager(lc fx.Lifecycle, pm *Manager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return pm.Start()
		},
		OnStop: func(context.Context) error {
			return pm.Stop()
		},
	})
}

// Start restarts tracking of any messages that were sent to chain.
func (pm *Manager) Start() error {
	return pm.restartPending()
}

// Stop shuts down any processes used by the manager
func (pm *Manager) Stop() error {
	pm.shutdown()
	return nil
}

func (pm *Manager) GetPaych(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, cid.Cid, error) {
	chanAccessor, err := pm.accessorByFromTo(from, to)
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	return chanAccessor.getPaych(ctx, from, to, amt)
}

// GetPaychWaitReady waits until the create channel / add funds message with the
// given message CID arrives.
// The returned channel address can safely be used against the Manager methods.
func (pm *Manager) GetPaychWaitReady(ctx context.Context, mcid cid.Cid) (address.Address, error) {
	// Find the channel associated with the message CID
	pm.lk.Lock()
	ci, err := pm.store.ByMessageCid(mcid)
	pm.lk.Unlock()

	if err != nil {
		if err == datastore.ErrNotFound {
			return address.Undef, xerrors.Errorf("Could not find wait msg cid %s", mcid)
		}
		return address.Undef, err
	}

	chanAccessor, err := pm.accessorByFromTo(ci.Control, ci.Target)
	if err != nil {
		return address.Undef, err
	}

	return chanAccessor.getPaychWaitReady(ctx, mcid)
}

func (pm *Manager) ListChannels() ([]address.Address, error) {
	// Need to take an exclusive lock here so that channel operations can't run
	// in parallel (see channelLock)
	pm.lk.Lock()
	defer pm.lk.Unlock()

	return pm.store.ListChannels()
}

func (pm *Manager) GetChannelInfo(addr address.Address) (*ChannelInfo, error) {
	ca, err := pm.accessorByAddress(addr)
	if err != nil {
		return nil, err
	}
	return ca.getChannelInfo(addr)
}

// CheckVoucherValid checks if the given voucher is valid (is or could become spendable at some point).
// If the channel is not in the store, fetches the channel from state (and checks that
// the channel To address is owned by the wallet).
func (pm *Manager) CheckVoucherValid(ctx context.Context, ch address.Address, sv *paych.SignedVoucher) error {
	// Get an accessor for the channel, creating it from state if necessary
	ca, err := pm.inboundChannelAccessor(ctx, ch)
	if err != nil {
		return err
	}

	_, err = ca.checkVoucherValid(ctx, ch, sv)
	return err
}

// CheckVoucherSpendable checks if the given voucher is currently spendable
func (pm *Manager) CheckVoucherSpendable(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	ca, err := pm.accessorByAddress(ch)
	if err != nil {
		return false, err
	}

	return ca.checkVoucherSpendable(ctx, ch, sv, secret, proof)
}

// AddVoucherOutbound adds a voucher for an outbound channel.
// Returns an error if the channel is not already in the store.
func (pm *Manager) AddVoucherOutbound(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	ca, err := pm.accessorByAddress(ch)
	if err != nil {
		return types.NewInt(0), err
	}
	return ca.addVoucher(ctx, ch, sv, proof, minDelta)
}

// AddVoucherInbound adds a voucher for an inbound channel.
// If the channel is not in the store, fetches the channel from state (and checks that
// the channel To address is owned by the wallet).
func (pm *Manager) AddVoucherInbound(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	// Get an accessor for the channel, creating it from state if necessary
	ca, err := pm.inboundChannelAccessor(ctx, ch)
	if err != nil {
		return types.BigInt{}, err
	}
	return ca.addVoucher(ctx, ch, sv, proof, minDelta)
}

// inboundChannelAccessor gets an accessor for the given channel. The channel
// must either exist in the store, or be an inbound channel that can be created
// from state.
func (pm *Manager) inboundChannelAccessor(ctx context.Context, ch address.Address) (*channelAccessor, error) {
	// Make sure channel is in store, or can be fetched from state, and that
	// the channel To address is owned by the wallet
	ci, err := pm.trackInboundChannel(ctx, ch)
	if err != nil {
		return nil, err
	}

	// This is an inbound channel, so To is the Control address (this node)
	from := ci.Target
	to := ci.Control
	return pm.accessorByFromTo(from, to)
}

func (pm *Manager) trackInboundChannel(ctx context.Context, ch address.Address) (*ChannelInfo, error) {
	// Need to take an exclusive lock here so that channel operations can't run
	// in parallel (see channelLock)
	pm.lk.Lock()
	defer pm.lk.Unlock()

	// Check if channel is in store
	ci, err := pm.store.ByAddress(ch)
	if err == nil {
		// Channel is in store, so it's already being tracked
		return ci, nil
	}

	// If there's an error (besides channel not in store) return err
	if err != ErrChannelNotTracked {
		return nil, err
	}

	// Channel is not in store, so get channel from state
	stateCi, err := pm.sa.loadStateChannelInfo(ctx, ch, DirInbound)
	if err != nil {
		return nil, err
	}

	// Check that channel To address is in wallet
	to := stateCi.Control // Inbound channel so To addr is Control (this node)
	toKey, err := pm.pchapi.StateAccountKey(ctx, to, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	has, err := pm.pchapi.WalletHas(ctx, toKey)
	if err != nil {
		return nil, err
	}
	if !has {
		msg := "cannot add voucher for channel %s: wallet does not have key for address %s"
		return nil, xerrors.Errorf(msg, ch, to)
	}

	// Save channel to store
	return pm.store.TrackChannel(stateCi)
}

func (pm *Manager) SubmitVoucher(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (cid.Cid, error) {
	ca, err := pm.accessorByAddress(ch)
	if err != nil {
		return cid.Undef, err
	}
	return ca.submitVoucher(ctx, ch, sv, secret, proof)
}

func (pm *Manager) AllocateLane(ch address.Address) (uint64, error) {
	ca, err := pm.accessorByAddress(ch)
	if err != nil {
		return 0, err
	}
	return ca.allocateLane(ch)
}

func (pm *Manager) ListVouchers(ctx context.Context, ch address.Address) ([]*VoucherInfo, error) {
	ca, err := pm.accessorByAddress(ch)
	if err != nil {
		return nil, err
	}
	return ca.listVouchers(ctx, ch)
}

func (pm *Manager) NextNonceForLane(ctx context.Context, ch address.Address, lane uint64) (uint64, error) {
	ca, err := pm.accessorByAddress(ch)
	if err != nil {
		return 0, err
	}
	return ca.nextNonceForLane(ctx, ch, lane)
}

func (pm *Manager) Settle(ctx context.Context, addr address.Address) (cid.Cid, error) {
	ca, err := pm.accessorByAddress(addr)
	if err != nil {
		return cid.Undef, err
	}
	return ca.settle(ctx, addr)
}

func (pm *Manager) Collect(ctx context.Context, addr address.Address) (cid.Cid, error) {
	ca, err := pm.accessorByAddress(addr)
	if err != nil {
		return cid.Undef, err
	}
	return ca.collect(ctx, addr)
}

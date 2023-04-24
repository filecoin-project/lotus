package migrations

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	versionedfsm "github.com/filecoin-project/go-ds-versioning/pkg/fsm"
	"github.com/filecoin-project/go-ds-versioning/pkg/versioned"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations/maptypes"
)

func NewActorAddr(t testing.TB, data string) address.Address {
	ret, err := address.NewActorAddress([]byte(data))
	require.NoError(t, err)
	return ret
}

func TestClientStateMigration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a v0 client deal state
	dealID := retrievalmarket.DealID(1)
	storeID := uint64(1)
	dummyCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)
	dealState := maptypes.ClientDealState1{
		DealProposal: retrievalmarket.DealProposal{
			PayloadCID: dummyCid,
			ID:         dealID,
			Params: retrievalmarket.Params{
				PieceCID:     &dummyCid,
				PricePerByte: abi.NewTokenAmount(0),
				UnsealPrice:  abi.NewTokenAmount(0),
			},
		},
		TotalFunds:       abi.NewTokenAmount(0),
		ClientWallet:     NewActorAddr(t, "client"),
		MinerWallet:      NewActorAddr(t, "miner"),
		TotalReceived:    0,
		CurrentInterval:  10,
		BytesPaidFor:     0,
		PaymentRequested: abi.NewTokenAmount(0),
		FundsSpent:       abi.NewTokenAmount(0),
		Status:           retrievalmarket.DealStatusNew,
		Sender:           peer.ID("sender"),
		UnsealFundsPaid:  big.Zero(),
		StoreID:          &storeID,
	}
	dealStateWithChannelID := dealState
	chid := datatransfer.ChannelID{
		Initiator: "initiator",
		Responder: "responder",
		ID:        1,
	}
	dealStateWithChannelID.ChannelID = chid

	testCases := []struct {
		name         string
		dealState1   *maptypes.ClientDealState1
		expChannelID *datatransfer.ChannelID
	}{{
		name:         "from v0 - v2 with channel ID",
		dealState1:   &dealState,
		expChannelID: nil,
	}, {
		name:         "from v0 - v2 with no channel ID",
		dealState1:   &dealStateWithChannelID,
		expChannelID: &chid,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds := dss.MutexWrap(datastore.NewMapDatastore())
			emptyList, err := versioned.BuilderList{}.Build()
			require.NoError(t, err)
			stateMachines, migrateStateMachines, err := versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
				Environment:     &mockClientEnv{},
				StateType:       maptypes.ClientDealState1{},
				StateKeyField:   "Status",
				Events:          fsm.Events{},
				StateEntryFuncs: fsm.StateEntryFuncs{},
				FinalityStates:  []fsm.StateKey{},
			}, emptyList, "1")
			require.NoError(t, err)

			// Run migration to v1 datastore
			err = migrateStateMachines(ctx)
			require.NoError(t, err)

			err = stateMachines.Begin(dealID, tc.dealState1)
			require.NoError(t, err)

			// Prepare to run migration to v2 datastore
			retrievalMigrations, err := ClientMigrations.Build()
			require.NoError(t, err)

			stateMachines, migrateStateMachines, err = versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
				Environment:     &mockClientEnv{},
				StateType:       retrievalmarket.ClientDealState{},
				StateKeyField:   "Status",
				Events:          clientstates.ClientEvents,
				StateEntryFuncs: clientstates.ClientStateEntryFuncs,
				FinalityStates:  clientstates.ClientFinalityStates,
			}, retrievalMigrations, "2")
			require.NoError(t, err)

			// Run migration to v2 datastore
			err = migrateStateMachines(ctx)
			require.NoError(t, err)

			var states []retrievalmarket.ClientDealState
			err = stateMachines.List(&states)
			require.NoError(t, err)

			require.Len(t, states, 1)
			if tc.expChannelID == nil {
				// Ensure that the channel ID is nil if it was not explicitly defined
				require.Nil(t, states[0].ChannelID)
			} else {
				// Ensure that the channel ID is correct if it was defined
				require.Equal(t, chid, *states[0].ChannelID)
			}
		})
	}
}

func TestProviderStateMigration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a v0 provider deal state
	dealID := retrievalmarket.DealID(1)
	storeID := uint64(1)
	dummyCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)
	dealState := maptypes.ProviderDealState1{
		DealProposal: retrievalmarket.DealProposal{
			PayloadCID: dummyCid,
			ID:         dealID,
			Params: retrievalmarket.Params{
				PieceCID:     &dummyCid,
				PricePerByte: abi.NewTokenAmount(0),
				UnsealPrice:  abi.NewTokenAmount(0),
			},
		},
		StoreID: storeID,
		PieceInfo: &piecestore.PieceInfo{
			PieceCID: dummyCid,
			Deals:    nil,
		},
		Status:          retrievalmarket.DealStatusNew,
		Receiver:        peer.ID("receiver"),
		TotalSent:       0,
		FundsReceived:   abi.NewTokenAmount(0),
		Message:         "hello",
		CurrentInterval: 10,
	}
	dealStateWithChannelID := dealState
	chid := datatransfer.ChannelID{
		Initiator: "initiator",
		Responder: "responder",
		ID:        1,
	}
	dealStateWithChannelID.ChannelID = chid

	testCases := []struct {
		name         string
		dealState0   *maptypes.ProviderDealState1
		expChannelID *datatransfer.ChannelID
	}{{
		name:         "from v0 - v2 with channel ID",
		dealState0:   &dealState,
		expChannelID: nil,
	}, {
		name:         "from v0 - v2 with no channel ID",
		dealState0:   &dealStateWithChannelID,
		expChannelID: &chid,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds := dss.MutexWrap(datastore.NewMapDatastore())
			emptyList, err := versioned.BuilderList{}.Build()
			require.NoError(t, err)
			stateMachines, migrateStateMachines, err := versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
				Environment:     &mockProviderEnv{},
				StateType:       maptypes.ProviderDealState1{},
				StateKeyField:   "Status",
				Events:          fsm.Events{},
				StateEntryFuncs: fsm.StateEntryFuncs{},
				FinalityStates:  []fsm.StateKey{},
			}, emptyList, "1")
			require.NoError(t, err)

			// Run migration to v1 datastore
			err = migrateStateMachines(ctx)
			require.NoError(t, err)

			err = stateMachines.Begin(dealID, tc.dealState0)
			require.NoError(t, err)

			// Prepare to run migration to v2 datastore
			retrievalMigrations, err := ProviderMigrations.Build()
			require.NoError(t, err)

			stateMachines, migrateStateMachines, err = versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
				Environment:     &mockProviderEnv{},
				StateType:       retrievalmarket.ProviderDealState{},
				StateKeyField:   "Status",
				Events:          providerstates.ProviderEvents,
				StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
				FinalityStates:  providerstates.ProviderFinalityStates,
			}, retrievalMigrations, "2")
			require.NoError(t, err)

			// Run migration to v2 datastore
			err = migrateStateMachines(ctx)
			require.NoError(t, err)

			var states []retrievalmarket.ProviderDealState
			err = stateMachines.List(&states)
			require.NoError(t, err)

			require.Len(t, states, 1)
			if tc.expChannelID == nil {
				// Ensure that the channel ID is nil if it was not explicitly defined
				require.Nil(t, states[0].ChannelID)
			} else {
				// Ensure that the channel ID is correct if it was defined
				require.Equal(t, chid, *states[0].ChannelID)
			}
		})
	}
}

type mockClientEnv struct {
}

func (e *mockClientEnv) Node() retrievalmarket.RetrievalClientNode {
	return nil
}

func (e *mockClientEnv) OpenDataTransfer(ctx context.Context, to peer.ID, proposal *retrievalmarket.DealProposal) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, nil
}

func (e *mockClientEnv) SendDataTransferVoucher(_ context.Context, _ datatransfer.ChannelID, _ *retrievalmarket.DealPayment) error {
	return nil
}

func (e *mockClientEnv) CloseDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return nil
}

func (e *mockClientEnv) FinalizeBlockstore(ctx context.Context, id retrievalmarket.DealID) error {
	return nil
}

var _ clientstates.ClientDealEnvironment = &mockClientEnv{}

type mockProviderEnv struct {
}

func (te *mockProviderEnv) PrepareBlockstore(ctx context.Context, dealID retrievalmarket.DealID, pieceCid cid.Cid) error {
	return nil
}

func (te *mockProviderEnv) Node() retrievalmarket.RetrievalProviderNode {
	return nil
}

func (te *mockProviderEnv) DeleteStore(dealID retrievalmarket.DealID) error {
	return nil
}
func (te *mockProviderEnv) ResumeDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return nil
}

func (te *mockProviderEnv) CloseDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return nil
}

func (te *mockProviderEnv) ChannelState(_ context.Context, _ datatransfer.ChannelID) (datatransfer.ChannelState, error) {
	return nil, nil
}

func (te *mockProviderEnv) UpdateValidationStatus(_ context.Context, _ datatransfer.ChannelID, _ datatransfer.ValidationResult) error {
	return nil
}

var _ providerstates.ProviderDealEnvironment = &mockProviderEnv{}

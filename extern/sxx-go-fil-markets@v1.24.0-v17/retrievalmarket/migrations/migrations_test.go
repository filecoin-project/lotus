package migrations

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	versionedfsm "github.com/filecoin-project/go-ds-versioning/pkg/fsm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/piecestore/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
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
	dealState := ClientDealState0{
		DealProposal0: DealProposal0{
			PayloadCID: dummyCid,
			ID:         dealID,
			Params0: Params0{
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
		dealState0   *ClientDealState0
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

			// Store the v0 client deal state to the datastore
			stateMachines0, err := fsm.New(ds, fsm.Parameters{
				Environment:     &mockClientEnv{},
				StateType:       ClientDealState0{},
				StateKeyField:   "Status",
				Events:          fsm.Events{},
				StateEntryFuncs: fsm.StateEntryFuncs{},
				FinalityStates:  []fsm.StateKey{},
			})
			require.NoError(t, err)

			err = stateMachines0.Begin(dealID, tc.dealState0)
			require.NoError(t, err)

			// Prepare to run migration to v2 datastore
			retrievalMigrations, err := ClientMigrations.Build()
			require.NoError(t, err)

			stateMachines, migrateStateMachines, err := versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
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
	dealState := ProviderDealState0{
		DealProposal0: DealProposal0{
			PayloadCID: dummyCid,
			ID:         dealID,
			Params0: Params0{
				PieceCID:     &dummyCid,
				PricePerByte: abi.NewTokenAmount(0),
				UnsealPrice:  abi.NewTokenAmount(0),
			},
		},
		StoreID: storeID,
		PieceInfo: &migrations.PieceInfo0{
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
		dealState0   *ProviderDealState0
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

			// Store the v0 provider deal state to the datastore
			stateMachines0, err := fsm.New(ds, fsm.Parameters{
				Environment:     &mockProviderEnv{},
				StateType:       ProviderDealState0{},
				StateKeyField:   "Status",
				Events:          fsm.Events{},
				StateEntryFuncs: fsm.StateEntryFuncs{},
				FinalityStates:  []fsm.StateKey{},
			})
			require.NoError(t, err)

			err = stateMachines0.Begin(dealID, tc.dealState0)
			require.NoError(t, err)

			// Prepare to run migration to v2 datastore
			retrievalMigrations, err := ProviderMigrations.Build()
			require.NoError(t, err)

			stateMachines, migrateStateMachines, err := versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
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

func (e *mockClientEnv) OpenDataTransfer(ctx context.Context, to peer.ID, proposal *retrievalmarket.DealProposal, legacy bool) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, nil
}

func (e *mockClientEnv) SendDataTransferVoucher(_ context.Context, _ datatransfer.ChannelID, _ *retrievalmarket.DealPayment, _ bool) error {
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

func (te *mockProviderEnv) TrackTransfer(deal retrievalmarket.ProviderDealState) error {
	return nil
}

func (te *mockProviderEnv) UntrackTransfer(deal retrievalmarket.ProviderDealState) error {
	return nil
}

func (te *mockProviderEnv) ResumeDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return nil
}

func (te *mockProviderEnv) CloseDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return nil
}

var _ providerstates.ProviderDealEnvironment = &mockProviderEnv{}

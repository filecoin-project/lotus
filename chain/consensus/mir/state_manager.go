package mir

import (
	"bytes"
	"fmt"

	"github.com/multiformats/go-multiaddr"

	availabilityevents "github.com/filecoin-project/mir/pkg/availability/events"
	"github.com/filecoin-project/mir/pkg/events"
	"github.com/filecoin-project/mir/pkg/modules"
	"github.com/filecoin-project/mir/pkg/pb/availabilitypb"
	"github.com/filecoin-project/mir/pkg/pb/commonpb"
	"github.com/filecoin-project/mir/pkg/pb/contextstorepb"
	"github.com/filecoin-project/mir/pkg/pb/eventpb"
	"github.com/filecoin-project/mir/pkg/pb/requestpb"
	t "github.com/filecoin-project/mir/pkg/types"
	"github.com/filecoin-project/mir/pkg/util/maputil"
)

const (
	availabilityModuleID = t.ModuleID("availability")
)

type Message []byte

type Batch struct {
	Messages   []Message
	Validators []t.NodeID
}

type StateManager struct {

	// The current epoch number.
	currentEpoch t.EpochNr

	// For each epoch number, stores the corresponding membership.
	memberships map[t.EpochNr]map[t.NodeID]t.NodeAddress

	// Channel to send batches to Eudico.
	NextBatch chan *Batch

	// Channel to send a membership.
	NewMembership chan map[t.NodeID]t.NodeAddress

	MirManager *Manager

	// TODO: The vote counting is leaking memory. Resolve that in garbage collection mechanism.
	reconfigurationVotes map[string]int
}

func NewStateManager(initialMembership map[t.NodeID]t.NodeAddress, m *Manager) *StateManager {
	// Initialize the membership for the first epochs.
	// We use configOffset+2 memberships to account for:
	// - The first epoch (epoch 0)
	// - The configOffset epochs that already have a fixed membership (epochs 1 to configOffset)
	// - The membership of the following epoch (configOffset+1) initialized with the same membership,
	//   but potentially replaced during the first epoch (epoch 0) through special configuration requests.
	memberships := make(map[t.EpochNr]map[t.NodeID]t.NodeAddress, ConfigOffset+2)
	for e := 0; e < ConfigOffset+2; e++ {
		memberships[t.EpochNr(e)] = initialMembership
	}

	sm := StateManager{
		NextBatch:            make(chan *Batch),
		NewMembership:        make(chan map[t.NodeID]t.NodeAddress, 1),
		MirManager:           m,
		memberships:          memberships,
		currentEpoch:         0,
		reconfigurationVotes: make(map[string]int),
	}
	return &sm
}

func (sm *StateManager) ApplyEvents(eventsIn *events.EventList) (*events.EventList, error) {
	return modules.ApplyEventsSequentially(eventsIn, sm.ApplyEvent)
}

func (sm *StateManager) ApplyEvent(event *eventpb.Event) (*events.EventList, error) {
	switch e := event.Type.(type) {
	case *eventpb.Event_Init:
		return events.EmptyList(), nil
	case *eventpb.Event_NewEpoch:
		return sm.applyNewEpoch(e.NewEpoch)
	case *eventpb.Event_DeliverCert:
		return sm.applyDeliverCertificate(e.DeliverCert)
	case *eventpb.Event_Availability:
		switch e := e.Availability.Type.(type) {
		case *availabilitypb.Event_ProvideTransactions:
			return sm.applyProvideTransactions(e.ProvideTransactions)
		default:
			return nil, fmt.Errorf("unexpected availability event type: %T", e)
		}
	case *eventpb.Event_AppSnapshotRequest:
		return sm.applySnapshotRequest(e.AppSnapshotRequest)
	case *eventpb.Event_AppRestoreState:
		return sm.applyRestoreState(e.AppRestoreState.Snapshot)
	default:
		return nil, fmt.Errorf("unexpected type of App event: %T", event.Type)
	}
}

// applyDeliver applies a delivered availability certificate.
func (sm *StateManager) applyDeliverCertificate(deliver *eventpb.DeliverCert) (*events.EventList, error) {

	// Skip padding certificates. Deliver events with nil certificates are considered noops.
	if deliver.Cert.Type == nil {
		return events.EmptyList(), nil
	}

	switch c := deliver.Cert.Type.(type) {
	case *availabilitypb.Cert_Msc:
		// If the certificate was produced by the multisig collector

		// Ignore empty batch availability certificates.
		if len(c.Msc.BatchId) == 0 {
			return events.EmptyList(), nil
		}

		// Request transaction payloads that the received certificate refers to
		// from the appropriate instance (there is one per epoch) of the availability layer,
		// which should respond with a ProvideTransactions event.
		return events.ListOf(availabilityevents.RequestTransactions(
			availabilityModuleID.Then(t.ModuleID(fmt.Sprintf("%v", sm.currentEpoch))),
			deliver.Cert,
			&availabilitypb.RequestTransactionsOrigin{
				Module: "app",
				Type: &availabilitypb.RequestTransactionsOrigin_ContextStore{
					ContextStore: &contextstorepb.Origin{ItemID: 0},
				},
			},
		)), nil
	default:
		return nil, fmt.Errorf("unknown availability certificate type: %T", deliver.Cert.Type)
	}
}

// applyProvideTransactions applies transactions received from the availability layer to the app state.
// In our case, it simply extends the message history
// by appending the payload of each received request as a new message.
// Each appended message is also printed to stdout.
// Special messages starting with `Config: ` are recognized, parsed, and treated accordingly.
func (sm *StateManager) applyProvideTransactions(ptx *availabilitypb.ProvideTransactions) (*events.EventList, error) {
	var msgs []Message

	// For each request in the batch
	for _, req := range ptx.Txs {
		switch req.Type {
		case TransportType:
			msgs = append(msgs, req.Data)
		case ReconfigurationType:
			err := sm.applyConfigMsg(req)
			if err != nil {
				return events.EmptyList(), err
			}
		}
	}

	// Send a batch to the Eudico node.
	sm.NextBatch <- &Batch{
		Messages:   msgs,
		Validators: maputil.GetSortedKeys(sm.memberships[sm.currentEpoch]),
	}

	return events.EmptyList(), nil
}

func (sm *StateManager) applyConfigMsg(in *requestpb.Request) error {
	newValSet := &ValidatorSet{}
	if err := newValSet.UnmarshalCBOR(bytes.NewReader(in.Data)); err != nil {
		return err
	}
	voted, err := sm.UpdateAndCheckVotes(newValSet)
	if err != nil {
		return err
	}
	if voted {
		err = sm.UpdateNextMembership(newValSet)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sm *StateManager) applyNewEpoch(newEpoch *eventpb.NewEpoch) (*events.EventList, error) {
	// Sanity check.
	if t.EpochNr(newEpoch.EpochNr) != sm.currentEpoch+1 {
		return nil, fmt.Errorf("expected next epoch to be %d, got %d", sm.currentEpoch+1, newEpoch.EpochNr)
	}

	// The base membership is the last one membership.
	newMembership := maputil.Copy(sm.memberships[t.EpochNr(newEpoch.EpochNr+ConfigOffset)])

	// Append a new membership data structure to be modified throughout the new epoch.
	sm.memberships[t.EpochNr(newEpoch.EpochNr+ConfigOffset+1)] = newMembership

	// Update current epoch number.
	oldEpoch := sm.currentEpoch
	sm.currentEpoch = t.EpochNr(newEpoch.EpochNr)

	// Remove old membership.
	delete(sm.memberships, oldEpoch)

	sm.NewMembership <- newMembership

	// Notify ISS about the new membership.
	return events.ListOf(events.NewConfig("iss", newMembership)), nil
}

func (sm *StateManager) UpdateNextMembership(valSet *ValidatorSet) error {
	_, mbs, err := validatorsMembership(valSet.GetValidators())
	if err != nil {
		return err
	}
	sm.memberships[sm.currentEpoch+ConfigOffset+1] = mbs
	return nil
}

// UpdateAndCheckVotes votes for the valSet and returns true if it has had enough votes for this valSet.
func (sm *StateManager) UpdateAndCheckVotes(valSet *ValidatorSet) (bool, error) {
	h, err := valSet.Hash()
	if err != nil {
		return false, err
	}
	sm.reconfigurationVotes[string(h)]++
	votes := sm.reconfigurationVotes[string(h)]
	nodes := len(sm.memberships[sm.currentEpoch])
	if votes < weakQuorum(nodes) {
		return false, nil
	}
	return true, nil
}

// applySnapshotRequest produces a StateSnapshotResponse event containing the current snapshot of the state.
// The snapshot is a binary representation of the application state that can be passed to applyRestoreState().
func (sm *StateManager) applySnapshotRequest(snapshotRequest *eventpb.AppSnapshotRequest) (*events.EventList, error) {
	return events.ListOf(events.AppSnapshotResponse(
		t.ModuleID(snapshotRequest.Module),
		nil,
		snapshotRequest.Origin,
	)), nil
}

// applyRestoreState restores the application's state to the one represented by the passed argument.
// The argument is a binary representation of the application state returned from Snapshot().
func (sm *StateManager) applyRestoreState(snapshot *commonpb.StateSnapshot) (*events.EventList, error) {
	sm.currentEpoch = t.EpochNr(snapshot.Configuration.EpochNr)
	sm.memberships = make(map[t.EpochNr]map[t.NodeID]t.NodeAddress, len(snapshot.Configuration.Memberships))

	for e, membership := range snapshot.Configuration.Memberships {
		sm.memberships[t.EpochNr(e)] = make(map[t.NodeID]t.NodeAddress)
		for nID, nAddr := range membership.Membership {
			var err error
			sm.memberships[t.EpochNr(e)][t.NodeID(nID)], err = multiaddr.NewMultiaddr(nAddr)
			if err != nil {
				return nil, err
			}
		}
	}

	newMembership := maputil.Copy(sm.memberships[t.EpochNr(snapshot.Configuration.EpochNr+ConfigOffset)])
	sm.memberships[t.EpochNr(snapshot.Configuration.EpochNr+ConfigOffset+1)] = newMembership

	return events.EmptyList(), nil
}

// ImplementsModule method only serves the purpose of indicating that this is a Module and must not be called.
func (sm *StateManager) ImplementsModule() {}

func maxFaulty(n int) int {
	// assuming n > 3f:
	//   return max f
	return (n - 1) / 3
}

func weakQuorum(n int) int {
	// assuming n > 3f:
	//   return min q: q > f
	return maxFaulty(n) + 1
}

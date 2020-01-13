package statemachine

import (
	"context"
	"reflect"
	"sync"

	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"
)

type StateHandler interface {
	// returns
	Plan(events []Event, user interface{}) (interface{}, error)
}

// StateGroup manages a group of state machines sharing the same logic
type StateGroup struct {
	sts       *statestore.StateStore
	hnd       StateHandler
	stateType reflect.Type

	lk  sync.Mutex
	sms map[datastore.Key]*StateMachine
}

// stateType: T - (reflect.TypeOf(MyStateStruct{}))
func New(ds datastore.Datastore, hnd StateHandler, stateType reflect.Type) *StateGroup {
	return &StateGroup{
		sts:       statestore.New(ds),
		hnd:       hnd,
		stateType: stateType,

		sms: map[datastore.Key]*StateMachine{},
	}
}

// Send sends an event to machine identified by `id`.
// `evt` is going to be passed into StateHandler.Planner, in the events[].User param
//
// If a state machine with the specified id doesn't exits, it's created, and it's
// state is set to zero-value of stateType provided in group constructor
func (s *StateGroup) Send(id interface{}, evt interface{}) (err error) {
	s.lk.Lock()
	defer s.lk.Unlock()

	sm, exist := s.sms[statestore.ToKey(id)]
	if !exist {
		sm, err = s.loadOrCreate(id)
		if err != nil {
			return xerrors.Errorf("loadOrCreate state: %w", err)
		}
		s.sms[statestore.ToKey(id)] = sm
	}

	return sm.send(Event{User: evt})
}

func (s *StateGroup) loadOrCreate(name interface{}) (*StateMachine, error) {
	exists, err := s.sts.Has(name)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if state for %v exists: %w", name, err)
	}

	if !exists {
		userState := reflect.New(s.stateType).Interface()

		err = s.sts.Begin(name, userState)
		if err != nil {
			return nil, xerrors.Errorf("saving initial state: %w", err)
		}
	}

	res := &StateMachine{
		planner:  s.hnd.Plan,
		eventsIn: make(chan Event),

		name:      name,
		st:        s.sts.Get(name),
		stateType: s.stateType,

		stageDone: make(chan struct{}),
		closing:   make(chan struct{}),
		closed:    make(chan struct{}),
	}

	go res.run()

	return res, nil
}

// Stop stops all state machines in this group
func (s *StateGroup) Stop(ctx context.Context) error {
	s.lk.Lock()
	defer s.lk.Unlock()

	for _, sm := range s.sms {
		if err := sm.stop(ctx); err != nil {
			return err
		}
	}

	return nil
}

// List outputs states of all state machines in this group
// out: *[]StateT
func (s *StateGroup) List(out interface{}) error {
	return s.sts.List(out)
}

// Get gets state for a single state machine
func (s *StateGroup) Get(id interface{}) *statestore.StoredState {
	return s.sts.Get(id)
}

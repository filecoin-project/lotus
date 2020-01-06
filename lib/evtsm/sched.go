package evtsm

import (
	"context"
	"reflect"
	"sync"

	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/statestore"
)

type StateHandler interface {
	// returns
	Plan(events []Event, user interface{}) (interface{}, error)
}

type Sched struct {
	sts       *statestore.StateStore
	hnd       StateHandler
	stateType reflect.Type

	lk  sync.Mutex
	sms map[datastore.Key]*ESm
}

// stateType: T - (reflect.TypeOf(MyStateStruct{}))
func New(ds datastore.Datastore, hnd StateHandler, stateType reflect.Type) *Sched {
	return &Sched{
		sts:       statestore.New(ds),
		hnd:       hnd,
		stateType: stateType,

		sms: map[datastore.Key]*ESm{},
	}
}

func (s *Sched) Send(to interface{}, evt interface{}) (err error) {
	s.lk.Lock()
	defer s.lk.Unlock()

	sm, exist := s.sms[statestore.ToKey(to)]
	if !exist {
		sm, err = s.loadOrCreate(to)
		if err != nil {
			return xerrors.Errorf("loadOrCreate state: %w", err)
		}
		s.sms[statestore.ToKey(to)] = sm
	}

	return sm.send(Event{User: evt})
}

func (s *Sched) loadOrCreate(name interface{}) (*ESm, error) {
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

	res := &ESm{
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

func (s *Sched) Stop(ctx context.Context) error {
	s.lk.Lock()
	defer s.lk.Unlock()

	for _, sm := range s.sms {
		if err := sm.stop(ctx); err != nil {
			return err
		}
	}

	return nil
}

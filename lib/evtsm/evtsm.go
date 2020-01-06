package evtsm

import (
	"context"
	"reflect"
	"sync/atomic"

	"github.com/filecoin-project/lotus/lib/statestore"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("evtsm")

// returns func(ctx Context, st <T>) (func(*<T>), error), where <T> is the typeOf(User) param
type Planner func(events []Event, user interface{}) (interface{}, error)

type ESm struct {
	planner  Planner
	eventsIn chan Event

	name      interface{}
	st        *statestore.StoredState
	stateType reflect.Type

	stageDone chan struct{}
	closing   chan struct{}
	closed    chan struct{}

	busy int32
}

func (fsm *ESm) run() {
	defer close(fsm.closed)

	var pendingEvents []Event

	for {
		// NOTE: This requires at least one event to be sent to trigger a stage
		//  This means that after restarting the state machine users of this
		//  code must send a 'restart' event
		select {
		case evt := <-fsm.eventsIn:
			pendingEvents = append(pendingEvents, evt)
		case <-fsm.stageDone:
			if len(pendingEvents) == 0 {
				continue
			}
		case <-fsm.closing:
			return
		}

		if atomic.CompareAndSwapInt32(&fsm.busy, 0, 1) {
			var nextStep interface{}
			var ustate interface{}

			err := fsm.mutateUser(func(user interface{}) (err error) {
				nextStep, err = fsm.planner(pendingEvents, user)
				ustate = user
				return err
			})
			if err != nil {
				log.Errorf("Executing event planner failed: %+v", err)
				return
			}

			pendingEvents = nil

			if nextStep == nil {
				continue
			}

			ctx := Context{
				ctx: context.TODO(),
				send: func(evt interface{}) error {
					return fsm.send(Event{User: evt})
				},
			}

			go func() {
				res := reflect.ValueOf(nextStep).Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(ustate).Elem()})

				if res[0].Interface() != nil {
					log.Errorf("executing step: %+v", res[0].Interface().(error)) // TODO: propagate top level
					return
				}

				atomic.StoreInt32(&fsm.busy, 0)
				fsm.stageDone <- struct{}{}
			}()

		}
	}
}

func (fsm *ESm) mutateUser(cb func(user interface{}) error) error {
	mutt := reflect.FuncOf([]reflect.Type{reflect.PtrTo(fsm.stateType)}, []reflect.Type{reflect.TypeOf(new(error)).Elem()}, false)

	mutf := reflect.MakeFunc(mutt, func(args []reflect.Value) (results []reflect.Value) {
		err := cb(args[0].Interface())
		return []reflect.Value{reflect.ValueOf(&err).Elem()}
	})

	return fsm.st.Mutate(mutf.Interface())
}

func (fsm *ESm) send(evt Event) error {
	fsm.eventsIn <- evt // TODO: ctx, at least
	return nil
}

func (fsm *ESm) stop(ctx context.Context) error {
	close(fsm.closing)

	select {
	case <-fsm.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

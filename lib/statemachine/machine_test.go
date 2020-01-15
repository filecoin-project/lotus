package statemachine

import (
	"context"
	"testing"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"gotest.tools/assert"
)

func init() {
	logging.SetLogLevel("*", "INFO")
}

type testHandler struct {
	t       *testing.T
	proceed chan struct{}
	done    chan struct{}
}

func (t *testHandler) Plan(events []Event, state interface{}) (interface{}, error) {
	return t.plan(events, state.(*TestState))
}

func (t *testHandler) plan(events []Event, state *TestState) (func(Context, TestState) error, error) {
	for _, event := range events {
		e := event.User.(*TestEvent)
		switch e.A {
		case "restart":
		case "start":
			state.A = 1
		case "b":
			state.A = 2
			state.B = e.Val
		}
	}

	switch state.A {
	case 1:
		return t.step0, nil
	case 2:
		return t.step1, nil
	default:
		t.t.Fatal(state.A)
	}
	panic("how?")
}

func (t *testHandler) step0(ctx Context, st TestState) error {
	ctx.Send(&TestEvent{A: "b", Val: 55})
	<-t.proceed
	return nil
}

func (t *testHandler) step1(ctx Context, st TestState) error {
	assert.Equal(t.t, uint64(2), st.A)

	close(t.done)
	return nil
}

func TestBasic(t *testing.T) {
	for i := 0; i < 1000; i++ { // run a few times to expose any races
		ds := datastore.NewMapDatastore()

		th := &testHandler{t: t, done: make(chan struct{}), proceed: make(chan struct{})}
		close(th.proceed)
		smm := New(ds, th, TestState{})

		if err := smm.Send(uint64(2), &TestEvent{A: "start"}); err != nil {
			t.Fatalf("%+v", err)
		}

		<-th.done
	}
}

func TestPersist(t *testing.T) {
	for i := 0; i < 1000; i++ { // run a few times to expose any races
		ds := datastore.NewMapDatastore()

		th := &testHandler{t: t, done: make(chan struct{}), proceed: make(chan struct{})}
		smm := New(ds, th, TestState{})

		if err := smm.Send(uint64(2), &TestEvent{A: "start"}); err != nil {
			t.Fatalf("%+v", err)
		}

		if err := smm.Stop(context.Background()); err != nil {
			t.Fatal(err)
			return
		}

		smm = New(ds, th, TestState{})
		if err := smm.Send(uint64(2), &TestEvent{A: "restart"}); err != nil {
			t.Fatalf("%+v", err)
		}
		close(th.proceed)

		<-th.done
	}
}

var _ StateHandler = &testHandler{}

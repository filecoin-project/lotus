package statemachine

import (
	"fmt"
	"strings"
	"time"
)

const (
	Running   StateType = "running"
	Suspended StateType = "suspended"

	Halt   EventType = "halt"
	Resume EventType = "resume"
)

type Suspendable interface {
	Halt()
	Resume()
}

type HaltAction struct{}

func (a *HaltAction) Execute(ctx EventContext) EventType {
	s, ok := ctx.(*Suspender)
	if !ok {
		fmt.Println("unable to halt, event context is not Suspendable")
		return NoOp
	}
	s.target.Halt()
	return NoOp
}

type ResumeAction struct{}

func (a *ResumeAction) Execute(ctx EventContext) EventType {
	s, ok := ctx.(*Suspender)
	if !ok {
		fmt.Println("unable to resume, event context is not Suspendable")
		return NoOp
	}
	s.target.Resume()
	return NoOp
}

type Suspender struct {
	StateMachine
	target Suspendable
	log    LogFn
}

type LogFn func(fmt string, args ...interface{})

func NewSuspender(target Suspendable, log LogFn) *Suspender {
	return &Suspender{
		target: target,
		log:    log,
		StateMachine: StateMachine{
			Current: Running,
			States: States{
				Running: State{
					Action: &ResumeAction{},
					Events: Events{
						Halt: Suspended,
					},
				},

				Suspended: State{
					Action: &HaltAction{},
					Events: Events{
						Resume: Running,
					},
				},
			},
		},
	}
}

func (s *Suspender) RunEvents(eventSpec string) {
	s.log("running event spec: %s", eventSpec)
	for _, et := range parseEventSpec(eventSpec, s.log) {
		if et.delay != 0 {
			//s.log("waiting %s", et.delay.String())
			time.Sleep(et.delay)
			continue
		}
		if et.event == "" {
			s.log("ignoring empty event")
			continue
		}
		s.log("sending event %s", et.event)
		err := s.SendEvent(et.event, s)
		if err != nil {
			s.log("error sending event %s: %s", et.event, err)
		}
	}
}

type eventTiming struct {
	delay time.Duration
	event EventType
}

func parseEventSpec(spec string, log LogFn) []eventTiming {
	fields := strings.Split(spec, "->")
	out := make([]eventTiming, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		words := strings.Split(f, " ")

		// TODO: try to implement a "waiting" state instead of special casing like this
		if words[0] == "wait" {
			if len(words) != 2 {
				log("expected 'wait' to be followed by duration, e.g. 'wait 30s'. ignoring.")
				continue
			}
			d, err := time.ParseDuration(words[1])
			if err != nil {
				log("bad argument for 'wait': %s", err)
				continue
			}
			out = append(out, eventTiming{delay: d})
		} else {
			out = append(out, eventTiming{event: EventType(words[0])})
		}
	}
	return out
}

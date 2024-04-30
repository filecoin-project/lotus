package alerting

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/journal"
)

var log = logging.Logger("alerting")

// Alerting provides simple stateful alert system. Consumers can register alerts,
// which can be raised and resolved.
//
// When an alert is raised or resolved, a related journal entry is recorded.
type Alerting struct {
	j journal.Journal

	lk     sync.Mutex
	alerts map[AlertType]Alert
}

// AlertType is a unique alert identifier
type AlertType struct {
	System, Subsystem string
}

// AlertEvent contains information about alert state transition
type AlertEvent struct {
	Type    string // either 'raised' or 'resolved'
	Message json.RawMessage
	Time    time.Time
}

type Alert struct {
	Type   AlertType
	Active bool

	LastActive   *AlertEvent // NOTE: pointer for nullability, don't mutate the referenced object!
	LastResolved *AlertEvent

	journalType journal.EventType
}

func NewAlertingSystem(j journal.Journal) *Alerting {
	return &Alerting{
		j: j,

		alerts: map[AlertType]Alert{},
	}
}

func (a *Alerting) AddAlertType(system, subsystem string) AlertType {
	a.lk.Lock()
	defer a.lk.Unlock()

	at := AlertType{
		System:    system,
		Subsystem: subsystem,
	}

	if _, exists := a.alerts[at]; exists {
		return at
	}

	et := a.j.RegisterEventType(system, subsystem)

	a.alerts[at] = Alert{
		Type:        at,
		Active:      false,
		journalType: et,
	}

	return at
}

func (a *Alerting) update(at AlertType, message interface{}, upd func(Alert, json.RawMessage) Alert) {
	a.lk.Lock()
	defer a.lk.Unlock()

	alert, ok := a.alerts[at]
	if !ok {
		log.Errorw("unknown alert", "type", at, "message", message)
	}

	rawMsg, err := json.Marshal(message)
	if err != nil {
		log.Errorw("marshaling alert message failed", "type", at, "error", err)
		rawMsg, err = json.Marshal(&struct {
			AlertError string
		}{
			AlertError: err.Error(),
		})
		log.Errorw("marshaling error failed", "type", at, "error", err)
	}

	a.alerts[at] = upd(alert, rawMsg)
}

// Raise marks the alert condition as active and records related event in the journal
func (a *Alerting) Raise(at AlertType, message interface{}) {
	log.Errorw("alert raised", "type", at, "message", message)

	a.update(at, message, func(alert Alert, rawMsg json.RawMessage) Alert {
		alert.Active = true
		alert.LastActive = &AlertEvent{
			Type:    "raised",
			Message: rawMsg,
			Time:    time.Now(),
		}

		a.j.RecordEvent(alert.journalType, func() interface{} {
			return alert.LastActive
		})

		return alert
	})
}

// Resolve marks the alert condition as resolved and records related event in the journal
func (a *Alerting) Resolve(at AlertType, message interface{}) {
	log.Errorw("alert resolved", "type", at, "message", message)

	a.update(at, message, func(alert Alert, rawMsg json.RawMessage) Alert {
		alert.Active = false
		alert.LastResolved = &AlertEvent{
			Type:    "resolved",
			Message: rawMsg,
			Time:    time.Now(),
		}

		a.j.RecordEvent(alert.journalType, func() interface{} {
			return alert.LastResolved
		})

		return alert
	})
}

// GetAlerts returns all registered (active and inactive) alerts
func (a *Alerting) GetAlerts() []Alert {
	a.lk.Lock()
	defer a.lk.Unlock()

	out := make([]Alert, 0, len(a.alerts))
	for _, alert := range a.alerts {
		out = append(out, alert)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type.System != out[j].Type.System {
			return out[i].Type.System < out[j].Type.System
		}

		return out[i].Type.Subsystem < out[j].Type.Subsystem
	})

	return out
}

func (a *Alerting) IsRaised(at AlertType) bool {
	a.lk.Lock()
	defer a.lk.Unlock()

	return a.alerts[at].Active
}

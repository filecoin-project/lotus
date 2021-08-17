package alerting

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/journal"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("alerting")

type Alerting struct {
	j journal.Journal

	lk     sync.Mutex
	alerts map[AlertType]Alert
}

type AlertType struct {
	System, Subsystem string
}

type AlertEvent struct {
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
		log.Errorw("marshaling marshaling error failed", "type", at, "error", err)
	}

	a.alerts[at] = upd(alert, rawMsg)
}

func (a *Alerting) Raise(at AlertType, message interface{}) {
	log.Errorw("alert raised", "type", at, "message", message)

	a.update(at, message, func(alert Alert, rawMsg json.RawMessage) Alert {
		alert.Active = true
		alert.LastActive = &AlertEvent{
			Message: rawMsg,
			Time:    time.Now(),
		}

		a.j.RecordEvent(alert.journalType, func() interface{} {
			return alert.LastActive
		})

		return alert
	})
}

func (a *Alerting) Resolve(at AlertType, message interface{}) {
	log.Errorw("alert resolved", "type", at, "message", message)

	a.update(at, message, func(alert Alert, rawMsg json.RawMessage) Alert {
		alert.Active = false
		alert.LastResolved = &AlertEvent{
			Message: rawMsg,
			Time:    time.Now(),
		}

		a.j.RecordEvent(alert.journalType, func() interface{} {
			return alert.LastResolved
		})

		return alert
	})
}

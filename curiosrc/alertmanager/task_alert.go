// Nobody associated with this software's development has any business relationship to pagerduty.
// This is provided as a convenient trampoline to SP's alert system of choice.

package alertmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/ctladdr"
)

const AlertMangerInterval = time.Hour

var log = logging.Logger("curio/alertmanager")

type AlertAPI interface {
	ctladdr.NodeApi
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (api.MinerInfo, error)
}

type AlertTask struct {
	api AlertAPI
	cfg config.CurioAlerting
	db  *harmonydb.DB
}

type alertOut struct {
	err         error
	alertString string
}

type alerts struct {
	ctx      context.Context
	api      AlertAPI
	db       *harmonydb.DB
	cfg      config.CurioAlerting
	alertMap map[string]*alertOut
}

type pdPayload struct {
	Summary       string      `json:"summary"`
	Severity      string      `json:"severity"`
	Source        string      `json:"source"`
	Component     string      `json:"component,omitempty"`
	Group         string      `json:"group,omitempty"`
	Class         string      `json:"class,omitempty"`
	CustomDetails interface{} `json:"custom_details,omitempty"`
}

type alertFunc func(al *alerts)

var alertFuncs = []alertFunc{
	balanceCheck,
	taskFailureCheck,
	permanentStorageCheck,
	wdPostCheck,
	wnPostCheck,
}

func NewAlertTask(api AlertAPI, db *harmonydb.DB, alertingCfg config.CurioAlerting) *AlertTask {
	return &AlertTask{
		api: api,
		db:  db,
		cfg: alertingCfg,
	}
}

func (a *AlertTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	if a.cfg.PageDutyIntegrationKey == "" {
		log.Warnf("PageDutyIntegrationKey is empty, not sending an alert")
		return true, nil
	}

	ctx := context.Background()

	alMap := make(map[string]*alertOut)

	altrs := &alerts{
		ctx:      ctx,
		api:      a.api,
		db:       a.db,
		cfg:      a.cfg,
		alertMap: alMap,
	}

	for _, al := range alertFuncs {
		al(altrs)
	}

	details := make(map[string]interface{})

	for k, v := range altrs.alertMap {
		if v != nil {
			if v.err != nil {
				details[k] = v.err.Error()
				continue
			}
			if v.alertString != "" {
				details[k] = v.alertString
			}
		}
	}

	// Alert only if required
	if len(details) > 0 {
		payloadData := &pdPayload{
			Summary:       "Curio Alert",
			Severity:      "critical",
			CustomDetails: details,
			Source:        "Curio Cluster",
		}

		err = a.sendAlert(payloadData)

		if err != nil {
			return false, err
		}
	}

	return true, nil

}

func (a *AlertTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	id := ids[0]
	return &id, nil
}

func (a *AlertTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  1,
		Name: "AlertManager",
		Cost: resources.Resources{
			Cpu: 1,
			Ram: 64 << 20,
			Gpu: 0,
		},
		IAmBored: harmonytask.SingletonTaskAdder(AlertMangerInterval, a),
	}
}

func (a *AlertTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	return
}

var _ harmonytask.TaskInterface = &AlertTask{}

// sendAlert sends an alert to PagerDuty with the provided payload data.
// It creates a PDData struct with the provided routing key, event action and payload.
// It creates an HTTP POST request with the PagerDuty event URL as the endpoint and the marshaled JSON data as the request body.
// It sends the request using an HTTP client with a maximum of 5 retries for network errors with exponential backoff before each retry.
// It handles different HTTP response status codes and returns an error based on the status code().
// If all retries fail, it returns an error indicating the last network error encountered.
func (a *AlertTask) sendAlert(data *pdPayload) error {

	type pdData struct {
		RoutingKey  string     `json:"routing_key"`
		EventAction string     `json:"event_action"`
		Payload     *pdPayload `json:"payload"`
	}

	payload := &pdData{
		RoutingKey:  a.cfg.PageDutyIntegrationKey,
		EventAction: "trigger",
		Payload:     data,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling JSON: %w", err)
	}

	req, err := http.NewRequest("POST", a.cfg.PagerDutyEventURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	var resp *http.Response

	for i := 0; i < 5; i++ { // Maximum of 5 retries
		resp, err = client.Do(req)
		if err != nil {
			time.Sleep(time.Duration(2*i) * time.Second) // Exponential backoff
			continue
		}
		defer func() { _ = resp.Body.Close() }()

		switch resp.StatusCode {
		case 202:
			log.Debug("Accepted: The event has been accepted by PagerDuty.")
			return nil
		case 400:
			bd, rerr := io.ReadAll(resp.Body)
			if rerr != nil {
				return xerrors.Errorf("Bad request: payload JSON is invalid. Failed to read the body: %w", err)
			}
			return xerrors.Errorf("Bad request: payload JSON is invalid %s", string(bd))
		case 429:
			log.Debug("Too many API calls, retrying after backoff...")
			time.Sleep(time.Duration(5*i) * time.Second) // Exponential backoff
		case 500, 501, 502, 503, 504:
			log.Debug("Server error, retrying after backoff...")
			time.Sleep(time.Duration(5*i) * time.Second) // Exponential backoff
		default:
			log.Errorw("Response status:", resp.Status)
			return xerrors.Errorf("Unexpected HTTP response: %s", resp.Status)
		}
	}
	return fmt.Errorf("after retries, last error: %w", err)
}

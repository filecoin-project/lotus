package alertmanager

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/ctladdr"
)

var MinimumBalance = types.MustParseFIL("5 FIL")

const AlertMangerInterval = 10 * time.Minute

var log = logging.Logger("curio/alertmanager")

type AlertAPI interface {
	ctladdr.NodeApi
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

type pdData struct {
	RoutingKey  string     `json:"routing_key"`
	EventAction string     `json:"event_action"`
	Payload     *pdPayload `json:"payload"`
}

type alertFunc func(al *alerts)

var alertFuncs = []alertFunc{
	balanceCheck,
	taskFailureCheck,
}

func NewAlertTask(api AlertAPI, db *harmonydb.DB, alertingCfg config.CurioAlerting) *AlertTask {
	return &AlertTask{
		api: api,
		db:  db,
		cfg: alertingCfg,
	}
}

func (a *AlertTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	alMap := make(map[string]*alertOut)

	altrs := &alerts{
		ctx:      ctx,
		api:      a.api,
		db:       a.db,
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
			details[k] = v.alertString
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
		defer resp.Body.Close()

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

// balanceCheck retrieves the machine details from the database and performs balance checks on unique addresses.
// It populates the alert map with any errors encountered during the process and with any alerts related to low wallet balance and missing wallets.
// The alert map key is "Balance Check".
// It queries the database for the configuration of each layer and decodes it using the toml.Decode function.
// It then iterates over the addresses in the configuration and curates a list of unique addresses.
// If an address is not found in the chain node, it adds an alert to the alert map.
// If the balance of an address is below 5 Fil, it adds an alert to the alert map.
// If there are any errors encountered during the process, the err field of the alert map is populated.
func balanceCheck(al *alerts) {
	Name := "Balance Check"
	al.alertMap[Name] = &alertOut{}

	// MachineDetails represents the structure of data received from the SQL query.
	type machineDetail struct {
		ID          int
		HostAndPort string
		Layers      string
	}
	var machineDetails []machineDetail

	// Get all layers in use
	err := al.db.Select(al.ctx, &machineDetails, `
				SELECT m.id, m.host_and_port, d.layers
				FROM harmony_machines m
				LEFT JOIN harmony_machine_details d ON m.id = d.machine_id;`)
	if err != nil {
		al.alertMap[Name].err = xerrors.Errorf("getting config layers for all machines: %w", err)
		return
	}

	// UniqueLayers takes an array of MachineDetails and returns a slice of unique layers.

	layerMap := make(map[string]bool)
	var uniqueLayers []string

	// Get unique layers in use
	for _, machine := range machineDetails {
		machine := machine
		// Split the Layers field into individual layers
		layers := strings.Split(machine.Layers, ",")
		for _, layer := range layers {
			layer = strings.TrimSpace(layer)
			if _, exists := layerMap[layer]; !exists && layer != "" {
				layerMap[layer] = true
				uniqueLayers = append(uniqueLayers, layer)
			}
		}
	}

	addrMap := make(map[string]bool)
	var uniqueAddrs []string

	// Get all unique addresses
	for _, layer := range uniqueLayers {
		text := ""
		cfg := config.DefaultCurioConfig()
		err := al.db.QueryRow(al.ctx, `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&text)
		if err != nil {
			if strings.Contains(err.Error(), sql.ErrNoRows.Error()) {
				al.alertMap[Name].err = xerrors.Errorf("missing layer '%s' ", layer)
				return
			}
			al.alertMap[Name].err = fmt.Errorf("could not read layer '%s': %w", layer, err)
			return
		}

		_, err = toml.Decode(text, cfg)
		if err != nil {
			al.alertMap[Name].err = fmt.Errorf("could not read layer, bad toml %s: %w", layer, err)
			return
		}

		for i := range cfg.Addresses {
			prec := cfg.Addresses[i].PreCommitControl
			com := cfg.Addresses[i].CommitControl
			term := cfg.Addresses[i].TerminateControl
			if prec != nil {
				for j := range prec {
					if _, ok := addrMap[prec[j]]; !ok && prec[j] != "" {
						addrMap[prec[j]] = true
						uniqueAddrs = append(uniqueAddrs, prec[j])
					}
				}
			}
			if com != nil {
				for j := range com {
					if _, ok := addrMap[com[j]]; !ok && com[j] != "" {
						addrMap[com[j]] = true
						uniqueAddrs = append(uniqueAddrs, com[j])
					}
				}
			}
			if term != nil {
				for j := range term {
					if _, ok := addrMap[term[j]]; !ok && term[j] != "" {
						addrMap[term[j]] = true
						uniqueAddrs = append(uniqueAddrs, term[j])
					}
				}
			}
		}
	}

	var ret string

	for _, addrStr := range uniqueAddrs {
		addr, err := address.NewFromString(addrStr)
		if err != nil {
			al.alertMap[Name].err = xerrors.Errorf("failed to parse address: %w", err)
			return
		}

		has, err := al.api.WalletHas(al.ctx, addr)
		if err != nil {
			al.alertMap[Name].err = err
			return
		}

		if !has {
			ret += fmt.Sprintf("Wallet %s was not found in chain node. ", addrStr)
		}

		balance, err := al.api.WalletBalance(al.ctx, addr)
		if err != nil {
			al.alertMap[Name].err = err
		}

		if abi.TokenAmount(MinimumBalance).GreaterThanEqual(balance) {
			ret += fmt.Sprintf("Balance for wallet %s is below 5 Fil. ", addrStr)
		}
	}
	if ret != "" {
		al.alertMap[Name].alertString = ret
	}
	return
}

// taskFailureCheck retrieves the count of failed tasks for all machines in the task history table.
// It populates the alert map with the machine name and the number of failures if it exceeds 10.
func taskFailureCheck(al *alerts) {
	Name := "TaskFailures"
	al.alertMap[Name] = &alertOut{}

	type taskFailure struct {
		Machine  string `db:"completed_by_host_and_port"`
		Failures int    `db:"failed_tasks_count"`
	}

	var taskFailures []taskFailure

	// Get all layers in use
	err := al.db.Select(al.ctx, &taskFailures, `
								SELECT completed_by_host_and_port, COUNT(*) AS failed_tasks_count
								FROM harmony_task_history
								WHERE result = FALSE
								AND work_end >= NOW() - $1::interval
								GROUP BY completed_by_host_and_port
								ORDER BY failed_tasks_count DESC;`, AlertMangerInterval.Minutes())
	if err != nil {
		al.alertMap[Name].err = xerrors.Errorf("getting failed task count for all machines: %w", err)
		return
	}

	if len(taskFailures) > 0 {
		for _, tf := range taskFailures {
			if tf.Failures > 10 {
				al.alertMap[Name].alertString += fmt.Sprintf("Machine: %s, Failures: %d. ", tf.Machine, tf.Failures)
			}
		}
	}
	return
}

func storageCheck(al *alerts) {

}

package hapi

import (
	"context"
	"github.com/filecoin-project/lotus/api/v1api"
	"html/template"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
)

type app struct {
	db *harmonydb.DB
	t  *template.Template

	rpcInfoLk  sync.Mutex
	workingApi v1api.FullNode

	actorInfoLk sync.Mutex
	actorInfos  []actorInfo
}

type actorInfo struct {
	Address string
	CLayers []string

	QualityAdjustedPower string
	RawBytePower         string

	Deadlines []actorDeadline
}

type actorDeadline struct {
	Empty      bool
	Current    bool
	Proven     bool
	PartFaulty bool
	Faulty     bool
}

func (a *app) actorSummary(w http.ResponseWriter, r *http.Request) {
	a.actorInfoLk.Lock()
	defer a.actorInfoLk.Unlock()

	a.executeTemplate(w, "actor_summary", a.actorInfos)
}

func (a *app) indexMachines(w http.ResponseWriter, r *http.Request) {
	s, err := a.clusterMachineSummary(r.Context())
	if err != nil {
		log.Errorf("cluster machine summary: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	a.executeTemplate(w, "cluster_machines", s)
}

func (a *app) indexTasks(w http.ResponseWriter, r *http.Request) {
	s, err := a.clusterTaskSummary(r.Context())
	if err != nil {
		log.Errorf("cluster task summary: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	a.executeTemplate(w, "cluster_tasks", s)
}

func (a *app) indexTasksHistory(w http.ResponseWriter, r *http.Request) {
	s, err := a.clusterTaskHistorySummary(r.Context())
	if err != nil {
		log.Errorf("cluster task history summary: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	a.executeTemplate(w, "cluster_task_history", s)
}

var templateDev = os.Getenv("LOTUS_WEB_DEV") == "1"

func (a *app) executeTemplate(w http.ResponseWriter, name string, data interface{}) {
	if templateDev {
		fs := os.DirFS("./cmd/lotus-provider/web/hapi/web")
		a.t = template.Must(template.ParseFS(fs, "*"))
	}
	if err := a.t.ExecuteTemplate(w, name, data); err != nil {
		log.Errorf("execute template %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

type machineSummary struct {
	Address      string
	ID           int64
	SinceContact string
}

type taskSummary struct {
	Name        string
	SincePosted string
	Owner       *string
	ID          int64
}

type taskHistorySummary struct {
	Name   string
	TaskID int64

	Posted, Start, End string

	Result bool
	Err    string

	CompletedBy string
}

func (a *app) clusterMachineSummary(ctx context.Context) ([]machineSummary, error) {
	rows, err := a.db.Query(ctx, "SELECT id, host_and_port, last_contact FROM harmony_machines")
	if err != nil {
		return nil, err // Handle error
	}
	defer rows.Close()

	var summaries []machineSummary
	for rows.Next() {
		var m machineSummary
		var lastContact time.Time

		if err := rows.Scan(&m.ID, &m.Address, &lastContact); err != nil {
			return nil, err // Handle error
		}

		m.SinceContact = time.Since(lastContact).Round(time.Second).String()

		summaries = append(summaries, m)
	}
	return summaries, nil
}

func (a *app) clusterTaskSummary(ctx context.Context) ([]taskSummary, error) {
	rows, err := a.db.Query(ctx, "SELECT id, name, update_time, owner_id FROM harmony_task")
	if err != nil {
		return nil, err // Handle error
	}
	defer rows.Close()

	var summaries []taskSummary
	for rows.Next() {
		var t taskSummary
		var posted time.Time

		if err := rows.Scan(&t.ID, &t.Name, &posted, &t.Owner); err != nil {
			return nil, err // Handle error
		}

		t.SincePosted = time.Since(posted).Round(time.Second).String()

		summaries = append(summaries, t)
	}
	return summaries, nil
}

func (a *app) clusterTaskHistorySummary(ctx context.Context) ([]taskHistorySummary, error) {
	rows, err := a.db.Query(ctx, "SELECT id, name, task_id, posted, work_start, work_end, result, err, completed_by_host_and_port FROM harmony_task_history ORDER BY work_end DESC LIMIT 15")
	if err != nil {
		return nil, err // Handle error
	}
	defer rows.Close()

	var summaries []taskHistorySummary
	for rows.Next() {
		var t taskHistorySummary
		var posted, start, end time.Time

		if err := rows.Scan(&t.TaskID, &t.Name, &t.TaskID, &posted, &start, &end, &t.Result, &t.Err, &t.CompletedBy); err != nil {
			return nil, err // Handle error
		}

		t.Posted = posted.Round(time.Second).Format("02 Jan 06 15:04")
		t.Start = start.Round(time.Second).Format("02 Jan 06 15:04")
		t.End = end.Round(time.Second).Format("02 Jan 06 15:04")

		summaries = append(summaries, t)
	}
	return summaries, nil
}

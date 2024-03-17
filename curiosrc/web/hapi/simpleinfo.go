package hapi

import (
	"context"
	"html/template"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api/v1api"
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

	ActorBalance, ActorAvailable, WorkerBalance string

	Win1, Win7, Win30 int64

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

func (a *app) indexPipelinePorep(w http.ResponseWriter, r *http.Request) {
	s, err := a.porepPipelineSummary(r.Context())
	if err != nil {
		log.Errorf("porep pipeline summary: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	a.executeTemplate(w, "pipeline_porep", s)
}

var templateDev = os.Getenv("LOTUS_WEB_DEV") == "1"

func (a *app) executeTemplate(w http.ResponseWriter, name string, data interface{}) {
	if templateDev {
		fs := os.DirFS("./cmd/curio/web/hapi/web")
		a.t = template.Must(template.ParseFS(fs, "*"))
	}
	if err := a.t.ExecuteTemplate(w, name, data); err != nil {
		log.Errorf("execute template %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

type machineRecentTask struct {
	TaskName string
	Success  int64
	Fail     int64
}

type machineSummary struct {
	Address      string
	ID           int64
	SinceContact string

	RecentTasks []*machineRecentTask
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

	Posted, Start, Queued, Took string

	Result bool
	Err    string

	CompletedBy string
}

func (a *app) clusterMachineSummary(ctx context.Context) ([]machineSummary, error) {
	// First get task summary for tasks completed in the last 24 hours
	// NOTE: This query uses harmony_task_history_work_index, task history may get big
	tsrows, err := a.db.Query(ctx, `SELECT hist.completed_by_host_and_port, hist.name, hist.result, count(1) FROM harmony_task_history hist
	    WHERE hist.work_end > now() - INTERVAL '1 day'
	    GROUP BY hist.completed_by_host_and_port, hist.name, hist.result
	    ORDER BY completed_by_host_and_port ASC`)
	if err != nil {
		return nil, err
	}
	defer tsrows.Close()

	// Map of machine -> task -> recent task
	taskSummaries := map[string]map[string]*machineRecentTask{}

	for tsrows.Next() {
		var taskName string
		var result bool
		var count int64
		var machine string

		if err := tsrows.Scan(&machine, &taskName, &result, &count); err != nil {
			return nil, err
		}

		if _, ok := taskSummaries[machine]; !ok {
			taskSummaries[machine] = map[string]*machineRecentTask{}
		}

		if _, ok := taskSummaries[machine][taskName]; !ok {
			taskSummaries[machine][taskName] = &machineRecentTask{TaskName: taskName}
		}

		if result {
			taskSummaries[machine][taskName].Success = count
		} else {
			taskSummaries[machine][taskName].Fail = count
		}
	}

	// Then machine summary
	rows, err := a.db.Query(ctx, "SELECT id, host_and_port, last_contact FROM harmony_machines order by host_and_port asc")
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

		// Add recent tasks
		if ts, ok := taskSummaries[m.Address]; ok {
			for _, t := range ts {
				m.RecentTasks = append(m.RecentTasks, t)
			}
			sort.Slice(m.RecentTasks, func(i, j int) bool {
				return m.RecentTasks[i].TaskName < m.RecentTasks[j].TaskName
			})
		}

		summaries = append(summaries, m)
	}
	return summaries, nil
}

func (a *app) clusterTaskSummary(ctx context.Context) ([]taskSummary, error) {
	rows, err := a.db.Query(ctx, "SELECT id, name, update_time, owner_id FROM harmony_task order by update_time asc, owner_id")
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

		t.Posted = posted.Local().Round(time.Second).Format("02 Jan 06 15:04")
		t.Start = start.Local().Round(time.Second).Format("02 Jan 06 15:04")
		//t.End = end.Local().Round(time.Second).Format("02 Jan 06 15:04")

		t.Queued = start.Sub(posted).Round(time.Second).String()
		if t.Queued == "0s" {
			t.Queued = start.Sub(posted).Round(time.Millisecond).String()
		}

		t.Took = end.Sub(start).Round(time.Second).String()
		if t.Took == "0s" {
			t.Took = end.Sub(start).Round(time.Millisecond).String()
		}

		summaries = append(summaries, t)
	}
	return summaries, nil
}

type porepPipelineSummary struct {
	Actor string

	CountSDR          int
	CountTrees        int
	CountPrecommitMsg int
	CountWaitSeed     int
	CountPoRep        int
	CountCommitMsg    int
	CountDone         int
	CountFailed       int
}

func (a *app) porepPipelineSummary(ctx context.Context) ([]porepPipelineSummary, error) {
	rows, err := a.db.Query(ctx, `
	SELECT 
		sp_id,
		COUNT(*) FILTER (WHERE after_sdr = false) as CountSDR,
		COUNT(*) FILTER (WHERE (after_tree_d = false OR after_tree_c = false OR after_tree_r = false) AND after_sdr = true) as CountTrees,
		COUNT(*) FILTER (WHERE after_tree_r = true and after_precommit_msg = false) as CountPrecommitMsg,
		COUNT(*) FILTER (WHERE after_precommit_msg_success = false AND after_precommit_msg = true) as CountWaitSeed,
		COUNT(*) FILTER (WHERE after_porep = false AND after_precommit_msg_success = true) as CountPoRep,
		COUNT(*) FILTER (WHERE after_commit_msg_success = false AND after_porep = true) as CountCommitMsg,
		COUNT(*) FILTER (WHERE after_commit_msg_success = true) as CountDone,
		COUNT(*) FILTER (WHERE failed = true) as CountFailed
	FROM 
		sectors_sdr_pipeline
	GROUP BY sp_id`)
	if err != nil {
		return nil, xerrors.Errorf("query: %w", err)
	}
	defer rows.Close()

	var summaries []porepPipelineSummary
	for rows.Next() {
		var summary porepPipelineSummary
		if err := rows.Scan(&summary.Actor, &summary.CountSDR, &summary.CountTrees, &summary.CountPrecommitMsg, &summary.CountWaitSeed, &summary.CountPoRep, &summary.CountCommitMsg, &summary.CountDone, &summary.CountFailed); err != nil {
			return nil, xerrors.Errorf("scan: %w", err)
		}
		summary.Actor = "f0" + summary.Actor

		summaries = append(summaries, summary)
	}
	return summaries, nil
}

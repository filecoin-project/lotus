package hapi

import (
	"embed"
	"html/template"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
)

//go:embed web/*
var templateFS embed.FS

func Routes(r *mux.Router, deps *deps.Deps) error {
	t, err := template.ParseFS(templateFS, "web/*")
	if err != nil {
		return xerrors.Errorf("parse templates: %w", err)
	}

	a := &app{
		db: deps.DB,
		t:  t,
	}

	go a.watchRpc()
	go a.watchActor()

	r.HandleFunc("/simpleinfo/actorsummary", a.actorSummary)
	r.HandleFunc("/simpleinfo/machines", a.indexMachines)
	r.HandleFunc("/simpleinfo/tasks", a.indexTasks)
	r.HandleFunc("/simpleinfo/taskhistory", a.indexTasksHistory)
	r.HandleFunc("/simpleinfo/pipeline-porep", a.indexPipelinePorep)
	return nil
}

var log = logging.Logger("lpweb")

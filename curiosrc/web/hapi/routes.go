package hapi

import (
	"embed"
	"text/template"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
)

//go:embed web/*
var templateFS embed.FS

func Routes(r *mux.Router, deps *deps.Deps) error {
	t, err := makeTemplate().ParseFS(templateFS, "web/*")
	if err != nil {
		return xerrors.Errorf("parse templates: %w", err)
	}

	a := &app{
		db: deps.DB,
		t:  t,
	}

	go a.watchRpc()
	go a.watchActor()

	// index page (simple info)
	r.HandleFunc("/simpleinfo/actorsummary", a.actorSummary)
	r.HandleFunc("/simpleinfo/machines", a.indexMachines)
	r.HandleFunc("/simpleinfo/tasks", a.indexTasks)
	r.HandleFunc("/simpleinfo/taskhistory", a.indexTasksHistory)
	r.HandleFunc("/simpleinfo/pipeline-porep", a.indexPipelinePorep)

	// pipeline-porep page
	r.HandleFunc("/pipeline-porep/sectors", a.pipelinePorepSectors)

	// node info page
	r.HandleFunc("/node/{id}", a.nodeInfo)

	// sector info page
	r.HandleFunc("/sector/{sp}/{id}", a.sectorInfo)
	return nil
}

func makeTemplate() *template.Template {
	return template.New("").Funcs(template.FuncMap{
		"toHumanBytes": func(b int64) string {
			return types.SizeStr(types.NewInt(uint64(b)))
		},
	})
}

var log = logging.Logger("curio/web")

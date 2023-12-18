package lpseal

import (
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
)

type SDRAPI interface {
}

type SDRTask struct {
	api SDRAPI
	db  *harmonydb.DB

	maxSDR int
}

func (s *SDRTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	//TODO implement me
	panic("implement me")
}

func (s *SDRTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SDRTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:         s.maxSDR,
		Name:        "SDR",
		Cost:        resources.Resources{},
		MaxFailures: 0,
		Follows:     nil,
	}
}

func (s *SDRTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	//TODO implement me
	panic("implement me")
}

var _ harmonytask.TaskInterface = &SDRTask{}

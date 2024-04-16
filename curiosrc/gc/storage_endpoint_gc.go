package gc

import (
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
)

type StorageEndpointGC struct {
}

func (s *StorageEndpointGC) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	//TODO implement me
	panic("implement me")
}

func (s *StorageEndpointGC) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	//TODO implement me
	panic("implement me")
}

func (s *StorageEndpointGC) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  1,
		Name: "",
		Cost: resources.Resources{},
	}
}

func (s *StorageEndpointGC) Adder(taskFunc harmonytask.AddTaskFunc) {
	//TODO implement me
	panic("implement me")
}

var _ harmonytask.TaskInterface = &StorageEndpointGC{}

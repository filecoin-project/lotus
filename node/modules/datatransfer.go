package modules

import (
	dagservicedt "github.com/filecoin-project/lotus/datatransfer/impl/dagservice"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// NewProviderDAGServiceDataTransfer returns a data transfer manager that just
// uses the provider's Staging DAG service for transfers
func NewProviderDAGServiceDataTransfer(dag dtypes.StagingDAG) dtypes.ProviderDataTransfer {
	return dagservicedt.NewDAGServiceDataTransfer(dag)
}

// NewClientDAGServiceDataTransfer returns a data transfer manager that just
// uses the clients's Client DAG service for transfers
func NewClientDAGServiceDataTransfer(dag dtypes.ClientDAG) dtypes.ClientDataTransfer {
	return dagservicedt.NewDAGServiceDataTransfer(dag)
}

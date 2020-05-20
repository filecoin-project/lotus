package client

import (
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
)

// NewCommonRPC creates a new http jsonrpc client.
func NewCommonRPC(addr string, requestHeader http.Header) (api.Common, jsonrpc.ClientCloser, error) {
	var res apistruct.CommonStruct
	closer, err := jsonrpc.NewMergeClient(addr, "Filecoin",
		[]interface{}{
			&res.Internal,
		},
		requestHeader,
	)

	return &res, closer, err
}

// NewFullNodeRPC creates a new http jsonrpc client.
func NewFullNodeRPC(addr string, requestHeader http.Header) (api.FullNode, jsonrpc.ClientCloser, error) {
	var res apistruct.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(addr, "Filecoin",
		[]interface{}{
			&res.CommonStruct.Internal,
			&res.Internal,
		}, requestHeader)

	return &res, closer, err
}

// NewStorageMinerRPC creates a new http jsonrpc client for storage miner
func NewStorageMinerRPC(addr string, requestHeader http.Header) (api.StorageMiner, jsonrpc.ClientCloser, error) {
	var res apistruct.StorageMinerStruct
	closer, err := jsonrpc.NewMergeClient(addr, "Filecoin",
		[]interface{}{
			&res.CommonStruct.Internal,
			&res.Internal,
		},
		requestHeader,
	)

	return &res, closer, err
}

func NewWorkerRPC(addr string, requestHeader http.Header) (api.WorkerApi, jsonrpc.ClientCloser, error) {
	var res apistruct.WorkerStruct
	closer, err := jsonrpc.NewMergeClient(addr, "Filecoin",
		[]interface{}{
			&res.Internal,
		},
		requestHeader,
	)

	return &res, closer, err
}

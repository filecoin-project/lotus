package client

import (
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

// NewFullNodeRPC creates a new http jsonrpc client.
func NewFullNodeRPC(addr string, requestHeader http.Header) (api.FullNode, error) {
	var res api.FullNodeStruct
	_, err := jsonrpc.NewMergeClient(addr, "Filecoin",
		[]interface{}{
			&res.CommonStruct.Internal,
			&res.Internal,
		}, requestHeader)

	return &res, err
}

// NewStorageMinerRPC creates a new http jsonrpc client for storage miner
func NewStorageMinerRPC(addr string, requestHeader http.Header) (api.StorageMiner, error) {
	var res api.StorageMinerStruct
	_, err := jsonrpc.NewMergeClient(addr, "Filecoin",
		[]interface{}{
			&res.CommonStruct.Internal,
			&res.Internal,
		}, requestHeader)

	return &res, err
}
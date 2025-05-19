package build

import (
	"bytes"
	"embed"
	"encoding/json"

	apitypes "github.com/filecoin-project/lotus/api/types"
)

//go:embed openrpc
var openrpcfs embed.FS

func mustReadOpenRPCDocument(data []byte) apitypes.OpenRPCDocument {
	m := apitypes.OpenRPCDocument{}
	err := json.NewDecoder(bytes.NewBuffer(data)).Decode(&m)
	if err != nil {
		log.Fatal(err)
	}
	return m
}

func OpenRPCDiscoverJSON_Full() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/full.json")
	if err != nil {
		panic(err)
	}
	return mustReadOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_Miner() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/miner.json")
	if err != nil {
		panic(err)
	}
	return mustReadOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_Worker() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/worker.json")
	if err != nil {
		panic(err)
	}
	return mustReadOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_Gateway() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/gateway.json")
	if err != nil {
		panic(err)
	}
	return mustReadOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_GatewayV2() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/v2/gateway.json")
	if err != nil {
		panic(err)
	}
	return mustReadOpenRPCDocument(data)
}

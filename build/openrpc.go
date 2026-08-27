package build

import (
	"bytes"
	"embed"
	"encoding/json"
	"sync"

	apitypes "github.com/filecoin-project/lotus/api/types"
)

//go:embed openrpc
var openrpcfs embed.FS

func mustReadOpenRPCDocument(path string) apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile(path)
	if err != nil {
		panic(err)
	}
	m := apitypes.OpenRPCDocument{}
	err = json.NewDecoder(bytes.NewBuffer(data)).Decode(&m)
	if err != nil {
		log.Fatal(err)
	}
	return m
}

// Decoded once and cached for the process lifetime.
var (
	openRPCDiscoverJSON_Full      = sync.OnceValue(func() apitypes.OpenRPCDocument { return mustReadOpenRPCDocument("openrpc/full.json") })
	openRPCDiscoverJSON_Miner     = sync.OnceValue(func() apitypes.OpenRPCDocument { return mustReadOpenRPCDocument("openrpc/miner.json") })
	openRPCDiscoverJSON_Worker    = sync.OnceValue(func() apitypes.OpenRPCDocument { return mustReadOpenRPCDocument("openrpc/worker.json") })
	openRPCDiscoverJSON_Gateway   = sync.OnceValue(func() apitypes.OpenRPCDocument { return mustReadOpenRPCDocument("openrpc/gateway.json") })
	openRPCDiscoverJSON_GatewayV2 = sync.OnceValue(func() apitypes.OpenRPCDocument { return mustReadOpenRPCDocument("openrpc/v2/gateway.json") })
)

func OpenRPCDiscoverJSON_Full() apitypes.OpenRPCDocument {
	return openRPCDiscoverJSON_Full()
}

func OpenRPCDiscoverJSON_Miner() apitypes.OpenRPCDocument {
	return openRPCDiscoverJSON_Miner()
}

func OpenRPCDiscoverJSON_Worker() apitypes.OpenRPCDocument {
	return openRPCDiscoverJSON_Worker()
}

func OpenRPCDiscoverJSON_Gateway() apitypes.OpenRPCDocument {
	return openRPCDiscoverJSON_Gateway()
}

func OpenRPCDiscoverJSON_GatewayV2() apitypes.OpenRPCDocument {
	return openRPCDiscoverJSON_GatewayV2()
}

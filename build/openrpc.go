package build

import (
	"encoding/json"

	rice "github.com/GeertJohan/go.rice"
)

type OpenRPCDocument map[string]interface{}

func OpenRPCDiscoverJSON_Full() OpenRPCDocument {
	data := rice.MustFindBox("openrpc").MustBytes("full.json")
	m := OpenRPCDocument{}
	json.Unmarshal(data, &m)
	return m
}

func OpenRPCDiscoverJSON_Miner() OpenRPCDocument {
	data := rice.MustFindBox("openrpc").MustBytes("miner.json")
	m := OpenRPCDocument{}
	json.Unmarshal(data, &m)
	return m
}

func OpenRPCDiscoverJSON_Worker() OpenRPCDocument {
	data := rice.MustFindBox("openrpc").MustBytes("worker.json")
	m := OpenRPCDocument{}
	json.Unmarshal(data, &m)
	return m
}

package build

import (
	"encoding/json"

	rice "github.com/GeertJohan/go.rice"
)

func OpenRPCDiscoverJSON_Full() map[string]interface{} {
	data := rice.MustFindBox("openrpc").MustBytes("full.json")
	m := map[string]interface{}{}
	json.Unmarshal(data, &m)
	return m
}

func OpenRPCDiscoverJSON_Miner() map[string]interface{} {
	data := rice.MustFindBox("openrpc").MustBytes("miner.json")
	m := map[string]interface{}{}
	json.Unmarshal(data, &m)
	return m
}

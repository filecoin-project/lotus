package build

import rice "github.com/GeertJohan/go.rice"

func OpenRPCDiscoverJSON_Full() []byte {
	return rice.MustFindBox("openrpc").MustBytes("full.json")
}

func OpenRPCDiscoverJSON_Miner() []byte {
	return rice.MustFindBox("openrpc").MustBytes("miner.json")
}

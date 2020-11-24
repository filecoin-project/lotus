package build

import (
	"bytes"
	"compress/gzip"
	"encoding/json"

	rice "github.com/GeertJohan/go.rice"
)

type OpenRPCDocument map[string]interface{}

func mustReadGzippedOpenRPCDocument(data []byte) OpenRPCDocument {
	zr, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		log.Fatal(err)
	}
	m := OpenRPCDocument{}
	err = json.NewDecoder(zr).Decode(&m)
	if err != nil {
		log.Fatal(err)
	}
	err = zr.Close()
	if err != nil {
		log.Fatal(err)
	}
	return m
}

func OpenRPCDiscoverJSON_Full() OpenRPCDocument {
	data := rice.MustFindBox("openrpc").MustBytes("full.json.gz")
	return mustReadGzippedOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_Miner() OpenRPCDocument {
	data := rice.MustFindBox("openrpc").MustBytes("miner.json.gz")
	return mustReadGzippedOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_Worker() OpenRPCDocument {
	data := rice.MustFindBox("openrpc").MustBytes("worker.json.gz")
	return mustReadGzippedOpenRPCDocument(data)
}

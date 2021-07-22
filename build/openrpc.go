package build

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"

	apitypes "github.com/filecoin-project/lotus/api/types"
)

//go:embed openrpc
var openrpcfs embed.FS

func mustReadGzippedOpenRPCDocument(data []byte) apitypes.OpenRPCDocument {
	zr, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		log.Fatal(err)
	}
	m := apitypes.OpenRPCDocument{}
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

func OpenRPCDiscoverJSON_Full() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/full.json.gz")
	if err != nil {
		panic(err)
	}
	return mustReadGzippedOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_Miner() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/miner.json.gz")
	if err != nil {
		panic(err)
	}
	return mustReadGzippedOpenRPCDocument(data)
}

func OpenRPCDiscoverJSON_Worker() apitypes.OpenRPCDocument {
	data, err := openrpcfs.ReadFile("openrpc/worker.json.gz")
	if err != nil {
		panic(err)
	}
	return mustReadGzippedOpenRPCDocument(data)
}

package build

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"

	rice "github.com/GeertJohan/go.rice"
)

type OpenRPCDocument map[string]interface{}

func mustReadGzippedOpenRPCDocument(data []byte) OpenRPCDocument {
	buf := bytes.NewBuffer(data)
	zr, err := gzip.NewReader(buf)
	if err != nil {
		log.Fatal(err)
	}
	uncompressed := bytes.NewBuffer([]byte{})
	_, err = io.Copy(uncompressed, zr)
	if err != nil {
		log.Fatal(err)
	}
	err = zr.Close()
	if err != nil {
		log.Fatal(err)
	}
	m := OpenRPCDocument{}
	err = json.Unmarshal(uncompressed.Bytes(), &m)
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

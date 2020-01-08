package build

import (
	rice "github.com/GeertJohan/go.rice"
	logging "github.com/ipfs/go-log"
)

// moved from now-defunct build/paramfetch.go
var log = logging.Logger("build")

func MaybeGenesis() []byte {
	builtinGen, err := rice.FindBox("genesis")
	if err != nil {
		log.Warn("loading built-in genesis: %s", err)
		return nil
	}
	genBytes, err := builtinGen.Bytes("devnet.car")
	if err != nil {
		log.Warn("loading built-in genesis: %s", err)
	}

	return genBytes
}

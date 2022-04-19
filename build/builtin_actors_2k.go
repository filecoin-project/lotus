//go:build debug || 2k || testground
// +build debug 2k testground

package build

import (
	_ "embed"
)

//go:embed builtin-actors/v8/builtin-actors-devnet.car
var actorsv8 []byte

func BuiltinActorsV8Bundle() []byte {
	return actorsv8
}

//go:embed builtin-actors/v7/builtin-actors-devnet.car
var actorsv7 []byte

func BuiltinActorsV7Bundle() []byte {
	return actorsv7
}

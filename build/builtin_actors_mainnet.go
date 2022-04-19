//go:build !debug && !2k && !testground && !calibnet && !nerpanet && !butterflynet && !interopnet
// +build !debug,!2k,!testground,!calibnet,!nerpanet,!butterflynet,!interopnet

package build

import (
	_ "embed"
)

//go:embed builtin-actors/v8/builtin-actors-mainnet.car
var actorsv8 []byte

func BuiltinActorsV8Bundle() []byte {
	return actorsv8
}

//go:embed builtin-actors/v7/builtin-actors-mainnet.car
var actorsv7 []byte

func BuiltinActorsV7Bundle() []byte {
	return actorsv7
}

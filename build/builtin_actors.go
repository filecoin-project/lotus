package build

import (
	_ "embed"
)

//go:embed builtin-actors/builtin-actors-v8.car
var actorsv8 []byte

//go:embed builtin-actors/builtin-actors-v7.car
var actorsv7 []byte

func BuiltinActorsV8Bundle() []byte {
	return actorsv8
}

func BuiltinActorsV7Bundle() []byte {
	return actorsv7
}

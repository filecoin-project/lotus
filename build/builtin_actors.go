package build

import (
	_ "embed"
)

//go:embed builtin-actors/builtin-actors-v8.car
var actorsv8 []byte

func BuiltinActorsV8Bundle() []byte {
	return actorsv8
}

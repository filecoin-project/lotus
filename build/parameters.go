package build

import (
	_ "embed"
)

//go:embed proof-params/parameters.json
var params []byte

func ParametersJSON() []byte {
	return params
}

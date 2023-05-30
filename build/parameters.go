package build

import (
	_ "embed"
)

//go:embed proof-params/parameters.json
var params []byte

//go:embed proof-params/srs-inner-product.json
var srs []byte

func ParametersJSON() []byte {
	return params
}

func SrsJSON() []byte {
	return srs
}

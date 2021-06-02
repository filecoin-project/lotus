package build

import (
	_ "embed"
)

//go:embed proof-params/parameters.json
var params []byte

func ParametersJSON() []byte {
	return params
}

func SrsJSON() []byte {
	return rice.MustFindBox("proof-params").MustBytes("srs-inner-product.json")
}

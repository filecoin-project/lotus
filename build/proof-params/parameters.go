package proofparams

import (
	_ "embed"
)

//go:embed parameters.json
var params []byte

//go:embed srs-inner-product.json
var srs []byte

func ParametersJSON() []byte {
	return params
}

func SrsJSON() []byte {
	return srs
}

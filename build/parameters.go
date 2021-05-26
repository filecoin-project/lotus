package build

import rice "github.com/GeertJohan/go.rice"

func ParametersJSON() []byte {
	return rice.MustFindBox("proof-params").MustBytes("parameters.json")
}

func SrsJSON() []byte {
	return rice.MustFindBox("proof-params").MustBytes("srs-inner-product.json")
}

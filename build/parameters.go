package build

import rice "github.com/GeertJohan/go.rice"

func ParametersJson() []byte {
	return rice.MustFindBox("proof-params").MustBytes("parameters.json")
}

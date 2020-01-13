package build

import rice "github.com/GeertJohan/go.rice"

var ParametersJson = rice.MustFindBox("proof-params").MustBytes("parameters.json")

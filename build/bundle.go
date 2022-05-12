package build

import (
	_ "embed"

	"github.com/filecoin-project/lotus/chain/actors"
)

var NetworkBundle string

//go:embed bundle.toml
var BuiltinActorBundles []byte

type BundleSpec struct {
	Bundles []Bundle
}

type Bundle struct {
	Version actors.Version
	Release string
}

package build

import (
	_ "embed"

	"github.com/filecoin-project/lotus/chain/actors"
)

var NetworkBundle string

//go:embed bundles.toml
var BuiltinActorBundles []byte

type BundleSpec struct {
	Bundles []Bundle
}

type Bundle struct {
	// actors version in this bundle
	Version actors.Version
	// release id
	Release string
	// bundle path: optional, uses the appropriate release bundle if unset
	Path string
	// development version; when set, in conjunction with path, it will always
	// load the bundle to the blockstore
	Development bool
}

package build

import (
	_ "embed"

	"github.com/filecoin-project/lotus/chain/actors"
)

var NetworkBundle string

func GetNetworkBundle() string {
	switch NetworkBundle {
	case "devnet":
		return "devnet"
	case "calibnet", "calibrationnet":
		return "calibrationnet"
	case "butterflynet":
		return "butterflynet"
	case "interopnet", "caterpillarnet":
		return "caterpillarnet"
	default:
		return "mainnet"
	}
}

//go:embed bundles.toml
var BuiltinActorBundles []byte

type BundleSpec struct {
	Bundles []Bundle
}

type Bundle struct {
	// Version is the actors version in this bundle
	Version actors.Version
	// Release is the release id
	Release string
	// Path is the (optional) bundle path; uses the appropriate release bundle if unset
	Path string
	// Devlopment indicates whether this is a development version; when set, in conjunction with path,
	// it will always load the bundle to the blockstore
	Development bool
}

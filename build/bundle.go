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
	// Version is the actors version in this bundle
	Version actors.Version
	// Release is the release id
	Release string
	// Path is the (optional) bundle path; takes precedence over url
	Path string
	// URL is the (optional) bundle URL; takes precdence over github release
	URL string
	// CHecksum is the bundle sha256 checksume in hex, when specifying a URL.
	Checksum string
	// Devlopment indicates whether this is a development version; when set, in conjunction with path,
	// it will always load the bundle to the blockstore, without recording the manifest CID in the
	// datastore.
	Development bool
}

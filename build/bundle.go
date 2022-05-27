package build

import (
	_ "embed"

	"github.com/filecoin-project/lotus/chain/actors"
)

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
	Path map[string]string
	// URL is the (optional) bundle URL; takes precedence over github release
	URL map[string]BundleURL
	// ManifestCID is the (optional) per network manifest CIDs for verification at load time
	ManifestCID map[string]string
	// Devlopment indicates whether this is a development version; when set, in conjunction with path,
	// it will always load the bundle to the blockstore, without recording the manifest CID in the
	// datastore.
	Development bool
}

type BundleURL struct {
	// URL is the url of the bundle
	URL string
	// Checksum is the sha256 checksum of the bundle
	Checksum string
}

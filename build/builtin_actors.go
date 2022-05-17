package build

import (
	"bytes"

	"github.com/filecoin-project/lotus/chain/actors"

	"github.com/BurntSushi/toml"
)

var BuiltinActorReleases map[actors.Version]Bundle

func init() {
	BuiltinActorReleases = make(map[actors.Version]Bundle)

	spec := BundleSpec{}

	r := bytes.NewReader(BuiltinActorBundles)
	_, err := toml.DecodeReader(r, &spec)
	if err != nil {
		panic(err)
	}

	for _, b := range spec.Bundles {
		BuiltinActorReleases[b.Version] = b
	}
}

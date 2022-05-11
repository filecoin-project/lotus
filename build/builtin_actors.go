package build

import (
	"github.com/filecoin-project/lotus/chain/actors"
)

// TODO a nicer interface would be to embed (and parse) a toml file containing the releases
var BuiltinActorReleases map[actors.Version]string

func init() {
	BuiltinActorReleases = map[actors.Version]string{
		actors.Version8: "b71c2ec785aec23d",
	}
}

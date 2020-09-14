package builtin

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/network"
)

type Version int

const (
	Version0 = iota
)

// Converts a network version into a specs-actors version.
func VersionForNetwork(version network.Version) Version {
	switch version {
	case network.Version0, network.Version1:
		return Version0
	default:
		panic(fmt.Sprintf("unsupported network version %d", version))
	}
}

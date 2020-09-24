package actors

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/network"
)

type Version int

const (
	Version0 = iota
	Version1
)

// Converts a network version into an actors adt version.
func VersionForNetwork(version network.Version) Version {
	switch version {
	case network.Version0, network.Version1, network.Version2:
		return Version0
	default:
		panic(fmt.Sprintf("unsupported network version %d", version))
	}
}

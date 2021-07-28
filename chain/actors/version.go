package actors

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/network"
)

type Version int

var LatestVersion = 5

var Versions = []int{0, 2, 3, 4, LatestVersion}

const (
	Version0 Version = 0
	Version2 Version = 2
	Version3 Version = 3
	Version4 Version = 4
	Version5 Version = 5
)

// Converts a network version into an actors adt version.
func VersionForNetwork(version network.Version) Version {
	// Handle previous api version (1 - 13)
	if version > network.Version0 && version < network.Version1 {
		version = version * network.Version1 // assumes (1) even spacing between integer network numbers and (2) network.Version0 == 0
	}
	switch version {
	case network.Version0, network.Version1, network.Version2, network.Version3:
		return Version0
	case network.Version4, network.Version5, network.Version6, network.Version6AndAHalf, network.Version7, network.Version8, network.Version9:
		return Version2
	case network.Version10, network.Version11:
		return Version3
	case network.Version12:
		return Version4
	case network.Version13:
		return Version5
	default:
		panic(fmt.Sprintf("unsupported network version %d", version))
	}
}

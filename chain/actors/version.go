package actors

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/network"
)

type Version int

/* inline-gen template

var LatestVersion = {{.latestActorsVersion}}

var Versions = []int{ {{range .actorVersions}} {{.}}, {{end}} }

const ({{range .actorVersions}}
	Version{{.}} Version = {{.}}{{end}}
)

/* inline-gen start */

var LatestVersion = 6

var Versions = []int{0, 2, 3, 4, 5, 6}

const (
	Version0 Version = 0
	Version2 Version = 2
	Version3 Version = 3
	Version4 Version = 4
	Version5 Version = 5
	Version6 Version = 6
)

/* inline-gen end */

// Converts a network version into an actors adt version.
func VersionForNetwork(version network.Version) (Version, error) {
	switch version {
	case network.Version0, network.Version1, network.Version2, network.Version3:
		return Version0, nil
	case network.Version4, network.Version5, network.Version6, network.Version7, network.Version8, network.Version9:
		return Version2, nil
	case network.Version10, network.Version11:
		return Version3, nil
	case network.Version12:
		return Version4, nil
	case network.Version13:
		return Version5, nil
	case network.Version14:
		return Version6, nil
	default:
		return -1, fmt.Errorf("unsupported network version %d", version)
	}
}

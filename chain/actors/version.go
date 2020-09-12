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

// VersionForNetwork resolves the network version into an specs-actors version.
func VersionForNetwork(v network.Version) Version {
	switch v {
	case network.Version0, network.Version1:
		return Version0
	default:
		panic(fmt.Sprintf("unimplemented network version: %d", v))
	}
}

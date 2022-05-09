package build

import (
	"encoding/hex"
	"hash/fnv"
	"os"
)

var CurrentCommit string
var BuildType int

const (
	BuildDefault = 0
	BuildMainnet = 0x1
	BuildDebug   = 0x2
)

func BuildTypeString() string {
	var currentNetwork string
	currentNetwork, ok := os.LookupEnv("LOTUS_NETWORK")
	if !ok {
		currentNetwork = buildDefaultNetwork
	}

	h := fnv.New128a()
	sum := h.Sum(MaybeGenesis())
	sums := hex.EncodeToString(sum[:5])

	debugstr := ""

	if BuildType|BuildDebug != 0 {
		debugstr = "+DEBUG"
	}

	return "+" + currentNetwork + "[" + sums + "]" + debugstr
}

// BuildVersion is the local build version
const BuildVersion = "1.15.3-dev"

func UserVersion() string {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion
	}

	return BuildVersion + BuildTypeString() + CurrentCommit
}

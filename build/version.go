package build

import (
	"embed"
	"os"
)

var CurrentCommit string
var BuildType int

const (
	BuildDefault      = 0
	BuildMainnet      = 0x1
	Build2k           = 0x2
	BuildDebug        = 0x3
	BuildCalibnet     = 0x4
	BuildInteropnet   = 0x5
	BuildButterflynet = 0x7
)

func BuildTypeString() string {
	switch BuildType {
	case BuildDefault:
		return ""
	case BuildMainnet:
		return "+mainnet"
	case Build2k:
		return "+2k"
	case BuildDebug:
		return "+debug"
	case BuildCalibnet:
		return "+calibnet"
	case BuildInteropnet:
		return "+interopnet"
	case BuildButterflynet:
		return "+butterflynet"
	default:
		return "+huh?"
	}
}

// BuildVersion is the local build version
const BuildVersion = "1.17.3-dev"

func UserVersion() string {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion
	}

	return BuildVersion + BuildTypeString() + CurrentCommit
}

type BuildInfo struct {
	GoMod string

	GitHead   string
	GitStatus string
}

//go:embed buildinfo/*.txt
var embeddedBuildInfo embed.FS

func Build() (*BuildInfo, error) {
	head, err := embeddedBuildInfo.ReadFile("buildinfo/head.txt")
	if err != nil {
		return nil, err
	}
	status, err := embeddedBuildInfo.ReadFile("buildinfo/status.txt")
	if err != nil {
		return nil, err
	}
	gomod, err := embeddedBuildInfo.ReadFile("buildinfo/gomod.txt")
	if err != nil {
		return nil, err
	}

	return &BuildInfo{
		GitHead:   string(head),
		GitStatus: string(status),
		GoMod:     string(gomod),
	}, nil
}

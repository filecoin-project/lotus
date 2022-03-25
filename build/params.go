package build

import (
	_ "embed"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/ipfs/go-cid"
)

//go:embed params/params.json
var paramsBytes []byte

var activeNetworkParams networkParams

type bootstrapperList []string
type networkConfig struct {
	BlockDelaySecs           uint64
	BootstrapPeerThreshold   int
	BreezeGasTampingDuration abi.ChainEpoch
	GenesisFile              string
	GenesisNetworkVersion    network.Version
	PropagationDelaySecs     uint64
	WhitelistedBlock         string
	Address                  address.Network
	Devnet                   bool
}
type devnetConfig struct {
	SupportedProofTypes     []abi.RegisteredSealProof
	ConsensusMinerMinPower  int64
	MinVerifiedDealSize     int64
	PreCommitChallengeDelay abi.ChainEpoch
}
type upgradeList map[string]abi.ChainEpoch
type networkParams struct {
	Bootstrappers bootstrapperList
	Config        networkConfig
	DevnetConfig  devnetConfig
	DrandSchedule dtypes.DrandSchedule
	Upgrades      upgradeList
}

func init() {
	networks := make(map[string]networkParams)
	var paramb []byte
	var err error
	paramfile, ok := os.LookupEnv("LOTUS_NETWORK_DEFINITION_FILE")
	if ok {
		paramb, err = ioutil.ReadFile(paramfile)
		if err != nil {
			panic(err)
		}
	} else {
		paramb = paramsBytes
	}
	err = json.Unmarshal(paramb, &networks)
	if err != nil {
		panic("cannot parse network params json")
	}

	currentNetwork, ok := os.LookupEnv("LOTUS_NETWORK")
	if !ok {
		currentNetwork = "mainnet"
	}

	activeNetworkParams, ok = networks[currentNetwork]
	if !ok {
		var keys []string
		for k := range networks {
			keys = append(keys, k)
		}
		log.Fatalf("unsupported lotus network %s. network must be one of the following: %v", currentNetwork, keys)
	}

	SetAddressNetwork(activeNetworkParams.Config.Address)

	if activeNetworkParams.Config.Devnet {
		if len(activeNetworkParams.DevnetConfig.SupportedProofTypes) != 0 {
			policy.SetSupportedProofTypes(activeNetworkParams.DevnetConfig.SupportedProofTypes...)
		}
		if activeNetworkParams.DevnetConfig.ConsensusMinerMinPower != 0 {
			policy.SetConsensusMinerMinPower(abi.NewStoragePower(activeNetworkParams.DevnetConfig.ConsensusMinerMinPower))
		}
		if activeNetworkParams.DevnetConfig.MinVerifiedDealSize != 0 {
			policy.SetMinVerifiedDealSize(abi.NewStoragePower(activeNetworkParams.DevnetConfig.MinVerifiedDealSize))
		}
		if activeNetworkParams.DevnetConfig.PreCommitChallengeDelay != 0 {
			policy.SetPreCommitChallengeDelay(activeNetworkParams.DevnetConfig.PreCommitChallengeDelay)
		}
	}
}

// configuration getters
func BlockDelaySecs() uint64 {
	return activeNetworkParams.Config.BlockDelaySecs
}

func BreezeGasTampingDuration() abi.ChainEpoch {
	return activeNetworkParams.Config.BreezeGasTampingDuration
}

func PropagationDelaySecs() uint64 {
	return activeNetworkParams.Config.PropagationDelaySecs
}

func WhitelistedBlock() cid.Cid {
	if activeNetworkParams.Config.WhitelistedBlock == "" {
		return cid.Undef
	}
	return MustParseCid(activeNetworkParams.Config.WhitelistedBlock)
}

// network upgrades
func UpgradeBreezeHeight() abi.ChainEpoch {
	return MustParseUpgrade("breeze")
}

func UpgradeSmokeHeight() abi.ChainEpoch {
	return MustParseUpgrade("smoke")
}

func UpgradeIgnitionHeight() abi.ChainEpoch {
	return MustParseUpgrade("ignition")
}

func UpgradeRefuelHeight() abi.ChainEpoch {
	return MustParseUpgrade("refuel")
}

func UpgradeTapeHeight() abi.ChainEpoch {
	return MustParseUpgrade("tape")
}

func UpgradeAssemblyHeight() abi.ChainEpoch {
	return MustParseUpgrade("assembly")
}

func UpgradeLiftoffHeight() abi.ChainEpoch {
	return MustParseUpgrade("liftoff")
}

func UpgradeKumquatHeight() abi.ChainEpoch {
	return MustParseUpgrade("kumquat")
}

func UpgradeCalicoHeight() abi.ChainEpoch {
	return MustParseUpgrade("calico")
}

func UpgradePersianHeight() abi.ChainEpoch {
	return MustParseUpgrade("persian")
}

func UpgradeOrangeHeight() abi.ChainEpoch {
	return MustParseUpgrade("orange")
}

func UpgradeClausHeight() abi.ChainEpoch {
	return MustParseUpgrade("claus")
}

func UpgradeTrustHeight() abi.ChainEpoch {
	return MustParseUpgrade("trust")
}

func UpgradeNorwegianHeight() abi.ChainEpoch {
	return MustParseUpgrade("norwegian")
}

func UpgradeTurboHeight() abi.ChainEpoch {
	return MustParseUpgrade("turbo")
}

func UpgradeHyperdriveHeight() abi.ChainEpoch {
	return MustParseUpgrade("hyperdrive")
}

func UpgradeChocolateHeight() abi.ChainEpoch {
	return MustParseUpgrade("chocolate")
}

func UpgradeOhSnapHeight() abi.ChainEpoch {
	return MustParseUpgrade("ohsnap")
}

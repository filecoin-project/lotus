package buildconstants

import (
	"encoding/json"
	"math/big"
	"os"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// moved from now-defunct build/paramfetch.go
var log = logging.Logger("build/buildtypes")

func SetAddressNetwork(n address.Network) {
	address.CurrentNetwork = n
}

func MustParseAddress(addr string) address.Address {
	ret, err := address.NewFromString(addr)
	if err != nil {
		panic(err)
	}

	return ret
}

func IsNearUpgrade(epoch, upgradeEpoch abi.ChainEpoch) bool {
	if upgradeEpoch < 0 {
		return false
	}
	return epoch > upgradeEpoch-policy.ChainFinality && epoch < upgradeEpoch+policy.ChainFinality
}

func MustParseID(id string) peer.ID {
	p, err := peer.Decode(id)
	if err != nil {
		panic(err)
	}
	return p
}

func wholeFIL(whole uint64) *big.Int {
	bigWhole := big.NewInt(int64(whole))
	return bigWhole.Mul(bigWhole, big.NewInt(int64(FilecoinPrecision)))
}

func F3Manifest() *manifest.Manifest {
	if F3ManifestBytes == nil {
		return nil
	}
	var manif manifest.Manifest

	if err := json.Unmarshal(F3ManifestBytes, &manif); err != nil {
		log.Panicf("failed to unmarshal F3 manifest: %s", err)
	}
	if err := manif.Validate(); err != nil {
		log.Panicf("invalid F3 manifest: %s", err)
	}

	if ptCid := os.Getenv("F3_INITIAL_POWERTABLE_CID"); ptCid != "" {
		if k, err := cid.Parse(ptCid); err != nil {
			log.Errorf("failed to parse F3_INITIAL_POWERTABLE_CID %q: %s", ptCid, err)
		} else if manif.InitialPowerTable.Defined() && k != manif.InitialPowerTable {
			log.Errorf("ignoring F3_INITIAL_POWERTABLE_CID as lotus has a hard-coded initial F3 power table")
		} else {
			manif.InitialPowerTable = k
		}
	}
	if !manif.InitialPowerTable.Defined() {
		log.Warn("initial power table is not specified, it will be populated automatically assuming this is testing network")
	}

	// EC Period sanity check
	if manif.EC.Period != time.Duration(BlockDelaySecs)*time.Second {
		log.Panicf("static manifest EC period is %v, expected %v", manif.EC.Period, time.Duration(BlockDelaySecs)*time.Second)
	}
	return &manif
}

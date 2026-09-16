package buildconstants

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"

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

const ethAddressLength = 20

// A masked-ID EVM address is 0xff, then zeroes, then a big-endian actor ID.
var ethMaskedIDPrefix = [ethAddressLength - 8]byte{0xff}

// MustParseFilOrEthAddress parses a Filecoin address or a 0x-prefixed EVM address; the masked-ID
// EVM form yields f0 and any other EVM address f410. The EVM half mirrors
// ethtypes.EthAddress.ToFilecoinAddress, which this package cannot import.
func MustParseFilOrEthAddress(addr string) address.Address {
	if !strings.HasPrefix(addr, "0x") {
		return MustParseAddress(addr)
	}
	payload, err := hex.DecodeString(addr[len("0x"):])
	if err != nil {
		panic(err)
	}
	if len(payload) != ethAddressLength {
		panic(fmt.Errorf("EVM address %s is %d bytes, want %d", addr, len(payload), ethAddressLength))
	}
	if bytes.HasPrefix(payload, ethMaskedIDPrefix[:]) {
		ret, err := address.NewIDAddress(binary.BigEndian.Uint64(payload[len(ethMaskedIDPrefix):]))
		if err != nil {
			panic(err)
		}
		return ret
	}
	ret, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, payload)
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

package build

import (
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// Core network constants

func BlocksTopic(netName dtypes.NetworkName) string   { return "/fil/blocks/" + string(netName) }
func MessagesTopic(netName dtypes.NetworkName) string { return "/fil/msgs/" + string(netName) }
func IndexerIngestTopic(netName dtypes.NetworkName) string {

	nn := string(netName)
	// The network name testnetnet is here for historical reasons.
	// Going forward we aim to use the name `mainnet` where possible.
	if nn == "testnetnet" {
		nn = "mainnet"
	}

	return "/indexer/ingest/" + nn
}
func DhtProtocolName(netName dtypes.NetworkName) protocol.ID {
	return protocol.ID("/fil/kad/" + string(netName))
}

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

func MustParseCid(c string) cid.Cid {
	ret, err := cid.Decode(c)
	if err != nil {
		panic(err)
	}

	return ret
}

package build

import (
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/filecoin-project/lotus/build/buildconstants"
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

// Deprecated: Use buildconstants.SetAddressNetwork instead.
var SetAddressNetwork = buildconstants.SetAddressNetwork

// Deprecated: Use buildconstants.MustParseAddress instead.
var MustParseAddress = buildconstants.MustParseAddress

// Deprecated: Use buildconstants.MustParseCid instead.
var MustParseCid = buildconstants.MustParseCid

package v1api

import (
	"github.com/filecoin-project/lotus/api"
)

type (
	FullNode       = api.FullNode
	FullNodeStruct = api.FullNodeStruct
)

type RawFullNodeAPI FullNode

func PermissionedFullAPI(a FullNode) FullNode {
	return api.PermissionedFullAPI(a)
}

type LotusProviderStruct = api.LotusProviderStruct

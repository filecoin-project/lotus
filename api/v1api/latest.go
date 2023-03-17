package v1api

import (
	"github.com/brossetti1/lotus/api"
)

type FullNode = api.FullNode
type FullNodeStruct = api.FullNodeStruct

type RawFullNodeAPI FullNode

func PermissionedFullAPI(a FullNode) FullNode {
	return api.PermissionedFullAPI(a)
}

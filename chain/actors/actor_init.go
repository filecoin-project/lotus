package actors

import (
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("actors")

type InitActor = init_.Actor

type InitActorState = init_.State

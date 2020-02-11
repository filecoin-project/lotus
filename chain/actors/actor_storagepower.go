package actors

import (
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
)

type StoragePowerActor = power.Actor

var SPAMethods = builtin.MethodsPower

type StoragePowerState = power.State

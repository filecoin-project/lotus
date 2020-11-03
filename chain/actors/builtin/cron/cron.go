package cron

import (
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
)

var (
	Address = builtin2.CronActorAddr
	Methods = builtin2.MethodsCron
)

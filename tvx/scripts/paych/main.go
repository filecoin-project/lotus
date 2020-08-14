package main

import (
	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

var (
	initialBal = abi.NewTokenAmount(200_000_000_000)
	toSend     = abi.NewTokenAmount(10_000)
)

func main() {
	g := NewGenerator()

	g.MessageVectorGroup("paych",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "create-ok",
				Version: "v1",
				Desc:    "",
			},
			Func: happyPathCreate,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "update-ok",
				Version: "v1",
				Desc:    "",
			},
			Func: happyPathUpdate,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "collect-ok",
				Version: "v1",
				Desc:    "",
			},
			Func: happyPathCollect,
		},
	)

	g.Wait()
}

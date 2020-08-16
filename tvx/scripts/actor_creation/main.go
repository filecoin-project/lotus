package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
)

func main() {
	g := NewGenerator()

	g.MessageVectorGroup("addresses",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "sequential-10",
				Version: "v1",
				Desc:    "actor addresses are sequential",
			},
			Func: sequentialAddresses,
		},
	)

	g.MessageVectorGroup("on_transfer",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-create-secp256k1",
				Version: "v1",
			},
			Func: actorCreationOnTransfer(actorCreationOnTransferParams{
				senderType:   address.SECP256K1,
				senderBal:    abi.NewTokenAmount(1_000_000_000_000_000),
				receiverAddr: MustNewSECP256K1Addr("publickeyfoo"),
				amount:       abi.NewTokenAmount(10_000),
				exitCode:     exitcode.Ok,
			}),
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-create-bls",
				Version: "v1",
			},
			Func: actorCreationOnTransfer(actorCreationOnTransferParams{
				senderType:   address.SECP256K1,
				senderBal:    abi.NewTokenAmount(1_000_000_000_000_000),
				receiverAddr: MustNewBLSAddr(1),
				amount:       abi.NewTokenAmount(10_000),
				exitCode:     exitcode.Ok,
			}),
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-secp256k1-insufficient-balance",
				Version: "v1",
			},
			Func: actorCreationOnTransfer(actorCreationOnTransferParams{
				senderType:   address.SECP256K1,
				senderBal:    abi.NewTokenAmount(9_999),
				receiverAddr: MustNewSECP256K1Addr("publickeyfoo"),
				amount:       abi.NewTokenAmount(10_000),
				exitCode:     exitcode.SysErrSenderStateInvalid,
			}),
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-bls-insufficient-balance",
				Version: "v1",
			},
			Func: actorCreationOnTransfer(actorCreationOnTransferParams{
				senderType:   address.SECP256K1,
				senderBal:    abi.NewTokenAmount(9_999),
				receiverAddr: MustNewBLSAddr(1),
				amount:       abi.NewTokenAmount(10_000),
				exitCode:     exitcode.SysErrSenderStateInvalid,
			}),
		},
	)

	g.Wait()
}

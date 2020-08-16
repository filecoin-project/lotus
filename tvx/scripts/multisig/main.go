package main

import (
	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
)

const (
	gasLimit  = 1_000_000_000
	gasFeeCap = 200
)

func main() {
	g := NewGenerator()
	defer g.Wait()

	g.MessageVectorGroup("basic",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-create",
				Version: "v1",
				Desc:    "multisig actor constructor ok",
			},
			Func: constructor,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-propose-and-cancel",
				Version: "v1",
				Desc:    "multisig actor propose and cancel ok",
			},
			Func: proposeAndCancelOk,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-propose-and-approve",
				Version: "v1",
				Desc:    "multisig actor propose, unauthorized proposals+approval, and approval ok",
			},
			Func: proposeAndApprove,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-add-signer",
				Version: "v1",
				Desc:    "multisig actor accepts only AddSigner messages that go through a reflexive flow",
			},
			Func: addSigner,
		},
	)
}

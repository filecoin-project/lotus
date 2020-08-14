package main

import (
	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
)

func main() {
	g := NewGenerator()

	g.MessageVectorGroup("nested_sends",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-basic",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_OkBasic,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-to-new-actor",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_OkToNewActor,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-to-new-actor-with-invoke",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_OkToNewActorWithInvoke,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-recursive",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_OkRecursive,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "ok-non-cbor-params-with-transfer",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_OKNonCBORParamsWithTransfer,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-non-existent-id-address",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailNonexistentIDAddress,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-non-existent-actor-address",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailNonexistentActorAddress,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-invalid-method-num-new-actor",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailInvalidMethodNumNewActor,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-invalid-method-num-for-actor",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailInvalidMethodNumForActor,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-missing-params",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailMissingParams,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-mismatch-params",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailMismatchParams,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-inner-abort",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailInnerAbort,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-aborted-exec",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailAbortedExec,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "fail-insufficient-funds-for-transfer-in-inner-send",
				Version: "v1",
				Desc:    "",
			},
			Func: nestedSends_FailInsufficientFundsForTransferInInnerSend,
		},
	)

	g.Wait()
}

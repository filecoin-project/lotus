package requestvalidation_test

import (
	"math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dss "github.com/ipfs/go-datastore/sync"
	blocksutil "github.com/ipfs/go-ipfs-blocksutil"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/libp2p/go-libp2p/core/peer"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-statestore"

	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	rv "github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
)

var blockGenerator = blocksutil.NewBlockGenerator()

func uniqueStorageDealProposal() (market.ClientDealProposal, error) {
	clientAddr, err := address.NewIDAddress(uint64(rand.Int()))
	if err != nil {
		return market.ClientDealProposal{}, err
	}
	providerAddr, err := address.NewIDAddress(uint64(rand.Int()))
	if err != nil {
		return market.ClientDealProposal{}, err
	}
	return market.ClientDealProposal{
		Proposal: market.DealProposal{
			PieceCID: blockGenerator.Next().Cid(),
			Client:   clientAddr,
			Provider: providerAddr,
		},
		ClientSignature: crypto.Signature{
			Data: []byte("foo bar cat dog"),
			Type: crypto.SigTypeBLS,
		},
	}, nil
}

func newClientDeal(minerID peer.ID, state storagemarket.StorageDealStatus) (storagemarket.ClientDeal, error) {
	newProposal, err := uniqueStorageDealProposal()
	if err != nil {
		return storagemarket.ClientDeal{}, err
	}
	proposalNd, err := cborutil.AsIpld(&newProposal)
	if err != nil {
		return storagemarket.ClientDeal{}, err
	}
	minerAddr, err := address.NewIDAddress(uint64(rand.Int()))
	if err != nil {
		return storagemarket.ClientDeal{}, err
	}

	return storagemarket.ClientDeal{
		ClientDealProposal: newProposal,
		ProposalCid:        proposalNd.Cid(),
		DataRef: &storagemarket.DataRef{
			Root: blockGenerator.Next().Cid(),
		},
		Miner:       minerID,
		MinerWorker: minerAddr,
		State:       state,
	}, nil
}

func newMinerDeal(clientID peer.ID, state storagemarket.StorageDealStatus) (storagemarket.MinerDeal, error) {
	newProposal, err := uniqueStorageDealProposal()
	if err != nil {
		return storagemarket.MinerDeal{}, err
	}
	proposalNd, err := cborutil.AsIpld(&newProposal)
	if err != nil {
		return storagemarket.MinerDeal{}, err
	}
	ref := blockGenerator.Next().Cid()

	return storagemarket.MinerDeal{
		ClientDealProposal: newProposal,
		ProposalCid:        proposalNd.Cid(),
		Client:             clientID,
		State:              state,
		Ref:                &storagemarket.DataRef{Root: ref},
	}, nil
}

type pushDeals struct {
	state *statestore.StateStore
}

func (pd *pushDeals) Get(proposalCid cid.Cid) (storagemarket.MinerDeal, error) {
	var deal storagemarket.MinerDeal
	err := pd.state.Get(proposalCid).Get(&deal)
	return deal, err
}

type pullDeals struct {
	state *statestore.StateStore
}

func (pd *pullDeals) Get(proposalCid cid.Cid) (storagemarket.ClientDeal, error) {
	var deal storagemarket.ClientDeal
	err := pd.state.Get(proposalCid).Get(&deal)
	return deal, err
}

func TestUnifiedRequestValidator(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	state := statestore.New(namespace.Wrap(ds, datastore.NewKey("/deals/client")))
	minerID := peer.ID("fakepeerid")
	clientID := peer.ID("fakepeerid2")
	block := blockGenerator.Next()

	t.Run("which only accepts pulls", func(t *testing.T) {
		urv := rv.NewUnifiedRequestValidator(nil, &pullDeals{state})

		t.Run("ValidatePush fails", func(t *testing.T) {
			_, err := urv.ValidatePush(datatransfer.ChannelID{}, minerID, basicnode.NewString("wrong DT type"), block.Cid(), nil)
			if !xerrors.Is(err, rv.ErrNoPushAccepted) {
				t.Fatal("Push should fail for the client request validator for storage deals")
			}
		})

	})

	t.Run("which only accepts pushes", func(t *testing.T) {
		urv := rv.NewUnifiedRequestValidator(&pushDeals{state}, nil)

		t.Run("ValidatePull fails", func(t *testing.T) {
			_, err := urv.ValidatePull(datatransfer.ChannelID{}, clientID, basicnode.NewString("wrong DT type"), block.Cid(), nil)
			if !xerrors.Is(err, rv.ErrNoPullAccepted) {
				t.Fatal("Pull should fail for the provider request validator for storage deals")
			}
		})

		AssertPushValidator(t, urv, clientID, state)
	})

	t.Run("which accepts pushes and pulls", func(t *testing.T) {
		urv := rv.NewUnifiedRequestValidator(&pushDeals{state}, &pullDeals{state})

		AssertValidatesPulls(t, urv, minerID, state)
		AssertPushValidator(t, urv, clientID, state)
	})
}

func AssertPushValidator(t *testing.T, validator datatransfer.RequestValidator, sender peer.ID, state *statestore.StateStore) {
	t.Run("ValidatePush fails deal not found", func(t *testing.T) {
		proposal, err := uniqueStorageDealProposal()
		if err != nil {
			t.Fatal("error creating proposal")
		}
		proposalNd, err := cborutil.AsIpld(&proposal)
		if err != nil {
			t.Fatal("error serializing proposal")
		}
		sdtv := rv.StorageDataTransferVoucher{proposalNd.Cid()}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePush(t, validator, datatransfer.ChannelID{}, sender, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, proposal.Proposal.PieceCID, nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if result.Accepted {
					t.Fatal("should not accept deal")
				}
			})
	})
	t.Run("ValidatePush fails wrong piece ref", func(t *testing.T) {
		minerDeal, err := newMinerDeal(sender, storagemarket.StorageDealProposalAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(minerDeal.ProposalCid, &minerDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		sdtv := rv.StorageDataTransferVoucher{minerDeal.ProposalCid}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePush(t, validator, datatransfer.ChannelID{}, sender, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, blockGenerator.Next().Cid(), nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if result.Accepted {
					t.Fatal("should not accept deal")
				}
			})
	})
	t.Run("ValidatePush fails wrong deal state", func(t *testing.T) {
		minerDeal, err := newMinerDeal(sender, storagemarket.StorageDealActive)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(minerDeal.ProposalCid, &minerDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		ref := minerDeal.Ref
		sdtv := rv.StorageDataTransferVoucher{minerDeal.ProposalCid}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePush(t, validator, datatransfer.ChannelID{}, sender, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, ref.Root, nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if result.Accepted {
					t.Fatal("should not accept deal")
				}
			})
	})
	t.Run("ValidatePush succeeds", func(t *testing.T) {
		minerDeal, err := newMinerDeal(sender, storagemarket.StorageDealValidating)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(minerDeal.ProposalCid, &minerDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		ref := minerDeal.Ref
		sdtv := rv.StorageDataTransferVoucher{minerDeal.ProposalCid}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePush(t, validator, datatransfer.ChannelID{}, sender, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, ref.Root, nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if !result.Accepted {
					t.Fatal("should accept deal")
				}
			})
	})
}

func AssertValidatesPulls(t *testing.T, validator datatransfer.RequestValidator, receiver peer.ID, state *statestore.StateStore) {
	t.Run("ValidatePull fails deal not found", func(t *testing.T) {
		proposal, err := uniqueStorageDealProposal()
		if err != nil {
			t.Fatal("error creating proposal")
		}
		proposalNd, err := cborutil.AsIpld(&proposal)
		if err != nil {
			t.Fatal("error serializing proposal")
		}
		sdtv := rv.StorageDataTransferVoucher{proposalNd.Cid()}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePull(t, validator, datatransfer.ChannelID{}, receiver, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, proposal.Proposal.PieceCID, nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if result.Accepted {
					t.Fatal("should not accept deal")
				}
			})
	})
	t.Run("ValidatePull fails wrong piece ref", func(t *testing.T) {
		clientDeal, err := newClientDeal(receiver, storagemarket.StorageDealProposalAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(clientDeal.ProposalCid, &clientDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		sdtv := rv.StorageDataTransferVoucher{clientDeal.ProposalCid}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePull(t, validator, datatransfer.ChannelID{}, receiver, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, blockGenerator.Next().Cid(), nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if result.Accepted {
					t.Fatal("should not accept deal")
				}
			})
	})
	t.Run("ValidatePull fails wrong deal state", func(t *testing.T) {
		clientDeal, err := newClientDeal(receiver, storagemarket.StorageDealActive)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(clientDeal.ProposalCid, &clientDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		payloadCid := clientDeal.DataRef.Root
		sdtv := rv.StorageDataTransferVoucher{clientDeal.ProposalCid}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePull(t, validator, datatransfer.ChannelID{}, receiver, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, payloadCid, nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if result.Accepted {
					t.Fatal("should not accept deal")
				}
			})
	})
	t.Run("ValidatePull succeeds", func(t *testing.T) {
		clientDeal, err := newClientDeal(receiver, storagemarket.StorageDealValidating)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(clientDeal.ProposalCid, &clientDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		payloadCid := clientDeal.DataRef.Root
		sdtv := rv.StorageDataTransferVoucher{clientDeal.ProposalCid}
		voucher := requestvalidation.BindnodeRegistry.TypeToNode(&sdtv)
		checkValidateAndRevalidatePull(t, validator, datatransfer.ChannelID{}, receiver, datatransfer.TypedVoucher{Voucher: voucher, Type: rv.StorageDataTransferVoucherType}, payloadCid, nil,
			func(t *testing.T, result datatransfer.ValidationResult, err error) {
				if err != nil {
					t.Fatal("unexpected error validating")
				}
				if !result.Accepted {
					t.Fatal("should accept deal")
				}
			})
	})
}

func checkValidateAndRevalidatePush(t *testing.T, validator datatransfer.RequestValidator, chid datatransfer.ChannelID, sender peer.ID, voucher datatransfer.TypedVoucher, baseCid cid.Cid, selector datamodel.Node,
	test func(t *testing.T, result datatransfer.ValidationResult, err error)) {
	result, err := validator.ValidatePush(chid, sender, voucher.Voucher, baseCid, selector)
	test(t, result, err)
	channel := tut.NewTestChannel(tut.TestChannelParams{
		IsPull:   false,
		Sender:   sender,
		Vouchers: []datatransfer.TypedVoucher{voucher},
		BaseCID:  baseCid,
		Selector: selector,
	})
	result, err = validator.ValidateRestart(chid, channel)
	test(t, result, err)
}

func checkValidateAndRevalidatePull(t *testing.T, validator datatransfer.RequestValidator, chid datatransfer.ChannelID, receiver peer.ID, voucher datatransfer.TypedVoucher, baseCid cid.Cid, selector datamodel.Node,
	test func(t *testing.T, result datatransfer.ValidationResult, err error)) {
	result, err := validator.ValidatePull(chid, receiver, voucher.Voucher, baseCid, selector)
	test(t, result, err)
	channel := tut.NewTestChannel(tut.TestChannelParams{
		IsPull:    true,
		Recipient: receiver,
		Vouchers:  []datatransfer.TypedVoucher{voucher},
		BaseCID:   baseCid,
		Selector:  selector,
	})
	result, err = validator.ValidateRestart(chid, channel)
	test(t, result, err)
}

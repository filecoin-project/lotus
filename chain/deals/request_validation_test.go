package deals_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dss "github.com/ipfs/go-datastore/sync"
	blocksutil "github.com/ipfs/go-ipfs-blocksutil"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/deals"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborrpc"
)

var blockGenerator = blocksutil.NewBlockGenerator()

type wrongDTType struct {
}

func (w *wrongDTType) ToBytes() []byte {
	return []byte{}
}

func (w *wrongDTType) FromBytes([]byte) error {
	return fmt.Errorf("not implemented")
}

func (w *wrongDTType) Identifier() string {
	return "WrongDTTYPE"
}

func uniqueStorageDealProposal() (actors.StorageDealProposal, error) {
	clientAddr, err := address.NewIDAddress(uint64(rand.Int()))
	if err != nil {
		return actors.StorageDealProposal{}, err
	}
	providerAddr, err := address.NewIDAddress(uint64(rand.Int()))
	if err != nil {
		return actors.StorageDealProposal{}, err
	}
	return actors.StorageDealProposal{
		PieceRef: blockGenerator.Next().Cid().Bytes(),
		Client:   clientAddr,
		Provider: providerAddr,
		ProposerSignature: &types.Signature{
			Data: []byte("foo bar cat dog"),
			Type: types.KTBLS,
		},
	}, nil
}

func newClientDeal(minerId peer.ID, state api.DealState) (deals.ClientDeal, error) {
	newProposal, err := uniqueStorageDealProposal()
	if err != nil {
		return deals.ClientDeal{}, err
	}
	proposalNd, err := cborrpc.AsIpld(&newProposal)
	if err != nil {
		return deals.ClientDeal{}, err
	}
	return deals.ClientDeal{
		Proposal:    newProposal,
		ProposalCid: proposalNd.Cid(),
		Miner:       minerId,
		State:       state,
	}, nil
}

func newMinerDeal(clientID peer.ID, state api.DealState) (deals.MinerDeal, error) {
	newProposal, err := uniqueStorageDealProposal()
	if err != nil {
		return deals.MinerDeal{}, err
	}
	proposalNd, err := cborrpc.AsIpld(&newProposal)
	if err != nil {
		return deals.MinerDeal{}, err
	}
	ref, err := cid.Cast(newProposal.PieceRef)
	if err != nil {
		return deals.MinerDeal{}, err
	}
	return deals.MinerDeal{
		Proposal:    newProposal,
		ProposalCid: proposalNd.Cid(),
		Client:      clientID,
		State:       state,
		Ref:         ref,
	}, nil
}

func TestClientRequestValidation(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	state := deals.ClientStateStore{deals.NewStateStore(namespace.Wrap(ds, datastore.NewKey("/deals/client")))}

	crv := deals.NewClientRequestValidator(ds)
	minerId := peer.ID("fakepeerid")
	block := blockGenerator.Next()
	t.Run("ValidatePush fails", func(t *testing.T) {
		if crv.ValidatePush(minerId, &wrongDTType{}, block.Cid(), nil) != deals.ErrNoPushAccepted {
			t.Fatal("Push should fail for the client request validator for storage deals")
		}
	})
	t.Run("ValidatePull fails deal not found", func(t *testing.T) {
		proposal, err := uniqueStorageDealProposal()
		if err != nil {
			t.Fatal("error creating proposal")
		}
		proposalNd, err := cborrpc.AsIpld(&proposal)
		if err != nil {
			t.Fatal("error serializing proposal")
		}
		pieceRef, err := cid.Cast(proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if crv.ValidatePull(minerId, &deals.StorageDataTransferVoucher{proposalNd.Cid()}, pieceRef, nil) != deals.ErrNoDeal {
			t.Fatal("Pull should fail if there is no deal stored")
		}
	})
	t.Run("ValidatePull fails wrong client", func(t *testing.T) {
		otherMiner := peer.ID("otherminer")
		clientDeal, err := newClientDeal(otherMiner, api.DealAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(clientDeal.ProposalCid, &clientDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		pieceRef, err := cid.Cast(clientDeal.Proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if crv.ValidatePull(minerId, &deals.StorageDataTransferVoucher{clientDeal.ProposalCid}, pieceRef, nil) != deals.ErrWrongPeer {
			t.Fatal("Pull should fail if miner address is incorrect")
		}
	})
	t.Run("ValidatePull fails wrong piece ref", func(t *testing.T) {
		clientDeal, err := newClientDeal(minerId, api.DealAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(clientDeal.ProposalCid, &clientDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		if crv.ValidatePull(minerId, &deals.StorageDataTransferVoucher{clientDeal.ProposalCid}, blockGenerator.Next().Cid(), nil) != deals.ErrWrongPiece {
			t.Fatal("Pull should fail if piece ref is incorrect")
		}
	})
	t.Run("ValidatePull fails wrong deal state", func(t *testing.T) {
		clientDeal, err := newClientDeal(minerId, api.DealComplete)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(clientDeal.ProposalCid, &clientDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		pieceRef, err := cid.Cast(clientDeal.Proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if crv.ValidatePull(minerId, &deals.StorageDataTransferVoucher{clientDeal.ProposalCid}, pieceRef, nil) != deals.ErrInacceptableDealState {
			t.Fatal("Pull should fail if deal is in a state that cannot be data transferred")
		}
	})
	t.Run("ValidatePull succeeds", func(t *testing.T) {
		clientDeal, err := newClientDeal(minerId, api.DealAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(clientDeal.ProposalCid, &clientDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		pieceRef, err := cid.Cast(clientDeal.Proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if crv.ValidatePull(minerId, &deals.StorageDataTransferVoucher{clientDeal.ProposalCid}, pieceRef, nil) != nil {
			t.Fatal("Pull should should succeed when all parameters are correct")
		}
	})
}

func TestProviderRequestValidation(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	state := deals.MinerStateStore{deals.NewStateStore(namespace.Wrap(ds, datastore.NewKey("/deals/client")))}

	mrv := deals.NewProviderRequestValidator(ds)
	clientID := peer.ID("fakepeerid")
	block := blockGenerator.Next()
	t.Run("ValidatePull fails", func(t *testing.T) {
		if mrv.ValidatePull(clientID, &wrongDTType{}, block.Cid(), nil) != deals.ErrNoPullAccepted {
			t.Fatal("Pull should fail for the provider request validator for storage deals")
		}
	})

	t.Run("ValidatePush fails deal not found", func(t *testing.T) {
		proposal, err := uniqueStorageDealProposal()
		if err != nil {
			t.Fatal("error creating proposal")
		}
		proposalNd, err := cborrpc.AsIpld(&proposal)
		if err != nil {
			t.Fatal("error serializing proposal")
		}
		pieceRef, err := cid.Cast(proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if mrv.ValidatePush(clientID, &deals.StorageDataTransferVoucher{proposalNd.Cid()}, pieceRef, nil) != deals.ErrNoDeal {
			t.Fatal("Push should fail if there is no deal stored")
		}
	})
	t.Run("ValidatePush fails wrong miner", func(t *testing.T) {
		otherClient := peer.ID("otherclient")
		minerDeal, err := newMinerDeal(otherClient, api.DealAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(minerDeal.ProposalCid, &minerDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		pieceRef, err := cid.Cast(minerDeal.Proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if mrv.ValidatePush(clientID, &deals.StorageDataTransferVoucher{minerDeal.ProposalCid}, pieceRef, nil) != deals.ErrWrongPeer {
			t.Fatal("Push should fail if miner address is incorrect")
		}
	})
	t.Run("ValidatePush fails wrong piece ref", func(t *testing.T) {
		minerDeal, err := newMinerDeal(clientID, api.DealAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(minerDeal.ProposalCid, &minerDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		if mrv.ValidatePush(clientID, &deals.StorageDataTransferVoucher{minerDeal.ProposalCid}, blockGenerator.Next().Cid(), nil) != deals.ErrWrongPiece {
			t.Fatal("Push should fail if piece ref is incorrect")
		}
	})
	t.Run("ValidatePush fails wrong deal state", func(t *testing.T) {
		minerDeal, err := newMinerDeal(clientID, api.DealComplete)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(minerDeal.ProposalCid, &minerDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		pieceRef, err := cid.Cast(minerDeal.Proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if mrv.ValidatePush(clientID, &deals.StorageDataTransferVoucher{minerDeal.ProposalCid}, pieceRef, nil) != deals.ErrInacceptableDealState {
			t.Fatal("Push should fail if deal is in a state that cannot be data transferred")
		}
	})
	t.Run("ValidatePush succeeds", func(t *testing.T) {
		minerDeal, err := newMinerDeal(clientID, api.DealAccepted)
		if err != nil {
			t.Fatal("error creating client deal")
		}
		if err := state.Begin(minerDeal.ProposalCid, &minerDeal); err != nil {
			t.Fatal("deal tracking failed")
		}
		pieceRef, err := cid.Cast(minerDeal.Proposal.PieceRef)
		if err != nil {
			t.Fatal("unable to construct piece cid")
		}
		if mrv.ValidatePush(clientID, &deals.StorageDataTransferVoucher{minerDeal.ProposalCid}, pieceRef, nil) != nil {
			t.Fatal("Push should should succeed when all parameters are correct")
		}
	})
}

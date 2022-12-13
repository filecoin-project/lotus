package network

import (
	"bufio"
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
)

type legacyDealStatusStream struct {
	p        peer.ID
	host     host.Host
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ DealStatusStream = (*legacyDealStatusStream)(nil)

func (d *legacyDealStatusStream) ReadDealStatusRequest() (DealStatusRequest, error) {
	var q migrations.DealStatusRequest0

	if err := q.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return DealStatusRequestUndefined, err
	}
	return DealStatusRequest{
		Proposal:  q.Proposal,
		Signature: q.Signature,
	}, nil
}

func (d *legacyDealStatusStream) WriteDealStatusRequest(q DealStatusRequest) error {
	return cborutil.WriteCborRPC(d.rw, &migrations.DealStatusRequest0{
		Proposal:  q.Proposal,
		Signature: q.Signature,
	})
}

func (d *legacyDealStatusStream) ReadDealStatusResponse() (DealStatusResponse, []byte, error) {
	var qr migrations.DealStatusResponse0

	if err := qr.UnmarshalCBOR(d.buffered); err != nil {
		return DealStatusResponseUndefined, nil, err
	}

	origBytes, err := cborutil.Dump(&qr.DealState)
	if err != nil {
		return DealStatusResponseUndefined, nil, err
	}
	return DealStatusResponse{
		DealState: storagemarket.ProviderDealState{
			State:         qr.DealState.State,
			Message:       qr.DealState.Message,
			Proposal:      qr.DealState.Proposal,
			ProposalCid:   qr.DealState.ProposalCid,
			AddFundsCid:   qr.DealState.AddFundsCid,
			PublishCid:    qr.DealState.PublishCid,
			DealID:        qr.DealState.DealID,
			FastRetrieval: qr.DealState.FastRetrieval,
		},
		Signature: qr.Signature,
	}, origBytes, nil
}

func (d *legacyDealStatusStream) WriteDealStatusResponse(qr DealStatusResponse, resign ResigningFunc) error {
	oldDs := migrations.ProviderDealState0{
		State:         qr.DealState.State,
		Message:       qr.DealState.Message,
		Proposal:      qr.DealState.Proposal,
		ProposalCid:   qr.DealState.ProposalCid,
		AddFundsCid:   qr.DealState.AddFundsCid,
		PublishCid:    qr.DealState.PublishCid,
		DealID:        qr.DealState.DealID,
		FastRetrieval: qr.DealState.FastRetrieval,
	}
	oldSig, err := resign(context.TODO(), &oldDs)
	if err != nil {
		return err
	}
	return cborutil.WriteCborRPC(d.rw, &migrations.DealStatusResponse0{
		DealState: oldDs,
		Signature: *oldSig,
	})
}

func (d *legacyDealStatusStream) Close() error {
	return d.rw.Close()
}

func (d *legacyDealStatusStream) RemotePeer() peer.ID {
	return d.p
}

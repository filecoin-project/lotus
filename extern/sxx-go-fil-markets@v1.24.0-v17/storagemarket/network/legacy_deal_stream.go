package network

import (
	"bufio"
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
)

type dealStreamv101 struct {
	p        peer.ID
	host     host.Host
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ StorageDealStream = (*dealStreamv101)(nil)

func (d *dealStreamv101) ReadDealProposal() (Proposal, error) {
	var ds migrations.Proposal0

	if err := ds.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return ProposalUndefined, err
	}

	return Proposal{
		DealProposal:  ds.DealProposal,
		Piece:         migrations.MigrateDataRef0To1(ds.Piece),
		FastRetrieval: ds.FastRetrieval,
	}, nil
}

func (d *dealStreamv101) WriteDealProposal(dp Proposal) error {
	var piece *migrations.DataRef0
	if dp.Piece != nil {
		piece = &migrations.DataRef0{
			TransferType: dp.Piece.TransferType,
			Root:         dp.Piece.Root,
			PieceCid:     dp.Piece.PieceCid,
			PieceSize:    dp.Piece.PieceSize,
		}
	}
	return cborutil.WriteCborRPC(d.rw, &migrations.Proposal0{
		DealProposal:  dp.DealProposal,
		Piece:         piece,
		FastRetrieval: dp.FastRetrieval,
	})
}

func (d *dealStreamv101) ReadDealResponse() (SignedResponse, []byte, error) {
	var dr migrations.SignedResponse0

	if err := dr.UnmarshalCBOR(d.buffered); err != nil {
		return SignedResponseUndefined, nil, err
	}
	origBytes, err := cborutil.Dump(&dr.Response)
	if err != nil {
		return SignedResponseUndefined, nil, err
	}
	return SignedResponse{
		Response: Response{
			State:          dr.Response.State,
			Message:        dr.Response.Message,
			Proposal:       dr.Response.Proposal,
			PublishMessage: dr.Response.PublishMessage,
		},
		Signature: dr.Signature,
	}, origBytes, nil
}

func (d *dealStreamv101) WriteDealResponse(dr SignedResponse, resign ResigningFunc) error {
	oldResponse := migrations.Response0{
		State:          dr.Response.State,
		Message:        dr.Response.Message,
		Proposal:       dr.Response.Proposal,
		PublishMessage: dr.Response.PublishMessage,
	}
	oldSig, err := resign(context.TODO(), &oldResponse)
	if err != nil {
		return err
	}
	return cborutil.WriteCborRPC(d.rw, &migrations.SignedResponse0{
		Response:  oldResponse,
		Signature: oldSig,
	})
}

func (d *dealStreamv101) Close() error {
	return d.rw.Close()
}

func (d *dealStreamv101) RemotePeer() peer.ID {
	return d.p
}

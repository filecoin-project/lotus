package network

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"unicode/utf8"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
)

type dealStreamv110 struct {
	p        peer.ID
	host     host.Host
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ StorageDealStream = (*dealStreamv110)(nil)

func (d *dealStreamv110) ReadDealProposal() (Proposal, error) {
	var ds migrations.Proposal1

	if err := ds.UnmarshalCBOR(d.buffered); err != nil {
		err = fmt.Errorf("unmarshalling v110 deal proposal: %w", err)
		log.Warnf(err.Error())
		return ProposalUndefined, err
	}

	// The signature over the deal proposal will be different between a v1.1.0
	// deal proposal and higher versions if the deal label cannot be parsed as
	// a utf8 string.
	// The signature is checked when submitting the Publish Storage Deals
	// message, so we reject the deal proposal here to avoid that scenario.
	if err := checkDealLabel(ds.DealProposal.Proposal.Label); err != nil {
		return ProposalUndefined, err
	}

	// Migrate the deal proposal to the new format
	prop, err := migrations.MigrateClientDealProposal0To1(*ds.DealProposal)
	if err != nil {
		err = fmt.Errorf("migrating v110 deal proposal to current version: %w", err)
		log.Warnf(err.Error())
		return ProposalUndefined, err
	}
	return Proposal{
		DealProposal:  prop,
		Piece:         ds.Piece,
		FastRetrieval: ds.FastRetrieval,
	}, nil
}

func checkDealLabel(label string) error {
	labelBytes := []byte(label)
	if !utf8.Valid(labelBytes) {
		return fmt.Errorf("cannot parse deal label 0x%s as string", hex.EncodeToString(labelBytes))
	}
	return nil
}

func (d *dealStreamv110) WriteDealProposal(dp Proposal) error {
	return cborutil.WriteCborRPC(d.rw, &dp)
}

func (d *dealStreamv110) ReadDealResponse() (SignedResponse, []byte, error) {
	var dr SignedResponse

	if err := dr.UnmarshalCBOR(d.buffered); err != nil {
		return SignedResponseUndefined, nil, err
	}
	origBytes, err := cborutil.Dump(&dr.Response)
	if err != nil {
		return SignedResponseUndefined, nil, err
	}
	return dr, origBytes, nil
}

func (d *dealStreamv110) WriteDealResponse(dr SignedResponse, _ ResigningFunc) error {
	return cborutil.WriteCborRPC(d.rw, &dr)
}

func (d *dealStreamv110) Close() error {
	return d.rw.Close()
}

func (d *dealStreamv110) RemotePeer() peer.ID {
	return d.p
}

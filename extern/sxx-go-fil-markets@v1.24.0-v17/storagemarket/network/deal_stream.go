package network

import (
	"bufio"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"
)

// TagPriority is the priority for deal streams -- they should generally be preserved above all else
const TagPriority = 100

type dealStreamv111 struct {
	p        peer.ID
	host     host.Host
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ StorageDealStream = (*dealStreamv111)(nil)

func (d *dealStreamv111) ReadDealProposal() (Proposal, error) {
	var ds Proposal

	if err := ds.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return ProposalUndefined, err
	}

	return ds, nil
}

func (d *dealStreamv111) WriteDealProposal(dp Proposal) error {
	return cborutil.WriteCborRPC(d.rw, &dp)
}

func (d *dealStreamv111) ReadDealResponse() (SignedResponse, []byte, error) {
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

func (d *dealStreamv111) WriteDealResponse(dr SignedResponse, _ ResigningFunc) error {
	return cborutil.WriteCborRPC(d.rw, &dr)
}

func (d *dealStreamv111) Close() error {
	return d.rw.Close()
}

func (d *dealStreamv111) RemotePeer() peer.ID {
	return d.p
}

package network

import (
	"bufio"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"
)

type dealStatusStream struct {
	p        peer.ID
	host     host.Host
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ DealStatusStream = (*dealStatusStream)(nil)

func (d *dealStatusStream) ReadDealStatusRequest() (DealStatusRequest, error) {
	var q DealStatusRequest

	if err := q.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return DealStatusRequestUndefined, err
	}
	return q, nil
}

func (d *dealStatusStream) WriteDealStatusRequest(q DealStatusRequest) error {
	return cborutil.WriteCborRPC(d.rw, &q)
}

func (d *dealStatusStream) ReadDealStatusResponse() (DealStatusResponse, []byte, error) {
	var qr DealStatusResponse

	if err := qr.UnmarshalCBOR(d.buffered); err != nil {
		return DealStatusResponseUndefined, nil, err
	}

	origBytes, err := cborutil.Dump(&qr.DealState)
	if err != nil {
		return DealStatusResponseUndefined, nil, err
	}
	return qr, origBytes, nil
}

func (d *dealStatusStream) WriteDealStatusResponse(qr DealStatusResponse, _ ResigningFunc) error {
	return cborutil.WriteCborRPC(d.rw, &qr)
}

func (d *dealStatusStream) Close() error {
	return d.rw.Close()
}

func (d *dealStatusStream) RemotePeer() peer.ID {
	return d.p
}

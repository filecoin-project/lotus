package network

import (
	"bufio"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"
)

type askStream struct {
	p        peer.ID
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ StorageAskStream = (*askStream)(nil)

func (as *askStream) ReadAskRequest() (AskRequest, error) {
	var a AskRequest

	if err := a.UnmarshalCBOR(as.buffered); err != nil {
		log.Warn(err)
		return AskRequestUndefined, err

	}

	return a, nil
}

func (as *askStream) WriteAskRequest(q AskRequest) error {
	return cborutil.WriteCborRPC(as.rw, &q)
}

func (as *askStream) ReadAskResponse() (AskResponse, []byte, error) {
	var resp AskResponse

	if err := resp.UnmarshalCBOR(as.buffered); err != nil {
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}

	origBytes, err := cborutil.Dump(resp.Ask.Ask)
	if err != nil {
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}
	return resp, origBytes, nil
}

func (as *askStream) WriteAskResponse(qr AskResponse, _ ResigningFunc) error {
	return cborutil.WriteCborRPC(as.rw, &qr)
}

func (as *askStream) Close() error {
	return as.rw.Close()
}

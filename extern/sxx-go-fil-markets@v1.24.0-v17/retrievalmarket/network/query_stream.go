package network

import (
	"bufio"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type queryStream struct {
	p        peer.ID
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ RetrievalQueryStream = (*queryStream)(nil)

func (qs *queryStream) ReadQuery() (retrievalmarket.Query, error) {
	var q retrievalmarket.Query

	if err := q.UnmarshalCBOR(qs.buffered); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryUndefined, err

	}

	return q, nil
}

func (qs *queryStream) RemotePeer() peer.ID {
	return qs.p
}

func (qs *queryStream) WriteQuery(q retrievalmarket.Query) error {
	return cborutil.WriteCborRPC(qs.rw, &q)
}

func (qs *queryStream) ReadQueryResponse() (retrievalmarket.QueryResponse, error) {
	var resp retrievalmarket.QueryResponse

	if err := resp.UnmarshalCBOR(qs.buffered); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	return resp, nil
}

func (qs *queryStream) WriteQueryResponse(qr retrievalmarket.QueryResponse) error {
	return cborutil.WriteCborRPC(qs.rw, &qr)
}

func (qs *queryStream) Close() error {
	return qs.rw.Close()
}

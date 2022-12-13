package network

import (
	"bufio"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
)

type oldQueryStream struct {
	p        peer.ID
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ RetrievalQueryStream = (*oldQueryStream)(nil)

func (qs *oldQueryStream) RemotePeer() peer.ID {
	return qs.p
}

func (qs *oldQueryStream) ReadQuery() (retrievalmarket.Query, error) {
	var q migrations.Query0

	if err := q.UnmarshalCBOR(qs.buffered); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryUndefined, err

	}

	return migrations.MigrateQuery0To1(q), nil
}

func (qs *oldQueryStream) WriteQuery(newQ retrievalmarket.Query) error {
	q := migrations.Query0{
		PayloadCID: newQ.PayloadCID,
		QueryParams0: migrations.QueryParams0{
			PieceCID: newQ.PieceCID,
		},
	}

	return cborutil.WriteCborRPC(qs.rw, &q)
}

func (qs *oldQueryStream) ReadQueryResponse() (retrievalmarket.QueryResponse, error) {
	var resp migrations.QueryResponse0

	if err := resp.UnmarshalCBOR(qs.buffered); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	return migrations.MigrateQueryResponse0To1(resp), nil
}

func (qs *oldQueryStream) WriteQueryResponse(newQr retrievalmarket.QueryResponse) error {
	qr := migrations.QueryResponse0{
		Status:                     newQr.Status,
		PieceCIDFound:              newQr.PieceCIDFound,
		Size:                       newQr.Size,
		PaymentAddress:             newQr.PaymentAddress,
		MinPricePerByte:            newQr.MinPricePerByte,
		MaxPaymentInterval:         newQr.MaxPaymentInterval,
		MaxPaymentIntervalIncrease: newQr.MaxPaymentIntervalIncrease,
		Message:                    newQr.Message,
		UnsealPrice:                newQr.UnsealPrice,
	}
	return cborutil.WriteCborRPC(qs.rw, &qr)
}

func (qs *oldQueryStream) Close() error {
	return qs.rw.Close()
}

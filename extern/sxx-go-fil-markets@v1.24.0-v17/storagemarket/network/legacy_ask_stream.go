package network

import (
	"bufio"
	"context"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
)

type legacyAskStream struct {
	p        peer.ID
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ StorageAskStream = (*legacyAskStream)(nil)

func (as *legacyAskStream) ReadAskRequest() (AskRequest, error) {
	var a migrations.AskRequest0

	if err := a.UnmarshalCBOR(as.buffered); err != nil {
		log.Warn(err)
		return AskRequestUndefined, err

	}

	return AskRequest{
		Miner: a.Miner,
	}, nil
}

func (as *legacyAskStream) WriteAskRequest(q AskRequest) error {
	oldQ := migrations.AskRequest0{
		Miner: q.Miner,
	}
	return cborutil.WriteCborRPC(as.rw, &oldQ)
}

func (as *legacyAskStream) ReadAskResponse() (AskResponse, []byte, error) {
	var resp migrations.AskResponse0

	if err := resp.UnmarshalCBOR(as.buffered); err != nil {
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}

	origBytes, err := cborutil.Dump(resp.Ask.Ask)
	if err != nil {
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}
	return AskResponse{
		Ask: &storagemarket.SignedStorageAsk{
			Ask:       migrations.MigrateStorageAsk0To1(resp.Ask.Ask),
			Signature: resp.Ask.Signature,
		},
	}, origBytes, nil
}

func (as *legacyAskStream) WriteAskResponse(qr AskResponse, resign ResigningFunc) error {
	newAsk := qr.Ask.Ask
	oldAsk := &migrations.StorageAsk0{
		Price:         newAsk.Price,
		VerifiedPrice: newAsk.VerifiedPrice,
		MinPieceSize:  newAsk.MinPieceSize,
		MaxPieceSize:  newAsk.MaxPieceSize,
		Miner:         newAsk.Miner,
		Timestamp:     newAsk.Timestamp,
		Expiry:        newAsk.Expiry,
		SeqNo:         newAsk.SeqNo,
	}
	oldSig, err := resign(context.TODO(), oldAsk)
	if err != nil {
		return err
	}
	return cborutil.WriteCborRPC(as.rw, &migrations.AskResponse0{
		Ask: &migrations.SignedStorageAsk0{
			Ask:       oldAsk,
			Signature: oldSig,
		},
	})
}

func (as *legacyAskStream) Close() error {
	return as.rw.Close()
}

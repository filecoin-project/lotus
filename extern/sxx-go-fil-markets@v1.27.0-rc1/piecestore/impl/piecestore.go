package piecestoreimpl

import (
	"context"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	versioned "github.com/filecoin-project/go-ds-versioning/pkg/statestore"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/piecestore/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var log = logging.Logger("piecestore")

// DSPiecePrefix is the name space for storing piece infos
var DSPiecePrefix = "/pieces"

// DSCIDPrefix is the name space for storing CID infos
var DSCIDPrefix = "/cid-infos"

// NewPieceStore returns a new piecestore based on the given datastore
func NewPieceStore(ds datastore.Batching) (piecestore.PieceStore, error) {
	pieceInfoMigrations, err := migrations.PieceInfoMigrations.Build()
	if err != nil {
		return nil, err
	}
	pieces, migratePieces := versioned.NewVersionedStateStore(namespace.Wrap(ds, datastore.NewKey(DSPiecePrefix)), pieceInfoMigrations, versioning.VersionKey("1"))
	cidInfoMigrations, err := migrations.CIDInfoMigrations.Build()
	if err != nil {
		return nil, err
	}
	cidInfos, migrateCidInfos := versioned.NewVersionedStateStore(namespace.Wrap(ds, datastore.NewKey(DSCIDPrefix)), cidInfoMigrations, versioning.VersionKey("1"))
	return &pieceStore{
		readySub:        pubsub.New(shared.ReadyDispatcher),
		pieces:          pieces,
		migratePieces:   migratePieces,
		cidInfos:        cidInfos,
		migrateCidInfos: migrateCidInfos,
	}, nil
}

type pieceStore struct {
	readySub        *pubsub.PubSub
	migratePieces   func(ctx context.Context) error
	pieces          versioned.StateStore
	migrateCidInfos func(ctx context.Context) error
	cidInfos        versioned.StateStore
}

func (ps *pieceStore) Start(ctx context.Context) error {
	go func() {
		var err error
		defer func() {
			err = ps.readySub.Publish(err)
			if err != nil {
				log.Warnf("Publish piecestore migration ready event: %s", err.Error())
			}
		}()
		err = ps.migratePieces(ctx)
		if err != nil {
			log.Errorf("Migrating pieceInfos: %s", err.Error())
			return
		}
		err = ps.migrateCidInfos(ctx)
		if err != nil {
			log.Errorf("Migrating cidInfos: %s", err.Error())
		}
	}()
	return nil
}

func (ps *pieceStore) OnReady(ready shared.ReadyFunc) {
	ps.readySub.Subscribe(ready)
}

// Store `dealInfo` in the PieceStore with key `pieceCID`.
func (ps *pieceStore) AddDealForPiece(pieceCID cid.Cid, _ cid.Cid, dealInfo piecestore.DealInfo) error {
	return ps.mutatePieceInfo(pieceCID, func(pi *piecestore.PieceInfo) error {
		for _, di := range pi.Deals {
			if di == dealInfo {
				return nil
			}
		}
		pi.Deals = append(pi.Deals, dealInfo)
		return nil
	})
}

// Store the map of blockLocations in the PieceStore's CIDInfo store, with key `pieceCID`
func (ps *pieceStore) AddPieceBlockLocations(pieceCID cid.Cid, blockLocations map[cid.Cid]piecestore.BlockLocation) error {
	for c, blockLocation := range blockLocations {
		err := ps.mutateCIDInfo(c, func(ci *piecestore.CIDInfo) error {
			for _, pbl := range ci.PieceBlockLocations {
				if pbl.PieceCID.Equals(pieceCID) && pbl.BlockLocation == blockLocation {
					return nil
				}
			}
			ci.PieceBlockLocations = append(ci.PieceBlockLocations, piecestore.PieceBlockLocation{BlockLocation: blockLocation, PieceCID: pieceCID})
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (ps *pieceStore) ListPieceInfoKeys() ([]cid.Cid, error) {
	var pis []piecestore.PieceInfo
	if err := ps.pieces.List(&pis); err != nil {
		return nil, err
	}

	out := make([]cid.Cid, 0, len(pis))
	for _, pi := range pis {
		out = append(out, pi.PieceCID)
	}

	return out, nil
}

func (ps *pieceStore) ListCidInfoKeys() ([]cid.Cid, error) {
	var cis []piecestore.CIDInfo
	if err := ps.cidInfos.List(&cis); err != nil {
		return nil, err
	}

	out := make([]cid.Cid, 0, len(cis))
	for _, ci := range cis {
		out = append(out, ci.CID)
	}

	return out, nil
}

// Retrieve the PieceInfo associated with `pieceCID` from the piece info store.
func (ps *pieceStore) GetPieceInfo(pieceCID cid.Cid) (piecestore.PieceInfo, error) {
	var out piecestore.PieceInfo
	if err := ps.pieces.Get(pieceCID).Get(&out); err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			return piecestore.PieceInfo{}, xerrors.Errorf("piece with CID %s: %w", pieceCID, retrievalmarket.ErrNotFound)
		}
		return piecestore.PieceInfo{}, err
	}
	return out, nil
}

// Retrieve the CIDInfo associated with `pieceCID` from the CID info store.
func (ps *pieceStore) GetCIDInfo(payloadCID cid.Cid) (piecestore.CIDInfo, error) {
	var out piecestore.CIDInfo
	if err := ps.cidInfos.Get(payloadCID).Get(&out); err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			return piecestore.CIDInfo{}, xerrors.Errorf("payload CID %s: %w", payloadCID, retrievalmarket.ErrNotFound)
		}
		return piecestore.CIDInfo{}, err
	}
	return out, nil
}

func (ps *pieceStore) ensurePieceInfo(pieceCID cid.Cid) error {
	has, err := ps.pieces.Has(pieceCID)

	if err != nil {
		return err
	}
	if has {
		return nil
	}

	pieceInfo := piecestore.PieceInfo{PieceCID: pieceCID}
	return ps.pieces.Begin(pieceCID, &pieceInfo)
}

func (ps *pieceStore) ensureCIDInfo(c cid.Cid) error {
	has, err := ps.cidInfos.Has(c)

	if err != nil {
		return err
	}

	if has {
		return nil
	}

	cidInfo := piecestore.CIDInfo{CID: c}
	return ps.cidInfos.Begin(c, &cidInfo)
}

func (ps *pieceStore) mutatePieceInfo(pieceCID cid.Cid, mutator interface{}) error {
	err := ps.ensurePieceInfo(pieceCID)
	if err != nil {
		return err
	}

	return ps.pieces.Get(pieceCID).Mutate(mutator)
}

func (ps *pieceStore) mutateCIDInfo(c cid.Cid, mutator interface{}) error {
	err := ps.ensureCIDInfo(c)
	if err != nil {
		return err
	}

	return ps.cidInfos.Get(c).Mutate(mutator)
}

// Package clientutils provides utility functions for the storage client & client FSM
package clientutils

import (
	"context"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-car"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/multiformats/go-multibase"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-commp-utils/writer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// CommP calculates the commP for a given dataref
// In Markets, CommP = PieceCid.
// We can't rely on the CARv1 payload in the given CARv2 file being deterministic as the client could have
// written a "non-deterministic/unordered" CARv2 file.
// So, we need to do a CARv1 traversal here by giving the traverser a random access CARv2 blockstore that wraps the given CARv2 file.
func CommP(ctx context.Context, bs bstore.Blockstore, data *storagemarket.DataRef, maxTraversalLinks uint64) (cid.Cid, abi.UnpaddedPieceSize, error) {
	// if we already have the PieceCid, there's no need to do anything here.
	if data.PieceCid != nil {
		return *data.PieceCid, data.PieceSize, nil
	}

	// It's an error if we don't already have the PieceCid for an offline deal i.e. manual transfer.
	if data.TransferType == storagemarket.TTManual {
		return cid.Undef, 0, xerrors.New("Piece CID and size must be set for manual transfer")
	}
	//
	// if carPath == "" {
	// 	return cid.Undef, 0, xerrors.New("need Carv2 file path to get a read-only blockstore")
	// }

	// // Open a read-only blockstore off the CAR file, wrapped in a filestore so
	// // it can read file positional references.
	// fs, err := stores.ReadOnlyFilestore(carPath)
	// if err != nil {
	// 	return cid.Undef, 0, xerrors.Errorf("failed to open carv2 blockstore: %w", err)
	// }
	// defer fs.Close()

	// do a CARv1 traversal with the DFS selector.
	sc := car.NewSelectiveCar(ctx, bs, []car.Dag{{Root: data.Root, Selector: selectorparse.CommonSelector_ExploreAllRecursively}}, car.MaxTraversalLinks(maxTraversalLinks))
	prepared, err := sc.Prepare()
	if err != nil {
		return cid.Undef, 0, xerrors.Errorf("failed to prepare CAR: %w", err)
	}

	// write out the deterministic CARv1 payload to the CommP writer and calculate the CommP.
	commpWriter := &writer.Writer{}
	err = prepared.Dump(ctx, commpWriter)
	if err != nil {
		return cid.Undef, 0, xerrors.Errorf("failed to write CARv1 to commP writer: %w", err)
	}
	dataCIDSize, err := commpWriter.Sum()
	if err != nil {
		return cid.Undef, 0, xerrors.Errorf("commpWriter.Sum failed: %w", err)
	}

	return dataCIDSize.PieceCID, dataCIDSize.PieceSize.Unpadded(), nil
}

// VerifyFunc is a function that can validate a signature for a given address and bytes
type VerifyFunc func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error)

// VerifyResponse verifies the signature on the given signed response matches
// the given miner address, using the given signature verification function
func VerifyResponse(ctx context.Context, resp network.SignedResponse, minerAddr address.Address, tok shared.TipSetToken, verifier VerifyFunc) error {
	b, err := cborutil.Dump(&resp.Response)
	if err != nil {
		return err
	}
	verified, err := verifier(ctx, *resp.Signature, minerAddr, b, tok)
	if err != nil {
		return err
	}

	if !verified {
		return xerrors.New("could not verify signature")
	}

	return nil
}

// LabelField makes a label field for a deal proposal as a multibase encoding
// of the payload CID (B58BTC for V0, B64 for V1)
func LabelField(payloadCID cid.Cid) (market.DealLabel, error) {
	var cidStr string
	var err error
	if payloadCID.Version() == 0 {
		cidStr, err = payloadCID.StringOfBase(multibase.Base58BTC)
	} else {
		cidStr, err = payloadCID.StringOfBase(multibase.Base64)
	}
	if err != nil {
		return market.EmptyDealLabel, err
	}

	return market.NewLabelFromString(cidStr)
}

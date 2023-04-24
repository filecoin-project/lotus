/*
Package blockrecorder provides utilits to record locations of CIDs to a
temporary metadata file, since writing a CAR happens BEFORE we actually hand off for sealing.
The metadata file is later used to populate the PieceStore
*/
package blockrecorder

import (
	"bufio"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
)

//go:generate cbor-gen-for PieceBlockMetadata

// PieceBlockMetadata is a record of where a given CID lives in a piece,
// in terms of its offset and size
type PieceBlockMetadata struct {
	CID    cid.Cid
	Offset uint64
	Size   uint64
}

// RecordEachBlockTo returns a OnNewCarBlockFunc that records the exact
// location of a given block's data in a CAR file, and writes that data
// to the given writer
func RecordEachBlockTo(out io.Writer) car.OnNewCarBlockFunc {
	return func(block car.Block) error {
		pbMetadata := &PieceBlockMetadata{
			CID:    block.BlockCID,
			Offset: block.Offset + block.Size - uint64(len(block.Data)),
			Size:   uint64(len(block.Data)),
		}
		return pbMetadata.MarshalCBOR(out)
	}
}

// ReadBlockMetadata reads previously recorded block metadata
func ReadBlockMetadata(input io.Reader) ([]PieceBlockMetadata, error) {
	var metadatas []PieceBlockMetadata
	buf := bufio.NewReaderSize(input, 16)
	for {
		var nextMetadata PieceBlockMetadata
		err := nextMetadata.UnmarshalCBOR(buf)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			return metadatas, nil
		}
		metadatas = append(metadatas, nextMetadata)
	}
}

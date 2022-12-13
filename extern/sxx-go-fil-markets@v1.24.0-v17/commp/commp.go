package commp

import (
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-commp-utils/writer"
	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
)

func GenerateCommp(reader io.Reader, payloadSize uint64, targetSize uint64) (cid.Cid, error) {
	// dump the CARv1 payload of the CARv2 file to the Commp Writer and get back the CommP.
	w := &writer.Writer{}
	written, err := io.Copy(w, reader)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to write to CommP writer: %w", err)
	}
	if written != int64(payloadSize) {
		return cid.Undef, xerrors.Errorf("number of bytes written to CommP writer %d not equal to the CARv1 payload size %d", written, payloadSize)
	}

	cidAndSize, err := w.Sum()
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get CommP: %w", err)
	}

	if uint64(cidAndSize.PieceSize) < targetSize {
		// need to pad up!
		rawPaddedCommp, err := commp.PadCommP(
			// we know how long a pieceCid "hash" is, just blindly extract the trailing 32 bytes
			cidAndSize.PieceCID.Hash()[len(cidAndSize.PieceCID.Hash())-32:],
			uint64(cidAndSize.PieceSize),
			uint64(targetSize),
		)
		if err != nil {
			return cid.Undef, err
		}
		cidAndSize.PieceCID, _ = commcid.DataCommitmentV1ToCID(rawPaddedCommp)
	}

	return cidAndSize.PieceCID, err
}

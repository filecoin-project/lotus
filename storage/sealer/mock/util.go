package mock

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/v2/zerocomm"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/genesis"
)

func CommDR(in []byte) (out [32]byte) {
	for i, b := range in {
		out[i] = ^b
	}

	return out
}

func PreSeal(spt abi.RegisteredSealProof, maddr address.Address, sectors int) (*genesis.Miner, *types.KeyInfo, error) {
	k, err := key.GenerateKey(types.KTBLS)
	if err != nil {
		return nil, nil, err
	}

	ssize, err := spt.SectorSize()
	if err != nil {
		return nil, nil, err
	}

	genm := &genesis.Miner{
		ID:            maddr,
		Owner:         k.Address,
		Worker:        k.Address,
		MarketBalance: big.NewInt(0),
		PowerBalance:  big.NewInt(0),
		SectorSize:    ssize,
		Sectors:       make([]*genesis.PreSeal, sectors),
	}

	for i := range genm.Sectors {
		label, err := market.NewLabelFromString(fmt.Sprintf("%d", i))
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to create label: %w", err)
		}

		preseal := &genesis.PreSeal{}

		preseal.ProofType = spt
		preseal.CommD = zerocomm.ZeroPieceCommitment(abi.PaddedPieceSize(ssize).Unpadded())
		d, _ := commcid.CIDToPieceCommitmentV1(preseal.CommD)
		r := CommDR(d)
		preseal.CommR, _ = commcid.ReplicaCommitmentV1ToCID(r[:])
		preseal.SectorID = abi.SectorNumber(i + 1)
		preseal.Deal = market.DealProposal{
			PieceCID:             preseal.CommD,
			PieceSize:            abi.PaddedPieceSize(ssize),
			Client:               k.Address,
			Provider:             maddr,
			Label:                label,
			StartEpoch:           1,
			EndEpoch:             10000,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		}
		preseal.DealClientKey = k.KeyInfo

		genm.Sectors[i] = preseal
	}

	return genm, &k.KeyInfo, nil
}

// ToReadableFile generates an os readable file from an io.Reader (converts via pipe & copy)
// This method is FOR TEST PURPOSES ONLY and has been copied verbatim from the legacy
// https://pkg.go.dev/github.com/filecoin-project/go-commp-utils@v0.1.4/ffiwrapper
func ToReadableFile(r io.Reader, n int64) (*os.File, func() error, error) {
	f, ok := r.(*os.File)
	if ok {
		return f, func() error { return nil }, nil
	}

	var w *os.File

	f, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	var wait sync.Mutex
	var werr error

	wait.Lock()
	go func() {
		defer wait.Unlock()

		var copied int64
		copied, werr = io.CopyN(w, r, n)
		if werr != nil {
			log.Warnf("toReadableFile: copy error: %+v", werr)
		}

		err := w.Close()
		if werr == nil && err != nil {
			werr = err
			log.Warnf("toReadableFile: close error: %+v", err)
			return
		}
		if copied != n {
			log.Warnf("copied different amount than expected: %d != %d", copied, n)
			werr = xerrors.Errorf("copied different amount than expected: %d != %d", copied, n)
		}
	}()

	return f, func() error {
		wait.Lock()
		return werr
	}, nil
}

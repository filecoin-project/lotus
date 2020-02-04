package sealing

import (
	"io"
	"math/bits"
	"math/rand"
	"sync"

	"github.com/hashicorp/go-multierror"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
)

func fillersFromRem(toFill uint64) ([]uint64, error) {
	// Convert to in-sector bytes for easier math:
	//
	// Sector size to user bytes ratio is constant, e.g. for 1024B we have 1016B
	// of user-usable data.
	//
	// (1024/1016 = 128/127)
	//
	// Given that we can get sector size by simply adding 1/127 of the user
	// bytes
	//
	// (we convert to sector bytes as they are nice round binary numbers)

	toFill += toFill / 127

	// We need to fill the sector with pieces that are powers of 2. Conveniently
	// computers store numbers in binary, which means we can look at 1s to get
	// all the piece sizes we need to fill the sector. It also means that number
	// of pieces is the number of 1s in the number of remaining bytes to fill
	out := make([]uint64, bits.OnesCount64(toFill))
	for i := range out {
		// Extract the next lowest non-zero bit
		next := bits.TrailingZeros64(toFill)
		psize := uint64(1) << next
		// e.g: if the number is 0b010100, psize will be 0b000100

		// set that bit to 0 by XORing it, so the next iteration looks at the
		// next bit
		toFill ^= psize

		// Add the piece size to the list of pieces we need to create
		out[i] = sectorbuilder.UserBytesForSectorSize(psize)
	}
	return out, nil
}

func (m *Sealing) fastPledgeCommitment(size uint64, parts uint64) (commP [sectorbuilder.CommLen]byte, err error) {
	parts = 1 << bits.Len64(parts) // round down to nearest power of 2
	if size/parts < 127 {
		parts = size / 127
	}

	piece := sectorbuilder.UserBytesForSectorSize((size + size/127) / parts)
	out := make([]sectorbuilder.PublicPieceInfo, parts)
	var lk sync.Mutex

	var wg sync.WaitGroup
	wg.Add(int(parts))
	for i := uint64(0); i < parts; i++ {
		go func(i uint64) {
			defer wg.Done()

			commP, perr := sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42+int64(i))), int64(piece)), piece)

			lk.Lock()
			if perr != nil {
				err = multierror.Append(err, perr)
			}
			out[i] = sectorbuilder.PublicPieceInfo{
				Size:  piece,
				CommP: commP,
			}
			lk.Unlock()
		}(i)
	}
	wg.Wait()

	if err != nil {
		return [32]byte{}, err
	}

	return sectorbuilder.GenerateDataCommitment(m.sb.SectorSize(), out)
}

func (m *Sealing) ListSectors() ([]SectorInfo, error) {
	var sectors []SectorInfo
	if err := m.sectors.List(&sectors); err != nil {
		return nil, err
	}
	return sectors, nil
}

func (m *Sealing) GetSectorInfo(sid uint64) (SectorInfo, error) {
	var out SectorInfo
	err := m.sectors.Get(sid).Get(&out)
	return out, err
}

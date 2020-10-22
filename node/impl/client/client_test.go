package client

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/zerocomm"
)

func TestClientDealPieceCIDZero(t *testing.T) {
	for i, s := range []struct {
		writes []int
		expect abi.PaddedPieceSize
	}{
		{writes: []int{200}, expect: 256},
		{writes: []int{200, 200}, expect: 512},

		{writes: []int{int(commPBuf)}, expect: commPBufPad},
		{writes: []int{int(commPBuf) * 2}, expect: 2 * commPBufPad},
		{writes: []int{int(commPBuf), int(commPBuf), int(commPBuf)}, expect: 4 * commPBufPad},
		{writes: []int{int(commPBuf), int(commPBuf), int(commPBuf), int(commPBuf), int(commPBuf), int(commPBuf), int(commPBuf), int(commPBuf), int(commPBuf)}, expect: 16 * commPBufPad},

		{writes: []int{200, int(commPBuf)}, expect: 2 * commPBufPad},
	} {
		s := s
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			w := &commPWriter{}
			var rawSum int64
			for _, write := range s.writes {
				rawSum += int64(write)
				_, err := w.Write(make([]byte, write))
				require.NoError(t, err)
			}

			p, err := w.Sum()
			require.NoError(t, err)
			require.Equal(t, rawSum, p.PayloadSize)
			require.Equal(t, s.expect, p.PieceSize)
			require.Equal(t, zerocomm.ZeroPieceCommitment(s.expect.Unpadded()).String(), p.PieceCID.String())
		})
	}
}

func TestClientDealPieceCIDData(t *testing.T) {
	dataLen := float64(commPBuf) * 6.78
	data, _ := ioutil.ReadAll(io.LimitReader(rand.Reader, int64(dataLen)))

	pr, sz := padreader.New(bytes.NewReader(data), uint64(dataLen))
	exp, err := ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg32GiBV1, pr, sz)
	require.NoError(t, err)

	w := &commPWriter{}
	_, err = io.Copy(w, bytes.NewReader(data))
	require.NoError(t, err)

	res, err := w.Sum()
	require.NoError(t, err)

	require.Equal(t, exp.String(), res.PieceCID.String())
}

func BenchmarkClientDealPieceCIDZero(b *testing.B) {
	buf := make([]byte, int(commPBuf)*b.N)
	b.SetBytes(int64(commPBuf))
	b.ResetTimer()

	w := &commPWriter{}

	_, err := w.Write(buf)
	require.NoError(b, err)
	o, err := w.Sum()

	b.StopTimer()

	require.NoError(b, err)
	require.Equal(b, zerocomm.ZeroPieceCommitment(o.PieceSize.Unpadded()).String(), o.PieceCID.String())
	require.Equal(b, int64(commPBuf)*int64(b.N), o.PayloadSize)
}

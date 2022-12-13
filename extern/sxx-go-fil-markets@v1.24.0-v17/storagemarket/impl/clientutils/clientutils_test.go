package clientutils_test

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-car"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/filecoin-project/go-fil-markets/stores"
)

func TestCommP(t *testing.T) {
	ctx := context.Background()

	t.Run("when PieceCID is already present on data ref", func(t *testing.T) {
		pieceCid := &shared_testutil.GenerateCids(1)[0]
		pieceSize := abi.UnpaddedPieceSize(rand.Uint64())
		data := &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			PieceCid:     pieceCid,
			PieceSize:    pieceSize,
		}
		respcid, ressize, err := clientutils.CommP(ctx, nil, data, 2<<29)
		require.NoError(t, err)
		require.Equal(t, respcid, *pieceCid)
		require.Equal(t, ressize, pieceSize)
	})

	genCommp := func(t *testing.T, ctx context.Context, root cid.Cid, bs bstore.Blockstore) cid.Cid {
		data := &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         root,
		}

		respcid, _, err := clientutils.CommP(ctx, bs, data, 2<<29)
		require.NoError(t, err)
		require.NotEqual(t, respcid, cid.Undef)
		return respcid
	}

	t.Run("when PieceCID needs to be generated", func(t *testing.T) {
		file1 := filepath.Join(shared_testutil.ThisDir(t), "../../fixtures/payload.txt")
		file2 := filepath.Join(shared_testutil.ThisDir(t), "../../fixtures/payload2.txt")

		var commP [][]cid.Cid
		for _, f := range []string{file1, file2} {
			rootFull, pathFull := shared_testutil.CreateDenseCARv2(t, f)
			rootFilestore, pathFilestore := shared_testutil.CreateRefCARv2(t, f)

			// assert the two files have different contents, but the same DAG root.
			assertFilesDiffer(t, pathFull, pathFilestore)
			require.Equal(t, rootFull, rootFilestore)

			bsFull, err := blockstore.OpenReadOnly(pathFull, blockstore.UseWholeCIDs(true))
			require.NoError(t, err)
			t.Cleanup(func() { bsFull.Close() })

			bsFilestore, err := blockstore.OpenReadOnly(pathFull, blockstore.UseWholeCIDs(true))
			require.NoError(t, err)
			t.Cleanup(func() { bsFilestore.Close() })

			fsFilestore, err := stores.FilestoreOf(bsFilestore)

			// commPs match for both since it's the same unixfs DAG.
			commpFull := genCommp(t, ctx, rootFull, bsFull)
			commpFilestore := genCommp(t, ctx, rootFilestore, fsFilestore)
			require.EqualValues(t, commpFull, commpFilestore)

			commP = append(commP, []cid.Cid{commpFull, commpFilestore})
		}

		// commP's are different across different files/DAGs.
		require.NotEqualValues(t, commP[0][0], commP[1][0])
		require.NotEqualValues(t, commP[0][1], commP[1][1])
	})
}

func TestLabelField(t *testing.T) {
	payloadCID := shared_testutil.GenerateCids(1)[0]
	label, err := clientutils.LabelField(payloadCID)
	require.NoError(t, err)
	labelStr, err := label.ToString()
	require.NoError(t, err)
	resultCid, err := cid.Decode(labelStr)
	require.NoError(t, err)
	require.True(t, payloadCID.Equals(resultCid))
}

// this test doesn't belong here, it should be a unit test in CARv2, but we can
// retain as a sentinel test.
// TODO maybe remove and trust that CARv2 behaves well.
func TestNoDuplicatesInCARv2(t *testing.T) {
	// The CARv2 file for a UnixFS DAG that has duplicates should NOT have duplicates.
	file1 := filepath.Join(shared_testutil.ThisDir(t), "../../fixtures/duplicate_blocks.txt")
	_, path := shared_testutil.CreateDenseCARv2(t, file1)
	require.NotEmpty(t, path)
	defer os.Remove(path)

	v2r, err := carv2.OpenReader(path)
	require.NoError(t, err)
	defer v2r.Close()
	v2rDataReader, err := v2r.DataReader()
	require.NoError(t, err)

	// Get a reader over the CARv1 payload of the CARv2 file.
	cr, err := car.NewCarReader(v2rDataReader)
	require.NoError(t, err)

	seen := make(map[cid.Cid]struct{})
	for {
		b, err := cr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		_, ok := seen[b.Cid()]
		require.Falsef(t, ok, "already seen cid %s", b.Cid())
		seen[b.Cid()] = struct{}{}
	}
}

func assertFilesDiffer(t *testing.T, f1Path string, f2Path string) {
	f1, err := os.Open(f1Path)
	require.NoError(t, err)
	defer f1.Close()

	f2, err := os.Open(f2Path)
	require.NoError(t, err)
	defer f2.Close()

	bzf1, err := ioutil.ReadAll(f1)
	require.NoError(t, err)

	bzf2, err := ioutil.ReadAll(f2)
	require.NoError(t, err)

	require.NotEqualValues(t, bzf1, bzf2)
}

package ffiwrapper

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/filecoin-ffi/cgo"
	"github.com/filecoin-project/go-commp-utils/v2"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/storage/pipeline/lib/nullreader"
	"github.com/filecoin-project/lotus/storage/sealer/commitment"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func init() {
	_ = logging.SetLogLevel("*", "DEBUG")
}

var sealProofType = abi.RegisteredSealProof_StackedDrg2KiBV1
var sectorSize, _ = sealProofType.SectorSize()

var sealRand = abi.SealRandomness{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2}

type seal struct {
	ref    storiface.SectorRef
	cids   storiface.SectorCids
	pi     abi.PieceInfo
	ticket abi.SealRandomness
}

func data(sn abi.SectorNumber, dlen abi.UnpaddedPieceSize) io.Reader {
	return io.MultiReader(
		io.LimitReader(rand.New(rand.NewSource(42+int64(sn))), int64(123)),
		io.LimitReader(rand.New(rand.NewSource(42+int64(sn))), int64(dlen-123)),
	)
}

func (s *seal) precommit(t *testing.T, sb *Sealer, id storiface.SectorRef, done func()) {
	defer done()
	dlen := abi.PaddedPieceSize(sectorSize).Unpadded()

	var err error
	r := data(id.ID.Number, dlen)
	s.pi, err = sb.AddPiece(context.TODO(), id, []abi.UnpaddedPieceSize{}, dlen, r)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}

	s.ticket = sealRand

	p1, err := sb.SealPreCommit1(context.TODO(), id, s.ticket, []abi.PieceInfo{s.pi})
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	cids, err := sb.SealPreCommit2(context.TODO(), id, p1)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	s.cids = cids
}

var seed = abi.InteractiveSealRandomness{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9}

func (s *seal) commit(t *testing.T, sb *Sealer, done func()) storiface.Proof {
	defer done()

	pc1, err := sb.SealCommit1(context.TODO(), s.ref, s.ticket, seed, []abi.PieceInfo{s.pi}, s.cids)
	if err != nil {
		t.Errorf("%+v", err)
		return nil
	}
	proof, err := sb.SealCommit2(context.TODO(), s.ref, pc1)
	if err != nil {
		t.Errorf("%+v", err)
		return nil
	}

	ok, err := ProofVerifier.VerifySeal(prooftypes.SealVerifyInfo{
		SectorID:              s.ref.ID,
		SealedCID:             s.cids.Sealed,
		SealProof:             s.ref.ProofType,
		Proof:                 proof,
		Randomness:            s.ticket,
		InteractiveRandomness: seed,
		UnsealedCID:           s.cids.Unsealed,
	})
	if err != nil {
		t.Errorf("%+v", err)
		return nil
	}

	if !ok {
		t.Errorf("proof failed to validate")
		return nil
	}

	return proof
}

func (s *seal) unseal(t *testing.T, sb *Sealer, sp *basicfs.Provider, si storiface.SectorRef, done func()) {
	defer done()

	var b bytes.Buffer
	_, err := sb.ReadPiece(context.TODO(), &b, si, 0, 1016)
	if err != nil {
		t.Fatal(err)
	}

	expect, _ := io.ReadAll(data(si.ID.Number, 1016))
	if !bytes.Equal(b.Bytes(), expect) {
		t.Fatal("read wrong bytes")
	}

	p, sd, err := sp.AcquireSector(context.TODO(), si, storiface.FTUnsealed, storiface.FTNone, storiface.PathStorage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p.Unsealed); err != nil {
		t.Fatal(err)
	}
	sd()

	_, err = sb.ReadPiece(context.TODO(), &b, si, 0, 1016)
	if err == nil {
		t.Fatal("HOW?!")
	}
	log.Info("this is what we expect: ", err)

	if err := sb.UnsealPiece(context.TODO(), si, 0, 1016, sealRand, s.cids.Unsealed); err != nil {
		t.Fatal(err)
	}

	b.Reset()
	_, err = sb.ReadPiece(context.TODO(), &b, si, 0, 1016)
	if err != nil {
		t.Fatal(err)
	}

	expect, _ = io.ReadAll(data(si.ID.Number, 1016))
	require.Equal(t, expect, b.Bytes())

	b.Reset()
	have, err := sb.ReadPiece(context.TODO(), &b, si, 0, 2032)
	if err != nil {
		t.Fatal(err)
	}

	if have {
		t.Errorf("didn't expect to read things")
	}

	if b.Len() != 0 {
		t.Fatal("read bytes")
	}
}

func post(t *testing.T, sealer *Sealer, skipped []abi.SectorID, seals ...seal) {
	randomness := abi.PoStRandomness{0, 9, 2, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 7}

	xsis := make([]prooftypes.ExtendedSectorInfo, len(seals))
	for i, s := range seals {
		xsis[i] = prooftypes.ExtendedSectorInfo{
			SealProof:    s.ref.ProofType,
			SectorNumber: s.ref.ID.Number,
			SealedCID:    s.cids.Sealed,
		}
	}

	ppt, err := xsis[0].SealProof.RegisteredWindowPoStProof()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ppt, err = ppt.ToV1_1PostProof()
	if err != nil {

		t.Fatalf("%+v", err)
	}

	proofs, skp, err := sealer.GenerateWindowPoSt(context.TODO(), seals[0].ref.ID.Miner, ppt, xsis, randomness)
	if len(skipped) > 0 {
		require.Error(t, err)
		require.EqualValues(t, skipped, skp)
		return
	}

	if err != nil {
		t.Fatalf("%+v", err)
	}

	sis := make([]prooftypes.SectorInfo, len(seals))
	for i, xsi := range xsis {
		sis[i] = prooftypes.SectorInfo{
			SealProof:    xsi.SealProof,
			SectorNumber: xsi.SectorNumber,
			SealedCID:    xsi.SealedCID,
		}
	}

	ok, err := ProofVerifier.VerifyWindowPoSt(context.TODO(), prooftypes.WindowPoStVerifyInfo{
		Randomness:        randomness,
		Proofs:            proofs,
		ChallengedSectors: sis,
		Prover:            seals[0].ref.ID.Miner,
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if !ok {
		t.Fatal("bad post")
	}
}

func corrupt(t *testing.T, sealer *Sealer, id storiface.SectorRef) {
	paths, done, err := sealer.sectors.AcquireSector(context.Background(), id, storiface.FTSealed, 0, storiface.PathStorage)
	require.NoError(t, err)
	defer done()

	log.Infof("corrupt %s", paths.Sealed)
	f, err := os.OpenFile(paths.Sealed, os.O_RDWR, 0664)
	require.NoError(t, err)

	_, err = f.WriteAt(bytes.Repeat([]byte{'d'}, 2048), 0)
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func getGrothParamFileAndVerifyingKeys(s abi.SectorSize) {
	dat, err := os.ReadFile("../../../build/proof-params/parameters.json")
	if err != nil {
		panic(err)
	}

	datSrs, err := os.ReadFile("../../../build/proof-params/srs-inner-product.json")
	if err != nil {
		panic(err)
	}

	err = paramfetch.GetParams(context.TODO(), dat, datSrs, uint64(s))
	if err != nil {
		panic(xerrors.Errorf("failed to acquire Groth parameters for 2KiB sectors: %w", err))
	}
}

// TestDownloadParams exists only so that developers and CI can pre-download
// Groth parameters and verifying keys before running the tests which rely on
// those parameters and keys. To do this, run the following command:
//
// go test -run=^TestDownloadParams
func TestDownloadParams(t *testing.T) {
	getGrothParamFileAndVerifyingKeys(sectorSize)
}

func TestSealAndVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	getGrothParamFileAndVerifyingKeys(sectorSize)

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	if err != nil {
		t.Fatal(err)
	}
	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			t.Error(err)
		}
	})

	si := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	s := seal{ref: si}

	start := time.Now()

	s.precommit(t, sb, si, func() {})

	precommit := time.Now()

	s.commit(t, sb, func() {})

	commit := time.Now()

	post(t, sb, nil, s)

	epost := time.Now()

	post(t, sb, nil, s)

	if err := sb.FinalizeSector(context.TODO(), si); err != nil {
		t.Fatalf("%+v", err)
	}

	s.unseal(t, sb, sp, si, func() {})

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("Commit: %s\n", commit.Sub(precommit).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(commit).String())
}

func TestSealPoStNoCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	getGrothParamFileAndVerifyingKeys(sectorSize)

	dir, err := os.MkdirTemp("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: dir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	t.Cleanup(func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", dir)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})

	si := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	s := seal{ref: si}

	start := time.Now()

	s.precommit(t, sb, si, func() {})

	precommit := time.Now()

	if err := sb.FinalizeSector(context.TODO(), si); err != nil {
		t.Fatal(err)
	}

	post(t, sb, nil, s)

	epost := time.Now()

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(precommit).String())
}

func TestMain(m *testing.M) {
	//setup()
	// Here it no-longer is bound to 30s but has 1m30s for the whole suite.
	getGrothParamFileAndVerifyingKeys(sectorSize)

	code := m.Run()
	//shutdown()
	os.Exit(code)
}

func TestSealAndVerify3(t *testing.T) {
	t.Skip("i flake on CI, re-enable me when you have a fix pls")
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "trace")

	dir, err := os.MkdirTemp("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: dir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})

	var wg sync.WaitGroup

	si1 := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}
	si2 := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 2},
		ProofType: sealProofType,
	}
	si3 := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 3},
		ProofType: sealProofType,
	}

	s1 := seal{ref: si1}
	s2 := seal{ref: si2}
	s3 := seal{ref: si3}

	wg.Add(3)
	go s1.precommit(t, sb, si1, wg.Done)
	time.Sleep(100 * time.Millisecond)
	go s2.precommit(t, sb, si2, wg.Done)
	time.Sleep(100 * time.Millisecond)
	go s3.precommit(t, sb, si3, wg.Done)
	wg.Wait()

	wg.Add(3)
	go s1.commit(t, sb, wg.Done)
	go s2.commit(t, sb, wg.Done)
	go s3.commit(t, sb, wg.Done)
	wg.Wait()

	post(t, sb, nil, s1, s2, s3)

	corrupt(t, sb, si1)
	corrupt(t, sb, si2)

	post(t, sb, []abi.SectorID{si1.ID, si2.ID}, s1, s2, s3)
}

func TestSealAndVerifyAggregate(t *testing.T) {
	numAgg := 5

	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	getGrothParamFileAndVerifyingKeys(sectorSize)

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	if err != nil {
		t.Fatal(err)
	}
	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			t.Error(err)
		}
	})

	avi := prooftypes.AggregateSealVerifyProofAndInfos{
		Miner:          miner,
		SealProof:      sealProofType,
		AggregateProof: policy.GetDefaultAggregationProof(),
		Proof:          nil,
		Infos:          make([]prooftypes.AggregateSealVerifyInfo, numAgg),
	}

	toAggregate := make([][]byte, numAgg)
	for i := 0; i < numAgg; i++ {
		si := storiface.SectorRef{
			ID:        abi.SectorID{Miner: miner, Number: abi.SectorNumber(i + 1)},
			ProofType: sealProofType,
		}

		s := seal{ref: si}
		s.precommit(t, sb, si, func() {})
		toAggregate[i] = s.commit(t, sb, func() {})

		avi.Infos[i] = prooftypes.AggregateSealVerifyInfo{
			Number:                abi.SectorNumber(i + 1),
			Randomness:            s.ticket,
			InteractiveRandomness: seed,
			SealedCID:             s.cids.Sealed,
			UnsealedCID:           s.cids.Unsealed,
		}
	}

	aggStart := time.Now()

	avi.Proof, err = ProofProver.AggregateSealProofs(avi, toAggregate)
	require.NoError(t, err)
	require.Len(t, avi.Proof, 11188)

	aggDone := time.Now()

	_, err = ProofProver.AggregateSealProofs(avi, toAggregate)
	require.NoError(t, err)

	aggHot := time.Now()

	ok, err := ProofVerifier.VerifyAggregateSeals(avi)
	require.NoError(t, err)
	require.True(t, ok)

	verifDone := time.Now()

	fmt.Printf("Aggregate: %s\n", aggDone.Sub(aggStart).String())
	fmt.Printf("Hot: %s\n", aggHot.Sub(aggDone).String())
	fmt.Printf("Verify: %s\n", verifDone.Sub(aggHot).String())
}

func BenchmarkWriteWithAlignment(b *testing.B) {
	bt := abi.UnpaddedPieceSize(2 * 127 * 1024 * 1024)
	b.SetBytes(int64(bt))

	for b.Loop() {
		b.StopTimer()
		rf, w, _ := mock.ToReadableFile(bytes.NewReader(bytes.Repeat([]byte{0xff, 0}, int(bt/2))), int64(bt))
		tf, _ := os.CreateTemp("/tmp/", "scrb-")
		b.StartTimer()

		ffi.WriteWithAlignment(abi.RegisteredSealProof_StackedDrg2KiBV1, rf, bt, tf, nil) // nolint:errcheck
		_ = w()
	}
}

func TestGenerateUnsealedCID(t *testing.T) {
	pt := abi.RegisteredSealProof_StackedDrg2KiBV1
	ups := int(abi.PaddedPieceSize(2048).Unpadded())

	commP := func(b []byte) cid.Cid {
		pf, werr, err := mock.ToReadableFile(bytes.NewReader(b), int64(len(b)))
		require.NoError(t, err)

		c, err := commp.GeneratePieceCIDFromFile(pt, pf, abi.UnpaddedPieceSize(len(b)))
		require.NoError(t, err)

		require.NoError(t, werr())

		return c
	}

	testCommEq := func(name string, in [][]byte, expect [][]byte) {
		t.Run(name, func(t *testing.T) {
			upi := make([]abi.PieceInfo, len(in))
			for i, b := range in {
				upi[i] = abi.PieceInfo{
					Size:     abi.UnpaddedPieceSize(len(b)).Padded(),
					PieceCID: commP(b),
				}
			}

			sectorPi := []abi.PieceInfo{
				{
					Size:     2048,
					PieceCID: commP(bytes.Join(expect, nil)),
				},
			}

			expectCid, err := proofs.GenerateUnsealedCID(pt, sectorPi)
			require.NoError(t, err)

			actualCid, err := proofs.GenerateUnsealedCID(pt, upi)
			require.NoError(t, err)

			require.Equal(t, expectCid, actualCid)
		})
	}

	barr := func(b byte, den int) []byte {
		return bytes.Repeat([]byte{b}, ups/den)
	}

	// 0000
	testCommEq("zero",
		nil,
		[][]byte{barr(0, 1)},
	)

	// 1111
	testCommEq("one",
		[][]byte{barr(1, 1)},
		[][]byte{barr(1, 1)},
	)

	// 11 00
	testCommEq("one|2",
		[][]byte{barr(1, 2)},
		[][]byte{barr(1, 2), barr(0, 2)},
	)

	// 1 0 00
	testCommEq("one|4",
		[][]byte{barr(1, 4)},
		[][]byte{barr(1, 4), barr(0, 4), barr(0, 2)},
	)

	// 11 2 0
	testCommEq("one|2-two|4",
		[][]byte{barr(1, 2), barr(2, 4)},
		[][]byte{barr(1, 2), barr(2, 4), barr(0, 4)},
	)

	// 1 0 22
	testCommEq("one|4-two|2",
		[][]byte{barr(1, 4), barr(2, 2)},
		[][]byte{barr(1, 4), barr(0, 4), barr(2, 2)},
	)

	// 1 0 22 0000
	testCommEq("one|8-two|4",
		[][]byte{barr(1, 8), barr(2, 4)},
		[][]byte{barr(1, 8), barr(0, 8), barr(2, 4), barr(0, 2)},
	)

	// 11 2 0 0000
	testCommEq("one|4-two|8",
		[][]byte{barr(1, 4), barr(2, 8)},
		[][]byte{barr(1, 4), barr(2, 8), barr(0, 8), barr(0, 2)},
	)

	// 1 0 22 3 0 00 4444 5 0 00
	testCommEq("one|16-two|8-three|16-four|4-five|16",
		[][]byte{barr(1, 16), barr(2, 8), barr(3, 16), barr(4, 4), barr(5, 16)},
		[][]byte{barr(1, 16), barr(0, 16), barr(2, 8), barr(3, 16), barr(0, 16), barr(0, 8), barr(4, 4), barr(5, 16), barr(0, 16), barr(0, 8)},
	)
}

func TestAddPiece512M(t *testing.T) {
	sz := abi.PaddedPieceSize(512 << 20).Unpadded()

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	if err != nil {
		t.Fatal(err)
	}
	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	cleanup := func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(cleanup)

	r := rand.New(rand.NewSource(0x7e5))

	c, err := sb.AddPiece(context.TODO(), storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  miner,
			Number: 0,
		},
		ProofType: abi.RegisteredSealProof_StackedDrg512MiBV1_1,
	}, nil, sz, io.LimitReader(r, int64(sz)))
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "baga6ea4seaqhyticusemlcrjhvulpfng4nint6bu3wpe5s3x4bnuj2rs47hfacy", c.PieceCID.String())
}

func BenchmarkAddPiece512M(b *testing.B) {
	sz := abi.PaddedPieceSize(512 << 20).Unpadded()
	b.SetBytes(int64(sz))

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	if err != nil {
		b.Fatal(err)
	}
	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	cleanup := func() {
		if b.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			b.Error(err)
		}
	}
	b.Cleanup(cleanup)

	for b.Loop() {
		c, err := sb.AddPiece(context.TODO(), storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  miner,
				Number: abi.SectorNumber(b.N),
			},
			ProofType: abi.RegisteredSealProof_StackedDrg512MiBV1_1,
		}, nil, sz, io.LimitReader(&nullreader.Reader{}, int64(sz)))
		if err != nil {
			b.Fatal(err)
		}
		fmt.Println(c)
	}
}

func TestAddPiece512MPadded(t *testing.T) {
	sz := abi.PaddedPieceSize(512 << 20).Unpadded()

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	if err != nil {
		t.Fatal(err)
	}
	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	cleanup := func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(cleanup)

	r := rand.New(rand.NewSource(0x7e5))

	c, err := sb.AddPiece(context.TODO(), storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  miner,
			Number: 0,
		},
		ProofType: abi.RegisteredSealProof_StackedDrg512MiBV1_1,
	}, nil, sz, io.LimitReader(r, int64(sz/4)))
	if err != nil {
		t.Fatalf("add piece failed: %s", err)
	}

	require.Equal(t, "baga6ea4seaqonenxyku4o7hr5xkzbqsceipf6xgli3on54beqbk6k246sbooobq", c.PieceCID.String())
}

func setupLogger(t *testing.T) *bytes.Buffer {
	_ = os.Setenv("RUST_LOG", "info")

	var bb bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_, _ = io.Copy(&bb, r)
		runtime.KeepAlive(w)
	}()

	err = cgo.InitLogFd(int32(w.Fd()))
	require.NoError(t, err)

	return &bb
}

func TestMulticoreSDR(t *testing.T) {
	if os.Getenv("TEST_RUSTPROOFS_LOGS") != "1" {
		t.Skip("skipping test without TEST_RUSTPROOFS_LOGS=1")
	}

	rustLogger := setupLogger(t)

	getGrothParamFileAndVerifyingKeys(sectorSize)

	dir, err := os.MkdirTemp("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: dir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	t.Cleanup(func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", dir)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})

	si := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	s := seal{ref: si}

	// check multicore
	_ = os.Setenv("FIL_PROOFS_USE_MULTICORE_SDR", "1")
	rustLogger.Reset()
	s.precommit(t, sb, si, func() {})

	ok := false
	for _, s := range strings.Split(rustLogger.String(), "\n") {
		if strings.Contains(s, "create_label::multi") {
			ok = true
			break
		}
	}

	require.True(t, ok)
}

func TestPoStChallengeAssumptions(t *testing.T) {
	var r [32]byte
	if _, err := crand.Read(r[:]); err != nil {
		panic(err)
	}
	r[31] &= 0x3f

	// behaves like a pure function
	{
		c1, err := ffi.GeneratePoStFallbackSectorChallenges(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 1000, r[:], []abi.SectorNumber{1, 2, 3, 4})
		require.NoError(t, err)

		c2, err := ffi.GeneratePoStFallbackSectorChallenges(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 1000, r[:], []abi.SectorNumber{1, 2, 3, 4})
		require.NoError(t, err)

		require.Equal(t, c1, c2)
	}

	// doesn't sort, challenges position dependent
	{
		c1, err := ffi.GeneratePoStFallbackSectorChallenges(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 1000, r[:], []abi.SectorNumber{1, 2, 3, 4})
		require.NoError(t, err)

		c2, err := ffi.GeneratePoStFallbackSectorChallenges(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 1000, r[:], []abi.SectorNumber{4, 2, 3, 1})
		require.NoError(t, err)

		require.NotEqual(t, c1, c2)

		require.Equal(t, c1.Challenges[2], c2.Challenges[2])
		require.Equal(t, c1.Challenges[3], c2.Challenges[3])

		require.NotEqual(t, c1.Challenges[1], c2.Challenges[1])
		require.NotEqual(t, c1.Challenges[4], c2.Challenges[4])
	}

	// length doesn't matter
	{
		c1, err := ffi.GeneratePoStFallbackSectorChallenges(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 1000, r[:], []abi.SectorNumber{1})
		require.NoError(t, err)

		c2, err := ffi.GeneratePoStFallbackSectorChallenges(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 1000, r[:], []abi.SectorNumber{1, 2})
		require.NoError(t, err)

		require.NotEqual(t, c1, c2)
		require.Equal(t, c1.Challenges[1], c2.Challenges[1])
	}

	// generate dedupes
	{
		c1, err := ffi.GeneratePoStFallbackSectorChallenges(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 1000, r[:], []abi.SectorNumber{1, 2, 1, 4})
		require.NoError(t, err)
		require.Len(t, c1.Sectors, 3)
		require.Len(t, c1.Challenges, 3)
	}
}

func TestDCAPCloses(t *testing.T) {
	sz := abi.PaddedPieceSize(2 << 10).Unpadded()

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	if err != nil {
		t.Fatal(err)
	}
	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	cleanup := func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(cleanup)

	t.Run("DataCid", func(t *testing.T) {
		r := rand.New(rand.NewSource(0x7e5))

		clr := &closeAssertReader{
			Reader: io.LimitReader(r, int64(sz)),
		}

		c, err := sb.DataCid(context.TODO(), sz, clr)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, "baga6ea4seaqeje7jy4hufnybpo7ckxzujaigqbcxhdjq7ojb4b6xzgqdugkyciq", c.PieceCID.String())
		require.True(t, clr.closed)
	})

	t.Run("AddPiece", func(t *testing.T) {
		r := rand.New(rand.NewSource(0x7e5))

		clr := &closeAssertReader{
			Reader: io.LimitReader(r, int64(sz)),
		}

		c, err := sb.AddPiece(context.TODO(), storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  miner,
				Number: 0,
			},
			ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1_1,
		}, nil, sz, clr)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, "baga6ea4seaqeje7jy4hufnybpo7ckxzujaigqbcxhdjq7ojb4b6xzgqdugkyciq", c.PieceCID.String())
		require.True(t, clr.closed)
	})
}

func TestSealAndVerifySynth(t *testing.T) {
	origSealProofType := sealProofType
	sealProofType = abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep
	t.Cleanup(func() {
		sealProofType = origSealProofType
	})

	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	getGrothParamFileAndVerifyingKeys(sectorSize)

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	if err != nil {
		t.Fatal(err)
	}
	miner := abi.ActorID(123)

	synthPorRepVProofsName := "syn-porep-vanilla-proofs.dat"

	printFileList := func(stage string, expectSynthPorep bool) {
		var hasSynthPorep bool

		fmt.Println("----file list:", stage)
		err := filepath.Walk(cdir, func(path string, info os.FileInfo, err error) error {
			if strings.Contains(path, synthPorRepVProofsName) {
				hasSynthPorep = true
			}
			fmt.Println(path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, expectSynthPorep, hasSynthPorep)

		fmt.Println("----")
	}

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			t.Error(err)
		}
	})

	si := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	s := seal{ref: si}

	start := time.Now()

	s.precommit(t, sb, si, func() {})

	printFileList("precommit", true)

	precommit := time.Now()

	s.commit(t, sb, func() {})

	printFileList("commit", true)

	commit := time.Now()

	post(t, sb, nil, s)

	printFileList("post", true)

	epost := time.Now()

	post(t, sb, nil, s)

	if err := sb.FinalizeSector(context.TODO(), si); err != nil {
		t.Fatalf("%+v", err)
	}

	printFileList("finalize", false)

	s.unseal(t, sb, sp, si, func() {})

	printFileList("unseal", false)

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("Commit: %s\n", commit.Sub(precommit).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(commit).String())
}

type closeAssertReader struct {
	io.Reader
	closed bool
}

func (c *closeAssertReader) Close() error {
	if c.closed {
		panic("double close")
	}

	c.closed = true

	return nil
}

var _ io.Closer = &closeAssertReader{}

func TestSealCommDRInGo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	cdir, err := os.MkdirTemp("", "sbtest-c-")
	require.NoError(t, err)
	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", cdir)
			return
		}
		if err := os.RemoveAll(cdir); err != nil {
			t.Error(err)
		}
	})

	si := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	s := seal{ref: si}

	s.precommit(t, sb, si, func() {})

	p, _, err := sp.AcquireSector(context.Background(), si, storiface.FTCache, storiface.FTNone, storiface.PathStorage)
	require.NoError(t, err)

	commr, err := commitment.PAuxCommR(p.Cache)
	require.NoError(t, err)

	commd, err := commitment.TreeDCommD(p.Cache)
	require.NoError(t, err)

	sealCid, err := commcid.ReplicaCommitmentV1ToCID(commr[:])
	require.NoError(t, err)

	unsealedCid, err := commcid.DataCommitmentV1ToCID(commd[:])
	require.NoError(t, err)

	require.Equal(t, s.cids.Sealed, sealCid)
	require.Equal(t, s.cids.Unsealed, unsealedCid)
}

func TestGenerateSDR(t *testing.T) {
	d := t.TempDir()

	miner := abi.ActorID(123)

	sp := &basicfs.Provider{
		Root: d,
	}
	sb, err := New(sp)
	require.NoError(t, err)

	si := storiface.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	s := seal{ref: si}

	sz := abi.PaddedPieceSize(sectorSize).Unpadded()

	s.pi, err = sb.AddPiece(context.TODO(), si, []abi.UnpaddedPieceSize{}, sz, nullreader.NewNullReader(sz))
	require.NoError(t, err)

	s.ticket = sealRand

	_, err = sb.SealPreCommit1(context.TODO(), si, s.ticket, []abi.PieceInfo{s.pi})
	require.NoError(t, err)

	// sdr for comparison

	sdrCache := filepath.Join(d, "sdrcache")

	commd, err := commcid.CIDToDataCommitmentV1(s.pi.PieceCID)
	require.NoError(t, err)

	replicaID, err := sealProofType.ReplicaId(si.ID.Miner, si.ID.Number, s.ticket, commd)
	require.NoError(t, err)

	err = ffi.GenerateSDR(sealProofType, sdrCache, replicaID)
	require.NoError(t, err)

	// list files in d recursively, for debug

	require.NoError(t, filepath.Walk(d, func(path string, info fs.FileInfo, err error) error {
		fmt.Println(path)
		return nil
	}))

	// compare
	lastLayerFile := "sc-02-data-layer-2.dat"

	sdrFile := filepath.Join(sdrCache, lastLayerFile)
	pc1File := filepath.Join(d, "cache/s-t0123-1/", lastLayerFile)

	sdrData, err := os.ReadFile(sdrFile)
	require.NoError(t, err)

	pc1Data, err := os.ReadFile(pc1File)
	require.NoError(t, err)

	require.Equal(t, sdrData, pc1Data)
}

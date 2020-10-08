package ffiwrapper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	saproof "github.com/filecoin-project/specs-actors/actors/runtime/proof"

	"github.com/ipfs/go-cid"

	logging "github.com/ipfs/go-log"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	ffi "github.com/filecoin-project/filecoin-ffi"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
)

func init() {
	logging.SetLogLevel("*", "DEBUG") //nolint: errcheck
}

var sealProofType = abi.RegisteredSealProof_StackedDrg2KiBV1
var sectorSize, _ = sealProofType.SectorSize()

var sealRand = abi.SealRandomness{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2}

type seal struct {
	id     abi.SectorID
	cids   storage.SectorCids
	pi     abi.PieceInfo
	ticket abi.SealRandomness
}

func data(sn abi.SectorNumber, dlen abi.UnpaddedPieceSize) io.Reader {
	return io.MultiReader(
		io.LimitReader(rand.New(rand.NewSource(42+int64(sn))), int64(123)),
		io.LimitReader(rand.New(rand.NewSource(42+int64(sn))), int64(dlen-123)),
	)
}

func (s *seal) precommit(t *testing.T, sb *Sealer, id abi.SectorID, done func()) {
	defer done()
	dlen := abi.PaddedPieceSize(sectorSize).Unpadded()

	var err error
	r := data(id.Number, dlen)
	s.pi, err = sb.AddPiece(context.TODO(), id, []abi.UnpaddedPieceSize{}, dlen, r)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	s.ticket = sealRand

	p1, err := sb.SealPreCommit1(context.TODO(), id, s.ticket, []abi.PieceInfo{s.pi})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	cids, err := sb.SealPreCommit2(context.TODO(), id, p1)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	s.cids = cids
}

func (s *seal) commit(t *testing.T, sb *Sealer, done func()) {
	defer done()
	seed := abi.InteractiveSealRandomness{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9}

	pc1, err := sb.SealCommit1(context.TODO(), s.id, s.ticket, seed, []abi.PieceInfo{s.pi}, s.cids)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	proof, err := sb.SealCommit2(context.TODO(), s.id, pc1)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ok, err := ProofVerifier.VerifySeal(saproof.SealVerifyInfo{
		SectorID:              s.id,
		SealedCID:             s.cids.Sealed,
		SealProof:             sealProofType,
		Proof:                 proof,
		Randomness:            s.ticket,
		InteractiveRandomness: seed,
		UnsealedCID:           s.cids.Unsealed,
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}
}

func (s *seal) unseal(t *testing.T, sb *Sealer, sp *basicfs.Provider, si abi.SectorID, done func()) {
	defer done()

	var b bytes.Buffer
	_, err := sb.ReadPiece(context.TODO(), &b, si, 0, 1016)
	if err != nil {
		t.Fatal(err)
	}

	expect, _ := ioutil.ReadAll(data(si.Number, 1016))
	if !bytes.Equal(b.Bytes(), expect) {
		t.Fatal("read wrong bytes")
	}

	p, sd, err := sp.AcquireSector(context.TODO(), si, stores.FTUnsealed, stores.FTNone, stores.PathStorage)
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

	expect, _ = ioutil.ReadAll(data(si.Number, 1016))
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

	sis := make([]saproof.SectorInfo, len(seals))
	for i, s := range seals {
		sis[i] = saproof.SectorInfo{
			SealProof:    sealProofType,
			SectorNumber: s.id.Number,
			SealedCID:    s.cids.Sealed,
		}
	}

	proofs, skp, err := sealer.GenerateWindowPoSt(context.TODO(), seals[0].id.Miner, sis, randomness)
	if len(skipped) > 0 {
		require.Error(t, err)
		require.EqualValues(t, skipped, skp)
		return
	}

	if err != nil {
		t.Fatalf("%+v", err)
	}

	ok, err := ProofVerifier.VerifyWindowPoSt(context.TODO(), saproof.WindowPoStVerifyInfo{
		Randomness:        randomness,
		Proofs:            proofs,
		ChallengedSectors: sis,
		Prover:            seals[0].id.Miner,
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if !ok {
		t.Fatal("bad post")
	}
}

func corrupt(t *testing.T, sealer *Sealer, id abi.SectorID) {
	paths, done, err := sealer.sectors.AcquireSector(context.Background(), id, stores.FTSealed, 0, stores.PathStorage)
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
	dat, err := ioutil.ReadFile("../../../build/proof-params/parameters.json")
	if err != nil {
		panic(err)
	}

	err = paramfetch.GetParams(context.TODO(), dat, uint64(s))
	if err != nil {
		panic(xerrors.Errorf("failed to acquire Groth parameters for 2KiB sectors: %w", err))
	}
}

// TestDownloadParams exists only so that developers and CI can pre-download
// Groth parameters and verifying keys before running the tests which rely on
// those parameters and keys. To do this, run the following command:
//
// go test -run=^TestDownloadParams
//
func TestDownloadParams(t *testing.T) {
	defer requireFDsClosed(t, openFDs(t))

	getGrothParamFileAndVerifyingKeys(sectorSize)
}

func TestSealAndVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	defer requireFDsClosed(t, openFDs(t))

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	getGrothParamFileAndVerifyingKeys(sectorSize)

	cdir, err := ioutil.TempDir("", "sbtest-c-")
	if err != nil {
		t.Fatal(err)
	}
	miner := abi.ActorID(123)

	cfg := &Config{
		SealProofType: sealProofType,
	}

	sp := &basicfs.Provider{
		Root: cdir,
	}
	sb, err := New(sp, cfg)
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
	defer cleanup()

	si := abi.SectorID{Miner: miner, Number: 1}

	s := seal{id: si}

	start := time.Now()

	s.precommit(t, sb, si, func() {})

	precommit := time.Now()

	s.commit(t, sb, func() {})

	commit := time.Now()

	post(t, sb, nil, s)

	epost := time.Now()

	post(t, sb, nil, s)

	if err := sb.FinalizeSector(context.TODO(), si, nil); err != nil {
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

	defer requireFDsClosed(t, openFDs(t))

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	getGrothParamFileAndVerifyingKeys(sectorSize)

	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	miner := abi.ActorID(123)

	cfg := &Config{
		SealProofType: sealProofType,
	}
	sp := &basicfs.Provider{
		Root: dir,
	}
	sb, err := New(sp, cfg)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	cleanup := func() {
		if t.Failed() {
			fmt.Printf("not removing %s\n", dir)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	}
	defer cleanup()

	si := abi.SectorID{Miner: miner, Number: 1}

	s := seal{id: si}

	start := time.Now()

	s.precommit(t, sb, si, func() {})

	precommit := time.Now()

	if err := sb.FinalizeSector(context.TODO(), si, nil); err != nil {
		t.Fatal(err)
	}

	post(t, sb, nil, s)

	epost := time.Now()

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(precommit).String())
}

func TestSealAndVerify3(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	defer requireFDsClosed(t, openFDs(t))

	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "trace")

	getGrothParamFileAndVerifyingKeys(sectorSize)

	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	miner := abi.ActorID(123)

	cfg := &Config{
		SealProofType: sealProofType,
	}
	sp := &basicfs.Provider{
		Root: dir,
	}
	sb, err := New(sp, cfg)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	}

	defer cleanup()

	var wg sync.WaitGroup

	si1 := abi.SectorID{Miner: miner, Number: 1}
	si2 := abi.SectorID{Miner: miner, Number: 2}
	si3 := abi.SectorID{Miner: miner, Number: 3}

	s1 := seal{id: si1}
	s2 := seal{id: si2}
	s3 := seal{id: si3}

	wg.Add(3)
	go s1.precommit(t, sb, si1, wg.Done) //nolint: staticcheck
	time.Sleep(100 * time.Millisecond)
	go s2.precommit(t, sb, si2, wg.Done) //nolint: staticcheck
	time.Sleep(100 * time.Millisecond)
	go s3.precommit(t, sb, si3, wg.Done) //nolint: staticcheck
	wg.Wait()

	wg.Add(3)
	go s1.commit(t, sb, wg.Done) //nolint: staticcheck
	go s2.commit(t, sb, wg.Done) //nolint: staticcheck
	go s3.commit(t, sb, wg.Done) //nolint: staticcheck
	wg.Wait()

	post(t, sb, nil, s1, s2, s3)

	corrupt(t, sb, si1)
	corrupt(t, sb, si2)

	post(t, sb, []abi.SectorID{si1, si2}, s1, s2, s3)
}

func BenchmarkWriteWithAlignment(b *testing.B) {
	bt := abi.UnpaddedPieceSize(2 * 127 * 1024 * 1024)
	b.SetBytes(int64(bt))

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rf, w, _ := ToReadableFile(bytes.NewReader(bytes.Repeat([]byte{0xff, 0}, int(bt/2))), int64(bt))
		tf, _ := ioutil.TempFile("/tmp/", "scrb-")
		b.StartTimer()

		ffi.WriteWithAlignment(abi.RegisteredSealProof_StackedDrg2KiBV1, rf, bt, tf, nil) // nolint:errcheck
		_ = w()
	}
}

func openFDs(t *testing.T) int {
	dent, err := ioutil.ReadDir("/proc/self/fd")
	require.NoError(t, err)

	var skip int
	for _, info := range dent {
		l, err := os.Readlink(filepath.Join("/proc/self/fd", info.Name()))
		if err != nil {
			continue
		}

		if strings.HasPrefix(l, "/dev/nvidia") {
			skip++
		}

		if strings.HasPrefix(l, "/var/tmp/filecoin-proof-parameters/") {
			skip++
		}
	}

	return len(dent) - skip
}

func requireFDsClosed(t *testing.T, start int) {
	openNow := openFDs(t)

	if start != openNow {
		dent, err := ioutil.ReadDir("/proc/self/fd")
		require.NoError(t, err)

		for _, info := range dent {
			l, err := os.Readlink(filepath.Join("/proc/self/fd", info.Name()))
			if err != nil {
				fmt.Printf("FD err %s\n", err)
				continue
			}

			fmt.Printf("FD %s -> %s\n", info.Name(), l)
		}
	}

	log.Infow("open FDs", "start", start, "now", openNow)
	require.Equal(t, start, openNow, "FDs shouldn't leak")
}

func TestGenerateUnsealedCID(t *testing.T) {
	pt := abi.RegisteredSealProof_StackedDrg2KiBV1
	ups := int(abi.PaddedPieceSize(2048).Unpadded())

	commP := func(b []byte) cid.Cid {
		pf, werr, err := ToReadableFile(bytes.NewReader(b), int64(len(b)))
		require.NoError(t, err)

		c, err := ffi.GeneratePieceCIDFromFile(pt, pf, abi.UnpaddedPieceSize(len(b)))
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

			expectCid, err := GenerateUnsealedCID(pt, sectorPi)
			require.NoError(t, err)

			actualCid, err := GenerateUnsealedCID(pt, upi)
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

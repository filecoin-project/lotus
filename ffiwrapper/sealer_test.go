package ffiwrapper

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/sector-storage/ffiwrapper/basicfs"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
)

func init() {
	logging.SetLogLevel("*", "INFO") //nolint: errcheck
}

var sectorSize = abi.SectorSize(2048)
var sealProofType = abi.RegisteredProof_StackedDRG2KiBSeal

type seal struct {
	id     abi.SectorID
	cids   storage.SectorCids
	pi     abi.PieceInfo
	ticket abi.SealRandomness
}

func (s *seal) precommit(t *testing.T, sb *Sealer, id abi.SectorID, done func()) {
	defer done()
	dlen := abi.PaddedPieceSize(sectorSize).Unpadded()

	var err error
	r := io.LimitReader(rand.New(rand.NewSource(42+int64(id.Number))), int64(dlen))
	s.pi, err = sb.AddPiece(context.TODO(), id, []abi.UnpaddedPieceSize{}, dlen, r)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	s.ticket = abi.SealRandomness{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2}

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

	ok, err := ProofVerifier.VerifySeal(abi.SealVerifyInfo{
		SectorID:              s.id,
		SealedCID:             s.cids.Sealed,
		RegisteredProof:       sealProofType,
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

func post(t *testing.T, sealer *Sealer, seals ...seal) time.Time {
	/*randomness := abi.PoStRandomness{0, 9, 2, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 7}

	sis := make([]abi.SectorInfo, len(seals))
	for i, s := range seals {
		sis[i] = abi.SectorInfo{
			RegisteredProof: sealProofType,
			SectorNumber:    s.id.Number,
			SealedCID:       s.cids.Sealed,
		}
	}

	candidates, err := sealer.GenerateEPostCandidates(context.TODO(), seals[0].id.Miner, sis, randomness, []abi.SectorNumber{})
	if err != nil {
		t.Fatalf("%+v", err)
	}*/

	fmt.Println("skipping post")

	genCandidates := time.Now()

	/*if len(candidates) != 1 {
		t.Fatal("expected 1 candidate")
	}

	candidatesPrime := make([]abi.PoStCandidate, len(candidates))
	for idx := range candidatesPrime {
		candidatesPrime[idx] = candidates[idx].Candidate
	}

	proofs, err := sealer.ComputeElectionPoSt(context.TODO(), seals[0].id.Miner, sis, randomness, candidatesPrime)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ePoStChallengeCount := ElectionPostChallengeCount(uint64(len(sis)), 0)

	ok, err := ProofVerifier.VerifyElectionPost(context.TODO(), abi.PoStVerifyInfo{
		Randomness:      randomness,
		Candidates:      candidatesPrime,
		Proofs:          proofs,
		EligibleSectors: sis,
		Prover:          seals[0].id.Miner,
		ChallengeCount:  ePoStChallengeCount,
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if !ok {
		t.Fatal("bad post")
	}
	*/
	return genCandidates
}

func getGrothParamFileAndVerifyingKeys(s abi.SectorSize) {
	dat, err := ioutil.ReadFile("../parameters.json")
	if err != nil {
		panic(err)
	}

	err = paramfetch.GetParams(dat, uint64(s))
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
	getGrothParamFileAndVerifyingKeys(sectorSize)
}

func TestSealAndVerify(t *testing.T) {
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

	genCandidiates := post(t, sb, s)

	epost := time.Now()

	post(t, sb, s)

	if err := sb.FinalizeSector(context.TODO(), si); err != nil {
		t.Fatalf("%+v", err)
	}

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("Commit: %s\n", commit.Sub(precommit).String())
	fmt.Printf("GenCandidates: %s\n", genCandidiates.Sub(commit).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(genCandidiates).String())
}

func TestSealPoStNoCommit(t *testing.T) {
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

	if err := sb.FinalizeSector(context.TODO(), si); err != nil {
		t.Fatal(err)
	}

	genCandidiates := post(t, sb, s)

	epost := time.Now()

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("GenCandidates: %s\n", genCandidiates.Sub(precommit).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(genCandidiates).String())
}

func TestSealAndVerify2(t *testing.T) {
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

	s1 := seal{id: si1}
	s2 := seal{id: si2}

	wg.Add(2)
	go s1.precommit(t, sb, si1, wg.Done) //nolint: staticcheck
	time.Sleep(100 * time.Millisecond)
	go s2.precommit(t, sb, si2, wg.Done) //nolint: staticcheck
	wg.Wait()

	wg.Add(2)
	go s1.commit(t, sb, wg.Done) //nolint: staticcheck
	go s2.commit(t, sb, wg.Done) //nolint: staticcheck
	wg.Wait()

	post(t, sb, s1, s2)
}

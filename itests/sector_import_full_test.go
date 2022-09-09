package itests

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/itests/kit"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestSectorImport(t *testing.T) {

	type testCase struct {
		c1handler func(s *ffiwrapper.Sealer) func(w http.ResponseWriter, r *http.Request)

		mutateRemoteMeta func(*api.RemoteSectorMeta)

		expectImportErrContains string
	}

	makeTest := func(mut func(*testCase)) *testCase {
		tc := &testCase{
			c1handler: remoteCommit1,
		}
		mut(tc)
		return tc
	}

	runTest := func(tc *testCase) func(t *testing.T) {
		return func(t *testing.T) {
			kit.QuietMiningLogs()

			var blockTime = 50 * time.Millisecond

			////////
			// Start a miner node

			client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC())
			ens.InterconnectAll().BeginMining(blockTime)

			ctx := context.Background()

			////////
			// Reserve some sector numbers on the miner node; We'll use one of those when creating the sector "remotely"
			snums, err := miner.SectorNumReserveCount(ctx, "test-reservation-0001", 16)
			require.NoError(t, err)

			sectorDir := t.TempDir()

			maddr, err := miner.ActorAddress(ctx)
			require.NoError(t, err)

			mid, err := address.IDFromAddress(maddr)
			require.NoError(t, err)

			spt, err := currentSealProof(ctx, client, maddr)
			require.NoError(t, err)

			ssize, err := spt.SectorSize()
			require.NoError(t, err)

			pieceSize := abi.PaddedPieceSize(ssize)

			////////
			// Create/Seal a sector up to pc2 outside of the pipeline

			// get one sector number from the reservation done on the miner above
			sn, err := snums.First()
			require.NoError(t, err)

			// create all the sector identifiers
			snum := abi.SectorNumber(sn)
			sid := abi.SectorID{Miner: abi.ActorID(mid), Number: snum}
			sref := storiface.SectorRef{ID: sid, ProofType: spt}

			// create a low-level sealer instance
			sealer, err := ffiwrapper.New(&basicfs.Provider{
				Root: sectorDir,
			})
			require.NoError(t, err)

			// CRETE THE UNSEALED FILE

			// create a reader for all-zero (CC) data
			dataReader := bytes.NewReader(bytes.Repeat([]byte{0}, int(pieceSize.Unpadded())))

			// create the unsealed CC sector file
			pieceInfo, err := sealer.AddPiece(ctx, sref, nil, pieceSize.Unpadded(), dataReader)
			require.NoError(t, err)

			// GENERATE THE TICKET

			// get most recent valid ticket epoch
			ts, err := client.ChainHead(ctx)
			require.NoError(t, err)
			ticketEpoch := ts.Height() - policy.SealRandomnessLookback

			// ticket entropy is cbor-seriasized miner addr
			buf := new(bytes.Buffer)
			require.NoError(t, maddr.MarshalCBOR(buf))

			// generate ticket randomness
			rand, err := client.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, ticketEpoch, buf.Bytes(), ts.Key())
			require.NoError(t, err)

			// EXECUTE PRECOMMIT 1 / 2

			// run PC1
			pc1out, err := sealer.SealPreCommit1(ctx, sref, abi.SealRandomness(rand), []abi.PieceInfo{pieceInfo})
			require.NoError(t, err)

			// run pc2
			scids, err := sealer.SealPreCommit2(ctx, sref, pc1out)
			require.NoError(t, err)

			// make finalized cache, put it in [sectorDir]/fin-cache while keeping the large cache for remote C1
			finDst := filepath.Join(sectorDir, "fin-cache", fmt.Sprintf("s-t01000-%d", snum))
			require.NoError(t, os.MkdirAll(finDst, 0777))
			require.NoError(t, sealer.FinalizeSectorInto(ctx, sref, finDst))

			////////
			// start http server serving sector data

			m := mux.NewRouter()
			m.HandleFunc("/sectors/{type}/{id}", remoteGetSector(sectorDir)).Methods("GET")
			m.HandleFunc("/sectors/{id}/commit1", tc.c1handler(sealer)).Methods("POST")
			srv := httptest.NewServer(m)

			unsealedURL := fmt.Sprintf("%s/sectors/unsealed/s-t0%d-%d", srv.URL, mid, snum)
			sealedURL := fmt.Sprintf("%s/sectors/sealed/s-t0%d-%d", srv.URL, mid, snum)
			cacheURL := fmt.Sprintf("%s/sectors/cache/s-t0%d-%d", srv.URL, mid, snum)
			remoteC1URL := fmt.Sprintf("%s/sectors/s-t0%d-%d/commit1", srv.URL, mid, snum)

			////////
			// import the sector and continue sealing

			rmeta := api.RemoteSectorMeta{
				State:  "PreCommitting",
				Sector: sid,
				Type:   spt,

				Pieces: []api.SectorPiece{
					{
						Piece:    pieceInfo,
						DealInfo: nil,
					},
				},

				TicketValue: abi.SealRandomness(rand),
				TicketEpoch: ticketEpoch,

				PreCommit1Out: pc1out,

				CommD: &scids.Unsealed,
				CommR: &scids.Sealed,

				DataUnsealed: &storiface.SectorData{
					Local: false,
					URL:   unsealedURL,
				},
				DataSealed: &storiface.SectorData{
					Local: false,
					URL:   sealedURL,
				},
				DataCache: &storiface.SectorData{
					Local: false,
					URL:   cacheURL,
				},

				RemoteCommit1Endpoint: remoteC1URL,
			}

			if tc.mutateRemoteMeta != nil {
				tc.mutateRemoteMeta(&rmeta)
			}

			err = miner.SectorReceive(ctx, rmeta)
			if tc.expectImportErrContains != "" {
				require.ErrorContains(t, err, tc.expectImportErrContains)
				return
			}

			require.NoError(t, err)

			// check that we see the imported sector
			ng, err := miner.SectorsListNonGenesis(ctx)
			require.NoError(t, err)
			require.Len(t, ng, 1)
			require.Equal(t, snum, ng[0])

			miner.WaitSectorsProvingAllowFails(ctx, map[abi.SectorNumber]struct{}{snum: {}}, map[api.SectorState]struct{}{api.SectorState(sealing.RemoteCommit1Failed): {}})
		}
	}

	// fail first remote c1, verifies that c1 retry works
	t.Run("c1-retry", runTest(makeTest(func(testCase *testCase) {
		prt := sealing.MinRetryTime
		sealing.MinRetryTime = time.Second
		t.Cleanup(func() {
			sealing.MinRetryTime = prt
		})

		testCase.c1handler = func(s *ffiwrapper.Sealer) func(w http.ResponseWriter, r *http.Request) {
			var failedOnce bool

			return func(w http.ResponseWriter, r *http.Request) {
				if !failedOnce {
					failedOnce = true
					w.WriteHeader(http.StatusBadGateway)
					return
				}

				remoteCommit1(s)(w, r)
			}
		}
	})))

	t.Run("nil-commd", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			meta.CommD = nil
		}
		testCase.expectImportErrContains = "both CommR/CommD cids need to be set for sectors in PreCommitting and later states"
	})))
	t.Run("nil-commr", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			meta.CommR = nil
		}
		testCase.expectImportErrContains = "both CommR/CommD cids need to be set for sectors in PreCommitting and later states"
	})))

	t.Run("nil-uns", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			meta.DataUnsealed = nil
		}
		testCase.expectImportErrContains = "expected DataUnsealed to be set"
	})))
	t.Run("nil-sealed", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			meta.DataSealed = nil
		}
		testCase.expectImportErrContains = "expected DataSealed to be set"
	})))
	t.Run("nil-cache", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			meta.DataCache = nil
		}
		testCase.expectImportErrContains = "expected DataCache to be set"
	})))

	t.Run("bad-commd", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			meta.CommD = meta.CommR
		}
		testCase.expectImportErrContains = "CommD cid has wrong prefix"
	})))
	t.Run("bad-commr", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			meta.CommR = meta.CommD
		}
		testCase.expectImportErrContains = "CommR cid has wrong prefix"
	})))

	t.Run("bad-ticket", runTest(makeTest(func(testCase *testCase) {
		testCase.mutateRemoteMeta = func(meta *api.RemoteSectorMeta) {
			// flip one bit
			meta.TicketValue[23] ^= 4
		}
		testCase.expectImportErrContains = "tickets differ"
	})))

}

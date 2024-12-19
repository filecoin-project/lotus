package paths_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/paths/mocks"
	"github.com/filecoin-project/lotus/storage/sealer/partialfile"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const metaFile = "sectorstore.json"

func createTestStorage(t *testing.T, p string, seal bool, att ...*paths.Local) storiface.ID {
	if err := os.MkdirAll(p, 0755); err != nil {
		if !os.IsExist(err) {
			require.NoError(t, err)
		}
	}

	cfg := &storiface.LocalStorageMeta{
		ID:       storiface.ID(uuid.New().String()),
		Weight:   10,
		CanSeal:  seal,
		CanStore: !seal,
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(p, metaFile), b, 0644))

	for _, s := range att {
		require.NoError(t, s.OpenPath(context.Background(), p))
	}

	return cfg.ID
}

func TestMoveShared(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	index := paths.NewMemIndex(nil)

	ctx := context.Background()

	dir := t.TempDir()

	openRepo := func(dir string) repo.LockedRepo {
		r, err := repo.NewFS(dir)
		require.NoError(t, err)
		require.NoError(t, r.Init(repo.Worker))
		lr, err := r.Lock(repo.Worker)
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = lr.Close()
		})

		err = lr.SetStorage(func(config *storiface.StorageConfig) {
			*config = storiface.StorageConfig{}
		})
		require.NoError(t, err)

		return lr
	}

	// setup two repos with two storage paths:
	// repo 1 with both paths
	// repo 2 with one path (shared)

	lr1 := openRepo(filepath.Join(dir, "l1"))
	lr2 := openRepo(filepath.Join(dir, "l2"))

	mux1 := mux.NewRouter()
	mux2 := mux.NewRouter()
	hs1 := httptest.NewServer(mux1)
	hs2 := httptest.NewServer(mux2)

	ls1, err := paths.NewLocal(ctx, lr1, index, []string{hs1.URL + "/remote"})
	require.NoError(t, err)
	ls2, err := paths.NewLocal(ctx, lr2, index, []string{hs2.URL + "/remote"})
	require.NoError(t, err)

	dirStor := filepath.Join(dir, "stor")
	dirSeal := filepath.Join(dir, "seal")

	id1 := createTestStorage(t, dirStor, false, ls1, ls2)
	id2 := createTestStorage(t, dirSeal, true, ls1)

	rs1 := paths.NewRemote(ls1, index, nil, 20, &paths.DefaultPartialFileHandler{})
	rs2 := paths.NewRemote(ls2, index, nil, 20, &paths.DefaultPartialFileHandler{})
	_ = rs2
	mux1.PathPrefix("/").Handler(&paths.FetchHandler{Local: ls1, PfHandler: &paths.DefaultPartialFileHandler{}})
	mux2.PathPrefix("/").Handler(&paths.FetchHandler{Local: ls2, PfHandler: &paths.DefaultPartialFileHandler{}})

	// add a sealed replica file to the sealing (non-shared) path

	s1ref := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  12,
			Number: 1,
		},
		ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1,
	}

	sp, sid, err := rs1.AcquireSector(ctx, s1ref, storiface.FTNone, storiface.FTSealed, storiface.PathSealing, storiface.AcquireMove)
	require.NoError(t, err)
	require.Equal(t, id2, storiface.ID(sid.Sealed))

	data := make([]byte, 2032)
	data[1] = 54
	require.NoError(t, os.WriteFile(sp.Sealed, data, 0666))
	fmt.Println("write to ", sp.Sealed)

	require.NoError(t, index.StorageDeclareSector(ctx, storiface.ID(sid.Sealed), s1ref.ID, storiface.FTSealed, true))

	// move to the shared path from the second node (remote move / delete)

	require.NoError(t, rs2.MoveStorage(ctx, s1ref, storiface.FTSealed))

	// check that the file still exists
	sp, sid, err = rs2.AcquireSector(ctx, s1ref, storiface.FTSealed, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	require.NoError(t, err)
	require.Equal(t, id1, storiface.ID(sid.Sealed))
	fmt.Println("read from ", sp.Sealed)

	read, err := os.ReadFile(sp.Sealed)
	require.NoError(t, err)
	require.EqualValues(t, data, read)
}

func TestReader(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)
	bz := []byte("Hello World")

	pfPath := "path"
	emptyPartialFile := &partialfile.PartialFile{}
	sectorSize := abi.SealProofInfos[1].SectorSize

	ft := storiface.FTUnsealed

	sectorRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  123,
			Number: 123,
		},
		ProofType: 1,
	}

	offset := abi.PaddedPieceSize(100)
	size := abi.PaddedPieceSize(1000)
	ctx := context.Background()

	tcs := map[string]struct {
		storeFnc func(s *mocks.MockStore)
		pfFunc   func(s *mocks.MockPartialFileHandler)
		indexFnc func(s *mocks.MockSectorIndex, serverURL string)

		needHttpServer bool

		getAllocatedReturnCode int
		getSectorReturnCode    int

		serverUrl string

		// expectation
		errStr               string
		expectedNonNilReader bool
		expectedSectorBytes  []byte
	}{

		// -------- have the unsealed file locally
		"fails when error while acquiring unsealed file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, xerrors.New("acquire error"))
			},

			errStr: "acquire error",
		},

		"fails when error while opening local partial (unsealed) file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, xerrors.New("pf open error"))
			},
			errStr: "pf open error",
		},

		"fails when error while checking if local unsealed file has piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					true, xerrors.New("piece check error"))
			},

			errStr: "piece check error",
		},

		"fails when error while closing local unsealed file that does not have the piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					false, nil)
				pf.EXPECT().Close(emptyPartialFile).Return(xerrors.New("close error")).Times(1)
			},
			errStr: "close error",
		},

		"fails when error while fetching reader for the local unsealed file that has the unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					true, nil)
				mockPfReader(pf, emptyPartialFile, offset, size, nil, xerrors.New("reader error"))

			},
			errStr: "reader error",
		},

		// ------------------- don't have the unsealed file locally

		"fails when error while finding sector": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, _ string) {
				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return(nil, xerrors.New("find sector error"))
			},
			errStr: "find sector error",
		},

		"fails when no worker has unsealed file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, _ string) {
				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return(nil, nil)
			},
			errStr: storiface.ErrSectorNotFound.Error(),
		},

		// --- nil reader when local unsealed file does NOT have unsealed piece
		"nil reader when local unsealed file does not have the unsealed piece and remote sector also dosen't have the unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					false, nil)

				pf.EXPECT().Close(emptyPartialFile).Return(nil).Times(1)

			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getAllocatedReturnCode: 500,
		},

		// ---- nil reader when none of the remote unsealed file has unsealed piece
		"nil reader when none of the worker has the unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getAllocatedReturnCode: 500,
		},

		"nil reader when none of the worker is able to serve the unsealed piece even though they have it": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getSectorReturnCode:    500,
			getAllocatedReturnCode: 200,
		},

		// ---- Success for local unsealed file
		"successfully fetches reader for piece from local unsealed file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					true, nil)

				f, err := os.CreateTemp("", "TestReader-")
				require.NoError(t, err)
				_, err = f.Write(bz)
				require.NoError(t, err)
				require.NoError(t, f.Close())
				f, err = os.Open(f.Name())
				require.NoError(t, err)

				mockPfReader(pf, emptyPartialFile, offset, size, f, nil)

			},

			expectedNonNilReader: true,
			expectedSectorBytes:  bz,
		},

		// --- Success for remote unsealed file
		// --- Success for remote unsealed file
		"successfully fetches reader from remote unsealed piece when local unsealed file does NOT have the unsealed Piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					false, nil)

				pf.EXPECT().Close(emptyPartialFile).Return(nil).Times(1)

			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getSectorReturnCode:    200,
			getAllocatedReturnCode: 200,
			expectedSectorBytes:    bz,
			expectedNonNilReader:   true,
		},

		"successfully fetches reader for piece from remote unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getSectorReturnCode:    200,
			getAllocatedReturnCode: 200,
			expectedSectorBytes:    bz,
			expectedNonNilReader:   true,
		},
	}

	for name, tc := range tcs {
		tc := tc
		t.Run(name, func(t *testing.T) {
			// create go mock controller here
			mockCtrl := gomock.NewController(t)
			// when test is done, assert expectations on all mock objects.
			defer mockCtrl.Finish()

			// create them mocks
			lstore := mocks.NewMockStore(mockCtrl)
			pfhandler := mocks.NewMockPartialFileHandler(mockCtrl)
			index := mocks.NewMockSectorIndex(mockCtrl)

			if tc.storeFnc != nil {
				tc.storeFnc(lstore)
			}
			if tc.pfFunc != nil {
				tc.pfFunc(pfhandler)
			}

			if tc.needHttpServer {
				// run http server
				ts := httptest.NewServer(&mockHttpServer{
					expectedSectorName: storiface.SectorName(sectorRef.ID),
					expectedFileType:   ft.String(),
					expectedOffset:     fmt.Sprintf("%d", offset.Unpadded()),
					expectedSize:       fmt.Sprintf("%d", size.Unpadded()),
					expectedSectorType: fmt.Sprintf("%d", sectorRef.ProofType),

					getAllocatedReturnCode: tc.getAllocatedReturnCode,
					getSectorReturnCode:    tc.getSectorReturnCode,
					getSectorBytes:         tc.expectedSectorBytes,
				})
				defer ts.Close()
				tc.serverUrl = fmt.Sprintf("%s/remote/%s/%s", ts.URL, ft.String(), storiface.SectorName(sectorRef.ID))
			}
			if tc.indexFnc != nil {
				tc.indexFnc(index, tc.serverUrl)
			}

			remoteStore := paths.NewRemote(lstore, index, nil, 6000, pfhandler)

			rdg, err := remoteStore.Reader(ctx, sectorRef, offset, size)
			var rd io.ReadCloser

			if tc.errStr != "" {
				if rdg == nil {
					require.Error(t, err)
					require.Nil(t, rdg)
					require.Contains(t, err.Error(), tc.errStr)
				} else {
					rd, err = rdg(0, storiface.PaddedByteIndex(size))
					require.Error(t, err)
					require.Nil(t, rd)
					require.Contains(t, err.Error(), tc.errStr)
				}
			} else {
				require.NoError(t, err)
			}

			if !tc.expectedNonNilReader {
				require.Nil(t, rd)
			} else {
				require.NotNil(t, rdg)
				rd, err := rdg(0, storiface.PaddedByteIndex(size))
				require.NoError(t, err)

				defer func() {
					require.NoError(t, rd.Close())
				}()

				if f, ok := rd.(*os.File); ok {
					require.NoError(t, os.Remove(f.Name()))
				}

				bz, err := io.ReadAll(rd)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSectorBytes, bz)
			}

		})
	}
}

func TestCheckIsUnsealed(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	pfPath := "path"
	ft := storiface.FTUnsealed
	emptyPartialFile := &partialfile.PartialFile{}

	sectorRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  123,
			Number: 123,
		},
		ProofType: 1,
	}
	sectorSize := abi.SealProofInfos[1].SectorSize

	offset := abi.PaddedPieceSize(100)
	size := abi.PaddedPieceSize(1000)
	ctx := context.Background()

	tcs := map[string]struct {
		storeFnc func(s *mocks.MockStore)
		pfFunc   func(s *mocks.MockPartialFileHandler)
		indexFnc func(s *mocks.MockSectorIndex, serverURL string)

		needHttpServer bool

		getAllocatedReturnCode int

		serverUrl string

		// expectation
		errStr            string
		expectedIsUnealed bool
	}{

		// -------- have the unsealed file locally
		"fails when error while acquiring unsealed file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, xerrors.New("acquire error"))
			},

			errStr: "acquire error",
		},

		"fails when error while opening local partial (unsealed) file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, xerrors.New("pf open error"))
			},
			errStr: "pf open error",
		},

		"fails when error while checking if local unsealed file has piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					true, xerrors.New("piece check error"))
			},

			errStr: "piece check error",
		},

		"fails when error while closing local unsealed file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)

				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					false, nil)

				pf.EXPECT().Close(emptyPartialFile).Return(xerrors.New("close error")).Times(1)
			},
			errStr: "close error",
		},

		// ------------------- don't have the unsealed file locally

		"fails when error while finding sector": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, _ string) {
				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return(nil, xerrors.New("find sector error"))
			},
			errStr: "find sector error",
		},

		"false when no worker has unsealed file": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, _ string) {
				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return(nil, nil)
			},
		},

		// false when local unsealed file does NOT have unsealed piece
		"false when local unsealed file does not have the piece and remote sector too dosen't have the piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					false, nil)

				pf.EXPECT().Close(emptyPartialFile).Return(nil).Times(1)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getAllocatedReturnCode: 500,
		},

		"false when none of the worker has the unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getAllocatedReturnCode: 500,
		},

		// ---- Success for local unsealed file
		"true when local unsealed file has the piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					true, nil)
				pf.EXPECT().Close(emptyPartialFile).Return(nil).Times(1)

			},

			expectedIsUnealed: true,
		},

		// --- Success for remote unsealed file
		"true if we have a  remote unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getAllocatedReturnCode: 200,
			expectedIsUnealed:      true,
		},

		"true when local unsealed file does NOT have the unsealed Piece but remote sector has the unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					false, nil)

				pf.EXPECT().Close(emptyPartialFile).Return(nil).Times(1)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := storiface.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]storiface.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getAllocatedReturnCode: 200,
			expectedIsUnealed:      true,
		},
	}

	for name, tc := range tcs {
		tc := tc
		t.Run(name, func(t *testing.T) {
			// create go mock controller here
			mockCtrl := gomock.NewController(t)
			// when test is done, assert expectations on all mock objects.
			defer mockCtrl.Finish()

			// create them mocks
			lstore := mocks.NewMockStore(mockCtrl)
			pfhandler := mocks.NewMockPartialFileHandler(mockCtrl)
			index := mocks.NewMockSectorIndex(mockCtrl)

			if tc.storeFnc != nil {
				tc.storeFnc(lstore)
			}
			if tc.pfFunc != nil {
				tc.pfFunc(pfhandler)
			}

			if tc.needHttpServer {
				// run http server
				ts := httptest.NewServer(&mockHttpServer{
					expectedSectorName: storiface.SectorName(sectorRef.ID),
					expectedFileType:   ft.String(),
					expectedOffset:     fmt.Sprintf("%d", offset.Unpadded()),
					expectedSize:       fmt.Sprintf("%d", size.Unpadded()),
					expectedSectorType: fmt.Sprintf("%d", sectorRef.ProofType),

					getAllocatedReturnCode: tc.getAllocatedReturnCode,
				})
				defer ts.Close()
				tc.serverUrl = fmt.Sprintf("%s/remote/%s/%s", ts.URL, ft.String(), storiface.SectorName(sectorRef.ID))
			}
			if tc.indexFnc != nil {
				tc.indexFnc(index, tc.serverUrl)
			}

			remoteStore := paths.NewRemote(lstore, index, nil, 6000, pfhandler)

			isUnsealed, err := remoteStore.CheckIsUnsealed(ctx, sectorRef, offset, size)

			if tc.errStr != "" {
				require.Error(t, err)
				require.False(t, isUnsealed)
				require.Contains(t, err.Error(), tc.errStr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedIsUnealed, isUnsealed)

		})
	}
}

func mockSectorAcquire(l *mocks.MockStore, sectorRef storiface.SectorRef, pfPath string, err error) {
	l.EXPECT().AcquireSector(gomock.Any(), sectorRef, storiface.FTUnsealed,
		storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
		Unsealed: pfPath,
	},
		storiface.SectorPaths{}, err).Times(1)
}

func mockPartialFileOpen(pf *mocks.MockPartialFileHandler, sectorSize abi.SectorSize, pfPath string, err error) {
	pf.EXPECT().OpenPartialFile(abi.PaddedPieceSize(sectorSize), pfPath).Return(&partialfile.PartialFile{},
		err).Times(1)
}

func mockCheckAllocation(pf *mocks.MockPartialFileHandler, offset, size abi.PaddedPieceSize, file *partialfile.PartialFile,
	out bool, err error) {
	pf.EXPECT().HasAllocated(file, storiface.UnpaddedByteIndex(offset.Unpadded()),
		size.Unpadded()).Return(out, err).Times(1)
}

func mockPfReader(pf *mocks.MockPartialFileHandler, file *partialfile.PartialFile, offset, size abi.PaddedPieceSize,
	outFile *os.File, err error) {
	pf.EXPECT().Reader(file, storiface.PaddedByteIndex(offset), size).Return(outFile, err)
}

type mockHttpServer struct {
	expectedSectorName string
	expectedFileType   string
	expectedOffset     string
	expectedSize       string
	expectedSectorType string

	getAllocatedReturnCode int

	getSectorReturnCode int
	getSectorBytes      []byte
}

func (m *mockHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := mux.NewRouter()
	mux.HandleFunc("/remote/{type}/{id}", m.getSector).Methods("GET")
	mux.HandleFunc("/remote/{type}/{id}/{spt}/allocated/{offset}/{size}", m.getAllocated).Methods("GET")
	mux.ServeHTTP(w, r)
}

func (m *mockHttpServer) getAllocated(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	if vars["id"] != m.expectedSectorName {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if vars["type"] != m.expectedFileType {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if vars["spt"] != m.expectedSectorType {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if vars["offset"] != m.expectedOffset {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if vars["size"] != m.expectedSize {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(m.getAllocatedReturnCode)
}

func (m *mockHttpServer) getSector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	if vars["id"] != m.expectedSectorName {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if vars["type"] != m.expectedFileType {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(m.getSectorReturnCode)
	_, _ = w.Write(m.getSectorBytes)
}

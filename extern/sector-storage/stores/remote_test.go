package stores_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/partialfile"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores/mocks"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestReader(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)
	bz := []byte("Hello World")

	pfPath := "path"
	ft := storiface.FTUnsealed
	emptyPartialFile := &partialfile.PartialFile{}

	sectorRef := storage.SectorRef{
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
		pfFunc   func(s *mocks.MockpartialFileHandler)
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

			pfFunc: func(pf *mocks.MockpartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, xerrors.New("pf open error"))
			},
			errStr: "pf open error",
		},

		"fails when error while checking if local unsealed file has piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockpartialFileHandler) {
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

			pfFunc: func(pf *mocks.MockpartialFileHandler) {
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

			pfFunc: func(pf *mocks.MockpartialFileHandler) {
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
		"nil reader when local unsealed file does not have the piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, pfPath, nil)
			},

			pfFunc: func(pf *mocks.MockpartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					false, nil)

				pf.EXPECT().Close(emptyPartialFile).Return(nil).Times(1)
			},
		},

		// ---- nil reader when none of the remote unsealed file has unsealed piece
		"nil reader when none of the worker has the unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := stores.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]stores.SectorStorageInfo{si}, nil).Times(1)
			},

			needHttpServer:         true,
			getAllocatedReturnCode: 500,
		},

		"nil reader when none of the worker is able to serve the unsealed piece even though they have it": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := stores.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]stores.SectorStorageInfo{si}, nil).Times(1)
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

			pfFunc: func(pf *mocks.MockpartialFileHandler) {
				mockPartialFileOpen(pf, sectorSize, pfPath, nil)
				mockCheckAllocation(pf, offset, size, emptyPartialFile,
					true, nil)

				f, err := ioutil.TempFile("", "TestReader-")
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
		"successfully fetches reader for piece from remote unsealed piece": {
			storeFnc: func(l *mocks.MockStore) {
				mockSectorAcquire(l, sectorRef, "", nil)
			},

			indexFnc: func(in *mocks.MockSectorIndex, url string) {
				si := stores.SectorStorageInfo{
					URLs: []string{url},
				}

				in.EXPECT().StorageFindSector(gomock.Any(), sectorRef.ID, storiface.FTUnsealed, gomock.Any(),
					false).Return([]stores.SectorStorageInfo{si}, nil).Times(1)
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
			pfhandler := mocks.NewMockpartialFileHandler(mockCtrl)
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

			remoteStore := stores.NewRemote(lstore, index, nil, 6000, pfhandler)

			rd, err := remoteStore.Reader(ctx, sectorRef, offset, size)

			if tc.errStr != "" {
				require.Error(t, err)
				require.Nil(t, rd)
				require.Contains(t, err.Error(), tc.errStr)
			} else {
				require.NoError(t, err)
			}

			if !tc.expectedNonNilReader {
				require.Nil(t, rd)
			} else {
				require.NotNil(t, rd)
				defer func() {
					require.NoError(t, rd.Close())
				}()

				if f, ok := rd.(*os.File); ok {
					require.NoError(t, os.Remove(f.Name()))
				}

				bz, err := ioutil.ReadAll(rd)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSectorBytes, bz)
			}

		})
	}
}

func mockSectorAcquire(l *mocks.MockStore, sectorRef storage.SectorRef, pfPath string, err error) {
	l.EXPECT().AcquireSector(gomock.Any(), sectorRef, storiface.FTUnsealed,
		storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
		Unsealed: pfPath,
	},
		storiface.SectorPaths{}, err).Times(1)
}

func mockPartialFileOpen(pf *mocks.MockpartialFileHandler, sectorSize abi.SectorSize, pfPath string, err error) {
	pf.EXPECT().OpenPartialFile(abi.PaddedPieceSize(sectorSize), pfPath).Return(&partialfile.PartialFile{},
		err).Times(1)
}

func mockCheckAllocation(pf *mocks.MockpartialFileHandler, offset, size abi.PaddedPieceSize, file *partialfile.PartialFile,
	out bool, err error) {
	pf.EXPECT().HasAllocated(file, storiface.UnpaddedByteIndex(offset.Unpadded()),
		size.Unpadded()).Return(out, err).Times(1)
}

func mockPfReader(pf *mocks.MockpartialFileHandler, file *partialfile.PartialFile, offset, size abi.PaddedPieceSize,
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
	getSectorReturnCode    int
	getSectorBytes         []byte
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

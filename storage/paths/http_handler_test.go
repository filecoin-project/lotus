package paths_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/paths/mocks"
	"github.com/filecoin-project/lotus/storage/sealer/partialfile"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestRemoteGetAllocated(t *testing.T) {

	emptyPartialFile := &partialfile.PartialFile{}
	pfPath := "path"
	expectedSectorRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  123,
			Number: 123,
		},
		ProofType: 0,
	}

	validSectorName := fmt.Sprintf("s-t0%d-%d", 123, 123)
	validSectorFileType := storiface.FTUnsealed.String()
	validSectorType := "1"
	sectorSize := abi.SealProofInfos[1].SectorSize

	validOffset := "100"
	validOffsetInt := 100

	validSize := "1000"
	validSizeInt := 1000

	type pieceInfo struct {
		sectorName string
		fileType   string
		sectorType string

		// piece info
		offset string
		size   string
	}
	validPieceInfo := pieceInfo{
		sectorName: validSectorName,
		fileType:   validSectorFileType,
		sectorType: validSectorType,
		offset:     validOffset,
		size:       validSize,
	}

	tcs := map[string]struct {
		piFnc    func(pi *pieceInfo)
		storeFnc func(s *mocks.MockStore)
		pfFunc   func(s *mocks.MockPartialFileHandler)

		// expectation
		expectedStatusCode int
	}{
		"fails when sector name is invalid": {
			piFnc: func(pi *pieceInfo) {
				pi.sectorName = "invalid"
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		"fails when file type is invalid": {
			piFnc: func(pi *pieceInfo) {
				pi.fileType = "invalid"
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		"fails when sector proof type is invalid": {
			piFnc: func(pi *pieceInfo) {
				pi.sectorType = "invalid"
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		"fails when offset is invalid": {
			piFnc: func(pi *pieceInfo) {
				pi.offset = "invalid"
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		"fails when size is invalid": {
			piFnc: func(pi *pieceInfo) {
				pi.size = "invalid"
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		"fails when errors out during acquiring unsealed sector file": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.MockStore) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: "path",
				},
					storiface.SectorPaths{}, xerrors.New("some error")).Times(1)
			},
		},
		"fails when unsealed sector file is not found locally": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.MockStore) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{},
					storiface.SectorPaths{}, nil).Times(1)
			},
		},
		"fails when error while opening partial file": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.MockStore) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil).Times(1)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				pf.EXPECT().OpenPartialFile(abi.PaddedPieceSize(sectorSize), pfPath).Return(&partialfile.PartialFile{},
					xerrors.New("some error")).Times(1)
			},
		},

		"fails when determining partial file allocation returns an error": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.MockStore) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil).Times(1)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				pf.EXPECT().OpenPartialFile(abi.PaddedPieceSize(sectorSize), pfPath).Return(emptyPartialFile,
					nil).Times(1)

				pf.EXPECT().HasAllocated(emptyPartialFile, storiface.UnpaddedByteIndex(validOffsetInt),
					abi.UnpaddedPieceSize(validSizeInt)).Return(true, xerrors.New("some error")).Times(1)
			},
		},
		"StatusRequestedRangeNotSatisfiable when piece is NOT allocated in partial file": {
			expectedStatusCode: http.StatusRequestedRangeNotSatisfiable,
			storeFnc: func(l *mocks.MockStore) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil).Times(1)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				pf.EXPECT().OpenPartialFile(abi.PaddedPieceSize(sectorSize), pfPath).Return(emptyPartialFile,
					nil).Times(1)

				pf.EXPECT().HasAllocated(emptyPartialFile, storiface.UnpaddedByteIndex(validOffsetInt),
					abi.UnpaddedPieceSize(validSizeInt)).Return(false, nil).Times(1)
			},
		},
		"OK when piece is allocated in partial file": {
			expectedStatusCode: http.StatusOK,
			storeFnc: func(l *mocks.MockStore) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil).Times(1)
			},

			pfFunc: func(pf *mocks.MockPartialFileHandler) {
				pf.EXPECT().OpenPartialFile(abi.PaddedPieceSize(sectorSize), pfPath).Return(emptyPartialFile,
					nil).Times(1)

				pf.EXPECT().HasAllocated(emptyPartialFile, storiface.UnpaddedByteIndex(validOffsetInt),
					abi.UnpaddedPieceSize(validSizeInt)).Return(true, nil).Times(1)
			},
		},
	}

	for name, tc := range tcs {
		tc := tc
		t.Run(name, func(t *testing.T) {
			// create go mock controller here
			mockCtrl := gomock.NewController(t)
			// when test is done, assert expectations on all mock objects.
			defer mockCtrl.Finish()

			lstore := mocks.NewMockStore(mockCtrl)
			pfhandler := mocks.NewMockPartialFileHandler(mockCtrl)

			handler := &paths.FetchHandler{
				Local:     lstore,
				PfHandler: pfhandler,
			}

			// run http server
			ts := httptest.NewServer(handler)
			defer ts.Close()

			pi := validPieceInfo

			if tc.piFnc != nil {
				tc.piFnc(&pi)
			}

			if tc.storeFnc != nil {
				tc.storeFnc(lstore)
			}
			if tc.pfFunc != nil {
				tc.pfFunc(pfhandler)
			}

			// call remoteGetAllocated
			url := fmt.Sprintf("%s/remote/%s/%s/%s/allocated/%s/%s",
				ts.URL,
				pi.fileType,
				pi.sectorName,
				pi.sectorType,
				pi.offset,
				pi.size)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()

			// assert expected status code
			require.Equal(t, tc.expectedStatusCode, resp.StatusCode)
		})
	}
}

func TestRemoteGetSector(t *testing.T) {
	str := "hello-world"
	fileBytes := []byte(str)

	validSectorName := fmt.Sprintf("s-t0%d-%d", 123, 123)
	validSectorFileType := storiface.FTUnsealed.String()
	expectedSectorRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  123,
			Number: 123,
		},
		ProofType: 0,
	}

	type sectorInfo struct {
		sectorName string
		fileType   string
	}
	validSectorInfo := sectorInfo{
		sectorName: validSectorName,
		fileType:   validSectorFileType,
	}

	tcs := map[string]struct {
		siFnc    func(pi *sectorInfo)
		storeFnc func(s *mocks.MockStore, path string)

		// reading a file or a dir
		isDir bool

		// expectation
		noResponseBytes       bool
		expectedContentType   string
		expectedStatusCode    int
		expectedResponseBytes []byte
	}{
		"fails when sector name is invalid": {
			siFnc: func(si *sectorInfo) {
				si.sectorName = "invalid"
			},
			expectedStatusCode: http.StatusInternalServerError,
			noResponseBytes:    true,
		},
		"fails when file type is invalid": {
			siFnc: func(si *sectorInfo) {
				si.fileType = "invalid"
			},
			expectedStatusCode: http.StatusInternalServerError,
			noResponseBytes:    true,
		},
		"fails when error while acquiring sector file": {
			storeFnc: func(l *mocks.MockStore, _ string) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: "path",
				},
					storiface.SectorPaths{}, xerrors.New("some error")).Times(1)
			},
			expectedStatusCode: http.StatusInternalServerError,
			noResponseBytes:    true,
		},
		"fails when acquired sector file path is empty": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.MockStore, _ string) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{},
					storiface.SectorPaths{}, nil).Times(1)
			},
			noResponseBytes: true,
		},
		"fails when acquired file does not exist": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.MockStore, _ string) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: "path",
				},
					storiface.SectorPaths{}, nil)
			},
			noResponseBytes: true,
		},
		"successfully read a sector file": {
			storeFnc: func(l *mocks.MockStore, path string) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: path,
				},
					storiface.SectorPaths{}, nil)
			},

			noResponseBytes:       false,
			expectedContentType:   "application/octet-stream",
			expectedStatusCode:    200,
			expectedResponseBytes: fileBytes,
		},
		"successfully read a sector dir": {
			storeFnc: func(l *mocks.MockStore, path string) {

				l.EXPECT().AcquireSector(gomock.Any(), expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: path,
				},
					storiface.SectorPaths{}, nil)
			},

			isDir:                 true,
			noResponseBytes:       false,
			expectedContentType:   "application/x-tar",
			expectedStatusCode:    200,
			expectedResponseBytes: fileBytes,
		},
	}

	for name, tc := range tcs {
		tc := tc
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			// when test is done, assert expectations on all mock objects.
			defer mockCtrl.Finish()
			lstore := mocks.NewMockStore(mockCtrl)
			pfhandler := mocks.NewMockPartialFileHandler(mockCtrl)

			var path string

			if !tc.isDir {
				// create file
				tempFile, err := os.CreateTemp("", "TestRemoteGetSector-")
				require.NoError(t, err)

				defer func() {
					_ = os.Remove(tempFile.Name())
				}()

				_, err = tempFile.Write(fileBytes)
				require.NoError(t, err)
				path = tempFile.Name()
			} else {
				// create dir with a file
				tempFile2, err := os.CreateTemp("", "TestRemoteGetSector-")
				require.NoError(t, err)
				defer func() {
					_ = os.Remove(tempFile2.Name())
				}()

				stat, err := os.Stat(tempFile2.Name())
				require.NoError(t, err)
				tempDir := t.TempDir()

				require.NoError(t, os.Rename(tempFile2.Name(), filepath.Join(tempDir, stat.Name())))

				path = tempDir
			}

			handler := &paths.FetchHandler{
				lstore,
				pfhandler,
			}

			// run http server
			ts := httptest.NewServer(handler)
			defer ts.Close()

			si := validSectorInfo
			if tc.siFnc != nil {
				tc.siFnc(&si)
			}

			if tc.storeFnc != nil {
				tc.storeFnc(lstore, path)
			}

			// call remoteGetAllocated
			url := fmt.Sprintf("%s/remote/%s/%s",
				ts.URL,
				si.fileType,
				si.sectorName,
			)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()

			bz, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// assert expected status code
			require.Equal(t, tc.expectedStatusCode, resp.StatusCode)

			if !tc.noResponseBytes {
				if !tc.isDir {
					require.EqualValues(t, tc.expectedResponseBytes, bz)
				}
			}

			require.Equal(t, tc.expectedContentType, resp.Header.Get("Content-Type"))
		})
	}
}

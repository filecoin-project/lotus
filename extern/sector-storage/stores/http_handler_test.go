package stores_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/partialfile"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores/mocks"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestRemoteGetAllocated(t *testing.T) {

	emptyPartialFile := &partialfile.PartialFile{}
	pfPath := "path"
	expectedSectorRef := storage.SectorRef{
		ID: abi.SectorID{
			123,
			123,
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
		storeFnc func(s *mocks.Store)
		pfFunc   func(s *mocks.PartialFileHandler)

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
			storeFnc: func(l *mocks.Store) {
				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: "path",
				},
					storiface.SectorPaths{}, xerrors.New("some error"))
			},
		},
		"fails when unsealed sector file is not found locally": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.Store) {

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{},
					storiface.SectorPaths{}, nil)
			},
		},
		"fails when partial file is not found locally": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.Store) {
				// will return emppty paths

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil)
			},

			pfFunc: func(pf *mocks.PartialFileHandler) {
				//OpenPartialFile(maxPieceSize abi.PaddedPieceSize, path string)
				pf.On("OpenPartialFile", abi.PaddedPieceSize(sectorSize), pfPath).Return(&partialfile.PartialFile{},
					xerrors.New("some error"))
			},
		},

		"fails when determining partial file allocation returns an error": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.Store) {
				// will return emppty paths

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil)
			},

			pfFunc: func(pf *mocks.PartialFileHandler) {
				pf.On("OpenPartialFile", abi.PaddedPieceSize(sectorSize), pfPath).Return(emptyPartialFile,
					nil)
				pf.On("HasAllocated", emptyPartialFile, storiface.UnpaddedByteIndex(validOffsetInt),
					abi.UnpaddedPieceSize(validSizeInt)).Return(true, xerrors.New("some error"))
			},
		},
		"StatusRequestedRangeNotSatisfiable when piece is NOT allocated in partial file": {
			expectedStatusCode: http.StatusRequestedRangeNotSatisfiable,
			storeFnc: func(l *mocks.Store) {
				// will return emppty paths

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil)
			},

			pfFunc: func(pf *mocks.PartialFileHandler) {
				pf.On("OpenPartialFile", abi.PaddedPieceSize(sectorSize), pfPath).Return(emptyPartialFile,
					nil)
				pf.On("HasAllocated", emptyPartialFile, storiface.UnpaddedByteIndex(validOffsetInt),
					abi.UnpaddedPieceSize(validSizeInt)).Return(false, nil)
			},
		},
		"OK when piece is allocated in partial file": {
			expectedStatusCode: http.StatusOK,
			storeFnc: func(l *mocks.Store) {
				// will return emppty paths

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: pfPath,
				},
					storiface.SectorPaths{}, nil)
			},

			pfFunc: func(pf *mocks.PartialFileHandler) {
				pf.On("OpenPartialFile", abi.PaddedPieceSize(sectorSize), pfPath).Return(emptyPartialFile,
					nil)
				pf.On("HasAllocated", emptyPartialFile, storiface.UnpaddedByteIndex(validOffsetInt),
					abi.UnpaddedPieceSize(validSizeInt)).Return(true, nil)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			lstore := &mocks.Store{}
			pfhandler := &mocks.PartialFileHandler{}

			handler := &stores.FetchHandler{
				lstore,
				pfhandler,
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
			defer resp.Body.Close()

			// assert expected status code
			require.Equal(t, tc.expectedStatusCode, resp.StatusCode)

			// assert expectations on the mocks
			lstore.AssertExpectations(t)
		})
	}
}

func TestRemoteGetSector(t *testing.T) {
	str := "hello-world"
	fileBytes := []byte(str)

	validSectorName := fmt.Sprintf("s-t0%d-%d", 123, 123)
	validSectorFileType := storiface.FTUnsealed.String()
	expectedSectorRef := storage.SectorRef{
		ID: abi.SectorID{
			123,
			123,
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
		storeFnc func(s *mocks.Store, path string)

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
			storeFnc: func(l *mocks.Store, _ string) {
				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: "path",
				},
					storiface.SectorPaths{}, xerrors.New("some error"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			noResponseBytes:    true,
		},
		"fails when acquired sector file path is empty": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.Store, _ string) {

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{},
					storiface.SectorPaths{}, nil)
			},
			noResponseBytes: true,
		},
		"fails when acquired file does not exist": {
			expectedStatusCode: http.StatusInternalServerError,
			storeFnc: func(l *mocks.Store, _ string) {

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
					storiface.FTNone, storiface.PathStorage, storiface.AcquireMove).Return(storiface.SectorPaths{
					Unsealed: "path",
				},
					storiface.SectorPaths{}, nil)
			},
			noResponseBytes: true,
		},
		"successfully read a sector file": {
			storeFnc: func(l *mocks.Store, path string) {

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
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
			storeFnc: func(l *mocks.Store, path string) {

				l.On("AcquireSector", mock.Anything, expectedSectorRef, storiface.FTUnsealed,
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
		t.Run(name, func(t *testing.T) {
			var path string

			if !tc.isDir {
				// create file
				tempFile, err := ioutil.TempFile("", "TestRemoteGetSector-")
				require.NoError(t, err)
				defer os.Remove(tempFile.Name())
				_, err = tempFile.Write(fileBytes)
				require.NoError(t, err)
				path = tempFile.Name()
			} else {
				// create dir with a file
				tempFile2, err := ioutil.TempFile("", "TestRemoteGetSector-")
				require.NoError(t, err)
				defer os.Remove(tempFile2.Name())
				stat, err := os.Stat(tempFile2.Name())
				require.NoError(t, err)
				tempDir, err := ioutil.TempDir("", "TestRemoteGetSector-")
				require.NoError(t, err)
				defer os.RemoveAll(tempDir)
				require.NoError(t, os.Rename(tempFile2.Name(), filepath.Join(tempDir, stat.Name())))

				path = tempDir
			}

			lstore := &mocks.Store{}
			pfhandler := &mocks.PartialFileHandler{}

			handler := &stores.FetchHandler{
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
			defer resp.Body.Close()

			bz, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)

			// assert expected status code
			require.Equal(t, tc.expectedStatusCode, resp.StatusCode)

			if !tc.noResponseBytes {
				if !tc.isDir {
					require.EqualValues(t, tc.expectedResponseBytes, bz)
				}
			}

			require.Equal(t, tc.expectedContentType, resp.Header.Get("Content-Type"))

			// assert expectations on the mocks
			lstore.AssertExpectations(t)
		})
	}
}

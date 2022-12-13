package shared_testutil

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/filestore"
)

var TestErrNotFound = errors.New("file not found")
var TestErrTempFile = errors.New("temp file creation failed")

// TestFileStoreParams are parameters for a test file store
type TestFileStoreParams struct {
	Files              []filestore.File
	AvailableTempFiles []filestore.File
	ExpectedDeletions  []filestore.Path
	ExpectedOpens      []filestore.Path
}

// TestFileStore is a mocked file store that can provide programmed returns
// and test expectations
type TestFileStore struct {
	files              []filestore.File
	availableTempFiles []filestore.File
	expectedDeletions  map[filestore.Path]struct{}
	expectedOpens      map[filestore.Path]struct{}
	deletedFiles       map[filestore.Path]struct{}
	openedFiles        map[filestore.Path]struct{}
}

// NewTestFileStore returns a new test file store from the given parameters
func NewTestFileStore(params TestFileStoreParams) *TestFileStore {
	fs := &TestFileStore{
		files:              params.Files,
		availableTempFiles: params.AvailableTempFiles,
		expectedDeletions:  make(map[filestore.Path]struct{}),
		expectedOpens:      make(map[filestore.Path]struct{}),
		deletedFiles:       make(map[filestore.Path]struct{}),
		openedFiles:        make(map[filestore.Path]struct{}),
	}
	for _, path := range params.ExpectedDeletions {
		fs.expectedDeletions[path] = struct{}{}
	}
	for _, path := range params.ExpectedOpens {
		fs.expectedOpens[path] = struct{}{}
	}
	return fs
}

// Open will open a file if it's in the file store
func (fs *TestFileStore) Open(p filestore.Path) (filestore.File, error) {
	var foundFile filestore.File
	for _, file := range fs.files {
		if p == file.Path() {
			foundFile = file
			break
		}
	}
	if foundFile == nil {
		return nil, TestErrNotFound
	}
	fs.openedFiles[p] = struct{}{}
	return foundFile, nil
}

// Create is not implement
func (fs *TestFileStore) Create(p filestore.Path) (filestore.File, error) {
	panic("not implemented")
}

// Store is not implemented
func (fs *TestFileStore) Store(p filestore.Path, f filestore.File) (filestore.Path, error) {
	panic("not implemented")
}

// Delete will delete a file if it is in the file store
func (fs *TestFileStore) Delete(p filestore.Path) error {
	var foundFile filestore.File
	for i, file := range fs.files {
		if p == file.Path() {
			foundFile = file
			fs.files[i] = fs.files[len(fs.files)-1]
			fs.files[len(fs.files)-1] = nil
			fs.files = fs.files[:len(fs.files)-1]
			break
		}
	}
	if foundFile == nil {
		return TestErrNotFound
	}
	fs.deletedFiles[p] = struct{}{}
	return nil
}

// CreateTemp will create a temporary file from the provided set of temporary files
func (fs *TestFileStore) CreateTemp() (filestore.File, error) {
	if len(fs.availableTempFiles) == 0 {
		return nil, TestErrTempFile
	}
	var tempFile filestore.File
	tempFile, fs.availableTempFiles = fs.availableTempFiles[0], fs.availableTempFiles[1:]
	fs.files = append(fs.files, tempFile)
	return tempFile, nil
}

// VerifyExpectations will verify that the correct files were opened and deleted
func (fs *TestFileStore) VerifyExpectations(t *testing.T) {
	require.Equal(t, fs.openedFiles, fs.expectedOpens)
	require.Equal(t, fs.deletedFiles, fs.expectedDeletions)
}

// TestFileParams are parameters for a test file
type TestFileParams struct {
	Buffer *bytes.Buffer
	Size   int64
	Path   filestore.Path
}

// NewTestFile generates a mocked filestore.File that has programmed returns
func NewTestFile(params TestFileParams) *TestFile {
	tf := &TestFile{
		Buffer: params.Buffer,
		size:   params.Size,
		path:   params.Path,
	}
	if tf.Buffer == nil {
		tf.Buffer = new(bytes.Buffer)
	}
	if tf.size == 0 {
		tf.size = rand.Int63()
	}
	if tf.path == filestore.Path("") {
		buf := make([]byte, 16)
		_, _ = rand.Read(buf)
		tf.path = filestore.Path(buf)
	}
	return tf
}

// TestFile is a mocked version of filestore.File with preset returns
// and a byte buffer for read/writes
type TestFile struct {
	*bytes.Buffer
	size int64
	path filestore.Path
}

// Path returns the preset path
func (f *TestFile) Path() filestore.Path {
	return f.path
}

// OsPath is not implemented
func (f *TestFile) OsPath() filestore.OsPath {
	return filestore.OsPath(f.path)
}

// Size returns the preset size
func (f *TestFile) Size() int64 {
	return f.size
}

// Close does nothing
func (f *TestFile) Close() error {
	return nil
}

// Seek is not implemented
func (f *TestFile) Seek(offset int64, whence int) (int64, error) {
	panic("not implemented")
}

package filestore

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func randBytes(n int) []byte {
	arr := make([]byte, n)
	_, err := rand.Read(arr)
	if err != nil {
		log.Fatal(err)
	}
	return arr
}

const baseDir = "_test/a/b/c/d"
const existingFile = "existing.txt"

func init() {
	err := os.MkdirAll(baseDir, 0755)
	if err != nil {
		log.Print(err.Error())
		return
	}
	filename := path.Join(baseDir, existingFile)
	file, err := os.Create(filename)
	if err != nil {
		log.Print(err.Error())
		return
	}
	defer file.Close()
	_, err = file.Write(randBytes(64))
	if err != nil {
		log.Print(err.Error())
		return
	}
}

func Test_SizeFails(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	name := Path("noFile.txt")
	file, err := store.Create(name)
	require.NoError(t, err)
	err = store.Delete(file.Path())
	require.NoError(t, err)
	require.Equal(t, int64(-1), file.Size())
}

func Test_OpenFileFails(t *testing.T) {
	base := "_test/a/b/c/d/e"
	err := os.MkdirAll(base, 0755)
	require.NoError(t, err)
	store, err := NewLocalFileStore(OsPath(base))
	require.NoError(t, err)
	err = os.Remove(base)
	require.NoError(t, err)
	_, err = store.Open(existingFile)
	require.Error(t, err)
}

func Test_RemoveSeparators(t *testing.T) {
	first, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	second, err := NewLocalFileStore(OsPath(fmt.Sprintf("%s%c%c", baseDir, os.PathSeparator, os.PathSeparator)))
	require.NoError(t, err)
	f1, err := first.Open(existingFile)
	require.NoError(t, err)
	f2, err := second.Open(existingFile)
	require.NoError(t, err)
	require.Equal(t, f1.Path(), f2.Path())
}

func Test_BaseDirIsFileFails(t *testing.T) {
	base := fmt.Sprintf("%s%c%s", baseDir, os.PathSeparator, existingFile)
	_, err := NewLocalFileStore(OsPath(base))
	require.Error(t, err)
}

func Test_CreateExistingFileFails(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	_, err = store.Create(Path(existingFile))
	require.Error(t, err)
}

func Test_StoreFails(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	file, err := store.Open(Path(existingFile))
	require.NoError(t, err)
	_, err = store.Store(Path(existingFile), file)
	require.Error(t, err)
}

func Test_OpenFails(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	name := Path("newFile.txt")
	_, err = store.Open(name)
	require.Error(t, err)
}

func Test_InvalidBaseDirectory(t *testing.T) {
	_, err := NewLocalFileStore("NoSuchDirectory")
	require.Error(t, err)
}

func Test_CreateFile(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	name := Path("newFile.txt")
	f, err := store.Create(name)
	require.NoError(t, err)
	defer func() {
		err := store.Delete(f.Path())
		require.NoError(t, err)
	}()
	bytesToWrite := 32
	written, err := f.Write(randBytes(bytesToWrite))
	require.NoError(t, err)
	require.Equal(t, bytesToWrite, written)
	require.Equal(t, int64(bytesToWrite), f.Size())
}

func Test_CreateTempFile(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	file, err := store.CreateTemp()
	require.NoError(t, err)
	defer func() {
		err := store.Delete(file.Path())
		require.NoError(t, err)
	}()
	bytesToWrite := 32
	written, err := file.Write(randBytes(bytesToWrite))
	require.NoError(t, err)
	require.Equal(t, bytesToWrite, written)
	require.Equal(t, int64(bytesToWrite), file.Size())
}

func Test_OpenAndReadFile(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	file, err := store.Open(Path(existingFile))
	require.NoError(t, err)
	size := file.Size()
	require.NotEqual(t, -1, size)
	pos := int64(size / 2)
	offset, err := file.Seek(pos, 0)
	require.NoError(t, err)
	require.Equal(t, pos, offset)
	buffer := make([]byte, size/2)
	read, err := file.Read(buffer)
	require.NoError(t, err)
	require.Equal(t, int(size/2), read)
	err = file.Close()
	require.NoError(t, err)
}

func Test_CopyFile(t *testing.T) {
	store, err := NewLocalFileStore(baseDir)
	require.NoError(t, err)
	file, err := store.Open(Path(existingFile))
	require.NoError(t, err)
	newFile := Path("newFile.txt")
	newPath, err := store.Store(newFile, file)
	require.NoError(t, err)
	err = store.Delete(newPath)
	require.NoError(t, err)
}

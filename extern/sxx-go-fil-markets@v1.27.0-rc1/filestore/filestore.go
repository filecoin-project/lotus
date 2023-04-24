package filestore

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

type fileStore struct {
	base string
}

// NewLocalFileStore creates a filestore mounted on a given local directory path
func NewLocalFileStore(baseDir OsPath) (FileStore, error) {
	base, err := checkIsDir(string(baseDir))
	if err != nil {
		return nil, err
	}
	return &fileStore{base}, nil
}

func (fs fileStore) filename(p Path) string {
	return filepath.Join(fs.base, string(p))
}

func (fs fileStore) Open(p Path) (File, error) {
	name := fs.filename(p)
	if _, err := os.Stat(name); err != nil {
		return nil, fmt.Errorf("error trying to open %s: %s", name, err.Error())
	}
	return newFile(OsPath(fs.base), p)
}

func (fs fileStore) Create(p Path) (File, error) {
	name := fs.filename(p)
	if _, err := os.Stat(name); err == nil {
		return nil, fmt.Errorf("file %s already exists", name)
	}
	return newFile(OsPath(fs.base), p)
}

func (fs fileStore) Store(p Path, src File) (Path, error) {
	dest, err := fs.Create(p)
	if err != nil {
		return Path(""), err
	}

	if _, err = io.Copy(dest, src); err != nil {
		dest.Close()
		return Path(""), err
	}
	return p, dest.Close()
}

func (fs fileStore) Delete(p Path) error {
	filename := string(p)
	full := path.Join(string(fs.base), string(filename))
	return os.Remove(full)
}

func (fs fileStore) CreateTemp() (File, error) {
	f, err := ioutil.TempFile(fs.base, "fstmp")
	if err != nil {
		return nil, err
	}
	filename := filepath.Base(f.Name())
	return &fd{File: f, basepath: fs.base, filename: filename}, nil
}

func checkIsDir(baseDir string) (string, error) {
	base := filepath.Clean(string(baseDir))
	info, err := os.Stat(base)
	if err != nil {
		return "", fmt.Errorf("error getting %s info: %s", base, err.Error())
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", base)
	}
	return base, nil
}

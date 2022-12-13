package filestore

import (
	"os"
	"path"
)

type fd struct {
	*os.File
	filename string
	basepath string
}

func newFile(basepath OsPath, filename Path) (File, error) {
	var err error
	result := fd{filename: string(filename), basepath: string(basepath)}
	full := path.Join(string(basepath), string(filename))
	result.File, err = os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (f fd) Path() Path {
	return Path(f.filename)
}

func (f fd) OsPath() OsPath {
	return OsPath(f.Name())
}

func (f fd) Size() int64 {
	info, err := os.Stat(f.Name())
	if err != nil {
		return -1
	}
	return info.Size()
}

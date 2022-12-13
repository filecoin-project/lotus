package filestore

import (
	"io"
)

// Path represents an abstract path to a file
type Path string

// OsPath represents a path that can be located on
// the operating system with standard os.File operations
type OsPath string

// File is a wrapper around an os file
type File interface {
	Path() Path
	OsPath() OsPath
	Size() int64

	io.Closer
	io.Reader
	io.Writer
	io.Seeker
}

// FileStore is an abstract filestore, used for storing temporary file data
// when handing off a deal to the Storage Mining module. Files are created by
// the storage market module, their path is given to the storage mining module
// when AddPiece is called. The Storage Mining module then reads from them
// from the FileStore, and deletes them once they have been sealed in a sector
type FileStore interface {
	Open(p Path) (File, error)
	Create(p Path) (File, error)
	Store(p Path, f File) (Path, error)
	Delete(p Path) error

	CreateTemp() (File, error)
}

# filestore

The `filestore` module is a simple wrapper for os.File. It is used by [pieceio](../pieceio),
[retrievialmarket](../retrievalmarket), and [storagemarket](../storagemarket).

## Installation
```bash
go get github.com/filecoin-project/go-fil-markets/filestore
```

## FileStore
FileStore is the primary export of this module.

### Usage
To create a new local filestore mounted on a given local directory path, use:
```go
package filestore

func NewLocalFileStore(basedirectory OsPath) (FileStore, error) 
```

A FileStore provides the following functions:
* [`Open`](filestore.go)
* [`Create`](filestore.go)
* [`Store`](filestore.go)
* [`Delete`](filestore.go)
* [`CreateTemp`](filestore.go)

Please the [tests](filestore_test.go) for more information about expected behavior.
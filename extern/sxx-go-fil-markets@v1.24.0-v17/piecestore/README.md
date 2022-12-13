# piecestore

The `piecestore` module is a simple encapsulation of two data stores, one for `PieceInfo` and
 another for `CIDInfo`.  The piecestore's main goal is to help 
 [storagemarket module](../storagemarket) and [retrievalmarket module](../retrievalmarket)
 find where sealed data lives inside of sectors. Storage market writes the
 data, and retrieval market reads it.

Both markets use `CIDInfo` to look up a Piece that contains the payload, and then
 use `PieceInfo` to find the sector that contains the piece.
  
The storage market has to write this data before it completes the deal in order to later
 look up the payload when the data is served.

## Installation
```bash
go get github.com/filecoin-project/go-fil-markets/piecestore
```

### PieceStore
`PieceStore` is primary export of this module. It is a database 
of piece info that can be modified and queried. The PieceStore 
interface is implemented in [piecestore.go](./piecestore.go).

It has two stores, one for `PieceInfo` keyed by `pieceCID`, and another for 
`CIDInfo`, keyed by `payloadCID`. These keys are of type `cid.CID`; see 
[github.com/ipfs/go-cid](https://github.com/ipfs/go-cid).

**To initialize a PieceStore**
```go
func NewPieceStore(ds datastore.Batching) PieceStore
```

**Parameters**
* `ds datastore.Batching` is a datastore for the deal's state machine. It is
 typically the node's own datastore that implements the IPFS datastore.Batching interface.
 See
  [github.com/ipfs/go-datastore](https://github.com/ipfs/go-datastore).


`PieceStore` implements the following functions:

* [`AddDealForPiece`](./piecestore.go)
* [`AddPieceBlockLocations`](./piecestore.go)
* [`GetPieceInfo`](./piecestore.go)
* [`GetCIDInfo`](./piecestore.go)

Please the [tests](piecestore_test.go) for more information about expected behavior.
package requestvalidation

import (
	_ "embed"
	"errors"

	"github.com/ipfs/go-cid"
	bindnoderegistry "github.com/ipld/go-ipld-prime/node/bindnode/registry"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

//go:generate cbor-gen-for StorageDataTransferVoucher

//go:embed types.ipldsch
var embedSchema []byte

var (
	// ErrWrongVoucherType means the voucher was not the correct type can validate against
	ErrWrongVoucherType = errors.New("cannot validate voucher type")

	// ErrNoPushAccepted just means clients do not accept pushes for storage deals
	ErrNoPushAccepted = errors.New("client should not receive data for a storage deal")

	// ErrNoPullAccepted just means providers do not accept pulls for storage deals
	ErrNoPullAccepted = errors.New("provider should not send data for a storage deal")

	// ErrNoDeal means no active deal was found for this vouchers proposal cid
	ErrNoDeal = errors.New("no deal found for this proposal")

	// ErrWrongPeer means that the other peer for this data transfer request does not match
	// the other peer for the deal
	ErrWrongPeer = errors.New("data Transfer peer id and Deal peer id do not match")

	// ErrWrongPiece means that the pieceref for this data transfer request does not match
	// the one specified in the deal
	ErrWrongPiece = errors.New("base CID for deal does not match CID for piece")

	// ErrInacceptableDealState means the deal for this transfer is not in a deal state
	// where transfer can be performed
	ErrInacceptableDealState = errors.New("deal is not in a state where deals are accepted")

	// DataTransferStates are the states in which it would make sense to actually start a data transfer
	// We accept deals even in the StorageDealTransferring state too as we could also also receive a data transfer restart request
	DataTransferStates = []storagemarket.StorageDealStatus{storagemarket.StorageDealValidating, storagemarket.StorageDealWaitingForData, storagemarket.StorageDealUnknown,
		storagemarket.StorageDealTransferring, storagemarket.StorageDealProviderTransferAwaitRestart}
)

// StorageDataTransferVoucher is the voucher type for data transfers
// used by the storage market
type StorageDataTransferVoucher struct {
	Proposal cid.Cid
}

// StorageDataTransferVoucherType is the unique string identifier for a StorageDataTransferVoucher
const StorageDataTransferVoucherType = datatransfer.TypeIdentifier("StorageDataTransferVoucher")

var BindnodeRegistry = bindnoderegistry.NewRegistry()

func init() {
	if err := BindnodeRegistry.RegisterType((*StorageDataTransferVoucher)(nil), string(embedSchema), "StorageDataTransferVoucher"); err != nil {
		panic(err.Error())
	}
}

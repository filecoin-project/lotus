package api

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
)

func init() {
	cbor.RegisterCborType(SealedRef{})
}

type Common interface {
	// Auth
	AuthVerify(ctx context.Context, token string) ([]string, error)
	AuthNew(ctx context.Context, perms []string) ([]byte, error)

	// network

	NetConnectedness(context.Context, peer.ID) (network.Connectedness, error)
	NetPeers(context.Context) ([]peer.AddrInfo, error)
	NetConnect(context.Context, peer.AddrInfo) error
	NetAddrsListen(context.Context) (peer.AddrInfo, error)
	NetDisconnect(context.Context, peer.ID) error

	// ID returns peerID of libp2p node backing this API
	ID(context.Context) (peer.ID, error)

	// Version provides information about API provider
	Version(context.Context) (Version, error)
}

// FullNode API is a low-level interface to the Filecoin network full node
type FullNode interface {
	Common

	// chain
	ChainNotify(context.Context) (<-chan *store.HeadChange, error)
	ChainHead(context.Context) (*types.TipSet, error)                // TODO: check serialization
	ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error // TODO: check serialization
	ChainGetRandomness(context.Context, *types.TipSet) ([]byte, error)
	ChainWaitMsg(context.Context, cid.Cid) (*MsgWait, error)
	ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*BlockMessages, error)
	ChainGetBlockReceipts(context.Context, cid.Cid) ([]*types.MessageReceipt, error)
	ChainGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error)
	ChainReadState(ctx context.Context, act *types.Actor, ts *types.TipSet) (*ActorState, error)

	// if tipset is nil, we'll use heaviest
	ChainCall(context.Context, *types.Message, *types.TipSet) (*types.MessageReceipt, error)

	// messages

	MpoolPending(context.Context, *types.TipSet) ([]*types.SignedMessage, error)
	MpoolPush(context.Context, *types.SignedMessage) error
	MpoolGetNonce(context.Context, address.Address) (uint64, error)

	// FullNodeStruct

	// miner

	MinerRegister(context.Context, address.Address) error
	MinerUnregister(context.Context, address.Address) error
	MinerAddresses(context.Context) ([]address.Address, error)
	MinerCreateBlock(context.Context, address.Address, *types.TipSet, []*types.Ticket, types.ElectionProof, []*types.SignedMessage, uint64) (*chain.BlockMsg, error)

	// // UX ?

	// wallet

	WalletNew(context.Context, string) (address.Address, error)
	WalletHas(context.Context, address.Address) (bool, error)
	WalletList(context.Context) ([]address.Address, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
	WalletSignMessage(context.Context, address.Address, *types.Message) (*types.SignedMessage, error)
	WalletDefaultAddress(context.Context) (address.Address, error)

	// Other

	// ClientImport imports file under the specified path into filestore
	ClientImport(ctx context.Context, path string) (cid.Cid, error)
	ClientStartDeal(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error)
	ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error)
	ClientFindData(ctx context.Context, root cid.Cid) ([]QueryOffer, error) // TODO: specify serialization mode we want (defaults to unixfs for now)
	ClientRetrieve(ctx context.Context, order RetrievalOrder) error // TODO: maybe just allow putting this straight into some file

	// ClientUnimport removes references to the specified file from filestore
	//ClientUnimport(path string)

	// ClientListImports lists imported files and their root CIDs
	ClientListImports(ctx context.Context) ([]Import, error)

	//ClientListAsks() []Ask

	StateMinerSectors(context.Context, address.Address) ([]*SectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address) ([]*SectorInfo, error)
	StateMinerPower(context.Context, address.Address, *types.TipSet) (MinerPower, error)

	PaychCreate(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, error)
	PaychList(context.Context) ([]address.Address, error)
	PaychStatus(context.Context, address.Address) (*PaychStatus, error)
	PaychClose(context.Context, address.Address) (cid.Cid, error)
	PaychVoucherCheckValid(context.Context, address.Address, *types.SignedVoucher) error
	PaychVoucherCheckSpendable(context.Context, address.Address, *types.SignedVoucher, []byte, []byte) (bool, error)
	PaychVoucherCreate(context.Context, address.Address, types.BigInt, uint64) (*types.SignedVoucher, error)
	PaychVoucherAdd(context.Context, address.Address, *types.SignedVoucher) error
	PaychVoucherList(context.Context, address.Address) ([]*types.SignedVoucher, error)
	PaychVoucherSubmit(context.Context, address.Address, *types.SignedVoucher) (cid.Cid, error)
}

// Full API is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddresses(context.Context) ([]address.Address, error)

	// Temp api for testing
	StoreGarbageData(context.Context) (uint64, error)

	// Get the status of a given sector by ID
	SectorsStatus(context.Context, uint64) (sectorbuilder.SectorSealingStatus, error)

	// List all staged sectors
	SectorsStagedList(context.Context) ([]sectorbuilder.StagedSectorMetadata, error)

	// Seal all staged sectors
	SectorsStagedSeal(context.Context) error

	SectorsRefs(context.Context) (map[string][]SealedRef, error)
}

// Version provides various build-time information
type Version struct {
	Version string

	// APIVersion is a binary encoded semver version of the remote implementing
	// this api
	//
	// See APIVersion in build/version.go
	APIVersion uint32

	// TODO: git commit / os / genesis cid?
}

type Import struct {
	Status   filestore.Status
	Key      cid.Cid
	FilePath string
	Size     uint64
}

type MsgWait struct {
	InBlock cid.Cid
	Receipt types.MessageReceipt
}

type BlockMessages struct {
	BlsMessages   []*types.Message
	SecpkMessages []*types.SignedMessage
}

type SectorInfo struct {
	SectorID uint64
	CommD    []byte
	CommR    []byte
}

type ActorState struct {
	Balance types.BigInt
	State   interface{}
}

type PaychStatus struct{}

type MinerPower struct {
	MinerPower types.BigInt
	TotalPower types.BigInt
}

type SealedRef struct {
	Piece  string
	Offset uint64
	Size   uint32
}

type QueryOffer struct {
	Err string

	Root cid.Cid

	Size     uint64
	MinPrice types.BigInt

	Miner       address.Address
	MinerPeerID peer.ID
}

func (o *QueryOffer) Order() RetrievalOrder {
	return RetrievalOrder{
		Root:        o.Root,
		Size:        o.Size,
		Miner:       o.Miner,
		MinerPeerID: o.MinerPeerID,
	}
}

type RetrievalOrder struct {
	// TODO: make this loss unixfs specific
	Root cid.Cid
	Size uint64
	// TODO: support offset

	Miner       address.Address
	MinerPeerID peer.ID
}

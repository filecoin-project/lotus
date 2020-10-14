package apistruct

import (
	"context"
	"io"
	"time"

	stnetwork "github.com/filecoin-project/go-state-types/network"

	"github.com/ipfs/go-cid"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	marketevents "github.com/filecoin-project/lotus/markets/loggers"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// All permissions are listed in permissioned.go
var _ = AllPermissions

type CommonStruct struct {
	Internal struct {
		AuthVerify func(ctx context.Context, token string) ([]auth.Permission, error) `perm:"read"`
		AuthNew    func(ctx context.Context, perms []auth.Permission) ([]byte, error) `perm:"admin"`

		NetConnectedness            func(context.Context, peer.ID) (network.Connectedness, error)    `perm:"read"`
		NetPeers                    func(context.Context) ([]peer.AddrInfo, error)                   `perm:"read"`
		NetConnect                  func(context.Context, peer.AddrInfo) error                       `perm:"write"`
		NetAddrsListen              func(context.Context) (peer.AddrInfo, error)                     `perm:"read"`
		NetDisconnect               func(context.Context, peer.ID) error                             `perm:"write"`
		NetFindPeer                 func(context.Context, peer.ID) (peer.AddrInfo, error)            `perm:"read"`
		NetPubsubScores             func(context.Context) ([]api.PubsubScore, error)                 `perm:"read"`
		NetAutoNatStatus            func(context.Context) (api.NatInfo, error)                       `perm:"read"`
		NetBandwidthStats           func(ctx context.Context) (metrics.Stats, error)                 `perm:"read"`
		NetBandwidthStatsByPeer     func(ctx context.Context) (map[string]metrics.Stats, error)      `perm:"read"`
		NetBandwidthStatsByProtocol func(ctx context.Context) (map[protocol.ID]metrics.Stats, error) `perm:"read"`
		NetAgentVersion             func(ctx context.Context, p peer.ID) (string, error)             `perm:"read"`

		ID      func(context.Context) (peer.ID, error)     `perm:"read"`
		Version func(context.Context) (api.Version, error) `perm:"read"`

		LogList     func(context.Context) ([]string, error)     `perm:"write"`
		LogSetLevel func(context.Context, string, string) error `perm:"write"`

		Shutdown func(context.Context) error                    `perm:"admin"`
		Closing  func(context.Context) (<-chan struct{}, error) `perm:"read"`
	}
}

// FullNodeStruct implements API passing calls to user-provided function values.
type FullNodeStruct struct {
	CommonStruct

	Internal struct {
		ChainNotify                   func(context.Context) (<-chan []*api.HeadChange, error)                                                            `perm:"read"`
		ChainHead                     func(context.Context) (*types.TipSet, error)                                                                       `perm:"read"`
		ChainGetRandomnessFromTickets func(context.Context, types.TipSetKey, crypto.DomainSeparationTag, abi.ChainEpoch, []byte) (abi.Randomness, error) `perm:"read"`
		ChainGetRandomnessFromBeacon  func(context.Context, types.TipSetKey, crypto.DomainSeparationTag, abi.ChainEpoch, []byte) (abi.Randomness, error) `perm:"read"`
		ChainGetBlock                 func(context.Context, cid.Cid) (*types.BlockHeader, error)                                                         `perm:"read"`
		ChainGetTipSet                func(context.Context, types.TipSetKey) (*types.TipSet, error)                                                      `perm:"read"`
		ChainGetBlockMessages         func(context.Context, cid.Cid) (*api.BlockMessages, error)                                                         `perm:"read"`
		ChainGetParentReceipts        func(context.Context, cid.Cid) ([]*types.MessageReceipt, error)                                                    `perm:"read"`
		ChainGetParentMessages        func(context.Context, cid.Cid) ([]api.Message, error)                                                              `perm:"read"`
		ChainGetTipSetByHeight        func(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)                                      `perm:"read"`
		ChainReadObj                  func(context.Context, cid.Cid) ([]byte, error)                                                                     `perm:"read"`
		ChainDeleteObj                func(context.Context, cid.Cid) error                                                                               `perm:"admin"`
		ChainHasObj                   func(context.Context, cid.Cid) (bool, error)                                                                       `perm:"read"`
		ChainStatObj                  func(context.Context, cid.Cid, cid.Cid) (api.ObjStat, error)                                                       `perm:"read"`
		ChainSetHead                  func(context.Context, types.TipSetKey) error                                                                       `perm:"admin"`
		ChainGetGenesis               func(context.Context) (*types.TipSet, error)                                                                       `perm:"read"`
		ChainTipSetWeight             func(context.Context, types.TipSetKey) (types.BigInt, error)                                                       `perm:"read"`
		ChainGetNode                  func(ctx context.Context, p string) (*api.IpldObject, error)                                                       `perm:"read"`
		ChainGetMessage               func(context.Context, cid.Cid) (*types.Message, error)                                                             `perm:"read"`
		ChainGetPath                  func(context.Context, types.TipSetKey, types.TipSetKey) ([]*api.HeadChange, error)                                 `perm:"read"`
		ChainExport                   func(context.Context, abi.ChainEpoch, bool, types.TipSetKey) (<-chan []byte, error)                                `perm:"read"`

		BeaconGetEntry func(ctx context.Context, epoch abi.ChainEpoch) (*types.BeaconEntry, error) `perm:"read"`

		GasEstimateGasPremium func(context.Context, uint64, address.Address, int64, types.TipSetKey) (types.BigInt, error)         `perm:"read"`
		GasEstimateGasLimit   func(context.Context, *types.Message, types.TipSetKey) (int64, error)                                `perm:"read"`
		GasEstimateFeeCap     func(context.Context, *types.Message, int64, types.TipSetKey) (types.BigInt, error)                  `perm:"read"`
		GasEstimateMessageGas func(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error) `perm:"read"`

		SyncState          func(context.Context) (*api.SyncState, error)                `perm:"read"`
		SyncSubmitBlock    func(ctx context.Context, blk *types.BlockMsg) error         `perm:"write"`
		SyncIncomingBlocks func(ctx context.Context) (<-chan *types.BlockHeader, error) `perm:"read"`
		SyncCheckpoint     func(ctx context.Context, key types.TipSetKey) error         `perm:"admin"`
		SyncMarkBad        func(ctx context.Context, bcid cid.Cid) error                `perm:"admin"`
		SyncUnmarkBad      func(ctx context.Context, bcid cid.Cid) error                `perm:"admin"`
		SyncUnmarkAllBad   func(ctx context.Context) error                              `perm:"admin"`
		SyncCheckBad       func(ctx context.Context, bcid cid.Cid) (string, error)      `perm:"read"`
		SyncValidateTipset func(ctx context.Context, tsk types.TipSetKey) (bool, error) `perm:"read"`

		MpoolGetConfig func(context.Context) (*types.MpoolConfig, error) `perm:"read"`
		MpoolSetConfig func(context.Context, *types.MpoolConfig) error   `perm:"write"`

		MpoolSelect func(context.Context, types.TipSetKey, float64) ([]*types.SignedMessage, error) `perm:"read"`

		MpoolPending func(context.Context, types.TipSetKey) ([]*types.SignedMessage, error) `perm:"read"`
		MpoolClear   func(context.Context, bool) error                                      `perm:"write"`

		MpoolPush          func(context.Context, *types.SignedMessage) (cid.Cid, error) `perm:"write"`
		MpoolPushUntrusted func(context.Context, *types.SignedMessage) (cid.Cid, error) `perm:"write"`

		MpoolPushMessage func(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error) `perm:"sign"`
		MpoolGetNonce    func(context.Context, address.Address) (uint64, error)                                    `perm:"read"`
		MpoolSub         func(context.Context) (<-chan api.MpoolUpdate, error)                                     `perm:"read"`

		MinerGetBaseInfo func(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*api.MiningBaseInfo, error) `perm:"read"`
		MinerCreateBlock func(context.Context, *api.BlockTemplate) (*types.BlockMsg, error)                                   `perm:"write"`

		WalletNew             func(context.Context, types.KeyType) (address.Address, error)                        `perm:"write"`
		WalletHas             func(context.Context, address.Address) (bool, error)                                 `perm:"write"`
		WalletList            func(context.Context) ([]address.Address, error)                                     `perm:"write"`
		WalletBalance         func(context.Context, address.Address) (types.BigInt, error)                         `perm:"read"`
		WalletSign            func(context.Context, address.Address, []byte) (*crypto.Signature, error)            `perm:"sign"`
		WalletSignMessage     func(context.Context, address.Address, *types.Message) (*types.SignedMessage, error) `perm:"sign"`
		WalletVerify          func(context.Context, address.Address, []byte, *crypto.Signature) (bool, error)      `perm:"read"`
		WalletDefaultAddress  func(context.Context) (address.Address, error)                                       `perm:"write"`
		WalletSetDefault      func(context.Context, address.Address) error                                         `perm:"admin"`
		WalletExport          func(context.Context, address.Address) (*types.KeyInfo, error)                       `perm:"admin"`
		WalletImport          func(context.Context, *types.KeyInfo) (address.Address, error)                       `perm:"admin"`
		WalletDelete          func(context.Context, address.Address) error                                         `perm:"write"`
		WalletValidateAddress func(context.Context, string) (address.Address, error)                               `perm:"read"`

		ClientImport                              func(ctx context.Context, ref api.FileRef) (*api.ImportRes, error)                                                `perm:"admin"`
		ClientListImports                         func(ctx context.Context) ([]api.Import, error)                                                                   `perm:"write"`
		ClientRemoveImport                        func(ctx context.Context, importID multistore.StoreID) error                                                      `perm:"admin"`
		ClientHasLocal                            func(ctx context.Context, root cid.Cid) (bool, error)                                                             `perm:"write"`
		ClientFindData                            func(ctx context.Context, root cid.Cid, piece *cid.Cid) ([]api.QueryOffer, error)                                 `perm:"read"`
		ClientMinerQueryOffer                     func(ctx context.Context, miner address.Address, root cid.Cid, piece *cid.Cid) (api.QueryOffer, error)            `perm:"read"`
		ClientStartDeal                           func(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error)                                          `perm:"admin"`
		ClientGetDealInfo                         func(context.Context, cid.Cid) (*api.DealInfo, error)                                                             `perm:"read"`
		ClientListDeals                           func(ctx context.Context) ([]api.DealInfo, error)                                                                 `perm:"write"`
		ClientGetDealUpdates                      func(ctx context.Context) (<-chan api.DealInfo, error)                                                            `perm:"read"`
		ClientRetrieve                            func(ctx context.Context, order api.RetrievalOrder, ref *api.FileRef) error                                       `perm:"admin"`
		ClientRetrieveWithEvents                  func(ctx context.Context, order api.RetrievalOrder, ref *api.FileRef) (<-chan marketevents.RetrievalEvent, error) `perm:"admin"`
		ClientQueryAsk                            func(ctx context.Context, p peer.ID, miner address.Address) (*storagemarket.StorageAsk, error)                    `perm:"read"`
		ClientCalcCommP                           func(ctx context.Context, inpath string) (*api.CommPRet, error)                                                   `perm:"read"`
		ClientGenCar                              func(ctx context.Context, ref api.FileRef, outpath string) error                                                  `perm:"write"`
		ClientDealSize                            func(ctx context.Context, root cid.Cid) (api.DataSize, error)                                                     `perm:"read"`
		ClientListDataTransfers                   func(ctx context.Context) ([]api.DataTransferChannel, error)                                                      `perm:"write"`
		ClientDataTransferUpdates                 func(ctx context.Context) (<-chan api.DataTransferChannel, error)                                                 `perm:"write"`
		ClientRestartDataTransfer                 func(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error          `perm:"write"`
		ClientRetrieveTryRestartInsufficientFunds func(ctx context.Context, paymentChannel address.Address) error                                                   `perm:"write"`

		StateNetworkName                   func(context.Context) (dtypes.NetworkName, error)                                                                   `perm:"read"`
		StateMinerSectors                  func(context.Context, address.Address, *bitfield.BitField, types.TipSetKey) ([]*miner.SectorOnChainInfo, error)     `perm:"read"`
		StateMinerActiveSectors            func(context.Context, address.Address, types.TipSetKey) ([]*miner.SectorOnChainInfo, error)                         `perm:"read"`
		StateMinerProvingDeadline          func(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)                                        `perm:"read"`
		StateMinerPower                    func(context.Context, address.Address, types.TipSetKey) (*api.MinerPower, error)                                    `perm:"read"`
		StateMinerInfo                     func(context.Context, address.Address, types.TipSetKey) (miner.MinerInfo, error)                                    `perm:"read"`
		StateMinerDeadlines                func(context.Context, address.Address, types.TipSetKey) ([]api.Deadline, error)                                     `perm:"read"`
		StateMinerPartitions               func(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error)            `perm:"read"`
		StateMinerFaults                   func(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)                                  `perm:"read"`
		StateAllMinerFaults                func(context.Context, abi.ChainEpoch, types.TipSetKey) ([]*api.Fault, error)                                        `perm:"read"`
		StateMinerRecoveries               func(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)                                  `perm:"read"`
		StateMinerPreCommitDepositForPower func(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (types.BigInt, error)            `perm:"read"`
		StateMinerInitialPledgeCollateral  func(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (types.BigInt, error)            `perm:"read"`
		StateMinerAvailableBalance         func(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)                                       `perm:"read"`
		StateSectorPreCommitInfo           func(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) `perm:"read"`
		StateSectorGetInfo                 func(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorOnChainInfo, error)         `perm:"read"`
		StateSectorExpiration              func(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorExpiration, error)          `perm:"read"`
		StateSectorPartition               func(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorLocation, error)            `perm:"read"`
		StateCall                          func(context.Context, *types.Message, types.TipSetKey) (*api.InvocResult, error)                                    `perm:"read"`
		StateReplay                        func(context.Context, types.TipSetKey, cid.Cid) (*api.InvocResult, error)                                           `perm:"read"`
		StateGetActor                      func(context.Context, address.Address, types.TipSetKey) (*types.Actor, error)                                       `perm:"read"`
		StateReadState                     func(context.Context, address.Address, types.TipSetKey) (*api.ActorState, error)                                    `perm:"read"`
		StateMsgGasCost                    func(context.Context, cid.Cid, types.TipSetKey) (*api.MsgGasCost, error)                                            `perm:"read"`
		StateWaitMsg                       func(ctx context.Context, cid cid.Cid, confidence uint64) (*api.MsgLookup, error)                                   `perm:"read"`
		StateWaitMsgLimited                func(context.Context, cid.Cid, uint64, abi.ChainEpoch) (*api.MsgLookup, error)                                      `perm:"read"`
		StateSearchMsg                     func(context.Context, cid.Cid) (*api.MsgLookup, error)                                                              `perm:"read"`
		StateListMiners                    func(context.Context, types.TipSetKey) ([]address.Address, error)                                                   `perm:"read"`
		StateListActors                    func(context.Context, types.TipSetKey) ([]address.Address, error)                                                   `perm:"read"`
		StateMarketBalance                 func(context.Context, address.Address, types.TipSetKey) (api.MarketBalance, error)                                  `perm:"read"`
		StateMarketParticipants            func(context.Context, types.TipSetKey) (map[string]api.MarketBalance, error)                                        `perm:"read"`
		StateMarketDeals                   func(context.Context, types.TipSetKey) (map[string]api.MarketDeal, error)                                           `perm:"read"`
		StateMarketStorageDeal             func(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)                                         `perm:"read"`
		StateLookupID                      func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)                       `perm:"read"`
		StateAccountKey                    func(context.Context, address.Address, types.TipSetKey) (address.Address, error)                                    `perm:"read"`
		StateChangedActors                 func(context.Context, cid.Cid, cid.Cid) (map[string]types.Actor, error)                                             `perm:"read"`
		StateGetReceipt                    func(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)                                      `perm:"read"`
		StateMinerSectorCount              func(context.Context, address.Address, types.TipSetKey) (api.MinerSectors, error)                                   `perm:"read"`
		StateListMessages                  func(ctx context.Context, match *types.Message, tsk types.TipSetKey, toht abi.ChainEpoch) ([]cid.Cid, error)        `perm:"read"`
		StateCompute                       func(context.Context, abi.ChainEpoch, []*types.Message, types.TipSetKey) (*api.ComputeStateOutput, error)           `perm:"read"`
		StateVerifierStatus                func(context.Context, address.Address, types.TipSetKey) (*abi.StoragePower, error)                                  `perm:"read"`
		StateVerifiedClientStatus          func(context.Context, address.Address, types.TipSetKey) (*abi.StoragePower, error)                                  `perm:"read"`
		StateVerifiedRegistryRootKey       func(ctx context.Context, tsk types.TipSetKey) (address.Address, error)                                             `perm:"read"`
		StateDealProviderCollateralBounds  func(context.Context, abi.PaddedPieceSize, bool, types.TipSetKey) (api.DealCollateralBounds, error)                 `perm:"read"`
		StateCirculatingSupply             func(context.Context, types.TipSetKey) (abi.TokenAmount, error)                                                     `perm:"read"`
		StateVMCirculatingSupplyInternal   func(context.Context, types.TipSetKey) (api.CirculatingSupply, error)                                               `perm:"read"`
		StateNetworkVersion                func(context.Context, types.TipSetKey) (stnetwork.Version, error)                                                   `perm:"read"`

		MsigGetAvailableBalance func(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)                                                                    `perm:"read"`
		MsigGetVestingSchedule  func(context.Context, address.Address, types.TipSetKey) (api.MsigVesting, error)                                                                 `perm:"read"`
		MsigGetVested           func(context.Context, address.Address, types.TipSetKey, types.TipSetKey) (types.BigInt, error)                                                   `perm:"read"`
		MsigCreate              func(context.Context, uint64, []address.Address, abi.ChainEpoch, types.BigInt, address.Address, types.BigInt) (cid.Cid, error)                   `perm:"sign"`
		MsigPropose             func(context.Context, address.Address, address.Address, types.BigInt, address.Address, uint64, []byte) (cid.Cid, error)                          `perm:"sign"`
		MsigApprove             func(context.Context, address.Address, uint64, address.Address) (cid.Cid, error)                                                                 `perm:"sign"`
		MsigApproveTxnHash      func(context.Context, address.Address, uint64, address.Address, address.Address, types.BigInt, address.Address, uint64, []byte) (cid.Cid, error) `perm:"sign"`
		MsigCancel              func(context.Context, address.Address, uint64, address.Address, types.BigInt, address.Address, uint64, []byte) (cid.Cid, error)                  `perm:"sign"`
		MsigAddPropose          func(context.Context, address.Address, address.Address, address.Address, bool) (cid.Cid, error)                                                  `perm:"sign"`
		MsigAddApprove          func(context.Context, address.Address, address.Address, uint64, address.Address, address.Address, bool) (cid.Cid, error)                         `perm:"sign"`
		MsigAddCancel           func(context.Context, address.Address, address.Address, uint64, address.Address, bool) (cid.Cid, error)                                          `perm:"sign"`
		MsigSwapPropose         func(context.Context, address.Address, address.Address, address.Address, address.Address) (cid.Cid, error)                                       `perm:"sign"`
		MsigSwapApprove         func(context.Context, address.Address, address.Address, uint64, address.Address, address.Address, address.Address) (cid.Cid, error)              `perm:"sign"`
		MsigSwapCancel          func(context.Context, address.Address, address.Address, uint64, address.Address, address.Address) (cid.Cid, error)                               `perm:"sign"`
		MsigRemoveSigner        func(ctx context.Context, msig address.Address, proposer address.Address, toRemove address.Address, decrease bool) (cid.Cid, error)              `perm:"sign"`

		MarketEnsureAvailable func(context.Context, address.Address, address.Address, types.BigInt) (cid.Cid, error) `perm:"sign"`

		PaychGet                    func(ctx context.Context, from, to address.Address, amt types.BigInt) (*api.ChannelInfo, error)           `perm:"sign"`
		PaychGetWaitReady           func(context.Context, cid.Cid) (address.Address, error)                                                   `perm:"sign"`
		PaychAvailableFunds         func(context.Context, address.Address) (*api.ChannelAvailableFunds, error)                                `perm:"sign"`
		PaychAvailableFundsByFromTo func(context.Context, address.Address, address.Address) (*api.ChannelAvailableFunds, error)               `perm:"sign"`
		PaychList                   func(context.Context) ([]address.Address, error)                                                          `perm:"read"`
		PaychStatus                 func(context.Context, address.Address) (*api.PaychStatus, error)                                          `perm:"read"`
		PaychSettle                 func(context.Context, address.Address) (cid.Cid, error)                                                   `perm:"sign"`
		PaychCollect                func(context.Context, address.Address) (cid.Cid, error)                                                   `perm:"sign"`
		PaychAllocateLane           func(context.Context, address.Address) (uint64, error)                                                    `perm:"sign"`
		PaychNewPayment             func(ctx context.Context, from, to address.Address, vouchers []api.VoucherSpec) (*api.PaymentInfo, error) `perm:"sign"`
		PaychVoucherCheck           func(context.Context, *paych.SignedVoucher) error                                                         `perm:"read"`
		PaychVoucherCheckValid      func(context.Context, address.Address, *paych.SignedVoucher) error                                        `perm:"read"`
		PaychVoucherCheckSpendable  func(context.Context, address.Address, *paych.SignedVoucher, []byte, []byte) (bool, error)                `perm:"read"`
		PaychVoucherAdd             func(context.Context, address.Address, *paych.SignedVoucher, []byte, types.BigInt) (types.BigInt, error)  `perm:"write"`
		PaychVoucherCreate          func(context.Context, address.Address, big.Int, uint64) (*api.VoucherCreateResult, error)                 `perm:"sign"`
		PaychVoucherList            func(context.Context, address.Address) ([]*paych.SignedVoucher, error)                                    `perm:"write"`
		PaychVoucherSubmit          func(context.Context, address.Address, *paych.SignedVoucher, []byte, []byte) (cid.Cid, error)             `perm:"sign"`

		CreateBackup func(ctx context.Context, fpath string) error `perm:"admin"`
	}
}

func (c *FullNodeStruct) StateMinerSectorCount(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MinerSectors, error) {
	return c.Internal.StateMinerSectorCount(ctx, addr, tsk)
}

type StorageMinerStruct struct {
	CommonStruct

	Internal struct {
		ActorAddress    func(context.Context) (address.Address, error)                 `perm:"read"`
		ActorSectorSize func(context.Context, address.Address) (abi.SectorSize, error) `perm:"read"`

		MiningBase func(context.Context) (*types.TipSet, error) `perm:"read"`

		MarketImportDealData      func(context.Context, cid.Cid, string) error                                                                                                                                 `perm:"write"`
		MarketListDeals           func(ctx context.Context) ([]api.MarketDeal, error)                                                                                                                          `perm:"read"`
		MarketListRetrievalDeals  func(ctx context.Context) ([]retrievalmarket.ProviderDealState, error)                                                                                                       `perm:"read"`
		MarketGetDealUpdates      func(ctx context.Context) (<-chan storagemarket.MinerDeal, error)                                                                                                            `perm:"read"`
		MarketListIncompleteDeals func(ctx context.Context) ([]storagemarket.MinerDeal, error)                                                                                                                 `perm:"read"`
		MarketSetAsk              func(ctx context.Context, price types.BigInt, verifiedPrice types.BigInt, duration abi.ChainEpoch, minPieceSize abi.PaddedPieceSize, maxPieceSize abi.PaddedPieceSize) error `perm:"admin"`
		MarketGetAsk              func(ctx context.Context) (*storagemarket.SignedStorageAsk, error)                                                                                                           `perm:"read"`
		MarketSetRetrievalAsk     func(ctx context.Context, rask *retrievalmarket.Ask) error                                                                                                                   `perm:"admin"`
		MarketGetRetrievalAsk     func(ctx context.Context) (*retrievalmarket.Ask, error)                                                                                                                      `perm:"read"`
		MarketListDataTransfers   func(ctx context.Context) ([]api.DataTransferChannel, error)                                                                                                                 `perm:"write"`
		MarketDataTransferUpdates func(ctx context.Context) (<-chan api.DataTransferChannel, error)                                                                                                            `perm:"write"`

		PledgeSector func(context.Context) error `perm:"write"`

		SectorsStatus                 func(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) `perm:"read"`
		SectorsList                   func(context.Context) ([]abi.SectorNumber, error)                                             `perm:"read"`
		SectorsRefs                   func(context.Context) (map[string][]api.SealedRef, error)                                     `perm:"read"`
		SectorStartSealing            func(context.Context, abi.SectorNumber) error                                                 `perm:"write"`
		SectorSetSealDelay            func(context.Context, time.Duration) error                                                    `perm:"write"`
		SectorGetSealDelay            func(context.Context) (time.Duration, error)                                                  `perm:"read"`
		SectorSetExpectedSealDuration func(context.Context, time.Duration) error                                                    `perm:"write"`
		SectorGetExpectedSealDuration func(context.Context) (time.Duration, error)                                                  `perm:"read"`
		SectorsUpdate                 func(context.Context, abi.SectorNumber, api.SectorState) error                                `perm:"admin"`
		SectorRemove                  func(context.Context, abi.SectorNumber) error                                                 `perm:"admin"`
		SectorMarkForUpgrade          func(ctx context.Context, id abi.SectorNumber) error                                          `perm:"admin"`

		WorkerConnect func(context.Context, string) error                             `perm:"admin"` // TODO: worker perm
		WorkerStats   func(context.Context) (map[uint64]storiface.WorkerStats, error) `perm:"admin"`
		WorkerJobs    func(context.Context) (map[uint64][]storiface.WorkerJob, error) `perm:"admin"`

		SealingSchedDiag func(context.Context) (interface{}, error) `perm:"admin"`

		StorageList          func(context.Context) (map[stores.ID][]stores.Decl, error)                                                                                    `perm:"admin"`
		StorageLocal         func(context.Context) (map[stores.ID]string, error)                                                                                           `perm:"admin"`
		StorageStat          func(context.Context, stores.ID) (fsutil.FsStat, error)                                                                                       `perm:"admin"`
		StorageAttach        func(context.Context, stores.StorageInfo, fsutil.FsStat) error                                                                                `perm:"admin"`
		StorageDeclareSector func(context.Context, stores.ID, abi.SectorID, stores.SectorFileType, bool) error                                                             `perm:"admin"`
		StorageDropSector    func(context.Context, stores.ID, abi.SectorID, stores.SectorFileType) error                                                                   `perm:"admin"`
		StorageFindSector    func(context.Context, abi.SectorID, stores.SectorFileType, abi.RegisteredSealProof, bool) ([]stores.SectorStorageInfo, error)                 `perm:"admin"`
		StorageInfo          func(context.Context, stores.ID) (stores.StorageInfo, error)                                                                                  `perm:"admin"`
		StorageBestAlloc     func(ctx context.Context, allocate stores.SectorFileType, spt abi.RegisteredSealProof, sealing stores.PathType) ([]stores.StorageInfo, error) `perm:"admin"`
		StorageReportHealth  func(ctx context.Context, id stores.ID, report stores.HealthReport) error                                                                     `perm:"admin"`
		StorageLock          func(ctx context.Context, sector abi.SectorID, read stores.SectorFileType, write stores.SectorFileType) error                                 `perm:"admin"`
		StorageTryLock       func(ctx context.Context, sector abi.SectorID, read stores.SectorFileType, write stores.SectorFileType) (bool, error)                         `perm:"admin"`

		DealsImportData                       func(ctx context.Context, dealPropCid cid.Cid, file string) error `perm:"write"`
		DealsList                             func(ctx context.Context) ([]api.MarketDeal, error)               `perm:"read"`
		DealsConsiderOnlineStorageDeals       func(context.Context) (bool, error)                               `perm:"read"`
		DealsSetConsiderOnlineStorageDeals    func(context.Context, bool) error                                 `perm:"admin"`
		DealsConsiderOnlineRetrievalDeals     func(context.Context) (bool, error)                               `perm:"read"`
		DealsSetConsiderOnlineRetrievalDeals  func(context.Context, bool) error                                 `perm:"admin"`
		DealsConsiderOfflineStorageDeals      func(context.Context) (bool, error)                               `perm:"read"`
		DealsSetConsiderOfflineStorageDeals   func(context.Context, bool) error                                 `perm:"admin"`
		DealsConsiderOfflineRetrievalDeals    func(context.Context) (bool, error)                               `perm:"read"`
		DealsSetConsiderOfflineRetrievalDeals func(context.Context, bool) error                                 `perm:"admin"`
		DealsPieceCidBlocklist                func(context.Context) ([]cid.Cid, error)                          `perm:"read"`
		DealsSetPieceCidBlocklist             func(context.Context, []cid.Cid) error                            `perm:"admin"`

		StorageAddLocal func(ctx context.Context, path string) error `perm:"admin"`

		PiecesListPieces   func(ctx context.Context) ([]cid.Cid, error)                               `perm:"read"`
		PiecesListCidInfos func(ctx context.Context) ([]cid.Cid, error)                               `perm:"read"`
		PiecesGetPieceInfo func(ctx context.Context, pieceCid cid.Cid) (*piecestore.PieceInfo, error) `perm:"read"`
		PiecesGetCIDInfo   func(ctx context.Context, payloadCid cid.Cid) (*piecestore.CIDInfo, error) `perm:"read"`

		CreateBackup func(ctx context.Context, fpath string) error `perm:"admin"`
	}
}

type WorkerStruct struct {
	Internal struct {
		// TODO: lower perms

		Version func(context.Context) (build.Version, error) `perm:"admin"`

		TaskTypes func(context.Context) (map[sealtasks.TaskType]struct{}, error) `perm:"admin"`
		Paths     func(context.Context) ([]stores.StoragePath, error)            `perm:"admin"`
		Info      func(context.Context) (storiface.WorkerInfo, error)            `perm:"admin"`

		AddPiece        func(ctx context.Context, sector abi.SectorID, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storage.Data) (abi.PieceInfo, error)                      `perm:"admin"`
		SealPreCommit1  func(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storage.PreCommit1Out, error)                                                           `perm:"admin"`
		SealPreCommit2  func(context.Context, abi.SectorID, storage.PreCommit1Out) (cids storage.SectorCids, err error)                                                                                            `perm:"admin"`
		SealCommit1     func(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (storage.Commit1Out, error) `perm:"admin"`
		SealCommit2     func(context.Context, abi.SectorID, storage.Commit1Out) (storage.Proof, error)                                                                                                             `perm:"admin"`
		FinalizeSector  func(context.Context, abi.SectorID, []storage.Range) error                                                                                                                                 `perm:"admin"`
		ReleaseUnsealed func(ctx context.Context, sector abi.SectorID, safeToFree []storage.Range) error                                                                                                           `perm:"admin"`
		Remove          func(ctx context.Context, sector abi.SectorID) error                                                                                                                                       `perm:"admin"`
		MoveStorage     func(ctx context.Context, sector abi.SectorID, types stores.SectorFileType) error                                                                                                          `perm:"admin"`
		StorageAddLocal func(ctx context.Context, path string) error                                                                                                                                               `perm:"admin"`

		UnsealPiece func(context.Context, abi.SectorID, storiface.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) error `perm:"admin"`
		ReadPiece   func(context.Context, io.Writer, abi.SectorID, storiface.UnpaddedByteIndex, abi.UnpaddedPieceSize) (bool, error)           `perm:"admin"`

		Fetch func(context.Context, abi.SectorID, stores.SectorFileType, stores.PathType, stores.AcquireMode) error `perm:"admin"`

		Closing func(context.Context) (<-chan struct{}, error) `perm:"admin"`
	}
}

type GatewayStruct struct {
	Internal struct {
		// TODO: does the gateway need perms?
		ChainGetTipSet          func(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
		ChainGetTipSetByHeight  func(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
		ChainHead               func(ctx context.Context) (*types.TipSet, error)
		GasEstimateMessageGas   func(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
		MpoolPush               func(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)
		MsigGetAvailableBalance func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
		MsigGetVested           func(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
		StateAccountKey         func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
		StateGetActor           func(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
		StateLookupID           func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
		StateWaitMsg            func(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error)
	}
}

type WalletStruct struct {
	Internal struct {
		WalletNew    func(context.Context, types.KeyType) (address.Address, error)                          `perm:"write"`
		WalletHas    func(context.Context, address.Address) (bool, error)                                   `perm:"write"`
		WalletList   func(context.Context) ([]address.Address, error)                                       `perm:"write"`
		WalletSign   func(context.Context, address.Address, []byte, api.MsgMeta) (*crypto.Signature, error) `perm:"sign"`
		WalletExport func(context.Context, address.Address) (*types.KeyInfo, error)                         `perm:"admin"`
		WalletImport func(context.Context, *types.KeyInfo) (address.Address, error)                         `perm:"admin"`
		WalletDelete func(context.Context, address.Address) error                                           `perm:"write"`
	}
}

// CommonStruct

func (c *CommonStruct) AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) {
	return c.Internal.AuthVerify(ctx, token)
}

func (c *CommonStruct) AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error) {
	return c.Internal.AuthNew(ctx, perms)
}

func (c *CommonStruct) NetPubsubScores(ctx context.Context) ([]api.PubsubScore, error) {
	return c.Internal.NetPubsubScores(ctx)
}

func (c *CommonStruct) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	return c.Internal.NetConnectedness(ctx, pid)
}

func (c *CommonStruct) NetPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	return c.Internal.NetPeers(ctx)
}

func (c *CommonStruct) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return c.Internal.NetConnect(ctx, p)
}

func (c *CommonStruct) NetAddrsListen(ctx context.Context) (peer.AddrInfo, error) {
	return c.Internal.NetAddrsListen(ctx)
}

func (c *CommonStruct) NetDisconnect(ctx context.Context, p peer.ID) error {
	return c.Internal.NetDisconnect(ctx, p)
}

func (c *CommonStruct) NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	return c.Internal.NetFindPeer(ctx, p)
}

func (c *CommonStruct) NetAutoNatStatus(ctx context.Context) (api.NatInfo, error) {
	return c.Internal.NetAutoNatStatus(ctx)
}

func (c *CommonStruct) NetBandwidthStats(ctx context.Context) (metrics.Stats, error) {
	return c.Internal.NetBandwidthStats(ctx)
}

func (c *CommonStruct) NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error) {
	return c.Internal.NetBandwidthStatsByPeer(ctx)
}

func (c *CommonStruct) NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error) {
	return c.Internal.NetBandwidthStatsByProtocol(ctx)
}

func (c *CommonStruct) NetAgentVersion(ctx context.Context, p peer.ID) (string, error) {
	return c.Internal.NetAgentVersion(ctx, p)
}

// ID implements API.ID
func (c *CommonStruct) ID(ctx context.Context) (peer.ID, error) {
	return c.Internal.ID(ctx)
}

// Version implements API.Version
func (c *CommonStruct) Version(ctx context.Context) (api.Version, error) {
	return c.Internal.Version(ctx)
}

func (c *CommonStruct) LogList(ctx context.Context) ([]string, error) {
	return c.Internal.LogList(ctx)
}

func (c *CommonStruct) LogSetLevel(ctx context.Context, group, level string) error {
	return c.Internal.LogSetLevel(ctx, group, level)
}

func (c *CommonStruct) Shutdown(ctx context.Context) error {
	return c.Internal.Shutdown(ctx)
}

func (c *CommonStruct) Closing(ctx context.Context) (<-chan struct{}, error) {
	return c.Internal.Closing(ctx)
}

// FullNodeStruct

func (c *FullNodeStruct) ClientListImports(ctx context.Context) ([]api.Import, error) {
	return c.Internal.ClientListImports(ctx)
}

func (c *FullNodeStruct) ClientRemoveImport(ctx context.Context, importID multistore.StoreID) error {
	return c.Internal.ClientRemoveImport(ctx, importID)
}

func (c *FullNodeStruct) ClientImport(ctx context.Context, ref api.FileRef) (*api.ImportRes, error) {
	return c.Internal.ClientImport(ctx, ref)
}

func (c *FullNodeStruct) ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error) {
	return c.Internal.ClientHasLocal(ctx, root)
}

func (c *FullNodeStruct) ClientFindData(ctx context.Context, root cid.Cid, piece *cid.Cid) ([]api.QueryOffer, error) {
	return c.Internal.ClientFindData(ctx, root, piece)
}

func (c *FullNodeStruct) ClientMinerQueryOffer(ctx context.Context, miner address.Address, root cid.Cid, piece *cid.Cid) (api.QueryOffer, error) {
	return c.Internal.ClientMinerQueryOffer(ctx, miner, root, piece)
}

func (c *FullNodeStruct) ClientStartDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	return c.Internal.ClientStartDeal(ctx, params)
}

func (c *FullNodeStruct) ClientGetDealInfo(ctx context.Context, deal cid.Cid) (*api.DealInfo, error) {
	return c.Internal.ClientGetDealInfo(ctx, deal)
}

func (c *FullNodeStruct) ClientListDeals(ctx context.Context) ([]api.DealInfo, error) {
	return c.Internal.ClientListDeals(ctx)
}

func (c *FullNodeStruct) ClientGetDealUpdates(ctx context.Context) (<-chan api.DealInfo, error) {
	return c.Internal.ClientGetDealUpdates(ctx)
}

func (c *FullNodeStruct) ClientRetrieve(ctx context.Context, order api.RetrievalOrder, ref *api.FileRef) error {
	return c.Internal.ClientRetrieve(ctx, order, ref)
}

func (c *FullNodeStruct) ClientRetrieveWithEvents(ctx context.Context, order api.RetrievalOrder, ref *api.FileRef) (<-chan marketevents.RetrievalEvent, error) {
	return c.Internal.ClientRetrieveWithEvents(ctx, order, ref)
}

func (c *FullNodeStruct) ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*storagemarket.StorageAsk, error) {
	return c.Internal.ClientQueryAsk(ctx, p, miner)
}
func (c *FullNodeStruct) ClientCalcCommP(ctx context.Context, inpath string) (*api.CommPRet, error) {
	return c.Internal.ClientCalcCommP(ctx, inpath)
}

func (c *FullNodeStruct) ClientGenCar(ctx context.Context, ref api.FileRef, outpath string) error {
	return c.Internal.ClientGenCar(ctx, ref, outpath)
}

func (c *FullNodeStruct) ClientDealSize(ctx context.Context, root cid.Cid) (api.DataSize, error) {
	return c.Internal.ClientDealSize(ctx, root)
}

func (c *FullNodeStruct) ClientListDataTransfers(ctx context.Context) ([]api.DataTransferChannel, error) {
	return c.Internal.ClientListDataTransfers(ctx)
}

func (c *FullNodeStruct) ClientDataTransferUpdates(ctx context.Context) (<-chan api.DataTransferChannel, error) {
	return c.Internal.ClientDataTransferUpdates(ctx)
}

func (c *FullNodeStruct) ClientRestartDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error {
	return c.Internal.ClientRestartDataTransfer(ctx, transferID, otherPeer, isInitiator)
}

func (c *FullNodeStruct) ClientRetrieveTryRestartInsufficientFunds(ctx context.Context, paymentChannel address.Address) error {
	return c.Internal.ClientRetrieveTryRestartInsufficientFunds(ctx, paymentChannel)
}

func (c *FullNodeStruct) GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.GasEstimateGasPremium(ctx, nblocksincl, sender, gaslimit, tsk)
}

func (c *FullNodeStruct) GasEstimateFeeCap(ctx context.Context, msg *types.Message, maxqueueblks int64, tsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.GasEstimateFeeCap(ctx, msg, maxqueueblks, tsk)
}

func (c *FullNodeStruct) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error) {
	return c.Internal.GasEstimateMessageGas(ctx, msg, spec, tsk)
}

func (c *FullNodeStruct) GasEstimateGasLimit(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (int64, error) {
	return c.Internal.GasEstimateGasLimit(ctx, msg, tsk)
}

func (c *FullNodeStruct) MpoolGetConfig(ctx context.Context) (*types.MpoolConfig, error) {
	return c.Internal.MpoolGetConfig(ctx)
}

func (c *FullNodeStruct) MpoolSetConfig(ctx context.Context, cfg *types.MpoolConfig) error {
	return c.Internal.MpoolSetConfig(ctx, cfg)
}

func (c *FullNodeStruct) MpoolSelect(ctx context.Context, tsk types.TipSetKey, tq float64) ([]*types.SignedMessage, error) {
	return c.Internal.MpoolSelect(ctx, tsk, tq)
}

func (c *FullNodeStruct) MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error) {
	return c.Internal.MpoolPending(ctx, tsk)
}

func (c *FullNodeStruct) MpoolClear(ctx context.Context, local bool) error {
	return c.Internal.MpoolClear(ctx, local)
}

func (c *FullNodeStruct) MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error) {
	return c.Internal.MpoolPush(ctx, smsg)
}

func (c *FullNodeStruct) MpoolPushUntrusted(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error) {
	return c.Internal.MpoolPushUntrusted(ctx, smsg)
}

func (c *FullNodeStruct) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	return c.Internal.MpoolPushMessage(ctx, msg, spec)
}

func (c *FullNodeStruct) MpoolSub(ctx context.Context) (<-chan api.MpoolUpdate, error) {
	return c.Internal.MpoolSub(ctx)
}

func (c *FullNodeStruct) MinerGetBaseInfo(ctx context.Context, maddr address.Address, epoch abi.ChainEpoch, tsk types.TipSetKey) (*api.MiningBaseInfo, error) {
	return c.Internal.MinerGetBaseInfo(ctx, maddr, epoch, tsk)
}

func (c *FullNodeStruct) MinerCreateBlock(ctx context.Context, bt *api.BlockTemplate) (*types.BlockMsg, error) {
	return c.Internal.MinerCreateBlock(ctx, bt)
}

func (c *FullNodeStruct) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return c.Internal.ChainHead(ctx)
}

func (c *FullNodeStruct) ChainGetRandomnessFromTickets(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	return c.Internal.ChainGetRandomnessFromTickets(ctx, tsk, personalization, randEpoch, entropy)
}

func (c *FullNodeStruct) ChainGetRandomnessFromBeacon(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	return c.Internal.ChainGetRandomnessFromBeacon(ctx, tsk, personalization, randEpoch, entropy)
}

func (c *FullNodeStruct) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	return c.Internal.ChainGetTipSetByHeight(ctx, h, tsk)
}

func (c *FullNodeStruct) WalletNew(ctx context.Context, typ types.KeyType) (address.Address, error) {
	return c.Internal.WalletNew(ctx, typ)
}

func (c *FullNodeStruct) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return c.Internal.WalletHas(ctx, addr)
}

func (c *FullNodeStruct) WalletList(ctx context.Context) ([]address.Address, error) {
	return c.Internal.WalletList(ctx)
}

func (c *FullNodeStruct) WalletBalance(ctx context.Context, a address.Address) (types.BigInt, error) {
	return c.Internal.WalletBalance(ctx, a)
}

func (c *FullNodeStruct) WalletSign(ctx context.Context, k address.Address, msg []byte) (*crypto.Signature, error) {
	return c.Internal.WalletSign(ctx, k, msg)
}

func (c *FullNodeStruct) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	return c.Internal.WalletSignMessage(ctx, k, msg)
}

func (c *FullNodeStruct) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *crypto.Signature) (bool, error) {
	return c.Internal.WalletVerify(ctx, k, msg, sig)
}

func (c *FullNodeStruct) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	return c.Internal.WalletDefaultAddress(ctx)
}

func (c *FullNodeStruct) WalletSetDefault(ctx context.Context, a address.Address) error {
	return c.Internal.WalletSetDefault(ctx, a)
}

func (c *FullNodeStruct) WalletExport(ctx context.Context, a address.Address) (*types.KeyInfo, error) {
	return c.Internal.WalletExport(ctx, a)
}

func (c *FullNodeStruct) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	return c.Internal.WalletImport(ctx, ki)
}

func (c *FullNodeStruct) WalletDelete(ctx context.Context, addr address.Address) error {
	return c.Internal.WalletDelete(ctx, addr)
}

func (c *FullNodeStruct) WalletValidateAddress(ctx context.Context, str string) (address.Address, error) {
	return c.Internal.WalletValidateAddress(ctx, str)
}

func (c *FullNodeStruct) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return c.Internal.MpoolGetNonce(ctx, addr)
}

func (c *FullNodeStruct) ChainGetBlock(ctx context.Context, b cid.Cid) (*types.BlockHeader, error) {
	return c.Internal.ChainGetBlock(ctx, b)
}

func (c *FullNodeStruct) ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error) {
	return c.Internal.ChainGetTipSet(ctx, key)
}

func (c *FullNodeStruct) ChainGetBlockMessages(ctx context.Context, b cid.Cid) (*api.BlockMessages, error) {
	return c.Internal.ChainGetBlockMessages(ctx, b)
}

func (c *FullNodeStruct) ChainGetParentReceipts(ctx context.Context, b cid.Cid) ([]*types.MessageReceipt, error) {
	return c.Internal.ChainGetParentReceipts(ctx, b)
}

func (c *FullNodeStruct) ChainGetParentMessages(ctx context.Context, b cid.Cid) ([]api.Message, error) {
	return c.Internal.ChainGetParentMessages(ctx, b)
}

func (c *FullNodeStruct) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	return c.Internal.ChainNotify(ctx)
}

func (c *FullNodeStruct) ChainReadObj(ctx context.Context, obj cid.Cid) ([]byte, error) {
	return c.Internal.ChainReadObj(ctx, obj)
}

func (c *FullNodeStruct) ChainDeleteObj(ctx context.Context, obj cid.Cid) error {
	return c.Internal.ChainDeleteObj(ctx, obj)
}

func (c *FullNodeStruct) ChainHasObj(ctx context.Context, o cid.Cid) (bool, error) {
	return c.Internal.ChainHasObj(ctx, o)
}

func (c *FullNodeStruct) ChainStatObj(ctx context.Context, obj, base cid.Cid) (api.ObjStat, error) {
	return c.Internal.ChainStatObj(ctx, obj, base)
}

func (c *FullNodeStruct) ChainSetHead(ctx context.Context, tsk types.TipSetKey) error {
	return c.Internal.ChainSetHead(ctx, tsk)
}

func (c *FullNodeStruct) ChainGetGenesis(ctx context.Context) (*types.TipSet, error) {
	return c.Internal.ChainGetGenesis(ctx)
}

func (c *FullNodeStruct) ChainTipSetWeight(ctx context.Context, tsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.ChainTipSetWeight(ctx, tsk)
}

func (c *FullNodeStruct) ChainGetNode(ctx context.Context, p string) (*api.IpldObject, error) {
	return c.Internal.ChainGetNode(ctx, p)
}

func (c *FullNodeStruct) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	return c.Internal.ChainGetMessage(ctx, mc)
}

func (c *FullNodeStruct) ChainGetPath(ctx context.Context, from types.TipSetKey, to types.TipSetKey) ([]*api.HeadChange, error) {
	return c.Internal.ChainGetPath(ctx, from, to)
}

func (c *FullNodeStruct) ChainExport(ctx context.Context, nroots abi.ChainEpoch, iom bool, tsk types.TipSetKey) (<-chan []byte, error) {
	return c.Internal.ChainExport(ctx, nroots, iom, tsk)
}

func (c *FullNodeStruct) BeaconGetEntry(ctx context.Context, epoch abi.ChainEpoch) (*types.BeaconEntry, error) {
	return c.Internal.BeaconGetEntry(ctx, epoch)
}

func (c *FullNodeStruct) SyncState(ctx context.Context) (*api.SyncState, error) {
	return c.Internal.SyncState(ctx)
}

func (c *FullNodeStruct) SyncSubmitBlock(ctx context.Context, blk *types.BlockMsg) error {
	return c.Internal.SyncSubmitBlock(ctx, blk)
}

func (c *FullNodeStruct) SyncIncomingBlocks(ctx context.Context) (<-chan *types.BlockHeader, error) {
	return c.Internal.SyncIncomingBlocks(ctx)
}

func (c *FullNodeStruct) SyncCheckpoint(ctx context.Context, tsk types.TipSetKey) error {
	return c.Internal.SyncCheckpoint(ctx, tsk)
}

func (c *FullNodeStruct) SyncMarkBad(ctx context.Context, bcid cid.Cid) error {
	return c.Internal.SyncMarkBad(ctx, bcid)
}

func (c *FullNodeStruct) SyncUnmarkBad(ctx context.Context, bcid cid.Cid) error {
	return c.Internal.SyncUnmarkBad(ctx, bcid)
}

func (c *FullNodeStruct) SyncUnmarkAllBad(ctx context.Context) error {
	return c.Internal.SyncUnmarkAllBad(ctx)
}

func (c *FullNodeStruct) SyncCheckBad(ctx context.Context, bcid cid.Cid) (string, error) {
	return c.Internal.SyncCheckBad(ctx, bcid)
}

func (c *FullNodeStruct) SyncValidateTipset(ctx context.Context, tsk types.TipSetKey) (bool, error) {
	return c.Internal.SyncValidateTipset(ctx, tsk)
}

func (c *FullNodeStruct) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	return c.Internal.StateNetworkName(ctx)
}

func (c *FullNodeStruct) StateMinerSectors(ctx context.Context, addr address.Address, sectorNos *bitfield.BitField, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	return c.Internal.StateMinerSectors(ctx, addr, sectorNos, tsk)
}

func (c *FullNodeStruct) StateMinerActiveSectors(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	return c.Internal.StateMinerActiveSectors(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*dline.Info, error) {
	return c.Internal.StateMinerProvingDeadline(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateMinerPower(ctx context.Context, a address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	return c.Internal.StateMinerPower(ctx, a, tsk)
}

func (c *FullNodeStruct) StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (miner.MinerInfo, error) {
	return c.Internal.StateMinerInfo(ctx, actor, tsk)
}

func (c *FullNodeStruct) StateMinerDeadlines(ctx context.Context, actor address.Address, tsk types.TipSetKey) ([]api.Deadline, error) {
	return c.Internal.StateMinerDeadlines(ctx, actor, tsk)
}

func (c *FullNodeStruct) StateMinerPartitions(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error) {
	return c.Internal.StateMinerPartitions(ctx, m, dlIdx, tsk)
}

func (c *FullNodeStruct) StateMinerFaults(ctx context.Context, actor address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	return c.Internal.StateMinerFaults(ctx, actor, tsk)
}

func (c *FullNodeStruct) StateAllMinerFaults(ctx context.Context, cutoff abi.ChainEpoch, endTsk types.TipSetKey) ([]*api.Fault, error) {
	return c.Internal.StateAllMinerFaults(ctx, cutoff, endTsk)
}

func (c *FullNodeStruct) StateMinerRecoveries(ctx context.Context, actor address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	return c.Internal.StateMinerRecoveries(ctx, actor, tsk)
}

func (c *FullNodeStruct) StateMinerPreCommitDepositForPower(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.StateMinerPreCommitDepositForPower(ctx, maddr, pci, tsk)
}

func (c *FullNodeStruct) StateMinerInitialPledgeCollateral(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.StateMinerInitialPledgeCollateral(ctx, maddr, pci, tsk)
}

func (c *FullNodeStruct) StateMinerAvailableBalance(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.StateMinerAvailableBalance(ctx, maddr, tsk)
}

func (c *FullNodeStruct) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) {
	return c.Internal.StateSectorPreCommitInfo(ctx, maddr, n, tsk)
}

func (c *FullNodeStruct) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	return c.Internal.StateSectorGetInfo(ctx, maddr, n, tsk)
}

func (c *FullNodeStruct) StateSectorExpiration(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorExpiration, error) {
	return c.Internal.StateSectorExpiration(ctx, maddr, n, tsk)
}

func (c *FullNodeStruct) StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok types.TipSetKey) (*miner.SectorLocation, error) {
	return c.Internal.StateSectorPartition(ctx, maddr, sectorNumber, tok)
}

func (c *FullNodeStruct) StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (*api.InvocResult, error) {
	return c.Internal.StateCall(ctx, msg, tsk)
}

func (c *FullNodeStruct) StateReplay(ctx context.Context, tsk types.TipSetKey, mc cid.Cid) (*api.InvocResult, error) {
	return c.Internal.StateReplay(ctx, tsk, mc)
}

func (c *FullNodeStruct) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	return c.Internal.StateGetActor(ctx, actor, tsk)
}

func (c *FullNodeStruct) StateReadState(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*api.ActorState, error) {
	return c.Internal.StateReadState(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateMsgGasCost(ctx context.Context, msgc cid.Cid, tsk types.TipSetKey) (*api.MsgGasCost, error) {
	return c.Internal.StateMsgGasCost(ctx, msgc, tsk)
}

func (c *FullNodeStruct) StateWaitMsg(ctx context.Context, msgc cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	return c.Internal.StateWaitMsg(ctx, msgc, confidence)
}

func (c *FullNodeStruct) StateWaitMsgLimited(ctx context.Context, msgc cid.Cid, confidence uint64, limit abi.ChainEpoch) (*api.MsgLookup, error) {
	return c.Internal.StateWaitMsgLimited(ctx, msgc, confidence, limit)
}

func (c *FullNodeStruct) StateSearchMsg(ctx context.Context, msgc cid.Cid) (*api.MsgLookup, error) {
	return c.Internal.StateSearchMsg(ctx, msgc)
}

func (c *FullNodeStruct) StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	return c.Internal.StateListMiners(ctx, tsk)
}

func (c *FullNodeStruct) StateListActors(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	return c.Internal.StateListActors(ctx, tsk)
}

func (c *FullNodeStruct) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	return c.Internal.StateMarketBalance(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateMarketParticipants(ctx context.Context, tsk types.TipSetKey) (map[string]api.MarketBalance, error) {
	return c.Internal.StateMarketParticipants(ctx, tsk)
}

func (c *FullNodeStruct) StateMarketDeals(ctx context.Context, tsk types.TipSetKey) (map[string]api.MarketDeal, error) {
	return c.Internal.StateMarketDeals(ctx, tsk)
}

func (c *FullNodeStruct) StateMarketStorageDeal(ctx context.Context, dealid abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	return c.Internal.StateMarketStorageDeal(ctx, dealid, tsk)
}

func (c *FullNodeStruct) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return c.Internal.StateLookupID(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return c.Internal.StateAccountKey(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateChangedActors(ctx context.Context, olnstate cid.Cid, newstate cid.Cid) (map[string]types.Actor, error) {
	return c.Internal.StateChangedActors(ctx, olnstate, newstate)
}

func (c *FullNodeStruct) StateGetReceipt(ctx context.Context, msg cid.Cid, tsk types.TipSetKey) (*types.MessageReceipt, error) {
	return c.Internal.StateGetReceipt(ctx, msg, tsk)
}

func (c *FullNodeStruct) StateListMessages(ctx context.Context, match *types.Message, tsk types.TipSetKey, toht abi.ChainEpoch) ([]cid.Cid, error) {
	return c.Internal.StateListMessages(ctx, match, tsk, toht)
}

func (c *FullNodeStruct) StateCompute(ctx context.Context, height abi.ChainEpoch, msgs []*types.Message, tsk types.TipSetKey) (*api.ComputeStateOutput, error) {
	return c.Internal.StateCompute(ctx, height, msgs, tsk)
}

func (c *FullNodeStruct) StateVerifierStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	return c.Internal.StateVerifierStatus(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	return c.Internal.StateVerifiedClientStatus(ctx, addr, tsk)
}

func (c *FullNodeStruct) StateVerifiedRegistryRootKey(ctx context.Context, tsk types.TipSetKey) (address.Address, error) {
	return c.Internal.StateVerifiedRegistryRootKey(ctx, tsk)
}

func (c *FullNodeStruct) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	return c.Internal.StateDealProviderCollateralBounds(ctx, size, verified, tsk)
}

func (c *FullNodeStruct) StateCirculatingSupply(ctx context.Context, tsk types.TipSetKey) (abi.TokenAmount, error) {
	return c.Internal.StateCirculatingSupply(ctx, tsk)
}

func (c *FullNodeStruct) StateVMCirculatingSupplyInternal(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error) {
	return c.Internal.StateVMCirculatingSupplyInternal(ctx, tsk)
}

func (c *FullNodeStruct) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (stnetwork.Version, error) {
	return c.Internal.StateNetworkVersion(ctx, tsk)
}

func (c *FullNodeStruct) MsigGetAvailableBalance(ctx context.Context, a address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.MsigGetAvailableBalance(ctx, a, tsk)
}

func (c *FullNodeStruct) MsigGetVestingSchedule(ctx context.Context, a address.Address, tsk types.TipSetKey) (api.MsigVesting, error) {
	return c.Internal.MsigGetVestingSchedule(ctx, a, tsk)
}

func (c *FullNodeStruct) MsigGetVested(ctx context.Context, a address.Address, sTsk types.TipSetKey, eTsk types.TipSetKey) (types.BigInt, error) {
	return c.Internal.MsigGetVested(ctx, a, sTsk, eTsk)
}

func (c *FullNodeStruct) MsigCreate(ctx context.Context, req uint64, addrs []address.Address, duration abi.ChainEpoch, val types.BigInt, src address.Address, gp types.BigInt) (cid.Cid, error) {
	return c.Internal.MsigCreate(ctx, req, addrs, duration, val, src, gp)
}

func (c *FullNodeStruct) MsigPropose(ctx context.Context, msig address.Address, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	return c.Internal.MsigPropose(ctx, msig, to, amt, src, method, params)
}

func (c *FullNodeStruct) MsigApprove(ctx context.Context, msig address.Address, txID uint64, signer address.Address) (cid.Cid, error) {
	return c.Internal.MsigApprove(ctx, msig, txID, signer)
}

func (c *FullNodeStruct) MsigApproveTxnHash(ctx context.Context, msig address.Address, txID uint64, proposer address.Address, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	return c.Internal.MsigApproveTxnHash(ctx, msig, txID, proposer, to, amt, src, method, params)
}

func (c *FullNodeStruct) MsigCancel(ctx context.Context, msig address.Address, txID uint64, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	return c.Internal.MsigCancel(ctx, msig, txID, to, amt, src, method, params)
}

func (c *FullNodeStruct) MsigAddPropose(ctx context.Context, msig address.Address, src address.Address, newAdd address.Address, inc bool) (cid.Cid, error) {
	return c.Internal.MsigAddPropose(ctx, msig, src, newAdd, inc)
}

func (c *FullNodeStruct) MsigAddApprove(ctx context.Context, msig address.Address, src address.Address, txID uint64, proposer address.Address, newAdd address.Address, inc bool) (cid.Cid, error) {
	return c.Internal.MsigAddApprove(ctx, msig, src, txID, proposer, newAdd, inc)
}

func (c *FullNodeStruct) MsigAddCancel(ctx context.Context, msig address.Address, src address.Address, txID uint64, newAdd address.Address, inc bool) (cid.Cid, error) {
	return c.Internal.MsigAddCancel(ctx, msig, src, txID, newAdd, inc)
}

func (c *FullNodeStruct) MsigSwapPropose(ctx context.Context, msig address.Address, src address.Address, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {
	return c.Internal.MsigSwapPropose(ctx, msig, src, oldAdd, newAdd)
}

func (c *FullNodeStruct) MsigSwapApprove(ctx context.Context, msig address.Address, src address.Address, txID uint64, proposer address.Address, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {
	return c.Internal.MsigSwapApprove(ctx, msig, src, txID, proposer, oldAdd, newAdd)
}

func (c *FullNodeStruct) MsigSwapCancel(ctx context.Context, msig address.Address, src address.Address, txID uint64, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {
	return c.Internal.MsigSwapCancel(ctx, msig, src, txID, oldAdd, newAdd)
}

func (c *FullNodeStruct) MsigRemoveSigner(ctx context.Context, msig address.Address, proposer address.Address, toRemove address.Address, decrease bool) (cid.Cid, error) {
	return c.Internal.MsigRemoveSigner(ctx, msig, proposer, toRemove, decrease)
}

func (c *FullNodeStruct) MarketEnsureAvailable(ctx context.Context, addr, wallet address.Address, amt types.BigInt) (cid.Cid, error) {
	return c.Internal.MarketEnsureAvailable(ctx, addr, wallet, amt)
}

func (c *FullNodeStruct) PaychGet(ctx context.Context, from, to address.Address, amt types.BigInt) (*api.ChannelInfo, error) {
	return c.Internal.PaychGet(ctx, from, to, amt)
}

func (c *FullNodeStruct) PaychGetWaitReady(ctx context.Context, sentinel cid.Cid) (address.Address, error) {
	return c.Internal.PaychGetWaitReady(ctx, sentinel)
}

func (c *FullNodeStruct) PaychAvailableFunds(ctx context.Context, ch address.Address) (*api.ChannelAvailableFunds, error) {
	return c.Internal.PaychAvailableFunds(ctx, ch)
}

func (c *FullNodeStruct) PaychAvailableFundsByFromTo(ctx context.Context, from, to address.Address) (*api.ChannelAvailableFunds, error) {
	return c.Internal.PaychAvailableFundsByFromTo(ctx, from, to)
}

func (c *FullNodeStruct) PaychList(ctx context.Context) ([]address.Address, error) {
	return c.Internal.PaychList(ctx)
}

func (c *FullNodeStruct) PaychStatus(ctx context.Context, pch address.Address) (*api.PaychStatus, error) {
	return c.Internal.PaychStatus(ctx, pch)
}

func (c *FullNodeStruct) PaychVoucherCheckValid(ctx context.Context, addr address.Address, sv *paych.SignedVoucher) error {
	return c.Internal.PaychVoucherCheckValid(ctx, addr, sv)
}

func (c *FullNodeStruct) PaychVoucherCheckSpendable(ctx context.Context, addr address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return c.Internal.PaychVoucherCheckSpendable(ctx, addr, sv, secret, proof)
}

func (c *FullNodeStruct) PaychVoucherAdd(ctx context.Context, addr address.Address, sv *paych.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	return c.Internal.PaychVoucherAdd(ctx, addr, sv, proof, minDelta)
}

func (c *FullNodeStruct) PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*api.VoucherCreateResult, error) {
	return c.Internal.PaychVoucherCreate(ctx, pch, amt, lane)
}

func (c *FullNodeStruct) PaychVoucherList(ctx context.Context, pch address.Address) ([]*paych.SignedVoucher, error) {
	return c.Internal.PaychVoucherList(ctx, pch)
}

func (c *FullNodeStruct) PaychSettle(ctx context.Context, a address.Address) (cid.Cid, error) {
	return c.Internal.PaychSettle(ctx, a)
}

func (c *FullNodeStruct) PaychCollect(ctx context.Context, a address.Address) (cid.Cid, error) {
	return c.Internal.PaychCollect(ctx, a)
}

func (c *FullNodeStruct) PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error) {
	return c.Internal.PaychAllocateLane(ctx, ch)
}

func (c *FullNodeStruct) PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []api.VoucherSpec) (*api.PaymentInfo, error) {
	return c.Internal.PaychNewPayment(ctx, from, to, vouchers)
}

func (c *FullNodeStruct) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (cid.Cid, error) {
	return c.Internal.PaychVoucherSubmit(ctx, ch, sv, secret, proof)
}

func (c *FullNodeStruct) CreateBackup(ctx context.Context, fpath string) error {
	return c.Internal.CreateBackup(ctx, fpath)
}

// StorageMinerStruct

func (c *StorageMinerStruct) ActorAddress(ctx context.Context) (address.Address, error) {
	return c.Internal.ActorAddress(ctx)
}

func (c *StorageMinerStruct) MiningBase(ctx context.Context) (*types.TipSet, error) {
	return c.Internal.MiningBase(ctx)
}

func (c *StorageMinerStruct) ActorSectorSize(ctx context.Context, addr address.Address) (abi.SectorSize, error) {
	return c.Internal.ActorSectorSize(ctx, addr)
}

func (c *StorageMinerStruct) PledgeSector(ctx context.Context) error {
	return c.Internal.PledgeSector(ctx)
}

// Get the status of a given sector by ID
func (c *StorageMinerStruct) SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	return c.Internal.SectorsStatus(ctx, sid, showOnChainInfo)
}

// List all staged sectors
func (c *StorageMinerStruct) SectorsList(ctx context.Context) ([]abi.SectorNumber, error) {
	return c.Internal.SectorsList(ctx)
}

func (c *StorageMinerStruct) SectorsRefs(ctx context.Context) (map[string][]api.SealedRef, error) {
	return c.Internal.SectorsRefs(ctx)
}

func (c *StorageMinerStruct) SectorStartSealing(ctx context.Context, number abi.SectorNumber) error {
	return c.Internal.SectorStartSealing(ctx, number)
}

func (c *StorageMinerStruct) SectorSetSealDelay(ctx context.Context, delay time.Duration) error {
	return c.Internal.SectorSetSealDelay(ctx, delay)
}

func (c *StorageMinerStruct) SectorGetSealDelay(ctx context.Context) (time.Duration, error) {
	return c.Internal.SectorGetSealDelay(ctx)
}

func (c *StorageMinerStruct) SectorSetExpectedSealDuration(ctx context.Context, delay time.Duration) error {
	return c.Internal.SectorSetExpectedSealDuration(ctx, delay)
}

func (c *StorageMinerStruct) SectorGetExpectedSealDuration(ctx context.Context) (time.Duration, error) {
	return c.Internal.SectorGetExpectedSealDuration(ctx)
}

func (c *StorageMinerStruct) SectorsUpdate(ctx context.Context, id abi.SectorNumber, state api.SectorState) error {
	return c.Internal.SectorsUpdate(ctx, id, state)
}

func (c *StorageMinerStruct) SectorRemove(ctx context.Context, number abi.SectorNumber) error {
	return c.Internal.SectorRemove(ctx, number)
}

func (c *StorageMinerStruct) SectorMarkForUpgrade(ctx context.Context, number abi.SectorNumber) error {
	return c.Internal.SectorMarkForUpgrade(ctx, number)
}

func (c *StorageMinerStruct) WorkerConnect(ctx context.Context, url string) error {
	return c.Internal.WorkerConnect(ctx, url)
}

func (c *StorageMinerStruct) WorkerStats(ctx context.Context) (map[uint64]storiface.WorkerStats, error) {
	return c.Internal.WorkerStats(ctx)
}

func (c *StorageMinerStruct) WorkerJobs(ctx context.Context) (map[uint64][]storiface.WorkerJob, error) {
	return c.Internal.WorkerJobs(ctx)
}

func (c *StorageMinerStruct) SealingSchedDiag(ctx context.Context) (interface{}, error) {
	return c.Internal.SealingSchedDiag(ctx)
}

func (c *StorageMinerStruct) StorageAttach(ctx context.Context, si stores.StorageInfo, st fsutil.FsStat) error {
	return c.Internal.StorageAttach(ctx, si, st)
}

func (c *StorageMinerStruct) StorageDeclareSector(ctx context.Context, storageId stores.ID, s abi.SectorID, ft stores.SectorFileType, primary bool) error {
	return c.Internal.StorageDeclareSector(ctx, storageId, s, ft, primary)
}

func (c *StorageMinerStruct) StorageDropSector(ctx context.Context, storageId stores.ID, s abi.SectorID, ft stores.SectorFileType) error {
	return c.Internal.StorageDropSector(ctx, storageId, s, ft)
}

func (c *StorageMinerStruct) StorageFindSector(ctx context.Context, si abi.SectorID, types stores.SectorFileType, spt abi.RegisteredSealProof, allowFetch bool) ([]stores.SectorStorageInfo, error) {
	return c.Internal.StorageFindSector(ctx, si, types, spt, allowFetch)
}

func (c *StorageMinerStruct) StorageList(ctx context.Context) (map[stores.ID][]stores.Decl, error) {
	return c.Internal.StorageList(ctx)
}

func (c *StorageMinerStruct) StorageLocal(ctx context.Context) (map[stores.ID]string, error) {
	return c.Internal.StorageLocal(ctx)
}

func (c *StorageMinerStruct) StorageStat(ctx context.Context, id stores.ID) (fsutil.FsStat, error) {
	return c.Internal.StorageStat(ctx, id)
}

func (c *StorageMinerStruct) StorageInfo(ctx context.Context, id stores.ID) (stores.StorageInfo, error) {
	return c.Internal.StorageInfo(ctx, id)
}

func (c *StorageMinerStruct) StorageBestAlloc(ctx context.Context, allocate stores.SectorFileType, spt abi.RegisteredSealProof, pt stores.PathType) ([]stores.StorageInfo, error) {
	return c.Internal.StorageBestAlloc(ctx, allocate, spt, pt)
}

func (c *StorageMinerStruct) StorageReportHealth(ctx context.Context, id stores.ID, report stores.HealthReport) error {
	return c.Internal.StorageReportHealth(ctx, id, report)
}

func (c *StorageMinerStruct) StorageLock(ctx context.Context, sector abi.SectorID, read stores.SectorFileType, write stores.SectorFileType) error {
	return c.Internal.StorageLock(ctx, sector, read, write)
}

func (c *StorageMinerStruct) StorageTryLock(ctx context.Context, sector abi.SectorID, read stores.SectorFileType, write stores.SectorFileType) (bool, error) {
	return c.Internal.StorageTryLock(ctx, sector, read, write)
}

func (c *StorageMinerStruct) MarketImportDealData(ctx context.Context, propcid cid.Cid, path string) error {
	return c.Internal.MarketImportDealData(ctx, propcid, path)
}

func (c *StorageMinerStruct) MarketListDeals(ctx context.Context) ([]api.MarketDeal, error) {
	return c.Internal.MarketListDeals(ctx)
}

func (c *StorageMinerStruct) MarketListRetrievalDeals(ctx context.Context) ([]retrievalmarket.ProviderDealState, error) {
	return c.Internal.MarketListRetrievalDeals(ctx)
}

func (c *StorageMinerStruct) MarketGetDealUpdates(ctx context.Context) (<-chan storagemarket.MinerDeal, error) {
	return c.Internal.MarketGetDealUpdates(ctx)
}

func (c *StorageMinerStruct) MarketListIncompleteDeals(ctx context.Context) ([]storagemarket.MinerDeal, error) {
	return c.Internal.MarketListIncompleteDeals(ctx)
}

func (c *StorageMinerStruct) MarketSetAsk(ctx context.Context, price types.BigInt, verifiedPrice types.BigInt, duration abi.ChainEpoch, minPieceSize abi.PaddedPieceSize, maxPieceSize abi.PaddedPieceSize) error {
	return c.Internal.MarketSetAsk(ctx, price, verifiedPrice, duration, minPieceSize, maxPieceSize)
}

func (c *StorageMinerStruct) MarketGetAsk(ctx context.Context) (*storagemarket.SignedStorageAsk, error) {
	return c.Internal.MarketGetAsk(ctx)
}

func (c *StorageMinerStruct) MarketSetRetrievalAsk(ctx context.Context, rask *retrievalmarket.Ask) error {
	return c.Internal.MarketSetRetrievalAsk(ctx, rask)
}

func (c *StorageMinerStruct) MarketGetRetrievalAsk(ctx context.Context) (*retrievalmarket.Ask, error) {
	return c.Internal.MarketGetRetrievalAsk(ctx)
}

func (c *StorageMinerStruct) MarketListDataTransfers(ctx context.Context) ([]api.DataTransferChannel, error) {
	return c.Internal.MarketListDataTransfers(ctx)
}

func (c *StorageMinerStruct) MarketDataTransferUpdates(ctx context.Context) (<-chan api.DataTransferChannel, error) {
	return c.Internal.MarketDataTransferUpdates(ctx)
}

func (c *StorageMinerStruct) DealsImportData(ctx context.Context, dealPropCid cid.Cid, file string) error {
	return c.Internal.DealsImportData(ctx, dealPropCid, file)
}

func (c *StorageMinerStruct) DealsList(ctx context.Context) ([]api.MarketDeal, error) {
	return c.Internal.DealsList(ctx)
}

func (c *StorageMinerStruct) DealsConsiderOnlineStorageDeals(ctx context.Context) (bool, error) {
	return c.Internal.DealsConsiderOnlineStorageDeals(ctx)
}

func (c *StorageMinerStruct) DealsSetConsiderOnlineStorageDeals(ctx context.Context, b bool) error {
	return c.Internal.DealsSetConsiderOnlineStorageDeals(ctx, b)
}

func (c *StorageMinerStruct) DealsConsiderOnlineRetrievalDeals(ctx context.Context) (bool, error) {
	return c.Internal.DealsConsiderOnlineRetrievalDeals(ctx)
}

func (c *StorageMinerStruct) DealsSetConsiderOnlineRetrievalDeals(ctx context.Context, b bool) error {
	return c.Internal.DealsSetConsiderOnlineRetrievalDeals(ctx, b)
}

func (c *StorageMinerStruct) DealsPieceCidBlocklist(ctx context.Context) ([]cid.Cid, error) {
	return c.Internal.DealsPieceCidBlocklist(ctx)
}

func (c *StorageMinerStruct) DealsSetPieceCidBlocklist(ctx context.Context, cids []cid.Cid) error {
	return c.Internal.DealsSetPieceCidBlocklist(ctx, cids)
}

func (c *StorageMinerStruct) DealsConsiderOfflineStorageDeals(ctx context.Context) (bool, error) {
	return c.Internal.DealsConsiderOfflineStorageDeals(ctx)
}

func (c *StorageMinerStruct) DealsSetConsiderOfflineStorageDeals(ctx context.Context, b bool) error {
	return c.Internal.DealsSetConsiderOfflineStorageDeals(ctx, b)
}

func (c *StorageMinerStruct) DealsConsiderOfflineRetrievalDeals(ctx context.Context) (bool, error) {
	return c.Internal.DealsConsiderOfflineRetrievalDeals(ctx)
}

func (c *StorageMinerStruct) DealsSetConsiderOfflineRetrievalDeals(ctx context.Context, b bool) error {
	return c.Internal.DealsSetConsiderOfflineRetrievalDeals(ctx, b)
}

func (c *StorageMinerStruct) StorageAddLocal(ctx context.Context, path string) error {
	return c.Internal.StorageAddLocal(ctx, path)
}

func (c *StorageMinerStruct) PiecesListPieces(ctx context.Context) ([]cid.Cid, error) {
	return c.Internal.PiecesListPieces(ctx)
}

func (c *StorageMinerStruct) PiecesListCidInfos(ctx context.Context) ([]cid.Cid, error) {
	return c.Internal.PiecesListCidInfos(ctx)
}

func (c *StorageMinerStruct) PiecesGetPieceInfo(ctx context.Context, pieceCid cid.Cid) (*piecestore.PieceInfo, error) {
	return c.Internal.PiecesGetPieceInfo(ctx, pieceCid)
}

func (c *StorageMinerStruct) PiecesGetCIDInfo(ctx context.Context, payloadCid cid.Cid) (*piecestore.CIDInfo, error) {
	return c.Internal.PiecesGetCIDInfo(ctx, payloadCid)
}

func (c *StorageMinerStruct) CreateBackup(ctx context.Context, fpath string) error {
	return c.Internal.CreateBackup(ctx, fpath)
}

// WorkerStruct

func (w *WorkerStruct) Version(ctx context.Context) (build.Version, error) {
	return w.Internal.Version(ctx)
}

func (w *WorkerStruct) TaskTypes(ctx context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return w.Internal.TaskTypes(ctx)
}

func (w *WorkerStruct) Paths(ctx context.Context) ([]stores.StoragePath, error) {
	return w.Internal.Paths(ctx)
}

func (w *WorkerStruct) Info(ctx context.Context) (storiface.WorkerInfo, error) {
	return w.Internal.Info(ctx)
}

func (w *WorkerStruct) AddPiece(ctx context.Context, sector abi.SectorID, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storage.Data) (abi.PieceInfo, error) {
	return w.Internal.AddPiece(ctx, sector, pieceSizes, newPieceSize, pieceData)
}

func (w *WorkerStruct) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storage.PreCommit1Out, error) {
	return w.Internal.SealPreCommit1(ctx, sector, ticket, pieces)
}

func (w *WorkerStruct) SealPreCommit2(ctx context.Context, sector abi.SectorID, p1o storage.PreCommit1Out) (storage.SectorCids, error) {
	return w.Internal.SealPreCommit2(ctx, sector, p1o)
}

func (w *WorkerStruct) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (storage.Commit1Out, error) {
	return w.Internal.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
}

func (w *WorkerStruct) SealCommit2(ctx context.Context, sector abi.SectorID, c1o storage.Commit1Out) (storage.Proof, error) {
	return w.Internal.SealCommit2(ctx, sector, c1o)
}

func (w *WorkerStruct) FinalizeSector(ctx context.Context, sector abi.SectorID, keepUnsealed []storage.Range) error {
	return w.Internal.FinalizeSector(ctx, sector, keepUnsealed)
}

func (w *WorkerStruct) ReleaseUnsealed(ctx context.Context, sector abi.SectorID, safeToFree []storage.Range) error {
	return w.Internal.ReleaseUnsealed(ctx, sector, safeToFree)
}

func (w *WorkerStruct) Remove(ctx context.Context, sector abi.SectorID) error {
	return w.Internal.Remove(ctx, sector)
}

func (w *WorkerStruct) MoveStorage(ctx context.Context, sector abi.SectorID, types stores.SectorFileType) error {
	return w.Internal.MoveStorage(ctx, sector, types)
}

func (w *WorkerStruct) StorageAddLocal(ctx context.Context, path string) error {
	return w.Internal.StorageAddLocal(ctx, path)
}

func (w *WorkerStruct) UnsealPiece(ctx context.Context, id abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, c cid.Cid) error {
	return w.Internal.UnsealPiece(ctx, id, index, size, randomness, c)
}

func (w *WorkerStruct) ReadPiece(ctx context.Context, writer io.Writer, id abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	return w.Internal.ReadPiece(ctx, writer, id, index, size)
}

func (w *WorkerStruct) Fetch(ctx context.Context, id abi.SectorID, fileType stores.SectorFileType, ptype stores.PathType, am stores.AcquireMode) error {
	return w.Internal.Fetch(ctx, id, fileType, ptype, am)
}

func (w *WorkerStruct) Closing(ctx context.Context) (<-chan struct{}, error) {
	return w.Internal.Closing(ctx)
}

func (g GatewayStruct) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return g.Internal.ChainHead(ctx)
}

func (g GatewayStruct) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return g.Internal.ChainGetTipSet(ctx, tsk)
}

func (g GatewayStruct) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	return g.Internal.ChainGetTipSetByHeight(ctx, h, tsk)
}

func (g GatewayStruct) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error) {
	return g.Internal.GasEstimateMessageGas(ctx, msg, spec, tsk)
}

func (g GatewayStruct) MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error) {
	return g.Internal.MpoolPush(ctx, sm)
}

func (g GatewayStruct) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	return g.Internal.MsigGetAvailableBalance(ctx, addr, tsk)
}

func (g GatewayStruct) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	return g.Internal.MsigGetVested(ctx, addr, start, end)
}

func (g GatewayStruct) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return g.Internal.StateAccountKey(ctx, addr, tsk)
}

func (g GatewayStruct) StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error) {
	return g.Internal.StateGetActor(ctx, actor, ts)
}

func (g GatewayStruct) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return g.Internal.StateLookupID(ctx, addr, tsk)
}

func (g GatewayStruct) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	return g.Internal.StateWaitMsg(ctx, msg, confidence)
}

func (c *WalletStruct) WalletNew(ctx context.Context, typ types.KeyType) (address.Address, error) {
	return c.Internal.WalletNew(ctx, typ)
}

func (c *WalletStruct) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return c.Internal.WalletHas(ctx, addr)
}

func (c *WalletStruct) WalletList(ctx context.Context) ([]address.Address, error) {
	return c.Internal.WalletList(ctx)
}

func (c *WalletStruct) WalletSign(ctx context.Context, k address.Address, msg []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	return c.Internal.WalletSign(ctx, k, msg, meta)
}

func (c *WalletStruct) WalletExport(ctx context.Context, a address.Address) (*types.KeyInfo, error) {
	return c.Internal.WalletExport(ctx, a)
}

func (c *WalletStruct) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	return c.Internal.WalletImport(ctx, ki)
}

func (c *WalletStruct) WalletDelete(ctx context.Context, addr address.Address) error {
	return c.Internal.WalletDelete(ctx, addr)
}

var _ api.Common = &CommonStruct{}
var _ api.FullNode = &FullNodeStruct{}
var _ api.StorageMiner = &StorageMinerStruct{}
var _ api.WorkerAPI = &WorkerStruct{}
var _ api.GatewayAPI = &GatewayStruct{}
var _ api.WalletAPI = &WalletStruct{}

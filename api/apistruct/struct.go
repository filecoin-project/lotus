package apistruct

import (
	"context"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/go-bitfield"

	datatransfer "github.com/filecoin-project/go-data-transfer"

	"github.com/filecoin-project/go-fil-markets/piecestore"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"

	"github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/go-multistore"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"

	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"

	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"

	"github.com/filecoin-project/lotus/extern/sector-storage/stores"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"

	marketevents "github.com/filecoin-project/lotus/markets/loggers"

	"github.com/filecoin-project/lotus/node/modules/dtypes"

	"github.com/filecoin-project/specs-storage/storage"

	"github.com/google/uuid"

	"github.com/ipfs/go-cid"

	"github.com/libp2p/go-libp2p-core/metrics"

	"github.com/libp2p/go-libp2p-core/network"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/libp2p/go-libp2p-core/protocol"

	"time"
)

type ChainIOStruct struct {
	Internal struct {
		ChainHasObj func(p0 context.Context, p1 cid.Cid) (bool, error) ``

		ChainReadObj func(p0 context.Context, p1 cid.Cid) ([]byte, error) ``
	}
}

type CommonStruct struct {
	Internal struct {
		AuthNew func(p0 context.Context, p1 []auth.Permission) ([]byte, error) ``

		AuthVerify func(p0 context.Context, p1 string) ([]auth.Permission, error) ``

		Closing func(p0 context.Context) (<-chan struct{}, error) ``

		Discover func(p0 context.Context) (apitypes.OpenRPCDocument, error) ``

		ID func(p0 context.Context) (peer.ID, error) ``

		LogList func(p0 context.Context) ([]string, error) ``

		LogSetLevel func(p0 context.Context, p1 string, p2 string) error ``

		NetAddrsListen func(p0 context.Context) (peer.AddrInfo, error) ``

		NetAgentVersion func(p0 context.Context, p1 peer.ID) (string, error) ``

		NetAutoNatStatus func(p0 context.Context) (api.NatInfo, error) ``

		NetBandwidthStats func(p0 context.Context) (metrics.Stats, error) ``

		NetBandwidthStatsByPeer func(p0 context.Context) (map[string]metrics.Stats, error) ``

		NetBandwidthStatsByProtocol func(p0 context.Context) (map[protocol.ID]metrics.Stats, error) ``

		NetBlockAdd func(p0 context.Context, p1 api.NetBlockList) error ``

		NetBlockList func(p0 context.Context) (api.NetBlockList, error) ``

		NetBlockRemove func(p0 context.Context, p1 api.NetBlockList) error ``

		NetConnect func(p0 context.Context, p1 peer.AddrInfo) error ``

		NetConnectedness func(p0 context.Context, p1 peer.ID) (network.Connectedness, error) ``

		NetDisconnect func(p0 context.Context, p1 peer.ID) error ``

		NetFindPeer func(p0 context.Context, p1 peer.ID) (peer.AddrInfo, error) ``

		NetPeerInfo func(p0 context.Context, p1 peer.ID) (*api.ExtendedPeerInfo, error) ``

		NetPeers func(p0 context.Context) ([]peer.AddrInfo, error) ``

		NetPubsubScores func(p0 context.Context) ([]api.PubsubScore, error) ``

		Session func(p0 context.Context) (uuid.UUID, error) ``

		Shutdown func(p0 context.Context) error ``

		Version func(p0 context.Context) (api.APIVersion, error) ``
	}
}

type FullNodeStruct struct {
	Internal struct {
		BeaconGetEntry func(p0 context.Context, p1 abi.ChainEpoch) (*types.BeaconEntry, error) ``

		ChainDeleteObj func(p0 context.Context, p1 cid.Cid) error ``

		ChainExport func(p0 context.Context, p1 abi.ChainEpoch, p2 bool, p3 types.TipSetKey) (<-chan []byte, error) ``

		ChainGetBlock func(p0 context.Context, p1 cid.Cid) (*types.BlockHeader, error) ``

		ChainGetBlockMessages func(p0 context.Context, p1 cid.Cid) (*api.BlockMessages, error) ``

		ChainGetGenesis func(p0 context.Context) (*types.TipSet, error) ``

		ChainGetMessage func(p0 context.Context, p1 cid.Cid) (*types.Message, error) ``

		ChainGetNode func(p0 context.Context, p1 string) (*api.IpldObject, error) ``

		ChainGetParentMessages func(p0 context.Context, p1 cid.Cid) ([]api.Message, error) ``

		ChainGetParentReceipts func(p0 context.Context, p1 cid.Cid) ([]*types.MessageReceipt, error) ``

		ChainGetPath func(p0 context.Context, p1 types.TipSetKey, p2 types.TipSetKey) ([]*api.HeadChange, error) ``

		ChainGetRandomnessFromBeacon func(p0 context.Context, p1 types.TipSetKey, p2 crypto.DomainSeparationTag, p3 abi.ChainEpoch, p4 []byte) (abi.Randomness, error) ``

		ChainGetRandomnessFromTickets func(p0 context.Context, p1 types.TipSetKey, p2 crypto.DomainSeparationTag, p3 abi.ChainEpoch, p4 []byte) (abi.Randomness, error) ``

		ChainGetTipSet func(p0 context.Context, p1 types.TipSetKey) (*types.TipSet, error) ``

		ChainGetTipSetByHeight func(p0 context.Context, p1 abi.ChainEpoch, p2 types.TipSetKey) (*types.TipSet, error) ``

		ChainHasObj func(p0 context.Context, p1 cid.Cid) (bool, error) ``

		ChainHead func(p0 context.Context) (*types.TipSet, error) ``

		ChainNotify func(p0 context.Context) (<-chan []*api.HeadChange, error) ``

		ChainReadObj func(p0 context.Context, p1 cid.Cid) ([]byte, error) ``

		ChainSetHead func(p0 context.Context, p1 types.TipSetKey) error ``

		ChainStatObj func(p0 context.Context, p1 cid.Cid, p2 cid.Cid) (api.ObjStat, error) ``

		ChainTipSetWeight func(p0 context.Context, p1 types.TipSetKey) (types.BigInt, error) ``

		ClientCalcCommP func(p0 context.Context, p1 string) (*api.CommPRet, error) ``

		ClientCancelDataTransfer func(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error ``

		ClientDataTransferUpdates func(p0 context.Context) (<-chan api.DataTransferChannel, error) ``

		ClientDealPieceCID func(p0 context.Context, p1 cid.Cid) (api.DataCIDSize, error) ``

		ClientDealSize func(p0 context.Context, p1 cid.Cid) (api.DataSize, error) ``

		ClientFindData func(p0 context.Context, p1 cid.Cid, p2 *cid.Cid) ([]api.QueryOffer, error) ``

		ClientGenCar func(p0 context.Context, p1 api.FileRef, p2 string) error ``

		ClientGetDealInfo func(p0 context.Context, p1 cid.Cid) (*api.DealInfo, error) ``

		ClientGetDealStatus func(p0 context.Context, p1 uint64) (string, error) ``

		ClientGetDealUpdates func(p0 context.Context) (<-chan api.DealInfo, error) ``

		ClientHasLocal func(p0 context.Context, p1 cid.Cid) (bool, error) ``

		ClientImport func(p0 context.Context, p1 api.FileRef) (*api.ImportRes, error) ``

		ClientListDataTransfers func(p0 context.Context) ([]api.DataTransferChannel, error) ``

		ClientListDeals func(p0 context.Context) ([]api.DealInfo, error) ``

		ClientListImports func(p0 context.Context) ([]api.Import, error) ``

		ClientMinerQueryOffer func(p0 context.Context, p1 address.Address, p2 cid.Cid, p3 *cid.Cid) (api.QueryOffer, error) ``

		ClientQueryAsk func(p0 context.Context, p1 peer.ID, p2 address.Address) (*storagemarket.StorageAsk, error) ``

		ClientRemoveImport func(p0 context.Context, p1 multistore.StoreID) error ``

		ClientRestartDataTransfer func(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error ``

		ClientRetrieve func(p0 context.Context, p1 api.RetrievalOrder, p2 *api.FileRef) error ``

		ClientRetrieveTryRestartInsufficientFunds func(p0 context.Context, p1 address.Address) error ``

		ClientRetrieveWithEvents func(p0 context.Context, p1 api.RetrievalOrder, p2 *api.FileRef) (<-chan marketevents.RetrievalEvent, error) ``

		ClientStartDeal func(p0 context.Context, p1 *api.StartDealParams) (*cid.Cid, error) ``

		CreateBackup func(p0 context.Context, p1 string) error ``

		GasEstimateFeeCap func(p0 context.Context, p1 *types.Message, p2 int64, p3 types.TipSetKey) (types.BigInt, error) ``

		GasEstimateGasLimit func(p0 context.Context, p1 *types.Message, p2 types.TipSetKey) (int64, error) ``

		GasEstimateGasPremium func(p0 context.Context, p1 uint64, p2 address.Address, p3 int64, p4 types.TipSetKey) (types.BigInt, error) ``

		GasEstimateMessageGas func(p0 context.Context, p1 *types.Message, p2 *api.MessageSendSpec, p3 types.TipSetKey) (*types.Message, error) ``

		MarketAddBalance func(p0 context.Context, p1 address.Address, p2 types.BigInt) (cid.Cid, error) ``

		MarketGetReserved func(p0 context.Context, p1 address.Address) (types.BigInt, error) ``

		MarketReleaseFunds func(p0 context.Context, p1 address.Address, p2 types.BigInt) error ``

		MarketReserveFunds func(p0 context.Context, p1 address.Address, p2 address.Address, p3 types.BigInt) (cid.Cid, error) ``

		MarketWithdraw func(p0 context.Context, p1 address.Address, p2 types.BigInt) (cid.Cid, error) ``

		MinerCreateBlock func(p0 context.Context, p1 *api.BlockTemplate) (*types.BlockMsg, error) ``

		MinerGetBaseInfo func(p0 context.Context, p1 address.Address, p2 abi.ChainEpoch, p3 types.TipSetKey) (*api.MiningBaseInfo, error) ``

		MpoolBatchPush func(p0 context.Context, p1 []*types.SignedMessage) ([]cid.Cid, error) ``

		MpoolBatchPushMessage func(p0 context.Context, p1 []*types.Message, p2 *api.MessageSendSpec) ([]*types.SignedMessage, error) ``

		MpoolBatchPushUntrusted func(p0 context.Context, p1 []*types.SignedMessage) ([]cid.Cid, error) ``

		MpoolClear func(p0 context.Context, p1 bool) error ``

		MpoolGetConfig func(p0 context.Context) (*types.MpoolConfig, error) ``

		MpoolGetNonce func(p0 context.Context, p1 address.Address) (uint64, error) ``

		MpoolPending func(p0 context.Context, p1 types.TipSetKey) ([]*types.SignedMessage, error) ``

		MpoolPush func(p0 context.Context, p1 *types.SignedMessage) (cid.Cid, error) ``

		MpoolPushMessage func(p0 context.Context, p1 *types.Message, p2 *api.MessageSendSpec) (*types.SignedMessage, error) ``

		MpoolPushUntrusted func(p0 context.Context, p1 *types.SignedMessage) (cid.Cid, error) ``

		MpoolSelect func(p0 context.Context, p1 types.TipSetKey, p2 float64) ([]*types.SignedMessage, error) ``

		MpoolSetConfig func(p0 context.Context, p1 *types.MpoolConfig) error ``

		MpoolSub func(p0 context.Context) (<-chan api.MpoolUpdate, error) ``

		MsigAddApprove func(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 address.Address, p6 bool) (cid.Cid, error) ``

		MsigAddCancel func(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 bool) (cid.Cid, error) ``

		MsigAddPropose func(p0 context.Context, p1 address.Address, p2 address.Address, p3 address.Address, p4 bool) (cid.Cid, error) ``

		MsigApprove func(p0 context.Context, p1 address.Address, p2 uint64, p3 address.Address) (cid.Cid, error) ``

		MsigApproveTxnHash func(p0 context.Context, p1 address.Address, p2 uint64, p3 address.Address, p4 address.Address, p5 types.BigInt, p6 address.Address, p7 uint64, p8 []byte) (cid.Cid, error) ``

		MsigCancel func(p0 context.Context, p1 address.Address, p2 uint64, p3 address.Address, p4 types.BigInt, p5 address.Address, p6 uint64, p7 []byte) (cid.Cid, error) ``

		MsigCreate func(p0 context.Context, p1 uint64, p2 []address.Address, p3 abi.ChainEpoch, p4 types.BigInt, p5 address.Address, p6 types.BigInt) (cid.Cid, error) ``

		MsigGetAvailableBalance func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (types.BigInt, error) ``

		MsigGetPending func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]*api.MsigTransaction, error) ``

		MsigGetVested func(p0 context.Context, p1 address.Address, p2 types.TipSetKey, p3 types.TipSetKey) (types.BigInt, error) ``

		MsigGetVestingSchedule func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MsigVesting, error) ``

		MsigPropose func(p0 context.Context, p1 address.Address, p2 address.Address, p3 types.BigInt, p4 address.Address, p5 uint64, p6 []byte) (cid.Cid, error) ``

		MsigRemoveSigner func(p0 context.Context, p1 address.Address, p2 address.Address, p3 address.Address, p4 bool) (cid.Cid, error) ``

		MsigSwapApprove func(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 address.Address, p6 address.Address) (cid.Cid, error) ``

		MsigSwapCancel func(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 address.Address) (cid.Cid, error) ``

		MsigSwapPropose func(p0 context.Context, p1 address.Address, p2 address.Address, p3 address.Address, p4 address.Address) (cid.Cid, error) ``

		PaychAllocateLane func(p0 context.Context, p1 address.Address) (uint64, error) ``

		PaychAvailableFunds func(p0 context.Context, p1 address.Address) (*api.ChannelAvailableFunds, error) ``

		PaychAvailableFundsByFromTo func(p0 context.Context, p1 address.Address) (*api.ChannelAvailableFunds, error) ``

		PaychCollect func(p0 context.Context, p1 address.Address) (cid.Cid, error) ``

		PaychGet func(p0 context.Context, p1 address.Address, p2 types.BigInt) (*api.ChannelInfo, error) ``

		PaychGetWaitReady func(p0 context.Context, p1 cid.Cid) (address.Address, error) ``

		PaychList func(p0 context.Context) ([]address.Address, error) ``

		PaychNewPayment func(p0 context.Context, p1 address.Address, p2 []api.VoucherSpec) (*api.PaymentInfo, error) ``

		PaychSettle func(p0 context.Context, p1 address.Address) (cid.Cid, error) ``

		PaychStatus func(p0 context.Context, p1 address.Address) (*api.PaychStatus, error) ``

		PaychVoucherAdd func(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher, p3 []byte, p4 types.BigInt) (types.BigInt, error) ``

		PaychVoucherCheckSpendable func(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher, p3 []byte, p4 []byte) (bool, error) ``

		PaychVoucherCheckValid func(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher) error ``

		PaychVoucherCreate func(p0 context.Context, p1 address.Address, p2 types.BigInt, p3 uint64) (*api.VoucherCreateResult, error) ``

		PaychVoucherList func(p0 context.Context, p1 address.Address) ([]*paych.SignedVoucher, error) ``

		PaychVoucherSubmit func(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher, p3 []byte, p4 []byte) (cid.Cid, error) ``

		StateAccountKey func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) ``

		StateAllMinerFaults func(p0 context.Context, p1 abi.ChainEpoch, p2 types.TipSetKey) ([]*api.Fault, error) ``

		StateCall func(p0 context.Context, p1 *types.Message, p2 types.TipSetKey) (*api.InvocResult, error) ``

		StateChangedActors func(p0 context.Context, p1 cid.Cid, p2 cid.Cid) (map[string]types.Actor, error) ``

		StateCirculatingSupply func(p0 context.Context, p1 types.TipSetKey) (abi.TokenAmount, error) ``

		StateCompute func(p0 context.Context, p1 abi.ChainEpoch, p2 []*types.Message, p3 types.TipSetKey) (*api.ComputeStateOutput, error) ``

		StateDealProviderCollateralBounds func(p0 context.Context, p1 abi.PaddedPieceSize, p2 bool, p3 types.TipSetKey) (api.DealCollateralBounds, error) ``

		StateDecodeParams func(p0 context.Context, p1 address.Address, p2 abi.MethodNum, p3 []byte, p4 types.TipSetKey) (struct{}, error) ``

		StateGetActor func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*types.Actor, error) ``

		StateGetReceipt func(p0 context.Context, p1 cid.Cid, p2 types.TipSetKey) (*types.MessageReceipt, error) ``

		StateListActors func(p0 context.Context, p1 types.TipSetKey) ([]address.Address, error) ``

		StateListMessages func(p0 context.Context, p1 *api.MessageMatch, p2 types.TipSetKey, p3 abi.ChainEpoch) ([]cid.Cid, error) ``

		StateListMiners func(p0 context.Context, p1 types.TipSetKey) ([]address.Address, error) ``

		StateLookupID func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) ``

		StateMarketBalance func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MarketBalance, error) ``

		StateMarketDeals func(p0 context.Context, p1 types.TipSetKey) (map[string]api.MarketDeal, error) ``

		StateMarketParticipants func(p0 context.Context, p1 types.TipSetKey) (map[string]api.MarketBalance, error) ``

		StateMarketStorageDeal func(p0 context.Context, p1 abi.DealID, p2 types.TipSetKey) (*api.MarketDeal, error) ``

		StateMinerActiveSectors func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]*miner.SectorOnChainInfo, error) ``

		StateMinerAvailableBalance func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (types.BigInt, error) ``

		StateMinerDeadlines func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]api.Deadline, error) ``

		StateMinerFaults func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (bitfield.BitField, error) ``

		StateMinerInfo func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (miner.MinerInfo, error) ``

		StateMinerInitialPledgeCollateral func(p0 context.Context, p1 address.Address, p2 miner.SectorPreCommitInfo, p3 types.TipSetKey) (types.BigInt, error) ``

		StateMinerPartitions func(p0 context.Context, p1 address.Address, p2 uint64, p3 types.TipSetKey) ([]api.Partition, error) ``

		StateMinerPower func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*api.MinerPower, error) ``

		StateMinerPreCommitDepositForPower func(p0 context.Context, p1 address.Address, p2 miner.SectorPreCommitInfo, p3 types.TipSetKey) (types.BigInt, error) ``

		StateMinerProvingDeadline func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) ``

		StateMinerRecoveries func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (bitfield.BitField, error) ``

		StateMinerSectorAllocated func(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (bool, error) ``

		StateMinerSectorCount func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MinerSectors, error) ``

		StateMinerSectors func(p0 context.Context, p1 address.Address, p2 *bitfield.BitField, p3 types.TipSetKey) ([]*miner.SectorOnChainInfo, error) ``

		StateNetworkName func(p0 context.Context) (dtypes.NetworkName, error) ``

		StateNetworkVersion func(p0 context.Context, p1 types.TipSetKey) (apitypes.NetworkVersion, error) ``

		StateReadState func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*api.ActorState, error) ``

		StateReplay func(p0 context.Context, p1 types.TipSetKey, p2 cid.Cid) (*api.InvocResult, error) ``

		StateSearchMsg func(p0 context.Context, p1 cid.Cid) (*api.MsgLookup, error) ``

		StateSearchMsgLimited func(p0 context.Context, p1 cid.Cid, p2 abi.ChainEpoch) (*api.MsgLookup, error) ``

		StateSectorExpiration func(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorExpiration, error) ``

		StateSectorGetInfo func(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorOnChainInfo, error) ``

		StateSectorPartition func(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorLocation, error) ``

		StateSectorPreCommitInfo func(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) ``

		StateVMCirculatingSupplyInternal func(p0 context.Context, p1 types.TipSetKey) (api.CirculatingSupply, error) ``

		StateVerifiedClientStatus func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*abi.StoragePower, error) ``

		StateVerifiedRegistryRootKey func(p0 context.Context, p1 types.TipSetKey) (address.Address, error) ``

		StateVerifierStatus func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*abi.StoragePower, error) ``

		StateWaitMsg func(p0 context.Context, p1 cid.Cid, p2 uint64) (*api.MsgLookup, error) ``

		StateWaitMsgLimited func(p0 context.Context, p1 cid.Cid, p2 uint64, p3 abi.ChainEpoch) (*api.MsgLookup, error) ``

		SyncCheckBad func(p0 context.Context, p1 cid.Cid) (string, error) ``

		SyncCheckpoint func(p0 context.Context, p1 types.TipSetKey) error ``

		SyncIncomingBlocks func(p0 context.Context) (<-chan *types.BlockHeader, error) ``

		SyncMarkBad func(p0 context.Context, p1 cid.Cid) error ``

		SyncState func(p0 context.Context) (*api.SyncState, error) ``

		SyncSubmitBlock func(p0 context.Context, p1 *types.BlockMsg) error ``

		SyncUnmarkAllBad func(p0 context.Context) error ``

		SyncUnmarkBad func(p0 context.Context, p1 cid.Cid) error ``

		SyncValidateTipset func(p0 context.Context, p1 types.TipSetKey) (bool, error) ``

		WalletBalance func(p0 context.Context, p1 address.Address) (types.BigInt, error) ``

		WalletDefaultAddress func(p0 context.Context) (address.Address, error) ``

		WalletDelete func(p0 context.Context, p1 address.Address) error ``

		WalletExport func(p0 context.Context, p1 address.Address) (*types.KeyInfo, error) ``

		WalletHas func(p0 context.Context, p1 address.Address) (bool, error) ``

		WalletImport func(p0 context.Context, p1 *types.KeyInfo) (address.Address, error) ``

		WalletList func(p0 context.Context) ([]address.Address, error) ``

		WalletNew func(p0 context.Context, p1 types.KeyType) (address.Address, error) ``

		WalletSetDefault func(p0 context.Context, p1 address.Address) error ``

		WalletSign func(p0 context.Context, p1 address.Address, p2 []byte) (*crypto.Signature, error) ``

		WalletSignMessage func(p0 context.Context, p1 address.Address, p2 *types.Message) (*types.SignedMessage, error) ``

		WalletValidateAddress func(p0 context.Context, p1 string) (address.Address, error) ``

		WalletVerify func(p0 context.Context, p1 address.Address, p2 []byte, p3 *crypto.Signature) (bool, error) ``
	}
}

type GatewayAPIStruct struct {
	Internal struct {
		ChainGetBlockMessages func(p0 context.Context, p1 cid.Cid) (*api.BlockMessages, error) ``

		ChainGetMessage func(p0 context.Context, p1 cid.Cid) (*types.Message, error) ``

		ChainGetTipSet func(p0 context.Context, p1 types.TipSetKey) (*types.TipSet, error) ``

		ChainGetTipSetByHeight func(p0 context.Context, p1 abi.ChainEpoch, p2 types.TipSetKey) (*types.TipSet, error) ``

		ChainHasObj func(p0 context.Context, p1 cid.Cid) (bool, error) ``

		ChainHead func(p0 context.Context) (*types.TipSet, error) ``

		ChainNotify func(p0 context.Context) (<-chan []*api.HeadChange, error) ``

		ChainReadObj func(p0 context.Context, p1 cid.Cid) ([]byte, error) ``

		GasEstimateMessageGas func(p0 context.Context, p1 *types.Message, p2 *api.MessageSendSpec, p3 types.TipSetKey) (*types.Message, error) ``

		MpoolPush func(p0 context.Context, p1 *types.SignedMessage) (cid.Cid, error) ``

		MsigGetAvailableBalance func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (types.BigInt, error) ``

		MsigGetPending func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]*api.MsigTransaction, error) ``

		MsigGetVested func(p0 context.Context, p1 address.Address, p2 types.TipSetKey, p3 types.TipSetKey) (types.BigInt, error) ``

		StateAccountKey func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) ``

		StateDealProviderCollateralBounds func(p0 context.Context, p1 abi.PaddedPieceSize, p2 bool, p3 types.TipSetKey) (api.DealCollateralBounds, error) ``

		StateGetActor func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*types.Actor, error) ``

		StateGetReceipt func(p0 context.Context, p1 cid.Cid, p2 types.TipSetKey) (*types.MessageReceipt, error) ``

		StateListMiners func(p0 context.Context, p1 types.TipSetKey) ([]address.Address, error) ``

		StateLookupID func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) ``

		StateMarketBalance func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MarketBalance, error) ``

		StateMarketStorageDeal func(p0 context.Context, p1 abi.DealID, p2 types.TipSetKey) (*api.MarketDeal, error) ``

		StateMinerInfo func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (miner.MinerInfo, error) ``

		StateMinerPower func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*api.MinerPower, error) ``

		StateMinerProvingDeadline func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) ``

		StateNetworkVersion func(p0 context.Context, p1 types.TipSetKey) (apitypes.NetworkVersion, error) ``

		StateSearchMsg func(p0 context.Context, p1 cid.Cid) (*api.MsgLookup, error) ``

		StateSectorGetInfo func(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorOnChainInfo, error) ``

		StateVerifiedClientStatus func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*abi.StoragePower, error) ``

		StateWaitMsg func(p0 context.Context, p1 cid.Cid, p2 uint64) (*api.MsgLookup, error) ``
	}
}

type SignableStruct struct {
	Internal struct {
		Sign func(p0 context.Context, p1 api.SignFunc) error ``
	}
}

type StorageMinerStruct struct {
	Internal struct {
		ActorAddress func(p0 context.Context) (address.Address, error) ``

		ActorAddressConfig func(p0 context.Context) (api.AddressConfig, error) ``

		ActorSectorSize func(p0 context.Context, p1 address.Address) (abi.SectorSize, error) ``

		CheckProvable func(p0 context.Context, p1 abi.RegisteredPoStProof, p2 []storage.SectorRef, p3 bool) (map[abi.SectorNumber]string, error) ``

		CreateBackup func(p0 context.Context, p1 string) error ``

		DealsConsiderOfflineRetrievalDeals func(p0 context.Context) (bool, error) ``

		DealsConsiderOfflineStorageDeals func(p0 context.Context) (bool, error) ``

		DealsConsiderOnlineRetrievalDeals func(p0 context.Context) (bool, error) ``

		DealsConsiderOnlineStorageDeals func(p0 context.Context) (bool, error) ``

		DealsConsiderUnverifiedStorageDeals func(p0 context.Context) (bool, error) ``

		DealsConsiderVerifiedStorageDeals func(p0 context.Context) (bool, error) ``

		DealsImportData func(p0 context.Context, p1 cid.Cid, p2 string) error ``

		DealsList func(p0 context.Context) ([]api.MarketDeal, error) ``

		DealsPieceCidBlocklist func(p0 context.Context) ([]cid.Cid, error) ``

		DealsSetConsiderOfflineRetrievalDeals func(p0 context.Context, p1 bool) error ``

		DealsSetConsiderOfflineStorageDeals func(p0 context.Context, p1 bool) error ``

		DealsSetConsiderOnlineRetrievalDeals func(p0 context.Context, p1 bool) error ``

		DealsSetConsiderOnlineStorageDeals func(p0 context.Context, p1 bool) error ``

		DealsSetConsiderUnverifiedStorageDeals func(p0 context.Context, p1 bool) error ``

		DealsSetConsiderVerifiedStorageDeals func(p0 context.Context, p1 bool) error ``

		DealsSetPieceCidBlocklist func(p0 context.Context, p1 []cid.Cid) error ``

		MarketCancelDataTransfer func(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error ``

		MarketDataTransferUpdates func(p0 context.Context) (<-chan api.DataTransferChannel, error) ``

		MarketGetAsk func(p0 context.Context) (*storagemarket.SignedStorageAsk, error) ``

		MarketGetDealUpdates func(p0 context.Context) (<-chan storagemarket.MinerDeal, error) ``

		MarketGetRetrievalAsk func(p0 context.Context) (*retrievalmarket.Ask, error) ``

		MarketImportDealData func(p0 context.Context, p1 cid.Cid, p2 string) error ``

		MarketListDataTransfers func(p0 context.Context) ([]api.DataTransferChannel, error) ``

		MarketListDeals func(p0 context.Context) ([]api.MarketDeal, error) ``

		MarketListIncompleteDeals func(p0 context.Context) ([]storagemarket.MinerDeal, error) ``

		MarketListRetrievalDeals func(p0 context.Context) ([]retrievalmarket.ProviderDealState, error) ``

		MarketPendingDeals func(p0 context.Context) (api.PendingDealInfo, error) ``

		MarketPublishPendingDeals func(p0 context.Context) error ``

		MarketRestartDataTransfer func(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error ``

		MarketSetAsk func(p0 context.Context, p1 types.BigInt, p2 types.BigInt, p3 abi.ChainEpoch, p4 abi.PaddedPieceSize, p5 abi.PaddedPieceSize) error ``

		MarketSetRetrievalAsk func(p0 context.Context, p1 *retrievalmarket.Ask) error ``

		MiningBase func(p0 context.Context) (*types.TipSet, error) ``

		PiecesGetCIDInfo func(p0 context.Context, p1 cid.Cid) (*piecestore.CIDInfo, error) ``

		PiecesGetPieceInfo func(p0 context.Context, p1 cid.Cid) (*piecestore.PieceInfo, error) ``

		PiecesListCidInfos func(p0 context.Context) ([]cid.Cid, error) ``

		PiecesListPieces func(p0 context.Context) ([]cid.Cid, error) ``

		PledgeSector func(p0 context.Context) (abi.SectorID, error) ``

		SealingAbort func(p0 context.Context, p1 storiface.CallID) error ``

		SealingSchedDiag func(p0 context.Context, p1 bool) (struct{}, error) ``

		SectorGetExpectedSealDuration func(p0 context.Context) (time.Duration, error) ``

		SectorGetSealDelay func(p0 context.Context) (time.Duration, error) ``

		SectorMarkForUpgrade func(p0 context.Context, p1 abi.SectorNumber) error ``

		SectorRemove func(p0 context.Context, p1 abi.SectorNumber) error ``

		SectorSetExpectedSealDuration func(p0 context.Context, p1 time.Duration) error ``

		SectorSetSealDelay func(p0 context.Context, p1 time.Duration) error ``

		SectorStartSealing func(p0 context.Context, p1 abi.SectorNumber) error ``

		SectorTerminate func(p0 context.Context, p1 abi.SectorNumber) error ``

		SectorTerminateFlush func(p0 context.Context) (*cid.Cid, error) ``

		SectorTerminatePending func(p0 context.Context) ([]abi.SectorID, error) ``

		SectorsList func(p0 context.Context) ([]abi.SectorNumber, error) ``

		SectorsListInStates func(p0 context.Context, p1 []api.SectorState) ([]abi.SectorNumber, error) ``

		SectorsRefs func(p0 context.Context) (map[string][]api.SealedRef, error) ``

		SectorsStatus func(p0 context.Context, p1 abi.SectorNumber, p2 bool) (api.SectorInfo, error) ``

		SectorsSummary func(p0 context.Context) (map[api.SectorState]int, error) ``

		SectorsUpdate func(p0 context.Context, p1 abi.SectorNumber, p2 api.SectorState) error ``

		StorageAddLocal func(p0 context.Context, p1 string) error ``

		StorageList func(p0 context.Context) (map[stores.ID][]stores.Decl, error) ``

		StorageLocal func(p0 context.Context) (map[stores.ID]string, error) ``

		StorageStat func(p0 context.Context, p1 stores.ID) (fsutil.FsStat, error) ``

		WorkerConnect func(p0 context.Context, p1 string) error ``

		WorkerJobs func(p0 context.Context) (map[uuid.UUID][]storiface.WorkerJob, error) ``

		WorkerStats func(p0 context.Context) (map[uuid.UUID]storiface.WorkerStats, error) ``
	}
}

type WalletAPIStruct struct {
	Internal struct {
		WalletDelete func(p0 context.Context, p1 address.Address) error ``

		WalletExport func(p0 context.Context, p1 address.Address) (*types.KeyInfo, error) ``

		WalletHas func(p0 context.Context, p1 address.Address) (bool, error) ``

		WalletImport func(p0 context.Context, p1 *types.KeyInfo) (address.Address, error) ``

		WalletList func(p0 context.Context) ([]address.Address, error) ``

		WalletNew func(p0 context.Context, p1 types.KeyType) (address.Address, error) ``

		WalletSign func(p0 context.Context, p1 address.Address, p2 []byte, p3 api.MsgMeta) (*crypto.Signature, error) ``
	}
}

type WorkerAPIStruct struct {
	Internal struct {
		Enabled func(p0 context.Context) (bool, error) ``

		Info func(p0 context.Context) (storiface.WorkerInfo, error) ``

		Paths func(p0 context.Context) ([]stores.StoragePath, error) ``

		ProcessSession func(p0 context.Context) (uuid.UUID, error) ``

		Remove func(p0 context.Context, p1 abi.SectorID) error ``

		Session func(p0 context.Context) (uuid.UUID, error) ``

		SetEnabled func(p0 context.Context, p1 bool) error ``

		StorageAddLocal func(p0 context.Context, p1 string) error ``

		TaskDisable func(p0 context.Context, p1 sealtasks.TaskType) error ``

		TaskEnable func(p0 context.Context, p1 sealtasks.TaskType) error ``

		TaskTypes func(p0 context.Context) (map[sealtasks.TaskType]struct{}, error) ``

		Version func(p0 context.Context) (api.Version, error) ``

		WaitQuiet func(p0 context.Context) error ``
	}
}

func (s *ChainIOStruct) ChainHasObj(p0 context.Context, p1 cid.Cid) (bool, error) {
	return s.Internal.ChainHasObj(p0, p1)
}

func (s *ChainIOStruct) ChainReadObj(p0 context.Context, p1 cid.Cid) ([]byte, error) {
	return s.Internal.ChainReadObj(p0, p1)
}

func (s *CommonStruct) AuthNew(p0 context.Context, p1 []auth.Permission) ([]byte, error) {
	return s.Internal.AuthNew(p0, p1)
}

func (s *CommonStruct) AuthVerify(p0 context.Context, p1 string) ([]auth.Permission, error) {
	return s.Internal.AuthVerify(p0, p1)
}

func (s *CommonStruct) Closing(p0 context.Context) (<-chan struct{}, error) {
	return s.Internal.Closing(p0)
}

func (s *CommonStruct) Discover(p0 context.Context) (apitypes.OpenRPCDocument, error) {
	return s.Internal.Discover(p0)
}

func (s *CommonStruct) ID(p0 context.Context) (peer.ID, error) {
	return s.Internal.ID(p0)
}

func (s *CommonStruct) LogList(p0 context.Context) ([]string, error) {
	return s.Internal.LogList(p0)
}

func (s *CommonStruct) LogSetLevel(p0 context.Context, p1 string, p2 string) error {
	return s.Internal.LogSetLevel(p0, p1, p2)
}

func (s *CommonStruct) NetAddrsListen(p0 context.Context) (peer.AddrInfo, error) {
	return s.Internal.NetAddrsListen(p0)
}

func (s *CommonStruct) NetAgentVersion(p0 context.Context, p1 peer.ID) (string, error) {
	return s.Internal.NetAgentVersion(p0, p1)
}

func (s *CommonStruct) NetAutoNatStatus(p0 context.Context) (api.NatInfo, error) {
	return s.Internal.NetAutoNatStatus(p0)
}

func (s *CommonStruct) NetBandwidthStats(p0 context.Context) (metrics.Stats, error) {
	return s.Internal.NetBandwidthStats(p0)
}

func (s *CommonStruct) NetBandwidthStatsByPeer(p0 context.Context) (map[string]metrics.Stats, error) {
	return s.Internal.NetBandwidthStatsByPeer(p0)
}

func (s *CommonStruct) NetBandwidthStatsByProtocol(p0 context.Context) (map[protocol.ID]metrics.Stats, error) {
	return s.Internal.NetBandwidthStatsByProtocol(p0)
}

func (s *CommonStruct) NetBlockAdd(p0 context.Context, p1 api.NetBlockList) error {
	return s.Internal.NetBlockAdd(p0, p1)
}

func (s *CommonStruct) NetBlockList(p0 context.Context) (api.NetBlockList, error) {
	return s.Internal.NetBlockList(p0)
}

func (s *CommonStruct) NetBlockRemove(p0 context.Context, p1 api.NetBlockList) error {
	return s.Internal.NetBlockRemove(p0, p1)
}

func (s *CommonStruct) NetConnect(p0 context.Context, p1 peer.AddrInfo) error {
	return s.Internal.NetConnect(p0, p1)
}

func (s *CommonStruct) NetConnectedness(p0 context.Context, p1 peer.ID) (network.Connectedness, error) {
	return s.Internal.NetConnectedness(p0, p1)
}

func (s *CommonStruct) NetDisconnect(p0 context.Context, p1 peer.ID) error {
	return s.Internal.NetDisconnect(p0, p1)
}

func (s *CommonStruct) NetFindPeer(p0 context.Context, p1 peer.ID) (peer.AddrInfo, error) {
	return s.Internal.NetFindPeer(p0, p1)
}

func (s *CommonStruct) NetPeerInfo(p0 context.Context, p1 peer.ID) (*api.ExtendedPeerInfo, error) {
	return s.Internal.NetPeerInfo(p0, p1)
}

func (s *CommonStruct) NetPeers(p0 context.Context) ([]peer.AddrInfo, error) {
	return s.Internal.NetPeers(p0)
}

func (s *CommonStruct) NetPubsubScores(p0 context.Context) ([]api.PubsubScore, error) {
	return s.Internal.NetPubsubScores(p0)
}

func (s *CommonStruct) Session(p0 context.Context) (uuid.UUID, error) {
	return s.Internal.Session(p0)
}

func (s *CommonStruct) Shutdown(p0 context.Context) error {
	return s.Internal.Shutdown(p0)
}

func (s *CommonStruct) Version(p0 context.Context) (api.APIVersion, error) {
	return s.Internal.Version(p0)
}

func (s *FullNodeStruct) BeaconGetEntry(p0 context.Context, p1 abi.ChainEpoch) (*types.BeaconEntry, error) {
	return s.Internal.BeaconGetEntry(p0, p1)
}

func (s *FullNodeStruct) ChainDeleteObj(p0 context.Context, p1 cid.Cid) error {
	return s.Internal.ChainDeleteObj(p0, p1)
}

func (s *FullNodeStruct) ChainExport(p0 context.Context, p1 abi.ChainEpoch, p2 bool, p3 types.TipSetKey) (<-chan []byte, error) {
	return s.Internal.ChainExport(p0, p1, p2, p3)
}

func (s *FullNodeStruct) ChainGetBlock(p0 context.Context, p1 cid.Cid) (*types.BlockHeader, error) {
	return s.Internal.ChainGetBlock(p0, p1)
}

func (s *FullNodeStruct) ChainGetBlockMessages(p0 context.Context, p1 cid.Cid) (*api.BlockMessages, error) {
	return s.Internal.ChainGetBlockMessages(p0, p1)
}

func (s *FullNodeStruct) ChainGetGenesis(p0 context.Context) (*types.TipSet, error) {
	return s.Internal.ChainGetGenesis(p0)
}

func (s *FullNodeStruct) ChainGetMessage(p0 context.Context, p1 cid.Cid) (*types.Message, error) {
	return s.Internal.ChainGetMessage(p0, p1)
}

func (s *FullNodeStruct) ChainGetNode(p0 context.Context, p1 string) (*api.IpldObject, error) {
	return s.Internal.ChainGetNode(p0, p1)
}

func (s *FullNodeStruct) ChainGetParentMessages(p0 context.Context, p1 cid.Cid) ([]api.Message, error) {
	return s.Internal.ChainGetParentMessages(p0, p1)
}

func (s *FullNodeStruct) ChainGetParentReceipts(p0 context.Context, p1 cid.Cid) ([]*types.MessageReceipt, error) {
	return s.Internal.ChainGetParentReceipts(p0, p1)
}

func (s *FullNodeStruct) ChainGetPath(p0 context.Context, p1 types.TipSetKey, p2 types.TipSetKey) ([]*api.HeadChange, error) {
	return s.Internal.ChainGetPath(p0, p1, p2)
}

func (s *FullNodeStruct) ChainGetRandomnessFromBeacon(p0 context.Context, p1 types.TipSetKey, p2 crypto.DomainSeparationTag, p3 abi.ChainEpoch, p4 []byte) (abi.Randomness, error) {
	return s.Internal.ChainGetRandomnessFromBeacon(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) ChainGetRandomnessFromTickets(p0 context.Context, p1 types.TipSetKey, p2 crypto.DomainSeparationTag, p3 abi.ChainEpoch, p4 []byte) (abi.Randomness, error) {
	return s.Internal.ChainGetRandomnessFromTickets(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) ChainGetTipSet(p0 context.Context, p1 types.TipSetKey) (*types.TipSet, error) {
	return s.Internal.ChainGetTipSet(p0, p1)
}

func (s *FullNodeStruct) ChainGetTipSetByHeight(p0 context.Context, p1 abi.ChainEpoch, p2 types.TipSetKey) (*types.TipSet, error) {
	return s.Internal.ChainGetTipSetByHeight(p0, p1, p2)
}

func (s *FullNodeStruct) ChainHasObj(p0 context.Context, p1 cid.Cid) (bool, error) {
	return s.Internal.ChainHasObj(p0, p1)
}

func (s *FullNodeStruct) ChainHead(p0 context.Context) (*types.TipSet, error) {
	return s.Internal.ChainHead(p0)
}

func (s *FullNodeStruct) ChainNotify(p0 context.Context) (<-chan []*api.HeadChange, error) {
	return s.Internal.ChainNotify(p0)
}

func (s *FullNodeStruct) ChainReadObj(p0 context.Context, p1 cid.Cid) ([]byte, error) {
	return s.Internal.ChainReadObj(p0, p1)
}

func (s *FullNodeStruct) ChainSetHead(p0 context.Context, p1 types.TipSetKey) error {
	return s.Internal.ChainSetHead(p0, p1)
}

func (s *FullNodeStruct) ChainStatObj(p0 context.Context, p1 cid.Cid, p2 cid.Cid) (api.ObjStat, error) {
	return s.Internal.ChainStatObj(p0, p1, p2)
}

func (s *FullNodeStruct) ChainTipSetWeight(p0 context.Context, p1 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.ChainTipSetWeight(p0, p1)
}

func (s *FullNodeStruct) ClientCalcCommP(p0 context.Context, p1 string) (*api.CommPRet, error) {
	return s.Internal.ClientCalcCommP(p0, p1)
}

func (s *FullNodeStruct) ClientCancelDataTransfer(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error {
	return s.Internal.ClientCancelDataTransfer(p0, p1, p2, p3)
}

func (s *FullNodeStruct) ClientDataTransferUpdates(p0 context.Context) (<-chan api.DataTransferChannel, error) {
	return s.Internal.ClientDataTransferUpdates(p0)
}

func (s *FullNodeStruct) ClientDealPieceCID(p0 context.Context, p1 cid.Cid) (api.DataCIDSize, error) {
	return s.Internal.ClientDealPieceCID(p0, p1)
}

func (s *FullNodeStruct) ClientDealSize(p0 context.Context, p1 cid.Cid) (api.DataSize, error) {
	return s.Internal.ClientDealSize(p0, p1)
}

func (s *FullNodeStruct) ClientFindData(p0 context.Context, p1 cid.Cid, p2 *cid.Cid) ([]api.QueryOffer, error) {
	return s.Internal.ClientFindData(p0, p1, p2)
}

func (s *FullNodeStruct) ClientGenCar(p0 context.Context, p1 api.FileRef, p2 string) error {
	return s.Internal.ClientGenCar(p0, p1, p2)
}

func (s *FullNodeStruct) ClientGetDealInfo(p0 context.Context, p1 cid.Cid) (*api.DealInfo, error) {
	return s.Internal.ClientGetDealInfo(p0, p1)
}

func (s *FullNodeStruct) ClientGetDealStatus(p0 context.Context, p1 uint64) (string, error) {
	return s.Internal.ClientGetDealStatus(p0, p1)
}

func (s *FullNodeStruct) ClientGetDealUpdates(p0 context.Context) (<-chan api.DealInfo, error) {
	return s.Internal.ClientGetDealUpdates(p0)
}

func (s *FullNodeStruct) ClientHasLocal(p0 context.Context, p1 cid.Cid) (bool, error) {
	return s.Internal.ClientHasLocal(p0, p1)
}

func (s *FullNodeStruct) ClientImport(p0 context.Context, p1 api.FileRef) (*api.ImportRes, error) {
	return s.Internal.ClientImport(p0, p1)
}

func (s *FullNodeStruct) ClientListDataTransfers(p0 context.Context) ([]api.DataTransferChannel, error) {
	return s.Internal.ClientListDataTransfers(p0)
}

func (s *FullNodeStruct) ClientListDeals(p0 context.Context) ([]api.DealInfo, error) {
	return s.Internal.ClientListDeals(p0)
}

func (s *FullNodeStruct) ClientListImports(p0 context.Context) ([]api.Import, error) {
	return s.Internal.ClientListImports(p0)
}

func (s *FullNodeStruct) ClientMinerQueryOffer(p0 context.Context, p1 address.Address, p2 cid.Cid, p3 *cid.Cid) (api.QueryOffer, error) {
	return s.Internal.ClientMinerQueryOffer(p0, p1, p2, p3)
}

func (s *FullNodeStruct) ClientQueryAsk(p0 context.Context, p1 peer.ID, p2 address.Address) (*storagemarket.StorageAsk, error) {
	return s.Internal.ClientQueryAsk(p0, p1, p2)
}

func (s *FullNodeStruct) ClientRemoveImport(p0 context.Context, p1 multistore.StoreID) error {
	return s.Internal.ClientRemoveImport(p0, p1)
}

func (s *FullNodeStruct) ClientRestartDataTransfer(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error {
	return s.Internal.ClientRestartDataTransfer(p0, p1, p2, p3)
}

func (s *FullNodeStruct) ClientRetrieve(p0 context.Context, p1 api.RetrievalOrder, p2 *api.FileRef) error {
	return s.Internal.ClientRetrieve(p0, p1, p2)
}

func (s *FullNodeStruct) ClientRetrieveTryRestartInsufficientFunds(p0 context.Context, p1 address.Address) error {
	return s.Internal.ClientRetrieveTryRestartInsufficientFunds(p0, p1)
}

func (s *FullNodeStruct) ClientRetrieveWithEvents(p0 context.Context, p1 api.RetrievalOrder, p2 *api.FileRef) (<-chan marketevents.RetrievalEvent, error) {
	return s.Internal.ClientRetrieveWithEvents(p0, p1, p2)
}

func (s *FullNodeStruct) ClientStartDeal(p0 context.Context, p1 *api.StartDealParams) (*cid.Cid, error) {
	return s.Internal.ClientStartDeal(p0, p1)
}

func (s *FullNodeStruct) CreateBackup(p0 context.Context, p1 string) error {
	return s.Internal.CreateBackup(p0, p1)
}

func (s *FullNodeStruct) GasEstimateFeeCap(p0 context.Context, p1 *types.Message, p2 int64, p3 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.GasEstimateFeeCap(p0, p1, p2, p3)
}

func (s *FullNodeStruct) GasEstimateGasLimit(p0 context.Context, p1 *types.Message, p2 types.TipSetKey) (int64, error) {
	return s.Internal.GasEstimateGasLimit(p0, p1, p2)
}

func (s *FullNodeStruct) GasEstimateGasPremium(p0 context.Context, p1 uint64, p2 address.Address, p3 int64, p4 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.GasEstimateGasPremium(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) GasEstimateMessageGas(p0 context.Context, p1 *types.Message, p2 *api.MessageSendSpec, p3 types.TipSetKey) (*types.Message, error) {
	return s.Internal.GasEstimateMessageGas(p0, p1, p2, p3)
}

func (s *FullNodeStruct) MarketAddBalance(p0 context.Context, p1 address.Address, p2 types.BigInt) (cid.Cid, error) {
	return s.Internal.MarketAddBalance(p0, p1, p2)
}

func (s *FullNodeStruct) MarketGetReserved(p0 context.Context, p1 address.Address) (types.BigInt, error) {
	return s.Internal.MarketGetReserved(p0, p1)
}

func (s *FullNodeStruct) MarketReleaseFunds(p0 context.Context, p1 address.Address, p2 types.BigInt) error {
	return s.Internal.MarketReleaseFunds(p0, p1, p2)
}

func (s *FullNodeStruct) MarketReserveFunds(p0 context.Context, p1 address.Address, p2 address.Address, p3 types.BigInt) (cid.Cid, error) {
	return s.Internal.MarketReserveFunds(p0, p1, p2, p3)
}

func (s *FullNodeStruct) MarketWithdraw(p0 context.Context, p1 address.Address, p2 types.BigInt) (cid.Cid, error) {
	return s.Internal.MarketWithdraw(p0, p1, p2)
}

func (s *FullNodeStruct) MinerCreateBlock(p0 context.Context, p1 *api.BlockTemplate) (*types.BlockMsg, error) {
	return s.Internal.MinerCreateBlock(p0, p1)
}

func (s *FullNodeStruct) MinerGetBaseInfo(p0 context.Context, p1 address.Address, p2 abi.ChainEpoch, p3 types.TipSetKey) (*api.MiningBaseInfo, error) {
	return s.Internal.MinerGetBaseInfo(p0, p1, p2, p3)
}

func (s *FullNodeStruct) MpoolBatchPush(p0 context.Context, p1 []*types.SignedMessage) ([]cid.Cid, error) {
	return s.Internal.MpoolBatchPush(p0, p1)
}

func (s *FullNodeStruct) MpoolBatchPushMessage(p0 context.Context, p1 []*types.Message, p2 *api.MessageSendSpec) ([]*types.SignedMessage, error) {
	return s.Internal.MpoolBatchPushMessage(p0, p1, p2)
}

func (s *FullNodeStruct) MpoolBatchPushUntrusted(p0 context.Context, p1 []*types.SignedMessage) ([]cid.Cid, error) {
	return s.Internal.MpoolBatchPushUntrusted(p0, p1)
}

func (s *FullNodeStruct) MpoolClear(p0 context.Context, p1 bool) error {
	return s.Internal.MpoolClear(p0, p1)
}

func (s *FullNodeStruct) MpoolGetConfig(p0 context.Context) (*types.MpoolConfig, error) {
	return s.Internal.MpoolGetConfig(p0)
}

func (s *FullNodeStruct) MpoolGetNonce(p0 context.Context, p1 address.Address) (uint64, error) {
	return s.Internal.MpoolGetNonce(p0, p1)
}

func (s *FullNodeStruct) MpoolPending(p0 context.Context, p1 types.TipSetKey) ([]*types.SignedMessage, error) {
	return s.Internal.MpoolPending(p0, p1)
}

func (s *FullNodeStruct) MpoolPush(p0 context.Context, p1 *types.SignedMessage) (cid.Cid, error) {
	return s.Internal.MpoolPush(p0, p1)
}

func (s *FullNodeStruct) MpoolPushMessage(p0 context.Context, p1 *types.Message, p2 *api.MessageSendSpec) (*types.SignedMessage, error) {
	return s.Internal.MpoolPushMessage(p0, p1, p2)
}

func (s *FullNodeStruct) MpoolPushUntrusted(p0 context.Context, p1 *types.SignedMessage) (cid.Cid, error) {
	return s.Internal.MpoolPushUntrusted(p0, p1)
}

func (s *FullNodeStruct) MpoolSelect(p0 context.Context, p1 types.TipSetKey, p2 float64) ([]*types.SignedMessage, error) {
	return s.Internal.MpoolSelect(p0, p1, p2)
}

func (s *FullNodeStruct) MpoolSetConfig(p0 context.Context, p1 *types.MpoolConfig) error {
	return s.Internal.MpoolSetConfig(p0, p1)
}

func (s *FullNodeStruct) MpoolSub(p0 context.Context) (<-chan api.MpoolUpdate, error) {
	return s.Internal.MpoolSub(p0)
}

func (s *FullNodeStruct) MsigAddApprove(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 address.Address, p6 bool) (cid.Cid, error) {
	return s.Internal.MsigAddApprove(p0, p1, p2, p3, p4, p5, p6)
}

func (s *FullNodeStruct) MsigAddCancel(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 bool) (cid.Cid, error) {
	return s.Internal.MsigAddCancel(p0, p1, p2, p3, p4, p5)
}

func (s *FullNodeStruct) MsigAddPropose(p0 context.Context, p1 address.Address, p2 address.Address, p3 address.Address, p4 bool) (cid.Cid, error) {
	return s.Internal.MsigAddPropose(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) MsigApprove(p0 context.Context, p1 address.Address, p2 uint64, p3 address.Address) (cid.Cid, error) {
	return s.Internal.MsigApprove(p0, p1, p2, p3)
}

func (s *FullNodeStruct) MsigApproveTxnHash(p0 context.Context, p1 address.Address, p2 uint64, p3 address.Address, p4 address.Address, p5 types.BigInt, p6 address.Address, p7 uint64, p8 []byte) (cid.Cid, error) {
	return s.Internal.MsigApproveTxnHash(p0, p1, p2, p3, p4, p5, p6, p7, p8)
}

func (s *FullNodeStruct) MsigCancel(p0 context.Context, p1 address.Address, p2 uint64, p3 address.Address, p4 types.BigInt, p5 address.Address, p6 uint64, p7 []byte) (cid.Cid, error) {
	return s.Internal.MsigCancel(p0, p1, p2, p3, p4, p5, p6, p7)
}

func (s *FullNodeStruct) MsigCreate(p0 context.Context, p1 uint64, p2 []address.Address, p3 abi.ChainEpoch, p4 types.BigInt, p5 address.Address, p6 types.BigInt) (cid.Cid, error) {
	return s.Internal.MsigCreate(p0, p1, p2, p3, p4, p5, p6)
}

func (s *FullNodeStruct) MsigGetAvailableBalance(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.MsigGetAvailableBalance(p0, p1, p2)
}

func (s *FullNodeStruct) MsigGetPending(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]*api.MsigTransaction, error) {
	return s.Internal.MsigGetPending(p0, p1, p2)
}

func (s *FullNodeStruct) MsigGetVested(p0 context.Context, p1 address.Address, p2 types.TipSetKey, p3 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.MsigGetVested(p0, p1, p2, p3)
}

func (s *FullNodeStruct) MsigGetVestingSchedule(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MsigVesting, error) {
	return s.Internal.MsigGetVestingSchedule(p0, p1, p2)
}

func (s *FullNodeStruct) MsigPropose(p0 context.Context, p1 address.Address, p2 address.Address, p3 types.BigInt, p4 address.Address, p5 uint64, p6 []byte) (cid.Cid, error) {
	return s.Internal.MsigPropose(p0, p1, p2, p3, p4, p5, p6)
}

func (s *FullNodeStruct) MsigRemoveSigner(p0 context.Context, p1 address.Address, p2 address.Address, p3 address.Address, p4 bool) (cid.Cid, error) {
	return s.Internal.MsigRemoveSigner(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) MsigSwapApprove(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 address.Address, p6 address.Address) (cid.Cid, error) {
	return s.Internal.MsigSwapApprove(p0, p1, p2, p3, p4, p5, p6)
}

func (s *FullNodeStruct) MsigSwapCancel(p0 context.Context, p1 address.Address, p2 address.Address, p3 uint64, p4 address.Address, p5 address.Address) (cid.Cid, error) {
	return s.Internal.MsigSwapCancel(p0, p1, p2, p3, p4, p5)
}

func (s *FullNodeStruct) MsigSwapPropose(p0 context.Context, p1 address.Address, p2 address.Address, p3 address.Address, p4 address.Address) (cid.Cid, error) {
	return s.Internal.MsigSwapPropose(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) PaychAllocateLane(p0 context.Context, p1 address.Address) (uint64, error) {
	return s.Internal.PaychAllocateLane(p0, p1)
}

func (s *FullNodeStruct) PaychAvailableFunds(p0 context.Context, p1 address.Address) (*api.ChannelAvailableFunds, error) {
	return s.Internal.PaychAvailableFunds(p0, p1)
}

func (s *FullNodeStruct) PaychAvailableFundsByFromTo(p0 context.Context, p1 address.Address) (*api.ChannelAvailableFunds, error) {
	return s.Internal.PaychAvailableFundsByFromTo(p0, p1)
}

func (s *FullNodeStruct) PaychCollect(p0 context.Context, p1 address.Address) (cid.Cid, error) {
	return s.Internal.PaychCollect(p0, p1)
}

func (s *FullNodeStruct) PaychGet(p0 context.Context, p1 address.Address, p2 types.BigInt) (*api.ChannelInfo, error) {
	return s.Internal.PaychGet(p0, p1, p2)
}

func (s *FullNodeStruct) PaychGetWaitReady(p0 context.Context, p1 cid.Cid) (address.Address, error) {
	return s.Internal.PaychGetWaitReady(p0, p1)
}

func (s *FullNodeStruct) PaychList(p0 context.Context) ([]address.Address, error) {
	return s.Internal.PaychList(p0)
}

func (s *FullNodeStruct) PaychNewPayment(p0 context.Context, p1 address.Address, p2 []api.VoucherSpec) (*api.PaymentInfo, error) {
	return s.Internal.PaychNewPayment(p0, p1, p2)
}

func (s *FullNodeStruct) PaychSettle(p0 context.Context, p1 address.Address) (cid.Cid, error) {
	return s.Internal.PaychSettle(p0, p1)
}

func (s *FullNodeStruct) PaychStatus(p0 context.Context, p1 address.Address) (*api.PaychStatus, error) {
	return s.Internal.PaychStatus(p0, p1)
}

func (s *FullNodeStruct) PaychVoucherAdd(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher, p3 []byte, p4 types.BigInt) (types.BigInt, error) {
	return s.Internal.PaychVoucherAdd(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) PaychVoucherCheckSpendable(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher, p3 []byte, p4 []byte) (bool, error) {
	return s.Internal.PaychVoucherCheckSpendable(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) PaychVoucherCheckValid(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher) error {
	return s.Internal.PaychVoucherCheckValid(p0, p1, p2)
}

func (s *FullNodeStruct) PaychVoucherCreate(p0 context.Context, p1 address.Address, p2 types.BigInt, p3 uint64) (*api.VoucherCreateResult, error) {
	return s.Internal.PaychVoucherCreate(p0, p1, p2, p3)
}

func (s *FullNodeStruct) PaychVoucherList(p0 context.Context, p1 address.Address) ([]*paych.SignedVoucher, error) {
	return s.Internal.PaychVoucherList(p0, p1)
}

func (s *FullNodeStruct) PaychVoucherSubmit(p0 context.Context, p1 address.Address, p2 *paych.SignedVoucher, p3 []byte, p4 []byte) (cid.Cid, error) {
	return s.Internal.PaychVoucherSubmit(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) StateAccountKey(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) {
	return s.Internal.StateAccountKey(p0, p1, p2)
}

func (s *FullNodeStruct) StateAllMinerFaults(p0 context.Context, p1 abi.ChainEpoch, p2 types.TipSetKey) ([]*api.Fault, error) {
	return s.Internal.StateAllMinerFaults(p0, p1, p2)
}

func (s *FullNodeStruct) StateCall(p0 context.Context, p1 *types.Message, p2 types.TipSetKey) (*api.InvocResult, error) {
	return s.Internal.StateCall(p0, p1, p2)
}

func (s *FullNodeStruct) StateChangedActors(p0 context.Context, p1 cid.Cid, p2 cid.Cid) (map[string]types.Actor, error) {
	return s.Internal.StateChangedActors(p0, p1, p2)
}

func (s *FullNodeStruct) StateCirculatingSupply(p0 context.Context, p1 types.TipSetKey) (abi.TokenAmount, error) {
	return s.Internal.StateCirculatingSupply(p0, p1)
}

func (s *FullNodeStruct) StateCompute(p0 context.Context, p1 abi.ChainEpoch, p2 []*types.Message, p3 types.TipSetKey) (*api.ComputeStateOutput, error) {
	return s.Internal.StateCompute(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateDealProviderCollateralBounds(p0 context.Context, p1 abi.PaddedPieceSize, p2 bool, p3 types.TipSetKey) (api.DealCollateralBounds, error) {
	return s.Internal.StateDealProviderCollateralBounds(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateDecodeParams(p0 context.Context, p1 address.Address, p2 abi.MethodNum, p3 []byte, p4 types.TipSetKey) (struct{}, error) {
	return s.Internal.StateDecodeParams(p0, p1, p2, p3, p4)
}

func (s *FullNodeStruct) StateGetActor(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*types.Actor, error) {
	return s.Internal.StateGetActor(p0, p1, p2)
}

func (s *FullNodeStruct) StateGetReceipt(p0 context.Context, p1 cid.Cid, p2 types.TipSetKey) (*types.MessageReceipt, error) {
	return s.Internal.StateGetReceipt(p0, p1, p2)
}

func (s *FullNodeStruct) StateListActors(p0 context.Context, p1 types.TipSetKey) ([]address.Address, error) {
	return s.Internal.StateListActors(p0, p1)
}

func (s *FullNodeStruct) StateListMessages(p0 context.Context, p1 *api.MessageMatch, p2 types.TipSetKey, p3 abi.ChainEpoch) ([]cid.Cid, error) {
	return s.Internal.StateListMessages(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateListMiners(p0 context.Context, p1 types.TipSetKey) ([]address.Address, error) {
	return s.Internal.StateListMiners(p0, p1)
}

func (s *FullNodeStruct) StateLookupID(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) {
	return s.Internal.StateLookupID(p0, p1, p2)
}

func (s *FullNodeStruct) StateMarketBalance(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MarketBalance, error) {
	return s.Internal.StateMarketBalance(p0, p1, p2)
}

func (s *FullNodeStruct) StateMarketDeals(p0 context.Context, p1 types.TipSetKey) (map[string]api.MarketDeal, error) {
	return s.Internal.StateMarketDeals(p0, p1)
}

func (s *FullNodeStruct) StateMarketParticipants(p0 context.Context, p1 types.TipSetKey) (map[string]api.MarketBalance, error) {
	return s.Internal.StateMarketParticipants(p0, p1)
}

func (s *FullNodeStruct) StateMarketStorageDeal(p0 context.Context, p1 abi.DealID, p2 types.TipSetKey) (*api.MarketDeal, error) {
	return s.Internal.StateMarketStorageDeal(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerActiveSectors(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	return s.Internal.StateMinerActiveSectors(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerAvailableBalance(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.StateMinerAvailableBalance(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerDeadlines(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]api.Deadline, error) {
	return s.Internal.StateMinerDeadlines(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerFaults(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (bitfield.BitField, error) {
	return s.Internal.StateMinerFaults(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerInfo(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (miner.MinerInfo, error) {
	return s.Internal.StateMinerInfo(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerInitialPledgeCollateral(p0 context.Context, p1 address.Address, p2 miner.SectorPreCommitInfo, p3 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.StateMinerInitialPledgeCollateral(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateMinerPartitions(p0 context.Context, p1 address.Address, p2 uint64, p3 types.TipSetKey) ([]api.Partition, error) {
	return s.Internal.StateMinerPartitions(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateMinerPower(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*api.MinerPower, error) {
	return s.Internal.StateMinerPower(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerPreCommitDepositForPower(p0 context.Context, p1 address.Address, p2 miner.SectorPreCommitInfo, p3 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.StateMinerPreCommitDepositForPower(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateMinerProvingDeadline(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) {
	return s.Internal.StateMinerProvingDeadline(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerRecoveries(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (bitfield.BitField, error) {
	return s.Internal.StateMinerRecoveries(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerSectorAllocated(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (bool, error) {
	return s.Internal.StateMinerSectorAllocated(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateMinerSectorCount(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MinerSectors, error) {
	return s.Internal.StateMinerSectorCount(p0, p1, p2)
}

func (s *FullNodeStruct) StateMinerSectors(p0 context.Context, p1 address.Address, p2 *bitfield.BitField, p3 types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	return s.Internal.StateMinerSectors(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateNetworkName(p0 context.Context) (dtypes.NetworkName, error) {
	return s.Internal.StateNetworkName(p0)
}

func (s *FullNodeStruct) StateNetworkVersion(p0 context.Context, p1 types.TipSetKey) (apitypes.NetworkVersion, error) {
	return s.Internal.StateNetworkVersion(p0, p1)
}

func (s *FullNodeStruct) StateReadState(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*api.ActorState, error) {
	return s.Internal.StateReadState(p0, p1, p2)
}

func (s *FullNodeStruct) StateReplay(p0 context.Context, p1 types.TipSetKey, p2 cid.Cid) (*api.InvocResult, error) {
	return s.Internal.StateReplay(p0, p1, p2)
}

func (s *FullNodeStruct) StateSearchMsg(p0 context.Context, p1 cid.Cid) (*api.MsgLookup, error) {
	return s.Internal.StateSearchMsg(p0, p1)
}

func (s *FullNodeStruct) StateSearchMsgLimited(p0 context.Context, p1 cid.Cid, p2 abi.ChainEpoch) (*api.MsgLookup, error) {
	return s.Internal.StateSearchMsgLimited(p0, p1, p2)
}

func (s *FullNodeStruct) StateSectorExpiration(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorExpiration, error) {
	return s.Internal.StateSectorExpiration(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateSectorGetInfo(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	return s.Internal.StateSectorGetInfo(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateSectorPartition(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorLocation, error) {
	return s.Internal.StateSectorPartition(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateSectorPreCommitInfo(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) {
	return s.Internal.StateSectorPreCommitInfo(p0, p1, p2, p3)
}

func (s *FullNodeStruct) StateVMCirculatingSupplyInternal(p0 context.Context, p1 types.TipSetKey) (api.CirculatingSupply, error) {
	return s.Internal.StateVMCirculatingSupplyInternal(p0, p1)
}

func (s *FullNodeStruct) StateVerifiedClientStatus(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*abi.StoragePower, error) {
	return s.Internal.StateVerifiedClientStatus(p0, p1, p2)
}

func (s *FullNodeStruct) StateVerifiedRegistryRootKey(p0 context.Context, p1 types.TipSetKey) (address.Address, error) {
	return s.Internal.StateVerifiedRegistryRootKey(p0, p1)
}

func (s *FullNodeStruct) StateVerifierStatus(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*abi.StoragePower, error) {
	return s.Internal.StateVerifierStatus(p0, p1, p2)
}

func (s *FullNodeStruct) StateWaitMsg(p0 context.Context, p1 cid.Cid, p2 uint64) (*api.MsgLookup, error) {
	return s.Internal.StateWaitMsg(p0, p1, p2)
}

func (s *FullNodeStruct) StateWaitMsgLimited(p0 context.Context, p1 cid.Cid, p2 uint64, p3 abi.ChainEpoch) (*api.MsgLookup, error) {
	return s.Internal.StateWaitMsgLimited(p0, p1, p2, p3)
}

func (s *FullNodeStruct) SyncCheckBad(p0 context.Context, p1 cid.Cid) (string, error) {
	return s.Internal.SyncCheckBad(p0, p1)
}

func (s *FullNodeStruct) SyncCheckpoint(p0 context.Context, p1 types.TipSetKey) error {
	return s.Internal.SyncCheckpoint(p0, p1)
}

func (s *FullNodeStruct) SyncIncomingBlocks(p0 context.Context) (<-chan *types.BlockHeader, error) {
	return s.Internal.SyncIncomingBlocks(p0)
}

func (s *FullNodeStruct) SyncMarkBad(p0 context.Context, p1 cid.Cid) error {
	return s.Internal.SyncMarkBad(p0, p1)
}

func (s *FullNodeStruct) SyncState(p0 context.Context) (*api.SyncState, error) {
	return s.Internal.SyncState(p0)
}

func (s *FullNodeStruct) SyncSubmitBlock(p0 context.Context, p1 *types.BlockMsg) error {
	return s.Internal.SyncSubmitBlock(p0, p1)
}

func (s *FullNodeStruct) SyncUnmarkAllBad(p0 context.Context) error {
	return s.Internal.SyncUnmarkAllBad(p0)
}

func (s *FullNodeStruct) SyncUnmarkBad(p0 context.Context, p1 cid.Cid) error {
	return s.Internal.SyncUnmarkBad(p0, p1)
}

func (s *FullNodeStruct) SyncValidateTipset(p0 context.Context, p1 types.TipSetKey) (bool, error) {
	return s.Internal.SyncValidateTipset(p0, p1)
}

func (s *FullNodeStruct) WalletBalance(p0 context.Context, p1 address.Address) (types.BigInt, error) {
	return s.Internal.WalletBalance(p0, p1)
}

func (s *FullNodeStruct) WalletDefaultAddress(p0 context.Context) (address.Address, error) {
	return s.Internal.WalletDefaultAddress(p0)
}

func (s *FullNodeStruct) WalletDelete(p0 context.Context, p1 address.Address) error {
	return s.Internal.WalletDelete(p0, p1)
}

func (s *FullNodeStruct) WalletExport(p0 context.Context, p1 address.Address) (*types.KeyInfo, error) {
	return s.Internal.WalletExport(p0, p1)
}

func (s *FullNodeStruct) WalletHas(p0 context.Context, p1 address.Address) (bool, error) {
	return s.Internal.WalletHas(p0, p1)
}

func (s *FullNodeStruct) WalletImport(p0 context.Context, p1 *types.KeyInfo) (address.Address, error) {
	return s.Internal.WalletImport(p0, p1)
}

func (s *FullNodeStruct) WalletList(p0 context.Context) ([]address.Address, error) {
	return s.Internal.WalletList(p0)
}

func (s *FullNodeStruct) WalletNew(p0 context.Context, p1 types.KeyType) (address.Address, error) {
	return s.Internal.WalletNew(p0, p1)
}

func (s *FullNodeStruct) WalletSetDefault(p0 context.Context, p1 address.Address) error {
	return s.Internal.WalletSetDefault(p0, p1)
}

func (s *FullNodeStruct) WalletSign(p0 context.Context, p1 address.Address, p2 []byte) (*crypto.Signature, error) {
	return s.Internal.WalletSign(p0, p1, p2)
}

func (s *FullNodeStruct) WalletSignMessage(p0 context.Context, p1 address.Address, p2 *types.Message) (*types.SignedMessage, error) {
	return s.Internal.WalletSignMessage(p0, p1, p2)
}

func (s *FullNodeStruct) WalletValidateAddress(p0 context.Context, p1 string) (address.Address, error) {
	return s.Internal.WalletValidateAddress(p0, p1)
}

func (s *FullNodeStruct) WalletVerify(p0 context.Context, p1 address.Address, p2 []byte, p3 *crypto.Signature) (bool, error) {
	return s.Internal.WalletVerify(p0, p1, p2, p3)
}

func (s *GatewayAPIStruct) ChainGetBlockMessages(p0 context.Context, p1 cid.Cid) (*api.BlockMessages, error) {
	return s.Internal.ChainGetBlockMessages(p0, p1)
}

func (s *GatewayAPIStruct) ChainGetMessage(p0 context.Context, p1 cid.Cid) (*types.Message, error) {
	return s.Internal.ChainGetMessage(p0, p1)
}

func (s *GatewayAPIStruct) ChainGetTipSet(p0 context.Context, p1 types.TipSetKey) (*types.TipSet, error) {
	return s.Internal.ChainGetTipSet(p0, p1)
}

func (s *GatewayAPIStruct) ChainGetTipSetByHeight(p0 context.Context, p1 abi.ChainEpoch, p2 types.TipSetKey) (*types.TipSet, error) {
	return s.Internal.ChainGetTipSetByHeight(p0, p1, p2)
}

func (s *GatewayAPIStruct) ChainHasObj(p0 context.Context, p1 cid.Cid) (bool, error) {
	return s.Internal.ChainHasObj(p0, p1)
}

func (s *GatewayAPIStruct) ChainHead(p0 context.Context) (*types.TipSet, error) {
	return s.Internal.ChainHead(p0)
}

func (s *GatewayAPIStruct) ChainNotify(p0 context.Context) (<-chan []*api.HeadChange, error) {
	return s.Internal.ChainNotify(p0)
}

func (s *GatewayAPIStruct) ChainReadObj(p0 context.Context, p1 cid.Cid) ([]byte, error) {
	return s.Internal.ChainReadObj(p0, p1)
}

func (s *GatewayAPIStruct) GasEstimateMessageGas(p0 context.Context, p1 *types.Message, p2 *api.MessageSendSpec, p3 types.TipSetKey) (*types.Message, error) {
	return s.Internal.GasEstimateMessageGas(p0, p1, p2, p3)
}

func (s *GatewayAPIStruct) MpoolPush(p0 context.Context, p1 *types.SignedMessage) (cid.Cid, error) {
	return s.Internal.MpoolPush(p0, p1)
}

func (s *GatewayAPIStruct) MsigGetAvailableBalance(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.MsigGetAvailableBalance(p0, p1, p2)
}

func (s *GatewayAPIStruct) MsigGetPending(p0 context.Context, p1 address.Address, p2 types.TipSetKey) ([]*api.MsigTransaction, error) {
	return s.Internal.MsigGetPending(p0, p1, p2)
}

func (s *GatewayAPIStruct) MsigGetVested(p0 context.Context, p1 address.Address, p2 types.TipSetKey, p3 types.TipSetKey) (types.BigInt, error) {
	return s.Internal.MsigGetVested(p0, p1, p2, p3)
}

func (s *GatewayAPIStruct) StateAccountKey(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) {
	return s.Internal.StateAccountKey(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateDealProviderCollateralBounds(p0 context.Context, p1 abi.PaddedPieceSize, p2 bool, p3 types.TipSetKey) (api.DealCollateralBounds, error) {
	return s.Internal.StateDealProviderCollateralBounds(p0, p1, p2, p3)
}

func (s *GatewayAPIStruct) StateGetActor(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*types.Actor, error) {
	return s.Internal.StateGetActor(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateGetReceipt(p0 context.Context, p1 cid.Cid, p2 types.TipSetKey) (*types.MessageReceipt, error) {
	return s.Internal.StateGetReceipt(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateListMiners(p0 context.Context, p1 types.TipSetKey) ([]address.Address, error) {
	return s.Internal.StateListMiners(p0, p1)
}

func (s *GatewayAPIStruct) StateLookupID(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (address.Address, error) {
	return s.Internal.StateLookupID(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateMarketBalance(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (api.MarketBalance, error) {
	return s.Internal.StateMarketBalance(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateMarketStorageDeal(p0 context.Context, p1 abi.DealID, p2 types.TipSetKey) (*api.MarketDeal, error) {
	return s.Internal.StateMarketStorageDeal(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateMinerInfo(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (miner.MinerInfo, error) {
	return s.Internal.StateMinerInfo(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateMinerPower(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*api.MinerPower, error) {
	return s.Internal.StateMinerPower(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateMinerProvingDeadline(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) {
	return s.Internal.StateMinerProvingDeadline(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateNetworkVersion(p0 context.Context, p1 types.TipSetKey) (apitypes.NetworkVersion, error) {
	return s.Internal.StateNetworkVersion(p0, p1)
}

func (s *GatewayAPIStruct) StateSearchMsg(p0 context.Context, p1 cid.Cid) (*api.MsgLookup, error) {
	return s.Internal.StateSearchMsg(p0, p1)
}

func (s *GatewayAPIStruct) StateSectorGetInfo(p0 context.Context, p1 address.Address, p2 abi.SectorNumber, p3 types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	return s.Internal.StateSectorGetInfo(p0, p1, p2, p3)
}

func (s *GatewayAPIStruct) StateVerifiedClientStatus(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*abi.StoragePower, error) {
	return s.Internal.StateVerifiedClientStatus(p0, p1, p2)
}

func (s *GatewayAPIStruct) StateWaitMsg(p0 context.Context, p1 cid.Cid, p2 uint64) (*api.MsgLookup, error) {
	return s.Internal.StateWaitMsg(p0, p1, p2)
}

func (s *SignableStruct) Sign(p0 context.Context, p1 api.SignFunc) error {
	return s.Internal.Sign(p0, p1)
}

func (s *StorageMinerStruct) ActorAddress(p0 context.Context) (address.Address, error) {
	return s.Internal.ActorAddress(p0)
}

func (s *StorageMinerStruct) ActorAddressConfig(p0 context.Context) (api.AddressConfig, error) {
	return s.Internal.ActorAddressConfig(p0)
}

func (s *StorageMinerStruct) ActorSectorSize(p0 context.Context, p1 address.Address) (abi.SectorSize, error) {
	return s.Internal.ActorSectorSize(p0, p1)
}

func (s *StorageMinerStruct) CheckProvable(p0 context.Context, p1 abi.RegisteredPoStProof, p2 []storage.SectorRef, p3 bool) (map[abi.SectorNumber]string, error) {
	return s.Internal.CheckProvable(p0, p1, p2, p3)
}

func (s *StorageMinerStruct) CreateBackup(p0 context.Context, p1 string) error {
	return s.Internal.CreateBackup(p0, p1)
}

func (s *StorageMinerStruct) DealsConsiderOfflineRetrievalDeals(p0 context.Context) (bool, error) {
	return s.Internal.DealsConsiderOfflineRetrievalDeals(p0)
}

func (s *StorageMinerStruct) DealsConsiderOfflineStorageDeals(p0 context.Context) (bool, error) {
	return s.Internal.DealsConsiderOfflineStorageDeals(p0)
}

func (s *StorageMinerStruct) DealsConsiderOnlineRetrievalDeals(p0 context.Context) (bool, error) {
	return s.Internal.DealsConsiderOnlineRetrievalDeals(p0)
}

func (s *StorageMinerStruct) DealsConsiderOnlineStorageDeals(p0 context.Context) (bool, error) {
	return s.Internal.DealsConsiderOnlineStorageDeals(p0)
}

func (s *StorageMinerStruct) DealsConsiderUnverifiedStorageDeals(p0 context.Context) (bool, error) {
	return s.Internal.DealsConsiderUnverifiedStorageDeals(p0)
}

func (s *StorageMinerStruct) DealsConsiderVerifiedStorageDeals(p0 context.Context) (bool, error) {
	return s.Internal.DealsConsiderVerifiedStorageDeals(p0)
}

func (s *StorageMinerStruct) DealsImportData(p0 context.Context, p1 cid.Cid, p2 string) error {
	return s.Internal.DealsImportData(p0, p1, p2)
}

func (s *StorageMinerStruct) DealsList(p0 context.Context) ([]api.MarketDeal, error) {
	return s.Internal.DealsList(p0)
}

func (s *StorageMinerStruct) DealsPieceCidBlocklist(p0 context.Context) ([]cid.Cid, error) {
	return s.Internal.DealsPieceCidBlocklist(p0)
}

func (s *StorageMinerStruct) DealsSetConsiderOfflineRetrievalDeals(p0 context.Context, p1 bool) error {
	return s.Internal.DealsSetConsiderOfflineRetrievalDeals(p0, p1)
}

func (s *StorageMinerStruct) DealsSetConsiderOfflineStorageDeals(p0 context.Context, p1 bool) error {
	return s.Internal.DealsSetConsiderOfflineStorageDeals(p0, p1)
}

func (s *StorageMinerStruct) DealsSetConsiderOnlineRetrievalDeals(p0 context.Context, p1 bool) error {
	return s.Internal.DealsSetConsiderOnlineRetrievalDeals(p0, p1)
}

func (s *StorageMinerStruct) DealsSetConsiderOnlineStorageDeals(p0 context.Context, p1 bool) error {
	return s.Internal.DealsSetConsiderOnlineStorageDeals(p0, p1)
}

func (s *StorageMinerStruct) DealsSetConsiderUnverifiedStorageDeals(p0 context.Context, p1 bool) error {
	return s.Internal.DealsSetConsiderUnverifiedStorageDeals(p0, p1)
}

func (s *StorageMinerStruct) DealsSetConsiderVerifiedStorageDeals(p0 context.Context, p1 bool) error {
	return s.Internal.DealsSetConsiderVerifiedStorageDeals(p0, p1)
}

func (s *StorageMinerStruct) DealsSetPieceCidBlocklist(p0 context.Context, p1 []cid.Cid) error {
	return s.Internal.DealsSetPieceCidBlocklist(p0, p1)
}

func (s *StorageMinerStruct) MarketCancelDataTransfer(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error {
	return s.Internal.MarketCancelDataTransfer(p0, p1, p2, p3)
}

func (s *StorageMinerStruct) MarketDataTransferUpdates(p0 context.Context) (<-chan api.DataTransferChannel, error) {
	return s.Internal.MarketDataTransferUpdates(p0)
}

func (s *StorageMinerStruct) MarketGetAsk(p0 context.Context) (*storagemarket.SignedStorageAsk, error) {
	return s.Internal.MarketGetAsk(p0)
}

func (s *StorageMinerStruct) MarketGetDealUpdates(p0 context.Context) (<-chan storagemarket.MinerDeal, error) {
	return s.Internal.MarketGetDealUpdates(p0)
}

func (s *StorageMinerStruct) MarketGetRetrievalAsk(p0 context.Context) (*retrievalmarket.Ask, error) {
	return s.Internal.MarketGetRetrievalAsk(p0)
}

func (s *StorageMinerStruct) MarketImportDealData(p0 context.Context, p1 cid.Cid, p2 string) error {
	return s.Internal.MarketImportDealData(p0, p1, p2)
}

func (s *StorageMinerStruct) MarketListDataTransfers(p0 context.Context) ([]api.DataTransferChannel, error) {
	return s.Internal.MarketListDataTransfers(p0)
}

func (s *StorageMinerStruct) MarketListDeals(p0 context.Context) ([]api.MarketDeal, error) {
	return s.Internal.MarketListDeals(p0)
}

func (s *StorageMinerStruct) MarketListIncompleteDeals(p0 context.Context) ([]storagemarket.MinerDeal, error) {
	return s.Internal.MarketListIncompleteDeals(p0)
}

func (s *StorageMinerStruct) MarketListRetrievalDeals(p0 context.Context) ([]retrievalmarket.ProviderDealState, error) {
	return s.Internal.MarketListRetrievalDeals(p0)
}

func (s *StorageMinerStruct) MarketPendingDeals(p0 context.Context) (api.PendingDealInfo, error) {
	return s.Internal.MarketPendingDeals(p0)
}

func (s *StorageMinerStruct) MarketPublishPendingDeals(p0 context.Context) error {
	return s.Internal.MarketPublishPendingDeals(p0)
}

func (s *StorageMinerStruct) MarketRestartDataTransfer(p0 context.Context, p1 datatransfer.TransferID, p2 peer.ID, p3 bool) error {
	return s.Internal.MarketRestartDataTransfer(p0, p1, p2, p3)
}

func (s *StorageMinerStruct) MarketSetAsk(p0 context.Context, p1 types.BigInt, p2 types.BigInt, p3 abi.ChainEpoch, p4 abi.PaddedPieceSize, p5 abi.PaddedPieceSize) error {
	return s.Internal.MarketSetAsk(p0, p1, p2, p3, p4, p5)
}

func (s *StorageMinerStruct) MarketSetRetrievalAsk(p0 context.Context, p1 *retrievalmarket.Ask) error {
	return s.Internal.MarketSetRetrievalAsk(p0, p1)
}

func (s *StorageMinerStruct) MiningBase(p0 context.Context) (*types.TipSet, error) {
	return s.Internal.MiningBase(p0)
}

func (s *StorageMinerStruct) PiecesGetCIDInfo(p0 context.Context, p1 cid.Cid) (*piecestore.CIDInfo, error) {
	return s.Internal.PiecesGetCIDInfo(p0, p1)
}

func (s *StorageMinerStruct) PiecesGetPieceInfo(p0 context.Context, p1 cid.Cid) (*piecestore.PieceInfo, error) {
	return s.Internal.PiecesGetPieceInfo(p0, p1)
}

func (s *StorageMinerStruct) PiecesListCidInfos(p0 context.Context) ([]cid.Cid, error) {
	return s.Internal.PiecesListCidInfos(p0)
}

func (s *StorageMinerStruct) PiecesListPieces(p0 context.Context) ([]cid.Cid, error) {
	return s.Internal.PiecesListPieces(p0)
}

func (s *StorageMinerStruct) PledgeSector(p0 context.Context) (abi.SectorID, error) {
	return s.Internal.PledgeSector(p0)
}

func (s *StorageMinerStruct) SealingAbort(p0 context.Context, p1 storiface.CallID) error {
	return s.Internal.SealingAbort(p0, p1)
}

func (s *StorageMinerStruct) SealingSchedDiag(p0 context.Context, p1 bool) (struct{}, error) {
	return s.Internal.SealingSchedDiag(p0, p1)
}

func (s *StorageMinerStruct) SectorGetExpectedSealDuration(p0 context.Context) (time.Duration, error) {
	return s.Internal.SectorGetExpectedSealDuration(p0)
}

func (s *StorageMinerStruct) SectorGetSealDelay(p0 context.Context) (time.Duration, error) {
	return s.Internal.SectorGetSealDelay(p0)
}

func (s *StorageMinerStruct) SectorMarkForUpgrade(p0 context.Context, p1 abi.SectorNumber) error {
	return s.Internal.SectorMarkForUpgrade(p0, p1)
}

func (s *StorageMinerStruct) SectorRemove(p0 context.Context, p1 abi.SectorNumber) error {
	return s.Internal.SectorRemove(p0, p1)
}

func (s *StorageMinerStruct) SectorSetExpectedSealDuration(p0 context.Context, p1 time.Duration) error {
	return s.Internal.SectorSetExpectedSealDuration(p0, p1)
}

func (s *StorageMinerStruct) SectorSetSealDelay(p0 context.Context, p1 time.Duration) error {
	return s.Internal.SectorSetSealDelay(p0, p1)
}

func (s *StorageMinerStruct) SectorStartSealing(p0 context.Context, p1 abi.SectorNumber) error {
	return s.Internal.SectorStartSealing(p0, p1)
}

func (s *StorageMinerStruct) SectorTerminate(p0 context.Context, p1 abi.SectorNumber) error {
	return s.Internal.SectorTerminate(p0, p1)
}

func (s *StorageMinerStruct) SectorTerminateFlush(p0 context.Context) (*cid.Cid, error) {
	return s.Internal.SectorTerminateFlush(p0)
}

func (s *StorageMinerStruct) SectorTerminatePending(p0 context.Context) ([]abi.SectorID, error) {
	return s.Internal.SectorTerminatePending(p0)
}

func (s *StorageMinerStruct) SectorsList(p0 context.Context) ([]abi.SectorNumber, error) {
	return s.Internal.SectorsList(p0)
}

func (s *StorageMinerStruct) SectorsListInStates(p0 context.Context, p1 []api.SectorState) ([]abi.SectorNumber, error) {
	return s.Internal.SectorsListInStates(p0, p1)
}

func (s *StorageMinerStruct) SectorsRefs(p0 context.Context) (map[string][]api.SealedRef, error) {
	return s.Internal.SectorsRefs(p0)
}

func (s *StorageMinerStruct) SectorsStatus(p0 context.Context, p1 abi.SectorNumber, p2 bool) (api.SectorInfo, error) {
	return s.Internal.SectorsStatus(p0, p1, p2)
}

func (s *StorageMinerStruct) SectorsSummary(p0 context.Context) (map[api.SectorState]int, error) {
	return s.Internal.SectorsSummary(p0)
}

func (s *StorageMinerStruct) SectorsUpdate(p0 context.Context, p1 abi.SectorNumber, p2 api.SectorState) error {
	return s.Internal.SectorsUpdate(p0, p1, p2)
}

func (s *StorageMinerStruct) StorageAddLocal(p0 context.Context, p1 string) error {
	return s.Internal.StorageAddLocal(p0, p1)
}

func (s *StorageMinerStruct) StorageList(p0 context.Context) (map[stores.ID][]stores.Decl, error) {
	return s.Internal.StorageList(p0)
}

func (s *StorageMinerStruct) StorageLocal(p0 context.Context) (map[stores.ID]string, error) {
	return s.Internal.StorageLocal(p0)
}

func (s *StorageMinerStruct) StorageStat(p0 context.Context, p1 stores.ID) (fsutil.FsStat, error) {
	return s.Internal.StorageStat(p0, p1)
}

func (s *StorageMinerStruct) WorkerConnect(p0 context.Context, p1 string) error {
	return s.Internal.WorkerConnect(p0, p1)
}

func (s *StorageMinerStruct) WorkerJobs(p0 context.Context) (map[uuid.UUID][]storiface.WorkerJob, error) {
	return s.Internal.WorkerJobs(p0)
}

func (s *StorageMinerStruct) WorkerStats(p0 context.Context) (map[uuid.UUID]storiface.WorkerStats, error) {
	return s.Internal.WorkerStats(p0)
}

func (s *WalletAPIStruct) WalletDelete(p0 context.Context, p1 address.Address) error {
	return s.Internal.WalletDelete(p0, p1)
}

func (s *WalletAPIStruct) WalletExport(p0 context.Context, p1 address.Address) (*types.KeyInfo, error) {
	return s.Internal.WalletExport(p0, p1)
}

func (s *WalletAPIStruct) WalletHas(p0 context.Context, p1 address.Address) (bool, error) {
	return s.Internal.WalletHas(p0, p1)
}

func (s *WalletAPIStruct) WalletImport(p0 context.Context, p1 *types.KeyInfo) (address.Address, error) {
	return s.Internal.WalletImport(p0, p1)
}

func (s *WalletAPIStruct) WalletList(p0 context.Context) ([]address.Address, error) {
	return s.Internal.WalletList(p0)
}

func (s *WalletAPIStruct) WalletNew(p0 context.Context, p1 types.KeyType) (address.Address, error) {
	return s.Internal.WalletNew(p0, p1)
}

func (s *WalletAPIStruct) WalletSign(p0 context.Context, p1 address.Address, p2 []byte, p3 api.MsgMeta) (*crypto.Signature, error) {
	return s.Internal.WalletSign(p0, p1, p2, p3)
}

func (s *WorkerAPIStruct) Enabled(p0 context.Context) (bool, error) {
	return s.Internal.Enabled(p0)
}

func (s *WorkerAPIStruct) Info(p0 context.Context) (storiface.WorkerInfo, error) {
	return s.Internal.Info(p0)
}

func (s *WorkerAPIStruct) Paths(p0 context.Context) ([]stores.StoragePath, error) {
	return s.Internal.Paths(p0)
}

func (s *WorkerAPIStruct) ProcessSession(p0 context.Context) (uuid.UUID, error) {
	return s.Internal.ProcessSession(p0)
}

func (s *WorkerAPIStruct) Remove(p0 context.Context, p1 abi.SectorID) error {
	return s.Internal.Remove(p0, p1)
}

func (s *WorkerAPIStruct) Session(p0 context.Context) (uuid.UUID, error) {
	return s.Internal.Session(p0)
}

func (s *WorkerAPIStruct) SetEnabled(p0 context.Context, p1 bool) error {
	return s.Internal.SetEnabled(p0, p1)
}

func (s *WorkerAPIStruct) StorageAddLocal(p0 context.Context, p1 string) error {
	return s.Internal.StorageAddLocal(p0, p1)
}

func (s *WorkerAPIStruct) TaskDisable(p0 context.Context, p1 sealtasks.TaskType) error {
	return s.Internal.TaskDisable(p0, p1)
}

func (s *WorkerAPIStruct) TaskEnable(p0 context.Context, p1 sealtasks.TaskType) error {
	return s.Internal.TaskEnable(p0, p1)
}

func (s *WorkerAPIStruct) TaskTypes(p0 context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return s.Internal.TaskTypes(p0)
}

func (s *WorkerAPIStruct) Version(p0 context.Context) (api.Version, error) {
	return s.Internal.Version(p0)
}

func (s *WorkerAPIStruct) WaitQuiet(p0 context.Context) error {
	return s.Internal.WaitQuiet(p0)
}

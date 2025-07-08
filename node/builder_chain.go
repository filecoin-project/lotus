package node

import (
	"os"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/exchange"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/messagesigner"
	"github.com/filecoin-project/lotus/chain/proofs"
	proofsffi "github.com/filecoin-project/lotus/chain/proofs/ffi"
	"github.com/filecoin-project/lotus/chain/stmgr"
	rpcstmgr "github.com/filecoin-project/lotus/chain/stmgr/rpc"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	ledgerwallet "github.com/filecoin-project/lotus/chain/wallet/ledger"
	"github.com/filecoin-project/lotus/chain/wallet/remotewallet"
	"github.com/filecoin-project/lotus/lib/peermgr"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/filecoin-project/lotus/node/impl/eth"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/impl/gasutils"
	"github.com/filecoin-project/lotus/node/impl/net"
	"github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/paychmgr"
	"github.com/filecoin-project/lotus/paychmgr/settler"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// Chain node provides access to the Filecoin blockchain, by setting up a full
// validator node, or by delegating some actions to other nodes (lite mode)
var ChainNode = Options(
	// Full node or lite node
	// TODO: Fix offline mode

	// Consensus settings
	Override(new(dtypes.DrandSchedule), modules.BuiltinDrandConfig),
	Override(new(stmgr.UpgradeSchedule), modules.UpgradeSchedule),
	Override(new(dtypes.NetworkName), modules.NetworkName),
	Override(new(modules.Genesis), modules.ErrorGenesis),
	Override(new(dtypes.AfterGenesisSet), modules.SetGenesis),
	Override(SetGenesisKey, modules.DoSetGenesis),
	Override(new(beacon.Schedule), modules.RandomSchedule),

	// Network bootstrap
	Override(new(dtypes.BootstrapPeers), modules.BuiltinBootstrap),
	Override(new(dtypes.DrandBootstrap), modules.DrandBootstrap),

	// Consensus: crypto dependencies
	Override(new(proofs.Verifier), ffiwrapper.ProofVerifier),
	Override(new(storiface.Verifier), proofsffi.ProofVerifier),
	Override(new(storiface.Prover), ffiwrapper.ProofProver),

	// Consensus: LegacyVM
	Override(new(vm.SyscallBuilder), vm.Syscalls),

	// Consensus: Chain storage/access
	Override(new(chain.Genesis), chain.LoadGenesis),
	Override(new(store.WeightFunc), filcns.Weight),
	Override(new(stmgr.Executor), consensus.NewTipSetExecutor(filcns.RewardFunc)),
	Override(new(consensus.Consensus), filcns.NewFilecoinExpectedConsensus),
	Override(new(*store.ChainStore), modules.ChainStore),
	Override(new(*stmgr.StateManager), modules.StateManager),
	Override(new(dtypes.ChainBitswap), modules.ChainBitswap),
	Override(new(dtypes.ChainBlockService), modules.ChainBlockService), // todo: unused

	// Consensus: Chain sync

	// We don't want the SyncManagerCtor to be used as an fx constructor, but rather as a value.
	// It will be called implicitly by the Syncer constructor.
	Override(new(chain.SyncManagerCtor), func() chain.SyncManagerCtor { return chain.NewSyncManager }),
	Override(new(*chain.Syncer), modules.NewSyncer),
	Override(new(exchange.Client), exchange.NewClient),

	// Chain networking
	Override(new(*hello.Service), hello.NewHelloService),
	Override(new(exchange.Server), exchange.NewServer),
	Override(new(*peermgr.PeerMgr), peermgr.NewPeerMgr),

	// Chain mining API dependencies
	Override(new(*slashfilter.SlashFilter), modules.NewSlashFilter),

	// Service: Message Pool
	Override(new(dtypes.DefaultMaxFeeFunc), modules.NewDefaultMaxFeeFunc),
	Override(new(*messagepool.MessagePool), modules.MessagePool),
	Override(new(*dtypes.MpoolLocker), new(dtypes.MpoolLocker)),

	// Service: Wallet
	Override(new(*messagesigner.MessageSigner), messagesigner.NewMessageSigner),
	Override(new(messagesigner.MsgSigner), func(ms *messagesigner.MessageSigner) *messagesigner.MessageSigner { return ms }),
	Override(new(*wallet.LocalWallet), wallet.NewWallet),
	Override(new(wallet.Default), From(new(*wallet.LocalWallet))),
	Override(new(api.Wallet), From(new(wallet.MultiWallet))),

	// Markets (storage)
	Override(new(*market.FundManager), market.NewFundManager),

	Override(new(*gasutils.GasPriceCache), gasutils.NewGasPriceCache),

	// Lite node API
	ApplyIf(isLiteNode,
		Override(new(messagepool.Provider), messagepool.NewProviderLite),
		Override(new(messagepool.MpoolNonceAPI), From(new(modules.MpoolNonceAPI))),
		Override(new(full.ChainModuleAPI), From(new(api.Gateway))),
		Override(new(full.ChainModuleAPIv2), From(new(v2api.Gateway))),
		Override(new(full.GasModuleAPI), From(new(api.Gateway))),
		Override(new(full.MpoolModuleAPI), From(new(api.Gateway))),
		Override(new(full.StateModuleAPI), From(new(api.Gateway))),
		Override(new(full.StateModuleAPIv2), From(new(v2api.Gateway))),
		Override(new(stmgr.StateManagerAPI), rpcstmgr.NewRPCStateManager),
		Override(new(full.ActorEventAPI), From(new(api.Gateway))),
		Override(new(full.EthFilecoinAPIV1), From(new(api.Gateway))),
		Override(new(full.EthFilecoinAPIV2), From(new(v2api.Gateway))),
		Override(new(eth.EthBasicAPI), From(new(api.Gateway))),
		Override(new(eth.EthEventsAPI), From(new(api.Gateway))),
		Override(new(full.EthLookupAPIV1), From(new(api.Gateway))),
		Override(new(full.EthLookupAPIV2), From(new(v2api.Gateway))),
		Override(new(full.EthTraceAPIV1), From(new(api.Gateway))),
		Override(new(full.EthTraceAPIV2), From(new(v2api.Gateway))),
		Override(new(full.EthGasAPIV1), From(new(api.Gateway))),
		Override(new(full.EthGasAPIV2), From(new(v2api.Gateway))),
		If(build.IsF3Enabled(), Override(new(full.F3CertificateProvider), From(new(api.Gateway)))),
		// EthSendAPI is a special case, we block the Untrusted method via GatewayEthSend even though it
		// shouldn't be exposed on the Gateway API.
		Override(new(eth.EthSendAPI), new(modules.GatewayEthSend)),
		// EthTransactionAPIV1 is a special case, we block the Limited methods via
		// GatewayEthTransaction even though it shouldn't be exposed on the Gateway API.
		Override(new(full.EthTransactionAPIV1), new(modules.GatewayEthTransaction)),
		Override(new(full.EthTransactionAPIV2), new(modules.GatewayEthTransaction)),

		Override(new(index.Indexer), modules.ChainIndexer(config.ChainIndexerConfig{
			EnableIndexer: false,
		})),
	),

	// Full node API / service startup
	ApplyIf(isFullNode,
		Override(new(messagepool.Provider), messagepool.NewProvider),
		Override(new(messagepool.MpoolNonceAPI), From(new(*messagepool.MessagePool))),
		Override(new(full.ChainModuleAPI), From(new(full.ChainModule))),
		Override(new(full.ChainModuleAPIv2), From(new(full.ChainModuleV2))),
		Override(new(full.GasModuleAPI), From(new(full.GasModule))),
		Override(new(full.MpoolModuleAPI), From(new(full.MpoolModule))),
		Override(new(full.StateModuleAPI), From(new(full.StateModule))),
		Override(new(full.StateModuleAPIv2), From(new(full.StateModuleV2))),
		Override(new(stmgr.StateManagerAPI), From(new(*stmgr.StateManager))),

		Override(RunHelloKey, modules.RunHello),
		Override(RunChainExchangeKey, modules.RunChainExchange),
		Override(RunPeerMgrKey, modules.RunPeerMgr),
		Override(HandleIncomingMessagesKey, modules.HandleIncomingMessages),
		Override(HandleIncomingBlocksKey, modules.HandleIncomingBlocks),
	),
	ApplyIf(func(s *Settings) bool {
		return build.IsF3Enabled() && !isLiteNode(s)
	},
		Override(new(dtypes.F3DS), modules.F3Datastore),
		Override(new(*lf3.Config), lf3.NewConfig),
		Override(new(lf3.F3Backend), lf3.New),
		Override(new(full.F3ModuleAPI), From(new(full.F3API))),
	),
)

func ConfigFullNode(c interface{}) Option {
	cfg, ok := c.(*config.FullNode)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
	}

	if cfg.Fevm.EnableEthRPC && !cfg.ChainIndexer.EnableIndexer {
		return Error(xerrors.New("EnableIndexer in the ChainIndexer configuration section must be set to true when setting EnableEthRPC to true"))
	}
	if cfg.Events.EnableActorEventsAPI && !cfg.ChainIndexer.EnableIndexer {
		return Error(xerrors.New("EnableIndexer in the ChainIndexer configuration section must be set to true when setting EnableActorEventsAPI to true"))
	}

	return Options(
		ConfigCommon(&cfg.Common, build.NodeUserVersion()),

		// always enable libp2p for full nodes
		Override(new(api.Net), new(api.NetStub)),
		Override(new(api.Common), From(new(common.CommonAPI))),
		Override(new(api.Net), From(new(net.NetAPI))),
		Override(new(api.Common), From(new(common.CommonAPI))),
		Override(StartListeningKey, lp2p.StartListening(cfg.Libp2p.ListenAddresses)),
		Override(ConnectionManagerKey, lp2p.ConnectionManager(
			cfg.Libp2p.ConnMgrLow,
			cfg.Libp2p.ConnMgrHigh,
			time.Duration(cfg.Libp2p.ConnMgrGrace),
			cfg.Libp2p.ProtectedPeers)),
		Override(new(network.ResourceManager), lp2p.ResourceManager(cfg.Libp2p.ConnMgrHigh)),
		Override(new(*pubsub.PubSub), lp2p.GossipSub),
		Override(new(*config.Pubsub), &cfg.Pubsub),

		ApplyIf(func(s *Settings) bool { return len(cfg.Libp2p.BootstrapPeers) > 0 },
			Override(new(dtypes.BootstrapPeers), modules.ConfigBootstrap(cfg.Libp2p.BootstrapPeers)),
		),

		Override(AddrsFactoryKey, lp2p.AddrsFactory(
			cfg.Libp2p.AnnounceAddresses,
			cfg.Libp2p.NoAnnounceAddresses)),

		If(!cfg.Libp2p.DisableNatPortMap, Override(NatPortMapKey, lp2p.NatPortMap)),

		Override(new(dtypes.UniversalBlockstore), modules.UniversalBlockstore),

		If(cfg.Chainstore.EnableSplitstore,
			If(cfg.Chainstore.Splitstore.ColdStoreType == "universal" || cfg.Chainstore.Splitstore.ColdStoreType == "messages",
				Override(new(dtypes.ColdBlockstore), From(new(dtypes.UniversalBlockstore)))),
			If(cfg.Chainstore.Splitstore.ColdStoreType == "discard",
				Override(new(dtypes.ColdBlockstore), modules.DiscardColdBlockstore)),
			If(cfg.Chainstore.Splitstore.HotStoreType == "badger",
				Override(new(dtypes.HotBlockstore), modules.BadgerHotBlockstore)),
			Override(new(dtypes.SplitBlockstore), modules.SplitBlockstore(&cfg.Chainstore)),
			Override(new(dtypes.BasicChainBlockstore), modules.ChainSplitBlockstore),
			Override(new(dtypes.BasicStateBlockstore), modules.StateSplitBlockstore),
			Override(new(dtypes.BaseBlockstore), From(new(dtypes.SplitBlockstore))),
			Override(new(dtypes.ExposedBlockstore), modules.ExposedSplitBlockstore),
			Override(new(dtypes.GCReferenceProtector), modules.SplitBlockstoreGCReferenceProtector),
		),
		If(!cfg.Chainstore.EnableSplitstore,
			Override(new(dtypes.BasicChainBlockstore), modules.ChainFlatBlockstore),
			Override(new(dtypes.BasicStateBlockstore), modules.StateFlatBlockstore),
			Override(new(dtypes.BaseBlockstore), From(new(dtypes.UniversalBlockstore))),
			Override(new(dtypes.ExposedBlockstore), From(new(dtypes.UniversalBlockstore))),
			Override(new(dtypes.GCReferenceProtector), modules.NoopGCReferenceProtector),
		),

		Override(new(dtypes.ChainBlockstore), From(new(dtypes.BasicChainBlockstore))),
		Override(new(dtypes.StateBlockstore), From(new(dtypes.BasicStateBlockstore))),

		If(os.Getenv("LOTUS_ENABLE_CHAINSTORE_FALLBACK") == "1",
			Override(new(dtypes.ChainBlockstore), modules.FallbackChainBlockstore),
			Override(new(dtypes.StateBlockstore), modules.FallbackStateBlockstore),
			Override(SetupFallbackBlockstoresKey, modules.InitFallbackBlockstores),
		),

		// If the Eth JSON-RPC is enabled, enable storing events at the ChainStore.
		// This is the case even if real-time and historic filtering are disabled,
		// as it enables us to serve logs in eth_getTransactionReceipt.
		If(cfg.Fevm.EnableEthRPC || cfg.Events.EnableActorEventsAPI || cfg.ChainIndexer.EnableIndexer, Override(StoreEventsKey, modules.EnableStoringEvents)),

		If(cfg.Wallet.RemoteBackend != "",
			Override(new(*remotewallet.RemoteWallet), remotewallet.SetupRemoteWallet(cfg.Wallet.RemoteBackend)),
		),
		If(cfg.Wallet.EnableLedger,
			Override(new(*ledgerwallet.LedgerWallet), ledgerwallet.NewWallet),
		),
		If(cfg.Wallet.DisableLocal,
			Unset(new(*wallet.LocalWallet)),
			Override(new(wallet.Default), wallet.NilDefault),
		),

		// In lite-mode Eth and events API is provided by gateway
		ApplyIf(isFullNode,
			If(cfg.Fevm.EnableEthRPC || cfg.Events.EnableActorEventsAPI,
				// Actor event filtering support, only needed for either Eth RPC and ActorEvents API
				Override(new(events.EventHelperAPI), From(new(modules.EventHelperAPI))),
				Override(new(*filter.EventFilterManager), modules.MakeEventFilterManager(cfg.Events)),
			),

			Override(new(eth.ChainStore), From(new(*store.ChainStore))),
			Override(new(eth.StateManager), From(new(*stmgr.StateManager))),
			Override(new(full.EthTipSetResolverV1), modules.MakeV1TipSetResolver),
			Override(new(full.EthTipSetResolverV2), modules.MakeV2TipSetResolver),
			Override(new(full.EthFilecoinAPIV1), modules.MakeEthFilecoinV1),
			Override(new(full.EthFilecoinAPIV2), modules.MakeEthFilecoinV2),

			If(cfg.Fevm.EnableEthRPC,
				Override(new(eth.StateAPI), From(new(full.StateAPI))),
				Override(new(eth.SyncAPI), From(new(full.SyncAPI))),
				Override(new(eth.MpoolAPI), From(new(full.MpoolAPI))),
				Override(new(eth.MessagePool), From(new(*messagepool.MessagePool))),
				Override(new(eth.GasAPI), From(new(full.GasModule))),

				Override(new(eth.EthBasicAPI), eth.NewEthBasicAPI),
				Override(new(eth.EthSendAPI), eth.NewEthSendAPI),
				Override(new(eth.EthEventsInternal), modules.MakeEthEventsExtended(cfg.Events, cfg.Fevm.EnableEthRPC)),
				Override(new(eth.EthEventsAPI), From(new(eth.EthEventsInternal))),

				Override(new(full.EthTransactionAPIV1), modules.MakeEthTransactionV1(cfg.Fevm)),
				Override(new(full.EthLookupAPIV1), modules.MakeEthLookupV1),
				Override(new(full.EthTraceAPIV1), modules.MakeEthTraceV1(cfg.Fevm)),
				Override(new(full.EthGasAPIV1), modules.MakeEthGasV1),

				Override(new(full.EthTransactionAPIV2), modules.MakeEthTransactionV2(cfg.Fevm)),
				Override(new(full.EthLookupAPIV2), modules.MakeEthLookupV2),
				Override(new(full.EthTraceAPIV2), modules.MakeEthTraceV2(cfg.Fevm)),
				Override(new(full.EthGasAPIV2), modules.MakeEthGasV2),
			),
			If(!cfg.Fevm.EnableEthRPC,
				Override(new(eth.EthBasicAPI), &eth.EthBasicDisabled{}),
				Override(new(eth.EthSendAPI), &eth.EthSendDisabled{}),
				Override(new(eth.EthEventsAPI), &eth.EthEventsDisabled{}),

				Override(new(full.EthTransactionAPIV1), &eth.EthTransactionDisabled{}),
				Override(new(full.EthLookupAPIV1), &eth.EthLookupDisabled{}),
				Override(new(full.EthTraceAPIV1), &eth.EthTraceDisabled{}),
				Override(new(full.EthGasAPIV1), &eth.EthGasDisabled{}),

				Override(new(full.EthTransactionAPIV2), &eth.EthTransactionDisabled{}),
				Override(new(full.EthLookupAPIV2), &eth.EthLookupDisabled{}),
				Override(new(full.EthTraceAPIV2), &eth.EthTraceDisabled{}),
				Override(new(full.EthGasAPIV2), &eth.EthGasDisabled{}),
			),

			If(cfg.Events.EnableActorEventsAPI,
				Override(new(full.ActorEventAPI), modules.ActorEventHandler(cfg.Events)),
			),
			If(!cfg.Events.EnableActorEventsAPI,
				Override(new(full.ActorEventAPI), &full.ActorEventDummy{}),
			),
		),

		// enable fault reporter when configured by the user
		If(cfg.FaultReporter.EnableConsensusFaultReporter,
			Override(ConsensusReporterKey, modules.RunConsensusFaultReporter(cfg.FaultReporter)),
		),

		ApplyIf(isLiteNode,
			Override(new(full.ChainIndexerAPI), func() full.ChainIndexerAPI { return nil }),
		),

		ApplyIf(isFullNode,
			Override(new(index.Indexer), modules.ChainIndexer(cfg.ChainIndexer)),
			Override(new(full.ChainIndexerAPI), modules.ChainIndexHandler(cfg.ChainIndexer)),
			If(cfg.ChainIndexer.EnableIndexer,
				Override(InitChainIndexerKey, modules.InitChainIndexer(cfg.ChainIndexer)),
			),
		),

		If(cfg.PaymentChannels.EnablePaymentChannelManager,
			Override(new(paychmgr.ManagerNodeAPI), From(new(modules.PaychManagerNodeAPI))),
			Override(new(*paychmgr.Store), modules.NewPaychStore),
			Override(new(*paychmgr.Manager), modules.NewManager),
			Override(HandlePaymentChannelManagerKey, modules.HandlePaychManager),
			Override(SettlePaymentChannelsKey, settler.SettlePaymentChannels),
			Override(new(paych.PaychAPI), From(new(paych.PaychImpl))),
		),
		If(!cfg.PaymentChannels.EnablePaymentChannelManager,
			Override(new(paych.PaychAPI), new(paych.DisabledPaych)),
		),
	)
}

type FullOption = Option

func Lite(enable bool) FullOption {
	return func(s *Settings) error {
		s.Lite = enable
		return nil
	}
}

func FullAPI(out *api.FullNode, fopts ...FullOption) Option {
	return Options(
		func(s *Settings) error {
			s.nodeType = repo.FullNode
			s.enableLibp2pNode = true
			return nil
		},
		Options(fopts...),
		func(s *Settings) error {
			resAPI := &impl.FullNodeAPI{}
			s.invokes[ExtractApiKey] = fx.Populate(resAPI)
			*out = resAPI
			return nil
		},
	)
}

func FullAPIv2(out *v2api.FullNode) Option {
	return func(s *Settings) error {
		resAPI := &impl.FullNodeAPIv2{}
		s.invokes[InitApiV2Key] = fx.Populate(resAPI)
		*out = resAPI
		return nil
	}
}

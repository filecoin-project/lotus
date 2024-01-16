package node

import (
	"os"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
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
	"github.com/filecoin-project/lotus/chain/stmgr"
	rpcstmgr "github.com/filecoin-project/lotus/chain/stmgr/rpc"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/verifier"
	verifierffi "github.com/filecoin-project/lotus/chain/verifier/ffi"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	ledgerwallet "github.com/filecoin-project/lotus/chain/wallet/ledger"
	"github.com/filecoin-project/lotus/chain/wallet/remotewallet"
	"github.com/filecoin-project/lotus/lib/peermgr"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/impl/net"
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
	Override(new(verifier.Verifier), ffiwrapper.ProofVerifier),
	Override(new(storiface.Verifier), verifierffi.ProofVerifier),
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

	// Service: Payment channels
	Override(new(paychmgr.PaychAPI), From(new(modules.PaychAPI))),
	Override(new(*paychmgr.Store), modules.NewPaychStore),
	Override(new(*paychmgr.Manager), modules.NewManager),
	Override(HandlePaymentChannelManagerKey, modules.HandlePaychManager),
	Override(SettlePaymentChannelsKey, settler.SettlePaymentChannels),

	// Markets (storage)
	Override(new(*market.FundManager), market.NewFundManager),

	Override(new(*full.GasPriceCache), full.NewGasPriceCache),

	Override(RelayIndexerMessagesKey, modules.RelayIndexerMessages),

	// Lite node API
	ApplyIf(isLiteNode,
		Override(new(messagepool.Provider), messagepool.NewProviderLite),
		Override(new(messagepool.MpoolNonceAPI), From(new(modules.MpoolNonceAPI))),
		Override(new(full.ChainModuleAPI), From(new(api.Gateway))),
		Override(new(full.GasModuleAPI), From(new(api.Gateway))),
		Override(new(full.MpoolModuleAPI), From(new(api.Gateway))),
		Override(new(full.StateModuleAPI), From(new(api.Gateway))),
		Override(new(stmgr.StateManagerAPI), rpcstmgr.NewRPCStateManager),
		Override(new(full.EthModuleAPI), From(new(api.Gateway))),
		Override(new(full.EthEventAPI), From(new(api.Gateway))),
		Override(new(full.ActorEventAPI), From(new(api.Gateway))),
	),

	// Full node API / service startup
	ApplyIf(isFullNode,
		Override(new(messagepool.Provider), messagepool.NewProvider),
		Override(new(messagepool.MpoolNonceAPI), From(new(*messagepool.MessagePool))),
		Override(new(full.ChainModuleAPI), From(new(full.ChainModule))),
		Override(new(full.GasModuleAPI), From(new(full.GasModule))),
		Override(new(full.MpoolModuleAPI), From(new(full.MpoolModule))),
		Override(new(full.StateModuleAPI), From(new(full.StateModule))),
		Override(new(stmgr.StateManagerAPI), From(new(*stmgr.StateManager))),

		Override(RunHelloKey, modules.RunHello),
		Override(RunChainExchangeKey, modules.RunChainExchange),
		Override(RunPeerMgrKey, modules.RunPeerMgr),
		Override(HandleIncomingMessagesKey, modules.HandleIncomingMessages),
		Override(HandleIncomingBlocksKey, modules.HandleIncomingBlocks),
	),

	If(build.IsF3Enabled(),
		Override(new(manifest.ManifestProvider), lf3.NewManifestProvider),
		Override(new(*lf3.F3), lf3.New),
	),
)

func ConfigFullNode(c interface{}) Option {
	cfg, ok := c.(*config.FullNode)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
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
		If(cfg.Fevm.EnableEthRPC || cfg.Events.EnableActorEventsAPI, Override(StoreEventsKey, modules.EnableStoringEvents)),

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
				Override(new(*filter.EventFilterManager), modules.EventFilterManager(cfg.Events)),
			),

			If(cfg.Fevm.EnableEthRPC,
				Override(new(*full.EthEventHandler), modules.EthEventHandler(cfg.Events, cfg.Fevm.EnableEthRPC)),
				Override(new(full.EthModuleAPI), modules.EthModuleAPI(cfg.Fevm)),
				Override(new(full.EthEventAPI), From(new(*full.EthEventHandler))),
			),
			If(!cfg.Fevm.EnableEthRPC,
				Override(new(full.EthModuleAPI), &full.EthModuleDummy{}),
				Override(new(full.EthEventAPI), &full.EthModuleDummy{}),
			),

			If(cfg.Events.EnableActorEventsAPI,
				Override(new(full.ActorEventAPI), modules.ActorEventHandler(cfg.Events)),
			),
			If(!cfg.Events.EnableActorEventsAPI,
				Override(new(full.ActorEventAPI), &full.ActorEventDummy{}),
			),
		),

		// enable message index for full node when configured by the user, otherwise use dummy.
		If(cfg.Index.EnableMsgIndex, Override(new(index.MsgIndex), modules.MsgIndex)),
		If(!cfg.Index.EnableMsgIndex, Override(new(index.MsgIndex), modules.DummyMsgIndex)),

		// enable fault reporter when configured by the user
		If(cfg.FaultReporter.EnableConsensusFaultReporter,
			Override(ConsensusReporterKey, modules.RunConsensusFaultReporter(cfg.FaultReporter)),
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

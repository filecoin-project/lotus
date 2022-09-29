// Package mir implements ISS consensus protocol using the Mir protocol framework.
package mir

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/consensus/mir/mempool"
	"github.com/filecoin-project/lotus/chain/consensus/mir/mempool/fifo"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/mir"
	"github.com/filecoin-project/mir/pkg/availability/batchdb/fakebatchdb"
	"github.com/filecoin-project/mir/pkg/availability/multisigcollector"
	mircrypto "github.com/filecoin-project/mir/pkg/crypto"
	"github.com/filecoin-project/mir/pkg/eventlog"
	"github.com/filecoin-project/mir/pkg/events"
	"github.com/filecoin-project/mir/pkg/iss"
	"github.com/filecoin-project/mir/pkg/logging"
	"github.com/filecoin-project/mir/pkg/modules"
	"github.com/filecoin-project/mir/pkg/net"
	mirlibp2p "github.com/filecoin-project/mir/pkg/net/libp2p"
	mirproto "github.com/filecoin-project/mir/pkg/pb/requestpb"
	"github.com/filecoin-project/mir/pkg/simplewal"
	t "github.com/filecoin-project/mir/pkg/types"
)

const (
	ValidatorsEnv        = "EUDICO_MIR_VALIDATORS"
	InterceptorOutputEnv = "EUDICO_INTERCEPTOR_OUTPUT"
)

// Manager manages the Eudico and Mir nodes participating in ISS consensus protocol.
type Manager struct {
	// Eudico related types.
	NetName dtypes.NetworkName
	Addr    address.Address
	Pool    *fifo.Pool

	// Mir related types.
	MirNode        *mir.Node
	MirID          string
	WAL            *simplewal.WAL
	Net            net.Transport
	ISS            *iss.ISS
	CryptoManager  *CryptoManager
	StateManager   *StateManager
	interceptor    *eventlog.Recorder
	CurrentMempool chan mempool.Descriptor

	// Reconfiguration related types.
	InitialValidatorSet  *ValidatorSet
	reconfigurationNonce uint64
}

func NewManager(ctx context.Context, addr address.Address, api v1api.FullNode) (*Manager, error) {
	netName, err := api.StateNetworkName(ctx)
	if err != nil {
		return nil, err
	}

	// subnetID, err := address.SubnetIDFromString(string(netName))
	// if err != nil {
	// 	return nil, err
	// }

	// libp2pKeyBytes, err := api.PrivKey(ctx)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get private key: %w", err)
	// }

	// libp2pKey, err := crypto.UnmarshalPrivateKey(libp2pKeyBytes)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to unmarshal private key: %w", err)
	// }

	// initialValidatorSet, err := getSubnetValidators(ctx, subnetID, api)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get validator set: %w", err)
	// }
	// if initialValidatorSet.Size() == 0 {
	// 	return nil, fmt.Errorf("empty validator set")
	// }

	// nodeIDs, initialMembership, err := validatorsMembership(initialValidatorSet.Validators)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to build node membership: %w", err)
	// }

	// memberships := make([]map[t.NodeID]t.NodeAddress, ConfigOffset+1)
	// for i := 0; i < ConfigOffset+1; i++ {
	// 	memberships[t.EpochNr(i)] = initialMembership
	// }

	// mirID := newMirID(subnetID.String(), addr.String())
	// mirAddr := initialMembership[t.NodeID(mirID)]

	// log.Info("Eudico node's Mir ID: ", mirID)
	// log.Info("Eudico node's address in Mir: ", mirAddr)
	// log.Info("Mir nodes IDs: ", nodeIDs)
	// log.Info("Mir nodes addresses: ", initialMembership)

	peerID, err := peer.AddrInfoFromP2pAddr(mirAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get addr info: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(libp2pKey),
		libp2p.DefaultTransports,
		libp2p.ListenAddrs(peerID.Addrs[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Logger.

	logger := newManagerLogger()

	// Create Mir modules.
	path := fmt.Sprintf("eudico-wal%s", strings.Replace(mirID, "/", "-", -1))
	wal, err := NewWAL(mirID, path)
	if err != nil {
		return nil, fmt.Errorf("failed to create WAL: %w", err)
	}

	netTransport, err := mirlibp2p.NewTransport(h, t.NodeID(mirID), logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}
	if err := netTransport.Start(); err != nil {
		return nil, fmt.Errorf("failed to start transport: %w", err)
	}
	netTransport.Connect(ctx, initialMembership)

	cryptoManager, err := NewCryptoManager(addr, api)
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto manager: %w", err)
	}

	// Instantiate an interceptor.

	var interceptor *eventlog.Recorder

	interceptorOutput := os.Getenv(InterceptorOutputEnv)
	if interceptorOutput != "" {
		path := fmt.Sprintf("eudico-recorder%s", strings.Replace(mirID, "/", "-", -1))
		interceptor, err = eventlog.NewRecorder(t.NodeID(mirID), path, logging.Decorate(logger, "Interceptor: "))
		if err != nil {
			return nil, fmt.Errorf("failed to create interceptor: %w", err)
		}
	}

	// Instantiate the ISS protocol module with default configuration.

	issConfig := iss.DefaultConfig(initialMembership)
	issConfig.ConfigOffset = ConfigOffset
	issProtocol, err := iss.New(
		t.NodeID(mirID),
		iss.DefaultModuleConfig(),
		issConfig,
		iss.InitialStateSnapshot([]byte{}, issConfig),
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("could not instantiate ISS protocol module: %w", err)
	}

	// Use fake batch database.
	batchdb := fakebatchdb.NewModule(
		&fakebatchdb.ModuleConfig{
			Self: "batchdb",
		},
	)

	m := Manager{
		Addr:                addr,
		SubnetID:            subnetID,
		NetName:             netName,
		Pool:                fifo.New(),
		MirID:               mirID,
		interceptor:         interceptor,
		WAL:                 wal,
		CryptoManager:       cryptoManager,
		Net:                 netTransport,
		ISS:                 issProtocol,
		InitialValidatorSet: initialValidatorSet,
		CurrentMempool:      make(chan mempool.Descriptor),
	}

	sm := NewStateManager(initialMembership, &m)

	// Use a mempool for incoming requests.
	mpool := mempool.NewModule(
		m.CurrentMempool,
		&mempool.ModuleConfig{
			Self:   "mempool",
			Hasher: "hasher",
		},
		&mempool.ModuleParams{
			MaxTransactionsInBatch: 10,
		},
	)

	availability := multisigcollector.NewReconfigurableModule(
		&multisigcollector.ModuleConfig{
			Self:    "availability",
			Mempool: "mempool",
			BatchDB: "batchdb",
			Net:     "net",
			Crypto:  "crypto",
		},
		t.NodeID(mirID),
		logger,
	)

	// Create a Mir node, using the default configuration and passing the modules initialized just above.
	mirModules, err := iss.DefaultModules(map[t.ModuleID]modules.Module{
		"net":          netTransport,
		"iss":          issProtocol,
		"app":          sm,
		"crypto":       mircrypto.New(m.CryptoManager),
		"mempool":      mpool,
		"batchdb":      batchdb,
		"availability": availability,
	}, iss.DefaultModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Mir modules: %w", err)
	}

	cfg := mir.NodeConfig{Logger: logger}
	newMirNode, err := mir.NewNode(t.NodeID(mirID), &cfg, mirModules, wal, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mir node: %w", err)
	}

	m.MirNode = newMirNode
	m.StateManager = sm

	return &m, nil
}

// Start starts the manager.
func (m *Manager) Start(ctx context.Context) chan error {
	log.Infof("Mir manager %s starting", m.MirID)

	errChan := make(chan error, 1)

	go func() {
		// Run Mir node until it stops.
		if err := m.MirNode.Run(ctx); err != nil && !errors.Is(err, mir.ErrStopped) {
			log.Infof("Mir manager %s: Mir node stopped with error: %v", m.MirID, err)
			errChan <- err
		}

		// Perform cleanup of Node's modules.
		m.Stop()
	}()

	return errChan
}

// Stop stops the manager and all its components.
func (m *Manager) Stop() {
	log.With("miner", m.MirID).Infof("Mir manager shutting down")
	defer log.With("miner", m.MirID).Info("Mir manager stopped")

	if err := m.WAL.Close(); err != nil {
		log.Errorf("Could not close write-ahead log: %s", err)
	}
	log.Info("WAL closed")

	if m.interceptor != nil {
		if err := m.interceptor.Stop(); err != nil {
			log.Errorf("Could not close interceptor: %s", err)
		}
		log.Info("Interceptor closed")
	}

	m.Net.Stop()
	log.Info("Network transport stopped")
}

// ReconfigureMirNode reconfigures the Mir node.
func (m *Manager) ReconfigureMirNode(ctx context.Context, nodes map[t.NodeID]t.NodeAddress) error {
	log.With("miner", m.MirID).Debug("Reconfiguring a Mir node")

	if len(nodes) == 0 {
		return fmt.Errorf("empty validator set")
	}

	go m.Net.Connect(ctx, nodes)
	go m.Net.CloseOldConnections(ctx, nodes)

	return nil
}

func (m *Manager) SubmitRequests(ctx context.Context, requests []*mirproto.Request) {
	if len(requests) == 0 {
		return
	}
	e := events.NewClientRequests("mempool", requests)
	if err := m.MirNode.InjectEvents(ctx, events.ListOf(e)); err != nil {
		log.Errorf("failed to submit requests to Mir: %s", err)
	}
	log.Infof("submitted %d requests to Mir", len(requests))
}

func parseTx(tx []byte) (interface{}, error) {
	ln := len(tx)
	// This is very simple input validation to be protected against invalid messages.
	if ln <= 2 {
		return nil, fmt.Errorf("mir tx len %d is too small", ln)
	}

	var err error
	var msg interface{}

	lastByte := tx[ln-1]
	// switch lastByte {
	// case common.SignedMessageType:
	// 	msg, err = types.DecodeSignedMessage(tx[:ln-1])
	// case common.CrossMessageType:
	// 	msg, err = types.DecodeUnverifiedCrossMessage(tx[:ln-1])
	// case common.ConfigMessageType:
	// 	return nil, fmt.Errorf("config message is not supported")
	// default:
	// 	err = fmt.Errorf("unknown message type %d", lastByte)
	// }

	if err != nil {
		return nil, err
	}

	return msg, nil
}

// GetMessages extracts Filecoin messages from a Mir batch.
func (m *Manager) GetMessages(batch *Batch) (msgs []*types.SignedMessage, crossMsgs []*types.Message) {
	log.Infof("received a block with %d messages", len(msgs))
	for _, tx := range batch.Messages {

		input, err := parseTx(tx)
		if err != nil {
			log.Error("unable to decode a message in Mir block:", err)
			continue
		}

		switch msg := input.(type) {
		case *types.SignedMessage:
			found := m.Pool.DeleteRequest(msg.Cid().String())
			if !found {
				log.Errorf("unable to find a request with %v hash", msg.Cid())
				continue
			}
			msgs = append(msgs, msg)
			log.Infof("got message: to=%s, nonce= %d", msg.Message.To, msg.Message.Nonce)
		case *types.UnverifiedCrossMsg:
			found := m.Pool.DeleteRequest(msg.Cid().String())
			if !found {
				log.Errorf("unable to find a request with %v hash", msg.Cid())
				continue
			}
			crossMsgs = append(crossMsgs, msg.Message)
			log.Infof("got cross-message: to=%s, nonce= %d", msg.Message.To, msg.Message.Nonce)
		default:
			log.Error("got unknown type request in a block")
		}
	}
	return
}

func (m *Manager) TransportRequests(msgs []*types.SignedMessage, crossMsgs []*types.UnverifiedCrossMsg) (
	requests []*mirproto.Request,
) {
	requests = append(requests, m.batchSignedMessages(msgs)...)
	requests = append(requests, m.batchCrossMessages(crossMsgs)...)
	return
}

func (m *Manager) ReconfigurationRequest(payload []byte) *mirproto.Request {
	r := mirproto.Request{
		ClientId: m.MirID,
		ReqNo:    m.reconfigurationNonce,
		Type:     ReconfigurationType,
		Data:     payload,
	}
	m.reconfigurationNonce++
	return &r
}

// batchPushSignedMessages pushes signed messages into the request pool and sends them to Mir.
func (m *Manager) batchSignedMessages(msgs []*types.SignedMessage) (
	requests []*mirproto.Request,
) {
	for _, msg := range msgs {
		clientID := newMirID(m.SubnetID.String(), msg.Message.From.String())
		nonce := msg.Message.Nonce
		if !m.Pool.IsTargetRequest(clientID) {
			continue
		}

		msgBytes, err := msg.Serialize()
		if err != nil {
			log.Error("unable to serialize message:", err)
			continue
		}
		// data := common.NewSignedMessageBytes(msgBytes, nil)

		r := &mirproto.Request{
			ClientId: clientID,
			ReqNo:    nonce,
			Type:     TransportType,
			Data:     data,
		}

		m.Pool.AddRequest(msg.Cid().String(), r)

		requests = append(requests, r)
	}
	return requests
}

// batchCrossMessages batches cross messages into the request pool and sends them to Mir.
func (m *Manager) batchCrossMessages(crossMsgs []*types.UnverifiedCrossMsg) (
	requests []*mirproto.Request,
) {
	for _, msg := range crossMsgs {
		msn, err := msg.Message.From.Subnet()
		if err != nil {
			log.Error("unable to get subnet from message:", err)
			continue
		}
		clientID := newMirID(msn.String(), msg.Message.From.String())
		nonce := msg.Message.Nonce
		if !m.Pool.IsTargetRequest(clientID) {
			continue
		}

		msgBytes, err := msg.Serialize()
		if err != nil {
			log.Error("unable to serialize cross-message:", err)
			continue
		}

		// data := common.NewCrossMessageBytes(msgBytes, nil)
		r := &mirproto.Request{
			ClientId: clientID,
			ReqNo:    nonce,
			Type:     TransportType,
			Data:     data,
		}
		m.Pool.AddRequest(msg.Cid().String(), r)
		requests = append(requests, r)
	}
	return requests
}

// ID prints Manager ID.
func (m *Manager) ID() string {
	addr := m.Addr.String()
	return fmt.Sprintf("%v:%v", m.SubnetID, addr[len(addr)-6:])
}

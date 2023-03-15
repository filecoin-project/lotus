package consensus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	hraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/ipfs/go-log/v2"
	p2praft "github.com/libp2p/go-libp2p-raft"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/filecoin-project/lotus/lib/addrutil"
	"github.com/filecoin-project/lotus/node/repo"
)

var raftLogger = log.Logger("raft-cluster")

// ErrWaitingForSelf is returned when we are waiting for ourselves to depart
// the peer set, which won't happen
var errWaitingForSelf = errors.New("waiting for ourselves to depart")

// RaftMaxSnapshots indicates how many snapshots to keep in the consensus data
// folder.
// TODO: Maybe include this in Config. Not sure how useful it is to touch
// this anyways.
var RaftMaxSnapshots = 5

// RaftLogCacheSize is the maximum number of logs to cache in-memory.
// This is used to reduce disk I/O for the recently committed entries.
var RaftLogCacheSize = 512

// How long we wait for updates during shutdown before snapshotting
var waitForUpdatesShutdownTimeout = 5 * time.Second
var waitForUpdatesInterval = 400 * time.Millisecond

// How many times to retry snapshotting when shutting down
var maxShutdownSnapshotRetries = 5

// raftWrapper wraps the hraft.Raft object and related things like the
// different stores used or the hraft.Configuration.
// Its methods provide functionality for working with Raft.
type raftWrapper struct {
	ctx           context.Context
	cancel        context.CancelFunc
	raft          *hraft.Raft
	config        *ClusterRaftConfig
	host          host.Host
	serverConfig  hraft.Configuration
	transport     *hraft.NetworkTransport
	snapshotStore hraft.SnapshotStore
	logStore      hraft.LogStore
	stableStore   hraft.StableStore
	boltdb        *raftboltdb.BoltStore
	repo          repo.LockedRepo
	staging       bool
}

// newRaftWrapper creates a Raft instance and initializes
// everything leaving it ready to use. Note, that Bootstrap() should be called
// to make sure the raft instance is usable.
func newRaftWrapper(
	host host.Host,
	cfg *ClusterRaftConfig,
	fsm hraft.FSM,
	repo repo.LockedRepo,
	staging bool,
) (*raftWrapper, error) {

	raftW := &raftWrapper{}
	raftW.config = cfg
	raftW.host = host
	raftW.staging = staging
	raftW.repo = repo
	// Set correct LocalID
	cfg.RaftConfig.LocalID = hraft.ServerID(host.ID().String())

	df := cfg.GetDataFolder(repo)
	err := makeDataFolder(df)
	if err != nil {
		return nil, err
	}

	err = raftW.makeServerConfig()
	if err != nil {
		return nil, err
	}

	err = raftW.makeTransport()
	if err != nil {
		return nil, err
	}

	err = raftW.makeStores()
	if err != nil {
		return nil, err
	}

	raftLogger.Debug("creating Raft")
	raftW.raft, err = hraft.NewRaft(
		cfg.RaftConfig,
		fsm,
		raftW.logStore,
		raftW.stableStore,
		raftW.snapshotStore,
		raftW.transport,
	)
	if err != nil {
		raftLogger.Error("initializing raft: ", err)
		return nil, err
	}

	raftW.ctx, raftW.cancel = context.WithCancel(context.Background())

	return raftW, nil
}

// makeDataFolder creates the folder that is meant to store Raft data. Ensures
// we always set 0700 mode.
func makeDataFolder(folder string) error {
	return os.MkdirAll(folder, 0700)
}

func (rw *raftWrapper) makeTransport() (err error) {
	raftLogger.Debug("creating libp2p Raft transport")
	rw.transport, err = p2praft.NewLibp2pTransport(
		rw.host,
		rw.config.NetworkTimeout,
	)
	return err
}

func (rw *raftWrapper) makeStores() error {
	raftLogger.Debug("creating BoltDB store")
	df := rw.config.GetDataFolder(rw.repo)
	store, err := raftboltdb.NewBoltStore(filepath.Join(df, "raft.db"))
	if err != nil {
		return err
	}

	// wraps the store in a LogCache to improve performance.
	// See consul/agent/consul/server.go
	cacheStore, err := hraft.NewLogCache(RaftLogCacheSize, store)
	if err != nil {
		return err
	}

	raftLogger.Debug("creating raft snapshot store")
	snapstore, err := hraft.NewFileSnapshotStoreWithLogger(
		df,
		RaftMaxSnapshots,
		hclog.FromStandardLogger(zap.NewStdLog(log.Logger("raft-snapshot").SugaredLogger.Desugar()), hclog.DefaultOptions),
	)
	if err != nil {
		return err
	}

	rw.logStore = cacheStore
	rw.stableStore = store
	rw.snapshotStore = snapstore
	rw.boltdb = store
	return nil
}

// Bootstrap calls BootstrapCluster on the Raft instance with a valid
// Configuration (generated from InitPeerset) when Raft has no state
// and we are not setting up a staging peer. It returns if Raft
// was boostrapped (true) and an error.
func (rw *raftWrapper) Bootstrap() (bool, error) {
	logger.Debug("checking for existing raft states")
	hasState, err := hraft.HasExistingState(
		rw.logStore,
		rw.stableStore,
		rw.snapshotStore,
	)
	if err != nil {
		return false, err
	}

	if hasState {
		logger.Debug("raft cluster is already initialized")

		// Inform the user that we are working with a pre-existing peerset
		logger.Info("existing Raft state found! raft.InitPeerset will be ignored")
		cf := rw.raft.GetConfiguration()
		if err := cf.Error(); err != nil {
			logger.Debug(err)
			return false, err
		}
		currentCfg := cf.Configuration()
		srvs := ""
		for _, s := range currentCfg.Servers {
			srvs += fmt.Sprintf("        %s\n", s.ID)
		}

		logger.Debugf("Current Raft Peerset:\n%s\n", srvs)
		return false, nil
	}

	if rw.staging {
		logger.Debug("staging servers do not need initialization")
		logger.Info("peer is ready to join a cluster")
		return false, nil
	}

	voters := ""
	for _, s := range rw.serverConfig.Servers {
		voters += fmt.Sprintf("        %s\n", s.ID)
	}

	logger.Infof("initializing raft cluster with the following voters:\n%s\n", voters)

	future := rw.raft.BootstrapCluster(rw.serverConfig)
	if err := future.Error(); err != nil {
		logger.Error("bootstrapping cluster: ", err)
		return true, err
	}
	return true, nil
}

// create Raft servers configuration. The result is used
// by Bootstrap() when it proceeds to Bootstrap.
func (rw *raftWrapper) makeServerConfig() error {
	peers := []peer.ID{}
	addrInfos, err := addrutil.ParseAddresses(context.Background(), rw.config.InitPeerset)
	if err != nil {
		return err
	}
	for _, addrInfo := range addrInfos {
		peers = append(peers, addrInfo.ID)
	}
	rw.serverConfig = makeServerConf(append(peers, rw.host.ID()))
	return nil
}

// creates a server configuration with all peers as Voters.
func makeServerConf(peers []peer.ID) hraft.Configuration {
	sm := make(map[string]struct{})

	servers := make([]hraft.Server, 0)

	// Servers are peers + self. We avoid duplicate entries below
	for _, pid := range peers {
		p := pid.String()
		_, ok := sm[p]
		if !ok { // avoid dups
			sm[p] = struct{}{}
			servers = append(servers, hraft.Server{
				Suffrage: hraft.Voter,
				ID:       hraft.ServerID(p),
				Address:  hraft.ServerAddress(p),
			})
		}
	}
	return hraft.Configuration{Servers: servers}
}

// WaitForLeader holds until Raft says we have a leader.
// Returns if ctx is canceled.
func (rw *raftWrapper) WaitForLeader(ctx context.Context) (string, error) {
	ticker := time.NewTicker(time.Second / 2)
	for {
		select {
		case <-ticker.C:
			if l := rw.raft.Leader(); l != "" {
				logger.Debug("waitForleaderTimer")
				logger.Infof("Current Raft Leader: %s", l)
				ticker.Stop()
				return string(l), nil
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func (rw *raftWrapper) WaitForVoter(ctx context.Context) error {
	logger.Debug("waiting until we are promoted to a voter")

	pid := hraft.ServerID(rw.host.ID().String())
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			logger.Debugf("%s: get configuration", pid)
			configFuture := rw.raft.GetConfiguration()
			if err := configFuture.Error(); err != nil {
				return err
			}

			if isVoter(pid, configFuture.Configuration()) {
				return nil
			}
			logger.Debugf("%s: not voter yet", pid)

			time.Sleep(waitForUpdatesInterval)
		}
	}
}

func isVoter(srvID hraft.ServerID, cfg hraft.Configuration) bool {
	for _, server := range cfg.Servers {
		if server.ID == srvID && server.Suffrage == hraft.Voter {
			return true
		}
	}
	return false
}

// WaitForUpdates holds until Raft has synced to the last index in the log
func (rw *raftWrapper) WaitForUpdates(ctx context.Context) error {

	logger.Debug("Raft state is catching up to the latest known version. Please wait...")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			lai := rw.raft.AppliedIndex()
			li := rw.raft.LastIndex()
			logger.Debugf("current Raft index: %d/%d",
				lai, li)
			if lai == li {
				return nil
			}
			time.Sleep(waitForUpdatesInterval)
		}
	}
}

func (rw *raftWrapper) WaitForPeer(ctx context.Context, pid string, depart bool) error {

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			peers, err := rw.Peers(ctx)
			if err != nil {
				return err
			}

			if len(peers) == 1 && pid == peers[0] && depart {
				return errWaitingForSelf
			}

			found := find(peers, pid)

			// departing
			if depart && !found {
				return nil
			}

			// joining
			if !depart && found {
				return nil
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

// Snapshot tells Raft to take a snapshot.
func (rw *raftWrapper) Snapshot() error {
	future := rw.raft.Snapshot()
	err := future.Error()
	if err != nil && err.Error() != hraft.ErrNothingNewToSnapshot.Error() {
		return err
	}
	return nil
}

// snapshotOnShutdown attempts to take a snapshot before a shutdown.
// Snapshotting might fail if the raft applied index is not the last index.
// This waits for the updates and tries to take a snapshot when the
// applied index is up to date.
// It will retry if the snapshot still fails, in case more updates have arrived.
// If waiting for updates times-out, it will not try anymore, since something
// is wrong. This is a best-effort solution as there is no way to tell Raft
// to stop processing entries because we want to take a snapshot before
// shutting down.
func (rw *raftWrapper) snapshotOnShutdown() error {
	var err error
	for i := 0; i < maxShutdownSnapshotRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), waitForUpdatesShutdownTimeout)
		err = rw.WaitForUpdates(ctx)
		cancel()
		if err != nil {
			logger.Warn("timed out waiting for state updates before shutdown. Snapshotting may fail")
			return rw.Snapshot()
		}

		err = rw.Snapshot()
		if err == nil {
			return nil // things worked
		}

		// There was an error
		err = errors.New("could not snapshot raft: " + err.Error())
		logger.Warnf("retrying to snapshot (%d/%d)...", i+1, maxShutdownSnapshotRetries)
	}
	return err
}

// Shutdown shutdown Raft and closes the BoltDB.
func (rw *raftWrapper) Shutdown(ctx context.Context) error {

	rw.cancel()

	var finalErr error

	err := rw.snapshotOnShutdown()
	if err != nil {
		finalErr = multierr.Append(finalErr, err)
	}

	future := rw.raft.Shutdown()
	err = future.Error()
	if err != nil {
		finalErr = multierr.Append(finalErr, err)
	}

	err = rw.boltdb.Close() // important!
	if err != nil {
		finalErr = multierr.Append(finalErr, err)
	}

	return finalErr
}

// AddPeer adds a peer to Raft
func (rw *raftWrapper) AddPeer(ctx context.Context, peerId peer.ID) error {

	// Check that we don't have it to not waste
	// log entries if so.
	peers, err := rw.Peers(ctx)
	if err != nil {
		return err
	}
	if find(peers, peerId.String()) {
		logger.Infof("%s is already a raft peerStr", peerId.String())
		return nil
	}

	err = rw.host.Connect(ctx, peer.AddrInfo{ID: peerId})
	if err != nil {
		return err
	}

	future := rw.raft.AddVoter(
		hraft.ServerID(peerId.String()),
		hraft.ServerAddress(peerId.String()),
		0,
		0,
	) // TODO: Extra cfg value?
	err = future.Error()
	if err != nil {
		logger.Error("raft cannot add peer: ", err)
	}
	return err
}

// RemovePeer removes a peer from Raft
func (rw *raftWrapper) RemovePeer(ctx context.Context, peer string) error {
	// Check that we have it to not waste
	// log entries if we don't.
	peers, err := rw.Peers(ctx)
	if err != nil {
		return err
	}
	if !find(peers, peer) {
		logger.Infof("%s is not among raft peers", peer)
		return nil
	}

	if len(peers) == 1 && peers[0] == peer {
		return errors.New("cannot remove ourselves from a 1-peer cluster")
	}

	rmFuture := rw.raft.RemoveServer(
		hraft.ServerID(peer),
		0,
		0,
	)
	err = rmFuture.Error()
	if err != nil {
		logger.Error("raft cannot remove peer: ", err)
		return err
	}

	return nil
}

// Leader returns Raft's leader. It may be an empty string if
// there is no leader or it is unknown.
func (rw *raftWrapper) Leader(ctx context.Context) string {
	return string(rw.raft.Leader())
}

func (rw *raftWrapper) Peers(ctx context.Context) ([]string, error) {
	ids := make([]string, 0)

	configFuture := rw.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return nil, err
	}

	for _, server := range configFuture.Configuration().Servers {
		ids = append(ids, string(server.ID))
	}

	return ids, nil
}

// CleanupRaft moves the current data folder to a backup location
//func CleanupRaft(cfg *Config) error {
//	dataFolder := cfg.GetDataFolder()
//	keep := cfg.BackupsRotate
//
//	meta, _, err := latestSnapshot(dataFolder)
//	if meta == nil && err == nil {
//		// no snapshots at all. Avoid creating backups
//		// from empty state folders.
//		logger.Infof("cleaning empty Raft data folder (%s)", dataFolder)
//		os.RemoveAll(dataFolder)
//		return nil
//	}
//
//	logger.Infof("cleaning and backing up Raft data folder (%s)", dataFolder)
//	dbh := newDataBackupHelper(dataFolder, keep)
//	err = dbh.makeBackup()
//	if err != nil {
//		logger.Warn(err)
//		logger.Warn("the state could not be cleaned properly")
//		logger.Warn("manual intervention may be needed before starting cluster again")
//	}
//	return nil
//}

// only call when Raft is shutdown
func (rw *raftWrapper) Clean() error {
	//return CleanupRaft(rw.config)
	return nil
}

func find(s []string, elem string) bool {
	for _, selem := range s {
		if selem == elem {
			return true
		}
	}
	return false
}

func (rw *raftWrapper) observePeers() {
	obsCh := make(chan hraft.Observation, 1)
	defer close(obsCh)

	observer := hraft.NewObserver(obsCh, true, func(o *hraft.Observation) bool {
		po, ok := o.Data.(hraft.PeerObservation)
		return ok && po.Removed
	})

	rw.raft.RegisterObserver(observer)
	defer rw.raft.DeregisterObserver(observer)

	for {
		select {
		case obs := <-obsCh:
			pObs := obs.Data.(hraft.PeerObservation)
			logger.Info("raft peer departed. Removing from peerstore: ", pObs.Peer.ID)
			pID, err := peer.Decode(string(pObs.Peer.ID))
			if err != nil {
				logger.Error(err)
				continue
			}
			rw.host.Peerstore().ClearAddrs(pID)
		case <-rw.ctx.Done():
			logger.Debug("stopped observing raft peers")
			return
		}
	}
}

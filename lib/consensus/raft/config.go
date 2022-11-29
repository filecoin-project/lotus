package consensus

import (
	"io/ioutil"
	"path/filepath"
	"time"

	hraft "github.com/hashicorp/raft"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

// Configuration defaults
var (
	DefaultDataSubFolder        = "raft-cluster"
	DefaultWaitForLeaderTimeout = 15 * time.Second
	DefaultCommitRetries        = 1
	DefaultNetworkTimeout       = 100 * time.Second
	DefaultCommitRetryDelay     = 200 * time.Millisecond
	DefaultBackupsRotate        = 6
)

// ClusterRaftConfig allows to configure the Raft Consensus component for the node cluster.
type ClusterRaftConfig struct {
	// config to enabled node cluster with raft consensus
	ClusterModeEnabled bool
	// A folder to store Raft's data.
	DataFolder string
	// InitPeerset provides the list of initial cluster peers for new Raft
	// peers (with no prior state). It is ignored when Raft was already
	// initialized or when starting in staging mode.
	InitPeerset []string
	// LeaderTimeout specifies how long to wait for a leader before
	// failing an operation.
	WaitForLeaderTimeout time.Duration
	// NetworkTimeout specifies how long before a Raft network
	// operation is timed out
	NetworkTimeout time.Duration
	// CommitRetries specifies how many times we retry a failed commit until
	// we give up.
	CommitRetries int
	// How long to wait between retries
	CommitRetryDelay time.Duration
	// BackupsRotate specifies the maximum number of Raft's DataFolder
	// copies that we keep as backups (renaming) after cleanup.
	BackupsRotate int
	// A Hashicorp Raft's configuration object.
	RaftConfig *hraft.Config

	// Tracing enables propagation of contexts across binary boundaries.
	Tracing bool
}

func DefaultClusterRaftConfig() *ClusterRaftConfig {
	var cfg ClusterRaftConfig
	cfg.DataFolder = "" // empty so it gets omitted
	cfg.InitPeerset = []string{}
	cfg.WaitForLeaderTimeout = DefaultWaitForLeaderTimeout
	cfg.NetworkTimeout = DefaultNetworkTimeout
	cfg.CommitRetries = DefaultCommitRetries
	cfg.CommitRetryDelay = DefaultCommitRetryDelay
	cfg.BackupsRotate = DefaultBackupsRotate
	cfg.RaftConfig = hraft.DefaultConfig()

	// These options are imposed over any Default Raft Config.
	cfg.RaftConfig.ShutdownOnRemove = false
	cfg.RaftConfig.LocalID = "will_be_set_automatically"

	// Set up logging
	cfg.RaftConfig.LogOutput = ioutil.Discard
	return &cfg
}

func NewClusterRaftConfig(userRaftConfig *config.UserRaftConfig) *ClusterRaftConfig {
	var cfg ClusterRaftConfig
	cfg.DataFolder = userRaftConfig.DataFolder
	cfg.InitPeerset = userRaftConfig.InitPeersetMultiAddr
	cfg.WaitForLeaderTimeout = time.Duration(userRaftConfig.WaitForLeaderTimeout)
	cfg.NetworkTimeout = time.Duration(userRaftConfig.NetworkTimeout)
	cfg.CommitRetries = userRaftConfig.CommitRetries
	cfg.CommitRetryDelay = time.Duration(userRaftConfig.CommitRetryDelay)
	cfg.BackupsRotate = userRaftConfig.BackupsRotate

	// Keep this to be default hraft config for now
	cfg.RaftConfig = hraft.DefaultConfig()

	// These options are imposed over any Default Raft Config.
	cfg.RaftConfig.ShutdownOnRemove = false
	cfg.RaftConfig.LocalID = "will_be_set_automatically"

	// Set up logging
	cfg.RaftConfig.LogOutput = ioutil.Discard

	return &cfg

}

// Validate checks that this configuration has working values,
// at least in appearance.
func ValidateConfig(cfg *ClusterRaftConfig) error {
	if cfg.RaftConfig == nil {
		return xerrors.Errorf("no hashicorp/raft.Config")
	}
	if cfg.WaitForLeaderTimeout <= 0 {
		return xerrors.Errorf("wait_for_leader_timeout <= 0")
	}

	if cfg.NetworkTimeout <= 0 {
		return xerrors.Errorf("network_timeout <= 0")
	}

	if cfg.CommitRetries < 0 {
		return xerrors.Errorf("commit_retries is invalid")
	}

	if cfg.CommitRetryDelay <= 0 {
		return xerrors.Errorf("commit_retry_delay is invalid")
	}

	if cfg.BackupsRotate <= 0 {
		return xerrors.Errorf("backups_rotate should be larger than 0")
	}

	return hraft.ValidateConfig(cfg.RaftConfig)
}

// GetDataFolder returns the Raft data folder that we are using.
func (cfg *ClusterRaftConfig) GetDataFolder(repo repo.LockedRepo) string {
	if cfg.DataFolder == "" {
		return filepath.Join(repo.Path(), DefaultDataSubFolder)
	}
	return filepath.Join(repo.Path(), cfg.DataFolder)
}

package impl

import (
	"context"
	"fmt"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/impl/client"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/impl/market"
	"github.com/filecoin-project/lotus/node/impl/net"
	"github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
)

var log = logging.Logger("node")

type FullNodeAPI struct {
	common.CommonAPI
	net.NetAPI
	full.ChainAPI
	client.API
	full.MpoolAPI
	full.GasAPI
	market.MarketAPI
	paych.PaychAPI
	full.StateAPI
	full.MsigAPI
	full.WalletAPI
	full.SyncAPI

	DS          dtypes.MetadataDS
	NetworkName dtypes.NetworkName
}

func (n *FullNodeAPI) CreateBackup(ctx context.Context, fpath string) error {
	return backup(ctx, n.DS, fpath)
}

func (n *FullNodeAPI) NodeStatus(ctx context.Context, inclChainStatus bool) (status api.NodeStatus, err error) {
	curTs, err := n.ChainHead(ctx)
	if err != nil {
		return status, err
	}

	status.SyncStatus.Epoch = uint64(curTs.Height())
	timestamp := time.Unix(int64(curTs.MinTimestamp()), 0)
	delta := time.Since(timestamp).Seconds()
	status.SyncStatus.Behind = uint64(delta / 30)

	// get peers in the messages and blocks topics
	peersMsgs := make(map[peer.ID]struct{})
	peersBlocks := make(map[peer.ID]struct{})

	for _, p := range n.PubSub.ListPeers(build.MessagesTopic(n.NetworkName)) {
		peersMsgs[p] = struct{}{}
	}

	for _, p := range n.PubSub.ListPeers(build.BlocksTopic(n.NetworkName)) {
		peersBlocks[p] = struct{}{}
	}

	// get scores for all connected and recent peers
	scores, err := n.NetPubsubScores(ctx)
	if err != nil {
		return status, err
	}

	for _, score := range scores {
		if score.Score.Score > lp2p.PublishScoreThreshold {
			_, inMsgs := peersMsgs[score.ID]
			if inMsgs {
				status.PeerStatus.PeersToPublishMsgs++
			}

			_, inBlocks := peersBlocks[score.ID]
			if inBlocks {
				status.PeerStatus.PeersToPublishBlocks++
			}
		}
	}

	if inclChainStatus && status.SyncStatus.Epoch > uint64(build.Finality) {
		blockCnt := 0
		ts := curTs

		for i := 0; i < 100; i++ {
			blockCnt += len(ts.Blocks())
			tsk := ts.Parents()
			ts, err = n.ChainGetTipSet(ctx, tsk)
			if err != nil {
				return status, err
			}
		}

		status.ChainStatus.BlocksPerTipsetLast100 = float64(blockCnt) / 100

		for i := 100; i < int(build.Finality); i++ {
			blockCnt += len(ts.Blocks())
			tsk := ts.Parents()
			ts, err = n.ChainGetTipSet(ctx, tsk)
			if err != nil {
				return status, err
			}
		}

		status.ChainStatus.BlocksPerTipsetLastFinality = float64(blockCnt) / float64(build.Finality)

	}

	return status, nil
}

// This method is exclusively used for Non Interactive Authentication based on SP Worker Address
func (n *FullNodeAPI) FilIdSp(ctx context.Context, maddr address.Address) (string, error) {
	FIL_AUTHHDR := "FIL-SPID-V0"

	head, err := n.ChainHead(ctx)
	if err != nil {
		return "", xerrors.Errorf("failed to get chain head: %w", err)
	}

	beacon, err := n.StateGetBeaconEntry(ctx, head.Height())
	if err != nil {
		return "", xerrors.Errorf("failed to get DRAND beacon: %w", err)
	}

	finality := head.Height() - miner.ChainFinality
	ftipset, err := n.ChainGetTipSetByHeight(ctx, finality, head.Key())
	if err != nil {
		return "", xerrors.Errorf("failed to get finality tipset: %w", err)
	}

	var msg []byte

	// Append prefix to avoid making a viable message
	// FIXME: this should be replaced by SPIDV1 combined with proper domain-separation when it arrives
	msg = append(msg, []byte("   ")...)
	// Append beacon
	msg = append(msg, beacon.Data...)

	// Get miner info for last finality
	minfo, err := n.StateMinerInfo(ctx, maddr, ftipset.Key())
	if err != nil {
		return "", xerrors.Errorf("failed to get miner info: %w", err)
	}

	worker := minfo.Worker
	var sig *crypto.Signature

	sig, err = n.WalletSign(ctx, worker, msg)
	if err != nil {
		return "", xerrors.Errorf("failed to get sign message with worker address key: %w", err)
	}

	return fmt.Sprintf("%s %d;%s;%s", FIL_AUTHHDR, head.Height(), maddr, string(append([]byte{byte(sig.Type)}, sig.Data...))), nil
}

// This method is exclusively used for Non Interactive Authentication based on provided Wallet Address
func (n *FullNodeAPI) FilIdAddr(ctx context.Context, addr address.Address) (string, error) {
	FIL_AUTHHDR := "FIL-SPID-V0"

	head, err := n.ChainHead(ctx)
	if err != nil {
		return "", xerrors.Errorf("failed to get chain head: %w", err)
	}

	beacon, err := n.StateGetBeaconEntry(ctx, head.Height())
	if err != nil {
		return "", xerrors.Errorf("failed to get DRAND beacon: %w", err)
	}

	var msg []byte

	// Append prefix to avoid making a viable message
	// FIXME: this should be replaced by SPIDV1 combined with proper domain-separation when it arrives
	msg = append(msg, []byte("   ")...)
	// Append beacon
	msg = append(msg, beacon.Data...)

	var sig *crypto.Signature

	sig, err = n.WalletSign(ctx, addr, msg)
	if err != nil {
		return "", xerrors.Errorf("failed to get sign message with worker address key: %w", err)
	}

	return fmt.Sprintf("%s %d;%s;%s", FIL_AUTHHDR, head.Height(), addr, string(append([]byte{byte(sig.Type)}, sig.Data...))), nil
}

var _ api.FullNode = &FullNodeAPI{}

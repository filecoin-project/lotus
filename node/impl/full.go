package impl

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"

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
	full.RaftAPI

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
func (n *FullNodeAPI) FilIdSp(ctx context.Context, spID address.Address, optionalArg []byte) (string, error) {

	head, err := n.ChainHead(ctx)
	if err != nil {
		return "", xerrors.Errorf("failed to get chain head: %w", err)
	}

	finHeight := head.Height() - miner.ChainFinality
	finTipSet, err := n.ChainGetTipSetByHeight(ctx, finHeight, head.Key())
	if err != nil {
		return "", xerrors.Errorf("failed to get finalized tipset at %d: %w", finHeight, err)
	}

	// Get miner info for last finality
	minfo, err := n.StateMinerInfo(ctx, spID, finTipSet.Key())
	if err != nil {
		return "", xerrors.Errorf("failed to get info for SP %s at %d: %w", spID.String(), finTipSet.Height(), err)
	}

	return signNoninteractiveChallege(
		ctx,
		n,
		"FIL-SPID-V0",
		spID, minfo.Worker,
		head.Height(),
		optionalArg,
	)
}

// This method is exclusively used for Non Interactive Authentication based on provided Wallet Address
func (n *FullNodeAPI) FilIdAddr(ctx context.Context, addr address.Address, optionalArg []byte) (string, error) {

	head, err := n.ChainHead(ctx)
	if err != nil {
		return "", xerrors.Errorf("failed to get chain head: %w", err)
	}

	if addr.Protocol() == address.ID {
		resolvedAddr, err := n.StateAccountKey(ctx, addr, head.Key())
		if err != nil {
			return "", xerrors.Errorf("could not resolve ID address %s to public address: %w", addr.String(), err)
		}
		addr = resolvedAddr
	}

	return signNoninteractiveChallege(
		ctx,
		n,
		"FIL-ADDRID-V0",
		addr, addr,
		head.Height(),
		optionalArg,
	)
}

func signNoninteractiveChallege(ctx context.Context, n *FullNodeAPI, challengeType string, target, signer address.Address, drandHeight abi.ChainEpoch, optionalArg []byte) (string, error) {

	if len(optionalArg) > 2048 {
		return "", xerrors.Errorf("optional argument longer than the maximum of 2048 bytes")
	}

	beacon, err := n.StateGetBeaconEntry(ctx, drandHeight)
	if err != nil {
		return "", xerrors.Errorf("failed to get DRAND beacon: %w", err)
	}

	sig, err := n.WalletSign(
		ctx,
		signer,
		append(
			append(
				append(
					// Prefix is to avoid ever producing a viable CBOR message
					// FIXME: this should be replaced by SPIDV1 combined with proper domain-separation when it arrives
					make([]byte, 0, 3+len(beacon.Data)+len(optionalArg)),
					[]byte("   ")...,
				),
				beacon.Data...,
			),
			optionalArg...,
		),
	)
	if err != nil {
		return "", xerrors.Errorf("failed to sign message with key %s: %w", signer.String(), err)
	}

	token := fmt.Sprintf("%s %d;%s;%s", challengeType, drandHeight, target, base64.StdEncoding.EncodeToString(sig.Data))
	if len(optionalArg) > 0 {
		token += ";" + base64.StdEncoding.EncodeToString(optionalArg)
	}

	return token, nil
}

func (n *FullNodeAPI) RaftState(ctx context.Context) (*api.RaftStateData, error) {
	return n.RaftAPI.GetRaftState(ctx)
}

func (n *FullNodeAPI) RaftLeader(ctx context.Context) (peer.ID, error) {
	return n.RaftAPI.Leader(ctx)
}

var _ api.FullNode = &FullNodeAPI{}

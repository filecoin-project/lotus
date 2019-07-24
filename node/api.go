package node

import (
	"context"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/miner"
	"github.com/filecoin-project/go-lotus/node/client"
	"github.com/filecoin-project/go-lotus/node/modules"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
)

var log = logging.Logger("node")

type FullNodeAPI struct {
	client.LocalStorage

	Host      host.Host
	Chain     *chain.ChainStore
	PubSub    *pubsub.PubSub
	Mpool     *chain.MessagePool
	Wallet    *chain.Wallet
	APISecret *modules.APIAlg
}

type jwtPayload struct {
	Allow []string
}

func (a *FullNodeAPI) AuthVerify(ctx context.Context, token string) ([]string, error) {
	var payload jwtPayload
	if _, err := jwt.Verify([]byte(token), (*jwt.HMACSHA)(a.APISecret), &payload); err != nil {
		return nil, xerrors.Errorf("JWT Verification failed: %w", err)
	}

	return payload.Allow, nil
}

func (a *FullNodeAPI) AuthNew(ctx context.Context, perms []string) ([]byte, error) {
	p := jwtPayload{
		Allow: perms, // TODO: consider checking validity
	}

	return jwt.Sign(&p, (*jwt.HMACSHA)(a.APISecret))
}

func (a *FullNodeAPI) ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error {
	if err := a.Chain.AddBlock(blk.Header); err != nil {
		return err
	}

	b, err := blk.Serialize()
	if err != nil {
		return err
	}

	// TODO: anything else to do here?
	return a.PubSub.Publish("/fil/blocks", b)
}

func (a *FullNodeAPI) ChainHead(context.Context) (*chain.TipSet, error) {
	return a.Chain.GetHeaviestTipSet(), nil
}

func (a *FullNodeAPI) ChainGetRandomness(ctx context.Context, pts *chain.TipSet) ([]byte, error) {
	// TODO: this needs to look back in the chain for the right random beacon value
	return []byte("foo bar random"), nil
}

func (a *FullNodeAPI) ChainWaitMsg(ctx context.Context, msg cid.Cid) (*api.MsgWait, error) {
	panic("TODO")
}

func (a *FullNodeAPI) ChainGetBlock(ctx context.Context, msg cid.Cid) (*chain.BlockHeader, error) {
	return a.Chain.GetBlock(msg)
}

func (a *FullNodeAPI) ChainGetBlockMessages(ctx context.Context, msg cid.Cid) ([]*chain.SignedMessage, error) {
	b, err := a.Chain.GetBlock(msg)
	if err != nil {
		return nil, err
	}

	return a.Chain.MessagesForBlock(b)
}

func (a *FullNodeAPI) ID(context.Context) (peer.ID, error) {
	return a.Host.ID(), nil
}

func (a *FullNodeAPI) Version(context.Context) (api.Version, error) {
	return api.Version{
		Version: build.Version,
	}, nil
}

func (a *FullNodeAPI) MpoolPending(ctx context.Context, ts *chain.TipSet) ([]*chain.SignedMessage, error) {
	// TODO: need to make sure we don't return messages that were already included in the referenced chain
	// also need to accept ts == nil just fine, assume nil == chain.Head()
	return a.Mpool.Pending(), nil
}

func (a *FullNodeAPI) MpoolPush(ctx context.Context, smsg *chain.SignedMessage) error {
	msgb, err := smsg.Serialize()
	if err != nil {
		return err
	}

	return a.PubSub.Publish("/fil/messages", msgb)
}

func (a *FullNodeAPI) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return a.Mpool.GetNonce(addr)
}

func (a *FullNodeAPI) MinerStart(ctx context.Context, addr address.Address) error {
	// hrm...
	m := miner.NewMiner(a, addr)

	go m.Mine(context.TODO())

	return nil
}

func (a *FullNodeAPI) MinerCreateBlock(ctx context.Context, addr address.Address, parents *chain.TipSet, tickets []chain.Ticket, proof chain.ElectionProof, msgs []*chain.SignedMessage) (*chain.BlockMsg, error) {
	fblk, err := chain.MinerCreateBlock(a.Chain, addr, parents, tickets, proof, msgs)
	if err != nil {
		return nil, err
	}

	var out chain.BlockMsg
	out.Header = fblk.Header
	for _, msg := range fblk.Messages {
		out.Messages = append(out.Messages, msg.Cid())
	}

	return &out, nil
}

func (a *FullNodeAPI) NetPeers(context.Context) ([]peer.AddrInfo, error) {
	conns := a.Host.Network().Conns()
	out := make([]peer.AddrInfo, len(conns))

	for i, conn := range conns {
		out[i] = peer.AddrInfo{
			ID: conn.RemotePeer(),
			Addrs: []ma.Multiaddr{
				conn.RemoteMultiaddr(),
			},
		}
	}

	return out, nil
}

func (a *FullNodeAPI) WalletNew(ctx context.Context, typ string) (address.Address, error) {
	return a.Wallet.GenerateKey(typ)
}

func (a *FullNodeAPI) WalletList(ctx context.Context) ([]address.Address, error) {
	return a.Wallet.ListAddrs()
}

func (a *FullNodeAPI) WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error) {
	return a.Chain.GetBalance(addr)
}

func (a *FullNodeAPI) WalletSign(ctx context.Context, k address.Address, msg []byte) (*chain.Signature, error) {
	return a.Wallet.Sign(k, msg)
}

func (a *FullNodeAPI) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	addrs, err := a.Wallet.ListAddrs()
	if err != nil {
		return address.Undef, err
	}

	// TODO: store a default address in the config or 'wallet' portion of the repo
	return addrs[0], nil
}

func (a *FullNodeAPI) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return a.Host.Connect(ctx, p)
}

func (a *FullNodeAPI) NetAddrsListen(context.Context) (peer.AddrInfo, error) {
	return peer.AddrInfo{
		ID:    a.Host.ID(),
		Addrs: a.Host.Addrs(),
	}, nil
}

var _ api.FullNode = &FullNodeAPI{}

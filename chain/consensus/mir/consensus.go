// Package mir implements Eudico consensus in Mir framework.
package mir

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var _ consensus.Consensus = &Mir{}

var RewardFunc = func(ctx context.Context, vmi vm.Interface, em stmgr.ExecMonitor,
	epoch abi.ChainEpoch, ts *types.TipSet, params []byte) error {
	// No Rewards applied for now, not even the fees.
	return nil
}

type Mir struct {
	store    *store.ChainStore
	beacon   beacon.Schedule
	sm       *stmgr.StateManager
	verifier storiface.Verifier
	genesis  *types.TipSet
}

func NewConsensus(
	ctx context.Context,
	sm *stmgr.StateManager,
	b beacon.Schedule,
	v storiface.Verifier,
	g chain.Genesis,
) (consensus.Consensus, error) {
	log.Info("Using Mir consensus")
	return &Mir{
		store:    sm.ChainStore(),
		beacon:   b,
		sm:       sm,
		verifier: v,
		genesis:  g,
	}, nil
}

func (bft *Mir) VerifyBlockSignature(ctx context.Context, h *types.BlockHeader,
	addr address.Address) error {
	// return sigs.CheckBlockSignature(ctx, h, addr)
	panic("not implemented")
}

func (bft *Mir) SignBlock(ctx context.Context, w api.Wallet,
	addr address.Address) error {

	// nosigbytes, err := next.SigningBytes()
	// if err != nil {
	// 	return xerrors.Errorf("failed to get signing bytes for block: %w", err)
	// }

	// sig, err := w.WalletSign(ctx, addr, nosigbytes, api.MsgMeta{
	// 	Type: api.MTBlock,
	// })
	// if err != nil {
	// 	return xerrors.Errorf("failed to sign new block: %w", err)
	// }
	// next.BlockSig = sig
	// return nil
	panic("not implemented")
}

// CreateBlock creates a Filecoin block from the input block template.
func (bft *Mir) CreateBlock(ctx context.Context, w lapi.Wallet, bt *lapi.BlockTemplate) (*types.FullBlock, error) {
	panic("not implemented")
}

func (bft *Mir) ValidateBlock(ctx context.Context, b *types.FullBlock) (err error) {
	panic("not implemented")
}

func (bft *Mir) ValidateBlockHeader(ctx context.Context, b *types.BlockHeader) (rejectReason string, err error) {
	panic("not implemented")
}

func (bft *Mir) minerIsValid(maddr address.Address) error {
	switch maddr.Protocol() {
	case address.BLS:
		fallthrough
	case address.SECP256K1:
		return nil
	}
	return xerrors.Errorf("miner address must be a key")
}

// IsEpochBeyondCurrMax is used in Filcns to detect delayed blocks.
// We are currently using defaults here and not worrying about it.
// We will consider potential changes of Consensus interface in https://github.com/filecoin-project/eudico/issues/143.
func (bft *Mir) IsEpochBeyondCurrMax(epoch abi.ChainEpoch) bool {
	return false
}

// Weight defines weight.
// We are just using a default weight for all subnet consensus algorithms.
func Weight(ctx context.Context, stateBs bstore.Blockstore, ts *types.TipSet) (types.BigInt, error) {
	if ts == nil {
		return types.NewInt(0), nil
	}

	return big.NewInt(int64(ts.Height() + 1)), nil
}

func validateLocalBlock(ctx context.Context, msg *pubsub.Message) (pubsub.ValidationResult, string) {
	stats.Record(ctx, metrics.BlockPublished.M(1))

	if size := msg.Size(); size > 1<<20-1<<15 {
		log.Errorf("ignoring oversize block (%dB)", size)
		return pubsub.ValidationIgnore, "oversize_block"
	}

	blk, what, err := decodeAndCheckBlock(msg)
	if err != nil {
		log.Errorf("got invalid local block: %s", err)
		return pubsub.ValidationIgnore, what
	}

	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept, ""
}

func decodeAndCheckBlock(msg *pubsub.Message) (*types.BlockMsg, string, error) {
	blk, err := types.DecodeBlockMsg(msg.GetData())
	if err != nil {
		return nil, "invalid", xerrors.Errorf("error decoding block: %w", err)
	}

	if count := len(blk.BlsMessages) + len(blk.SecpkMessages); count > build.BlockMessageLimit {
		return nil, "too_many_messages", xerrors.Errorf("block contains too many messages (%d)", count)
	}

	// make sure we have a signature
	if blk.Header.BlockSig != nil {
		return nil, "missing_signature", xerrors.Errorf("block with a signature")
	}

	return blk, "", nil
}

// Package mir implements Eudico consensus in Mir framework.
package mir

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var _ consensus.Consensus = &Mir{}

var RewardFunc = func(ctx context.Context, vmi vm.Interface, em stmgr.ExecMonitor,
	epoch abi.ChainEpoch, ts *types.TipSet, params []byte) error {
	// TODO: No RewardFunc implemented for mir yet
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
	netName dtypes.NetworkName,
) (consensus.Consensus, error) {
	return &Mir{
		store:    sm.ChainStore(),
		beacon:   b,
		sm:       sm,
		verifier: v,
		genesis:  g,
	}, nil
}

// CreateBlock creates a Filecoin block from the input block template.
func (bft *Mir) CreateBlock(ctx context.Context, w lapi.Wallet, bt *lapi.BlockTemplate) (*types.FullBlock, error) {
	/*
		b, err := common.PrepareBlockForSignature(ctx, bft.sm, bt)
		if err != nil {
			return nil, err
		}

		// We don't sign blocks mined by Mir validators
	*/
	return nil, nil
}

func (bft *Mir) ValidateBlockHeader(ctx context.Context, b *types.BlockHeader) (rejectReason string, err error) {
	return "", nil
}

func (bft *Mir) ValidateBlock(ctx context.Context, b *types.FullBlock) (err error) {
	log.Infof("starting block validation process at @%d", b.Header.Height)

	/*
		if err := common.BlockSanityChecks(hierarchical.Mir, b.Header); err != nil {
			return xerrors.Errorf("incoming header failed basic sanity checks: %w", err)
		}

		h := b.Header

		baseTs, err := bft.store.LoadTipSet(ctx, types.NewTipSetKey(h.Parents...))
		if err != nil {
			return xerrors.Errorf("load parent tipset failed (%s): %w", h.Parents, err)
		}

		// fast checks first
		if h.Height != baseTs.Height()+1 {
			return xerrors.Errorf("block height not parent height+1: %d != %d", h.Height, baseTs.Height()+1)
		}

		now := uint64(build.Clock.Now().Unix())
		if h.Timestamp > now+build.AllowableClockDriftSecs {
			return xerrors.Errorf("block was from the future (now=%d, blk=%d): %w", now, h.Timestamp, consensus.ErrTemporal)
		}
		if h.Timestamp > now {
			log.Warn("Got block from the future, but within threshold", h.Timestamp, build.Clock.Now().Unix())
		}

		msgsChecks := common.CheckMsgsWithoutBlockSig(ctx, bft.store, bft.sm, bft.subMgr, bft.resolver, bft.netName, b, baseTs)

		minerCheck := async.Err(func() error {
			if err := bft.minerIsValid(b.Header.Miner); err != nil {
				return xerrors.Errorf("minerIsValid failed: %w", err)
			}
			return nil
		})

		pweight, err := Weight(ctx, nil, baseTs)
		if err != nil {
			return xerrors.Errorf("getting parent weight: %w", err)
		}

		if types.BigCmp(pweight, b.Header.ParentWeight) != 0 {
			return xerrors.Errorf("parent weight different: %s (header) != %s (computed)",
				b.Header.ParentWeight, pweight)
		}

		stateRootCheck := common.CheckStateRoot(ctx, bft.store, bft.sm, b, baseTs)

		await := []async.ErrorFuture{
			minerCheck,
			stateRootCheck,
		}

		await = append(await, msgsChecks...)

		var merr error
		for _, fut := range await {
			if err := fut.AwaitContext(ctx); err != nil {
				merr = multierror.Append(merr, err)
			}
		}
		if merr != nil {
			mulErr := merr.(*multierror.Error)
			mulErr.ErrorFormat = func(es []error) string {
				if len(es) == 1 {
					return fmt.Sprintf("1 error occurred:\n\t* %+v\n\n", es[0])
				}

				points := make([]string, len(es))
				for i, err := range es {
					points[i] = fmt.Sprintf("* %+v", err)
				}

				return fmt.Sprintf("%d errors occurred:\n\t%s\n\n", len(es), strings.Join(points, "\n\t"))
			}
			return mulErr
		}

		log.Infof("block at @%d is valid", b.Header.Height)
	*/
	return nil
}

// func (bft *Mir) minerIsValid(maddr address.Address) error {
// 	switch maddr.Protocol() {
// 	case address.BLS:
// 		fallthrough
// 	case address.SECP256K1:
// 		return nil
// 	}
// 	return xerrors.Errorf("miner address must be a key")
// }

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

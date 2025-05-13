package eth

import (
	"context"
	"errors"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var _ TipSetResolver = (*tipSetResolver)(nil)

type F3CertificateProvider interface {
	F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error)
}

type tipSetResolver struct {
	cs               ChainStore
	f3               F3CertificateProvider // can be nil if disabled
	useF3ForFinality bool                  // if true, attempt to use F3 to determine "finalized" tipset
}

func NewTipSetResolver(cs ChainStore, f3 F3CertificateProvider, useF3ForFinality bool) TipSetResolver {
	return &tipSetResolver{cs: cs, f3: f3, useF3ForFinality: useF3ForFinality}
}

func (tsr *tipSetResolver) getLatestF3Cert(ctx context.Context) (*certs.FinalityCertificate, error) {
	if tsr.f3 == nil {
		return nil, nil
	}
	cert, err := tsr.f3.F3GetLatestCertificate(ctx)
	if err != nil {
		if errors.Is(err, f3.ErrF3NotRunning) || errors.Is(err, api.ErrF3NotReady) {
			// Only fall back to EC finality if F3 isn't running or not ready.
			log.Debugw("F3 not running or not ready, falling back to EC finality", "err", err)
			return nil, nil
		}
		return nil, err
	}
	return cert, nil
}

func (tsr *tipSetResolver) getFinalizedF3TipSetFromCert(ctx context.Context, cert *certs.FinalityCertificate) (*types.TipSet, error) {
	// Extract the finalized tipeset from the certificate.
	tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
	if err != nil {
		return nil, xerrors.Errorf("decoding latest f3 cert tipset key: %w", err)
	}
	ts, err := tsr.cs.LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading latest f3 cert tipset: %s: %w", tsk, err)
	}
	return ts, nil
}

func (tsr *tipSetResolver) getSafeF3TipSet(ctx context.Context) (*types.TipSet, error) {
	// To determine the safe tipset, we check the finalized F3 tipset and compare that to the tipset
	// we consider safe from EC and return the higher of the two.
	var f3Ts *types.TipSet
	cert, err := tsr.getLatestF3Cert(ctx)
	if err != nil {
		return nil, err
	} else if cert != nil {
		f3Ts, err = tsr.getFinalizedF3TipSetFromCert(ctx, cert)
		if err != nil {
			return nil, err
		}
	} // else F3 is disabled or not ready
	ecTs, err := tsr.getSafeECTipSet(ctx)
	if err != nil {
		return nil, err
	}
	if f3Ts == nil || f3Ts.Height() < ecTs.Height() {
		return ecTs, nil
	}
	// F3 is finalizing a higher height than EC safe; return F3 tipset
	return f3Ts, nil
}

func (tsr *tipSetResolver) getFinalizedF3TipSet(ctx context.Context) (*types.TipSet, error) {
	cert, err := tsr.getLatestF3Cert(ctx)
	if err != nil {
		return nil, err
	} else if cert == nil {
		// F3 is disabled or not ready; fall back to EC finality.
		return tsr.getFinalizedECTipSet(ctx)
	}
	// Check F3 finalized tipset against the heaviest tipset, and if it is too far
	// behind fall back to EC.
	head := tsr.cs.GetHeaviestTipSet()
	if head == nil {
		return nil, xerrors.Errorf("no known heaviest tipset")
	}
	f3FinalizedHeight := abi.ChainEpoch(cert.ECChain.Head().Epoch)
	if head.Height()-f3FinalizedHeight > policy.ChainFinality {
		log.Debugw("Falling back to EC finalized tipset as the latest F3 finalized tipset is too far behind", "headHeight", head.Height(), "f3FinalizedHeight", f3FinalizedHeight)
		return tsr.getFinalizedECTipSet(ctx)
	}
	// F3 is finalizing a higher height than EC safe; return F3 tipset
	return tsr.getFinalizedF3TipSetFromCert(ctx, cert)
}

func (tsr *tipSetResolver) getSafeECTipSet(ctx context.Context) (*types.TipSet, error) {
	head := tsr.cs.GetHeaviestTipSet()
	height := head.Height()

	// Default safe distance (used for v2 APIs, or v1 APIs when F3 is finalizing)
	safeDistance := buildconstants.SafeHeightDistance

	// For v1 APIs we are still using ethtypes.SafeEpochDelay. This can be removed in the future when
	// F3 is always finalizing if we want v1 "safe" to take advantage of F3.
	if !tsr.useF3ForFinality {
		safeDistance = ethtypes.SafeEpochDelay
	}

	safeHeight := max(0, height-safeDistance)
	return tsr.cs.GetTipsetByHeight(ctx, safeHeight, head, true)
}

func (tsr *tipSetResolver) getFinalizedECTipSet(ctx context.Context) (*types.TipSet, error) {
	head := tsr.cs.GetHeaviestTipSet()
	height := max(0, head.Height()-policy.ChainFinality)
	return tsr.cs.GetTipsetByHeight(ctx, height, head, true)
}

func (tsr *tipSetResolver) getTipSetByTag(ctx context.Context, tag string) (*types.TipSet, error) {
	head := tsr.cs.GetHeaviestTipSet()
	switch tag {
	case ethtypes.BlockTagEarliest:
		return nil, xerrors.Errorf(`block param "%s" is not supported`, ethtypes.BlockTagEarliest)
	case ethtypes.BlockTagPending:
		return head, nil
	case ethtypes.BlockTagLatest:
		parent, err := tsr.cs.GetTipSetFromKey(ctx, head.Parents())
		if err != nil {
			return nil, xerrors.New("failed to get parent tipset")
		}
		return parent, nil
	case ethtypes.BlockTagSafe:
		if tsr.f3 != nil && tsr.useF3ForFinality {
			return tsr.getSafeF3TipSet(ctx)
		}
		return tsr.getSafeECTipSet(ctx)
	case ethtypes.BlockTagFinalized:
		if tsr.f3 != nil && tsr.useF3ForFinality {
			return tsr.getFinalizedF3TipSet(ctx)
		}
		return tsr.getFinalizedECTipSet(ctx)
	default:
		return nil, xerrors.Errorf("unknown block tag: %s", tag)
	}
}

func (tsr *tipSetResolver) GetTipsetByBlockNumber(ctx context.Context, blkParam string, strict bool) (*types.TipSet, error) {
	switch blkParam {
	case ethtypes.BlockTagEarliest, ethtypes.BlockTagPending, ethtypes.BlockTagLatest, ethtypes.BlockTagFinalized, ethtypes.BlockTagSafe:
		return tsr.getTipSetByTag(ctx, blkParam)
	}

	num, err := ethtypes.EthUint64FromString(blkParam)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse block number: %v", err)
	}
	head := tsr.cs.GetHeaviestTipSet()
	if abi.ChainEpoch(num) > head.Height()-1 {
		return nil, xerrors.New("requested a future epoch (beyond 'latest')")
	}
	ts, err := tsr.cs.GetTipsetByHeight(ctx, abi.ChainEpoch(num), head, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset at height: %v", num)
	}
	if strict && ts.Height() != abi.ChainEpoch(num) {
		return nil, api.NewErrNullRound(abi.ChainEpoch(num))
	}
	return ts, nil
}

func (tsr *tipSetResolver) GetTipsetByBlockNumberOrHash(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) (*types.TipSet, error) {
	head := tsr.cs.GetHeaviestTipSet()

	predefined := blkParam.PredefinedBlock
	if predefined != nil {
		return tsr.getTipSetByTag(ctx, *predefined)
	}

	if blkParam.BlockNumber != nil {
		height := abi.ChainEpoch(*blkParam.BlockNumber)
		if height > head.Height()-1 {
			return nil, xerrors.New("requested a future epoch (beyond 'latest')")
		}
		ts, err := tsr.cs.GetTipsetByHeight(ctx, height, head, true)
		if err != nil {
			return nil, xerrors.Errorf("failed to get tipset at height: %v", height)
		}
		return ts, nil
	}

	if blkParam.BlockHash != nil {
		ts, err := tsr.cs.GetTipSetByCid(ctx, blkParam.BlockHash.ToCid())
		if err != nil {
			return nil, xerrors.Errorf("failed to get tipset by hash: %v", err)
		}

		// verify that the tipset is in the canonical chain
		if blkParam.RequireCanonical {
			// walk up the current chain (our head) until we reach ts.Height()
			walkTs, err := tsr.cs.GetTipsetByHeight(ctx, ts.Height(), head, true)
			if err != nil {
				return nil, xerrors.Errorf("failed to get tipset at height: %v", ts.Height())
			}

			// verify that it equals the expected tipset
			if !walkTs.Equals(ts) {
				return nil, xerrors.New("tipset is not canonical")
			}
		}

		return ts, nil
	}

	return nil, xerrors.New("invalid block param")
}

func (tsr *tipSetResolver) GetTipSetByHash(ctx context.Context, blkParam ethtypes.EthHash) (*types.TipSet, error) {
	ts, err := tsr.cs.GetTipSetByCid(ctx, blkParam.ToCid())
	if err != nil {
		// TODO: https://github.com/filecoin-project/lotus/issues/13043
		// if ipld.IsNotFound(err) {
		// 	return nil, nil
		// }
		return nil, xerrors.Errorf("failed to get tipset by hash: %v", err)
	}
	// TODO: remove this
	if ts == nil {
		return nil, xerrors.Errorf("failed to find tipset for block hash: %s", blkParam)
	}
	return ts, nil
}

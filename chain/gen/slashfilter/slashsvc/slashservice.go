package slashsvc

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	levelds "github.com/ipfs/go-ds-leveldb"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("slashsvc")

type ConsensusSlasherApi interface {
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message, spec *lapi.MessageSendSpec) (*types.SignedMessage, error)
	SyncIncomingBlocks(context.Context) (<-chan *types.BlockHeader, error)
	WalletDefaultAddress(context.Context) (address.Address, error)
}

func SlashConsensus(ctx context.Context, a ConsensusSlasherApi, p string, from string) error {
	var fromAddr address.Address

	ds, err := levelds.NewDatastore(p, &levelds.Options{
		Compression: ldbopts.NoCompression,
		NoSync:      false,
		Strict:      ldbopts.StrictAll,
		ReadOnly:    false,
	})
	if err != nil {
		return xerrors.Errorf("open leveldb: %w", err)
	}
	sf := slashfilter.New(ds)
	if from == "" {
		defaddr, err := a.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}
		fromAddr = defaddr
	} else {
		addr, err := address.NewFromString(from)
		if err != nil {
			return err
		}

		fromAddr = addr
	}

	blocks, err := a.SyncIncomingBlocks(ctx)
	if err != nil {
		return xerrors.Errorf("sync incoming blocks failed: %w", err)
	}

	log.Infow("consensus fault reporter", "from", fromAddr)
	go func() {
		for block := range blocks {
			otherBlock, extraBlock, fault, err := slashFilterMinedBlock(ctx, sf, a, block)
			if err != nil {
				if format.IsNotFound(err) {
					log.Debugf("block not found in chain: %s", err)
				} else {
					log.Errorf("slash detector errored: %s", err)
				}
				continue
			}
			if fault {
				log.Errorf("<!!> SLASH FILTER DETECTED FAULT DUE TO BLOCKS %s and %s", otherBlock.Cid(), block.Cid())
				bh1, err := cborutil.Dump(otherBlock)
				if err != nil {
					log.Errorf("could not dump otherblock:%s, err:%s", otherBlock.Cid(), err)
					continue
				}

				bh2, err := cborutil.Dump(block)
				if err != nil {
					log.Errorf("could not dump block:%s, err:%s", block.Cid(), err)
					continue
				}

				params := miner.ReportConsensusFaultParams{
					BlockHeader1: bh1,
					BlockHeader2: bh2,
				}
				if extraBlock != nil {
					be, err := cborutil.Dump(extraBlock)
					if err != nil {
						log.Errorf("could not dump block:%s, err:%s", block.Cid(), err)
						continue
					}
					params.BlockHeaderExtra = be
				}

				enc, err := actors.SerializeParams(&params)
				if err != nil {
					log.Errorf("could not serialize declare faults parameters: %s", err)
					continue
				}
				for {
					head, err := a.ChainHead(ctx)
					if err != nil || head.Height() > block.Height {
						break
					}
					time.Sleep(time.Second * 10)
				}
				message, err := a.MpoolPushMessage(ctx, &types.Message{
					To:     block.Miner,
					From:   fromAddr,
					Value:  types.NewInt(0),
					Method: builtin.MethodsMiner.ReportConsensusFault,
					Params: enc,
				}, nil)
				if err != nil {
					log.Errorf("ReportConsensusFault to messagepool error:%s", err)
					continue
				}
				log.Infof("ReportConsensusFault message CID:%s", message.Cid())

			}
		}
	}()

	return nil
}

func slashFilterMinedBlock(ctx context.Context, sf *slashfilter.SlashFilter, a ConsensusSlasherApi, blockB *types.BlockHeader) (*types.BlockHeader, *types.BlockHeader, bool, error) {
	blockC, err := a.ChainGetBlock(ctx, blockB.Parents[0])
	if err != nil {
		return nil, nil, false, xerrors.Errorf("chain get block error:%s", err)
	}

	blockACid, fault, err := sf.MinedBlock(ctx, blockB, blockC.Height)
	if err != nil {
		return nil, nil, false, xerrors.Errorf("slash filter check block error:%s", err)
	}

	if !fault {
		return nil, nil, false, nil
	}

	blockA, err := a.ChainGetBlock(ctx, blockACid)
	if err != nil {
		return nil, nil, false, xerrors.Errorf("failed to get blockA: %w", err)
	}

	// (a) double-fork mining (2 blocks at one epoch)
	if blockA.Height == blockB.Height {
		return blockA, nil, true, nil
	}

	// (b) time-offset mining faults (2 blocks with the same parents)
	if types.CidArrsEqual(blockB.Parents, blockA.Parents) {
		return blockA, nil, true, nil
	}

	// (c) parent-grinding fault
	// Here extra is the "witness", a third block that shows the connection between A and B as
	// A's sibling and B's parent.
	// Specifically, since A is of lower height, it must be that B was mined omitting A from its tipset
	//
	//      B
	//      |
	//  [A, C]
	if types.CidArrsEqual(blockA.Parents, blockC.Parents) && blockA.Height == blockC.Height &&
		types.CidArrsContains(blockB.Parents, blockC.Cid()) && !types.CidArrsContains(blockB.Parents, blockA.Cid()) {
		return blockA, blockC, true, nil
	}

	log.Error("unexpectedly reached end of slashFilterMinedBlock despite fault being reported!")
	return nil, nil, false, nil
}

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/build"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/types/mock"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

func TestGatewayAPIChainGetTipSetByHeight(t *testing.T) {
	ctx := context.Background()

	lookbackTimestamp := uint64(time.Now().Unix()) - uint64(LookbackCap.Seconds())
	type args struct {
		h         abi.ChainEpoch
		tskh      abi.ChainEpoch
		genesisTS uint64
	}
	tests := []struct {
		name   string
		args   args
		expErr bool
	}{{
		name: "basic",
		args: args{
			h:    abi.ChainEpoch(1),
			tskh: abi.ChainEpoch(5),
		},
	}, {
		name: "genesis",
		args: args{
			h:    abi.ChainEpoch(0),
			tskh: abi.ChainEpoch(5),
		},
	}, {
		name: "same epoch as tipset",
		args: args{
			h:    abi.ChainEpoch(5),
			tskh: abi.ChainEpoch(5),
		},
	}, {
		name: "tipset too old",
		args: args{
			// Tipset height is 5, genesis is at LookbackCap - 10 epochs.
			// So resulting tipset height will be 5 epochs earlier than LookbackCap.
			h:         abi.ChainEpoch(1),
			tskh:      abi.ChainEpoch(5),
			genesisTS: lookbackTimestamp - build.BlockDelaySecs*10,
		},
		expErr: true,
	}, {
		name: "lookup height too old",
		args: args{
			// Tipset height is 5, lookup height is 1, genesis is at LookbackCap - 3 epochs.
			// So
			// - lookup height will be 2 epochs earlier than LookbackCap.
			// - tipset height will be 2 epochs later than LookbackCap.
			h:         abi.ChainEpoch(1),
			tskh:      abi.ChainEpoch(5),
			genesisTS: lookbackTimestamp - build.BlockDelaySecs*3,
		},
		expErr: true,
	}, {
		name: "tipset and lookup height within acceptable range",
		args: args{
			// Tipset height is 5, lookup height is 1, genesis is at LookbackCap.
			// So
			// - lookup height will be 1 epoch later than LookbackCap.
			// - tipset height will be 5 epochs later than LookbackCap.
			h:         abi.ChainEpoch(1),
			tskh:      abi.ChainEpoch(5),
			genesisTS: lookbackTimestamp,
		},
	}}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGatewayDepsAPI{}
			a := &GatewayAPI{api: mock}

			// Create tipsets from genesis up to tskh and return the highest
			ts := mock.createTipSets(tt.args.tskh, tt.args.genesisTS)

			got, err := a.ChainGetTipSetByHeight(ctx, tt.args.h, ts.Key())
			if tt.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.args.h, got.Height())
			}
		})
	}
}

type mockGatewayDepsAPI struct {
	lk      sync.RWMutex
	tipsets []*types.TipSet
}

func (m *mockGatewayDepsAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	return m.tipsets[len(m.tipsets)-1], nil
}

func (m *mockGatewayDepsAPI) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	for _, ts := range m.tipsets {
		if ts.Key() == tsk {
			return ts, nil
		}
	}

	return nil, nil
}

// createTipSets creates tipsets from genesis up to tskh and returns the highest
func (m *mockGatewayDepsAPI) createTipSets(h abi.ChainEpoch, genesisTimestamp uint64) *types.TipSet {
	m.lk.Lock()
	defer m.lk.Unlock()

	targeth := h + 1 // add one for genesis block
	if genesisTimestamp == 0 {
		genesisTimestamp = uint64(time.Now().Unix()) - build.BlockDelaySecs*uint64(targeth)
	}
	var currts *types.TipSet
	for currh := abi.ChainEpoch(0); currh < targeth; currh++ {
		blks := mock.MkBlock(currts, 1, 1)
		if currh == 0 {
			blks.Timestamp = genesisTimestamp
		}
		currts = mock.TipSet(blks)
		m.tipsets = append(m.tipsets, currts)
	}

	return m.tipsets[len(m.tipsets)-1]
}

func (m *mockGatewayDepsAPI) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	return m.tipsets[h], nil
}

func (m *mockGatewayDepsAPI) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) MpoolPushUntrusted(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateWaitMsgLimited(ctx context.Context, msg cid.Cid, confidence uint64, h abi.ChainEpoch) (*api.MsgLookup, error) {
	panic("implement me")
}

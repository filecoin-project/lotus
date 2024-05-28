package sealing

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

func (m *Sealing) ListSectors() ([]SectorInfo, error) {
	var sectors []SectorInfo
	if err := m.sectors.List(&sectors); err != nil {
		return nil, err
	}
	return sectors, nil
}

func (m *Sealing) GetSectorInfo(sid abi.SectorNumber) (SectorInfo, error) {
	var out SectorInfo
	err := m.sectors.Get(uint64(sid)).Get(&out)
	return out, err
}

func collateralSendAmount(ctx context.Context, api interface {
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (big.Int, error)
}, maddr address.Address, cfg sealiface.Config, collateral abi.TokenAmount) (abi.TokenAmount, error) {
	if cfg.CollateralFromMinerBalance {
		if cfg.DisableCollateralFallback {
			return big.Zero(), nil
		}

		avail, err := api.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return big.Zero(), xerrors.Errorf("getting available miner balance: %w", err)
		}

		avail = big.Sub(avail, cfg.AvailableBalanceBuffer)
		if avail.LessThan(big.Zero()) {
			avail = big.Zero()
		}

		collateral = big.Sub(collateral, avail)
		if collateral.LessThan(big.Zero()) {
			collateral = big.Zero()
		}
	}

	return collateral, nil
}

func simulateMsgGas(ctx context.Context, sa interface {
	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
},
	from, to address.Address, method abi.MethodNum, value, maxFee abi.TokenAmount, params []byte) (*types.Message, error) {
	msg := types.Message{
		To:     to,
		From:   from,
		Value:  value,
		Method: method,
		Params: params,
	}

	var b bytes.Buffer
	err := msg.MarshalCBOR(&b)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal the signed message: %w", err)
	}

	gmsg, err := sa.GasEstimateMessageGas(ctx, &msg, nil, types.EmptyTSK)
	if err != nil {
		err = fmt.Errorf("message %x failed: %w", b.Bytes(), err)
	}
	return gmsg, err
}

func sendMsg(ctx context.Context, sa interface {
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
}, from, to address.Address, method abi.MethodNum, value, maxFee abi.TokenAmount, params []byte) (cid.Cid, error) {
	msg := types.Message{
		To:     to,
		From:   from,
		Value:  value,
		Method: method,
		Params: params,
	}

	smsg, err := sa.MpoolPushMessage(ctx, &msg, &api.MessageSendSpec{MaxFee: maxFee})
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

package itests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMigrationNV29SolsticeFaultFeeDebt verifies drain→fault→FeeDebt→top-up→RepayDebt cycle for 10x sector.
func TestMigrationNV29SolsticeFaultFeeDebt(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	balanceOnly := func() string {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		return act.Balance.String()
	}

	ledger := func() (balance, available, feeDebt abi.TokenAmount) {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		var mst stminer.State
		req.NoError(gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client)).Get(ctx, act.Head, &mst))
		avail := big.Subtract(act.Balance, mst.LockedFunds, mst.PreCommitDeposits, mst.InitialPledge, mst.FeeDebt)
		if avail.LessThan(big.Zero()) {
			avail = big.Zero()
		}
		return act.Balance, avail, mst.FeeDebt
	}

	sendFromOwner := func(value abi.TokenAmount, method abi.MethodNum, params []byte) {
		msg, merr := client.MpoolPushMessage(ctx, &types.Message{
			From:   client.DefaultKey.Address,
			To:     maddr,
			Value:  value,
			Method: method,
			Params: params,
		}, nil)
		req.NoError(merr)
		lookup, werr := client.StateWaitMsg(ctx, msg.Cid(), 2, lapi.LookbackNoLimit, true)
		req.NoError(werr)
		req.True(lookup.Receipt.ExitCode.IsSuccess(),
			"message (method %d) must succeed; exit=%d", method, lookup.Receipt.ExitCode)
	}

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	onboarded, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(onboarded, 1)
	um.WaitTillActivatedAndAssertPower(onboarded, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)
	sn := onboarded[0]

	info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(info.Flags&miner.FULL_QA_POWER, "native NV29 CC sector must carry FULL_QA_POWER (10x)")

	_, avail0, debt0 := ledger()
	req.True(debt0.IsZero(), "no fee debt before the fault; got %s", debt0)
	req.True(avail0.GreaterThan(big.Zero()), "miner must start with a positive available balance; got %s", avail0)

	withdrawParams, aerr := actors.SerializeParams(&stminer.WithdrawBalanceParams{AmountRequested: types.FromFil(1000)})
	req.NoError(aerr)
	sendFromOwner(big.Zero(), builtin.MethodsMiner.WithdrawBalance, withdrawParams)
	_, avail1, debt1 := ledger()
	req.True(debt1.IsZero(), "draining must not create fee debt; got %s", debt1)
	req.True(avail1.LessThan(types.NewInt(1e6)),
		"available balance must be drained to ~0 after WithdrawBalance; got %s (balance %s)", avail1, balanceOnly())

	um.DeclareFaults([]abi.SectorNumber{sn})

	endDebt := time.Now().Add(3 * time.Minute)
	var debtAccrued abi.TokenAmount
	for {
		_, _, feeDebt := ledger()
		if feeDebt.GreaterThan(big.Zero()) {
			debtAccrued = feeDebt
			break
		}
		if time.Now().After(endDebt) {
			require.FailNowf(t, "FeeDebt accrual timeout",
				"continued-fault penalty never produced FeeDebt with a drained available balance; balance=%s", balanceOnly())
		}
		h, herr := client.ChainHead(ctx)
		req.NoError(herr)
		client.WaitTillChain(ctx, kit.HeightAtLeast(h.Height()+40))
	}

	faulted, err := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isFaulted, err := faulted.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isFaulted, "the 10x sector must be declared faulty while it accrues FeeDebt")
	stillInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotNil(stillInfo, "the faulted 10x sector must still exist (miner not terminated for debt)")
	t.Logf("FeeDebt accrued on the drained, faulted 10x sector: %s", debtAccrued)

	sendFromOwner(types.FromFil(10), builtin.MethodSend, nil) // plain transfer -> available balance
	sendFromOwner(big.Zero(), builtin.MethodsMiner.RepayDebt, nil)

	_, avail2, debt2 := ledger()
	req.True(debt2.IsZero(), "RepayDebt must clear the FeeDebt back to 0; remaining=%s", debt2)
	req.True(avail2.GreaterThan(big.Zero()), "the miner must hold positive available balance after the top-up")

	// generous top-up covers the continuing fault fee, so FeeDebt stays 0 after one more period.
	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(di.Open+di.WPoStProvingPeriod+10))
	_, _, debt3 := ledger()
	req.True(debt3.IsZero(),
		"after one more proving period FeeDebt must still be 0 (top-up covers the continuing fault fee); remaining=%s", debt3)

	um.AssertNoWindowPostError()
}

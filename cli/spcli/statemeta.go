package spcli

import (
	"github.com/fatih/color"

	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

type StateMeta struct {
	I     int
	Col   color.Attribute
	State sealing.SectorState
}

var StateOrder = map[sealing.SectorState]StateMeta{}
var StateList = []StateMeta{
	{Col: 39, State: "Total"},
	{Col: color.FgGreen, State: sealing.Proving},
	{Col: color.FgGreen, State: sealing.Available},
	{Col: color.FgGreen, State: sealing.UpdateActivating},

	{Col: color.FgMagenta, State: sealing.ReceiveSector},

	{Col: color.FgBlue, State: sealing.Empty},
	{Col: color.FgBlue, State: sealing.WaitDeals},
	{Col: color.FgBlue, State: sealing.AddPiece},
	{Col: color.FgBlue, State: sealing.SnapDealsWaitDeals},
	{Col: color.FgBlue, State: sealing.SnapDealsAddPiece},

	{Col: color.FgRed, State: sealing.UndefinedSectorState},
	{Col: color.FgYellow, State: sealing.Packing},
	{Col: color.FgYellow, State: sealing.GetTicket},
	{Col: color.FgYellow, State: sealing.PreCommit1},
	{Col: color.FgYellow, State: sealing.PreCommit2},
	{Col: color.FgYellow, State: sealing.PreCommitting},
	{Col: color.FgYellow, State: sealing.PreCommitWait},
	{Col: color.FgYellow, State: sealing.SubmitPreCommitBatch},
	{Col: color.FgYellow, State: sealing.PreCommitBatchWait},
	{Col: color.FgYellow, State: sealing.WaitSeed},
	{Col: color.FgYellow, State: sealing.Committing},
	{Col: color.FgYellow, State: sealing.CommitFinalize},
	{Col: color.FgYellow, State: sealing.SubmitCommit},
	{Col: color.FgYellow, State: sealing.CommitWait},
	{Col: color.FgYellow, State: sealing.SubmitCommitAggregate},
	{Col: color.FgYellow, State: sealing.CommitAggregateWait},
	{Col: color.FgYellow, State: sealing.FinalizeSector},
	{Col: color.FgYellow, State: sealing.SnapDealsPacking},
	{Col: color.FgYellow, State: sealing.UpdateReplica},
	{Col: color.FgYellow, State: sealing.ProveReplicaUpdate},
	{Col: color.FgYellow, State: sealing.SubmitReplicaUpdate},
	{Col: color.FgYellow, State: sealing.ReplicaUpdateWait},
	{Col: color.FgYellow, State: sealing.WaitMutable},
	{Col: color.FgYellow, State: sealing.FinalizeReplicaUpdate},
	{Col: color.FgYellow, State: sealing.ReleaseSectorKey},

	{Col: color.FgCyan, State: sealing.Terminating},
	{Col: color.FgCyan, State: sealing.TerminateWait},
	{Col: color.FgCyan, State: sealing.TerminateFinality},
	{Col: color.FgCyan, State: sealing.TerminateFailed},
	{Col: color.FgCyan, State: sealing.Removing},
	{Col: color.FgCyan, State: sealing.Removed},
	{Col: color.FgCyan, State: sealing.AbortUpgrade},

	{Col: color.FgRed, State: sealing.FailedUnrecoverable},
	{Col: color.FgRed, State: sealing.AddPieceFailed},
	{Col: color.FgRed, State: sealing.SealPreCommit1Failed},
	{Col: color.FgRed, State: sealing.SealPreCommit2Failed},
	{Col: color.FgRed, State: sealing.PreCommitFailed},
	{Col: color.FgRed, State: sealing.ComputeProofFailed},
	{Col: color.FgRed, State: sealing.RemoteCommitFailed},
	{Col: color.FgRed, State: sealing.CommitFailed},
	{Col: color.FgRed, State: sealing.CommitFinalizeFailed},
	{Col: color.FgRed, State: sealing.PackingFailed},
	{Col: color.FgRed, State: sealing.FinalizeFailed},
	{Col: color.FgRed, State: sealing.Faulty},
	{Col: color.FgRed, State: sealing.FaultReported},
	{Col: color.FgRed, State: sealing.FaultedFinal},
	{Col: color.FgRed, State: sealing.RemoveFailed},
	{Col: color.FgRed, State: sealing.DealsExpired},
	{Col: color.FgRed, State: sealing.RecoverDealIDs},
	{Col: color.FgRed, State: sealing.SnapDealsAddPieceFailed},
	{Col: color.FgRed, State: sealing.SnapDealsDealsExpired},
	{Col: color.FgRed, State: sealing.ReplicaUpdateFailed},
	{Col: color.FgRed, State: sealing.ReleaseSectorKeyFailed},
	{Col: color.FgRed, State: sealing.FinalizeReplicaUpdateFailed},
}

func init() {
	for i, state := range StateList {
		StateOrder[state.State] = StateMeta{
			I:   i,
			Col: state.Col,
		}
	}
}

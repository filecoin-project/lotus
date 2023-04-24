package migrations

import (
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-ds-versioning/pkg/versioned"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations/maptypes"
)

// NoOpClientDealState0To1 does nothing (old type removed)
func NoOpClientDealState0To1(oldDs *maptypes.ClientDealState1) (*maptypes.ClientDealState1, error) {
	return oldDs, nil
}

// MigrateClientDealState1To2 migrates from v1 to v2 of a ClientDealState.
// The difference is that in v2 the ChannelID is a pointer, because the
// ChannelID is not set until the data transfer has started, so it should
// initially be nil.
func MigrateClientDealState1To2(oldDs *maptypes.ClientDealState1) (*retrievalmarket.ClientDealState, error) {
	var chid *datatransfer.ChannelID
	if oldDs.ChannelID.Initiator != "" && oldDs.ChannelID.Responder != "" {
		chid = &oldDs.ChannelID
	}
	return &retrievalmarket.ClientDealState{
		DealProposal:         oldDs.DealProposal,
		StoreID:              oldDs.StoreID,
		ChannelID:            chid,
		LastPaymentRequested: oldDs.LastPaymentRequested,
		AllBlocksReceived:    oldDs.AllBlocksReceived,
		TotalFunds:           oldDs.TotalFunds,
		ClientWallet:         oldDs.ClientWallet,
		MinerWallet:          oldDs.MinerWallet,
		PaymentInfo:          oldDs.PaymentInfo,
		Status:               oldDs.Status,
		Sender:               oldDs.Sender,
		TotalReceived:        oldDs.TotalReceived,
		Message:              oldDs.Message,
		BytesPaidFor:         oldDs.BytesPaidFor,
		CurrentInterval:      oldDs.CurrentInterval,
		PaymentRequested:     oldDs.PaymentRequested,
		FundsSpent:           oldDs.FundsSpent,
		UnsealFundsPaid:      oldDs.UnsealFundsPaid,
		WaitMsgCID:           oldDs.WaitMsgCID,
		VoucherShortfall:     oldDs.VoucherShortfall,
		LegacyProtocol:       true,
	}, nil
}

// NoOpProviderDealState0To1 does nothing (old type removed)
func NoOpProviderDealState0To1(oldDs *maptypes.ProviderDealState1) (*maptypes.ProviderDealState1, error) {
	return oldDs, nil
}

// MigrateProviderDealState1To2 migrates from v1 to v2 of a
// MigrateProviderDealState.
// The difference is that in v2 the ChannelID is a pointer, because the
// ChannelID is not set until the data transfer has started, so it should
// initially be nil.
func MigrateProviderDealState1To2(oldDs *maptypes.ProviderDealState1) (*retrievalmarket.ProviderDealState, error) {
	var chid *datatransfer.ChannelID
	if oldDs.ChannelID.Initiator != "" && oldDs.ChannelID.Responder != "" {
		chid = &oldDs.ChannelID
	}
	return &retrievalmarket.ProviderDealState{
		DealProposal:  oldDs.DealProposal,
		StoreID:       oldDs.StoreID,
		ChannelID:     chid,
		PieceInfo:     oldDs.PieceInfo,
		Status:        oldDs.Status,
		Receiver:      oldDs.Receiver,
		FundsReceived: oldDs.FundsReceived,
		Message:       oldDs.Message,
	}, nil
}

// ClientMigrations are migrations for the client's store of retrieval deals
var ClientMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(NoOpClientDealState0To1, "1"),
	versioned.NewVersionedBuilder(MigrateClientDealState1To2, "2").OldVersion("1"),
}

// ProviderMigrations are migrations for the providers's store of retrieval deals
var ProviderMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(NoOpProviderDealState0To1, "1").
		FilterKeys([]string{"/retrieval-ask", "/retrieval-ask/latest", "/retrieval-ask/1/latest", "/retrieval-ask/versions/current"}),
	versioned.NewVersionedBuilder(MigrateProviderDealState1To2, "2").OldVersion("1"),
}

// AskMigrations are migrations for the providers's retrieval ask
var AskMigrations = versioned.BuilderList{}

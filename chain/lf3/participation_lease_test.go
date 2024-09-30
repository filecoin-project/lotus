package lf3

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-f3/gpbft"

	"github.com/filecoin-project/lotus/api"
)

func TestLeaser(t *testing.T) {
	nodeID := peer.ID("peerID")
	progress := mockProgress{currentInstance: 10}
	subject := newParticipationLeaser(nodeID, progress.Progress, 5)

	t.Run("participate", func(t *testing.T) {
		ticket, err := subject.getOrRenewParticipationTicket(123, nil, 5)
		require.NoError(t, err)

		lease, err := subject.participate(ticket)
		require.NoError(t, err)
		require.Equal(t, uint64(123), lease.MinerID)
		require.Equal(t, nodeID, lease.Issuer)
		require.Equal(t, uint64(5), lease.ValidityTerm) // Current instance (10) + offset (5)
	})
	t.Run("get participants", func(t *testing.T) {
		progress.currentInstance = 11
		ticket1, err := subject.getOrRenewParticipationTicket(123, nil, 4)
		require.NoError(t, err)
		ticket2, err := subject.getOrRenewParticipationTicket(456, nil, 5)
		require.NoError(t, err)

		_, err = subject.participate(ticket1)
		require.NoError(t, err)
		_, err = subject.participate(ticket2)
		require.NoError(t, err)

		// Both participants should still be valid.
		participants := subject.getParticipantsByInstance(11)
		require.Len(t, participants, 2)
		require.Contains(t, participants, uint64(123))
		require.Contains(t, participants, uint64(456))

		// After instance 16, only participant 456 should be valid.
		participants = subject.getParticipantsByInstance(16)
		require.Len(t, participants, 1)
		require.Contains(t, participants, uint64(456))

		// After instance 17, no participant must have a lease.
		participants = subject.getParticipantsByInstance(17)
		require.Empty(t, participants)
	})
	t.Run("expired ticket", func(t *testing.T) {
		ticket, err := subject.getOrRenewParticipationTicket(123, nil, 5)
		require.NoError(t, err)

		progress.currentInstance += 10
		lease, err := subject.participate(ticket)
		require.ErrorIs(t, err, api.ErrF3ParticipationTicketExpired)
		require.Zero(t, lease)
	})
	t.Run("too many instances", func(t *testing.T) {
		ticket, err := subject.getOrRenewParticipationTicket(123, nil, 6)
		require.Error(t, err, api.ErrF3ParticipationTooManyInstances)
		require.Nil(t, ticket)

		// Generate a token from the same subject but with higher term, then assert that
		// original subject with lower term rejects it.
		subjectSpoofWithHigherMaxLease := newParticipationLeaser(nodeID, progress.Progress, 6)
		ticket, err = subjectSpoofWithHigherMaxLease.getOrRenewParticipationTicket(123, nil, 6)
		require.NoError(t, err)
		require.NotEmpty(t, ticket)
		lease, err := subject.participate(ticket)
		require.ErrorIs(t, err, api.ErrF3ParticipationTooManyInstances)
		require.Zero(t, lease)

	})
	t.Run("invalid ticket", func(t *testing.T) {
		lease, err := subject.participate([]byte("ghoti"))
		require.ErrorIs(t, err, api.ErrF3ParticipationTicketInvalid)
		require.Zero(t, lease)
	})
	t.Run("issuer mismatch", func(t *testing.T) {
		anotherIssuer := newParticipationLeaser("barreleye", progress.Progress, 5)
		ticket, err := anotherIssuer.getOrRenewParticipationTicket(123, nil, 5)
		require.NoError(t, err)
		lease, err := subject.participate(ticket)
		require.ErrorIs(t, err, api.ErrF3ParticipationIssuerMismatch)
		require.Zero(t, lease)
	})
	t.Run("expired previous ticket", func(t *testing.T) {
		previous, err := subject.getOrRenewParticipationTicket(123, nil, 5)
		require.NoError(t, err)

		// Get or renew without progress
		newTicket, err := subject.getOrRenewParticipationTicket(123, previous, 5)
		require.NoError(t, err)
		require.NotNil(t, newTicket)
		require.Equal(t, previous, newTicket)

		// Get or renew with overlapping validity progress
		progress.currentInstance += 3
		newTicket, err = subject.getOrRenewParticipationTicket(123, previous, 5)
		require.NoError(t, err)
		require.NotNil(t, newTicket)
		require.NotEqual(t, previous, newTicket)

		// Get or renew with expired previous
		progress.currentInstance += 10
		newTicket, err = subject.getOrRenewParticipationTicket(123, previous, 5)
		require.NoError(t, err)
		require.NotNil(t, newTicket)
		require.NotEqual(t, previous, newTicket)

		// Get or renew with valid but mismatching issuer
		progress.currentInstance -= 10
		anotherIssuer := newParticipationLeaser("barreleye", progress.Progress, 5)
		newTicket, err = anotherIssuer.getOrRenewParticipationTicket(123, previous, 5)
		require.ErrorIs(t, err, api.ErrF3ParticipationIssuerMismatch)
		require.Empty(t, newTicket)

		// Get or renew with expired but mismatching issuer
		progress.currentInstance += 10
		newTicket, err = anotherIssuer.getOrRenewParticipationTicket(123, previous, 5)
		require.NoError(t, err)
		require.NotNil(t, newTicket)
		require.NotEqual(t, previous, newTicket)

		// Get or renew with expired but mismatching session
		progress.currentInstance -= 10
		subjectAtNewSession := newParticipationLeaser(nodeID, progress.Progress, 5)
		newTicket, err = subjectAtNewSession.getOrRenewParticipationTicket(123, previous, 5)
		require.NoError(t, err)
		require.NotNil(t, newTicket)
		require.NotEqual(t, previous, newTicket)
	})
}

type mockProgress struct{ currentInstance uint64 }

func (m *mockProgress) Progress() (uint64, uint64, gpbft.Phase) {
	return m.currentInstance, 0, gpbft.INITIAL_PHASE
}

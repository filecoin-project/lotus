package lf3

import (
	"bytes"
	"errors"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
)

type f3Status = func() (manifest.Manifest, gpbft.InstanceProgress)

type leaser struct {
	mutex                sync.Mutex
	leases               map[uint64]api.F3ParticipationLease
	issuer               string // issuer is the base58 encoding of the node peer ID.
	status               f3Status
	maxLeasableInstances uint64
	// Signals that a lease was created and/or updated.
	notifyParticipation chan struct{}
}

func newParticipationLeaser(nodeId peer.ID, status f3Status, maxLeasedInstances uint64) *leaser {
	return &leaser{
		leases:               make(map[uint64]api.F3ParticipationLease),
		issuer:               nodeId.String(),
		status:               status,
		notifyParticipation:  make(chan struct{}, 1),
		maxLeasableInstances: maxLeasedInstances,
	}
}

func (l *leaser) getOrRenewParticipationTicket(participant uint64, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error) {

	if instances == 0 {
		return nil, errors.New("not enough instances")
	}

	if instances > l.maxLeasableInstances {
		return nil, api.ErrF3ParticipationTooManyInstances
	}

	manifest, instant := l.status()
	currentInstance := instant.ID
	if len(previous) != 0 {
		// A previous ticket is present. To avoid overlapping lease across multiple
		// instances for the same participant check its validity and only proceed to
		// issue a new ticket if:
		//   - either it is expired/invalid, or
		//   - it is valid and was issued by this node.
		//
		// Otherwise, return ErrF3ParticipationIssuerMismatch to signal to the caller the need for retry.
		switch _, err := l.validateTicket(manifest.NetworkName, currentInstance, previous); {
		case errors.Is(err, api.ErrF3ParticipationTicketInvalid):
			// Invalid ticket means the miner must have got the ticket from a node with a potentially different version.
			// Refuse to issue a new ticket in case there is some other node with active lease for the miner.
			return nil, err
		case errors.Is(err, api.ErrF3ParticipationTicketExpired):
			// The current instance is beyond the validity term of the previous lease. It is
			// safe to proceed to issuing a ticket from current instance onwards for the term
			// asked for.
		case errors.Is(err, api.ErrF3ParticipationIssuerMismatch):
			// The previous ticket is still valid and is not issued by this node; return error.
			return nil, err
		case errors.Is(err, api.ErrF3ParticipationTooManyInstances):
			// We don't care if the previous lease was for too many instances. What we care
			// about is that the new ticket is within the max which was checked right at the
			// top.
		case err != nil:
			log.Errorw("Unexpected error occurred while validating previous participation ticket", "participant", participant, "err", err)
			return nil, err
		default:
			// The previous ticket was issued by this node and is still valid. It is safe to
			// proceed with issuing a new ticket with overlapping validity.
		}
		log.Debugw("Renewing previously issued participation ticket with overlapping lease", "participant", participant, "startInstance", currentInstance, "validFor", instances)
	}

	return l.newParticipationTicket(manifest.NetworkName, participant, currentInstance, instances)
}

func (l *leaser) participate(ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	manifest, instant := l.status()
	newLease, err := l.validateTicket(manifest.NetworkName, instant.ID, ticket)
	if err != nil {
		return api.F3ParticipationLease{}, err
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	currentLease, found := l.leases[newLease.MinerID]
	if found {
		// short-circuite for reparticipation.
		if currentLease == newLease {
			newLease.ValidityTerm = newLease.ToInstance() - instant.ID
			newLease.FromInstance = instant.ID
			return newLease, nil
		}
		if currentLease.Network == newLease.Network && currentLease.FromInstance > newLease.FromInstance {
			// For safety, strictly require lease start instance to never decrease.
			return api.F3ParticipationLease{}, api.ErrF3ParticipationTicketStartBeforeExisting
		}
	} else {
		log.Infof("started participating in F3 for miner %d", newLease.MinerID)
	}
	l.leases[newLease.MinerID] = newLease
	select {
	case l.notifyParticipation <- struct{}{}:
	default:
	}
	newLease.ValidityTerm = newLease.ToInstance() - instant.ID
	newLease.FromInstance = instant.ID
	return newLease, nil
}

func (l *leaser) getParticipantsByInstance(network gpbft.NetworkName, instance uint64) []uint64 {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	currentManifest, _ := l.status()
	currentNetwork := currentManifest.NetworkName
	if currentNetwork != network {
		return nil
	}
	var participants []uint64
	for id, lease := range l.leases {
		if _, err := l.validateLease(currentNetwork, instance, lease); err != nil {
			// Lazily clear old leases.
			log.Warnf("lost F3 participation lease for miner %d at instance %d since it is no longer valid: %v ", id, instance, err)
			delete(l.leases, id)
		} else {
			participants = append(participants, id)
		}
	}
	return participants
}

func (l *leaser) getValidLeases() []api.F3ParticipationLease {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	currentManifest, progress := l.status()
	var leases []api.F3ParticipationLease
	for id, lease := range l.leases {
		// Lazily clear old leases.
		if validatedLease, err := l.validateLease(currentManifest.NetworkName, progress.ID, lease); err != nil {
			log.Warnf("lost F3 participation lease for miner %d at instance %d while getting valid leases since it is no longer valid: %v ", id, progress.ID, err)
			delete(l.leases, id)
		} else {
			leases = append(leases, validatedLease)
		}
	}
	return leases
}

func (l *leaser) newParticipationTicket(nn gpbft.NetworkName, participant uint64, from uint64, instances uint64) (api.F3ParticipationTicket, error) {
	// Lotus node API and miners run in a trusted environment. For now we make the
	// ticket to simply be the CBOR encoding of the lease. In the future, where the
	// assumptions of trust may no longer hold, ticket could be encrypted and
	// decrypted at the time of issuing the actual lease.
	var buf bytes.Buffer
	if err := (&api.F3ParticipationLease{
		Network:      nn,
		Issuer:       l.issuer,
		MinerID:      participant,
		FromInstance: from,
		ValidityTerm: instances,
	}).MarshalCBOR(&buf); err != nil {
		return nil, xerrors.Errorf("issuing participation ticket: %w", err)
	}
	return buf.Bytes(), nil
}

func (l *leaser) validateTicket(currentNetwork gpbft.NetworkName, currentInstance uint64, t api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	var lease api.F3ParticipationLease
	reader := bytes.NewReader(t)
	if err := lease.UnmarshalCBOR(reader); err != nil {
		return api.F3ParticipationLease{}, api.ErrF3ParticipationTicketInvalid
	}
	return l.validateLease(currentNetwork, currentInstance, lease)
}

func (l *leaser) validateLease(currentNetwork gpbft.NetworkName, currentInstance uint64, lease api.F3ParticipationLease) (api.F3ParticipationLease, error) {
	// Combine the errors to remove significance of the order by which they are
	// checked outside if this function.
	var err error
	if currentNetwork != lease.Network || currentInstance > lease.ToInstance() {
		err = multierr.Append(err, api.ErrF3ParticipationTicketExpired)
	}
	if l.issuer != lease.Issuer {
		err = multierr.Append(err, api.ErrF3ParticipationIssuerMismatch)
	}
	if lease.ValidityTerm > l.maxLeasableInstances {
		err = multierr.Append(err, api.ErrF3ParticipationTooManyInstances)
	}

	if err != nil {
		return api.F3ParticipationLease{}, err
	}
	return lease, nil
}

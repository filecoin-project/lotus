package fifo

import (
	"github.com/ipfs/go-cid"

	mirrequest "github.com/filecoin-project/mir/pkg/pb/requestpb"
)

// Pool is a structure to implement the simplest pool that enforces FIFO policy on client messages.
// When a client sends a message we add clientID to orderingClients map and clientByCID.
// When we receive a message we find the clientID and remove it from orderingClients.
// We don't need using sync primitives since the pool's methods are called only by one goroutine.
type Pool struct {
	clientByCID     map[cid.Cid]string // messageCID -> clientID
	orderingClients map[string]bool    // clientID -> bool
}

func New() *Pool {
	return &Pool{
		clientByCID:     make(map[cid.Cid]string),
		orderingClients: make(map[string]bool),
	}
}

// AddRequest adds the request if it satisfies to the FIFO policy.
func (p *Pool) AddRequest(cid cid.Cid, r *mirrequest.Request) (exist bool) {
	_, exist = p.orderingClients[r.ClientId]
	if !exist {
		p.clientByCID[cid] = r.ClientId
		p.orderingClients[r.ClientId] = true
	}
	return
}

// IsTargetRequest returns whether the request with clientID should be sent or there is a request from that client that
// is in progress of ordering.
func (p *Pool) IsTargetRequest(clientID string) bool {
	_, inProgress := p.orderingClients[clientID]
	return !inProgress
}

// DeleteRequest deletes the target request by the key h.
func (p *Pool) DeleteRequest(cid cid.Cid) (ok bool) {
	clientID, ok := p.clientByCID[cid]
	if ok {
		delete(p.orderingClients, clientID)
		delete(p.clientByCID, cid)
		return
	}
	return
}

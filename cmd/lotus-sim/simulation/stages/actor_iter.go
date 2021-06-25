package stages

import (
	"math/rand"

	"github.com/filecoin-project/go-address"
)

// actorIter is a simple persistent iterator that loops over a set of actors.
type actorIter struct {
	actors []address.Address
	offset int
}

// shuffle randomly permutes the set of actors.
func (p *actorIter) shuffle() {
	rand.Shuffle(len(p.actors), func(i, j int) {
		p.actors[i], p.actors[j] = p.actors[j], p.actors[i]
	})
}

// next returns the next actor's address and advances the iterator.
func (p *actorIter) next() address.Address {
	next := p.actors[p.offset]
	p.offset++
	p.offset %= len(p.actors)
	return next
}

// add adds a new actor to the iterator.
func (p *actorIter) add(addr address.Address) {
	p.actors = append(p.actors, addr)
}

// len returns the number of actors in the iterator.
func (p *actorIter) len() int {
	return len(p.actors)
}

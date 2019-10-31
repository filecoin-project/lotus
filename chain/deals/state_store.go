package deals

import (
	"bytes"
	"github.com/filecoin-project/lotus/lib/statestore"

	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/ipfs/go-cid"
)

type MinerStateStore struct {
	*statestore.StateStore
}

func (st *MinerStateStore) MutateMiner(i cid.Cid, mutator func(*MinerDeal) error) error {
	return st.Mutate(i, minerMutator(mutator))
}

func minerMutator(m func(*MinerDeal) error) func([]byte) ([]byte, error) {
	return func(in []byte) ([]byte, error) {
		deal := new(MinerDeal)
		err := cborrpc.ReadCborRPC(bytes.NewReader(in), deal)
		if err != nil {
			return nil, err
		}

		if err := m(deal); err != nil {
			return nil, err
		}

		return cborrpc.Dump(deal)
	}
}

type ClientStateStore struct {
	*statestore.StateStore
}

func (st *ClientStateStore) MutateClient(i cid.Cid, mutator func(*ClientDeal) error) error {
	return st.Mutate(i, clientMutator(mutator))
}

func clientMutator(m func(*ClientDeal) error) func([]byte) ([]byte, error) {
	return func(in []byte) ([]byte, error) {
		deal := new(ClientDeal)
		err := cborrpc.ReadCborRPC(bytes.NewReader(in), deal)
		if err != nil {
			return nil, err
		}

		if err := m(deal); err != nil {
			return nil, err
		}

		return cborrpc.Dump(deal)
	}
}

func (st *ClientStateStore) ListClient() ([]ClientDeal, error) {
	var out []ClientDeal

	l, err := st.List()
	if err != nil {
		return nil, err
	}
	for _, res := range l {
		var deal ClientDeal
		err := cborrpc.ReadCborRPC(bytes.NewReader(res.Value), &deal)
		if err != nil {
			return nil, err
		}

		out = append(out, deal)
	}

	return out, nil
}

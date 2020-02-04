package storage

import (
	"testing"

	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/go-address"

	s2 "github.com/filecoin-project/go-storage-miner"
	"github.com/filecoin-project/lotus/storage/sbmock"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
)

func TestFastPledge(t *testing.T) {
	sz := uint64(16 << 20)

	ds := datastore.NewMapDatastore()

	s := s2.NewSealing(nil, sbmock.NewMockSectorBuilder(0, sz), ds, address.Undef, address.Undef)
	if _, err := s.FastPledgeCommitment(sectorbuilder.UserBytesForSectorSize(sz), 5); err != nil {
		t.Fatalf("%+v", err)
	}

	sz = uint64(1024)

	s = s2.NewSealing(nil, sbmock.NewMockSectorBuilder(0, sz), ds, address.Undef, address.Undef)
	if _, err := s.FastPledgeCommitment(sectorbuilder.UserBytesForSectorSize(sz), 64); err != nil {
		t.Fatalf("%+v", err)
	}
}

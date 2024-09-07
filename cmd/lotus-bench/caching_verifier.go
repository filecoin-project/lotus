package main

import (
	"bufio"
	"context"
	"errors"

	"github.com/ipfs/go-datastore"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/crypto/blake2b"

	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/lib/must"
)

type cachingVerifier struct {
	ds      datastore.Datastore
	backend proofs.Verifier
}

const bufsize = 128

func (cv cachingVerifier) withCache(execute func() (bool, error), param cbg.CBORMarshaler) (bool, error) {
	hasher := must.One(blake2b.New256(nil))
	wr := bufio.NewWriterSize(hasher, bufsize)
	err := param.MarshalCBOR(wr)
	if err != nil {
		log.Errorf("could not marshal call info: %+v", err)
		return execute()
	}
	err = wr.Flush()
	if err != nil {
		log.Errorf("could not flush: %+v", err)
		return execute()
	}
	hash := hasher.Sum(nil)
	key := datastore.NewKey(string(hash))
	fromDs, err := cv.ds.Get(context.Background(), key)
	if err == nil {
		switch fromDs[0] {
		case 's':
			return true, nil
		case 'f':
			return false, nil
		case 'e':
			return false, errors.New(string(fromDs[1:]))
		default:
			log.Errorf("bad cached result in cache %s(%x)", fromDs[0], fromDs[0])
			return execute()
		}
	} else if errors.Is(err, datastore.ErrNotFound) {
		// recalc
		ok, err := execute()
		var save []byte
		if err != nil {
			if ok {
				log.Errorf("success with an error: %+v", err)
			} else {
				save = append([]byte{'e'}, []byte(err.Error())...)
			}
		} else if ok {
			save = []byte{'s'}
		} else {
			save = []byte{'f'}
		}

		if len(save) != 0 {
			errSave := cv.ds.Put(context.Background(), key, save)
			if errSave != nil {
				log.Errorf("error saving result: %+v", errSave)
			}
		}

		return ok, err
	} else {
		log.Errorf("could not get data from cache: %+v", err)
		return execute()
	}
}

func (cv *cachingVerifier) VerifySeal(svi prooftypes.SealVerifyInfo) (bool, error) {
	return cv.withCache(func() (bool, error) {
		return cv.backend.VerifySeal(svi)
	}, &svi)
}

func (cv *cachingVerifier) VerifyWinningPoSt(ctx context.Context, info prooftypes.WinningPoStVerifyInfo) (bool, error) {
	return cv.backend.VerifyWinningPoSt(ctx, info)
}

func (cv *cachingVerifier) VerifyWindowPoSt(ctx context.Context, info prooftypes.WindowPoStVerifyInfo) (bool, error) {
	return cv.withCache(func() (bool, error) {
		return cv.backend.VerifyWindowPoSt(ctx, info)
	}, &info)
}
func (cv *cachingVerifier) GenerateWinningPoStSectorChallenge(ctx context.Context, proofType abi.RegisteredPoStProof, a abi.ActorID, rnd abi.PoStRandomness, u uint64) ([]uint64, error) {
	return cv.backend.GenerateWinningPoStSectorChallenge(ctx, proofType, a, rnd, u)
}

func (cv cachingVerifier) VerifyAggregateSeals(aggregate prooftypes.AggregateSealVerifyProofAndInfos) (bool, error) {
	return cv.backend.VerifyAggregateSeals(aggregate)
}

func (cv cachingVerifier) VerifyReplicaUpdate(update prooftypes.ReplicaUpdateInfo) (bool, error) {
	return cv.backend.VerifyReplicaUpdate(update)
}

var _ proofs.Verifier = (*cachingVerifier)(nil)

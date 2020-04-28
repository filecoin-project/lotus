package main

import (
	"context"
	"encoding/binary"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	pb "github.com/filecoin-project/lotus/cmd/crand/pb"
	lru "github.com/hashicorp/golang-lru"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type server struct {
	p   Params
	pub ffi.PublicKey

	cache *lru.ARCCache
	pb.UnimplementedCrandServer
}

func (s *server) GetRandomness(_ context.Context, rq *pb.RandomnessRequest) (*pb.RandomnessReply, error) {
	reqRound := rq.Round
	curRound := uint64(time.Since(s.p.GenesisTime)/s.p.Round.D()) + 1
	if reqRound == 0 {
		reqRound = curRound
	}
	log.Infof("current round: %d, reqRound: %d", curRound, reqRound)
	if reqRound > curRound {
		return nil, status.Errorf(codes.Unavailable, "randomenss is part of the future")
	}
	if v, ok := s.cache.Get(reqRound); ok {
		return &pb.RandomnessReply{Randomness: v.([]byte)}, nil
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, reqRound)

	log.Infow("signing", "round", reqRound)
	sig := ffi.PrivateKeySign(s.p.Priv, buf)
	s.cache.Add(reqRound, sig[:])
	return &pb.RandomnessReply{Randomness: sig[:], Round: reqRound}, nil
}

func (s *server) GetInfo(_ context.Context, _ *pb.InfoRequest) (*pb.InfoReply, error) {
	return &pb.InfoReply{Pubkey: s.pub[:], GenesisTs: s.p.GenesisTime.Unix(), Round: int64(s.p.Round)}, nil
}

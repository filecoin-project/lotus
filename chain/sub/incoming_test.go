// stm: #unit
package sub

import (
	"bytes"
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/announce/message"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/mocks"
	"github.com/filecoin-project/lotus/chain/types"
)

type getter struct {
	msgs []*types.Message
}

func (g *getter) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) { panic("NYI") }

func (g *getter) GetBlocks(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	ch := make(chan blocks.Block, len(g.msgs))
	for _, m := range g.msgs {
		by, err := m.Serialize()
		if err != nil {
			panic(err)
		}
		b, err := blocks.NewBlockWithCid(by, m.Cid())
		if err != nil {
			panic(err)
		}
		ch <- b
	}
	close(ch)
	return ch
}

func TestFetchCidsWithDedup(t *testing.T) {
	msgs := []*types.Message{}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, &types.Message{
			From: address.TestAddress,
			To:   address.TestAddress,

			Nonce: uint64(i),
		})
	}
	cids := []cid.Cid{}
	for _, m := range msgs {
		cids = append(cids, m.Cid())
	}
	g := &getter{msgs}

	//stm: @CHAIN_INCOMING_FETCH_MESSAGES_BY_CID_001
	// the cids have a duplicate
	res, err := FetchMessagesByCids(context.TODO(), g, append(cids, cids[0]))

	t.Logf("err: %+v", err)
	t.Logf("res: %+v", res)
	if err == nil {
		t.Errorf("there should be an error")
	}
	if err == nil && (res[0] == nil || res[len(res)-1] == nil) {
		t.Fatalf("there is a nil message: first %p, last %p", res[0], res[len(res)-1])
	}
}

func TestIndexerMessageValidator_Validate(t *testing.T) {
	validCid, err := cid.Decode("QmbpDgg5kRLDgMxS8vPKNFXEcA6D5MC4CkuUdSWDVtHPGK")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name           string
		selfPID        string
		senderPID      string
		extraData      []byte
		wantValidation pubsub.ValidationResult
	}{
		{
			name:           "invalid extra data is rejected",
			selfPID:        "12D3KooWQiCbqEStCkdqUvr69gQsrp9urYJZUCkzsQXia7mbqbFW",
			senderPID:      "12D3KooWE8yt84RVwW3sFcd6WMjbUdWrZer2YtT4dmtj3dHdahSZ",
			extraData:      []byte("f0127896"), // note, casting encoded address to byte is invalid.
			wantValidation: pubsub.ValidationReject,
		},
		{
			name:           "same sender and receiver is ignored",
			selfPID:        "12D3KooWQiCbqEStCkdqUvr69gQsrp9urYJZUCkzsQXia7mbqbFW",
			senderPID:      "12D3KooWQiCbqEStCkdqUvr69gQsrp9urYJZUCkzsQXia7mbqbFW",
			wantValidation: pubsub.ValidationIgnore,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mc := gomock.NewController(t)
			node := mocks.NewMockFullNode(mc)
			subject := NewIndexerMessageValidator(peer.ID(tc.selfPID), node, node)
			message := message.Message{
				Cid:       validCid,
				Addrs:     nil,
				ExtraData: tc.extraData,
			}
			buf := bytes.NewBuffer(nil)
			if err := message.MarshalCBOR(buf); err != nil {
				t.Fatal(err)
			}

			topic := "topic"
			pbm := &pb.Message{
				Data:  buf.Bytes(),
				Topic: &topic,
				From:  nil,
				Seqno: nil,
			}
			validate := subject.Validate(context.Background(), peer.ID(tc.senderPID), &pubsub.Message{
				Message:       pbm,
				ReceivedFrom:  peer.ID(tc.senderPID),
				ValidatorData: nil,
			})

			if validate != tc.wantValidation {
				t.Fatalf("expected %v but got %v", tc.wantValidation, validate)
			}
		})
	}
}

func TestIdxValidator(t *testing.T) {
	validCid, err := cid.Decode("QmbpDgg5kRLDgMxS8vPKNFXEcA6D5MC4CkuUdSWDVtHPGK")
	if err != nil {
		t.Fatal(err)
	}

	addr, err := address.NewFromString("f01024")
	if err != nil {
		t.Fatal(err)
	}

	buf1, err := addr.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	selfPID := "12D3KooWQiCbqEStCkdqUvr69gQsrp9urYJZUCkzsQXia7mbqbFW"
	senderPID := "12D3KooWE8yt84RVwW3sFcd6WMjbUdWrZer2YtT4dmtj3dHdahSZ"
	extraData := buf1

	mc := gomock.NewController(t)
	node := mocks.NewMockFullNode(mc)
	node.EXPECT().ChainHead(gomock.Any()).Return(nil, nil).AnyTimes()

	subject := NewIndexerMessageValidator(peer.ID(selfPID), node, node)
	message := message.Message{
		Cid:       validCid,
		Addrs:     nil,
		ExtraData: extraData,
	}
	buf := bytes.NewBuffer(nil)
	if err := message.MarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	topic := "topic"

	privk, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(privk.GetPublic())
	if err != nil {
		t.Fatal(err)
	}

	node.EXPECT().StateMinerInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(api.MinerInfo{PeerId: &id}, nil).AnyTimes()

	pbm := &pb.Message{
		Data:  buf.Bytes(),
		Topic: &topic,
		From:  []byte(id),
		Seqno: []byte{1, 1, 1, 1, 2, 2, 2, 2},
	}
	validate := subject.Validate(context.Background(), peer.ID(senderPID), &pubsub.Message{
		Message:       pbm,
		ReceivedFrom:  peer.ID("f01024"), // peer.ID(senderPID),
		ValidatorData: nil,
	})
	if validate != pubsub.ValidationAccept {
		t.Error("Expected to receive ValidationAccept")
	}
	msgInfo, cached := subject.peerCache.Get(addr)
	if !cached {
		t.Fatal("Message info should be in cache")
	}
	seqno := msgInfo.lastSeqno
	msgInfo.rateLimit = nil // prevent interference from rate limiting

	t.Log("Sending DoS msg")
	privk, _, err = crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := peer.IDFromPublicKey(privk.GetPublic())
	if err != nil {
		t.Fatal(err)
	}
	pbm = &pb.Message{
		Data:  buf.Bytes(),
		Topic: &topic,
		From:  []byte(id2),
		Seqno: []byte{255, 255, 255, 255, 255, 255, 255, 255},
	}
	validate = subject.Validate(context.Background(), peer.ID(senderPID), &pubsub.Message{
		Message:       pbm,
		ReceivedFrom:  peer.ID(senderPID),
		ValidatorData: nil,
	})
	if validate != pubsub.ValidationReject {
		t.Error("Expected to get ValidationReject")
	}
	msgInfo, cached = subject.peerCache.Get(addr)
	if !cached {
		t.Fatal("Message info should be in cache")
	}
	msgInfo.rateLimit = nil // prevent interference from rate limiting

	// Check if DoS is possible.
	if msgInfo.lastSeqno != seqno {
		t.Fatal("Sequence number should not have been updated")
	}

	t.Log("Sending another valid message from miner...")
	pbm = &pb.Message{
		Data:  buf.Bytes(),
		Topic: &topic,
		From:  []byte(id),
		Seqno: []byte{1, 1, 1, 1, 2, 2, 2, 3},
	}
	validate = subject.Validate(context.Background(), peer.ID(senderPID), &pubsub.Message{
		Message:       pbm,
		ReceivedFrom:  peer.ID("f01024"), // peer.ID(senderPID),
		ValidatorData: nil,
	})
	if validate != pubsub.ValidationAccept {
		t.Fatal("Did not receive ValidationAccept")
	}
}

package hello

import (
	"bufio"
	"context"
	"github.com/filecoin-project/go-lotus/cborrpc"
	"github.com/libp2p/go-libp2p-core/host"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
)

const ProtocolID = "/fil/hello/1.0.0"

var log = logging.Logger("hello")

func init() {
	cbor.RegisterCborType(Message{})
}

type Message struct {
	HeaviestTipSet       []cid.Cid
	HeaviestTipSetWeight uint64
	GenesisHash          cid.Cid
}

type NewStreamFunc func(context.Context, peer.ID, ...protocol.ID) (inet.Stream, error)

type Service struct {
	newStream NewStreamFunc

	//cs        *ChainStore
	//syncer    *Syncer
}

func NewHelloService(h host.Host) *Service {
	return &Service{
		newStream: h.NewStream,
	}
}

func (hs *Service) HandleStream(s inet.Stream) {
	defer s.Close()

	log.Debugw("Handling hello")

	var hmsg Message
	if err := cborrpc.ReadCborRPC(bufio.NewReader(s), &hmsg); err != nil {
		log.Error("failed to read hello message: ", err)
		return
	}
	log.Debugw("heaviest tipset", "tipset", hmsg.HeaviestTipSet)
	log.Debugw("got genesis from hello", "hash", hmsg.GenesisHash)

	/*if hmsg.GenesisHash != hs.syncer.genesis.Cids()[0] {
		log.Error("other peer has different genesis!")
		s.Conn().Close()
		return
	}

	ts, err := hs.syncer.FetchTipSet(context.Background(), s.Conn().RemotePeer(), hmsg.HeaviestTipSet)
	if err != nil {
		log.Errorf("failed to fetch tipset from peer during hello: %s", err)
		return
	}

	hs.syncer.InformNewHead(s.Conn().RemotePeer(), ts)*/
}

func (hs *Service) SayHello(ctx context.Context, pid peer.ID) error {
	/*s, err := hs.newStream(ctx, pid, ProtocolID)
	if err != nil {
		return err
	}

	hts := hs.cs.GetHeaviestTipSet()
	weight := hs.cs.Weight(hts)
	gen, err := hs.cs.GetGenesis()
	if err != nil {
		return err
	}

	hmsg := &Message{
		HeaviestTipSet:       hts.Cids(),
		HeaviestTipSetWeight: weight,
		GenesisHash:          gen.Cid(),
	}
	fmt.Println("SENDING HELLO MESSAGE: ", hts.Cids())
	fmt.Println("hello message genesis: ", gen.Cid())

	if err := WriteCborRPC(s, hmsg); err != nil {
		return err
	}*/

	return nil
}

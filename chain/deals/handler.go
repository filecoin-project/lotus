package deals

import (
	"context"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"math"

	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"

	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() {
	cbor.RegisterCborType(MinerDeal{})
}

type MinerDeal struct {
	Client      peer.ID
	Proposal    StorageDealProposal
	ProposalCid cid.Cid
	State       DealState

	Ref cid.Cid
}

type Handler struct {
	w  *wallet.Wallet
	sb *sectorbuilder.SectorBuilder

	// TODO: Use a custom protocol or graphsync in the future
	// TODO: GC
	dag dtypes.StagingDAG

	deals StateStore

	incoming chan MinerDeal

	actor address.Address

	stop    chan struct{}
	stopped chan struct{}
}

type fetchResult struct {
	id  cid.Cid
	err error
}

func NewHandler(w *wallet.Wallet, ds dtypes.MetadataDS, sb *sectorbuilder.SectorBuilder, dag dtypes.StagingDAG) (*Handler, error) {
	addr, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return nil, err
	}
	minerAddress, err := address.NewFromBytes(addr)
	if err != nil {
		return nil, err
	}

	return &Handler{
		w:   w,
		sb:  sb,
		dag: dag,

		incoming: make(chan MinerDeal),

		actor: minerAddress,

		deals: StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))},
	}, nil
}

func (h *Handler) Run(ctx context.Context) {
	go func() {
		defer log.Error("quitting deal handler loop")
		defer close(h.stopped)
		fetched := make(chan fetchResult)

		for {
			select {
			case deal := <-h.incoming:
				log.Info("incoming deal")

				if err := h.deals.Begin(deal.ProposalCid, deal); err != nil {
					// TODO: This can happen when client re-sends proposal
					log.Errorf("deal tracking failed: %s", err)
					continue
				}

				go func(id cid.Cid) {
					log.Info("fetching data for a deal")
					err := merkledag.FetchGraph(ctx, deal.Ref, h.dag)
					select {
					case fetched <- fetchResult{
						id:  id,
						err: err,
					}:
					case <-h.stop:
					}
				}(deal.ProposalCid)
			case result := <-fetched:
				if result.err != nil {
					log.Errorf("failed to fetch data for deal: %s", result.err)
					// TODO: fail deal
				}

				// TODO: send response if client still there
				// TODO: staging

				// TODO: async
				log.Info("sealing deal")

				var deal MinerDeal
				err := h.deals.MutateMiner(result.id, func(in MinerDeal) (MinerDeal, error) {
					in.State = Sealing
					deal = in
					return in, nil
				})
				if err != nil {
					// TODO: fail deal
					log.Errorf("deal tracking failed (set sealing): %s", err)
					continue
				}

				root, err := h.dag.Get(ctx, deal.Ref)
				if err != nil {
					// TODO: fail deal
					log.Errorf("failed to get file root for deal: %s", err)
					continue
				}

				// TODO: abstract this away into ReadSizeCloser + implement different modes
				n, err := unixfile.NewUnixfsFile(ctx, h.dag, root)
				if err != nil {
					// TODO: fail deal
					log.Errorf("cannot open unixfs file: %s", err)
					continue
				}

				f, ok := n.(files.File)
				if !ok {
					// TODO: we probably got directory, how should we handle this in unixfs mode?
					log.Errorf("unsupported unixfs type")
					// TODO: fail deal
					continue
				}

				size, err := f.Size()
				if err != nil {
					log.Errorf("failed to get file size: %s", err)
					// TODO: fail deal
					continue
				}

				// TODO: can we use pipes?
				sectorID, err := h.sb.AddPiece(ctx, deal.Proposal.PieceRef, uint64(size), f)
				if err != nil {
					// TODO: fail deal
					log.Errorf("AddPiece failed: %s", err)
					continue
				}

				log.Warnf("New Sector: %d", sectorID)

				// TODO: update state, tell client
			case <-h.stop:
				return
			}
		}
	}()
}

func (h *Handler) HandleStream(s inet.Stream) {
	defer s.Close()

	log.Info("Handling storage deal proposal!")

	var proposal SignedStorageDealProposal
	if err := cborrpc.ReadCborRPC(s, &proposal); err != nil {
		log.Errorw("failed to read proposal message", "error", err)
		return
	}

	// TODO: Validate proposal maybe
	// (and signature, obviously)

	if proposal.Proposal.MinerAddress != h.actor {
		log.Errorf("proposal with wrong MinerAddress: %s", proposal.Proposal.MinerAddress)
		// TODO: send error
		return
	}

	switch proposal.Proposal.SerializationMode {
	//case SerializationRaw:
	//case SerializationIPLD:
	case SerializationUnixFs:
	default:
		log.Errorf("deal proposal with unsupported serialization: %s", proposal.Proposal.SerializationMode)
		// TODO: send error
		return
	}

	// TODO: Review: Not signed?
	proposalNd, err := cbor.WrapObject(proposal.Proposal, math.MaxUint64, -1)
	if err != nil {
		log.Error(err)
		return
	}

	response := StorageDealResponse{
		State:    Accepted,
		Message:  "",
		Proposal: proposalNd.Cid(),
	}

	msg, err := cbor.DumpObject(response)
	if err != nil {
		log.Errorw("failed to serialize response message", "error", err)
		return
	}

	def, err := h.w.ListAddrs()
	if err != nil {
		log.Error(err)
		return
	}
	if len(def) != 1 {
		// NOTE: If this ever happens for a good reason, implement this with GetWorker on the miner actor
		// TODO: implement with GetWorker on the miner actor
		log.Errorf("Expected only 1 address in wallet, got %d", len(def))
		return
	}

	sig, err := h.w.Sign(def[0], msg)
	if err != nil {
		log.Errorw("failed to sign response message", "error", err)
		return
	}

	log.Info("accepting deal")

	signedResponse := &SignedStorageDealResponse{
		Response:  response,
		Signature: sig,
	}
	if err := cborrpc.WriteCborRPC(s, signedResponse); err != nil {
		log.Errorw("failed to write deal response", "error", err)
		return
	}

	ref, err := cid.Parse(proposal.Proposal.PieceRef)
	if err != nil {
		log.Error(err)
		return
	}

	log.Info("processing deal")

	h.incoming <- MinerDeal{
		Client:      s.Conn().RemotePeer(),
		Proposal:    proposal.Proposal,
		ProposalCid: proposalNd.Cid(),
		State:       Accepted,

		Ref: ref,
	}
}

func (h *Handler) Stop() {
	close(h.stop)
	<-h.stopped
}

package deals

import (
	"context"
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

	stop    chan struct{}
	stopped chan struct{}
}

func NewHandler(w *wallet.Wallet, ds dtypes.MetadataDS, sb *sectorbuilder.SectorBuilder, dag dtypes.StagingDAG) *Handler {
	return &Handler{
		w:   w,
		dag: dag,

		deals: StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))},
	}
}

func (h *Handler) Run(ctx context.Context) {
	go func() {
		defer close(h.stopped)
		fetched := make(chan cid.Cid)

		for {
			select {
			case deal := <-h.incoming:
				log.Info("incoming deal")

				if err := h.deals.Begin(deal.ProposalCid, deal); err != nil {
					// TODO: This can happen when client re-sends proposal
					log.Errorf("deal tracking failed: %s", err)
					return
				}

				go func(id cid.Cid) {
					err := merkledag.FetchGraph(ctx, deal.Ref, h.dag)
					if err != nil {
						return
					}

					select {
					case fetched <- id:
					case <-h.stop:
					}
				}(deal.ProposalCid)
			case id := <-fetched:
				// TODO: send response if client still there
				// TODO: staging

				// TODO: async
				log.Info("sealing deal")

				var deal MinerDeal
				err := h.deals.MutateMiner(id, func(in MinerDeal) (MinerDeal, error) {
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
					return
				}

				// TODO: abstract this away into ReadSizeCloser + implement different modes
				n, err := unixfile.NewUnixfsFile(ctx, h.dag, root)
				if err != nil {
					// TODO: fail deal
					log.Errorf("cannot open unixfs file: %s", err)
					return
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
					return
				}

				// TODO: can we use pipes?
				sectorID, err := h.sb.AddPiece(ctx, deal.Proposal.PieceRef, uint64(size), f)
				if err != nil {
					// TODO: fail deal
					log.Errorf("AddPiece failed: %s", err)
					return
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
	sig, err := h.w.Sign(proposal.Proposal.MinerAddress, msg)
	if err != nil {
		log.Errorw("failed to sign response message", "error", err)
		return
	}

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
		return
	}

	h.incoming <- MinerDeal{
		Client:      s.Conn().RemotePeer(),
		Proposal:    proposal.Proposal,
		ProposalCid: proposalNd.Cid(),
		State:       Accepted,

		Ref: ref,
	}
}

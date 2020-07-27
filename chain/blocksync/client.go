package blocksync

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"time"

	host "github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	incrt "github.com/filecoin-project/lotus/lib/increadtimeout"
	"github.com/filecoin-project/lotus/lib/peermgr"
)

// Protocol client.
// FIXME: Rename to just `Client`. Not done at the moment to avoid
//  disrupt too much of the consumer code, should be done along
//  https://github.com/filecoin-project/lotus/issues/2612.
type BlockSync struct {
	// Connection manager used to contact the server.
	// FIXME: We should have a reduced interface here, initialized
	//  just with our protocol ID, we shouldn't be able to open *any*
	//  connection.
	host  host.Host

	peerTracker *bsPeerTracker
}

func NewClient(
	host host.Host,
	pmgr peermgr.MaybePeerMgr,
) *BlockSync {
	return &BlockSync{
		host:        host,
		peerTracker: newPeerTracker(pmgr.Mgr),
	}
}

// Main logic of the client request service. The provided `Request`
// is sent to the `singlePeer` if one is indicated or to all available
// ones otherwise. The response is processed and validated according
// to the `Request` options. Either a `ValidatedResponse` is returned
// (which can be safely accessed), or an `error` that may represent
// either a response error status, a failed validation or an internal
// error.
//
// This is the internal single-point-of-entry for all external-facing
// APIs, currently we have 3 very heterogeneous services exposed:
// * GetBlocks:         Headers
// * GetFullTipSet:     Headers | Messages
// * GetChainMessages:            Messages
// This function handles all the different combinations of the available
// request options without disrupting external calls. In the future the
// consumers should be forced to use a more standardized service and
// adhere to a single API derived from this function.
func (client *BlockSync) doRequest(
	ctx context.Context,
	req *Request,
	singlePeer *peer.ID,
) (*ValidatedResponse, error) {
		// Validate request.
		if req.Length == 0 {
			return nil, xerrors.Errorf("invalid request of length 0")
		}
		if req.Length > MaxRequestLength {
			return nil, xerrors.Errorf("request length (%d) above maximum (%d)",
				req.Length, MaxRequestLength)
		}
		if req.Options == 0 {
			return nil, xerrors.Errorf("request with no options set")
		}

		// Generate the list of peers to be queried, either the
		// `singlePeer` indicated or all peers available (sorted
		// by an internal peer tracker with some randomness injected).
	var peers []peer.ID
	if singlePeer != nil {
		peers = []peer.ID{*singlePeer}
	} else {
		peers = client.getShuffledPeers()
		if len(peers) == 0 {
			return nil, xerrors.Errorf("no peers available")
		}
	}

	// Try the request for each peer in the list,
	// return on the first successful response.
	// FIXME: Doing this serially isn't great, but fetching in parallel
	//  may not be a good idea either. Think about this more.
	startTime := build.Clock.Now()
	// FIXME: Should we track time per peer instead of a global one?
	for _, peer := range peers {
		select {
		case <-ctx.Done():
			return nil, xerrors.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		// Send request, read response.
		res, err := client.sendRequestToPeer(ctx, peer, req)
		if err != nil {
			if !xerrors.Is(err, inet.ErrNoConn) {
				log.Warnf("could not connect to peer %s: %s",
					peer.String(), err)
			}
			continue
		}

		// Process and validate response.
		validRes, err := client.processResponse(req, res)
		if err != nil {
			log.Warnf("processing peer %s response failed: %s",
				peer.String(), err)
			continue
		}

		client.peerTracker.logGlobalSuccess(build.Clock.Since(startTime))
		client.host.ConnManager().TagPeer(peer, "bsync", SUCCESS_PEER_TAG_VALUE)
		return validRes, nil
	}

	errString := "doRequest failed for all peers"
	if singlePeer != nil {
		errString = "doRequest failed for single peer"
		// (The peer has already been logged before, don't print it again.)
	}
	return nil, xerrors.Errorf(errString)
}

// Process and validate response. Check the status and that the information
// returned matches the request (and its integrity). Extract the information
// into a `ValidatedResponse` for the external-facing APIs to select what they
// want.
//
// We are conflating in the single error returned both status and validation
// errors. Peer penalization should happen here then, before returning, so
// we can apply the correct penalties depending on the cause of the error.
func (client *BlockSync) processResponse(
	req *Request,
	res *Response,
	// FIXME: Add the `peer` as argument once we implement penalties.
) (*ValidatedResponse, error) {
	err := res.statusToError()
	if err != nil {
		return nil, xerrors.Errorf("status error: %s", err)
	}

	options := parseOptions(req.Options)
	if options.noOptionsSet() {
		// Safety check, this shouldn't happen, and even if it did
		// it should be caught by the peer in its error status.
		return nil, xerrors.Errorf("nothing was requested")
	}

	// Verify that the chain segment returned is in the valid range.
	// Note that the returned length might be less than requested.
	resLength := len(res.Chain)
	if resLength == 0 {
		return nil, xerrors.Errorf("got no chain in successful response")
	}
	if resLength > int(req.Length) {
		return nil, xerrors.Errorf("got longer response (%d) than requested (%d)",
			resLength, req.Length)
	}
	if resLength < int(req.Length) && res.Status != Partial {
		return nil, xerrors.Errorf("got less than requested without a proper status: %s", res.Status)
	}

	validRes := &ValidatedResponse{}
	if options.IncludeHeaders {
		// Check for valid block sets and extract them into `TipSet`s.
		validRes.Tipsets = make([]*types.TipSet, resLength)
		for i := 0; i < resLength; i++ {
			validRes.Tipsets[i], err = types.NewTipSet(res.Chain[i].Blocks)
			if err != nil {
				return nil, xerrors.Errorf("invalid tipset blocks at height (head - %d): %w", i, err)
			}
		}

		// Check that the returned head matches the one requested.
		if !types.CidArrsEqual(validRes.Tipsets[0].Cids(), req.Head) {
			return nil, xerrors.Errorf("returned chain head does not match request")
		}

		// Check `TipSet` are connected (valid chain).
		for i := 0; i < len(validRes.Tipsets) - 1; i++ {
			if validRes.Tipsets[i].IsChildOf(validRes.Tipsets[i+1]) == false {
				return nil, fmt.Errorf("tipsets are not connected at height (head - %d)/(head - %d)",
					i, i+1)
				// FIXME: Maybe give more information here, like CIDs.
			}
		}
	}

	if options.IncludeMessages {
		validRes.Messages = make([]*CompactedMessages, resLength)
		for i := 0; i < resLength; i++ {
			if res.Chain[i].Messages == nil {
				return nil, xerrors.Errorf("no messages included for tipset at height (head - %d): %w", i)
			}
			validRes.Messages[i] = res.Chain[i].Messages
		}

		if options.IncludeHeaders {
			// If the headers were also returned check that the compression
			// indexes are valid before `toFullTipSets()` is called by the
			// consumer.
			for tipsetIdx := 0; tipsetIdx < resLength; tipsetIdx++ {
				msgs := res.Chain[tipsetIdx].Messages
				blocksNum := len(res.Chain[tipsetIdx].Blocks)
				if len(msgs.BlsIncludes) != blocksNum {
					return nil, xerrors.Errorf("BlsIncludes (%d) does not match number of blocks (%d)",
						len(msgs.BlsIncludes), blocksNum)
				}
				if len(msgs.SecpkIncludes) != blocksNum {
					return nil, xerrors.Errorf("SecpkIncludes (%d) does not match number of blocks (%d)",
						len(msgs.SecpkIncludes), blocksNum)
				}
				for blockIdx := 0; blockIdx < blocksNum; blockIdx++ {
					for _, mi := range msgs.BlsIncludes[blockIdx] {
						if int(mi) >= len(msgs.Bls) {
							return nil, xerrors.Errorf("index in BlsIncludes (%d) exceeds number of messages (%d)",
								mi, len(msgs.Bls))
						}
					}
					for _, mi := range msgs.SecpkIncludes[blockIdx] {
						if int(mi) >= len(msgs.Secpk) {
							return nil, xerrors.Errorf("index in SecpkIncludes (%d) exceeds number of messages (%d)",
								mi, len(msgs.Secpk))
						}
					}
				}
			}
		}
	}

	return validRes, nil
}

// GetBlocks fetches count blocks from the network, from the provided tipset
// *backwards*, returning as many tipsets as count.
//
// {hint/usage}: This is used by the Syncer during normal chain syncing and when
// resolving forks.
func (client *BlockSync) GetBlocks(
	ctx context.Context,
	tsk types.TipSetKey,
	count int,
) ([]*types.TipSet, error) {
	ctx, span := trace.StartSpan(ctx, "bsync.GetBlocks")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("tipset", fmt.Sprint(tsk.Cids())),
			trace.Int64Attribute("count", int64(count)),
		)
	}

	req := &Request{
		Head:    tsk.Cids(),
		Length:  uint64(count),
		Options: Headers,
	}

	validRes, err := client.doRequest(ctx, req, nil)
	if err != nil {
		return nil, err
	}

	return validRes.Tipsets, nil
}

func (client *BlockSync) GetFullTipSet(
	ctx context.Context,
	peer peer.ID,
	tsk types.TipSetKey,
) (*store.FullTipSet, error) {
	// TODO: round robin through these peers on error

	req := &Request{
		Head:    tsk.Cids(),
		Length:  1,
		Options: Headers | Messages,
	}

	validRes, err := client.doRequest(ctx, req, &peer)
	if err != nil {
		return nil, err
	}

	return validRes.toFullTipSets()[0], nil
	// If `doRequest` didn't fail we are guaranteed to have at least
	//  *one* tipset here, so it's safe to index directly.
}

func (client *BlockSync) GetChainMessages(
	ctx context.Context,
	head *types.TipSet,
	length uint64,
	) ([]*CompactedMessages, error) {
	ctx, span := trace.StartSpan(ctx, "GetChainMessages")
	defer span.End()

	req := &Request{
		Head:    head.Cids(),
		Length:  length,
		Options: Messages,
	}

	validRes, err := client.doRequest(ctx, req, nil)
	if err != nil {
		return nil, err
	}

	return validRes.Messages, nil
}

// Send a request to a peer. Write request in the stream and read the
// response back. We do not do any processing of the request/response
// here.
func (client *BlockSync) sendRequestToPeer(
	ctx context.Context,
	peer peer.ID,
	req *Request,
) (_ *Response, err error) {
	// Trace code.
	ctx, span := trace.StartSpan(ctx, "sendRequestToPeer")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("peer", peer.Pretty()),
		)
	}
	defer func() {
		if err != nil {
			if span.IsRecordingEvents() {
				span.SetStatus(trace.Status{
					Code:    5,
					Message: err.Error(),
				})
			}
		}
	}()
	// -- TRACE --

	supported, err := client.host.Peerstore().SupportsProtocols(peer, BlockSyncProtocolID)
	if err != nil {
		return nil, xerrors.Errorf("failed to get protocols for peer: %w", err)
	}
	if len(supported) == 0 || supported[0] != BlockSyncProtocolID {
		return nil, xerrors.Errorf("peer %s does not support protocol %s",
			peer, BlockSyncProtocolID)
		// FIXME: `ProtoBook` should support a *single* protocol check that returns
		//  a bool instead of a list.
	}

	connectionStart := build.Clock.Now()

	// Open stream to peer.
	stream, err := client.host.NewStream(
		inet.WithNoDial(ctx, "should already have connection"),
		peer,
		BlockSyncProtocolID)
	if err != nil {
		client.RemovePeer(peer)
		return nil, xerrors.Errorf("failed to open stream to peer: %w", err)
	}

	// Write request.
	_ = stream.SetWriteDeadline(time.Now().Add(WRITE_REQ_DEADLINE))
	if err := cborutil.WriteCborRPC(stream, req); err != nil {
		_ = stream.SetWriteDeadline(time.Time{})
		// FIXME: What's the point of setting a blank deadline that won't time out?
		//  Is this our way of clearing the old one?
		client.peerTracker.logFailure(peer, build.Clock.Since(connectionStart))
		return nil, err
	}
	// FIXME: Same, why are we doing this again here?
	_ = stream.SetWriteDeadline(time.Time{})

	// Read response.
	var res Response
	err = cborutil.ReadCborRPC(
		// FIXME: Extract constants.
		bufio.NewReader(incrt.New(stream, READ_RES_MIN_SPEED, READ_RES_DEADLINE)),
		&res)
	if err != nil {
		client.peerTracker.logFailure(peer, build.Clock.Since(connectionStart))
		return nil, err
	}

	// FIXME: Move all this together at the top using a defer as done elsewhere.
	//  Maybe we need to declare `res` in the signature.
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.Int64Attribute("resp_status", int64(res.Status)),
			trace.StringAttribute("msg", res.ErrorMessage),
			trace.Int64Attribute("chain_len", int64(len(res.Chain))),
		)
	}

	client.peerTracker.logSuccess(peer, build.Clock.Since(connectionStart))
	return &res, nil
}

func (client *BlockSync) AddPeer(p peer.ID) {
	client.peerTracker.addPeer(p)
}

func (client *BlockSync) RemovePeer(p peer.ID) {
	client.peerTracker.removePeer(p)
}

// getShuffledPeers returns a preference-sorted set of peers (by latency
// and failure counting), shuffling the first few peers so we don't always
// pick the same peer.
// FIXME: Merge with the shuffle if we *always* do it.
func (client *BlockSync) getShuffledPeers() []peer.ID {
	peers := client.peerTracker.prefSortedPeers()
	shufflePrefix(peers)
	return peers
}

func shufflePrefix(peers []peer.ID) {
	prefix := SHUFFLE_PEERS_PREFIX
	if len(peers) < prefix {
		prefix = len(peers)
	}

	buf := make([]peer.ID, prefix)
	perm := rand.Perm(prefix)
	for i, v := range perm {
		buf[i] = peers[v]
	}

	copy(peers, buf)
}

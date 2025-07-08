package exchange

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.opencensus.io/trace"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	incrt "github.com/filecoin-project/lotus/lib/increadtimeout"
	"github.com/filecoin-project/lotus/lib/peermgr"
)

// Set the max exchange message size to 120MiB. Purely based on gas numbers, we can include ~8MiB of
// messages per block, so I've set this to 120MiB to be _very_ safe.
const maxExchangeMessageSize = (15 * 8) << 20

// client implements exchange.Client, using the libp2p ChainExchange protocol
// as the fetching mechanism.
type client struct {
	// Connection manager used to contact the server.
	// FIXME: We should have a reduced interface here, initialized
	//  just with our protocol ID, we shouldn't be able to open *any*
	//  connection.
	host host.Host

	peerTracker *bsPeerTracker
}

var _ Client = (*client)(nil)

// NewClient creates a new libp2p-based exchange.Client that uses the libp2p
// ChainExhange protocol as the fetching mechanism.
func NewClient(lc fx.Lifecycle, host host.Host, pmgr peermgr.MaybePeerMgr) Client {
	return &client{
		host:        host,
		peerTracker: newPeerTracker(lc, host, pmgr.Mgr),
	}
}

// Main logic of the client request service. The provided `Request`
// is sent to the `singlePeer` if one is indicated or to all available
// ones otherwise. The response is processed and validated according
// to the `Request` options. Either a `validatedResponse` is returned
// (which can be safely accessed), or an `error` that may represent
// either a response error status, a failed validation or an internal
// error.
//
// This is the internal single point of entry for all external-facing
// APIs, currently we have 3 very heterogeneous services exposed:
// * GetBlocks:         Headers
// * GetFullTipSet:     Headers | Messages
// * GetChainMessages:            Messages
// This function handles all the different combinations of the available
// request options without disrupting external calls. In the future the
// consumers should be forced to use a more standardized service and
// adhere to a single API derived from this function.
func (c *client) doRequest(
	ctx context.Context,
	req *Request,
	singlePeer *peer.ID,
	// In the `GetChainMessages` case, we won't request the headers but we still
	// need them to check the integrity of the `CompactedMessages` in the response
	// so the tipset blocks need to be provided by the caller.
	tipsets []*types.TipSet,
) (*validatedResponse, error) {
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
		peers = c.getShuffledPeers()
		if len(peers) == 0 {
			return nil, xerrors.Errorf("no peers available")
		}
	}

	// Try the request for each peer in the list,
	// return on the first successful response.
	// FIXME: Doing this serially isn't great, but fetching in parallel
	//  may not be a good idea either. Think about this more.
	globalTime := build.Clock.Now()
	// Global time used to track what is the expected time we will need to get
	// a response if a client fails us.
	for _, peer := range peers {
		select {
		case <-ctx.Done():
			return nil, xerrors.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		// Send request, read response.
		res, err := c.sendRequestToPeer(ctx, peer, req)
		if err != nil {
			if !errors.Is(err, network.ErrNoConn) {
				log.Warnf("could not send request to peer %s: %s",
					peer.String(), err)
			}
			continue
		}

		// Process and validate response.
		validRes, err := c.processResponse(req, res, tipsets)
		if err != nil {
			log.Debugf("processing peer %s response failed: %s",
				peer.String(), err)
			continue
		}

		c.peerTracker.logGlobalSuccess(build.Clock.Since(globalTime))
		c.host.ConnManager().TagPeer(peer, "bsync", SuccessPeerTagValue)
		return validRes, nil
	}

	errString := "doRequest failed for all peers"
	if singlePeer != nil {
		errString = fmt.Sprintf("doRequest failed for single peer %s", *singlePeer)
	}
	return nil, xerrors.Errorf(errString)
}

// Process and validate response. Check the status, the integrity of the
// information returned, and that it matches the request. Extract the information
// into a `validatedResponse` for the external-facing APIs to select what they
// need.
//
// We are conflating in the single error returned both status and validation
// errors. Peer penalization should happen here then, before returning, so
// we can apply the correct penalties depending on the cause of the error.
// FIXME: Add the `peer` as argument once we implement penalties.
func (c *client) processResponse(req *Request, res *Response, tipsets []*types.TipSet) (r *validatedResponse, err error) {
	err = res.statusToError()
	if err != nil {
		return nil, xerrors.Errorf("status error: %s", err)
	}

	defer func() {
		if rerr := recover(); rerr != nil {
			log.Errorf("process response error: %s", rerr)
			err = xerrors.Errorf("process response error: %s", rerr)
			return
		}
	}()

	options := parseOptions(req.Options)
	if options.noOptionsSet() {
		// Safety check: this shouldn't have been sent, and even if it did
		// it should have been caught by the peer in its error status.
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
		return nil, xerrors.Errorf("got less than requested without a proper status: %d", res.Status)
	}

	validRes := &validatedResponse{}
	if options.IncludeHeaders {
		// Check for valid block sets and extract them into `TipSet`s.
		validRes.tipsets = make([]*types.TipSet, resLength)
		for i := 0; i < resLength; i++ {
			if res.Chain[i] == nil {
				return nil, xerrors.Errorf("response with nil tipset in pos %d", i)
			}
			for blockIdx, block := range res.Chain[i].Blocks {
				if block == nil {
					return nil, xerrors.Errorf("tipset with nil block in pos %d", blockIdx)
					// FIXME: Maybe we should move this check to `NewTipSet`.
				}
			}

			validRes.tipsets[i], err = types.NewTipSet(res.Chain[i].Blocks)
			if err != nil {
				return nil, xerrors.Errorf("invalid tipset blocks at height (head - %d): %w", i, err)
			}
		}

		// Check that the returned head matches the one requested.
		if !types.CidArrsEqual(validRes.tipsets[0].Cids(), req.Head) {
			return nil, xerrors.Errorf("returned chain head does not match request")
		}

		// Check `TipSet`s are connected (valid chain).
		for i := 0; i < len(validRes.tipsets)-1; i++ {
			if validRes.tipsets[i].IsChildOf(validRes.tipsets[i+1]) == false {
				return nil, fmt.Errorf("tipsets are not connected at height (head - %d)/(head - %d)",
					i, i+1)
				// FIXME: Maybe give more information here, like CIDs.
			}
		}
	}

	if options.IncludeMessages {
		validRes.messages = make([]*CompactedMessages, resLength)
		for i := 0; i < resLength; i++ {
			if res.Chain[i].Messages == nil {
				return nil, xerrors.Errorf("no messages included for tipset at height (head - %d)", i)
			}
			validRes.messages[i] = res.Chain[i].Messages
		}

		if options.IncludeHeaders {
			// If the headers were also returned check that the compression
			// indexes are valid before `toFullTipSets()` is called by the
			// consumer.
			err := c.validateCompressedIndices(res.Chain)
			if err != nil {
				return nil, err
			}
		} else {
			// If we didn't request the headers they should have been provided
			// by the caller.
			if len(tipsets) < len(res.Chain) {
				return nil, xerrors.Errorf("not enough tipsets provided for message response validation, needed %d, have %d", len(res.Chain), len(tipsets))
			}
			chain := make([]*BSTipSet, 0, resLength)
			for i, resChain := range res.Chain {
				next := &BSTipSet{
					Blocks:   tipsets[i].Blocks(),
					Messages: resChain.Messages,
				}
				chain = append(chain, next)
			}

			err := c.validateCompressedIndices(chain)
			if err != nil {
				return nil, err
			}
		}
	}

	return validRes, nil
}

func (c *client) validateCompressedIndices(chain []*BSTipSet) error {
	resLength := len(chain)
	for tipsetIdx := 0; tipsetIdx < resLength; tipsetIdx++ {
		msgs := chain[tipsetIdx].Messages
		blocksNum := len(chain[tipsetIdx].Blocks)

		if len(msgs.BlsIncludes) != blocksNum {
			return xerrors.Errorf("BlsIncludes (%d) does not match number of blocks (%d)",
				len(msgs.BlsIncludes), blocksNum)
		}

		if len(msgs.SecpkIncludes) != blocksNum {
			return xerrors.Errorf("SecpkIncludes (%d) does not match number of blocks (%d)",
				len(msgs.SecpkIncludes), blocksNum)
		}

		blsLen := uint64(len(msgs.Bls))
		secpLen := uint64(len(msgs.Secpk))
		for blockIdx := 0; blockIdx < blocksNum; blockIdx++ {
			for _, mi := range msgs.BlsIncludes[blockIdx] {
				if mi >= blsLen {
					return xerrors.Errorf("index in BlsIncludes (%d) exceeds number of messages (%d)",
						mi, len(msgs.Bls))
				}
			}

			for _, mi := range msgs.SecpkIncludes[blockIdx] {
				if mi >= secpLen {
					return xerrors.Errorf("index in SecpkIncludes (%d) exceeds number of messages (%d)",
						mi, len(msgs.Secpk))
				}
			}
		}
	}

	return nil
}

// GetBlocks implements Client.GetBlocks(). Refer to the godocs there.
func (c *client) GetBlocks(ctx context.Context, tsk types.TipSetKey, count int) ([]*types.TipSet, error) {
	ctx, span := trace.StartSpan(ctx, "bsync.GetBlocks")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("tipset", fmt.Sprint(tsk.Cids())),
			trace.Int64Attribute("count", int64(count)),
		)
	}

	var ret []*types.TipSet
	start := tsk.Cids()
	for len(ret) < count {
		req := &Request{
			Head:    start,
			Length:  uint64(count - len(ret)),
			Options: Headers,
		}

		validRes, err := c.doRequest(ctx, req, nil, nil)
		if err != nil {
			return nil, xerrors.Errorf("failed to doRequest: %w", err)
		}

		if len(validRes.tipsets) == 0 {
			return nil, xerrors.Errorf("doRequest fetched zero tipsets: %w", err)
		}

		ret = append(ret, validRes.tipsets...)

		last := validRes.tipsets[len(validRes.tipsets)-1]
		if last.Height() <= 1 {
			// we've walked all the way up to genesis, return
			break
		}

		start = last.Parents().Cids()
	}

	return ret, nil
}

// GetFullTipSet implements Client.GetFullTipSet(). Refer to the godocs there.
func (c *client) GetFullTipSet(ctx context.Context, peer peer.ID, tsk types.TipSetKey) (*store.FullTipSet, error) {
	// TODO: round robin through these peers on error

	req := &Request{
		Head:    tsk.Cids(),
		Length:  1,
		Options: Headers | Messages,
	}

	validRes, err := c.doRequest(ctx, req, &peer, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to doRequest: %w", err)
	}

	fullTipsets := validRes.toFullTipSets()

	if len(fullTipsets) == 0 {
		return nil, xerrors.New("unexpectedly got no tipsets in exchange")
	}

	return fullTipsets[0], nil
}

// GetChainMessages implements Client.GetChainMessages(). Refer to the godocs there.
func (c *client) GetChainMessages(ctx context.Context, tipsets []*types.TipSet) ([]*CompactedMessages, error) {
	head := tipsets[0]
	length := uint64(len(tipsets))

	ctx, span := trace.StartSpan(ctx, "GetChainMessages")
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("tipset", fmt.Sprint(head.Cids())),
			trace.Int64Attribute("count", int64(length)),
		)
	}
	defer span.End()

	req := &Request{
		Head:    head.Cids(),
		Length:  length,
		Options: Messages,
	}

	validRes, err := c.doRequest(ctx, req, nil, tipsets)
	if err != nil {
		return nil, err
	}

	return validRes.messages, nil
}

// Send a request to a peer. Write request in the stream and read the
// response back. We do not do any processing of the request/response
// here.
func (c *client) sendRequestToPeer(ctx context.Context, peer peer.ID, req *Request) (_ *Response, err error) {
	// Trace code.
	ctx, span := trace.StartSpan(ctx, "sendRequestToPeer")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("peer", peer.String()),
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

	supported, err := c.host.Peerstore().SupportsProtocols(peer, ChainExchangeProtocolID)
	if err != nil {
		c.RemovePeer(peer)
		return nil, xerrors.Errorf("failed to get protocols for peer: %w", err)
	}
	if len(supported) == 0 || (supported[0] != ChainExchangeProtocolID) {
		return nil, xerrors.Errorf("peer %s does not support protocols %s",
			peer, []string{ChainExchangeProtocolID})
	}

	connectionStart := build.Clock.Now()

	sctx, cancel := context.WithTimeout(ctx, streamOpenTimeout)
	defer cancel()

	// Open stream to peer.
	stream, err := c.host.NewStream(
		network.WithNoDial(sctx, "should already have connection"),
		peer,
		ChainExchangeProtocolID)
	if err != nil {
		c.RemovePeer(peer)
		return nil, xerrors.Errorf("failed to open stream to peer: %w", err)
	}

	defer stream.Close() //nolint:errcheck

	// Write request.
	_ = stream.SetWriteDeadline(time.Now().Add(WriteReqDeadline))
	if err := cborutil.WriteCborRPC(stream, req); err != nil {
		_ = stream.SetWriteDeadline(time.Time{})
		c.peerTracker.logFailure(peer, build.Clock.Since(connectionStart), req.Length)
		// FIXME: Should we also remove peer here?
		return nil, err
	}
	_ = stream.SetWriteDeadline(time.Time{}) // clear deadline // FIXME: Needs
	//  its own API (https://github.com/libp2p/go-libp2p/core/issues/162).
	if err := stream.CloseWrite(); err != nil {
		log.Warnw("CloseWrite err", "error", err)
	}

	// Read response, limiting the size of the response to maxExchangeMessageSize as we allow a
	// lot of messages (10k+) but they'll mostly be quite small.
	var res Response
	err = cborutil.ReadCborRPC(
		bufio.NewReader(io.LimitReader(incrt.New(stream, ReadResMinSpeed, ReadResDeadline), maxExchangeMessageSize)),
		&res)
	if err != nil {
		c.peerTracker.logFailure(peer, build.Clock.Since(connectionStart), req.Length)
		return nil, xerrors.Errorf("failed to read chainxchg response: %w", err)
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

	c.peerTracker.logSuccess(peer, build.Clock.Since(connectionStart), uint64(len(res.Chain)))
	// FIXME: We should really log a success only after we validate the response.
	//  It might be a bit hard to do.
	return &res, nil
}

// AddPeer implements Client.AddPeer(). Refer to the godocs there.
func (c *client) AddPeer(p peer.ID) {
	c.peerTracker.addPeer(p)
}

// RemovePeer implements Client.RemovePeer(). Refer to the godocs there.
func (c *client) RemovePeer(p peer.ID) {
	c.peerTracker.removePeer(p)
}

// getShuffledPeers returns a preference-sorted set of peers (by latency
// and failure counting), shuffling the first few peers so we don't always
// pick the same peer.
// FIXME: Consider merging with `shufflePrefix()s`.
func (c *client) getShuffledPeers() []peer.ID {
	peers := c.peerTracker.prefSortedPeers()
	shufflePrefix(peers)
	return peers
}

func shufflePrefix(peers []peer.ID) {
	prefix := ShufflePeersPrefix
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

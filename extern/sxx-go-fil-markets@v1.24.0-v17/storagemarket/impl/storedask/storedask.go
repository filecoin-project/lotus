package storedask

import (
	"bytes"
	"context"
	"sync"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	versionedds "github.com/filecoin-project/go-ds-versioning/pkg/datastore"
	"github.com/filecoin-project/go-ds-versioning/pkg/versioned"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
)

var log = logging.Logger("storedask")

// DefaultPrice is the default price for unverified deals (in attoFil / GiB / Epoch)
var DefaultPrice = abi.NewTokenAmount(500000000)

// DefaultVerifiedPrice is the default price for verified deals (in attoFil / GiB / Epoch)
var DefaultVerifiedPrice = abi.NewTokenAmount(50000000)

// DefaultDuration is the default number of epochs a storage ask is in effect for
const DefaultDuration abi.ChainEpoch = 1000000

// DefaultMinPieceSize is the minimum accepted piece size for data
const DefaultMinPieceSize abi.PaddedPieceSize = 256

// DefaultMaxPieceSize is the default maximum accepted size for pieces for deals
// TODO: It would be nice to default this to the miner's sector size
const DefaultMaxPieceSize abi.PaddedPieceSize = 1 << 20

// StoredAsk implements a persisted SignedStorageAsk that lasts through restarts
// It also maintains a cache of the current SignedStorageAsk in memory
type StoredAsk struct {
	askLk sync.RWMutex
	ask   *storagemarket.SignedStorageAsk
	ds    datastore.Batching
	dsKey datastore.Key
	spn   storagemarket.StorageProviderNode
	actor address.Address
}

// NewStoredAsk returns a new instance of StoredAsk
// It will initialize a new SignedStorageAsk on disk if one is not set
// Otherwise it loads the current SignedStorageAsk from disk
func NewStoredAsk(ds datastore.Batching, dsKey datastore.Key, spn storagemarket.StorageProviderNode, actor address.Address,
	opts ...storagemarket.StorageAskOption) (*StoredAsk, error) {
	s := &StoredAsk{
		spn:   spn,
		actor: actor,
		dsKey: dsKey,
	}

	askMigrations, err := versioned.BuilderList{
		versioned.NewVersionedBuilder(migrations.GetMigrateSignedStorageAsk0To1(s.sign), versioning.VersionKey("1")),
	}.Build()

	if err != nil {
		return nil, err
	}

	versionedDs, migrateDs := versionedds.NewVersionedDatastore(ds, askMigrations, versioning.VersionKey("1"))

	// TODO: this is a bit risky -- but this is just a single key so it's probably ok to run migrations in the constructor
	err = migrateDs(context.TODO())
	if err != nil {
		return nil, err
	}

	s.ds = versionedDs

	if err := s.tryLoadAsk(); err != nil {
		return nil, err
	}

	if s.ask == nil {
		// TODO: we should be fine with this state, and just say it means 'not actively accepting deals'
		// for now... lets just set a price
		if err := s.SetAsk(DefaultPrice, DefaultVerifiedPrice, DefaultDuration, opts...); err != nil {
			return nil, xerrors.Errorf("failed setting a default price: %w", err)
		}
	}
	return s, nil
}

// SetAsk configures the storage miner's ask with the provided prices (for unverified and verified deals),
// duration, and options. Any previously-existing ask is replaced.  If no options are passed to configure
// MinPieceSize and MaxPieceSize, the previous ask's values will be used, if available.
// It also increments the sequence number on the ask
func (s *StoredAsk) SetAsk(price abi.TokenAmount, verifiedPrice abi.TokenAmount, duration abi.ChainEpoch, options ...storagemarket.StorageAskOption) error {
	s.askLk.Lock()
	defer s.askLk.Unlock()
	var seqno uint64
	minPieceSize := DefaultMinPieceSize
	maxPieceSize := DefaultMaxPieceSize
	if s.ask != nil {
		seqno = s.ask.Ask.SeqNo + 1
		minPieceSize = s.ask.Ask.MinPieceSize
		maxPieceSize = s.ask.Ask.MaxPieceSize
	}

	ctx := context.TODO()

	_, height, err := s.spn.GetChainHead(ctx)
	if err != nil {
		return err
	}
	ask := &storagemarket.StorageAsk{
		Price:         price,
		VerifiedPrice: verifiedPrice,
		Timestamp:     height,
		Expiry:        height + duration,
		Miner:         s.actor,
		SeqNo:         seqno,
		MinPieceSize:  minPieceSize,
		MaxPieceSize:  maxPieceSize,
	}

	for _, option := range options {
		option(ask)
	}

	sig, err := s.sign(ctx, ask)
	if err != nil {
		return err
	}
	return s.saveAsk(&storagemarket.SignedStorageAsk{
		Ask:       ask,
		Signature: sig,
	})

}

func (s *StoredAsk) sign(ctx context.Context, ask *storagemarket.StorageAsk) (*crypto.Signature, error) {
	tok, _, err := s.spn.GetChainHead(ctx)
	if err != nil {
		return nil, err
	}

	return providerutils.SignMinerData(ctx, ask, s.actor, tok, s.spn.GetMinerWorkerAddress, s.spn.SignBytes)
}

// GetAsk returns the current signed storage ask, or nil if one does not exist.
func (s *StoredAsk) GetAsk() *storagemarket.SignedStorageAsk {
	s.askLk.RLock()
	defer s.askLk.RUnlock()
	if s.ask == nil {
		return nil
	}
	ask := *s.ask
	return &ask
}

func (s *StoredAsk) tryLoadAsk() error {
	s.askLk.Lock()
	defer s.askLk.Unlock()

	err := s.loadAsk()
	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			log.Warn("no previous ask found, miner will not accept deals until a price is set")
			return nil
		}
		return err
	}

	return nil
}

func (s *StoredAsk) loadAsk() error {
	askb, err := s.ds.Get(context.TODO(), s.dsKey)
	if err != nil {
		return xerrors.Errorf("failed to load most recent ask from disk: %w", err)
	}

	var ssa storagemarket.SignedStorageAsk
	if err := cborutil.ReadCborRPC(bytes.NewReader(askb), &ssa); err != nil {
		return err
	}

	s.ask = &ssa
	return nil
}

func (s *StoredAsk) saveAsk(a *storagemarket.SignedStorageAsk) error {
	b, err := cborutil.Dump(a)
	if err != nil {
		return err
	}

	if err := s.ds.Put(context.TODO(), s.dsKey, b); err != nil {
		return err
	}

	s.ask = a
	return nil
}

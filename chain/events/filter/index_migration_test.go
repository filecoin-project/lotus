package filter

import (
	"context"
	"database/sql"
	_ "embed"
	"io"
	pseudo "math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	syncds "github.com/ipfs/go-datastore/sync"
	cbor "github.com/ipfs/go-ipld-cbor"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

//go:embed testdata/events_db_v3_calibnet_sample1.sql
var eventsDBVersion3Sample1 string

type (
	v3EventRow struct {
		id           int64
		height       uint64
		tipsetKey    []byte
		tipsetKeyCid []byte
		emitterAddr  []byte
		eventIndex   int
		messageCid   []byte
		messageIndex int
		reverted     bool
	}
	v4EventRow struct {
		id           int64
		height       uint64
		tipsetKey    []byte
		tipsetKeyCid []byte
		emitter      uint64
		eventIndex   int
		messageCid   []byte
		messageIndex int
		reverted     bool
	}
	eventEntry struct {
		eventId int64
		indexed int64
		flags   []byte
		key     string
		codec   uint64
		value   []byte
	}
)

func TestMigration_V3ToV4Sample1(t *testing.T) {
	const (
		seed    = 4585623
		timeout = 20 * time.Second
	)

	rng := pseudo.New(pseudo.NewSource(seed))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)

	// Populate the sample data and assert expected schema version.
	testDir := t.TempDir()
	v3DbPath := path.Join(testDir, "events.db")
	v3Db, err := sql.Open("sqlite3", v3DbPath)
	require.NoError(t, err)
	requireSqlExec(ctx, t, v3Db, eventsDBVersion3Sample1)
	requireSchemaVersion(t, v3Db, 3)

	addrsMap := map[address.Address]struct{}{}
	var addrs []address.Address
	for row := range listV3Events(ctx, t, v3Db) {
		addr, err := address.NewFromBytes(row.emitterAddr)
		require.NoError(t, err)
		if _, exists := addrsMap[addr]; !exists {
			addrsMap[addr] = struct{}{}
			addrs = append(addrs, addr)
		}
	}

	require.NoError(t, v3Db.Close())

	// Copy the database file for migration, retaining the original for test comparison.
	migratedDbPath := path.Join(testDir, "migrated-events.db")
	requireFileCopy(t, v3DbPath, migratedDbPath)

	// Assert that event index can instantiate successfully, which includes successful migration process.
	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, syncds.MutexWrap(datastore.NewMapDatastore()), filcns.Weight, nil)
	cst := cbor.NewCborStore(cs.StateBlockstore())
	network, err := state.VersionForNetwork(build.TestNetworkVersion)
	require.NoError(t, err)
	tree, err := state.NewStateTree(cst, network)
	require.NoError(t, err)
	//for _, addr := range addrs {
	//	_, err = tree.RegisterNewAddress(addr)
	//	require.NoError(t, err)
	//}
	_, err = tree.Flush(ctx)
	require.NoError(t, err)

	set := fakeTipSet(t, rng, abi.ChainEpoch(14000), []cid.Cid{})
	require.NoError(t, cs.SetHead(ctx, set))
	//policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	//policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	//policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
	//generator, err := gen.NewGenerator()
	//require.NoError(t, err)
	////_, err = generator.NextTipSet()
	////require.NoError(t, err)
	//manager, err := stmgr.NewStateManager(generator.ChainStore(), nil, nil, filcns.DefaultUpgradeSchedule(), generator.BeaconSchedule(), nil, index.DummyMsgIndex)
	//generator.SetStateManager(manager)
	//generator.ChainStore().
	//for i := 0; i < 100; i++ {
	//	_, err = generator.NextTipSetFromMiners(generator.CurTipset.TipSet(), addrs, 0)
	//	require.NoError(t, err)
	//}

	ar := newRandomActorResolver(rng)
	subject, err := NewEventIndex(ctx, migratedDbPath, cs)
	require.NoError(t, err)
	require.NoError(t, subject.Close())

	// Assert schema version 4 in migrated DB path.
	v4Db, err := sql.Open("sqlite3", migratedDbPath)
	require.NoError(t, err)
	requireSchemaVersion(t, v4Db, 4)
	t.Cleanup(func() { require.NoError(t, v4Db.Close()) })

	// Open the original sample DB
	v3Db, err = sql.Open("sqlite3", v3DbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, v3Db.Close()) })

	// Assert row counts are equal across tables.
	require.Equal(t, countTableRows(ctx, t, v3Db, "event_entry"), countTableRows(ctx, t, v4Db, "event_entry"))
	require.Equal(t, countTableRows(ctx, t, v3Db, "event"), countTableRows(ctx, t, v4Db, "event"))

	// Assert each event maps to its expected row.
	for v3Event := range listV3Events(ctx, t, v3Db) {
		v4Event := getV4EventByID(ctx, t, v4Db, v3Event.id)

		require.Equal(t, v3Event.id, v4Event.id)
		require.Equal(t, v3Event.height, v4Event.height)
		require.Equal(t, v3Event.tipsetKey, v4Event.tipsetKey)
		require.Equal(t, v3Event.tipsetKeyCid, v4Event.tipsetKeyCid)
		require.Equal(t, v3Event.eventIndex, v4Event.eventIndex)
		require.Equal(t, v3Event.messageCid, v4Event.messageCid)
		require.Equal(t, v3Event.messageIndex, v4Event.messageIndex)
		require.Equal(t, v3Event.reverted, v4Event.reverted)

		// Assert that emitter actor ID matches the resolved actor ID.
		v3EmitterAddr, err := address.NewFromBytes(v3Event.emitterAddr)
		require.NoError(t, err)
		wantEmitter, err := ar(ctx, v3EmitterAddr, nil)
		require.NoError(t, err)
		require.EqualValues(t, wantEmitter, v4Event.emitter)
	}

	// Assert event entries are unchanged.
	for wantEntry := range listEventEntries(ctx, t, v3Db) {
		requireEntryExists(ctx, t, v4Db, wantEntry)
	}
}

func requireFileCopy(t *testing.T, source string, destination string) {
	t.Helper()
	srcFile, err := os.Open(source)
	require.NoError(t, err)
	destFile, err := os.Create(destination)
	require.NoError(t, err)
	_, err = io.Copy(destFile, srcFile)
	require.NoError(t, err)
	require.NoError(t, destFile.Sync())
	require.NoError(t, destFile.Close())
	require.NoError(t, srcFile.Close())
}

func requireSchemaVersion(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	require.NoError(t, db.QueryRow(`SELECT max(version) FROM _meta`).Scan(&got))
	require.Equal(t, want, got)
}

func listV3Events(ctx context.Context, t *testing.T, db *sql.DB) <-chan v3EventRow {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT id, height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted FROM event`)
	require.NoError(t, err)
	events := make(chan v3EventRow)
	go func() {
		defer func() {
			close(events)
			_ = rows.Close()
		}()
		for rows.Next() {
			var event v3EventRow
			require.NoError(t,
				rows.Scan(&event.id, &event.height, &event.tipsetKey, &event.tipsetKeyCid, &event.emitterAddr, &event.eventIndex, &event.messageCid, &event.messageIndex, &event.reverted))
			select {
			case <-ctx.Done():
			case events <- event:
			}
		}
	}()
	return events
}

func listEventEntries(ctx context.Context, t *testing.T, db *sql.DB) <-chan eventEntry {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT event_id, indexed, flags, key, codec, value FROM event_entry`)
	require.NoError(t, err)
	entries := make(chan eventEntry)
	go func() {
		defer func() {
			close(entries)
			_ = rows.Close()
		}()
		for rows.Next() {
			var entry eventEntry
			require.NoError(t,
				rows.Scan(&entry.eventId, &entry.indexed, &entry.flags, &entry.key, &entry.codec, &entry.value))
			select {
			case <-ctx.Done():
			case entries <- entry:
			}
		}
	}()
	return entries
}

func countTableRows(ctx context.Context, t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count))
	return count
}

func getV4EventByID(ctx context.Context, t *testing.T, db *sql.DB, id int64) v4EventRow {
	t.Helper()
	row := db.QueryRowContext(ctx, `SELECT id, height, tipset_key, tipset_key_cid, emitter, event_index, message_cid, message_index, reverted FROM event WHERE id=?`, id)
	require.NoError(t, row.Err())
	var event v4EventRow
	require.NoError(t,
		row.Scan(&event.id, &event.height, &event.tipsetKey, &event.tipsetKeyCid, &event.emitter, &event.eventIndex, &event.messageCid, &event.messageIndex, &event.reverted))
	return event
}

func requireEntryExists(ctx context.Context, t *testing.T, db *sql.DB, entry eventEntry) {
	t.Helper()
	row := db.QueryRowContext(ctx,
		`SELECT 
					event_id, indexed, flags, key, codec, value FROM event_entry 
				WHERE 
					event_id=? AND indexed=? AND flags=? AND key=? AND codec=? AND value=?`,
		entry.eventId, entry.indexed, entry.flags, entry.key, entry.codec, entry.value)
	require.NoError(t, row.Err())
	var got eventEntry
	require.NoError(t, row.Scan(&got.eventId, &got.indexed, &got.flags, &got.key, &got.codec, &got.value))
	require.Equal(t, entry, got)
}

func requireSqlExec(ctx context.Context, t *testing.T, db *sql.DB, stmt string) {
	t.Helper()
	result, err := db.ExecContext(ctx, stmt)
	require.NoError(t, err)
	affected, err := result.RowsAffected()
	require.NoError(t, err)
	require.NotZero(t, affected)
}

func newRandomActorResolver(rng *pseudo.Rand) ActorResolver {
	// Configure the cache to effectively never expire.
	// This results in a sticky random actor resolution, which is good enough for testing.
	const (
		size           = 1000
		expiry         = 1000 * time.Hour
		cacheNilTipSet = true
	)
	return NewCachedActorResolver(
		func(context.Context, address.Address, *types.TipSet) (abi.ActorID, error) {
			return abi.ActorID(rng.Int63()), nil
		}, size, expiry, cacheNilTipSet)
}

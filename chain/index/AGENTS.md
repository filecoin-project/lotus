# Chain index (chain/index)

The SQLite chain index is persistent, operator-owned state, often older than the running binary. Read the root `AGENTS.md` operator constraints first.

- Schema additions arrive empty on existing databases. Anything that requires new rows to exist must ship with a fast, online way to populate them.
- The `tipset_bloom` row is both a logs bloom and the event-index completion marker. A missing row renders as an all-0xff `logsBloom`, so a placeholder 0xff row is invisible over RPC.
- Coverage comes from the chain, not from SQL: the index can't attest to tipsets it never saw. Deriving the canonical set from the index lets an unindexed tipset answer as "no events".
- `ChainValidateIndex` holds the writer lock for the whole call when backfilling, so long validations block indexing of new tipsets. `database is locked` busy timeouts happen on production nodes.
- An event can have zero entries (an EVM LOG0 with empty data has no topics and no data). Such an event must match every filter that doesn't require an entry: event materialisation LEFT joins `event_entry` to keep it, while key filters inner join deliberately because they require a matching entry. Lose it and getLogs and receipts drop these logs while `eth_subscribe` (`chain/events/filter/event.go`, in memory) still sends them. The SQL path and the subscription path implement one spec separately; check a change against both.
- `PopulateFromSnapshot` indexes tipsets without events or blooms, so a snapshot import leaves its whole message window eventless: at least finality (`lotus chain export --recent-stateroots` requires 900 or more; Forest snapshots have carried about 2000). Natural eventless runs observed on mainnet and calibnet have been under 10 epochs, an observation rather than a protocol limit. A tool that infers "unindexed" from a run of empty tipsets must tolerate natural runs and still catch a snapshot window (`lotus-shed chainindex restore-blooms` uses half of finality).

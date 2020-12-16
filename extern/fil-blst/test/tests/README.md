# Stored proofs

## Single proofs

winning32.prf

## Batch proofs

seal32_x10.prf

The batch proof is structured a little differently than the single proof:
- proof count - 8B
- public input count - 8B
- Verifying key - this was at the end of the single proof file
- proof 0
  - proof (greek letters)
  - inputs (public input count Fr values)
- proof 1
  - proof (greek letters)
  - inputs (public input count Fr values)
etc. 

Each proof should verify individually and the set should verify as a batch.

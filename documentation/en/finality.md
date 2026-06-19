# Finality in Filecoin

This document explains how Lotus determines when a tipset is "finalized", meaning it will not be reorganized out of the canonical chain.

## Overview

Filecoin has two independent finality mechanisms that run in parallel:

1. **F3 (Fast Finality)**: a BFT-style finality gadget that produces certificates for finalized tipsets. When running normally, F3 finalizes tipsets within 4-10 epochs.
2. **EC Probabilistic Finality**: based on observed block production, computes the probability that a confirmed tipset could be reorganized by an adversarial fork. Under healthy conditions (~5 blocks produced per tipset), the one-in-a-billion (2^-30) finality guarantee is typically achieved within ~30 epochs.

Lotus takes the **most recent** finalized tipset from either source. If F3 is finalizing at depth 7 and the EC calculator says depth 22, F3 wins. If F3 is lagging at depth 40 but the calculator says depth 25, the calculator wins.

If both are unavailable, Lotus falls back to the static 900-epoch EC finality assumption which represents an absolute worst-case chain health assumption given the one-in-a-billion finality guarantee.

## What "finalized" and "safe" mean

A **finalized** tipset has a negligible probability of being reverted. In Ethereum terminology, this maps to the `"finalized"` block tag. Applications that need strong settlement guarantees (bridges, exchanges, payment confirmations) should wait for finality before considering a transaction settled.

The **safe** tag returns a tipset that balances recency with confidence, sitting between `"finalized"` and `"latest"`. It is defined as whichever is more recent: the finalized tipset, or the tipset at a fixed lookback distance from head (200 epochs for v2 / Eth v2, 30 epochs for Eth v1). Under normal conditions where F3 and/or the EC calculator are providing a finality depth shallower than the safe distance, `"safe"` and `"finalized"` will return the same tipset. When finality mechanisms are degraded or lagging, `"safe"` provides a more recent tipset than `"finalized"` at the cost of a weaker guarantee.

Typically `"finalized"` is the right choice for most applications that don't want to be concerned with chain reorganization.

## Querying finality

### v2 API

The `"finalized"` and `"safe"` tags can be used as tipset selectors:

```
Filecoin.ChainGetTipSet({"tag":"finalized"})
Filecoin.ChainGetTipSet({"tag":"safe"})
```

To understand how finality is currently being determined:

```
Filecoin.ChainGetTipSetFinalityStatus()
```

This returns:

| Field | Description |
|-------|-------------|
| `ecFinalityThresholdDepth` | Epoch depth at which the reorg probability drops below 2^-30. -1 if chain health is too degraded. |
| `ecFinalizedTipSet` | The tipset at that depth (nil if threshold not met) |
| `f3FinalizedTipSet` | The tipset finalized by F3 (nil if F3 is unavailable) |
| `finalizedTipSet` | The overall finalized tipset the node is using |
| `head` | Current chain head |

This endpoint is useful for monitoring and diagnostics: understanding *why* finality is at a particular depth, and whether F3 or the EC calculator is currently driving finalization.

### Ethereum JSON-RPC

```
eth_getBlockByNumber("finalized", false)
eth_getBlockByNumber("safe", false)
```

The `"finalized"` and `"safe"` tags use the same parallel F3 + EC calculator logic described above.

### lotus-shed

```bash
lotus-shed finality-calculator
```

Displays a table of reorg probabilities at various depths for the current chain, along with the threshold depth where the 2^-30 guarantee is met. Supports `--csv` for machine-readable output.

## How EC probabilistic finality works

The EC calculator examines recent chain history (block counts per epoch over the last 900 epochs) and computes an upper bound on the probability that an adversary controlling up to 30% of mining power could build a longer fork that replaces a confirmed tipset. The algorithm is specified in [FRC-0089](https://github.com/filecoin-project/FIPs/blob/master/FRCs/frc-0089.md), with formal proofs and evaluation in ["The Finality Calculator: Analyzing and Quantifying Filecoin's Finality Guarantees"](https://arxiv.org/abs/2603.01307) (Goren & Soares, 2026).

Key factors that affect the threshold depth:

- **Block production rate**: more blocks per epoch means faster finality. Healthy mainnet (~4.5 blocks/epoch) typically reaches 2^-30 within 25-35 epochs. Degraded production (2-3 blocks/epoch) pushes the threshold deeper or may not reach it at all.
- **Adversary power assumption**: the standard assumption is 30% Byzantine power. This is the same assumption that the static 900-epoch finality was designed around.

The calculator is at least as conservative as the static 900-epoch assumption. When chain health is poor, the threshold moves deeper (not shallower), and if conditions are severely degraded, the calculator reports that the threshold is not met (`ecFinalityThresholdDepth: -1`), causing the node to fall back to the static 900 epochs. Network partitions reduce observed block production, which makes the calculator *more* conservative, not less.

# Raúl's notes

## Storage mining

The Storage Mining System is the part of the Filecoin Protocol that deals with
storing Client’s data, producing proof artifacts that demonstrate correct
storage behavior, and managing the work involved.

## Preseals

In the Filecoin consensus protocol, the miners' probability of being eligible
to mine a block in a given epoch is directly correlated with their power in the
network. This creates a chicken-and-egg problem at genesis. Since there are no
miners, there is no power in the network, therefore no miner is eligible to mine
and advance the chain.

Preseals are sealed sectors that are blessed at genesis, thus conferring
their miners the possibility to win round elections and successfully mine a
block. Without preseals, the chain would be dead on arrival.

Preseals work with fauxrep and faux sealing, which are special-case
implementations of PoRep and the sealing logic that do not depend on slow
sealing.

### Not implemented things

**Sector Resealing:** Miners should be able to ’re-seal’ sectors, to allow them
to take a set of sectors with mostly expired pieces, and combine the
not-yet-expired pieces into a single (or multiple) sectors.

**Sector Transfer:** Miners should be able to re-delegate the responsibility of
storing data to another miner. This is tricky for many reasons, and will not be
implemented in the initial release of Filecoin, but could provide interesting
capabilities down the road.

## Catch-up/rush mining

In catch-up or rush mining, miners make up for chain history that does not
exist. It's a recovery/healing procedure. The chain runs at at constant
25 second epoch time. When in the network mining halts for some reason
(consensus/liveness bug, drand availability issues, etc.), upon a restart miners
will go and backfill the chain history by mining backdated blocks in
the appropriate timestamps.
   
There are a few things worth highlighting:
   * mining runs in a hot loop, and there is no time for miners to gossip about
     their blocks; therefore they end up building the chain solo, as they can't
     incorprate other blocks into tipsets.
   * the miner with most power will mine most blocks.
   * presumably, as many forks in the network will appear as miners who mined a
     block + a fork filled with null rounds only (for miners that didn't win a
     round).
   * at the end of the catch-up, the heaviest fork will win the race, and it may
     be possible for the most powerful miner pre-disruption to affect the
     outcome by choosing the messages that go in their blocks.

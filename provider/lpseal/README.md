# Lotus-Provider Sealer

## Overview

The lotus-provider sealer is a collection of harmony tasks and a common poller
which implement the sealing functionality of the Filecoin protocol.

## Pipeline Tasks

* SDR pipeline
  * `SDR` - Generate SDR layers
  * `SDRTrees` - Generate tree files (TreeD, TreeR, TreeC)
  * `PreCommitSubmit` - Submit precommit message to the network
  * `PoRep` - Generate PoRep proof
  * `CommitSubmit` - Submit commit message to the network

# Poller

The poller is a background process running on every node which runs any of the
SDR pipeline tasks. It periodically checks the state of sectors in the SDR pipeline
and schedules any tasks to run which will move the sector along the pipeline.

# Error Handling

* Pipeline tasks are expected to always finish successfully as harmonytask tasks.
  If a sealing task encounters an error, it should mark the sector pipeline entry
  as failed and exit without erroring. The poller will then figure out a recovery
  strategy for the sector.

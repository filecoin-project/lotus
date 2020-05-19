---
name: Sealing Issues
about: Create a report for help with sealing (commit) failures.
title: ''
labels: 'sealing'
assignees: ''

---

Please provide all the information requested here to help us troubleshoot "commit failed" issues.
If the information requested is missing, we will probably have to just ask you to provide it anyway,
before we can help debug.

**Describe the problem**

A brief description of the problem you encountered while proving (sealing) a sector.

Including what commands you ran, and a description of your setup, is very helpful.

**Sectors list**

The output of `./lotus-storage-miner sectors list`.

**Sectors status**

The output of `./lotus-storage-miner sectors status --log <sectorId>` for the failed sector(s).

**Lotus storage miner logs**

Please go through the logs of your storage miner, and include screenshots of any error-like messages you find.

**Version**

The output of `./lotus --version`.

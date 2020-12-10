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

**Sectors status**

The output of `lotus-miner sectors status --log <sectorId>` for the failed sector(s).

**Lotus miner logs**

Please go through the logs of your miner, and include screenshots of any error-like messages you find.

Alternatively please upload full log files and share a link here

**Lotus miner diagnostic info**

Please collect the following diagnostic information, and share a link here

* lotus-miner diagnostic info `lotus-miner info all > allinfo`

** Code modifications **

If you have modified parts of lotus, please describe which areas were modified,
and the scope of those modifications

**Version**

The output of `lotus --version`.

---
name: Deal Making Issues
about: Create a report for help with deal making failures.
title: "[Deal Making Issue]"
labels: hint/needs-triaging, area/markets
assignees: ''

---

> Note: For security-related bugs/issues, please follow the [security policy](https://github.com/filecoin-project/lotus/security/policy).

Please provide all the information requested here to help us troubleshoot "deal making failed" issues.
If the information requested is missing, we will probably have to just ask you to provide it anyway,
before we can help debug.

**Basic Information**
Including information like, Are you the client or the miner? Is this a storage deal or a retrieval deal? Is it an offline deal?

**Describe the problem**

A brief description of the problem you encountered while trying to make a deal. 

**Version**

The output of `lotus --version`.

**Setup**

You miner(if applicable) and daemon setup, i.e: What hardware do you use, how much ram and etc.

**To Reproduce**
 Steps to reproduce the behavior:
 1. Run '...'
 2. See error

**Deal status**

The output of `lotus client list-deals -v` and/or `lotus-miner storage-deals|retrieval-deals|data-transfers list [-v]` commands for the deal(s) in question.

**Lotus daemon and miner logs**

Please go through the logs of your daemon and miner(if applicable), and include screenshots of any error/warning-like messages you find.

Alternatively please upload full log files and share a link here

** Code modifications **

If you have modified parts of lotus, please describe which areas were modified,
and the scope of those modifications

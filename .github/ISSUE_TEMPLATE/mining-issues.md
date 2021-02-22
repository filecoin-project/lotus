---
name: Mining Issues
about: Create a report for help with mining failures.
title: "[Mining Issue]"
labels: hint/needs-triaging, area/mining
assignees: ''

---

> Note: For security-related bugs/issues, please follow the [security policy](https://github.com/filecoin-project/lotus/security/policy).

Please provide all the information requested here to help us troubleshoot "mining/WinningPoSt failed" issues.
If the information requested is missing, you may be asked you to provide it.

**Describe the problem**
A brief description of the problem you encountered while mining new blocks.

**Version**

The output of `lotus --version`.

**Setup**

You miner and daemon setup, including what hardware do you use, your environment variable settings, how do you run your miner and worker, do you use GPU and etc.

**Lotus daemon and miner logs**

Please go through the logs of your daemon and miner, and include screenshots of any error/warning-like messages you find, highlighting the one has "winning post" in it.

Alternatively please upload full log files and share a link here

** Code modifications **

If you have modified parts of lotus, please describe which areas were modified,
and the scope of those modifications

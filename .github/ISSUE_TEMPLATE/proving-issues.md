---
name: Proving Issues
about: Create a report for help with proving failures.
title: "[Proving Issue]"
labels: area/proving, hint/needs-triaging
assignees: ''

---

Please provide all the information requested here to help us troubleshoot "proving/wdpost failed" issues.
If the information requested is missing, we will probably have to just ask you to provide it anyway,
before we can help debug.

**Describe the problem**
A brief description of the problem you encountered while proving the storage.

**Version**

The output of `lotus --version`.

**Setup**

You miner, worker and daemon setup.

**Commands**

Commands you ran.

**Proving status**

The output of `lotus-miner proving` info.

**Lotus miner logs**

Please go through the logs of your miner, and include screenshots of any error-like messages you find, highlighting the one has "window post" in it.

Alternatively please upload full log files and share a link here

**Lotus miner diagnostic info**

Please collect the following diagnostic information, and share a link here

* lotus-miner diagnostic info `lotus-miner info all > allinfo`

** Code modifications **

If you have modified parts of lotus, please describe which areas were modified,
and the scope of those modifications

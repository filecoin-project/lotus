# V2 API Documentation Context

## Project Overview
This directory contains the experimental user-facing documentation for Filecoin's V2 API. The purpose of this work is to maintain the documentation in source control for easier team review before publishing to external platforms like Notion.

## Source Information
- Original source: https://filoznotebook.notion.site/Filecoin-V2-APIs-1d0dc41950c1808b914de5966d501658
- This documentation describes experimental V2 APIs that are subject to change

## Workflow
1. (If this hasn't already been don) Copy updates from Notion into this repository
2. Make changes, likely using Claude Code by pointing to local changes or changes in a PR.
2. Regenerate the table of contents if you've added or modified sections
3. Submit changes for team review via pull request
4. After approval, publish updated content back to Notion

**Important**: Always regenerate the table of contents before committing changes to ensure it accurately reflects the document structure. The table of contents is comprehensive and includes all sections of the document, helping readers navigate the content.  It also helps give an overview in the diff of what content is being added/changed and where in the document.  

## Key Files
- `api-v2-experimental.md`: The main user facing documentation file that is copied to Notion.
- Related code: `api/v2api/full.go` (API definition)
- Related code: `chain/types/tipset_selector.go` (Key types)
- Related code: `node/impl/full/chain_v2.go` (API implementation)
- Related code: `node/impl/full/state_v2.go` (API implementation)
- Related code: `node/impl/eth/api.go` (ETH V2 API implementation)
- Related code: `node/impl/eth/tipsetresolver.go` (ETH block specifier to tipset conversion)
- Related code: `node/impl/eth/filecoin.go` (Filecoin-specific ETH methods)
- Generated docs: `documentation/en/api-v2-unstable-methods.md`

## ETH V2 APIs Key Implementation Details

The ETH V2 APIs implementation in PR #13026 introduces important changes to how block specifiers like "safe" and "finalized" work:

1. **TipSet Resolution**: The file `node/impl/eth/tipsetresolver.go` contains the key logic for converting Ethereum block specifiers to Filecoin tipsets. The V2 implementation:
   - Connects directly to the F3 subsystem for finality information
   - Implements a more responsive definition of "safe" and "finalized" blocks
   - Falls back gracefully to EC finality when F3 is not available

2. **Block Handling**: In V2 ETH APIs, there's no longer a `-1` offset that was present in V1. This means:
   - "latest" truly refers to the latest tipset (head)
   - "safe" refers to either F3 finalized or head-200, whichever is more recent
   - "finalized" directly maps to F3 finality when available

3. **Request Path**: Requests to `/v2` ETH endpoints are processed through dedicated handlers that use the F3-aware tipset resolution logic, offering faster confirmation times.

## Commands

### Regenerating the Table of Contents
For Claude to regenerate the table of contents:
1. Extract all headers from the document using grep:
```bash
grep -n '^#\|^##\|^###\|^####' api-v2-experimental.md
```
2. Use this information to update the Table of Contents section, ensuring all headers are properly nested according to their level and linked.

Humans in their IDE can use [Markdown All in One](https://marketplace.visualstudio.com/items?itemName=yzhang.markdown-all-in-one).
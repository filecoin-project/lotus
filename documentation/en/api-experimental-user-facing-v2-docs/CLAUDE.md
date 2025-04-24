# V2 API Documentation Context

## Project Overview
This directory contains the experimental user-facing documentation for Filecoin's V2 API. The purpose of this work is to maintain the documentation in source control for easier team review before publishing to external platforms like Notion.

## Source Information
- Original source: https://filoznotebook.notion.site/Filecoin-V2-APIs-1d0dc41950c1808b914de5966d501658
- This documentation describes experimental V2 APIs that are subject to change

## Workflow
1. (If this hasn't already been don) Copy updates from Notion into this repository
2. Make changes, likely using Claude Code by pointing to to local changes or changes in a PR.
2. Regenerate the table of contents if you've added or modified sections
3. Submit changes for team review via pull request
4. After approval, publish updated content back to Notion

**Important**: Always regenerate the table of contents before committing changes to ensure it accurately reflects the document structure. The table of contents is comprehensive and includes all sections of the document, helping readers navigate the content.  It also helps give an overview in the diff of what content is being added/changed and where in the document.  

## Key Files
- `api-v2-experimental.md`: The main user facing documentation file that is copied to Notion.
- Related code: `api/v2api/full.go` (API implementation)
- Related code: `chain/types/tipset_selector.go` (Key types)
- Generated docs: `documentation/en/api-v2-unstable-methods.md`

## Commands

### Regenerating the Table of Contents
For Claude to regenerate the table of contents:
1. Extract all headers from the document using grep:
```bash
grep -n '^#\|^##\|^###\|^####' api-v2-experimental.md
```
2. Use this information to update the Table of Contents section, ensuring all headers are properly nested according to their level and linked.

Humans in their IDE can use [Markdown All in One](https://marketplace.visualstudio.com/items?itemName=yzhang.markdown-all-in-one).
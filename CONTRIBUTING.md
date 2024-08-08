> 2024-08-08: this will get flushed out more soon as part of https://github.com/filecoin-project/lotus/issues/12360

### PR Title Conventions
PR titles should follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) standard.
This means the PR title should be in the form of of `<type>(<scope>): <description>`
  - example: `fix(mempool): introduce a cache for valid signatures`
  - `type`: MUST be one of _build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test_
  - `scope`: OPTIONAL arbitrary string that is usually one of _api, chain, deps, mempool, multisig, networking, paych, proving, sealing, state, wallet_
  - Breaking changes must add a `!`
  - Optionally the PR title can be prefixed with `[skip changelog]` if no changelog edits should be made for this change.
Note that this enforced with https://github.com/filecoin-project/lotus/blob/master/.github/workflows/pr-title-check.yml

### CHANGELOG Management
To expedite the release process, the CHANGELOG is built-up incrementally.  
We enforce that each PR updates CHANGELOG.md or signals that the change doesn't need it.
If the PR affects users (e.g., new feature, bug fix, system requirements change), update the CHANGELOG.md and add details to the UNRELEASED section.
If the change does not require a CHANGELOG.md entry, do one of the following:
- Add `[skip changelog]` to the PR title
- Add the label `skip/changelog` to the PR
Note that this enforced with https://github.com/filecoin-project/lotus/blob/master/.github/workflows/changelog.yml
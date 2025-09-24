<!-- omit from toc -->
# Contributing to Lotus

Lotus is a universally open project and welcomes contributions of all kinds: code, docs, and more. We appreciate your interest in contributing!

- [Before Contributing](#before-contributing)
- [Implementing Changes](#implementing-changes)
- [Working with builtin-actors](#working-with-builtin-actors)
- [PR Title Conventions](#pr-title-conventions)
- [CHANGELOG Management](#changelog-management)
- [Dependency Management](#dependency-management)
- [Go Version Management](#go-version-management)
- [Markdown Conventions](#markdown-conventions)
  - [Table Of Contents](#table-of-contents)
- [Getting Help](#getting-help)

## Before Contributing

1. Familiarize yourself with our [release flow](LOTUS_RELEASE_FLOW.md) to understand how changes are incorporated into releases.  This includes our branch strategy and where to target PRs.
2. If the proposal entails a protocol change, please first submit a [Filecoin Improvement Proposal](https://github.com/filecoin-project/FIPs).
3. If the change is complex and requires prior discussion, [open an issue](github.com/filecoin-project/lotus/issues) or a [discussion](https://github.com/filecoin-project/lotus/discussions) to request feedback before you start working on a pull request. This is to avoid disappointment and sunk costs, in case the change is not actually needed or accepted.
4. Please refrain from submitting PRs to adapt existing code to subjective preferences. The changeset should contain functional or technical improvements/enhancements, bug fixes, new features, or some other clear material contribution. Simple stylistic changes are likely to be rejected in order to reduce code churn.

## Implementing Changes

When implementing a change:

1. Adhere to the standard Go formatting guidelines, e.g. [Effective Go](https://golang.org/doc/effective_go.html). Run `go fmt`.
2. Stick to the idioms and patterns used in the codebase. Familiar-looking code has a higher chance of being accepted than eerie code. Pay attention to commonly used variable and parameter names, avoidance of naked returns, error handling patterns, etc.
3. Comments: follow the advice on the [Commentary](https://golang.org/doc/effective_go.html#commentary) section of Effective Go.
4. Minimize code churn. Modify only what is strictly necessary. Well-encapsulated changesets will get a quicker response from maintainers.
5. Lint your code with [`golangci-lint`](https://golangci-lint.run) (CI will reject your PR if unlinted).
6. Add tests.
7. Write clean, thoughtful, and detailed [commit messages](https://chris.beams.io/posts/git-commit/). This is even more important than the PR description, because commit messages are stored _inside_ the Git history. One good rule is: if you are happy posting the commit message as the PR description, then it's a good commit message.

## Working with builtin-actors

For doing FIP development that involves a change in [builtin-actors](https://github.com/filecoin-project/builtin-actors) which needs to also be exposed in Lotus, see the [builtin-actor Development Guide](documentation/misc/Builtin-actors_Development.md).

## PR Title Conventions

PR titles should follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) standard.
This means the PR title should be in the form of `<type>(<scope>): <description>`
  - example: `fix(mempool): introduce a cache for valid signatures`
  - `type`: MUST be one of _build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test_
  - `scope`: OPTIONAL arbitrary string that is usually one of _api, chain, deps, mempool, multisig, networking, paych, proving, sealing, state, wallet_
  - Breaking changes must add a `!`

Alternatively, titles can match a GitHub revert title: `Revert "<original title>"`
 - example: `Revert "feat: add new feature"`

Note that this is enforced with https://github.com/filecoin-project/lotus/blob/master/.github/workflows/pr-title-check.yml

If it is unclear why one's title is getting flagged, it the regex defined in [pr-title-check.yml](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/pr-title-check.yml#L28) can be plugged into a debugger like [regex101.com](https://regex101.com/r/kUrU3F/2).

## CHANGELOG Management

To expedite the release process, the CHANGELOG is built-up incrementally.
We enforce that each PR updates CHANGELOG.md or signals that the change doesn't need it.
If the PR affects users (e.g., new feature, bug fix, system requirements change), update the CHANGELOG.md and add details to the UNRELEASED section.
If the change does not require a CHANGELOG.md entry, do one of the following:
- Add `[skip changelog]` to the PR description
- Add the label `skip/changelog` to the PR

Note that this is enforced with https://github.com/filecoin-project/lotus/blob/master/.github/workflows/changelog.yml

## Dependency Management

We strive to use release dependencies because:
1. Security / reliability - While there's no guarantee that a released version doesn't have bugs or issues, it seems fair to assume that non-released versions have even more. For example, https://github.com/filecoin-project/lotus/issues/12467 was triggered because of a bug in non-released library that lotus was depending on when the latest released version didn't have the bug.
2. Faster builds
3. Makes Lotus a better citizen when it's imported by other projects.

We enforce that each dependency on an unreleased version of a package is explicitly documented in the `go.mod` file via an inline comment of the form `dependency-check-ignore: <reason>`.
* If you are adding such a dependency, please add a suitable comment to the `go.mod` file as well.
* This requirement applies both to direct and indirect dependencies.
* This requirement applies to packages that have released versions (i.e., is not a `v0.0.0`).
* This is enforced with https://github.com/filecoin-project/lotus/blob/master/.github/workflows/dependency-check.yml
* This enforcement was initially done per [#7131](https://github.com/filecoin-project/lotus/issues/7131).

## Go Version Management

### Philosophy

Lotus follows a conservative Go version strategy to balance stability with access to new features:

- **Stay One Minor Version Behind**: We typically use the Go version that is one minor version behind the latest stable release
- **Gradual Adoption**: When a new Go minor version is released, we continue using the previous version for approximately one month before upgrading

This approach provides:
- **Stability**: Allows time for the Go community to identify and fix issues in new releases
- **Ecosystem Compatibility**: Ensures our dependencies have time to test and adapt to new Go versions
- **Predictable Upgrades**: Contributors and downstream users can anticipate Go version requirements

### Example Timeline

Here's how our Go version policy works in practice:

```
Time t: Latest Go minor version is 1.24.x. We use 1.23.x.

Time t': Go 1.25.x is released. We continue using 1.23.x 
         (transition period of ~1 month).

Time t'+1 month: Latest Go minor version is 1.25.x. We upgrade to 1.24.x.
```

### Patch Version Updates

Between minor version upgrades, we periodically update to newer patch releases (e.g., 1.23.7 → 1.23.10) to incorporate:
- Security fixes
- Bug fixes
- Performance improvements

These patch updates happen more frequently and are generally lower risk.

### Go Version Update Process

When updating the Go version (either patch or minor), the following files must be updated consistently:

1. **`go.mod`** - The Go language version directive.  This specifies the Go language version for module compilation.
2. **`GO_VERSION_MIN`** - Minimum version enforced by the Makefile.
3. **`README.md`** - Documentation and badge (2 locations).  This documents requirements for contributors and users.
4. **`Dockerfile`** - Docker base image version.  This ensures container builds use the correct Go version.

#### Step-by-Step Process
   
```bash
OLD_GO_VERSION="1.23.7"
NEW_GO_VERSION="1.23.10"

# Update go.mod
sed -i "s/go $OLD_GO_VERSION/go $NEW_GO_VERSION/" go.mod
   
# Update GO_VERSION_MIN
echo "$NEW_GO_VERSION" > GO_VERSION_MIN
   
# Update README.md badge and documentation
sed -i "s/$OLD_GO_VERSION/$NEW_GO_VERSION/g" README.md
   
# Update Dockerfile
sed -i "s/FROM golang:$OLD_GO_VERSION-bullseye/FROM golang:$NEW_GO_VERSION-bullseye/" Dockerfile

# Add a changelog entry

# Validate
go mod tidy
make build
make unittests
make lint
```

#### Example Reference

For an example of a Go patch version update, see [PR #13190](https://github.com/filecoin-project/lotus/pull/13190), which updated from Go 1.23.7 to 1.23.10.
For an example of a Go minor version update, see [PR #13354](https://github.com/filecoin-project/lotus/pull/13354), which updated from Go 1.23.10 to 1.24.7.

### Developer Notes

- **`.go-version`** files are gitignored and developer-specific
- Developers can create local `.go-version` files for their preferred Go version managers
- The formal Go version requirements are documented in the tracked files listed above
- Go version updates should be coordinated changes affecting all relevant files

## Markdown Conventions
We optimize our markdown files for viewing on GitHub. That isn't to say other syntaxes can't be used, but that is the flavor we focus on and at the minimum don't want to break.

### Table Of Contents
For longer documents, we tend to inline a table of contents (TOC). This is slightly annoying with Markdown on GitHub because the actual table of contents needs to be checked-in rather than just giving a directive to the renderer like []`[[_TOC_]]` on GitLab](https://docs.gitlab.com/ee/user/markdown.html#table-of-contents).  (Yes GitHub UI does enable someone to see an auto-generated outline if they click the "outline" button on the top right-hand corner when viewing a Markdown file, but that isn't necessarily obvious to users, requires an extra click, and doesn't enable any customization.)

[There is tooling](https://stackoverflow.com/questions/11948245/markdown-to-create-pages-and-table-of-contents) that can help with auto-generating and updating a TOC and allow customization.  Maintainers aren't wedded to a particular format/standard/tool but have gone with [VS Code Markdown All in One](https://marketplace.visualstudio.com/items?itemName=yzhang.markdown-all-in-one) because it has what we needed and is the most popular VS Code plugin for this functionality (at least as of 202408).

## Getting Help

If you need help with your contribution, please don't hesitate to ask questions in our [discussions](https://github.com/filecoin-project/lotus/discussions) or reach out to [the maintainers](https://github.com/orgs/filecoin-project/teams/lotus-maintainers).

Thank you for contributing to Lotus!

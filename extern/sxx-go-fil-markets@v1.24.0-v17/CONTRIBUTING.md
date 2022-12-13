# Contributing to this repo

First, thank you for your interest in contributing to this project! Before you pick up your first issue and start
changing code, please:

1. Review all documentation for the module you're interested in.
1. Look through the [issues for this repo](https://github.com/filecoin-project/go-fil-markets/issues) for relevant discussions.
1. If you have questions about an issue, post a comment in the issue.
1. If you want to submit changes that aren't covered by an issue, file a new one with your proposal, outlining what problem you found/feature you want to implement, and how you intend to implement a solution.

For best results, before submitting a PR, make sure:
1. It has met all acceptance criteria for the issue.
1. It addresses only the one issue and does not make other, irrelevant changes.
1. Your code conforms to our coding style guide.
1. You have adequate test coverage (this should be indicated by CI results anyway).
1. If you like, check out [current PRs](https://github.com/filecoin-project/go-fil-markets/pulls) to see how others do it.

Special Note:
If editing README.md, please conform to the [standard readme specification](https://github.com/RichardLitt/standard-readme/blob/master/spec.md).

### PR Process

Active development of `go-fil-markets` occurs on the `master` branch. All PRs should be made to the `master` branch, which is the default branch on Github.

Before a PR can be merged to `master`, it must:
1. Pass continuous integration.
1. Be rebased and up to date with the `master` branch
1. Be approved by at least two maintainers

When merging normal PRs to `master`, always use squash and merge to maintain a linear commit history.

### Release Process

When creating a new full release, branch off master with a branch named release/*version-number*, where *version-number* is the ultimate tag you intend to create.

Continue to develop on master and merge commits to your release branch as neccesary till the release is ready.

When the release is ready, tag it, then merge the branch back into master so that it is part of the version history of master. Delete the release branch.

### Hotfix Process

Hot-fixes operate just like release branches, except they are branched off an existing tag and should be named hotfix/*version-number*. When ready, they receive their own tag and then are merged back to master, then deleted.

For external reference, his git flow and release process is essentially the [OneFlow git workflow](https://www.endoflineblog.com/oneflow-a-git-branching-model-and-workflow)

Following the release of Filecoin Mainnet, this library will following a semantic versioning scheme for tagged releases.

### Testing

- All new code should be accompanied by unit tests. Prefer focused unit tests to integration tests for thorough validation of behaviour. Existing code is not necessarily a good model, here.
- Integration tests should test integration, not comprehensive functionality
- Tests should be placed in a separate package named `$PACKAGE_test`. For example, a test of the `chain` package should live in a package named `chain_test`. In limited situations, exceptions may be made for some "white box" tests placed in the same package as the code it tests.

### Conventions and Style

#### Imports
We use the following import ordering.
```
import (
        [stdlib packages, alpha-sorted]
        <single, empty line>
        [external packages]
        <single, empty line>
        [other-filecoin-project packages]
        <single, empty line>
        [go-fil-markets packages]
)
```

Where a package name does not match its directory name, an explicit alias is expected (`goimports` will add this for you).

Example:

```go
import (
	"context"
	"testing"

	cmds "github.com/ipfs/go-ipfs-cmds"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	
	"github.com/filecoin-project/go-fil-markets/filestore/file"
)
```

You can run `script/fiximports` to put all your code in the desired format
#### Comments
Comments are a communication to other developers (including your future self) to help them understand and maintain code. Good comments describe the _intent_ of the code, without repeating the procedures directly.

- A `TODO:` comment describes a change that is desired but could not be immediately implemented. It must include a reference to a GitHub issue outlining whatever prevents the thing being done now (which could just be a matter of priority).
- A `NOTE:` comment indicates an aside, some background info, or ideas for future improvement, rather than the intent of the current code. It's often fine to document such ideas alongside the code rather than an issue (at the loss of a space for discussion).
- `FIXME`, `HACK`, `XXX` and similar tags indicating that some code is to be avoided in favour of `TODO`, `NOTE` or some straight prose.

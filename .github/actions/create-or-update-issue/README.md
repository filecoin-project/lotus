# Create Or Update Issue on Failure

This GitHub Action creates an issue or comments on an existing issue if a workflow run fails.

## Inputs

- `GITHUB_TOKEN` (required): The GitHub token for authentication.
- `title` (required): The title of the issue to create or update.
- `body` (required): The body of the issue to create or update.
- `label` (required): The label to add to the issue. Used to identify the issue.

## Outputs

- `issue-number`: The issue number if an issue was found or created.

## Usage

Here is an example of how to use this action in your workflow:

```yaml
name: CI

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v2

      # Add your build steps here

      - name: Create issue on failure
        if: failure()
        uses: ./.github/actions/create-issue-on-failure
        with:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          title: 'Build failed'
          body: 'The build has failed. Please check the details and fix the issue.'
          label: 'build-failure'
```

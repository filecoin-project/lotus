# Lotus Release Tool

The Lotus Release Tool is a CLI (Command Line Interface) utility designed to facilitate interactions with the Lotus Node and Miner metadata. This tool allows users to retrieve version information in either JSON or text format. The Lotus Release Tool was initially developed as a part of the [2024Q2 release process automation initiative](https://github.com/filecoin-project/lotus/issues/12010) and is used in the GitHub Actions workflows related to the release process.

## Features

- List all projects with their expected version information.
- Create a new release issue from a template.
- Output logs in JSON or text format.

## Installation

To install the Lotus Release Tool, you need to have Go installed on your system.

1. Build the tool:
    ```sh
    go build -o release ./cmd/release
    ```

## Usage

The `release` tool provides several commands and options to interact with the Lotus Node and Miner.

### Commands

- **List Projects**: List all projects with their version information.
    ```sh
    ./release list-projects
    ```
- **Create Issue**: Create a new release issue from a template.
    ```sh
    ./release create-issue
    ```

### Global Options

- **--json**: Format output as JSON.
    ```sh
    ./release --json
    ```

## Examples

List Lotus Node and Lotus Miner version information with JSON formatted output:
```sh
./release --json list-projects
```

Create a new release issue from a template:
```sh
./release create-issue --type node --tag 1.30.1 --level patch --network-upgrade --discussion-link https://github.com/filecoin-project/lotus/discussions/12010 --changelog-link https://github.com/filecoin-project/lotus/blob/v1.30.1/CHANGELOG.md --rc1-date 2023-04-01 --rc1-precision day --rc1-confidence confirmed --stable-date 2023-05-01 --stable-precision week --stable-confidence estimated
```

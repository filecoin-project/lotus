# Lotus Release Tool

The Lotus Release Tool is a CLI (Command Line Interface) utility designed to facilitate interactions with the Lotus Node and Miner metadata. This tool allows users to retrieve version information in either JSON or text format. The Lotus Release Tool was developed as a part of the [2024Q2 release process automation initiative](https://github.com/filecoin-project/lotus/issues/12010) and is used in the GitHub Actions workflows related to the release process.

## Features

- List all projects with their expected version information.
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

### Options

- **--json**: Format output as JSON.
    ```sh
    ./release --json list-projects
    ```

## Example

List Lotus Node and Lotus Miner version information with JSON formatted output:
```sh
./release --json list-projects
```

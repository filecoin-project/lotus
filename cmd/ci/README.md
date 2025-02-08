# Lotus CI Tool

The Lotus CI Tool is a CLI (Command Line Interface) utility designed to facilitate interactions with the Lotus metadata. This tool allows users to retrieve version information in either JSON or text format which is required in the Lotus CI environment.

## Features

- List all test group execution contexts.
- Get the metadata for a test group.

## Installation

To install the Lotus CI Tool, you need to have Go installed on your system.

1. Build the tool:
    ```sh
    go build -o ci ./cmd/ci
    ```

## Usage

The `ci` tool provides several commands and options to interact with.

### Commands

- **List Test Group Execution Contexts**: List all test group execution contexts.
    ```sh
    ./ci list-test-group-execution-contexts
    ```
- **Get Test Group Metadata**: Get the metadata for a test group.
    ```sh
    ./ci get-test-group-metadata --name "unit-cli"
    ```

### Global Options

- **--json**: Format output as JSON.
    ```sh
    ./ci --json
    ```

## Examples

List all test group execution contexts with JSON formatted output:
```sh
./ci --json list-test-group-execution-contexts
```

Get the metadata for a test group with JSON formatted output:
```sh
./ci --json get-test-group-metadata --name "unit-cli"
```

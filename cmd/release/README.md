# Lotus Release Tool

The Lotus Release Tool is a CLI (Command Line Interface) utility designed to facilitate interactions with the Lotus Node and Miner metadata. This tool allows users to retrieve version information in either JSON or text format. The Lotus Release Tool was developed as a part of the release process automation initiative and is used in the GitHub Actions workflows related to the release process.

## Features

- Retrieve version information for the Lotus Node.
- Retrieve version information for the Lotus Miner.
- Output logs in JSON or text format.

## Installation

To install the Lotus Release Tool, you need to have Go installed on your system.

1. Build the tool:
    ```sh
    go build -o release ./cmd/release
    ```

## Usage

The `release` tool provides several commands and options to interact with the Lotus Node and Miner.

### Basic Commands

- **Node Version**: Retrieve the Lotus Node version.
    ```sh
    ./release node version
    ```

- **Miner Version**: Retrieve the Lotus Miner version.
    ```sh
    ./release miner version
    ```

### Options

- **--json**: Format output as JSON.
    ```sh
    ./release --json node version
    ```

## Example

Retrieve the Lotus Node version with JSON formatted output:
```sh
./release --json node version
```

Retrieve the Lotus Miner version with text formatted output:
```sh
./release miner version
```

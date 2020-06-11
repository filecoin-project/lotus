# Setup Troubleshooting

## Config: Clearing data

Here is a command that will delete your chain data, stored wallets, stored data and any miners you have set up:

```sh
rm -rf ~/.lotus ~/.lotusstorage
```

This command usually resolves any issues with running `lotus` but it is not always required for updates. We will share information about when resetting your chain data and miners is required for an update in the future.

## Error: Failed to connect bootstrap peer

```sh
WARN  peermgr peermgr/peermgr.go:131  failed to connect to bootstrap peer: failed to dial : all dials failed
  * [/ip4/147.75.80.17/tcp/1347] failed to negotiate security protocol: connected to wrong peer
```

- Try running the build steps again and make sure that you have the latest code from GitHub.

```sh
ERROR hello hello/hello.go:81 other peer has different genesis!
```

- Try deleting your file system's `~/.lotus` directory. Check that it exists with `ls ~/.lotus`.

```sh
- repo is already locked
```

- You already have another lotus daemon running.

## Config: Open files limit

On most systems you can check the open files limit with:

```sh
ulimit -n
```

You can also modify this number by using the `ulimit` command. It gives you the ability to control the resources available for the shell or process started by it. If the number is below 10000, you can change it with the following command prior to starting the Lotus daemon:

```sh
ulimit -n 10000
```

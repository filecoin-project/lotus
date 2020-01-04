# Updating Lotus

If you installed Lotus on your machine, you can upgrade to the latest version by doing the following:

```sh
# get the latest
git pull origin master

# clean and remake the binaries
make clean && make build
```

Sometimes when you run Lotus after a pull, certain commands such as `lotus daemon` may break.

Here is a command that will delete your chain data, stored wallets and any miners you have set up:

```sh
rm -rf ~/.lotus ~/.lotusstorage
```

This command usually resolves any issues with running `lotus` commands but it is not always required for updates. We will share information about when resetting your chain data and miners is required for an update in the future.

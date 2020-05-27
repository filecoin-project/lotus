# Lotus and Miner as a systemd service

Lotus is capable of running as a systemd service daemon. You can find installable service files for systemd in the [lotus repo](https://github.com/filecoin-project/lotus/tree/master/scripts) as files with `.service` extension. In order to install these service files, you can copy these `.service` files to the default systemd service path.

## Installing via `make`

NOTE: Before using lotus and lotus-miner as systemd services, don't forget to `sudo make install` to ensure the binaries are accessible by the root user.

If your host uses the default systemd service path, it can be installed with `sudo make install-services`:

```sh
$ sudo make install-services
```

## Interacting with service logs

Logs from the services can be reviewed using `journalctl`.

### Follow logs from a specific service unit

```sh
$ sudo journalctl -u lotus-daemon -f
```

### View logs in reverse order

```sh
$ sudo journalctl -u lotus-miner -r
```

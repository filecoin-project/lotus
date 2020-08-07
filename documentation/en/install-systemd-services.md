# Use Lotus with systemd

Lotus is capable of running as a systemd service daemon. You can find installable service files for systemd in the [lotus repo scripts directory](https://github.com/filecoin-project/lotus/tree/master/scripts) as files with `.service` extension. In order to install these service files, you can copy these `.service` files to the default systemd unit load path.

The services expect their binaries to be present in `/usr/local/bin/`. You can use `make` to install them by running:

```sh
$ sudo make install
```

for `lotus` and `lotus-storage-miner` and 

```sh
$ sudo make install-chainwatch
```

for the `chainwatch` tool.

## Installing services via `make`

If your host uses the default systemd unit load path, the `lotus-daemon` and `lotus-miner` services can be installed by running:

```sh
$ sudo make install-services
```

To install the the `lotus-chainwatch` service run:

```sh
$ sudo make install-chainwatch-service
```

You can install all services together by running:

```sh
$ sudo make install-all-services
```

The `lotus-daemon` and the `lotus-miner` services can be installed individually too by running:

```sh
$ sudo make install-daemon-service
```

and 

```sh
$ sudo make install-miner-service
```

### Notes

When installing the `lotus-miner` and/or `lotus-chainwatch` service the `lotus-daemon` service gets automatically installed since the other two services depend on it being installed to run.

All `install-*-service*` commands will install the latest binaries in the lotus build folders to `/usr/local/bin/`. If you do not want to use the latest build binaries please copy the `*.service` files by hand.

## Removing via `make`

All services can beremoved via `make`. To remove all services together run:

```sh
$ sudo make clean-all-services
```

Individual services can be removed by running:

```sh
$ sudo make clean-chainwatch-services
$ sudo make clean-miner-services
$ sudo make clean-daemon-services
```

### Notes

The services will be stoppend and disabled when removed.

Removing the `lotus-daemon` service will automatically remove the depending services `lotus-miner` and `lotus-chainwatch`.


## Controlling services

All service can be controlled with the `systemctl`. A few basic control commands are listed below. To get detailed infos about the capabilities of the `systemctl` command please consult your distributions man pages by running: 

```sh
$ man systemctl
```

### Start/Stop services

You can start the services by running:

```sh
$ sudo systemctl start lotus-daemon
$ sudo systemctl start lotus-miner
$ sudo systemctl start lotus-chainwatch
```

and can be stopped by running:

```sh
$ sudo systemctl stop lotus-daemon
$ sudo systemctl stop lotus-miner
$ sudo systemctl stop lotus-chainwatch
```

### Enabling services on startup

To enable the services to run automatically on startup execute:

```sh
$ sudo systemctl enable lotus-daemon
$ sudo systemctl enable lotus-miner
$ sudo systemctl enable lotus-chainwatch
```

To disable the services on startup run:

```sh
$ sudo systemctl disable lotus-daemon
$ sudo systemctl disable lotus-miner
$ sudo systemctl disable lotus-chainwatch
```
### Notes

Systemd will not let services be enabled or started without their requirements. Starting the `lotus-chainwatch` and/or `lotus-miner` service with automatically start the `lotus-daemon` service (if installed!). Stopping the `lotus-daemon` service will stop the other two services. The same pattern is executed for enabling and disabling the services. 

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

### Log files

Besides the systemd service logs all services save their own log files in `/var/log/lotus/`.

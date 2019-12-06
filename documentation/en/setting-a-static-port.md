# Static Ports

For a **storage deal**, you can set a static port.

## Setup

To change the random **swarm port**, you may edit the `config.toml` file located under `$LOTUS_PATH`. The default location of this file is `$HOME/.lotus`.

To change the port to `1347`:

```sh
[Libp2p]
  ListenAddresses = ["/ip4/0.0.0.0/tcp/1347", "/ip6/::/tcp/1347"]
```

After changing the port value, restart your **daemon**.

## Ubuntu's Uncomplicated Firewall

Open firewall manually:

```sh
ufw allow 1347/tcp
```

Or open and modify the profile located at `/etc/ufw/applications.d/lotus-daemon`:

```sh
[Lotus Daemon]
title=Lotus Daemon
description=Lotus Daemon firewall rules
ports=1347/tcp
```

Then run these commands:

```sh
ufw update lotus-daemon
ufw allow lotus-daemon
```
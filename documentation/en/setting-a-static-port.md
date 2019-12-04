# Static port

For a **storage deal**, you may want to set a static port and open it in your firewall to ensure clients can connect to you.

Lotus binds to a random **swarm port** by default.

## Setup

To change the random **swarm port**, you may edit the `config.toml` file located under `$LOTUS_PATH`. The default location of this file is `$HOME/.lotus`.

Here is an example of changing the port to `1347`.

```sh
[Libp2p]
  ListenAddresses = ["/ip4/0.0.0.0/tcp/1347", "/ip6/::/tcp/1347"]
```

Once you update `config.toml`, restart your **daemon**.

## Ubuntu's Uncomplicated Firewall

Open firewall manually

```sh
# ufw allow 1347/tcp
```

Or open and modify the profile located at `/etc/ufw/applications.d/lotus-daemon`

```sh
[Lotus Daemon]
title=Lotus Daemon
description=Lotus Daemon firewall rules
ports=1347/tcp
```

Then run the following commands

```sh
$ ufw update lotus-daemon
$ ufw allow lotus-daemon
```
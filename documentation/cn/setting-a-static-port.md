# 静态端口

取决于你的网络如何设置，你可能需要设置一个静态端口来成功地连接到peers，并用你的**Lotus存储矿工**执行存储交易。

## 设置

要改变随机**群端口**, 你可以编辑`$LOTUS_STORAGE_PATH`下保存的`config.toml` 文件。此文件的默认保存位置是`$HOME/.lotusstorage`.

将端口改到`1347`:

```sh
[Libp2p]
  ListenAddresses = ["/ip4/0.0.0.0/tcp/1347", "/ip6/::/tcp/1347"]
```

改变端口后，重启你的**守护进程**。

## Ubuntu的防火墙

手动打开防火墙：

```sh
ufw allow 1347/tcp
```

或打开并修改 `/etc/ufw/applications.d/lotus-daemon`中保存的文件：

```sh
[Lotus Daemon]
title=Lotus Daemon
description=Lotus Daemon firewall rules
ports=1347/tcp
```

然后运行以下指令：

```sh
ufw update lotus-daemon
ufw allow lotus-daemon
```

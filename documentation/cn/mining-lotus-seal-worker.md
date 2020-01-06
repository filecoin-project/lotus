# Lotus Seal Worker

**Lotus Seal Worker**是一个额外进程，可以从你的Lotus存储矿工中卸载繁重的处理任务。它可以在与你的`lotus-storage-miner`相同的机器上运行，或者在另一台可以在高速网络上通信的机器上运行。

## 注意：在中国使用Lotus存储矿工

如果你试图在中国使用`lotus-storage-miner`，你应该在你的机器上设置以下**环境变量**：

```sh
IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```

## 开始

运行以下指令，确保你安装了`lotus-seal-worker`

```sh
make lotus-seal-worker
```

## 与存储矿工一起运行

你可能希望在与**Lotus存储矿工**相同的机器上运行 **Lotus Seal Worker**。这会让你可以更容易地设置密封任务的进程优先级低于你更重要的存储矿工进程。

如果要这样做，只需要运行`lotus-seal-worker run`，然后seal worker就会从`LOTUS_STORAGE_PATH`矿工库中自动挑选正确的授权通证。

检查 **Lotus Seal Worker** 是否正确连接到你的存储矿工，可通过运行`lotus-storage-miner info`并检查远程worker数量是否增加：

```sh
why@computer ~/lotus> lotus-storage-miner info
Miner: t0103
Sector Size: 16.0 MiB
Power: 0 B / 16.0 MiB (0%)
Worker use:
        Local: 0 / 2 (+1 reserved)
        **Remote: 0 / 1**
PoSt Submissions: Not Proving
Sectors:  map[Committing:0 Proving:0 Total:0]
```

## 在网络上运行

警告：此设置比本地运行复杂一些。

要使用一台完全独立的电脑来进行密封任务，你可能想要在一台单独的通过局域网连接到你的**Lotus存储矿工**的机器上运行`lotus-seal-worker`。

首先，你需要保证你的`lotus-storage-miner`API在网络上是可访问的。

若要这样做，打开 `~/.lotusstorage/config.toml`（或者你手动设置`LOTUS_STORAGE_PATH`，检查一下该目录），再寻找API字段

默认配置：

```toml
[API]
ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
```

为了让你的节点在局域网可访问，你需要决定你的机器的局域网IP，并改变文件中的`127.0.0.1`到该地址。

一个安全性较低的选择是将其更改为`0.0.0.0`。这将允许任何可以在该端口上连接到您的计算机的人访问[API](https://docs.lotu.sh/en+ API)。他们仍然需要一个认证令牌。

接下来，你需要 [创建一个认证令牌](https://docs.lotu.sh/en+api-scripting-support#generate-a-jwt-46)。所有Lotus API要求认证令牌来保证你的进程安全，可抵御试图进行非认证访问请求的攻击者。

### 连接Lotus Seal Worker

在运行 `lotus-seal-worker`的机器上, 将`STORAGE_API_INFO` 环境变量设置为`TOKEN:STORAGE_NODE_MULTIADDR`. `TOKEN`是我们以上创建的通证，`STORAGE_NODE_MULTIADDR`是`config.toml`中设置的 **Lotus存储矿工**的`multiaddr` 。

这些设置好后，运行：

```sh
lotus-seal-worker run
```

检查**Lotus Seal Worker**是否连接到你的**Lotus存储矿工**，可以运行`lotus-storage-miner info` 并查看远程worker数量是否增加。

```sh
why@computer ~/lotus> lotus-storage-miner info
Miner: t05749
Sector Size: 1 GiB
Power: 0 B / 136 TiB (0.0000%)
  Committed: 1 GiB
  Proving: 1 GiB
Worker use:
  Local: 0 / 1 (+1 reserved)
  **Remote: 0 / 1**
Sectors:  map[Proving:1 Total:1]
```

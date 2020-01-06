# Lotus

Lotus是**Filecoin去中心化存储网络**的一个实现. 你可以运行Lotus软件客户端来加入**Filecoin测试网**.

点击[Filecoin Spec](https://github.com/filecoin-project/specs)以获取Filecoin的更多资细节。

## 在这里我能学到什么？

- 如何在[Arch Linux](https://docs.lotu.sh/en+install-lotus-arch), [Ubuntu](https://docs.lotu.sh/en+install-lotus-ubuntu), [MacOS](https://docs.lotu.sh/en+install-lotus-macos)上下载Lotus
- 加入[Lotus Testnet](https://docs.lotu.sh/en+join-testnet).
- [Storing](https://docs.lotu.sh/en+storing-data) 或 [retrieving](https://docs.lotu.sh/en+retrieving-data) 数据.
- 在你的 [CLI](https://docs.lotu.sh/en+mining)中使用 **Lotus存储矿工** 挖Filecoin。

## Lotus有什么特别？

Lotus按模块构建，在使用同一个进程时保持一个干净的API边界。安装Lotus包括两个不同的程序：

- **Lotus节点**
- **Lotus存储矿工**

**Lotus存储矿工** 意图在管理一个存储矿工实体的机器上运行。并且对于所有的链交互需求，**Lotus存储矿工**通过websocket **JSON-RPC**的API与**Lotus节点**通信。

这样，一个挖矿操作能很容易地运行一个或多个**Lotus存储矿工**，连接到一个或多个**Lotus节点**实体。


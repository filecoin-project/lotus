# 检索数据

> 最近对于这些说明有一个bug报告。如果你刚好遇到任何问题，请创建一个 [GitHub issue](https://github.com/filecoin-project/lotus/issues/new)，会有管理员以他们的最快速度解决这个问题。

以下是你在网络中用**Lotus存储矿工**存储并密封一个**数据CID**后可以运行的操作：

如果你想学习如何在一个矿工上存储**数据CID**，请阅读此处[此处](https://docs.lotu.sh/en+storing-data)指导。

## 通过数据CID寻找

```sh
lotus client find <Data CID>
# LOCAL
# RETRIEVAL <miner>@<miner peerId>-<deal funds>-<size>
```

## 通过数据CID检索

所有字段都需要

```sh
lotus client retrieve <Data CID> <outfile>
```

若outfile不存在，它将会创建在Lotus repository目录中。

这个命令将会启动一个**检索交易**，并将数据写入你的电脑。这个过程可能需要2-10分钟。


# 挖矿故障排查

## 配置：Filecoin证明参数目录

如果你想将**Filecoin证明参数**放到另一个目录中，使用以下环境变量：

```sh
FIL_PROOFS_PARAMETER_CACHE
```

## 错误：无法获取bellman.lock

创建**Bellman lock**文件为一个进程锁定一个GPU。这个bug在文件未恰当清理时出现：

```sh
mining block failed: computing election proof: github.com/filecoin-project/lotus/miner.(*Miner).mineOne
```

当存储矿工无法获取`bellman.lock`时这个bug会出现。要修复它，你需要停止`lotus-storage-miner` 并移除 `/tmp/bellman.lock`。

## 错误：获取API endpoint失败

```sh
lotus-storage-miner info
# WARN  main  lotus-storage-miner/main.go:73  failed to get api endpoint: (/Users/myrmidon/.lotusstorage) %!w(*errors.errorString=&{API not running (no endpoint)}):
```

如果你看到这个，意味着你的 **Lotus存储矿工** 还没准备好。你需要完成[链同步](https://docs.lotu.sh/en+join-testnet)。

## 错误：你的电脑可能不够快

```sh
CAUTION: block production took longer than the block delay. Your computer may not be fast enough to keep up
```

如果你看到这个，说明你的电脑太慢了，你的区块未包含在链上，而且你不会收到任何奖励。

## 错误：设备没有剩余空间

```sh
lotus-storage-miner pledge-sector
# No space left on device (os error 28)
```

如果你看到这个，说明`pledge-sector`写入太多数据到`$TMPDIR`，而`$TMPDIR`默认是根分区（Linux设置普遍如此）。通常你的根分区不会获得最大的存储分区，因此你需要把环境变量更改到另一个路径。

## 错误：未使用GPU

如果你怀疑你的GPU未被使用，首先确认它是否按照[testing configuration page](hardware-mining.md)的要求正确配置。一旦你完成这步（并且在必要时适当设置`BELLMAN_CUSTOM_GPU`），你可以通过运行一个快速的lotus-bench基准验证你的GPU是否被使用。

首先，在一个终端运行`nvtop`以查看GPU利用率，然后在另一个终端运行：

```sh
lotus-bench --sector-size=1024
```

这个进程使用相当数量的GPU，通常需要0-4分钟时间才能完成。如果整个进程中，你从lotus nvtop中未看见任何活动，很可能你的GPU有哪里配置错误了。

## 检查同步进度

你可以使用这个指令来检查你的同步还差了多少：

```sh
date -d @$(./lotus chain getblock $(./lotus chain head) | jq .Timestamp)
```

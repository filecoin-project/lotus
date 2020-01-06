# 存储挖矿

以下是学习如何运行存储挖矿的一些指导。请在[此处](https://docs.lotu.sh/en+hardware-mining)查看硬件规格。

首次尝试进行存储挖矿的人可以先[加入测试网](https://docs.lotu.sh/en+join-testnet)，获取有用信息。

## 注意：在中国使用Lotus存储矿工

如果你试图在中国使用`lotus-storage-miner`，你应该在你的机器上设置以下**环境变量**：

```sh
IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```

## 开始

请用以下指令测试，保证你的钱包中至少有一个**BLS地址** 。

```sh
lotus wallet list
```

用你的钱包地址：

- 访问[faucet](https://lotus-faucet.kittyhawk.wtf/miner.html)
- 点击“创建矿工
- 不要刷新该页面。这个操作可能会花一段时间

这个任务完成后你将会看到：

```sh
New storage miners address is: <YOUR_NEW_MINING_ADDRESS>
```

## 初始化存储矿工

在一个CLI窗口中，用以下指令来启动你的矿工：

```sh
lotus-storage-miner init --actor=ACTOR_VALUE_RECEIVED --owner=OWNER_VALUE_RECEIVED
```

例子：

```sh
lotus-storage-miner init --actor=t01424 --owner=t3spmep2xxsl33o4gxk7yjxcobyohzgj3vejzerug25iinbznpzob6a6kexcbeix73th6vjtzfq7boakfdtd6a
```

这个操作完成需要花一段时间。

## 挖矿

挖矿运行以下指令：

```sh
lotus-storage-miner run
```

如果你正在下载**Filecoin证明参数**，下载可能会花一段时间。

获取你的矿工的信息：

```sh
lotus-storage-miner info
# example: miner id `t0111`
```

**密封** 任意数据以开始产生**PoSt**:

```sh
lotus-storage-miner pledge-sector
```

- 警告：按Linux配置，这个指令将会把数据写入`$TMPDIR`，这通常不是最大的分区。可能的情况下，你应该将该路径指向最大的分区。

获取**矿工算力** 和**扇区使用**:

```sh
lotus-storage-miner state power
# returns total power

lotus-storage-miner state power <miner>

lotus-storage-miner state sectors <miner>
```

## 改变昵称

用以下指令更新`~/.lotus/config.toml`:

```sh
[Metrics]
Nickname="fun"
```

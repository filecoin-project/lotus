# 协议实验室标准测试配置

> 这个文件页面描述了协议实验室团队在Lotus上测试 **Lotus存储矿工**的标准测试配置。不能保证这些测试配置适用于主网的Filecoin存储挖矿。如果你需要买新的硬件来参与Filecoin测试网，我们建议您不要购买超过测试网所需的硬件。参阅此文了解更多[Protocol Labs Standard Testing Configuration post](https://filecoin.io/blog/filecoin-testnet-mining/).

挖块要求的**扇区大小**和**最小抵押存储**是两个非常重要的Filecoin测试网参数，影响着硬件的选择。在测试网期间我们会继续优化所有参数。

因此，必须提醒您，我们的Filecoin主网标准测试配置可能并且将会改变。

## 配置案例

以下是Lotus上密封32GB扇区的最低设置案例：

- 硬盘驱动空间2TB
- 8核CPU
- 128GB RAM

注意：密封1GB的扇区不要求这么高的配置；但是随着我们对密封32GB扇区的性能的改善，很有可能以后将不支持1GB。

## 测试网发现

- 如果你只有128GB RAM，在一个SSD上构建256GB的**NVMe** swap将会帮助你避免在挖矿时出现内存不足的问题。

## 基准GPU

要获取**区块奖励**必须有GPU。以下几种GPU已被证实可以快速生成**SNARKs** 以成功在Lotus测试网挖块。

- GeForce RTX 2080 Ti
- GeForce RTX 2080 SUPER
- GeForce RTX 2080
- GeForce GTX 1080 Ti
- GeForce GTX 1080
- GeForce GTX 1060

## 测试其他GPU

如果你想测试一个非明确支持的GPU，请使用以下全局**环境变量**：

```sh
BELLMAN_CUSTOM_GPU="<NAME>:<NUMBER_OF_CORES>"
```

以下是测试1536核心的GeForce GTX 1660 Ti的案例：

```sh
BELLMAN_CUSTOM_GPU="GeForce GTX 1660 Ti:1536"
```

要获取你的GPU的核心数，需要检查你的卡的规格。

## 基准

想要测试并贡献硬件设置给**Filecoin测试网**的人，这里是[基准工具](https://github.com/filecoin-project/lotus/tree/testnet-staging/cmd/lotus-bench) 和 [GitHub issue讨论](https://github.com/filecoin-project/lotus/issues/694)。


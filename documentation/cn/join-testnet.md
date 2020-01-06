## 加入测试网

任何人都可以创建一个 **Lotus 节点** 并连接到 **Lotus 测试网**. 这是最好的研究当前的CLI和 **Filecoin去中心化存储市场**的方法。

如果你已经安装了旧版本，出错时你可能需要清除现存的链数据、已保存的钱包和矿工，你可以用这个指令：

```sh
rm -rf ~/.lotus ~/.lotusstorage
```

## 注意：在中国使用Lotus节点

如果你试图在中国使用`lotus`，你应该在你的机器上设置这个**环境变量**：

```sh
IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```

## 开始

用 `./build`中的默认配置启动**守护进程**

```sh
lotus daemon
```

在另一个终端窗口，检查你与点的连接：

```sh
lotus net peers | wc -l
```

要连接到网络，你需要连接到至少1个点。如果你看见的是零个点，请阅读我们的 [检修节点](https://docs.lotu.sh/en+setup-troubleshooting)。

## 链同步

当守护进程在运行中时，下一个步就是同步链。运行以下指令来启动链同步进程。要查看当前的链高度，访问 [网络数据页面](http://stats.testnet.filecoin.io/)

```sh
lotus sync wait
```

- 此步骤会耗费30分钟-几小时的时间。
- 完成后，你将可以执行**Lotus测试网**操作。

## 创建你的首个地址

用BLS签名格式初始化一个钱包：

```sh
lotus wallet new bls
```

以下是所返回的例子：

```sh
t3vhfme4qfvegqaz7m7q6o6afjcs67n6kpzv7t2eozio4chwpafwa2y4l7zhwd5eom7jmihzdg4s52dpvnclza
```

- 访问[faucet](https://lotus-faucet.kittyhawk.wtf/funds.html)来添加资金.
- 粘贴你创建的地址
- 点击发送按钮

## 检查钱包地址余额

Lotus测试网的钱包余额是**FIL**，FIL的最小面值是**attoFil**，1 attoFil = 10^-18 FIL

```sh
lotus wallet balance <YOUR_NEW_ADDRESS>
```

如果你的**链**没有完全同步，你的钱包中就看不到任何attoFIL。

## 发送FIL给另一个钱包

用以下指令发送FIL给另一个钱包：

```
lotus send <target> <amount>
```

## 监控图表

要查看最新的网络活动，包括**链区块高度，区块高度，出块时间，全网算力，最大区块生产者矿工**，请查看[监控图表](https://stats.testnet.filecoin.io)。
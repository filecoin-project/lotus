# 存储数据

> 有关这些说明的最新bug报告。如果你刚好遇到任何问题，请创建一个 [GitHub issue](https://github.com/filecoin-project/lotus/issues/new)，会有管理员以他们的最快速度解决这个问题。

以下是如何在**Lotus测试网**存储数据的说明。

## 添加一个本地文件

添加一个本地文件使你可以在**Lotus测试网**进行矿工交易。

```sh
lotus client import ./your-example-file.txt
```

成功后，这个指令将会返回一个**数据CID**。

## 列出你的本地文件

运行这个指令，按字节的`CID`, `name`, `size`查看文件列表和 `status`：

```sh
lotus client local
```

输出例子：

```sh
bafkreierupr5ioxn4obwly4i2a5cd2rwxqi6kwmcyyylifxjsmos7hrgpe Development/sample-1.txt 2332 ok
bafkreieuk7h4zs5alzpdyhlph4lxkefowvwdho3a3pml6j7dam5mipzaii Development/sample-2.txt 30618 ok
```

## 在Lotus测试网进行矿工交易

获取可以存储数据的所有矿工列表：

```sh
lotus state list-miners
```

获取你希望其存储数据的矿工的请求：

```sh
lotus client query-ask <miner>
```

将**数据CID**存到一个矿工处：

```sh
lotus client deal <Data CID> <miner> <price> <duration>
```

- 用attoFIL计价。
- `duration`代表着矿工会保持你的文件托管的时间。以块表示，每个块代表45秒。

成功后，这个指令将会返回一个**数据CID**。

存储矿工先**密封**文件，文件才可检索。若**Lotus存储矿工**不在一个被委派密封的机器上运行，这个进程将会花费很长时间件。
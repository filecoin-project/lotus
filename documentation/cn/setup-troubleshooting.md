# 设置故障排查

## 清除数据

以下指令会删除你的链数据、已保存的钱包、已保存的数据和你设置的任何矿工：

```sh
rm -rf ~/.lotus ~/.lotusstorage
```

这个指令通常解决任何运行lotus的问题，但更新并不一定需要该指令。将来，我们会分享升级需要的重置链数据和矿工时的信息。

## 错误：连接到bootstrap点失败

```sh
WARN  peermgr peermgr/peermgr.go:131  failed to connect to bootstrap peer: failed to dial : all dials failed
  * [/ip4/147.75.80.17/tcp/1347] failed to negotiate security protocol: connected to wrong peer
```

- 尝试重新运行“Build”步骤，确保你已经获取了GitHub中的最新代码。

```sh
ERROR hello hello/hello.go:81 other peer has different genesis!
```

- 尝试删除你的文件系统中的`~/.lotus`目录。检查它是否有 `ls ~/.lotus`。

```sh
- repo is already locked
```

- 你已经在运行另一个lotus守护进程了。

## 警告：获取信息失败

会出现一些错误，无法停止Lotus运行：

```sh
ERROR chainstore  store/store.go:564  get message get failed: <Data CID>: blockstore: block not found
```

- 有人在请求你没有的**数据CID**。

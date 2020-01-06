# 存储故障排查		

## 错误：路由：未找到

```sh
WARN  main  lotus/main.go:72  routing: not found
```

- 矿工不在线

## 错误：启动交易失败

```sh
WARN  main  lotus/main.go:72  failed to start deal: computing commP failed: generating CommP: Piece must be at least 127 bytes
```

- 最小文件尺寸为127字节。

## 错误：检索中收到0kb文件的响应

只有密封的文件才可被检索。矿工可以用以下指令检查密封进度：

```sh
lotus-storage-miner sectors list
```

密封完成后，`pSet: NO` 会变成`pSet: YES`。这个时候就可以在**Lotus存储矿工** [检索](https://docs.lotu.sh/en+retrieving-data)到**数据CID**。


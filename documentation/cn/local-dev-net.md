# 设置本地Devnet

以调试模式建立Lotus二进制文件，将可以使用1024字节扇区。

```sh
make debug
```

预密封一些扇区：

```sh
./lotus-seed pre-seal --sector-size 1024 --num-sectors 2
```

创建创世区块，启动首个节点：

```sh
./lotus daemon --lotus-make-random-genesis=dev.gen --genesis-presealed-sectors=~/.genesis-sectors/pre-seal-t0101.json --bootstrap=false
```

建立创世矿工：

```sh
./lotus-storage-miner init --genesis-miner --actor=t0101 --sector-size=1024 --pre-sealed-sectors=~/.genesis-sectors --nosync
```

最后，启动矿工：

```sh
./lotus-storage-miner run --nosync
```

如果一切进行顺利，你将会运行自己的本地Lotus Devnet.


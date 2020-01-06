# 升级Lotus

如果你已经在你的机器中安装了Lotus，那么进行以下步骤就可以升级到最新版本：

```sh
# get the latest
git pull origin master

# clean and remake the binaries
make clean && make build
```

有时候当你在pull之后运行Lotus时，例如运行`lotus daemon` ，报错。

以下指令将删除你所有的链数据，存储钱包和构建的矿工。

```sh
rm -rf ~/.lotus ~/.lotusstorage
```

这指令通常通过运行`lotus` 指令解决任何问题，但不是升级就一定要运行该指令。将来我们会分享一些关于升级所需的链数据和矿工重置的信息。
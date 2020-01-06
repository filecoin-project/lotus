# MacOS安装说明

## 获取XCode命令行工具

查看你是否已经通过CLI安装了XCode命令行工具，运行：

```sh
xcode-select -p
```

若此指令返回一个路径，你可以进行下一步；否则，你要通过CLI安装这个工具，运行：

```sh
xcode-select --install
```

若要升级，运行：

```sh
sudo rm -rf /Library/Developer/CommandLineTools
xcode-select --install
```

## 获取HomeBrew

我们建议MacOS用户使用[HomeBrew](https://brew.sh) 来安装每个必要的包。

检查你是否有HomeBrew:

```sh
brew -v
```

如果你安装了HomeBrew，这个指令就会返回一个版本号；否则结果为空。

在你的终端输入这个指令会安装HomeBrew：

```sh
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
```

用这个指令 `brew install` 来安装以下安装包:

```sh
brew install go bzr jq pkg-config rustup
```

克隆

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

构建(Build)

```sh
make clean && make all
sudo make install
```

安装Lotus后，你可以从你的CLI直接运行`lotus` 来查看使用说明文件。接下来，你可以加入 [Lotus测试网](https://docs.lotu.sh/en+join-testnet)。
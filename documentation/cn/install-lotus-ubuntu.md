# Ubuntu安装说明

接下来步骤将会安装以下依赖项：

- go (1.13 or higher)
- gcc (7.4.0 or higher)
- git (version 2 or higher)
- bzr (some go dependency needs this)
- jq
- pkg-config
- opencl-icd-loader
- opencl driver (like nvidia-opencl on arch) (for GPU acceleration)
- opencl-headers (build)
- rustup (proofs build)
- llvm (proofs build)
- clang (proofs build)

运行

```sh
sudo apt update
sudo apt install mesa-opencl-icd ocl-icd-opencl-dev
```

构建(Build)

```sh
sudo add-apt-repository ppa:longsleep/golang-backports
sudo apt update
sudo apt install golang-go gcc git bzr jq pkg-config mesa-opencl-icd ocl-icd-opencl-dev
```

克隆

```sh
git clone https://github.com/filecoin-project/lotus.git
cd lotus/
```

安装

```sh
make clean && make all
sudo make install
```

安装Lotus后，你可以从你的CLI直接运行`lotus` 来查看使用说明文件。接下来，你可以加入 [Lotus测试网](https://docs.lotu.sh/en+join-testnet)。

# API脚本支持

你可能想将运行 **Lotus存储矿工**或 **Lotus节点** 的工作分配给其他机器。以下是设置必要的授权和环境变量的方法：

## 生成JWT

用以下指令为你的环境变量生成一个JWT：

```sh
lotus auth create-token --perm admin
lotus-storage-miner auth create-token --perm admin
```

## 环境变量

环境变量是当前的shell定义的变量，由任何子shell或子进程继承。环境变量用于传递信息到shell孵化的进程中。

用你生成的JWT，你可以将它和**多重地址**分配给合适的环境变量。

```sh
# Lotus Node
FULLNODE_API_INFO="JWT_TOKEN:/ip4/127.0.0.1/tcp/1234/http"

# Lotus Storage Miner
STORAGE_API_INFO="JWT_TOKEN:/ip4/127.0.0.1/tcp/2345/http"
```

- **Lotus节点**的`多重地址`存储在`~/.lotus/api`
- 默认通证存储在`~/.lotus/token`
- **Lotus存储矿工**的`多重地址` 存储在 `~/.lotusstorage/config`.
- 默认通证存储在`~/.lotusstorage/token`

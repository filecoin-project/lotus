# API

此文档是如何进行API调用的概览。

**JSON-RPC**包的实现细节[在此](https://github.com/filecoin-project/lotus/tree/master/lib/jsonrpc)查看。

## 概览：如何通过修改config.toml来更改API endpoint？

API请求默认地址为`127.0.0.1:1234`，可通过修改`lotus/config.toml`文件进行更改.

- `http://[api:port]/rpc/v0` - HTTP endpoint
- `ws://[api:port]/rpc/v0` - Websocket endpoint
- `PUT http://[api:port]/rest/v0/import` - 文件导入，需要写入权限

## 用法

你可以根据你的需要查看相应的文档：

- [Both Lotus node + storage miner APIs](https://github.com/filecoin-project/lotus/blob/master/api/api_common.go)
- [Lotus node API](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go)
- [Storage miner API](https://github.com/filecoin-project/lotus/blob/master/api/api_storage.go)

所有API所需的权限：[api/struct.go](https://github.com/filecoin-project/lotus/blob/master/api/apistruct/struct.go).

## 如何进行API请求？

示范：构建一个API请求

将调用[api/api.go](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go)中的`ChainHead`方法。

```go
ChainHead(context.Context) (*types.TipSet, error)
```

执行 CURL 指令。其中， `{ "method": "Filecoin.ChainHead" }`就调用了 `ChainHead`方法:

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method": "Filecoin.ChainHead", "params": [], "id": 3 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

如果请求的时候要求授权，则添加一个授权header：

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer $(cat ~/.lotusstorage/token)" \
     --data '{ "jsonrpc": "2.0", "method": "Filecoin.ChainHead", "params": [], "id": 3 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

> 将来我们会添加一个工具，简化API请求的创建和试验的过程。

## CURL授权

授权请求，只需要在HTTP header中加入**JWT** ，例如：

```sh
-H "Authorization: Bearer $(cat ~/.lotusstorage/token)"
```

**Lotus节点**的通证存储在`~/.lotus/token`，**Lotus**存储矿工的通证存储在`~/.lotusstorage/token`。

## 如何生成通证？

用以下指令即可生成自定义权限的JWT：

```sh
# Lotus Node
lotus auth create-token --perm admin

# Lotus Storage Miner
lotus-storage-miner auth create-token --perm admin
```

## 应该使用哪种授权？

浏览[api/struct.go](https://github.com/filecoin-project/lotus/blob/master/api/struct.go)时，你会碰到以下几种类型：

- `read` - 可读取节点状态，无私有数据。
- `write` - 可对本地库 / 链进行写入，还拥有`read`的权限。
- `sign` - 可用钱包中存储的私钥进行签名， 还拥有`read` 和`write` 权限。
- `admin` - 拥有管理权限, 还拥有`read` 、`write` 和`sign` 权限。

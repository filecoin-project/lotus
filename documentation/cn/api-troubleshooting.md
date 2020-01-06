# API故障排查

## 类型：参数

`参数`必须是一个数组。如果没有`参数`你需要传入一个空的数组。

## 类型：TipSet

对于一些方法，例如`Filecoin.StateMinerPower`，需要传入`TipSet`的参数，可以通过传入`null`来使用当前链头。

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method": "Filecoin.StateMinerPower", "params": ["t0101", null], "id": 3 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

## 类型：发送CID

如果你没有将CID序列化为一个 [JSON IPLD link](https://did-ipid.github.io/ipid-did-method/#txref), 你会收到一个错误。以下是一个失败的CURL请求的例子：

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method":"Filecoin.ClientGetDealInfo", "params": ["bafyreiaxl446wlnu6t6dpq4ivrjf4gda4gvsoi4rr6mpxau7z25xvk5pl4"], "id": 0 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

要修复它，就要将`参数`属性改为：

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method":"Filecoin.ClientGetDealInfo", "params": [{"/": "bafyreiaxl446wlnu6t6dpq4ivrjf4gda4gvsoi4rr6mpxau7z25xvk5pl4"}], "id": 0 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

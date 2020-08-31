# API Troubleshooting

## Types: params

`params` must be an array. If there are no `params` you should still pass an empty array.

## Types: TipSet

For methods such as `Filecoin.StateMinerPower`, where the method accepts the argument of the type `TipSet`, you can pass `null` to use the current chain head.

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method": "Filecoin.StateMinerPower", "params": ["t0101", null], "id": 3 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

## Types: Sending a CID

If you do not serialize the CID as a [JSON IPLD link](https://did-ipid.github.io/ipid-did-method/#txref), you will receive an error. Here is an example of a broken CURL request:

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method":"Filecoin.ClientGetDealInfo", "params": ["bafyreiaxl446wlnu6t6dpq4ivrjf4gda4gvsoi4rr6mpxau7z25xvk5pl4"], "id": 0 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

To fix it, change the `params` property to:

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method":"Filecoin.ClientGetDealInfo", "params": [{"/": "bafyreiaxl446wlnu6t6dpq4ivrjf4gda4gvsoi4rr6mpxau7z25xvk5pl4"}], "id": 0 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

import util from 'ipld-dag-cbor'
import { Buffer } from 'buffer'
import { Tagged } from 'borc'

async function pushMessage(client, from, inmsg) {
    if(!inmsg.GasLimit) {
        inmsg.GasLimit = "1000"
    }
    if(!inmsg.GasPrice) {
        inmsg.GasPrice = "0"
    }
    if(!inmsg.Params) {
        inmsg.Params = "oA==" // 0b101_00000: empty cbor map: {}
    }
    if(!inmsg.Value) {
        inmsg.Value = "0"
    }
    if(!inmsg.Method) {
        inmsg.Method = 0
    }

    inmsg.Nonce = await client.call('Filecoin.MpoolGetNonce', [from])

/*    const msg = [
        inmsg.To,
        inmsg.From,

        inmsg.Nonce,

        inmsg.Value,

        inmsg.GasPrice,
        inmsg.GasLimit,

        inmsg.Method,
        Buffer.from(inmsg.Params, 'base64'),
    ]*/

    const signed = await client.call('Filecoin.WalletSignMessage', [from, inmsg])

    console.log(signed)

    await client.call('Filecoin.MpoolPush', [signed])
}

export default pushMessage